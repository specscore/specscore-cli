# Exit codes

Every `specscore` command exits with a documented code. The canonical source is
[`pkg/exitcode/exitcode.go`](../pkg/exitcode/exitcode.go); this page is the
human-readable reference.

**Convention:** codes `1`–`9` are *expected, documented* errors. Codes `≥10`
are *unexpected* (treated as a crash by telemetry — see
[`docs/telemetry.md`](telemetry.md)). `0` is success.

| Code | Name | Meaning |
|------|------|---------|
| `0` | `Success` | Operation completed successfully. |
| `1` | `Conflict` | Concurrent-modification conflict. |
| `2` | `InvalidArgs` | Missing or invalid command arguments/flags. |
| `3` | `NotFound` | Requested resource does not exist. |
| `4` | `InvalidState` | State transition is not allowed. |
| `5` | `AmbiguousSlug` | Slug auto-resolution found multiple candidates. |
| `6` | `TargetNotSpecScore` | Target directory is not a SpecScore-managed repo. |
| `7` | `DirtyTree` | Working tree has uncommitted changes in paths to be modified. |
| `8` | `UnsupportedCommand` | Subcommand not recognized — typically an outdated `specscore` that predates a required subcommand. |
| `10` | `Unexpected` | Catch-all for unexpected runtime errors. |

Code `9` is currently unused and reserved.

## `UnsupportedCommand` (8) vs the shell's `127`

These two answer different questions and must not be conflated:

- **`127`** is set by the **shell**, not by `specscore`, when the binary is not
  on `PATH` at all (command not found). `specscore` never runs, so it cannot set
  this code.
- **`8`** is set by **`specscore`** when the binary *is* present and runs, but the
  requested subcommand is not recognized — the usual cause being an installed
  version that predates a subcommand the caller needs.

This lets a caller branch cleanly:

| Observed exit | Meaning | Caller action |
|---------------|---------|---------------|
| `127` | binary absent | install (`/specscore:install`) |
| `8` | present but subcommand unsupported / too old | upgrade |
| other non-zero | the command ran and genuinely failed | surface the error |

`8` is scoped to unknown **subcommands** only. Unknown *flags* keep their
existing semantics, and an error that already carries a specific exit code
(e.g. `NotFound` = 3) is never overridden.

## Shell/OS-reserved codes (for reference)

`specscore` deliberately keeps its codes inside `0`–`10` and never reuses the
codes a POSIX shell reserves:

| Code | Meaning |
|------|---------|
| `126` | Command found but not executable (e.g. permission denied). |
| `127` | Command not found (binary absent). |
| `128` | Invalid argument to `exit`. |
| `128+N` | Terminated by signal `N` (e.g. `130` = SIGINT / Ctrl-C). |
