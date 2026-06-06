---
type: sidekick-seed
slug: consolidate-duplicated-parsers-and-helpers-in-specscore-cli
captured_at: 2026-06-06T09:10:14Z
captured_by: user
captured_during: null
trigger: explicit
status: queued
synchestra_task: null
---
# Consolidate duplicated parsers and helpers in specscore-cli for code reuse

The decision-parser frontmatter bug — one of two near-identical decision parsers was fixed for leading-frontmatter tolerance, the other (in decision_immutability.go) was missed and surfaced only when the specscore meta-spec was migrated — shows the cost of duplicated logic: a fix to one call site silently leaves the others broken.

Candidates to consolidate behind a single source of truth per concern:

- Per-type title scans: feature / idea / decision / plan each re-implement "find the `# Title`, skipping any leading frontmatter block".
- Frontmatter helpers: frontmatterEndIndex / bodyAfterFrontmatter / parseLeadingFrontmatter overlap in responsibility.
- The many near-identical `walk*` functions in adherence_footer.go.
- Spec-root resolution (resolveConfigSpecRoot was extracted mid-task; check for remaining inline copies).

Goal: one parser/helper per concern so a single fix covers all call sites and this class of "fixed one, missed the duplicate" bug can't recur.
