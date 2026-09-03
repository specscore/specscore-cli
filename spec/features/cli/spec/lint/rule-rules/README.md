---
format: https://specscore.md/feature-specification
status: Implementing
---

# Feature: Rule Lint Rules

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/rule-rules?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/rule-rules?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/rule-rules?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/rule-rules?op=request-change) |
**Status:** Implementing
**Source Ideas:** —

## Summary

Adds the `R-001`–`R-011` lint family for the [Rule](../../../rule/README.md) artifact kind, which has two forms sharing one identity: an inline rule is a single row in `spec/rules/README.md`, and a detailed rule is that same row plus `spec/rules/<slug>/README.md`. Also registers the two doc types (`Rule detail README`, `rules-index README`) that bring rules under the shared frontmatter, status-mirror, and adherence-footer rules.

One checker is registered under all eleven ids, mirroring the `L-` family. `--fix` repairs the index table shape, adds a row for an orphan document, corrects a stale link cell, and rewrites a drifted document header from its row — never the reverse, and never the prose an author wrote.

## Problem

A rule is normative text: other people and other agents act on it. That makes the failure modes specific and worth checking mechanically.

A rule claiming `Enforced` with no `Control` is the worst of them — it reads as binding, so a reviewer stops looking for the gate, and no gate exists. A detail document whose header disagrees with its row is the second: two representations of one rule, each authoritative to whoever happened to open it, is how one rule quietly becomes two. A rule whose Sources name a Lesson that never points back is the third: the reasoning link rots invisibly, and the next reader cannot tell whether the Lesson was renamed, deleted, or never connected. A skill listing a rule that no longer exists is the fourth: the instruction survives its constraint.

None of these can be caught by reading one file. They need the whole tree, which is what lint is for.

## Behavior

### Detection

#### REQ: rule-detection

The family MUST read `spec/rules/README.md` for rows and `spec/rules/<slug>/README.md` for detail documents whose first H1 heading matches `# Rule: <title>`. A `spec/rules/` directory that does not exist MUST produce no violations at all, so the family is silent in a repository that has recorded no rule.

#### REQ: undiscoverable-directories-reported

A directory under `spec/rules/` that contains a `README.md` but is skipped by discovery — because its name is not a canonical slug, or because its body declares no `# Rule:` heading — MUST be reported as `R-001`. Silently skipping it would leave the file on disk and invisible to the index and to every other rule.

### R-001 — detail-document shape

#### REQ: rule-r-001-shape

`R-001` MUST be registered at severity `error` and MUST report: a missing, duplicated, or out-of-order member of the ordered field set (`Status`, `Date`, `Owner`, `Statement`, `Scope`, `Enforcement`, `Control`, `Sources`, `Why`, `Exceptions`, `Supersedes`, `Superseded By`) — except `Scope` and `Sources`, which are list fields and may legitimately repeat across lines; an empty value in a field that must carry content; an empty value where the em-dash sentinel is required; a `**Date:**` that is not `YYYY-MM-DD`; a missing `## Instructions`, `## Examples` or `## Open Questions` section; a `## Examples` lacking either `### Compliant` or `### Violation`; and an empty `# Rule:` title.

### R-002 / R-003 — row validity and index shape

#### REQ: rule-r-002-status-values

`R-002` MUST report an index row whose `Status` is outside `Draft`, `Active`, `Superseded`, naming the offending value and the accepted set.

#### REQ: rule-r-003-index-shape

`R-003` MUST report a rules index that lacks the canonical seven-column header, a row that cannot be represented by that shape (including an identity cell that is neither a bare slug nor `[slug](slug/README.md)`), a duplicated slug, rows not sorted by slug, and a row with no Statement. It MUST also report a `spec/rules/` holding detail documents but no `README.md` index, since the index is where a rule is published. Every row-scoped violation MUST carry the row's source line.

### R-004 — row ↔ document pairing

#### REQ: rule-r-004-pairing

`R-004` MUST report a linked row with no detail document, a detail document whose row does not link to it, and a detail document with no row at all. `--fix` MUST add the missing row (projected from the document) and correct a stale link cell in either direction; a linked row with no document MUST stay a finding, because writing that document is authoring, not repair.

### R-005 / R-006 / R-007 — field validity

#### REQ: rule-r-005-control-required

`R-005` MUST report an `Enforcement` value outside `Stated`, `Enforced`, `Automated`, and MUST report an `Enforced` or `Automated` rule whose `Control` is empty or the sentinel. The message MUST say that an enforced rule with no control is a stated rule wearing a stronger label. `Stated` MUST NOT require a control.

#### REQ: rule-r-006-scope-values

