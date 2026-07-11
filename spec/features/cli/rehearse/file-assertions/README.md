---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: rehearse file assertions — verify filesystem state in scenarios

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/file-assertions?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/file-assertions?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/file-assertions?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/file-assertions?op=request-change) |
**Status:** Stable
**Source Ideas:** —
**Supersedes:** —

## Summary

`file` assert blocks in Rehearse scenario markdown verify filesystem state after scenario steps: file existence, content matching, permissions. Complements `bash` and future assertion types. Syntax: `` ### Assert: file `<path>` `<kind>` `` with a markdown code block containing the assertion details (existence, exact/partial content match, permissions). Enables scenarios to verify that CLI commands produce expected files with correct content and permissions.

## Problem

Rehearse scenarios currently verify CLI behavior via bash assertions (exit codes, stdout/stderr output). But many CLI workflows produce or modify files. Without filesystem assertions, scenarios must work around this via bash commands (stat, grep, ls) or manual file inspection. A dedicated `file` assert block makes file verification explicit and readable, following the pattern of `bash` assert blocks.

## Behavior

### File assertion structure

#### REQ: file-assert-block-syntax

Scenarios contain zero or more `file` assert blocks. Syntax:

```markdown
### Assert: file `<path>` <kind>

<assertion details in markdown code block>
```

