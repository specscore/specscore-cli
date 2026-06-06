---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Init (CLI)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/init?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/init?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/init?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/init?op=request-change) |

**Status:** Approved

## Summary

`specscore init` scaffolds a SpecScore-managed project root: writes `specscore.yaml` with the mandatory schema-pointer comment on line 1, creates the `spec/` tree with lint-clean Ideas and Features indexes, and optionally creates `spec/research/` and `spec/decisions/` trees behind opt-in flags. Refuses to clobber an existing `specscore.yaml` unless `--force`. Idempotent on partial state — completes missing pieces without erroring on what's already there.

## Synopsis

```
specscore init [--title <text>] [--host <h>] [--org <o>] [--repo <r>] [-i|--interactive] [--force] [--project <path>]
```

## Problem

A new SpecScore project today is bootstrapped by hand: the author writes `specscore.yaml` with the right schema-pointer comment, creates `spec/ideas/` and `spec/features/` directories, and authors lint-clean index READMEs for each — all to satisfy the canonical [Repo Config](../../repo-config/README.md), [Ideas Index](../../ideas-index/README.md), and [Features Index](../../features-index/README.md) Features. Get any one of those wrong and `specscore spec lint` fails on first run, before any actual specs exist.

`init` removes that friction. It produces a lint-clean root by construction, so authors edit content rather than fight structure. It mirrors the same pattern as [`idea new`](../idea/new/README.md): scaffolders that hide the schema, refuse to clobber, exit cleanly with documented codes.

## Behavior

### Project root resolution

The command operates on a single project root. The root is either explicit or auto-detected.

#### REQ: project-flag-or-cwd