`R-006` MUST report an empty scope list, a duplicated scope entry, and any entry that is not `fleet`, `product:<name>`, `repo:<owner>/<repository>`, or `path:<glob>` (validated as a doublestar pattern).

#### REQ: rule-r-007-source-resolution

`R-007` MUST report a duplicated source entry, a malformed reference, and a typed reference that does not resolve: `lesson:<slug>` against `spec/lessons/`, `idea:<slug>` against the active and archived Idea directories, `decision:<NNNN|NNNN-slug>` against `spec/decisions/` and its `archived/` subtree. An `http(s)` URL MUST be accepted on syntax alone — resolving it would make lint depend on the network.

### R-008 — the strict lesson↔rule pair

#### REQ: rule-r-008-bidirectional

`R-008` MUST check the promotion relation in both directions. A rule listing `lesson:<slug>` in Sources MUST be named by that Lesson's `**Promotes To:**`; a Lesson carrying `**Promotes To:** rule:<slug>` MUST name a rule listed in the index whose Sources cite it. A Lesson promoting to a *different* rule than the one citing it MUST be reported against the citing rule, naming both targets. A malformed `**Promotes To:**` value MUST be reported against the Lesson. Every message MUST name the command that repairs it. When Lesson discovery itself fails, the pairing check MUST degrade to "no lessons" rather than aborting the lint run.

### R-009 — supersession integrity

#### REQ: rule-r-009-supersession

`R-009` MUST report a `**Supersedes:**` or `**Superseded By:**` value that does not resolve to a rule listed in the index, a `**Supersedes:**` target lacking the inverse `**Superseded By:**` pointer, a `Superseded` rule with no `**Superseded By:**`, and a supersession cycle. Supersession is a detail-document concept: an inline rule has no supersession fields to check.

### R-010 — the rule↔skill pair

#### REQ: rule-r-010-skill-pair

`R-010` MUST check the rule↔skill relation in both directions. A detail document referencing `skill:<name>` MUST reach a skill under the configured skills directory whose `## Rules` section lists `rule:<slug>`; a skill listing `rule:<slug>` MUST reach an indexed rule whose Instructions reference `skill:<name>`. A skill naming an *inline* rule MUST be reported naming `specscore rule expand`, because an inline rule has nowhere to reciprocate. Only references under a skill's `## Rules` heading count as declarations: a `rule:` token in prose is discussion, and treating it as a declaration would make the check fire on documentation. The skills directory MUST default to `ai/skills` and honour a `rules.skills_path` override in `specscore.yaml`.

### R-011 — the mirror

#### REQ: rule-r-011-mirror

`R-011` MUST report a detail document whose `Status`, `Statement`, `Scope`, `Enforcement`, `Control` or `Sources` differs from its index row, naming both values and stating that the row is the source of truth. `--fix` MUST rewrite the *document* from the row (including the frontmatter `status:` mirror) and MUST NEVER rewrite the row from the document. A document with no row has nothing to mirror against and MUST be left to `R-004`.

### Fix-pass discipline

#### REQ: rule-fix-is-bounded-and-idempotent

`--fix` MUST be idempotent: a second pass changes no byte. It MUST NEVER modify a detail document's `Why`, `Exceptions`, `## Instructions` or `## Examples`: those are the author's normative prose, and a fixer that rewrote them would be inventing policy. It MUST NEVER derive a row's data cells from a document except when the row is entirely missing — the one direction in which a document may seed a row, and only because the alternative is an artifact invisible to every reader of the index.

### Shared doc-type registration

#### REQ: rule-doc-types-registered

`Rule detail README` (status-bearing, `https://specscore.md/rule-specification`) and `rules-index README` (status-less, `https://specscore.md/rules-index-specification`) MUST be registered as document types, so the shared `format-field`, `status-mirror`, `footer-format-mirror` and `adherence-footer` rules cover rules without any rule-specific code. The detail walker MUST visit only `spec/rules/<slug>/README.md` and MUST exclude the index.

#### REQ: rule-family-registry-parity

Every `R-` id MUST appear in the lint rule registry with a non-empty description, family `rule`, and severity `error`, so `specscore rules` and `docs/lint-rules.md` list them and `CheckRegistryParity` passes.

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [spec lint](../README.md) | Parent: hosts the family in the default rule suite and the `--fix` pass. |
| [cli/rule](../../../rule/README.md) | The verbs whose artifacts this family validates; `specscore rule lint` runs this family alone. |
| [lesson-rules](../lesson-rules/README.md) | Structural sibling whose one-checker-many-ids shape this family mirrors; `R-008` reads the Lesson half of the promotion pair it owns. |

