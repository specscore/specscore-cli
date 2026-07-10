---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: rehearse new --dry-run — preview scaffold without writing

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/new-dry-run?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/new-dry-run?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/new-dry-run?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/new-dry-run?op=request-change) |
**Status:** Stable
**Source Ideas:** —
**Supersedes:** —

## Summary

`specscore rehearse new <ac-ref> --dry-run` prints the generated scaffold markdown to stdout without creating any files or committing to git. Lets users preview the scaffold before writing, inspect its structure, or pipe it to other tools (e.g., wc -l, less, diff against an existing file). Complements the v0.5 scaffold-writing pathway.

## Problem

v0.5's `rehearse new` writes directly to disk. Users can't preview the output before committing to a file or git history. The `--dry-run` flag enables inspection workflows: verify the scaffold is correct (Given/When/Then extracted correctly, formatting looks right), then run without `--dry-run` to write. Also enables testing: assert the scaffold output via subprocess without touching the filesystem.

## Behavior

### Command interface

#### REQ: dry-run-flag

`specscore rehearse new <ac-ref> --dry-run` accepts the `--dry-run` boolean flag (defaults to false). When set:
- The command parses the AC reference, resolves it, extracts Given/When/Then, and generates the scaffold markdown (all existing v0.5 logic)
- Instead of creating files or committing, it prints the scaffold markdown to stdout
- No file I/O, no git operations, no exit-code changes for AC resolution (exit 0 = success, exit 2 = AC not found, etc.)
- Exit 2 for errors in AC resolution (missing feature, missing AC, bad reference format) — same as non-dry-run

#### REQ: dry-run-ignores-flags

When `--dry-run` is set:
- The `--force` flag is ignored (no file exists check needed)
- The `--commit` flag is ignored (no git operations)
- No error if the target scenario file already exists (dry-run doesn't write)

#### REQ: stdout-content-exact

The output to stdout is the exact scaffold markdown string, with no additional logging, timestamps, or wrapper text. The string is suitable for piping to other commands (wc, less, diff, tee, etc.).

### Error handling

#### REQ: error-handling-same

Error paths (missing feature, missing AC, invalid reference format) produce the same exit codes and error messages as the non-dry-run path. Exit 2 for all pre-generation errors, exit 0 for success.

## Architecture & Components

- `internal/cli/rehearse.go` — modify `rehearseNewCommand()` to:
  - Add `--dry-run` flag
  - In the Run function, check if dry-run is set after generating the scaffold
  - If dry-run: print to stdout and return, skip file I/O and git operations
  - If not dry-run: existing behavior (write file, optional commit)

No new packages or seam-injected functions needed (reuse v0.5's resolver/extractor/generator + existing file-I/O seams).

## Testing Strategy

Unit tests in `internal/cli/rehearse_test.go`:
- `TestRehearseNew_DryRunPrintsScaffold` — verifies scaffold is printed to stdout via an `io.Writer` parameter threaded through `runRehearseNew()` (same pattern as `runRehearseRun` uses `cmd.OutOrStdout()`)
- `TestRehearseNew_DryRunIgnoresForce` — `--dry-run --force` prints scaffold (no write attempt, no error)
- `TestRehearseNew_DryRunIgnoresCommit` — `--dry-run --commit` prints scaffold (no git call)
- `TestRehearseNew_DryRunExitCodeSame` — `--dry-run` on missing AC still exits 2 (error to stderr)
- `TestRehearseNew_DryRunNormalWriteWorks` — without `--dry-run`, write still works

E2e: `spec/features/cli/rehearse/new-dry-run/_tests/` with two scenario stubs (dry-run-prints, dry-run-ignores-flags).

## Not Doing / Out of Scope

- Dry-run does NOT write to a temp file or check if the write would succeed
- Does NOT support `--dry-run` for other `rehearse` commands (only `new`)
- Does NOT add a `--json` or `--format` flag to control output format (scaffold is markdown only)

## Acceptance Criteria

### AC: dry-run-flag

Scenario: --dry-run prints scaffold to stdout
Given a feature `cli/studio/index` with an AC `index-two-repos`
When I run `specscore rehearse new cli/studio/index#ac:index-two-repos --dry-run`
Then the command exits 0 and prints the scaffold markdown to stdout

### AC: dry-run-ignores-flags

Scenario: --dry-run ignores --force and --commit
Given an existing scenario file at `spec/features/cli/studio/index/_tests/index-two-repos.md`
When I run `specscore rehearse new cli/studio/index#ac:index-two-repos --dry-run --force --commit`
Then the command exits 0, prints the scaffold to stdout, and does not create or modify the file

### AC: error-handling-same

Scenario: error paths unchanged
Given a feature that does not exist
When I run `specscore rehearse new missing-feature#ac:nonexistent --dry-run`
Then the command exits 2 with an error message on stderr (same as without --dry-run)

## Open Questions

None at this time.

## Autonomous Decisions

- Dry-run prints to stdout with no additional logging or formatting — the scaffold string is the sole output, making it suitable for piping
- No separate `--output` flag or file redirection — `>` and pipes are the user's responsibility, per Unix philosophy
- Error messages go to stderr (standard); stdout is reserved for the scaffold only

---
*This document follows the https://specscore.md/feature-specification*
