# specscore idea new

Scaffold a lint-clean Idea artifact at `spec/ideas/<slug>.md`. Each required section is emitted with an HTML-comment prompt describing what belongs there. Content can be supplied via flags or gathered interactively with `-i`.

**CLI reference:** [`cli/idea/new`](https://github.com/specscore/specscore-cli/blob/main/spec/features/cli/idea/new/README.md)

## When to use

- **Capturing a half-formed thought** in a typed, reviewable form before deciding whether to design a feature for it
- **Pre-spec triage:** When a "should we…?" conversation needs an artifact to anchor it
- **Promotion source:** Ideas can be promoted into one or more Features (the `## Promotes To:` field is auto-filled when a Feature references the Idea via `## Source Ideas:`)
- **Lint-clean on creation:** the scaffolder emits valid `Not Doing` entries, OQ section, adherence footer, and HTML-comment prompts that don't trip placeholder rules

## Command

```bash
specscore idea new <slug> \
  [--title <text>] \
  [--owner <id>] \
  [--hmw <text>] \
  [--context <text>] \
  [--recommended-direction <text>] \
  [--mvp <text>] \
  [--not-doing "<thing> — <reason>" ...] \
  [-i | --interactive] \
  [--force] \
  [--project <path>]
```

## Parameters

| Name / Flag | Required | Description |
|---|---|---|
| `<slug>` | Yes | Idea slug — lowercase, hyphen-separated, URL-safe. Becomes `spec/ideas/<slug>.md` and the Idea's declared `id`. |
| `--title` | No | Idea title. Defaults to title-cased slug. |
| `--owner` | No | Owner / author. Defaults to `$USER`. |
| `--hmw` | No | One "How Might We…" sentence for the Problem Statement. |
| `--context` | No | Context section body. |
| `--recommended-direction` | No | Recommended Direction body. |
| `--mvp` | No | MVP Scope body. |
| `--not-doing` | No | Repeatable. Format: `"<thing> — <reason>"`. Accumulates `Not Doing` entries. |
| `-i`, `--interactive` | No | Prompt for each field on stdin, using the flag value as the default when supplied. |
| `--force` | No | Overwrite an existing `spec/ideas/<slug>.md` instead of exiting `1`. |
| `--project` | No | Project root path. Autodetected from `cwd` if omitted. |

## Exit codes

| Code | Meaning | What to do |
|---|---|---|
| `0` | Idea file created | Open `spec/ideas/<slug>.md` and replace `<!-- TODO: ... -->` prompts with content. |
| `1` | File already exists and `--force` not supplied | Pick a different slug, or pass `--force` to overwrite (destructive). |
| `2` | Missing or invalid `<slug>`, or invalid flag value | Check arguments. |
| `10+` | Unexpected I/O failure while writing | Log and escalate. |

## Examples

### Minimal scaffold

```bash
specscore idea new offline-mode
# Creates spec/ideas/offline-mode.md with TODO prompts
```

### With flag-supplied content

```bash
specscore idea new offline-mode \
  --title "Offline Mode" \
  --hmw "How might we let users keep working when their connection drops without losing data?" \
  --not-doing "Sync conflict resolution UI — out of MVP scope" \
  --not-doing "Multi-device merge — a separate idea"
```

### Interactive prompt

```bash
specscore idea new offline-mode -i
# Prompts on stdin for each field; flag values become defaults.
```

### Overwrite an existing draft

```bash
specscore idea new offline-mode --force
# Existing file replaced. No backup is made.
```

### Verify lint-clean immediately

```bash
specscore idea new my-idea
specscore spec lint --severity warning
# 0 violations found
```

## Notes

- **Lint-clean guarantee:** the generated file passes [`spec lint`](../../spec/references/lint.md) on first creation, even if every body field was left as a `<!-- TODO: ... -->` prompt. Prompts are valid Markdown and don't trip placeholder rules.
- **Slug format:** lowercase, hyphen-separated, URL-safe. `My_Idea` exits `2`.
- **Conflict semantics:** by default, a pre-existing `spec/ideas/<slug>.md` is a hard `1` (Conflict). `--force` overwrites without backup — use deliberately.
- **Owner default:** `$USER`. Tracked as an [open question](https://github.com/specscore/specscore-cli/blob/main/spec/features/cli/idea/new/README.md#outstanding-questions) whether to derive from `gh auth status` instead.
- **Methodology vs. artifact:** this skill creates the file. For the divergent/convergent process that produces a *good* Idea, use the [`specstudio:ideate`](https://github.com/specscore/specstudio-skills) skill — it delegates file creation to this command when the CLI is installed.
- **Promotion:** when a Feature later names the Idea in its `## Source Ideas:` field, tooling appends to the Idea's `## Promotes To:` list and transitions its status from `Approved` → `Specifying` (while a referenced Feature is still `Draft`/`Under Review`) or `Approved` → `Specified` (once all referenced Features are `Approved`). See the [Idea feature spec](https://github.com/specscore/specscore-cli/blob/main/spec/features/idea/README.md) for the full lifecycle.