`--project <path>` MUST resolve a project root explicitly. When absent, the project root MUST be the current working directory. The command does NOT walk upward looking for an existing root (that's a different concern — finding a project, not initializing one).

#### REQ: project-root-must-exist

The resolved project root MUST be an existing directory. If the path does not exist or is not a directory, exit `2` with a message naming the path. The command MUST NOT auto-create the project root itself — that's a `mkdir` concern outside the CLI's job.

### Conflict detection

`specscore.yaml` is the marker file for a SpecScore-managed project. Its presence indicates "already initialized" and triggers the conflict-vs-force decision.

#### REQ: conflict-detection

If `<project-root>/specscore.yaml` exists and `--force` is NOT supplied, the command MUST exit `1` (Conflict) with a message naming the path. No partial write may occur before the conflict check completes — every other artifact (`spec/ideas/README.md`, etc.) MUST also remain untouched.

#### REQ: force-overrides-conflict

`--force` opts in to overwriting an existing `specscore.yaml`. With `--force`, an existing config is replaced atomically (single-file write) with the newly scaffolded version. `--force` does NOT delete or rewrite anything else: existing `spec/ideas/`, `spec/features/`, or other project content is preserved untouched.

### Idempotent partial-state resume

When `specscore.yaml` is absent but parts of the `spec/` tree already exist (e.g., a previous `specscore idea new` ran in an unconfigured directory), init completes the missing pieces without erroring on present ones.

#### REQ: partial-state-resume

When `specscore.yaml` does not exist at the project root (so the conflict check passes), init MUST create it AND create any missing index files at the file level: if `spec/ideas/README.md` does not exist as a file, init creates it; if it does exist, init preserves it byte-identical. The same rule applies to `spec/features/README.md`. The check is per-file, not per-directory: an existing `spec/ideas/` directory containing other files (e.g., one or more Idea artifacts) but lacking a `README.md` MUST result in the README being created without disturbing the sibling files. This is idempotent rerun — if a previous init exited mid-flight, re-running completes the bootstrap without the user needing to clean up.

### `specscore.yaml` content

The generated config MUST conform to the canonical [Repo Config](../../repo-config/README.md) Feature.

#### REQ: schema-pointer-line-1

Line 1 of the generated `specscore.yaml` MUST be exactly:

```yaml
# SpecScore Repo Config Schema: https://specscore.md/repo-config
```

This satisfies [`repo-config#req:schema-header-comment`](../../repo-config/README.md). Any deviation — whitespace, additional content, missing comment — is a contract violation.

#### REQ: project-block-from-flags-and-inference

The `project:` block MUST be populated from a combination of flags and inference, in this precedence order (highest first):

1. Explicit flag value (`--title`, `--host`, `--org`, `--repo`)
2. Interactive prompt response (when `-i` is supplied and the field has no flag value)
3. Inferred value: `host`/`org`/`repo` parsed from `git remote get-url origin` if a remote is configured AND parseable as a recognized SSH or HTTPS Git URL form; `title` defaulting to the basename of the project root directory
4. Field omitted from the output (the canonical Repo Config schema treats every `project:` field as optional)

When all four sources yield no value for a field, the field MUST be omitted from `project:` rather than emitted with an empty string. When `project:` itself ends up with no fields, the entire block MAY be omitted from the output (matches `repo-config#req:project-block-optional`).

A *missing* git remote (no `origin` configured) is NOT an error — inference simply skips step 3 for that field and falls through to step 4 (omit). A *present-but-unparseable* git remote (e.g., a non-Git URL or an unrecognized form) is also NOT an error — same fall-through behavior. The exit-code-`2` "unparseable git remote" condition (see Exit codes) applies only when the user explicitly relies on inference (no flag overrides) AND the remote URL is malformed in a way that prevents *any* inference, leaving the user's intent ambiguous; this is a tightening reserved for future Feature revisions if user reports show silent omission causes confusion. In MVP, missing or unparseable remotes silently fall through to omission.

### Spec-tree scaffolding

The `spec/` tree's mandatory subdirectories and indexes are created on every successful init.

#### REQ: ideas-tree-created

Init MUST create `spec/ideas/README.md` as a lint-clean Ideas Index per the [Ideas Index](../../ideas-index/README.md) Feature: title `# Ideas`, `## Index` section with an empty index table (header row only), `## Open Questions` containing `None at this time.`, and the adherence footer `*This document follows the https://specscore.md/ideas-index-specification*`.

#### REQ: features-tree-created

Init MUST create `spec/features/README.md` as a lint-clean Features Index per the [Features Index](../../features-index/README.md) Feature: title `# Features`, `## Index` section with an empty index table (header row only), `## Open Questions` containing `None at this time.`, and the adherence footer.

#### REQ: optional-subtrees-out-of-scope

`spec/research/`, `spec/decisions/`, and any other optional spec subtrees are NOT scaffolded by `init` in this MVP. The canonical Index Features for those subtrees do not yet exist, so init has no schema to target. Authors who need them today create them by hand (`mkdir spec/research`); future Feature revisions add `--with-research` / `--with-decisions` flags once the canonical Indexes are specified.

### Lint-clean output guarantee

Every artifact init writes MUST satisfy `specscore spec lint`.

#### REQ: lint-clean-on-create

After `specscore init` completes successfully, an immediate `specscore spec lint` MUST exit `0`. No new violations may be introduced by init's writes. This guarantee covers `specscore.yaml`, the mandatory indexes, and any optional indexes created via `--with-*` flags.

### Interactive mode

`-i` switches the command from non-interactive (flags + inference + omission) to step-by-step prompts.

#### REQ: interactive-mode

`-i` / `--interactive` MUST prompt for each project-metadata field on stdin: `title`, `host`, `org`, `repo`. The flag value (when supplied) is the prompt's default; pressing Enter accepts the default. Empty input MUST be treated as "omit this field." Non-interactive runs use flag values plus inference plus omission without prompting.

#### REQ: interactive-non-tty

When `-i` is supplied but stdin is not a TTY (e.g., the command is invoked in a script or with stdin closed), the command MUST exit `2` with a stderr message naming the conflict (`-i requires an interactive terminal; either run in a TTY or omit -i to use flags + inference`). The command MUST NOT silently degrade to non-interactive behavior — that would hide a likely user error.

#### REQ: interactive-flags-only-mode-controls

`-i` is metadata-only. Mode-control flags (`--force`, `--project`) MUST NOT be prompted for; they are flag-only because they affect command semantics rather than artifact content.

### Exit codes

| Code | Condition |
|---|---|
| `0` | Init completed; all expected artifacts present and lint-clean |
| `1` | `specscore.yaml` already exists and `--force` not supplied |
| `2` | Invalid `--project` path, invalid flag value, or unparseable git remote when no overrides supplied |
| `10` | Unexpected I/O failure while writing |

#### REQ: exit-code-discipline

The command MUST exit with one of the documented codes above and no other code (e.g., never `127` for an unexpected condition). Unexpected conditions surface as code `10` with a descriptive stderr message.

## Parameters

This command takes no positional arguments. All inputs are flag-driven.

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [Repo Config](../../repo-config/README.md) | Source of truth for `specscore.yaml` schema. `init` produces a config conforming to it. |
| [Ideas Index](../../ideas-index/README.md) | Source of truth for `spec/ideas/README.md` shape. `init` produces an empty-but-lint-clean instance. |
| [Features Index](../../features-index/README.md) | Source of truth for `spec/features/README.md` shape. Same pattern as Ideas Index. |
| [CLI](../README.md) | Inherits the shared exit-code contract and `--project` flag convention. `init` is the only CLI command that creates `specscore.yaml` itself; every other CLI command assumes it exists. |
| [Idea New (CLI)](../idea/new/README.md) | Sibling scaffolder. `idea new` requires an existing project root (i.e., a successful prior `init`). The two flows together form the full bootstrap → first-artifact path. |
| [`init/` skill](https://github.com/specscore/specstudio-skills/blob/main/spec/features/skills/init/README.md) (specstudio-skills) | Agent-side wrapper that delegates to this CLI for spec-tree scaffolding. The skill's `prefer-specscore-init-cli` REQ commits to invoking `specscore init` as the preferred path; this Feature is the contract that REQ targets. |

## Acceptance Criteria

### AC: greenfield-no-remote

**Requirements:** cli/init#req:project-flag-or-cwd, cli/init#req:schema-pointer-line-1, cli/init#req:ideas-tree-created, cli/init#req:features-tree-created, cli/init#req:lint-clean-on-create, cli/init#req:project-block-from-flags-and-inference, cli/init#req:optional-subtrees-out-of-scope

**Given** an empty directory at `/tmp/test-greenfield-no-remote` with `git init` run but NO git remote configured
**When** `cd /tmp/test-greenfield-no-remote && specscore init` runs with no flags
**Then** `specscore.yaml` exists at the project root with line 1 exactly equal to `# SpecScore Repo Config Schema: https://specscore.md/repo-config`; the `project:` block is either absent or contains only `title:` derived from the directory basename (`host:`, `org:`, `repo:` are omitted because no remote is configured and no flags supplied); `spec/ideas/README.md` and `spec/features/README.md` exist as lint-clean indexes (titles `# Ideas` and `# Features`, `## Index` sections with empty header-only tables, `## Open Questions` containing `None at this time.`, and the adherence footers); `spec/research/` and `spec/decisions/` directories are NOT created (out of MVP scope); `specscore spec lint` immediately afterwards exits `0`; the command exits `0`.

### AC: greenfield-with-remote-inference

**Requirements:** cli/init#req:project-block-from-flags-and-inference

**Given** an empty directory at `/tmp/test-greenfield-with-remote` with `git init` run AND `git remote add origin git@github.com:acme/example.git`
**When** `cd /tmp/test-greenfield-with-remote && specscore init` runs with no flags
**Then** `specscore.yaml` exists with a `project:` block containing `host: github.com`, `org: acme`, `repo: example` (inferred from the remote) and `title: test-greenfield-with-remote` (inferred from the basename); the command exits `0`.

### AC: conflict-without-force

**Requirements:** cli/init#req:conflict-detection

**Given** a project root containing an existing `specscore.yaml` (regardless of its content)
**When** `specscore init` runs without `--force`
**Then** the command exits `1`, prints a message naming the path of the existing file, and creates/modifies no other files in the project root. The existing `specscore.yaml` is byte-identical to its pre-invocation state.

### AC: force-overwrites

**Requirements:** cli/init#req:force-overrides-conflict

**Given** a project root with an existing `specscore.yaml` (e.g., a stale one from an earlier abandoned init)
**When** `specscore init --force --title "Acme"` runs
**Then** `specscore.yaml` is replaced with the freshly-scaffolded version, line 1 is the canonical schema-pointer comment, `project.title` reads `Acme`; the command exits `0`. Any pre-existing `spec/ideas/`, `spec/features/`, or unrelated project content is preserved untouched (`--force` does not extend to non-config files).

### AC: partial-state-resume

**Requirements:** cli/init#req:partial-state-resume, cli/init#req:lint-clean-on-create

**Given** a project root that contains an existing `spec/ideas/` directory with one Idea file but NO `specscore.yaml` and NO `spec/features/`
**When** `specscore init` runs (no flags)
**Then** `specscore.yaml` is created, `spec/features/README.md` is created lint-clean, `spec/ideas/README.md` is created lint-clean *only if it did not already exist* (existing pre-Idea-files content is preserved), the command exits `0`, and `specscore spec lint` reports `0` violations against the resulting tree.

### AC: project-metadata-applied

**Requirements:** cli/init#req:project-block-from-flags-and-inference

**Given** a greenfield project root with no git remote configured
**When** `specscore init --title "Acme Service" --host github.com --org acme --repo service` runs
**Then** the generated `specscore.yaml` contains a `project:` block with `title: Acme Service`, `host: github.com`, `org: acme`, `repo: service`. When the same command runs in a project root WITH a git remote `git@github.com:acme/service.git` and NO flags, the same fields are populated by inference. Explicit flags override inference; absent flags AND absent inference cause the field to be omitted from the output (not emitted as empty).

### AC: interactive-mode-prompts-and-defaults

**Requirements:** cli/init#req:interactive-mode, cli/init#req:interactive-flags-only-mode-controls

**Given** a greenfield project root and an invocation like `specscore init -i --title "PrefilledTitle"`
**When** the user runs the command from a TTY and provides stdin input
**Then** the command prompts for `title` (showing `PrefilledTitle` as default), `host`, `org`, `repo` in order; pressing Enter accepts the default; entering an empty value MUST be interpreted as "omit this field"; `--force` and `--project` are NOT prompted for (they remain flag-only).

### AC: interactive-non-tty-rejected

**Requirements:** cli/init#req:interactive-non-tty

**Given** an invocation `specscore init -i` where stdin is not a TTY (e.g., piped from `</dev/null` or invoked from a non-interactive script)
**When** the command runs
**Then** the command exits `2` with a stderr message naming the conflict (referencing `-i` and the missing TTY); no files are created in the project root.

### AC: exit-codes-discipline

**Requirements:** cli/init#req:exit-code-discipline, cli/init#req:project-root-must-exist, cli/init#req:conflict-detection

**Given** various error conditions
**When** the command runs:
- `specscore init --project /does/not/exist` exits `2` with a message naming the path (the path does not exist)
- `specscore init --project /etc/hosts` exits `2` with a message saying the path is not a directory (the path exists but is not a directory)
- `specscore init --bogus-flag` exits `2` with a flag-parsing error (invalid flag value or unknown flag)
- `specscore init` in an already-initialized repo without `--force` exits `1`
- A simulated I/O failure on writing `specscore.yaml` exits `10`
- A successful init exits `0`

**Then** each documented exit code surfaces under its documented condition; no other exit codes are ever returned by this command.

## Open Questions

- Should `init` accept a `--from <repo-url>` flag to inherit `project:` block fields from a remote repository's `specscore.yaml` (i.e., bootstrap as a sibling of an existing SpecScore project)? Useful for monorepo-style multi-project setups; not in MVP.
- Should the `viewer:` block be settable via flag (`--viewer-name`, `--viewer-url`) for projects that publish to a non-SpecStudio viewer? Today the canonical default applies; flag would be additive when needed.
- Should `--owner` be added back as a flag once Repo Config grows a `project.owner` field? When that lands upstream, this Feature revises additively to wire the flag.

---
*This document follows the https://specscore.md/feature-specification*
