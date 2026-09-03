---
format: https://specscore.md/feature-specification
status: Implementing
---

# Feature: Rule (CLI)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rule?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rule?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rule?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rule?op=request-change) |
**Status:** Implementing
**Source Ideas:** —

## Summary

`specscore rule` is the CRUD surface for the **Rule** artifact kind: one normative sentence — MUST or NEVER, in plain words — carrying the scope it binds, the reason it exists, the sources that produced it, and the control that enforces it.

A rule has two forms and one identity. An **inline** rule is exactly one row in `spec/rules/README.md` and nothing else. A **detailed** rule keeps the identical row, linked to `spec/rules/<slug>/README.md`, which adds the reason, worked compliant and violating examples, agent instructions, exceptions, and supersession. The index row is the source of truth for every field it carries.

The group exposes `new`, `expand`, `list`, `show`, `update`, `delete`, `promote`, and `lint`; every verb is non-interactive and accepts `--format text|yaml|json`.

Note the deliberate singular. `specscore rule` is this artifact kind; [`specscore rules`](../rules/README.md) (plural) is the unrelated read-only catalog of *lint* rules. The two are distinct commands and neither shadows the other.

## Synopsis

```
specscore rule new <slug> [--statement …] [--scope …]... [--source …]... [--status …]
                          [--enforcement Stated|Enforced|Automated] [--control …]
                          [--detailed] [--title …] [--owner …] [--date …]
                          [--why …] [--exceptions …] [--instructions …]
                          [--compliant …] [--violation …] [--supersedes …] [--skill …]... [--force]
specscore rule expand <slug> [--why …] [--instructions …] [--compliant …] [--violation …]
                             [--exceptions …] [--supersedes …] [--skill …]... [--title …] [--owner …] [--date …]
specscore rule list [--scope …] [--status …] [--enforcement …] [--applies-to <path>]
specscore rule show <slug>
specscore rule update <slug> [--statement …] [--scope …]... [--status …]
                             [--enforcement … --control …] [--add-source …]... [--remove-source …]...
                             [--title …] [--why …] [--exceptions …] [--supersedes …] [--superseded-by …]
specscore rule delete <slug> [--supersede-with <slug>]
specscore rule promote --from-lesson <lesson-slug> <rule-slug> [--inline] [--statement …] [--scope …]...
                       [--enforcement …] [--control …] [--why …] [--skill …]...
specscore rule lint [--fix]
```

## Problem

Durable operating knowledge — "never mock an extension backend", "always gofmt before building", "pin the exact peer version at the publishing package" — accumulates in per-agent memory files. Those files do not transfer. A new session, a new machine, or a different agent runtime starts blind, and the knowledge is re-learned the expensive way: by repeating the mistake it encodes.

The existing kinds each hold part of the answer and none of it whole. A [Lesson](../lesson/README.md) explains the *process gap* a defect exposed; a Decision explains *why a choice was made*; an Idea explains *a direction*. What none of them is, is the single transferable sentence you can hand to a fresh agent with no other context. A Lesson is a narrative about the past; a rule is an instruction about the next commit.

The two forms exist because the two failure modes are opposite. Give every rule a directory and recording one becomes ceremony, so nobody records the ninety per cent that really are one sentence. Give no rule a document and the ten per cent that need a worked example — where the boundary actually falls, what a compliant diff looks like — have nowhere to put it, and the rule gets read three different ways. One row is the floor; a document is the opt-in ceiling.

## Behavior

### An inline rule is one row, and the row is the rule

#### REQ: canonical-row

The rules index MUST carry a `## Rules` table with exactly the header `| Rule | Status | Scope | Enforcement | Control | Sources | Statement |`. Each row's identity cell MUST be either a bare canonical slug (an inline rule) or `[<slug>](<slug>/README.md)` (a detailed rule) — no other shape, because an identity cell is the one place a guess would silently rename a rule. Rows MUST be unique by slug and sorted by slug. Free text in a cell MUST have its pipes escaped so a statement can never fabricate a column.

