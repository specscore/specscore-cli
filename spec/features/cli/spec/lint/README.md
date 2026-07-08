---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Spec Lint

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint?op=request-change) |
>
> **AI skill:** [GitHub](https://github.com/specscore/ai-plugin-specscore/blob/main/skills/spec/references/lint.md) · [local](../../../../../../ai-plugin-specscore/skills/spec/references/lint.md) — if this command's CLI signature or behavior changes, update the linked skill to keep agents in sync.

**Status:** Approved
**Source Ideas:** index-entries-autofix, lint-fix-reports-modified-files, modelspec-cli-validation

## Summary

`specscore spec lint` scans the specification tree and reports violations of structural conventions. Violations are categorized by severity (error, warning, info). `--fix` applies autofixes for rules that support them (adherence footers, view links, idea sync / index / archived-order rules, phantom rows in feature indices, missing rows for orphan child directories). When `--fix` runs, the command reports the exact set of files it modified — as a human summary on stderr (text format) and as a `fixed` array in `--format json|yaml` — so no autofix change is ever silently lost from a commit. `--fix=<targets>` scopes the pass to named fixes (e.g. `--fix=no-source`); some fixes are opt-in and run only when named. When a reported violation is fixable, a `How to fix:` section names the exact `--fix=<target>` command.

## Contents

| Directory | Description |
|---|---|
| [plan-rules/](plan-rules/README.md) | Lint rules `P-001`–`P-005` and parser extensions for single-file Plans (`**Mode:**`, `**Status:**`, `**Depends-On:**`, `**Parent:**`, placeholder body token) |
| [issue-rules](issue-rules/README.md) | Adds 15 lint rules (`I-001`–`I-015`) and the underlying `issue` artifact parser to `specscore spec lint`, implementing the contract reserved by the SpecStudio `issue-artifact-type` Feature in the [`specstudio-skills`](https://github.com/specscore/specstudio-skills) repo. |
| [feature-rules](feature-rules/README.md) | Adds feature-level lint rules to specscore spec lint — starting with feature-source-ideas-required, which enforces that every Feature carries a **Source Ideas:** line (with an explicit-empty sentinel) and backfills it via --fix. |
| [legacy-status-autofix](legacy-status-autofix/README.md) | Idempotent --fix rewrites of a closed, documented set of legacy artifact status tokens (plan, decision, idea) to their canonical replacements; unknown/free-form values stay non-autofixable errors. |

## Synopsis

```
specscore spec lint [--fix[=<targets>]] [--severity <error|warning|info>] [--rules <list>] [--ignore <list>] [--format <text|json|yaml>] [--project <path>]
```

## Problem

SpecScore's structural rules are useless if they are not enforced. Humans miss adherence footers, ship OQ sections that say nothing, mis-nest headings, or let indexes drift from reality. A deterministic linter catches every violation mechanically so review effort goes to content, not conventions.

## Behavior

### Rule suite

The default run enables every known rule. The canonical rule list lives in the [lint package](../../../../../pkg/lint) and is not duplicated here.

#### REQ: default-runs-all-rules

Without `--rules` or `--ignore`, `spec lint` MUST execute every registered rule against the project.

#### REQ: rules-whitelist

`--rules a,b,c` MUST restrict execution to the named rules. Any rule name not in the registered set MUST exit `2` (InvalidArgs) with a message naming the unknown rule. The command MUST fail fast — no lint run is performed when any name is invalid.

#### REQ: ignore-blacklist

`--ignore x,y` MUST exclude the named rules from execution. Unknown rule names in `--ignore` follow the same hard-error policy as `--rules`.

#### REQ: mutually-exclusive-filters

`--rules` and `--ignore` MUST NOT be combined. Supplying both MUST exit `2`.

### Features index synchronization

Every directory under `spec/features/` that contains a `README.md` is treated as a feature index for its immediate sub-features. The `index-entries` rule keeps the index in sync with the filesystem in both directions.

#### REQ: index-entries-bidirectional

`index-entries` MUST report a violation when:

- the index contains a Markdown link to a child README (link target ending in `<dirname>/README.md`) but that directory does not exist on disk, OR
- a child directory exists on disk (with its own `README.md`) but is not linked from the parent index.

Both directions apply at every level of the feature tree, including the root `spec/features/README.md`. Hidden directories (starting with `.`) and underscore-prefixed convention directories (e.g. `_args/`) are excluded.

#### REQ: index-entries-fix-deletes-phantom-rows

When `index-entries` reports `Index mentions non-existent directory: <name>` and `spec lint` runs with `--fix`, the fixer MUST remove from the parent README's index table the single row whose link target ends in `<name>/README.md`. Surrounding rows, table delimiters, and the rest of the document MUST be preserved.

#### REQ: index-entries-fix-inserts-orphan-rows

When `index-entries` reports `Child directory not listed in index: <name>` and `spec lint` runs with `--fix`, the fixer MUST append a row that links the missing child. The row shape MUST match what `specscore feature new` already writes via `UpdateFeatureIndex` / `UpdateParentContents`:

- At the root features index (`spec/features/README.md`), a 4-cell row of the form `| \[<name>\]\(<name>/README.md\) | <status> | — | TODO: Add description. |`. `<status>` is parsed from the child's `**Status:**` header via `feature.ParseFeatureStatus`; `Kind` and `Description` use the same hand-maintained placeholder convention `feature new` codifies.
- At a nested feature index, a 2-cell row in the `## Contents` table of the form `| \[<name>\]\(<name>/README.md\) | TODO: Add description. |`. The `## Contents` block is created if absent.

The fixer MUST NOT mutate any cell beyond the inserted row; existing rows are preserved byte-for-byte. The deletion direction (phantom rows) runs first so the insertion phase reads a phantom-free index.

This REQ does NOT violate `fix-is-safe-subset`. Status flows from a structurally-parsed field; Kind and Description use placeholders the project has already codified for `feature new`, so the autofix is byte-identical to user-driven scaffolding. The placeholders are visibly under-filled (`—`, `TODO: ...`), inviting the author to populate them rather than masking missing intent.

### Open Questions section

Every `README.md` under the `spec/` tree MUST contain a `## Open Questions` section — including the root `spec/README.md` and READMEs under `spec/research/`, `spec/decisions/`, and other sibling subtrees, not only `spec/features/` and `spec/plans/`. This mirrors the convention declared in `AGENTS.md`: *"Every README.md MUST have an Open Questions section."* The `oq-section` rule validates the section's presence; a separate `oq-not-empty` rule (warning severity) flags an existing section that has no body content. The canonical heading text is `## Open Questions`; a legacy `## Outstanding Questions` heading is rejected with a distinct, actionable message and is autofixable. The `oq` rule-name abbreviation is preserved across the rename.

#### REQ: oq-section-required

`oq-section` MUST report a violation when any `README.md` anywhere under `spec/` (recursive) lacks a level-2 `## Open Questions` heading and also lacks a legacy `## Outstanding Questions` heading (which is reported by `REQ:oq-section-legacy-heading` instead). Severity: `error`. Message: `Open Questions section not found`.

#### REQ: oq-section-legacy-heading

`oq-section` MUST treat a level-2 `## Outstanding Questions` heading as a violation distinct from "not found": severity `error`, rule name `oq-section`, message `Legacy heading "## Outstanding Questions" found; rename to "## Open Questions" (run with --fix to migrate)`. A file that contains the legacy heading MUST NOT also produce an `oq-section-required` violation — the two REQs are mutually exclusive per file.

#### REQ: oq-section-fix-rewrites-legacy-heading

When `--fix` runs and `REQ:oq-section-legacy-heading` reports a violation, the fixer MUST replace the heading line matching `^##\s+Outstanding\s+Questions\s*$` with `## Open Questions`. The rewrite MUST be line-scoped: the rest of the file MUST be preserved byte-for-byte. Other occurrences of the phrase "Outstanding Questions" (prose, code blocks, anchor identifiers, link text) MUST NOT be modified by this autofix. The fixer MUST walk every `.md` file under `spec/` (recursive), not only the `README.md` files the check phase reports on — so legacy headings in single-file Idea artifacts (`spec/ideas/<slug>.md`) and other ad-hoc `.md` files under `spec/` migrate in the same pass.

#### REQ: oq-not-empty-rule

A separate `oq-not-empty` rule MUST report a warning when the `## Open Questions` section exists but contains no body content (only blank lines before the next heading or end-of-file). Severity: `warning`. Message: `Open Questions section appears empty`.

### Dogfood version pin

CI workflows that dogfood `specscore spec lint` typically install a specific released CLI version (`SPECSCORE_VERSION: vX.Y.Z` in `.github/workflows/dogfood.yml` or similar). When a convention change ships in a new CLI release, the pinned version must be bumped or CI will run with an old CLI that doesn't understand the new convention — silently passing locally while failing in CI, or vice versa. The `dogfood-version-bump` rule catches drift between the pinned CLI version and the CLI version actually running the lint.

#### REQ: dogfood-version-bump-detects-stale-pin

`dogfood-version-bump` (severity: `warning`) MUST scan every YAML file under `.github/workflows/` (extensions `.yml` and `.yaml`) for lines whose pattern matches `SPECSCORE_VERSION:\s*"?v?(\d+\.\d+\.\d+)"?` (optional quotes, optional `v` prefix; trailing comments allowed). For each match, the rule compares the parsed semver against the CLI's own version (as reported by `--version`). When the pinned version is strictly less than the CLI version, the rule MUST emit one violation per match. Message: `Pinned SPECSCORE_VERSION v<pinned> is older than the running CLI version v<cli>; bump the pin to match`. The rule MUST emit no violation when pinned == CLI version, when pinned > CLI version (the user's deliberate forward-pin), or when no `SPECSCORE_VERSION` is found in any workflow file.

#### REQ: dogfood-version-bump-skips-when-binary-version-unparseable

When the CLI's own version cannot be parsed as semver (e.g., `dev` for local builds, or anything else not matching `v?\d+\.\d+\.\d+`), the rule MUST emit NO violations regardless of what is pinned. Dev builds are explicit overrides and the rule has nothing meaningful to say in that mode.

#### REQ: dogfood-version-bump-skips-when-pin-unparseable

When a `SPECSCORE_VERSION:` line is present but the value does not parse as semver (e.g., `latest`, `main`, `${{ inputs.version }}`), the rule MUST skip that match silently. Non-semver pins are intentional overrides (rolling, dispatch-driven, etc.) and out of scope.

#### REQ: dogfood-version-bump-no-autofix

`dogfood-version-bump` MUST NOT support `--fix`. Bumping a pinned CLI version is a deliberate human decision (per the convention `# bump intentionally via PR` comment seen in dogfood workflows) and the rule's role is purely to surface drift — never to silently rewrite the pin.

### Idea change-request and lifecycle rules

Rules added to support the change-request idea type, the extended lifecycle statuses (`Specifying`, `Implemented`), and related structural constraints.

| Rule | Severity | Description |
|---|---|---|
| `idea-type-values` | error | `**Type:**` field when present must be `feature-request` or `change-request`. |
| `idea-type-title-consistency` | error | `# Proposal:` prefix requires `change-request` type; `# Idea:` prefix rejects `change-request`. |
| `idea-targets-required` | error | `**Targets:**` required for change-request, prohibited for feature-request. |
| `idea-targets-exists` | error | `**Targets:**` value must reference an existing Feature directory (one containing `README.md`). |
| `idea-change-request-location` | error | Change-request idea must live at `spec/features/{targets}/proposals/{slug}.md`. |
| `idea-phase-non-empty` | error | `**Phase:**` field when present must be non-empty. |

#### Modified rules

| Rule | Change |
|---|---|
| `idea-status-values` | Now accepts 8 statuses: Draft, Under Review, Approved, Specifying, Specified, Implementing, Implemented, Archived. |
| `idea-sync-lint-strict` | New derivation levels: `Specifying` (any referenced Feature at Draft/Under Review), `Implemented` (all referenced Features at Stable **or Deprecated** — `Deprecated` is a post-Stable "done" state and derives like `Stable`, never dragging the Idea backward). Skips change-request ideas (author-managed). |
| `idea-specified-requires-promotion` | Now checks `Specifying`, `Specified`, `Implementing`, and `Implemented` for non-empty `**Promotes To:**`. Skips change-request ideas. |
| `idea-feature-cross-reference` | Accepts `Specifying` and `Implemented` as valid statuses for cross-referenced ideas. |
| `idea-archived-location` | Relaxed for change-request ideas — they remain at their feature-scoped `proposals/` path when archived. |
| `idea-related-ideas-target-exists` | Resolves slugs under `spec/features/*/proposals/` in addition to `spec/ideas/`. |
| `idea-index-completeness` | Scans `spec/features/*/proposals/` directories for change-request ideas. |

### Severity filtering

Each rule has a built-in severity (`error`, `warning`, `info`). `--severity` sets the minimum severity reported.

#### REQ: severity-default

Default `--severity` MUST be `error`. Warnings and info-level findings are suppressed unless the caller explicitly widens the filter.

#### REQ: severity-values

`--severity` MUST accept `error`, `warning`, or `info`. Any other value MUST exit `2`. The ordering is strict: `info ⊂ warning ⊂ error` (wider filter includes narrower).

### Autofix

Rules that support autofix declare so in their registration. `--fix` applies only those fixes; rules without autofix still report violations unchanged.

#### REQ: fix-accepts-optional-targets

`--fix` MUST accept an optional comma-separated value: `--fix=<target1,target2,…>`. A bare `--fix` (no `=value`) MUST run the standard fix pass — every fixer that is on by default — and is equivalent to `--fix=all`. The value `all` is always valid. Any target that is neither `all` nor a known fix-target name MUST exit `2` with a message naming the unknown target.

#### REQ: fix-targets-scope-the-pass

When `--fix=<targets>` names one or more targets other than `all`, the fix pass MUST be scoped to exactly those targets: only fixers whose action name is named run; every other fixer (including default-on ones) MUST be skipped. A target's action name defaults to the rule name(s) it is registered under; a fixer MAY declare a distinct action name (e.g. the plan no-source fix answers to `no-source`, not its `P-00x` rule IDs).

#### REQ: fix-opt-in-targets

A fix MAY be declared *opt-in*: never run by the unscoped pass (bare `--fix` / `--fix=all`), only when its target is explicitly named. `no-source` is opt-in — it rewrites a plan that declares no source line at all into `**Source:** none`, and is applied only under `--fix=no-source` (or a list that includes it).

#### REQ: fix-how-to-fix-hint

In text format, after the violation report, `spec lint` MUST print a `How to fix:` section listing — once per distinct fix target among the remaining violations — the count of fixable findings and the exact `specscore spec lint --fix=<target>` command that repairs them. A violation carries a fix target only when a fixer can actually resolve it. The section MUST be written to stderr (leaving stdout as the violation report) and MUST be omitted when no remaining violation is fixable. In `--format json|yaml`, each violation instead carries an optional `fix_target` field.

#### REQ: fix-is-safe-subset

`--fix` MUST only mutate what the rule declares safe to mutate. Mutations that require semantic interpretation of document intent beyond structural conventions MUST NOT be autofixed. Structural rewrites of a recognized trailing adherence-footer block are safe and allowed.

#### REQ: adherence-footer-fix-rewrites-trailing-footer

When the `adherence-footer` rule runs with `--fix`:

- if the required URL is missing and no adherence-footer block exists at end-of-file, the canonical footer block MUST be appended;
- if an adherence-footer block exists at end-of-file but carries the wrong `https://specscore.md/*-specification` URL, the fixer MUST rewrite that existing block to the canonical URL for the document type;
- the fixer MUST leave exactly one adherence-footer block at end-of-file.

#### REQ: fix-is-idempotent

Running `spec lint --fix` twice in a row MUST yield no changes on the second run. The second run exits `0` with no output beyond the standard violation report.

### Fixed-files reporting

`--fix` mutates files in place. Before this behavior the command was silent about which files it changed: a clean `--fix` produced empty stdout and exit `0`, so a caller (human, CI, or agent) had no authoritative signal and could only guess from a `git diff` — a guess that has dropped legitimate autofix changes from commits. `spec lint` now reports exactly what `--fix` modified, on every `--fix` run, by default.

#### REQ: fix-reports-modified-files

When run with `--fix`, `spec lint` MUST determine the exact set of files the fix pass modified and make that set available to the caller. The set MUST be derived from actual content change (a file whose bytes are identical before and after the fix pass MUST NOT appear), MUST list each modified file at most once, and MUST report paths relative to the project root in a deterministic (sorted) order.

#### REQ: fix-report-json-envelope

With `--fix` and `--format json` (or `--format yaml`), stdout MUST be a single object carrying two keys: `fixed` (the array of modified project-relative paths, possibly empty) and `violations` (the remaining-violations array). The structured fixed/violations result MUST NOT be split across stdout and stderr. Without `--fix`, structured output MUST remain the bare violations array it is today — the envelope shape applies only when `--fix` is set, preserving the existing non-fix contract.

#### REQ: fix-report-text-summary

With `--fix` in the default text format, when the fix pass modifies one or more files the command MUST print a human-readable summary to **stderr** naming each modified path. stdout MUST continue to carry only the remaining-violations report (unchanged from a non-`--fix` run). When the fix pass modifies zero files, no summary is printed.

#### REQ: fix-report-default-on

The fixed-files report MUST be emitted whenever `--fix` is used, with no opt-in flag required to enable it. Silence-by-default is the failure mode this Feature exists to remove, so the report is never gated behind a flag.

### Exit codes

`spec lint` signals violations through the exit code so CI can gate on it.

#### REQ: exit-1-on-violations

If any violation at or above the effective `--severity` is found, the command MUST exit `1`. Exit `0` is only returned when zero such violations exist.

### Output formats

Default output is text. `--format json` and `--format yaml` produce structured output suitable for tool consumption.

### Repo-config gate

`spec lint` operates on a SpecScore-managed project, which is anchored by a [`specscore.yaml`](../../../repo-config/README.md) file at the project root. Unlike `feature` / `idea` / `task` commands, lint does NOT fall back to a bare `spec/features/` directory — a config file is the single source of truth for project identity, viewer settings, and module layout, all of which the linter relies on.

#### REQ: specscore-yaml-required

`spec lint` MUST refuse to run when no `specscore.yaml` is found in the working directory or any of its ancestors. The command MUST exit `3` (NotFound) with a message that (1) names `specscore.yaml` as mandatory and (2) tells the caller to run `specscore init` to create it. The message MUST NOT mention the legacy `spec/features/` fallback used by other commands.

### Doc-Kind rule families

The lint package is the canonical registry for every rule that validates a SpecScore Doc-Kind instance. Each Doc-Kind owns a family of rules whose names share a kind-specific prefix (e.g., `idea-*`, `entity-*`, `property-*`). Authoritative contracts for each family live in the corresponding Doc-Kind's CLI Feature; the `spec lint` Feature carries the registration and discovery rules that every family must satisfy.

#### REQ: entity-and-property-rules-registered

The `entity-*` and `property-*` rule families MUST be present in `pkg/lint`'s canonical `allRuleNames` map, selectable via `--rules <name>`, and exercised by Go-level unit tests. The full list of `entity-*` rule names is enumerated in [cli/entity#lint-enforcement](../../entity/README.md#lint-enforcement); the full list of `property-*` rule names is enumerated in [cli/property#lint-enforcement](../../property/README.md#lint-enforcement). Adding a new rule to either family requires updating both the lint package's `allRuleNames` map AND the corresponding CLI Feature's rule table — drift between the two is a manual review concern in MVP (see the Outstanding Question on `lint-rule-table-completeness`).

#### REQ: managed-section-rewrite-on-fix

When `spec lint --fix` runs over a tree containing `*.entity.md` or `*.property.md` files, the fixer MUST rewrite every managed body wrapped in the canonical `<!-- managed-by: specscore lint --fix -->` / `<!-- end-managed -->` markers. The rewriter MUST be deterministic and idempotent — re-running `--fix` on a clean tree MUST produce zero further changes. The specific managed sections covered are enumerated in:

- [cli/entity#req:properties-table-rendered](../../entity/README.md#req-properties-table-rendered) — the `## Properties` table on entity files.
- [cli/entity#req:referenced-by-from-inheritance](../../entity/README.md#req-referenced-by-from-inheritance) — the `## Referenced by` section on entity files.
- [cli/property#req:referenced-by-from-entities](../../property/README.md#req-referenced-by-from-entities) — the `## Referenced by` section on property files.

This REQ extends [REQ: fix-is-idempotent](#req-fix-is-idempotent) — the second-pass-clean contract applies to managed-section rewrites in addition to adherence-footer and idea-sync rewrites.

### Document-types-registry consumer-path parsing

The upstream [document-types-registry](https://github.com/specscore/specscore/blob/main/spec/features/document-types-registry/README.md) Feature relaxes the `Consumer Path` column in `spec/features/README.md` (in the meta-spec repo) to accept either a single glob or a **comma-separated list of globs** for Document-Kind rows. The CLI MUST honour this form wherever it parses a `Consumer Path` cell.

#### REQ: consumer-path-multi-glob

When `pkg/lint` or any other consumer parses a `Consumer Path` cell from a registry row, the parser MUST treat the cell value as a comma-separated list of globs. Whitespace surrounding each comma MUST be tolerated. The result MUST be the union of files matched by every glob in the list. An empty cell value (or the dash placeholder `—`) MUST yield an empty glob list — never an error. Empty entries between commas (leading commas, trailing commas, or doubled internal commas like `a,,b`) MUST be discarded silently regardless of position — the parser is permissive, not strict, because the goal is to forward-resolve the row, not to police whitespace.

This REQ defines the parsing contract. The CLI rules that consume the parsed glob list today are the entity-location and property-location rules ([cli/entity#req:discovery-scope](../../entity/README.md#req-discovery-scope), [cli/property#req:discovery-scope](../../property/README.md#req-discovery-scope)); the parsing surface is forward-compatible — a future `every-feature-registered` rule that cross-checks every registry row would consume the same parser.

### GraphSpec rule integration

GraphSpec validation is specified first under [`specscore graph lint`](../../graph/validation/README.md). Repo-wide `specscore spec lint` may later include GraphSpec rules, but that integration is intentionally gated because GraphSpec is still a bootstrap language.

#### REQ: graph-rules-not-default-until-gated

`specscore spec lint` MUST NOT enable GraphSpec-specific `graph-*` rules by default until GraphSpec declares a stable enough validation contract or repo configuration opts in. Experimental `spec/graph/` trees should be valid SpecScore repository content before GraphSpec validation is globally enforced.

#### REQ: graph-rule-registry-compatibility

When GraphSpec rules are registered in `pkg/lint`, they MUST use the same rule-selection machinery as existing rule families: `--rules`, `--ignore`, `--severity`, structured output, and exit-code behavior. Their authoritative CLI contract lives under [Graph Validation](../../graph/validation/README.md), not in this generic lint feature.

### ModelSpec rule integration

ModelSpec is an independent specification language for application data models. The
CLI may validate ModelSpec documents, but ModelSpec semantics remain owned by the
ModelSpec specification, not the CLI, SpecScore, or GraphSpec.

#### REQ: modelspec-rules-not-graphspec

`specscore spec lint` MUST NOT model ModelSpec validation as GraphSpec validation.
ModelSpec rules, if registered, MUST use their own rule family and diagnostics.

#### REQ: modelspec-rule-registry-compatibility

When ModelSpec rules are registered in `pkg/lint`, they MUST use the same
rule-selection machinery as existing rule families: `--rules`, `--ignore`,
`--severity`, structured output, and exit-code behavior. Rule behavior that depends on
ModelSpec concepts MUST cite ModelSpec as the semantic source of truth.

#### REQ: modelspec-validation-gated

ModelSpec-specific rules SHOULD run only when ModelSpec files are present or when the
caller explicitly targets ModelSpec validation. Existing `specscore spec lint`
workflows MUST NOT require repositories to create a non-empty ModelSpec, and MUST NOT
break merely because a repository has no ModelSpec files.

## Parameters

None. All inputs are flags.

## Exit codes

| Code | Condition |
|---|---|
| `0` | No violations at or above `--severity` |
| `1` | One or more violations at or above `--severity` |
| `2` | Invalid `--rules` / `--ignore` name, invalid `--severity`/`--format` value, conflicting filters |
| `3` | `specscore.yaml` not found in any ancestor directory |
| `10` | Unexpected I/O failure while scanning |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [adherence-footer](../../../adherence-footer/README.md) | Defines the footer contract; `spec lint` enforces it. Autofix inserts missing footers and rewrites incorrect trailing footer URLs. |
| [idea](../../../idea/README.md) | Declares idea-specific rules; `spec lint` runs them alongside shared rules. |
| [feature](../../../feature/README.md) | Declares the required-sections rules for feature READMEs. |
| [CLI](../../README.md) | Inherits exit-code contract and project autodetection. `spec lint`'s exit-1-on-violations convention is part of the shared contract. |

## Acceptance Criteria

### AC: clean-tree-exits-0

**Requirements:** cli/spec/lint#req:exit-1-on-violations

On a specification tree with zero error-severity violations, `specscore spec lint` exits `0` with no violation lines printed.

### AC: violations-exit-1

**Requirements:** cli/spec/lint#req:exit-1-on-violations

A tree containing at least one error-severity violation exits `1`. The violation list is printed to stdout (text format) or is the payload of the structured output.

### AC: unknown-rule-name-exits-2

**Requirements:** cli/spec/lint#req:rules-whitelist

`specscore spec lint --rules not-a-rule` exits `2` with a message naming the unknown rule. No lint run is performed.

### AC: fix-idempotent

**Requirements:** cli/spec/lint#req:fix-is-idempotent

Running `spec lint --fix` twice consecutively on the same tree yields no file changes on the second run and exits `0`.

### AC: adherence-footer-fix-replaces-trailing-wrong-url

**Requirements:** cli/spec/lint#req:adherence-footer-fix-rewrites-trailing-footer

If a document ends with an adherence-footer block that uses the wrong `specscore.md/*-specification` URL, running `spec lint --fix` rewrites that block to the canonical URL and does not append a second footer block.

### AC: index-entries-fix-removes-phantom-row

**Requirements:** cli/spec/lint#req:index-entries-fix-deletes-phantom-rows

Given a parent README whose index table contains a Markdown link whose target is `ghost/README.md` while no `ghost/` directory exists on disk, running `specscore spec lint --fix` removes that single row from the index table and leaves every other row intact. A second consecutive `spec lint --fix` produces no further changes (per `fix-is-idempotent`).

### AC: index-entries-fix-inserts-orphan-row

**Requirements:** cli/spec/lint#req:index-entries-fix-inserts-orphan-rows

Given a root features index that lists `auth` while a `billing/` directory with `**Status:** Stable` also exists on disk but is unlinked, running `specscore spec lint --fix` appends the row `| \[billing\]\(billing/README.md\) | Stable | — | TODO: Add description. |` to the index table, preserves the existing `auth` row byte-for-byte, and emits no further changes on a second consecutive pass. The nested case behaves the same way with a 2-cell `| \[<name>\]\(<name>/README.md\) | TODO: Add description. |` row in the `## Contents` table of the parent feature.

### AC: index-entries-flags-orphan-child

**Requirements:** cli/spec/lint#req:index-entries-bidirectional

Given a feature tree where `spec/features/orphan/README.md` exists on disk but `spec/features/README.md` does not link to `orphan/`, running `specscore spec lint` exits `1` with an `index-entries` violation on `features/README.md` whose message names the unlisted child directory.

### AC: oq-section-missing-flagged

**Requirements:** cli/spec/lint#req:oq-section-required

A README under `spec/` that contains no `## Open Questions` heading and no `## Outstanding Questions` heading exits `1` with one `oq-section` violation whose message reads `Open Questions section not found`. This applies equally to `spec/README.md` itself, `spec/research/README.md`, `spec/decisions/README.md`, and to every nested feature/plan/idea README — every `README.md` under the `spec/` tree is in scope.

### AC: oq-section-legacy-heading-flagged-and-fixed

**Requirements:** cli/spec/lint#req:oq-section-legacy-heading, cli/spec/lint#req:oq-section-fix-rewrites-legacy-heading

A README whose OQ-style heading is `## Outstanding Questions` exits `1` with one `oq-section` violation pointing at the legacy heading and the actionable rename message. Running `specscore spec lint --fix` rewrites the single heading line in place to `## Open Questions`, preserves every other line byte-for-byte (including any prose mentions of "Outstanding Questions"), and a second consecutive `spec lint --fix` yields no further changes (per `fix-is-idempotent`).

### AC: dogfood-version-bump-flags-stale-pin

**Requirements:** cli/spec/lint#req:dogfood-version-bump-detects-stale-pin, cli/spec/lint#req:dogfood-version-bump-skips-when-binary-version-unparseable, cli/spec/lint#req:dogfood-version-bump-skips-when-pin-unparseable

Given a `.github/workflows/dogfood.yml` that pins `SPECSCORE_VERSION: v0.2.0` while the running `specscore` binary reports version `0.3.0`, `specscore spec lint --severity warning` exits `1` with one `dogfood-version-bump` warning naming both versions. When the binary's version is `dev` (or any non-semver string), or when the pinned value is `latest` / `main` / a `${{ inputs.* }}` expression, the rule emits no violation regardless of what else the workflow contains. A workflow with `SPECSCORE_VERSION: v0.3.0` against a `0.3.0` binary is silent.

### AC: missing-specscore-yaml-exits-3

**Requirements:** cli/spec/lint#req:specscore-yaml-required

Running `specscore spec lint` in a directory tree that has no `specscore.yaml` in any ancestor exits `3`. The error message names `specscore.yaml` as mandatory and instructs the caller to run `specscore init` to create it. The presence of a bare `spec/features/` directory does NOT satisfy the gate.
### AC: entity-and-property-rules-selectable

**Requirements:** cli/spec/lint#req:entity-and-property-rules-registered

`specscore spec lint --rules entity-location` and `specscore spec lint --rules property-frontmatter-required-fields` each run successfully (exit `0` on a clean tree) without rejecting the rule name. Each name in [cli/entity](../../entity/README.md)'s and [cli/property](../../property/README.md)'s rule tables is accepted by `--rules`; passing a misspelled name (e.g., `entity-locatoin`) exits `2` per [REQ: rules-whitelist](#req-rules-whitelist).

### AC: managed-section-fix-idempotent

**Requirements:** cli/spec/lint#req:managed-section-rewrite-on-fix, cli/spec/lint#req:fix-is-idempotent

Given a tree containing entity files with stale `## Properties` tables and property files with stale `## Referenced by` sections, running `specscore spec lint --fix` once rewrites every managed body to its canonical rendering and exits `0`. Running `specscore spec lint --fix` a second time produces zero file changes and exits `0` with no violation lines printed.

### AC: consumer-path-multi-glob-parsed

**Requirements:** cli/spec/lint#req:consumer-path-multi-glob

Given a registry row whose `Consumer Path` cell is `spec/features/**/*.entity.md, spec/features/**/*.property.md`, the CLI's parser produces a glob list of length two containing both globs. Whitespace around the comma is tolerated. A cell value of `—` (or the empty string) produces an empty glob list. Cell values with leading, trailing, or doubled commas (e.g. `,a,b`, `a,b,`, `a,,b`) MUST produce a glob list with the empty entries silently discarded, never an error.

### AC: fix-reports-only-changed-files

**Requirements:** cli/spec/lint#req:fix-reports-modified-files

Given a spec tree with exactly one file needing an autofix (e.g. a single missing adherence footer), running `specscore spec lint --fix --format json` reports that one file's project-relative path in the `fixed` array and no other paths. Running the same command a second time (now a clean tree, per `fix-is-idempotent`) reports an empty `fixed` array.

### AC: fix-json-envelope-only-under-fix

**Requirements:** cli/spec/lint#req:fix-report-json-envelope

Given a spec tree with one autofixable violation, `specscore spec lint --fix --format json` emits a single JSON object on stdout containing both a `fixed` array and a `violations` array. The same tree linted with `specscore spec lint --format json` (no `--fix`) emits a bare JSON array of violations with no `fixed` key — the pre-existing non-fix contract is preserved.

### AC: fix-text-summary-on-stderr

**Requirements:** cli/spec/lint#req:fix-report-text-summary

Given a spec tree with one autofixable violation, running `specscore spec lint --fix` in default text format writes a summary naming the modified path to stderr while stdout carries only the remaining-violations report. Running `specscore spec lint --fix` again on the now-clean tree writes no stderr summary (zero files modified).

### AC: fix-report-needs-no-flag

**Requirements:** cli/spec/lint#req:fix-report-default-on

Given a spec tree with one autofixable violation, running `specscore spec lint --fix` with no flags beyond `--fix` (and an optional `--format`) produces the fixed-files report — the stderr summary in text format, the `fixed` key in `--format json|yaml` — without any additional enabling flag.

## Open Questions

- Should `spec lint` accept a path argument (`spec lint spec/features/cli/`) to lint a subtree, for faster feedback during development? Today the full tree is always scanned.
- Should `--fix` have a paired `--dry-run` that prints the intended edits without applying them, so authors can preview fixes before accepting?
- **Should a `lint-rule-table-completeness` rule fire when a Doc-Kind's CLI Feature's rule table drifts from `pkg/lint`'s `allRuleNames` map** (e.g., a rule is in `allRuleNames` but missing from the table, or vice versa)? Lean: yes, as a follow-on to the entity/property work — keeps the Feature spec and the code in lockstep.
- **Should the multi-glob `Consumer Path` parser be exposed as a public helper in `pkg/projectdef`** (so consumers outside `pkg/lint` can use the same parser) or stay package-private to `pkg/lint`? Lean: package-private until a second caller emerges, per the meta-spec convention of deferring extraction.
- **How should `pkg/lint`'s `Lint()` surface the fixed-files set** — a new `Result{ Violations, Fixed }` struct, a second return value, or a sibling `LintWithResult`? This affects the public `pkg/lint` Go API; settle at plan time. (Carried from the `lint-fix-reports-modified-files` Idea.)
- **Is a terse `--format paths` (one fixed path per line on stdout) worth adding** for `lint --fix | git add`-style pipelines, or does the JSON `.fixed[]` field suffice? Deferred from MVP. (Carried from the `lint-fix-reports-modified-files` Idea.)
- When should GraphSpec validation graduate from `specscore graph lint` into default `specscore spec lint`?

---
*This document follows the https://specscore.md/feature-specification*
