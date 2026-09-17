---
format: https://specscore.md/feature-specification
status: Implementing
---

# Feature: Install

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/install?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/install?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/install?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/install?op=request-change) |

**Status:** Implementing
**Source Ideas:** —

## Summary

`specscore install` lists the other fleet CLIs relevant to specscore
(`wb`, `ingitdb`, `synchestra`, `chatwright`, `codegrapher`), each with its
live installed status, and `specscore install <name>...` installs named ones
consistently with how specscore itself was installed. `specscore upgrade`
is the fleet-wide counterpart: `specscore upgrade` (no arguments) reports
every installed catalog CLI plus specscore itself — current version, latest
stable release, and verdict — without changing anything;
`specscore upgrade --all`/`specscore upgrade <name>...` upgrade what the
report showed. `specscore self-update` is `specscore upgrade specscore`:
both reach the exact same library call, because specscore is always
upgraded last and classified from its own self-update Config, never a `PATH`
probe of its own binary. The behavior is not specified here: specscore binds
the shared
[strongo/cli-helpers](https://specscore.studio/app/github.com/strongo/cli-helpers/spec/features/cli-install?op=explore)
library (`github.com/strongo/cli-helpers/cliinstall`), whose Feature owns the
catalog, status probing, destination policy, Homebrew-cask and direct-release
install methods, and every failure guarantee. This Feature specifies only
what is specscore's own — the command wiring and specscore's exit-code
mapping.

## Synopsis

```
specscore install                              # list fleet CLIs relevant to specscore, with live status
specscore install --all                        # list every catalog CLI, not just the ones relevant to specscore
specscore install wb                            # show details/relevance/plan for wb, confirm once, install it
specscore install wb ingitdb --yes              # install several, skipping the confirmation prompt
specscore install wb --dry-run                  # report the plan without installing anything
specscore install nosuchcli                     # refused before any confirmation, network request, or write
specscore install --format json                 # machine-readable listing/result

specscore upgrade                                # report every installed catalog CLI plus specscore, unchanged
specscore upgrade --all                          # upgrade every installed catalog CLI plus specscore
specscore upgrade wb ingitdb --yes                # upgrade several, skipping the confirmation prompt
specscore upgrade --all --check                   # report upgrade availability only; change nothing
specscore upgrade wb --dry-run                    # report the plan without upgrading anything
specscore upgrade nosuchcli                       # refused before any confirmation, network request, or write
specscore self-update                             # equivalent to `specscore upgrade specscore`
```

## Problem

specscore is used alongside sibling fleet CLIs — `wb` runs
`specscore spec lint` for every checkout with a `spec/` tree, `codegrapher`
links source symbols to the SpecScore artifacts they implement, `ingitdb`
stores structured project data specscore's Studio index can export as INGR
recordsets, `synchestra` turns specscore Features and Plans into task queues,
and `chatwright` verifies conversational behavior alongside `specscore
rehearse` scenarios — but specscore had no way to tell a user any of this or
help them install a sibling CLI consistently with how they installed
specscore itself.

This is the same problem `self-update` already solved for specscore's own
binary: detect the install method, resolve a release, verify it, and place it
safely. Installing a *different* CLI needs the identical machinery plus a
catalog of identities and a destination policy, which now live once in
`github.com/strongo/cli-helpers/cliinstall` rather than being rederived by
every consumer.

## Behavior

### Command surface

#### REQ: command-name

The CLI MUST expose the command as `specscore install`, taking zero or more
target-name positional arguments.

#### REQ: library-provided-behavior

The command MUST obtain its behavior from
`github.com/strongo/cli-helpers/cliinstall`'s `cobracmd.New` adapter rather
than reimplementing it. The catalog, relevance matrix, status probing,
destination policy, Homebrew-cask and direct-release install methods,
verification, confirmation gating, dry run, and batch reporting are inherited
from that library's Feature and MUST NOT be restated or reinterpreted here.
specscore MUST NOT hand-roll its own catalog entries, process execution, or
package-manager invocation code.

#### REQ: flag-surface

The command MUST expose `--all`, `--yes` (short `-y`), `--dry-run`, `--dir`,
and `--format text|json`, bound to the library's corresponding options.

#### REQ: upgrade-command

The CLI MUST expose `specscore upgrade [name...]`, built from
`github.com/strongo/cli-helpers/cliinstall/cobracmd`'s `cobracmd.NewUpgrade`
rather than reimplementing any of its behavior. The command inherits the
library's full upgrade flag surface — `--all`, `--check`, `--yes`/`-y`,
`--dry-run`, and `--format text|json` — none of which is re-specified here,
and MUST carry no `update` alias
(cli-install#req:update-alias-policy: "`upgrade` MUST NOT get an `update`
alias"; specscore's own `update` alias stays on `self-update` only).
`upgrade`'s `HostConfig` and `HostAfterUpdate` MUST be the exact SAME
`selfUpdateConfig()`-derived value and hook (currently none) `self-update`
itself builds from, so `specscore self-update` and `specscore upgrade
specscore` reach the identical library call
(cli-install#req:self-update-equals-upgrade-self,
cli-install#req:host-target-is-running-binary).

### specscore's configuration of the library

#### REQ: specscore-host-identity

specscore MUST identify itself to the library by its catalog id, `"specscore"`
(`cli-install#req:host-identity-from-catalog`). A host id absent from the
compiled catalog is a programming error the command constructor panics on,
caught by this repository's own tests, never a runtime state a user sees.

### Exit codes

#### REQ: exit-code-contract

The command MUST map the library's typed failure kinds and its own usage
error onto specscore's own exit codes, matching this repository's shared
[exit-code contract](../README.md#shared-exit-code-contract):

| Outcome | Exit code |
|---|---|
| Success (including a no-op batch, e.g. every target already installed) | `0` |
| Invalid command input (`--all` combined with names, an invalid `--format`), or a name that is not a catalog id | `2` (InvalidArgs) |
| No usable destination directory (the per-user bin directory is not on `PATH` and no fallback applies), or the destination is already occupied by a file the library will not overwrite | `4` (InvalidState) |
| Every failure kind [self-update](../self-update/README.md) already maps — ambiguous detection, release-lookup or download failure, checksum mismatch, permission denied, a refused downgrade, an unhonorable version pin, or a managed command failing | The same code `self-update` returns for that kind |
| Any other batch failure | `9` |

No message from this command carries a `self-update:` prefix, so a script
that greps for one to distinguish the two commands cannot mistake one for the
other.

#### REQ: upgrade-exit-code-contract

`specscore upgrade` MUST use the exact SAME error mapper `install` uses
(the table above), extended with one upgrades-available method
(cli-install#req:upgrade-check), called only from an explicit `--check` over
named targets or `--all` (never from the apply path: `upgrade --all` or
`upgrade <name>...` without `--check` never calls it, regardless of what the
batch finds), whenever that `--check` finds at least one target with an
update available or an undetermined verdict. That method MUST map the
same way `self-update --check` already does: exit `10`
(`selfUpdateCheckPendingCode`) with an empty message
(`cli/self-update#req:exit-code-contract`). The bare `specscore upgrade`
report (no names, no `--all`) MUST exit `0` regardless of verdict — it never
calls the upgrades-available method at all
(cli-install#req:upgrade-no-args-reports). `upgrade nosuchcli` MUST exit `2`
and name the unknown target, matching `install nosuchcli` exactly.

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [strongo/cli-helpers: CLI Install Command Library](https://specscore.studio/app/github.com/strongo/cli-helpers/spec/features/cli-install?op=explore) | Owns the behavior contract this Feature binds. specscore is a consumer; behavior changes belong there. |
| [CLI](../README.md) | Parent feature. Inherits shared CLI conventions and the shared error path. |
| [Self-Update](../self-update/README.md) | Sibling command built on the same fleet catalog (`cliinstall.ByID("specscore")`); `specscore install specscore` is reported as already installed with a `specscore self-update` pointer rather than reinstalling. `specscore self-update` and `specscore upgrade specscore` reach the identical library call (cli-install#req:self-update-equals-upgrade-self). |
| [Version](../version/README.md) | `version --json` is what every other fleet CLI's `install specscore` probes to report specscore's own installed status. |

## Acceptance Criteria

### AC: lists-relevant-fleet-clis

**Requirements:** cli/install#req:command-name, cli/install#req:library-provided-behavior

**Given** an installed `specscore` binary
**When** the user runs `specscore install` with no arguments
**Then** the command lists the fleet CLIs relevant to specscore (`wb`, `ingitdb`, `synchestra`, `chatwright`, `codegrapher`), each with its live status, without any network request or filesystem write.

### AC: unknown-target-refused

**Requirements:** cli/install#req:exit-code-contract

**Given** an installed `specscore` binary
**When** the user runs `specscore install nosuchcli`
**Then** the command fails before any confirmation, network request, or write, exits `2`, and the message names the unknown target and lists valid catalog ids, carrying no `self-update:` prefix.

### AC: destination-and-shared-failures-map-to-specscore-codes

**Requirements:** cli/install#req:exit-code-contract

**Given** a host with no usable destination directory, and separately a release lookup that fails
**When** the user runs `specscore install <name> --yes` in each case
**Then** the first exits `4` and the second exits with the same code `specscore self-update` would return for that same underlying failure kind, and neither message carries a `self-update:` prefix.

### AC: self-update-equals-upgrade-self

**Requirements:** cli/install#req:upgrade-command, cli-install#req:self-update-equals-upgrade-self

**Given** the real `self-update` and `upgrade` commands, each wired exactly as `root.go` builds them
**When** `specscore self-update --check` and `specscore upgrade specscore --check` run against the same release state
**Then** both report the same current/latest verdict and exit the same code.

### AC: upgrade-unknown-target-and-no-args-report

**Requirements:** cli/install#req:upgrade-exit-code-contract

**Given** an installed `specscore` binary
**When** the user runs `specscore upgrade nosuchcli`, and separately `specscore upgrade` with no arguments while an upgrade is available
**Then** the first fails before any confirmation, network request, or write and exits `2` naming the unknown target, and the second exits `0` regardless of what the report shows.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