#### REQ: unparseable-rows-are-never-dropped

A row-like line the seven-column contract cannot represent MUST be preserved verbatim by every write path, and reported. No verb may reduce the rule set by writing the index — an unescaped `|` pasted into a Statement is the most ordinary authoring slip there is, and a kind that exists so operating knowledge stops evaporating cannot have a path where a benign command evaporates some.

A mutating verb MUST refuse (exit `1`) while the index holds a line it cannot read, naming the line, the slug its identity cell suggests, the parse failure, and the sanctioned repair. The one exception is a line whose identity cell names the very rule the verb is writing: that line *is* that row, and the verb replaces it.

`--fix` MAY repair such a line only when the repair is unambiguous — surplus cells caused by an unescaped `|` in the Statement, which is provable because the four columns between the identity cell and the Statement have closed grammars that must all validate first. Every other shape is preserved and reported, never guessed at. A duplicate row is removed only when it is byte-identical to the row it repeats; two rows that disagree are ambiguous and are both kept.

#### REQ: scaffold-needs-only-a-slug

`specscore rule new <slug>` with no other flag MUST record a lint-clean `Draft` inline row with `fleet` scope, `Stated` enforcement, and a TODO statement, and MUST NOT create a directory. Recording a rule under time pressure has to be one command, for the same reason it does for Lessons: a rough rule a later `rule update` sharpens beats a rule that was never written down.

#### REQ: detailed-form-is-opt-in

`--detailed` MUST additionally write `spec/rules/<slug>/README.md` and link the row to it. Any document-only flag (`--why`, `--exceptions`, `--instructions`, `--compliant`, `--violation`, `--supersedes`, `--skill`) MUST imply `--detailed`, because those fields have nowhere else to live. `specscore rule expand <slug>` MUST give an existing inline rule a document, seeding its mirrored header from the row so the new document starts in agreement with it; expanding a rule that already has one is a conflict, not an overwrite.

### The row is the source of truth

#### REQ: mirror-is-row-governed

A detail document MUST repeat the row's `Status`, `Statement`, `Scope`, `Enforcement`, `Control` and `Sources` in its header, and those values MUST equal the row's. `rule update` MUST write the row first and then mirror every changed value into the document; `spec lint --fix` MUST repair a mismatch by rewriting the document from the row and MUST NEVER rewrite the row from the document. Two representations of one rule that are each authoritative in some direction is exactly how they become two different rules.

#### REQ: document-fields-are-the-authors

`Why`, `Exceptions`, `Supersedes`, `Superseded By`, `## Instructions` and `## Examples` belong to the document alone. No fixer may rewrite them: they are normative prose, and a fixer that guessed at them would be inventing policy rather than repairing formatting. Editing them on an inline rule MUST exit `4` naming `specscore rule expand`.

#### REQ: detail-document-shape

A detail document MUST declare, in this exact order: `**Status:**`, `**Date:**`, `**Owner:**`, `**Statement:**`, `**Scope:**`, `**Enforcement:**`, `**Control:**`, `**Sources:**`, `**Why:**`, `**Exceptions:**`, `**Supersedes:**`, `**Superseded By:**`, followed by `## Instructions`, `## Examples` (with both `### Compliant` and `### Violation`), and `## Open Questions`. `Control`, `Sources`, `Supersedes` and `Superseded By` MUST carry the em-dash sentinel `—` when empty, so "absent" and "the author forgot" never look alike. Both examples are required: an example set showing only the happy path leaves the reader guessing at the boundary the rule draws.

### Closed vocabularies, and a tier that must be backed

#### REQ: superseded-requires-a-successor

`Superseded` is a detail-document status: supersession pointers live there, so an inline row at `Superseded` has nowhere to name its successor. A verb asked to write one MUST exit `4` naming `specscore rule expand`, and lint MUST report a tree that already contains one. A retired rule with no forwarding address is indistinguishable from a live one to every reader.