## Acceptance Criteria

### AC: family-is-silent-without-rules (verifies REQ:rule-detection)

**Given** a spec tree with no `spec/rules/` directory
**When** `specscore spec lint` runs
**Then** no `R-` violation is reported.

### AC: both-forms-are-clean-when-scaffolded (verifies REQ:rule-r-001-shape)

**Given** one inline rule and one detailed rule written by `specscore rule new`
**When** `specscore spec lint` runs
**Then** no violation is reported against any path under `rules/` — including the shared frontmatter, status-mirror and adherence-footer rules.

### AC: undiscoverable-directory-is-reported (verifies REQ:undiscoverable-directories-reported)

**Given** `spec/rules/x/README.md` with no `# Rule:` heading
**When** `specscore spec lint` runs
**Then** an `R-001` violation reports that the document is invisible to the index and to every other check.

### AC: index-shape-problems-are-reported-with-lines (verifies REQ:rule-r-003-index-shape)

**Given** a rules index with a five-cell row, a duplicated slug, and rows out of slug order
**When** `specscore spec lint` runs
**Then** `R-003` reports each problem, and the row-scoped violations carry the offending source line.

### AC: pairing-is-reported-in-both-directions (verifies REQ:rule-r-004-pairing)

**Given** one linked row with no document and one document whose row does not link
**When** `specscore spec lint` runs
**Then** `R-004` reports both; `--fix` adds the missing link and leaves the unwritten document a finding.

### AC: enforced-without-control (verifies REQ:rule-r-005-control-required)

**Given** a row with `Enforced` and a sentinel `Control`
**When** `specscore spec lint` runs
**Then** an `R-005` violation states that an enforced rule with no control is a stated rule wearing a stronger label.

### AC: unresolvable-source (verifies REQ:rule-r-007-source-resolution)

**Given** a rule whose Sources list `lesson:ghost`
**When** `specscore spec lint` runs
**Then** an `R-007` violation reports that the source does not resolve to a Lesson under `spec/lessons/`.

### AC: lesson-pair-is-checked-both-ways (verifies REQ:rule-r-008-bidirectional)

**Given** a rule citing `lesson:l` where Lesson `l` has no `**Promotes To:**`
**When** `specscore spec lint` runs
**Then** an `R-008` violation is reported against the row naming the repair command; and in the mirror case — a Lesson promoting to a rule that does not cite it — an `R-008` violation is reported against the Lesson.

### AC: skill-pair-is-checked-both-ways (verifies REQ:rule-r-010-skill-pair)

**Given** a detail document referencing `skill:go-hygiene` and a skill that does not list it back
**When** `specscore spec lint` runs
**Then** an `R-010` violation names the exact line to add; a skill listing a rule that is not indexed, or one that is inline, is reported against the skill.

### AC: configured-skills-path-is-honoured (verifies REQ:rule-r-010-skill-pair)

**Given** skills kept at `tools/skills` and `rules.skills_path: tools/skills` in `specscore.yaml`
**When** `specscore spec lint` runs
**Then** the pair resolves; without the override the same tree reports `R-010`.

### AC: mirror-drift-is-repaired-from-the-row (verifies REQ:rule-r-011-mirror)

**Given** a detail document whose `**Status:**` disagrees with its row
**When** `specscore spec lint --fix` runs
**Then** the document's body and frontmatter statuses are rewritten to the row's value, the row is unchanged, and a second pass changes no byte.

### AC: fix-never-edits-authored-content (verifies REQ:rule-fix-is-bounded-and-idempotent)

**Given** a drifted detail document carrying hand-written Instructions and Examples
**When** `specscore spec lint --fix` runs
**Then** those sections are byte-identical afterwards.

### AC: registry-parity (verifies REQ:rule-family-registry-parity)

**When** the registry-parity check runs
**Then** every `R-` id the checker can emit is present in the registry and no registry entry lacks an emitting checker.

## Open Questions

- `R-009` requires an inverse `**Superseded By:**` on a `**Supersedes:**` target but no verb writes both halves the way `rule promote` does for the lesson pair. Should `rule update --supersedes` write the inverse pointer automatically, or is the explicit two-command form the safer default?
- The family reports an unresolvable `decision:` reference but accepts any four-digit number that matches a filename prefix. Should it validate that the matched file is actually a Decision artifact, or is filename evidence enough given the `D-` family already owns that tree?
- `R-010` reads skills from a single directory. A repository that vendors skills from several sources (a plugin, a shared pack) would need a list. Should `rules.skills_path` accept one?

---
*This document follows the https://specscore.md/feature-specification*