Where:
- `<path>` is a file path (relative or absolute; relative paths are from the scenario's working directory, typically the repo root)
- `<kind>` is one of: `exists`, `missing`, `contains`, `not-contains`, or `permissions`

Examples:
```markdown
### Assert: file `.specscore/rehearse/latest.json` exists

The file should exist after the rehearse run command.

### Assert: file `spec/features/cli/studio/index/_tests/two-repos.md` contains

Given that specscore rehearse new was run, the file should contain the Given/When/Then text.

```markdown
Given a workspace with two repos
When I run specscore studio index
Then both repos are listed
```
```

### Assert: file `build/output.txt` permissions

The file should have mode 0644 (readable by all, writable by owner).

```
0644
```
```

#### REQ: assertion-kinds-defined

Each kind verifies a specific property:

- **exists**: File exists at the path. No additional details needed.
- **missing**: File does NOT exist at the path. Inverse of exists.
- **contains**: File content includes the specified text (substring match). Text is provided in a markdown code block (preserving whitespace and formatting).
- **not-contains**: File content does NOT include the specified text.
- **permissions**: File mode matches the specified octal value (e.g., 0644). Provided as a single line in code block.

#### REQ: assertion-failure-output

When an assertion fails (e.g., file doesn't exist, content doesn't match, permissions differ), the runner outputs:
- Assertion kind and path
- Expected vs. actual value (for content and permissions)
- Exit code 1 (scenario fails)

Success outputs nothing (silent pass).

### Runner integration

#### REQ: runner-parses-file-blocks

`specscore rehearse run` parses markdown for `### Assert: file` headings, extracts the kind and path, and evaluates each assertion after running the bash steps. Parsing is case-insensitive for kind names but case-sensitive for paths.

#### REQ: file-block-order

File assertions are evaluated in the order they appear in the scenario. If any assertion fails, the scenario stops and exits 1. Earlier assertions' successes do not affect later ones.

#### REQ: file-block-working-directory

File paths are resolved relative to the scenario's temporary working directory (the temp dir where bash steps execute). For scenarios in `spec/features/cli/studio/index/_tests/`, if the scenario's bash steps change to a subdirectory within the workdir (e.g., `cd spec/features/cli/studio/index`), relative paths in file assertions are still resolved from the original workdir root, not from that subdirectory. Absolute paths (starting with `/`) are also supported (for testing against system files, though rare). Paths are resolved at assertion evaluation time, which is after all bash steps have executed.

## Architecture & Components

- `internal/rehearse/scenario` — extend `ParseBytes` or add a second parse pass to recognize `### Assert: file` H3 headings (note: these are headings, not fenced code blocks, so they require a distinct parse mechanism from the existing `blocks.Registry` fenced-block dispatcher)
- `internal/rehearse/blocks/fileblock` — new package with `File` struct implementing file assertion evaluation (stat, read, compare content/permissions)
- `internal/rehearse/runner` — extend `runScenario()` to evaluate file assertions after bash steps, using the parsed file assertions from the scenario

Data flow: scenario markdown → scenario parser extracts `### Assert: file` H3 headings + fenced-code details → runner evaluates each assertion (stat, read, compare) → pass/fail per assertion → exit code reflects overall scenario status.

## Testing Strategy

Unit tests for parsing and assertion evaluation (with `// Verifies:` trailers per project convention):
- `internal/rehearse/scenario` tests: parse `### Assert: file` H3 headings and code blocks, extract path/kind/details
- `internal/rehearse/blocks/fileblock` tests: evaluate each kind (exists, missing, contains, not-contains, permissions) against fixture files
- E2e: scenarios in `spec/features/cli/rehearse/file-assertions/_tests/` with fixture files and expected pass/fail outcomes

All tests maintain 100% statement coverage before commit.

## Not Doing / Out of Scope

- No **directory** assertions (only files)
- No **glob patterns** or wildcards in paths (exact paths only)
- No **ownership** assertions (uid/gid) — only permissions
- No **symlink** target assertions
- No **modification time** assertions
- No regex patterns for content matching (substring only)

## Acceptance Criteria

### AC: exists-kind

Scenario: file exists assertion passes
Given a scenario that creates a file at `.specscore/test.txt`
When the runner evaluates `### Assert: file `.specscore/test.txt` exists`
Then the assertion passes and the scenario succeeds

### AC: missing-kind

Scenario: file missing assertion passes
Given a scenario where a file should not exist
When the runner evaluates `### Assert: file `nonexistent.txt` missing`
Then the assertion passes and the scenario succeeds

### AC: contains-kind

Scenario: file contains assertion passes
Given a scenario that writes "hello world" to a file
When the runner evaluates `### Assert: file `output.txt` contains` with "hello world" in the code block
Then the assertion passes

### AC: contains-fails-mismatch

Scenario: file contains assertion fails on mismatch
Given a file with content "goodbye"
When the runner evaluates `### Assert: file `output.txt` contains` with "hello" in the code block
Then the assertion fails, the scenario exits 1, and the output shows expected vs. actual

### AC: not-contains-kind

Scenario: file not-contains assertion passes
Given a file with content "goodbye"
When the runner evaluates `### Assert: file `output.txt` not-contains` with "hello" in the code block
Then the assertion passes and the scenario succeeds

### AC: permissions-kind

Scenario: file permissions assertion passes
Given a file with permissions 0644
When the runner evaluates `### Assert: file `spec.md` permissions` with "0644" in the code block
Then the assertion passes

### AC: runner-parses-file-blocks

Scenario: runner recognizes file assert blocks
Given a scenario with multiple `### Assert: file` blocks
When the runner parses the scenario
Then all file blocks are recognized and queued for evaluation

## Open Questions

- Should file assertions support environment variable expansion (e.g., `$HOME/.config/app.conf`)? Deferred to v0.7.
- Should there be a `size` kind for asserting file size? Deferred to v0.7.

## Autonomous Decisions

- File paths are resolved relative to the scenario's working directory (typically repo root), not the scenario file's directory. This keeps assertions portable across scenarios.
- Substring matching for `contains` / `not-contains` is sufficient; regex patterns are deferred to v0.7.
- Permissions are asserted as octal mode only (0644, 0755, etc.), not symbolic notation. Simpler parsing and more portable.
- Silent pass (no output) for successful file assertions, matching the `bash` assert behavior.

---
*This document follows the https://specscore.md/feature-specification*