#### REQ: closed-vocabularies

`Status` MUST be one of `Draft`, `Active`, `Superseded`. `Enforcement` MUST be one of `Stated` (an agent or human is told; nothing refuses), `Enforced` (a named control refuses), `Automated` (a named control refuses and repairs). `Enforced` and `Automated` MUST name a non-empty `Control`; `Stated` MUST NOT be required to, because the absence of a control is exactly what that tier means. Any verb that would write a control-requiring tier without a control MUST exit `2` before touching the tree.

#### REQ: scope-grammar

Each `Scope` entry MUST be `fleet`, `product:<name>`, `repo:<owner>/<repository>`, or `path:<glob>`. A bare unprefixed token other than the `fleet` keyword MUST be rejected rather than guessed at — a mis-scoped rule binds work it was never meant to bind, which is worse than a rejected one.

#### REQ: source-references

Each `Sources` entry MUST be `lesson:<slug>`, `decision:<NNNN|NNNN-slug>`, `idea:<slug>`, or an `http(s)` URL. Typed references MUST resolve to an existing artifact in the same spec tree, and every verb that writes one MUST refuse (exit `2`) when it does not — grammar and resolution are checked at the same moment, so a typo'd cross-link cannot land silently and leave lint to find it later. Free URLs are validated syntactically only, because resolving them would make lint depend on the network.

### Lessons promote into rules, and the pair is strict

#### REQ: lesson-promotion-pair

A Lesson MAY carry an optional `**Promotes To:** rule:<slug>` field, placed after its canonical relation block so an existing Lesson stays lint-clean without it. That pointer and the rule's `lesson:<slug>` source are one relation and MUST agree in both directions. `specscore rule promote` writes both halves in one command, detailed by default — a Lesson already carries the reason a rule needs, so discarding it to save a file would lose the more valuable half.

One consequence is deliberate and is why the check can be strict at all: a Lesson carries exactly *one* promotion pointer, so it can be the source of at most one rule, while a rule may cite several Lessons. Promoting a Lesson that already promotes elsewhere MUST exit `4` rather than silently repointing it. A Lesson that merely informed a rule without producing it belongs in that rule's `**Why:**`, not its Sources.

### Rules pair with skills

#### REQ: skill-pair

A detail document's Instructions MAY reference `skill:<name>`, and a skill's `## Rules` section MAY list `rule:<slug>`. Both directions MUST resolve and MUST reciprocate: a skill that silently outlives the rule constraining it is the failure this catches. Only references under a skill's `## Rules` heading count as declarations — a `rule:` token in prose is discussion. Skills are read from `ai/skills/` unless `rules.skills_path` in `specscore.yaml` says otherwise. A skill may only bind a *detailed* rule, because an inline rule has nowhere to name the skill back.

### Reads are cheap and answer the question an agent has

#### REQ: list-reads-only-the-index

`rule list` MUST read only `spec/rules/README.md` and MUST NOT open a detail document. The listing has to stay cheap enough for a tool to run it at the start of every agent stream and print the rules that apply.

#### REQ: applies-to-resolves-scope

`rule list --applies-to <path>` MUST list only the rules whose Scope covers that path, by this table:

| Scope | Matches |
|---|---|
| `fleet` | everything |
| `path:<glob>` | doublestar against the path AND against every trailing suffix of it, so a repo-relative pattern still matches an absolute path a caller holds. Deliberately generous: `path:cli/**` also matches `vendor/x/cli/y.go`. |
| `product:<name>` | `<name>` appears as a whole path segment |
| `repo:<owner>/<name>` | `<owner>` and `<name>` appear as **consecutive** whole path segments. A bare `<name>` MUST NOT match. |

The repo rule is strict on purpose. Matching a bare repository name would make every rule scoped to a repo called `docs`, `api`, `web` or `cli` bind every path in the fleet containing that directory — and a mis-scoped rule that binds work it was never meant to bind is worse than one that fails to match, because nobody goes looking for it.

