---
format: https://specscore.md/plan-specification
status: Draft
---

# Plan: Feature 1 — plan new --parent

**Status:** Draft
**Source Feature:** cli/plan/new
**Date:** 2026-06-07
**Owner:** alexander.trakhimenok
**Supersedes:** —
**Parent:** specscore:cross-repo-plan-composition

## Summary

Bootstrap sub-plan for the cross-repo-plan-composition master plan: implement the `--parent <plan-ref>` flag on `specscore plan new` (emit a verbatim `**Parent:**` body-metadata line, same-repo slug or cross-repo `<repo-slug>:<slug>`). This is the `cli/plan/new` half of Feature 1; the `P-005` lint half is its sibling sub-plan (`feature-1-plan-rules-p005`). It is the root of the bootstrap — the master's Task 1 — so it carries `**Parent:** specscore:cross-repo-plan-composition` (a cross-repo soft reference, ignored by today's lint and validated by `P-005` once that ships).

## Approach

The `plan new` verb already exists and its six original ACs are implemented and shipped. This plan adds only the new `--parent` behavior: parse the flag, reject an empty value (exit 2), and emit `**Parent:** <value>` verbatim into the header block after `**Supersedes:**` when present (and nothing when absent). The six pre-existing ACs are listed under Deferred AC Coverage as already-implemented — this plan does not re-verify them.

## Tasks

### Task 1: Add --parent flag and verbatim Parent emission

**Status:** complete
**Depends-On:** —
**Verifies:** cli/plan/new#ac:parent-recorded-verbatim

Add the optional `--parent <plan-ref>` flag to the `plan new` cobra command. On a non-empty value, write a `**Parent:** <value>` line into the scaffolded plan's header block (after `**Supersedes:**`), recording the value verbatim with no resolution or sibling-repo scanning. On an empty value, exit `2`. When the flag is absent, emit no `**Parent:**` line. Add Go tests asserting verbatim emission for a same-repo slug and a cross-repo `<repo-slug>:<slug>` value, absence when omitted, and exit 2 on empty.

## Deferred AC Coverage

- cli/plan/new#ac:scaffolded-plan-is-lint-clean — already implemented and shipped in the approved `plan new` verb; unchanged by the `--parent` addition.
- cli/plan/new#ac:scaffold-emits-frontmatter — already implemented; `--parent` adds a body line only, no frontmatter change.
- cli/plan/new#ac:source-required — already implemented; `--parent` is orthogonal to the `--feature`/`--idea` source binding.
- cli/plan/new#ac:existing-file-conflict — already implemented; no-clobber behavior is unchanged.
- cli/plan/new#ac:invalid-slug-rejected — already implemented; slug validation is unchanged.
- cli/plan/new#ac:ancestor-indexes-materialized — already implemented; ancestor-index materialization is unchanged.
- cli/plan/new#ac:source-without-acceptance-criteria-is-lint-clean — covered by the issue-166 generator-through-production-lint regression; the gallery's placeholder AC guidance is comment-only.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
