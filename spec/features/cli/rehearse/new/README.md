---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: rehearse new — scaffold scenarios from acceptance criteria

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/new?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/new?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/new?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/new?op=request-change) |
**Status:** Stable
**Source Ideas:** —
**Supersedes:** —

## Summary

`specscore rehearse new <feature-slug>#ac:<ac-slug>` scaffolds a Rehearse scenario file pre-populated with the AC's Given/When/Then text, Verifies metadata, frontmatter structure, and a placeholder bash block ready for implementation. Paves the road for `specstudio:implement` subagents to author scenarios deterministically without hand-writing boilerplate. Implements Rehearse v0.5 per the governing Idea (`rehearse` repo, `spec/ideas/rehearse-evidence-layer.md`).

## Problem

v0.4 proved that implementation agents can author passing scenarios deterministically when the pattern is clear (Verifies frontmatter, Given/When/Then in the body, bash blocks with asserts). But every scenario started from a blank slate — agents drafted both the structure and the steps, wasting tokens on boilerplate. Scaffolding the AC-derived skeleton (metadata + Given/When/Then) lets subagents focus on the implementation logic: the step blocks and asserts that do the real work. This unlocks the paved road: scenario authorship as a first-class implement-phase task.

## Behavior

### Scenario discovery and AC resolution

#### REQ: ac-reference-format

`specscore rehearse new <ac-ref>` accepts a full AC reference in the form `<feature-slug>#ac:<ac-slug>`, e.g. `cli/studio/index#ac:index-two-repos`. The command resolves this to the AC definition in `spec/features/<feature-slug>/README.md` (exact heading-based lookup: `### AC: <ac-slug>` section). Missing feature, missing AC, or unparsable reference → exit 2 with an actionable error message.

#### REQ: scenario-path-convention

The scenario file is written to `spec/features/<feature-slug>/_tests/<ac-slug>.md` (the AC slug as the filename, no parent-feature prefix). The `_tests/` directory is created if absent. An existing file at that path → exit 2 (do not overwrite); the user must delete it first.

### Scenario scaffolding

#### REQ: frontmatter-and-metadata

The scaffold opens with YAML frontmatter per the SpecScore Scenario format:
```
---
format: https://specscore.md/scenario-specification
---

# Rehearse: <ac-slug>

**Status:** pending
**Verifies:** <feature-slug>#ac:<ac-slug>
```

The filename slug becomes the title (humanized: hyphens → spaces, title-cased). The `Verifies:` line references the AC exactly (AC reference only, no `(REQ: ...)` suffix — the runner's Verifies parser and Studio adapter ingest the AC reference independent of REQ annotations; the suffix is decorative in hand-authored stubs but omitted in scaffolds to keep the skeleton minimal).

#### REQ: given-when-then-scaffold

Immediately after the metadata, the command extracts and includes the AC's Given/When/Then text verbatim:

```
Scenario source: [../README.md](../README.md) → `### AC: <ac-slug>`.

Given <precondition from AC>
When <action from AC>
Then <observable outcome from AC>
```

The extraction is line-based: after the `### AC: <ac-slug>` heading, look for `Scenario: <name>` (optional), then extract lines matching `Given`, `When`, `Then` (either plain-text or bold: `**Given**`, `**When**`, `**Then**`). Extract continues until the next `### AC:` heading or `## Acceptance Criteria` section. If the AC body deviates from this format, the scaffold includes the raw text as-is (fallback graceful degradation).

#### REQ: placeholder-step-block

A bash step block footer provides a ready-to-edit template:

```
### Step: [TODO — implement the scenario steps]

A placeholder \`\`\`bash block with a comment guides the author:

\`\`\`bash
# TODO: Implement steps to verify the scenario's Given/When/Then.
# Use $SPECSCORE to invoke the CLI under test (e.g., $SPECSCORE rehearse run ...).
# Assert exit codes and output as needed.
echo "TODO: implement scenario steps"
\`\`\`
```

The block is NOT executable; its purpose is to guide the implementer. The AC's Given/When/Then is the spec; the steps are what land here.

### CLI interface

#### REQ: rehearse-new-command

`specscore rehearse new <ac-ref>` is a top-level command under the `rehearse` group (alongside `rehearse run`). Flags:
- `--force` to overwrite an existing file at the scenario path (replaces its contents without prompting).
- `--commit` to create a git commit with the scaffolded file. Conventional commit message format: `feat(rehearse): scaffold <ac-slug> scenario` with a `Verifies: <feature-slug>#ac:<ac-slug>` trailer (linking the scaffold commit to the AC it implements, consistent with v0.4 paved-road conventions).
- No `--push` — the user stages and commits manually or uses `--commit`.

Missing `<ac-ref>` → exit 2. Invalid reference format → exit 2 with guidance on the correct form.

### Integration with implement workflow

#### REQ: implement-instructs-authorship

The `specstudio:implement` skill's subagent prompt (per the Implement Feature's `subagent-contract` section) gains new guidance:

> For tasks verifying ACs with Rehearse stubs (Status: pending), author scenarios via `specscore rehearse new <ac-slug>` (run it once; then edit the scaffolded file to implement the steps). This ensures consistent skeleton structure and lets you focus on the step blocks and asserts.

Subagents (implementing rehearse-evidence-layer tasks, for instance) will use this as the paved road: call `specscore rehearse new`, then edit the file to add step blocks, then test.