`--scope`, `--status` and `--enforcement` are exact, case-insensitive filters that compose with `--applies-to` under AND semantics. An unrecognized filter value MUST exit `2` naming it, never resolve to a silently empty result.

#### REQ: unreadable-scope-fails-toward-binding

A rule whose Scope cell does not parse MUST still be listed, flagged `scope_error`, reported on stderr, and the command MUST exit `1`. `rule list` is designed to run standalone at the start of an agent stream with no lint pass behind it, so dropping the rule would answer "nothing applies" to a question whose real answer is "one rule might, and its scope needs a human".

#### REQ: structured-output-is-portable

`--format json` and `--format yaml` MUST emit repo-relative paths and MUST NOT emit the em-dash sentinel: it is a storage convention of the Markdown table, not a value. A missing scalar is the empty string and a missing list is an empty array — one convention across every field, so a consumer needs no special case. `--format text` keeps absolute paths, because that output is what a caller hands to an editor.

#### REQ: reads-never-mutate

`list` and `show` MUST leave the spec tree byte-identical. `show` MUST render either form — reporting which one — and MUST additionally resolve what a reader cannot get by opening the files: the Lessons promoting to this rule, the Features citing it, the skills it binds, and any source reference that does not resolve.

### Mutations are bounded and reversible-by-review

#### REQ: bounded-write-set

`new`, `expand`, `update`, `promote` and `delete` MUST touch only the requested rule's row and directory, the two ancestor indexes (`spec/README.md`, `spec/rules/README.md`), and — for `promote` and `delete --supersede-with` — the specific Lessons and rules whose links they repoint. No mutating verb may run a repository-wide fixer; that stays exclusive to `specscore spec lint --fix`.

#### REQ: update-is-in-place

Editing a detail document MUST rewrite only the named bold fields and leave every other byte untouched — comments, worked examples, hand-written Open Questions. Regenerating from a template would silently discard an author's examples and make the verb unsafe to run on a reviewed rule. `rule update` with no edit flag MUST exit `2`.

#### REQ: source-edits-are-incremental

`--add-source` and `--remove-source` edit the existing list. Adding a source already present, or removing one that is absent, MUST exit `2` rather than silently succeeding — a no-op that reports success hides a typo in a reference the reader will later trust.

#### REQ: delete-refuses-live-links

`rule delete <slug>` MUST exit `4`, naming every blocker, while any Lesson promotes to the rule, any other rule supersedes or is superseded by it, any skill lists it, or any Feature cites it. `--supersede-with <slug>` MUST instead repoint every Lesson pointer and rule relation at the named successor — adding each inherited Lesson to the successor's Sources so the strict pair still holds, and recording `**Supersedes:** <old>` on the successor when it is detailed — and only then remove the row and any directory. That breadcrumb is the trail back for the prose references the verb has just warned about, so a `**Supersedes:**` target that no longer exists is history rather than a defect; `**Superseded By:**` still MUST resolve, because a forward pointer with no destination leaves a reader nowhere.

Skill and Feature references are prose: they MUST be reported rather than rewritten.

### The rule family is part of `spec lint`

#### REQ: lint-family-registered

The `R-001`..`R-011` rules MUST be registered in the lint rule registry and run as part of `specscore spec lint`; `specscore rule lint` is a focused view of the same family. Their contract lives in [cli/spec/lint/rule-rules](../spec/lint/rule-rules/README.md).

### Shared flags

Every command in this group accepts the shared flags defined in the [CLI parent](../README.md): `--project`, `--format`, and `-h/--help`.

## Exit codes

