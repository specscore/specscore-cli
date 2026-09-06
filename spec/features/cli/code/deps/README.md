---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: Code Deps

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/code/deps?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/code/deps?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/code/deps?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/code/deps?op=request-change) |
>
> **AI skill:** [GitHub](https://github.com/specscore/ai-plugin-specscore/blob/main/skills/code/references/deps.md) · [local](../../../../../../ai-plugin-specscore/skills/code/references/deps.md) — if this command's CLI signature or behavior changes, update the linked skill to keep agents in sync.

**Status:** Stable
**Source Ideas:** —

## Summary

`specscore code deps` scans source files for typed `specscore:implements`, `specscore:verifies`, and `specscore:references` directives, legacy untyped annotations, and bare SpecScore URLs in comments. It lists the SpecScore resources (features, plans, docs) those files depend on and can validate typed links offline.

## Synopsis

```
specscore code deps [--path <glob>] [--type <feature|plan|doc>] [--check]
```

## Problem

Authors and CI need to answer "what specs does this code claim to implement?" without reading every comment. Without a dedicated query command, the only options are grep (fragile, misses URL-only references) or opening files one by one.

## Behavior

### Inputs

The command operates on the working tree under the project root. It reads files matching `--path` and extracts resource references from comments.

#### REQ: path-glob

`--path` MUST accept a double-star glob (e.g., `pkg/**/*.go`, `internal/cli/*.go`). The default value `**/*` matches every file in the tree.

#### REQ: type-filter

`--type` MAY be one of `feature`, `plan`, or `doc`. When set, results MUST be filtered to the given resource type. When omitted, results include all types. Any other value is a `2` (InvalidArgs) error.

#### REQ: offline-requirement-citation-check

`--check` validates Feature citations only from the current project or explicit local `projects:` mirrors. A `#REQ:<id>` fragment resolves to exactly one non-fenced `#### REQ: <id>` heading; authority identity must match the mirror's `project.host`, `project.org`, and `project.repo`. Missing, renamed, duplicate, malformed, unavailable, or mismatched targets fail. No network fetch occurs. A `?ref=` pin is accepted only when the local checkout HEAD is that revision, and is read from that commit. Feature citations without fragments check resource existence; Plan and doc citations remain listed but have no REQ-anchor check.

#### REQ: offline-typed-source-link-check

`--check` MUST parse typed `implements`, `verifies`, and `references` directives before validating their targets. `implements` MUST target a Feature REQ. `verifies` MUST target a Feature AC or REQ. Feature `#req:<id>` and `#ac:<id>` fragments MUST resolve to exactly one non-fenced `#### REQ: <id>` or `### AC: <id>` heading in the current project or an explicit local mirror. Fragment kind matching is case-insensitive so canonical lowercase typed links and legacy uppercase links resolve identically. Invalid relation/target pairs and missing, renamed, duplicate, malformed, unavailable, or identity-mismatched anchors fail deterministically without a network request. Plan and doc citations remain listing-only.

### Sources matched

The scanner recognizes two forms of reference in comments:

1. `specscore:` annotations as defined by the [source-references](../../../source-references/README.md) feature.
2. Bare `https://specscore.md/...` URLs.

#### REQ: both-forms

The scanner MUST detect both forms. A reference that appears in either form MUST be reported exactly once per source file.

#### REQ: prefix-body-whitespace

The scanner MUST accept optional whitespace between the annotation prefix colon and the reference body: `// specscore: feature/x` MUST resolve identically to `// specscore:feature/x`. An annotation whose reference body cannot resolve locally MUST still be listed as a dependency (resolution is by convention, not filtered against the local spec tree).

### Output

Output lists, per source file, the resources it depends on.

#### REQ: stable-output

Output MUST be stable for the same working tree across runs — files sorted, references within a file sorted. This makes the output safe to diff in CI.

## Parameters

None. All inputs are flags.

## Exit codes

Standard CLI exit codes (see [parent](../../README.md#shared-exit-code-contract)). The ones this command can return:

| Code | Condition |
|---|---|
| `0` | Scan completed (zero or more references reported) |
| `2` | `--type` or `--path` is invalid |
| `4` | `--check` found one or more invalid Feature citations; diagnostics are sorted by source file and canonical reference. |
| `10` | Unexpected I/O failure while scanning |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [source-references](../../../source-references/README.md) | Defines the annotation syntax this command parses. |
| [CodeGrapher](https://github.com/code-grapher/codegrapher) | Attaches accepted typed directives to parsed source symbols; `code deps --check` validates the same directive targets before CodeGrapher projects them. |

## Acceptance Criteria

### AC: path-filter-works

**Requirements:** cli/code/deps#req:path-glob

Supplying `--path pkg/**/*.go` restricts the scan to Go files under `pkg/`. Files outside the glob are not opened.

### AC: type-filter-works

**Requirements:** cli/code/deps#req:type-filter

Supplying `--type feature` causes the output to contain only feature references; `plan` and `doc` references are suppressed. An invalid `--type` value exits `2`.

### AC: both-annotation-and-url-detected

**Requirements:** cli/code/deps#req:both-forms

A file containing both a `specscore:` annotation and a bare `https://specscore.md/...` URL in comments reports the referenced resources without duplication.

### AC: whitespace-after-prefix-tolerated

**Requirements:** cli/code/deps#req:prefix-body-whitespace

A comment `// specscore: feature/column-validation` (a space between the colon and the body) reports `spec/features/column-validation`, identical to the no-space `// specscore:feature/column-validation` form. The dependency is listed even when the feature resolves to another repo's spec tree.

### AC: offline-requirement-citations-checked

**Requirements:** cli/code/deps#req:offline-requirement-citation-check

**Given** source code cites same-repo or configured local cross-repo Features with `#REQ:<id>`

**When** `specscore code deps --check` runs

**Then** exact headings pass; renamed/deleted, duplicate, malformed, missing-mirror, and identity-mismatched citations exit `4` with deterministic file-and-reference diagnostics, without a network request.

### AC: typed-implementation-and-test-links-checked

**Requirements:** cli/code/deps#req:offline-typed-source-link-check

**Given** implementation code carries `specscore:implements ...#req:<id>` and an executable test carries `specscore:verifies ...#ac:<id>` using canonical lowercase fragments

**When** `specscore code deps --check` scans the changed source and test scope

**Then** it lists both Feature targets, accepts each exact live REQ and AC, and rejects a missing anchor or an invalid relation/target pair with a deterministic source diagnostic, without fetching or scanning unrelated repositories.

## Open Questions

- Should `--path` accept a comma-separated list of globs for unions (e.g., `pkg/**/*.go,internal/**/*.go`) or should the current single-glob behavior stay, requiring callers to run the command twice?

---
*This document follows the https://specscore.md/feature-specification*