## Architecture & Components

- `internal/cli/rehearse.go` — new `rehearseNewCommand()` function returning the `new` subcommand under `rehearseCommand()`.
- `internal/rehearse/scaffold` — new package: `ScaffoldFromAC(featureSlug, acSlug)` returning a string (the file content). No side effects; testable.
- `pkg/feature` — reuse the existing AC parser to extract Given/When/Then from the Feature README at the specified path.
- File I/O: stdlib `os`, `os/filepath`, `io/ioutil` for creating `_tests/` directory and writing the file.
- Test seams: package-level vars wrapping filesystem operations (read-file, stat, mkdir-all, write-file) and git-commit (mirroring `internal/rehearse/runner/stubs.go` patterns for git exec seam).

Data flow: CLI parses `<ac-ref>` → feature lookup → AC extraction (Given/When/Then) → scaffold string generation → file write + optional commit.

## Error Handling & Failure Modes

- Feature not found at `spec/features/<feature-slug>/README.md` → exit 2, name the path.
- AC not found in the feature → exit 2, list available ACs.
- AC text not in Given/When/Then format → warning, scaffold includes raw text as-is.
- Scenario file already exists → exit 2 unless `--force` (then overwrite silently).
- Unwritable `_tests/` directory → exit 2.
- `--commit` fails (git not in repo, dirty state, hook fails) → exit 1 after the file is written (the scaffold survives, commit is optional).

## Testing Strategy

Unit tests per package with fixture Features (read-only: specs under testdata/) and expected scaffold output. E2e: CLI-level tests invoking `rehearse new` against fixture Feature repos, verifying file content, path, frontmatter, extracted Given/When/Then, optional commit. Coverage gate holds 100%.

## Not Doing / Out of Scope

- `rehearse new` does NOT edit existing scenario files (no --append, no --merge).
- Does NOT auto-run the scenario to verify it passes (the author does that).
- Does NOT interact with the Studio index or fact store.
- Standalone use case (repo without specscore.yaml) is supported — relative Feature paths are resolved from cwd.

## Rehearse Integration

This Feature itself has 5 ACs with testable (CLI) surfaces; Rehearse stubs will be scaffolded under `_tests/`.

## Acceptance Criteria

### AC: resolve-ac-reference

Scenario: valid AC reference resolves
Given a feature `cli/studio/index` with an AC `index-two-repos`
When I run `specscore rehearse new cli/studio/index#ac:index-two-repos`
Then the command exits 0 and creates a file at `spec/features/cli/studio/index/_tests/index-two-repos.md`

### AC: extract-given-when-then

Scenario: AC text is extracted into the scaffold
Given the AC whose `### AC: index-two-repos` body has `Scenario: index a two-repo workspace` followed by `Given a studio.yaml...`, `When I run specscore studio index`, `Then the command exits 0...`
When I run the command above
Then the file contains the verbatim Given/When/Then text under a `Scenario source:` heading

### AC: frontmatter-verifies-metadata

Scenario: frontmatter and Verifies line are correct
Given the command from AC: resolve-ac-reference
When I check the file's first 10 lines
Then `**Status:** pending` and `**Verifies:** cli/studio/index#ac:index-two-repos` are present

### AC: placeholder-bash-block

Scenario: bash block placeholder guides implementation
Given the command from AC: resolve-ac-reference
When I read the file's last 10 lines
Then there is a \`\`\`bash block with `# TODO: Implement steps...` and `echo "TODO: implement scenario steps"`

### AC: missing-ac-error

Scenario: invalid AC reference fails cleanly
Given a feature that exists but the AC slug `nonexistent` does not
When I run `specscore rehearse new cli/studio/index#ac:nonexistent`
Then the command exits 2 and prints an error naming the missing AC

### AC: commit-flag

Scenario: --commit stages and commits the new scaffold
Given a git repository with a feature that defines an AC
When I run `specscore rehearse new <feature>#ac:<slug> --commit`
Then the command stages the newly written scaffold (`git add`) and creates a
commit whose subject is `feat(rehearse): scaffold <slug> scenario` with a
`Verifies: <feature>#ac:<slug>` trailer, and the scaffold file is tracked with
no pending changes (a bare `git commit <path>` without staging would not
include the new file)

## Open Questions

None at this time.

## Autonomous Decisions

- Scenario filename uses bare AC slug (no feature-slug prefix) — consistent with the corpus naming convention and the `_tests/` colocation.
- Extract Given/When/Then by looking for `Scenario: <name>` heading after the AC heading, then Given/When/Then lines; if the format varies, include raw text as-is. This trades perfect parsing for robustness (ACs may evolve).
- Bash block placeholder includes a comment + dummy echo rather than assertion logic, leaving that to the author. This keeps the scaffold minimal and focused on the AC's literal text.
- `--dry-run` is deferred to v0.6; v0.5 scaffolds directly to disk. Subagents can inspect the written file before running the scenario, so the same verification happens — just not in a single `--dry-run` invocation. This keeps v0.5 scope tight.
- Standalone mode (repo without specscore.yaml) resolves Feature paths relative to cwd, not from repo root — consistent with `rehearse run`'s behavior. If cwd is a subdirectory, paths must be explicit or the command fails. No walk-to-root — that's an implementation detail of `Discover` in the Feature package, not this command's responsibility.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