| Code | Condition |
|---|---|
| `0` | The operation succeeded (a listing may legitimately be empty). |
| `1` | A `rule lint` run reported at least one error-severity violation, or a `new`/`promote`/`expand` target already exists and `--force` was not passed. |
| `2` | Invalid arguments: a missing or extra positional, an invalid slug, an unrecognized `--format`/`--status`/`--enforcement`/`--scope`/`--source`/`--skill` value, a control-requiring tier with no control, an `update` with no edit flag, a duplicate `--add-source`, or an absent `--remove-source`. |
| `3` | The named rule is not listed in the index, or (for `promote`) the named Lesson does not exist. |
| `4` | `delete` refused because live links remain; `promote` refused because the Lesson already promotes to a different rule; or a document-only edit was requested on an inline rule. |
| `10` | An unexpected I/O or parse failure. |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [CLI](../README.md) | Inherits the shared exit-code contract, `--format`/`--project` conventions, and project autodetection. |
| [cli/lesson](../lesson/README.md) | Closest structural sibling and a cross-linked kind: a Lesson's `**Promotes To:**` and a rule's `lesson:` source are one strict relation, written together by `rule promote`. |
| [cli/spec/lint](../spec/lint/README.md) | Hosts the `R-001`–`R-011` family documented in [cli/spec/lint/rule-rules](../spec/lint/rule-rules/README.md). |
| [cli/rules](../rules/README.md) | Unrelated despite the near-identical name: that group lists *lint* rules, this one manages the Rule artifact kind. |

## Acceptance Criteria

### AC: new-is-inline-by-default (verifies REQ:scaffold-needs-only-a-slug)

**Given** a SpecScore project with no `spec/rules/` directory
**When** the user runs `specscore rule new never-mock-backends --statement "…"`
**Then** `spec/rules/README.md` gains one row whose identity cell is the bare slug, no `spec/rules/never-mock-backends/` directory is created, and `specscore rule lint` exits `0`.

### AC: slug-only-new-is-lint-clean (verifies REQ:scaffold-needs-only-a-slug)

**When** the user runs `specscore rule new x` with no other flag
**Then** the row is recorded and `specscore rule lint` exits `0`.

### AC: detail-flags-imply-detailed (verifies REQ:detailed-form-is-opt-in)

**When** the user runs `specscore rule new x --why "…"` without `--detailed`
**Then** `spec/rules/x/README.md` is written and the row links to it.

### AC: expand-seeds-from-the-row (verifies REQ:detailed-form-is-opt-in)

**Given** an inline rule whose row says `Active`, `path:**/*.go`, `Enforced` with a control
**When** the user runs `specscore rule expand <slug>`
**Then** the new document repeats all of those values verbatim, the row's identity cell becomes a link, and `specscore rule lint` exits `0`.

### AC: mirror-drift-is-reported-and-fixed-from-the-row (verifies REQ:mirror-is-row-governed)

**Given** a detail document whose `**Status:**` was hand-edited to disagree with its row
**When** the user runs `specscore spec lint`
**Then** an `R-011` violation names both values; running `--fix` rewrites the *document* to match the row, leaves the row unchanged, and a second `--fix` changes no byte.

### AC: fix-never-edits-authored-content (verifies REQ:document-fields-are-the-authors)

**Given** a drifted detail document carrying hand-written Instructions and Examples
**When** the user runs `specscore spec lint --fix`
**Then** the mirrored header is repaired and the Why, Instructions and Examples are byte-identical.

### AC: document-only-edit-on-an-inline-rule-exits-4 (verifies REQ:document-fields-are-the-authors)

**When** the user runs `specscore rule update <inline-slug> --why "…"`
**Then** the command exits `4` naming `specscore rule expand`, and nothing is written.

### AC: closed-vocabularies-are-enforced (verifies REQ:closed-vocabularies)

**When** the user runs `specscore rule new x --enforcement Enforced` with no `--control`
**Then** the command exits `2` naming the missing control, and no row is written.

### AC: scope-grammar-rejects-a-bare-token (verifies REQ:scope-grammar)

**When** the user runs `specscore rule new x --scope sneat`
**Then** the command exits `2` explaining that a scope must be `fleet`, `product:<name>`, `repo:<owner/repo>`, or `path:<glob>` — it is never guessed to be a product name.

### AC: applies-to-selects-by-scope (verifies REQ:applies-to-resolves-scope)

**Given** a rule `alpha` scoped `fleet` and a rule `zeta` scoped `path:**/*.go`
**When** the user runs `specscore rule list --applies-to docs/x.md`
**Then** stdout lists only `alpha`; running it with `--applies-to internal/cli/x.go` lists both.

### AC: unrecognized-filter-exits-2 (verifies REQ:applies-to-resolves-scope)

**When** the user runs `specscore rule list --status bogus`
**Then** the command exits `2` naming `bogus` and the legal set — never an empty-but-successful result, which would read as "no rules apply".

### AC: show-renders-both-forms (verifies REQ:reads-never-mutate)

**Given** one inline rule and one detailed rule
**When** the user runs `specscore rule show <slug> --format json` against each
**Then** both documents carry `form`, the detailed one additionally carries `detail_path`, `why` and `skills`, the inline one leaves those empty, and the spec tree is byte-identical afterwards.

### AC: update-preserves-unrelated-bytes (verifies REQ:update-is-in-place)

**Given** a detailed rule whose document carries a hand-written HTML comment
**When** the user runs `specscore rule update <slug> --statement "Never x."`
**Then** the row and the document's mirrored statement both change, the comment is still present byte-for-byte, and lint stays clean.

### AC: source-edits-are-incremental (verifies REQ:source-edits-are-incremental)

**When** the user runs `specscore rule update <slug> --remove-source decision:9999` for a source the rule does not list
**Then** the command exits `2` and nothing is written.

### AC: promote-writes-both-halves (verifies REQ:lesson-promotion-pair)

**Given** a canonical Lesson whose `**Control:**` names an enforceable sentence
**When** the user runs `specscore rule promote --from-lesson <lesson> <rule>`
**Then** the new rule is detailed, its statement is pre-filled from that Control, its Sources list `lesson:<lesson>`, the Lesson gains `**Promotes To:** rule:<rule>`, and `specscore rule lint` exits `0`.

### AC: a-lesson-promotes-once (verifies REQ:lesson-promotion-pair)

**Given** a Lesson already carrying `**Promotes To:** rule:a`
**When** the user runs `specscore rule promote --from-lesson <that lesson> b`
**Then** the command exits `4` naming the existing target, and neither artifact is modified.

### AC: broken-lesson-pair-is-reported-both-ways (verifies REQ:lesson-promotion-pair)

**Given** a rule listing `lesson:l` in Sources and a Lesson `l` with no `**Promotes To:**`
**When** the user runs `specscore spec lint`
**Then** an `R-008` violation reports that the source does not point back; the mirror case — a Lesson promoting to a rule that does not cite it — reports `R-008` against the Lesson.

### AC: skill-pair-is-checked-both-ways (verifies REQ:skill-pair)

**Given** a detailed rule whose Instructions name `skill:go-hygiene`
**When** the skill does not list `rule:<slug>` under `## Rules`
**Then** an `R-010` violation names the repair; and a skill listing a rule that does not name it back reports `R-010` against the skill.

### AC: delete-refuses-live-links (verifies REQ:delete-refuses-live-links)

**Given** a rule a Lesson promotes to
**When** the user runs `specscore rule delete <slug>`
**Then** the command exits `4` naming `lesson:<slug>` as the blocker and the artifact remains on disk.

### AC: supersede-with-repoints-and-deletes (verifies REQ:delete-refuses-live-links)

**When** the user runs `specscore rule delete <slug> --supersede-with <successor>`
**Then** the Lesson's `**Promotes To:**` is repointed at the successor, the successor's Sources gain that Lesson (and its document mirror is updated), the old row and directory are gone, and `specscore rule lint` exits `0`.

### AC: list-does-not-open-detail-documents (verifies REQ:list-reads-only-the-index)

**When** the user runs `specscore rule list` in a project with detailed rules
**Then** the listing is produced from the index alone and the spec tree is byte-identical afterwards.

### AC: an-unparseable-row-survives-every-verb (verifies REQ:unparseable-rows-are-never-dropped)

**Given** an index carrying a hand-written row whose Statement has an unescaped `|`
**When** the user runs `rule new`, `rule expand`, `rule update`, `rule delete` or `rule promote` against a *different* rule
**Then** each exits `1` naming the broken row and `specscore rule lint --fix`, the index is byte-identical, and the row is still there.

### AC: fix-repairs-only-the-unambiguous-row (verifies REQ:unparseable-rows-are-never-dropped)

**When** the user runs `specscore rule lint --fix` on that index
**Then** the row is repaired by escaping the surplus `|`, the statement round-trips to its original text, and lint is clean; a row whose parse failure is NOT attributable to the Statement is instead preserved verbatim and still reported.

### AC: repo-scope-does-not-bind-a-bare-name (verifies REQ:applies-to-resolves-scope)

**Given** a rule scoped `repo:specscore/docs`
**When** the user runs `specscore rule list --applies-to docs/x.md` or `--applies-to otherorg/docs/x.md`
**Then** the rule is NOT listed; `--applies-to projects/specscore/docs/x.md` does list it.

### AC: unreadable-scope-is-listed-not-hidden (verifies REQ:unreadable-scope-fails-toward-binding)

**Given** a rule whose Scope cell does not parse
**When** the user runs `specscore rule list --applies-to <any path>`
**Then** the rule appears in the output flagged `scope_error`, stderr names the bad scope text, and the command exits `1`.

### AC: inline-superseded-is-refused (verifies REQ:superseded-requires-a-successor)

**When** the user runs `specscore rule update <inline-slug> --status Superseded`
**Then** the command exits `4` naming `specscore rule expand`; a tree that already holds such a row reports `R-009`.

### AC: unresolvable-source-is-refused (verifies REQ:source-references)

**When** the user runs `specscore rule update <slug> --add-source lesson:ghost`
**Then** the command exits `2`, nothing is written, and the tree stays lint-clean.

### AC: structured-output-is-portable (verifies REQ:structured-output-is-portable)

**When** the user runs `specscore rule show <slug> --format json`
**Then** `index_path` and `detail_path` are repo-relative, no field carries the em-dash sentinel, and an empty scalar is `""` while an empty list is `[]`.

### AC: every-verb-accepts-format-json (verifies the shared-flags behavior)

**When** each of `new`, `expand`, `list`, `show`, `update`, `delete`, `promote`, `lint` is invoked with `--format json`
**Then** the command emits a JSON document on stdout and exits with its documented code.

## Open Questions

- The strict pair makes a Lesson the source of at most one rule. That keeps "which rule did this Lesson become?" unambiguous, but a single Lesson that genuinely produced two independent rules currently has to pick one and cite the other from `**Why:**`. Should `**Promotes To:**` become a list once real usage shows that case?
- A rule has no lifecycle-transition verb (`rule change-status`) — `update --status` writes the field directly, without the legal-transition matrix `lesson change-status` enforces. Should rule statuses gain a transition matrix, or is a three-value vocabulary too small to be worth one?
- Supersession lives only in a detail document, so an inline rule must be expanded before it can be superseded — `rule update --status Superseded` on an inline rule is now a refusal pointing at `rule expand`. That is defensible (retiring a rule is worth a paragraph) but it is still two steps. Should the row carry supersession too?
- `--fix` repairs an unescaped `|` only when it falls in the Statement, because the columns before it have closed grammars that make the attribution provable. A pipe in `Control` produces the same line shape and is refused. Is a `--fix=R-003 --assume-statement` escape hatch worth having, or does that reintroduce the guessing this rule exists to prevent?
- `--applies-to` infers `product:` and `repo:` matches from path segments, which is a heuristic. Should `rule list` gain explicit `--in-repo` / `--in-product` flags so a caller that knows its context does not have to encode it in a path?

---
*This document follows the https://specscore.md/feature-specification*
