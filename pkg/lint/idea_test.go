package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validIdeaBody returns a full, lint-clean idea markdown body.
// title and status are parameterized.
func validIdeaBody(title, status string, extraFields map[string]string) string {
	fields := map[string]string{
		"Status":        status,
		"Date":          "2026-04-10",
		"Owner":         "alice",
		"Promotes To":   "—",
		"Supersedes":    "—",
		"Related Ideas": "—",
	}
	for k, v := range extraFields {
		fields[k] = v
	}
	order := []string{"Status"}
	if _, ok := extraFields["Archived"]; ok {
		order = append(order, "Archived")
	}
	order = append(order, "Date", "Owner", "Promotes To", "Supersedes", "Related Ideas")
	if _, ok := extraFields["Archive Note"]; ok {
		order = append(order, "Archive Note")
	}
	var header strings.Builder
	header.WriteString("# Idea: " + title + "\n\n")
	for _, k := range order {
		header.WriteString("**" + k + ":** " + fields[k] + "\n")
	}
	header.WriteString("\n")
	header.WriteString(`## Problem Statement
How Might We ship faster?

## Context
Triggering observation.

## Recommended Direction
Do it.

## Alternatives Considered
Nope.

## MVP Scope
Small.

## Not Doing (and Why)
- Thing — reason

## Key Assumptions to Validate
| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | Users want this | Survey |

## SpecScore Integration
- **New Features this would create:** TBD

## Open Questions
None at this time.
`)
	return header.String()
}

// validProposalBody returns a full, lint-clean change-request (proposal) body.
// The title uses "# Proposal:" prefix and includes Type/Targets fields.
func validProposalBody(title, status, targets string, extraFields map[string]string) string {
	fields := map[string]string{
		"Status":        status,
		"Type":          "change-request",
		"Targets":       targets,
		"Date":          "2026-04-10",
		"Owner":         "alice",
		"Promotes To":   "—",
		"Supersedes":    "—",
		"Related Ideas": "—",
	}
	for k, v := range extraFields {
		fields[k] = v
	}
	order := []string{"Status", "Type", "Targets"}
	if _, ok := fields["Phase"]; ok {
		order = append(order, "Phase")
	}
	if _, ok := extraFields["Archived"]; ok {
		order = append(order, "Archived")
	}
	order = append(order, "Date", "Owner", "Promotes To", "Supersedes", "Related Ideas")
	if _, ok := extraFields["Archive Note"]; ok {
		order = append(order, "Archive Note")
	}
	var header strings.Builder
	header.WriteString("# Proposal: " + title + "\n\n")
	for _, k := range order {
		header.WriteString("**" + k + ":** " + fields[k] + "\n")
	}
	header.WriteString("\n")
	header.WriteString(`## Problem Statement
How Might We ship faster?

## Context
Triggering observation.

## Recommended Direction
Do it.

## Alternatives Considered
Nope.

## MVP Scope
Small.

## Not Doing (and Why)
- Thing — reason

## Key Assumptions to Validate
| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | Users want this | Survey |

## SpecScore Integration
- **New Features this would create:** TBD

## Open Questions
None at this time.
`)
	return header.String()
}

// writeSpec creates a fake spec repo structure under dir and returns specRoot.
func writeSpec(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	specRoot := filepath.Join(dir, "spec")
	for rel, content := range files {
		path := filepath.Join(specRoot, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return specRoot
}

// hasRule returns whether vs contains a violation with the given rule name.
func hasRule(vs []Violation, rule string) bool {
	for _, v := range vs {
		if v.Rule == rule {
			return true
		}
	}
	return false
}

const activeIndex = `# SpecScore Ideas

## Index

| Idea | Status | Date | Owner | Promotes To |
|------|--------|------|-------|-------------|

## Open Questions

None at this time.
`

const archivedIndex = `# Archived Ideas

## Open Questions

None at this time.
`

func TestCheckIdeas_CleanIdea(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndexWith("offline-mode"),
		"ideas/archived/README.md": archivedIndex,
		"ideas/offline-mode.md":    validIdeaBody("Offline Mode", "Approved", nil),
	})
	vs, err := CheckIdeas(specRoot, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) > 0 {
		t.Fatalf("expected 0 violations, got %d: %+v", len(vs), vs)
	}
}

func activeIndexWith(slugs ...string) string {
	var b strings.Builder
	b.WriteString("# SpecScore Ideas\n\n## Index\n\n| Idea | Status | Date | Owner | Promotes To |\n|------|--------|------|-------|-------------|\n")
	for _, s := range slugs {
		b.WriteString("| [" + s + "](" + s + ".md) | Approved | 2026-04-10 | alice | — |\n")
	}
	b.WriteString("\n## Open Questions\n\nNone at this time.\n")
	return b.String()
}

// archivedIndexWith builds an archived-ideas index listing the given slugs in
// the chronological `- <date> — [slug](slug.md) — <note>` format the
// idea-archived-index-chronological rule expects. Dates default to the
// validIdeaBody date (2026-04-10) and the note column to "—".
func archivedIndexWith(slugs ...string) string {
	var b strings.Builder
	b.WriteString("# Archived Ideas\n\n")
	for _, s := range slugs {
		b.WriteString("- 2026-04-10 — [" + s + "](" + s + ".md) — —\n")
	}
	b.WriteString("\n## Open Questions\n\nNone at this time.\n")
	return b.String()
}

func TestCheckIdeas_InvalidSlug(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndex,
		"ideas/archived/README.md": archivedIndex,
		"ideas/Bad_Slug.md":        validIdeaBody("Bad Slug", "Approved", nil),
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-slug-format") {
		t.Errorf("expected idea-slug-format violation, got: %+v", vs)
	}
}

func TestCheckIdeas_IdeaDirectoryRejected(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":              activeIndex,
		"ideas/archived/README.md":     archivedIndex,
		"ideas/offline-mode/README.md": "# Idea: Offline Mode\n",
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-single-file") {
		t.Errorf("expected idea-single-file violation")
	}
}

func TestCheckIdeas_MissingTitle(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndexWith("bad"),
		"ideas/archived/README.md": archivedIndex,
		"ideas/bad.md":             "**Status:** Draft\n",
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-title-format") {
		t.Errorf("expected idea-title-format violation: %+v", vs)
	}
}

func TestCheckIdeas_WrongTitlePrefix(t *testing.T) {
	body := "# Feature: Something\n\n**Status:** Draft\n**Date:** 2026-04-10\n**Owner:** alice\n**Promotes To:** —\n**Supersedes:** —\n**Related Ideas:** —\n"
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndexWith("bad"),
		"ideas/archived/README.md": archivedIndex,
		"ideas/bad.md":             body,
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-title-format") {
		t.Errorf("expected idea-title-format violation")
	}
}

func TestCheckIdeas_MissingHeaderField(t *testing.T) {
	body := strings.Replace(validIdeaBody("X", "Draft", nil), "**Owner:** alice\n", "", 1)
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndexWith("x"),
		"ideas/archived/README.md": archivedIndex,
		"ideas/x.md":               body,
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-header-fields") {
		t.Errorf("expected idea-header-fields violation")
	}
}

func TestCheckIdeas_InvalidStatus(t *testing.T) {
	body := strings.Replace(validIdeaBody("X", "Draft", nil), "**Status:** Draft", "**Status:** WIP", 1)
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndexWith("x"),
		"ideas/archived/README.md": archivedIndex,
		"ideas/x.md":               body,
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-status-values") {
		t.Errorf("expected idea-status-values violation")
	}
}

func TestCheckIdeas_SpecifiedWithoutPromotion(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndexWith("x"),
		"ideas/archived/README.md": archivedIndex,
		"ideas/x.md":               validIdeaBody("X", "Specified", nil),
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-specified-requires-promotion") {
		t.Errorf("expected idea-specified-requires-promotion violation")
	}
}

// An idea carrying **Archived:** true but living OUTSIDE archived/ fires
// idea-archived-location: the flag and the location must agree.
func TestCheckIdeas_ArchivedFlagOutsideArchivedDir(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndexWith("x"),
		"ideas/archived/README.md": archivedIndex,
		"ideas/x.md":               validIdeaBody("X", "Stale", map[string]string{"Archived": "true"}),
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-archived-location") {
		t.Errorf("expected idea-archived-location violation")
	}
}

// A file UNDER archived/ that does NOT carry **Archived:** true fires
// idea-archived-location (the inverse-direction mismatch).
func TestCheckIdeas_ArchivedDirMissingFlag(t *testing.T) {
	body := validIdeaBody("X", "Stale", nil) // terminal status, but no flag
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndex,
		"ideas/archived/README.md": archivedIndex,
		"ideas/archived/x.md":      body,
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-archived-location") {
		t.Errorf("expected idea-archived-location violation for archived/ file without **Archived:** true: %+v", vs)
	}
}

// The **Archive Note:** is optional: an archived idea (terminal status +
// flag, under archived/) with no note is lint-clean.
func TestCheckIdeas_ArchiveNoteOptional(t *testing.T) {
	body := validIdeaBody("X", "Stale", map[string]string{"Archived": "true"})
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndex,
		"ideas/archived/README.md": archivedIndexWith("x"),
		"ideas/archived/x.md":      body,
	})
	vs, _ := CheckIdeas(specRoot, false)
	if hasRule(vs, "idea-archive-note") {
		t.Errorf("archive note is optional; no violation expected: %+v", vs)
	}
}

// A present-but-empty **Archive Note:** fires idea-archive-note (format check).
func TestCheckIdeas_ArchiveNoteEmptyFires(t *testing.T) {
	body := validIdeaBody("X", "Stale", map[string]string{"Archived": "true", "Archive Note": "—"})
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndex,
		"ideas/archived/README.md": archivedIndexWith("x"),
		"ideas/archived/x.md":      body,
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-archive-note") {
		t.Errorf("expected idea-archive-note violation for empty note: %+v", vs)
	}
}

// An archived idea carrying a NON-terminal status fires idea-archived-location.
func TestCheckIdeas_ArchivedNonTerminalStatus(t *testing.T) {
	body := validIdeaBody("X", "Approved", map[string]string{"Archived": "true"})
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndex,
		"ideas/archived/README.md": archivedIndexWith("x"),
		"ideas/archived/x.md":      body,
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-archived-location") {
		t.Errorf("expected idea-archived-location violation for non-terminal archived status: %+v", vs)
	}
}

// A supersede target that is NOT archived (active, not **Archived:** true)
// fires idea-supersedes-target-archived.
func TestCheckIdeas_SupersedesNonArchived(t *testing.T) {
	x := validIdeaBody("X", "Approved", nil)
	y := strings.Replace(validIdeaBody("Y", "Approved", nil), "**Supersedes:** —", "**Supersedes:** x", 1)
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndexWith("x", "y"),
		"ideas/archived/README.md": archivedIndex,
		"ideas/x.md":               x,
		"ideas/y.md":               y,
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-supersedes-target-archived") {
		t.Errorf("expected idea-supersedes-target-archived violation")
	}
}

func TestCheckIdeas_InvalidRelatedIdeasSyntax(t *testing.T) {
	body := strings.Replace(validIdeaBody("X", "Approved", nil), "**Related Ideas:** —", "**Related Ideas:** bogus_rel:other", 1)
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndexWith("x"),
		"ideas/archived/README.md": archivedIndex,
		"ideas/x.md":               body,
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-related-ideas-format") {
		t.Errorf("expected idea-related-ideas-format violation")
	}
}

func TestCheckIdeas_BrokenRelatedSlug(t *testing.T) {
	body := strings.Replace(validIdeaBody("X", "Approved", nil), "**Related Ideas:** —", "**Related Ideas:** depends_on:ghost", 1)
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndexWith("x"),
		"ideas/archived/README.md": archivedIndex,
		"ideas/x.md":               body,
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-related-ideas-target-exists") {
		t.Errorf("expected idea-related-ideas-target-exists violation")
	}
}

func TestCheckIdeas_EmptyNotDoing(t *testing.T) {
	body := validIdeaBody("X", "Approved", nil)
	body = strings.Replace(body, "- Thing — reason\n", "", 1)
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndexWith("x"),
		"ideas/archived/README.md": archivedIndex,
		"ideas/x.md":               body,
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-not-doing-non-empty") {
		t.Errorf("expected idea-not-doing-non-empty violation")
	}
}

func TestCheckIdeas_MissingMustBeTrue(t *testing.T) {
	body := validIdeaBody("X", "Approved", nil)
	body = strings.Replace(body, "| Must-be-true | Users want this | Survey |\n", "", 1)
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndexWith("x"),
		"ideas/archived/README.md": archivedIndex,
		"ideas/x.md":               body,
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-must-be-true-present") {
		t.Errorf("expected idea-must-be-true-present violation")
	}
}

func TestCheckIdeas_MissingHMWFraming(t *testing.T) {
	body := validIdeaBody("X", "Approved", nil)
	body = strings.Replace(body, "How Might We ship faster?", "Some other framing.", 1)
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndexWith("x"),
		"ideas/archived/README.md": archivedIndex,
		"ideas/x.md":               body,
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-hmw-framing") {
		t.Errorf("expected idea-hmw-framing warning")
	}
}

func TestCheckIdeas_MissingRequiredSection(t *testing.T) {
	body := validIdeaBody("X", "Approved", nil)
	body = strings.Replace(body, "## MVP Scope\nSmall.\n\n", "", 1)
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndexWith("x"),
		"ideas/archived/README.md": archivedIndex,
		"ideas/x.md":               body,
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-required-sections") {
		t.Errorf("expected idea-required-sections violation")
	}
}

func TestCheckIdeas_SyncDriftDetectedAndFixed(t *testing.T) {
	feature := `# Feature: Offline Sync

**Status:** Draft
**Source Ideas:** offline-mode

## Summary
Stuff.

## Open Questions

None at this time.
`
	body := validIdeaBody("Offline Mode", "Approved", nil)
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":                 activeIndexWith("offline-mode"),
		"ideas/archived/README.md":        archivedIndex,
		"ideas/offline-mode.md":           body,
		"features/offline-sync/README.md": feature,
	})

	// Without --fix: drift reported.
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-sync-lint-strict") {
		t.Fatalf("expected idea-sync-lint-strict: %+v", vs)
	}

	// With --fix: drift repaired.
	vs2, _ := CheckIdeas(specRoot, true)
	if hasRule(vs2, "idea-sync-lint-strict") {
		t.Errorf("fix did not repair drift: %+v", vs2)
	}

	// Verify idea file was rewritten.
	data, err := os.ReadFile(filepath.Join(specRoot, "ideas", "offline-mode.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	// Referencing Feature is Draft, so derived status is Specifying.
	if !strings.Contains(s, "**Status:** Specifying") {
		t.Errorf("status not updated: %s", s)
	}
	if !strings.Contains(s, "**Promotes To:** offline-sync") {
		t.Errorf("promotes-to not updated: %s", s)
	}

	// Subsequent lint passes.
	vs3, _ := CheckIdeas(specRoot, false)
	if hasRule(vs3, "idea-sync-lint-strict") {
		t.Errorf("subsequent lint still reports drift: %+v", vs3)
	}
}

// TestCheckIdeas_SyncFixUpdatesFrontmatterMirror guards against issue #68:
// when the idea-sync auto-advance rewrites the body **Status:**, it must also
// update the leading frontmatter `status:` mirror so a single `lint --fix`
// pass converges instead of introducing a status-mirror drift only `migrate`
// can repair.
func TestCheckIdeas_SyncFixUpdatesFrontmatterMirror(t *testing.T) {
	feature := `# Feature: Offline Sync

**Status:** Draft
**Source Ideas:** offline-mode

## Summary
Stuff.

## Open Questions

None at this time.
`
	// Idea carries a frontmatter `status:` mirror in sync with the body.
	body := "---\nstatus: Approved\n---\n" + validIdeaBody("Offline Mode", "Approved", nil)
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":                 activeIndexWith("offline-mode"),
		"ideas/archived/README.md":        archivedIndex,
		"ideas/offline-mode.md":           body,
		"features/offline-sync/README.md": feature,
	})

	// With --fix: body advances Approved -> Specifying.
	if _, err := CheckIdeas(specRoot, true); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(specRoot, "ideas", "offline-mode.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "**Status:** Specifying") {
		t.Fatalf("body status not advanced: %s", s)
	}
	// The frontmatter mirror MUST advance in lockstep, else the next lint
	// reports a status-mirror drift that --fix itself introduced.
	if frontmatterStatus(data) != "Specifying" {
		t.Errorf("frontmatter status mirror stale: got %q, want %q\n%s",
			frontmatterStatus(data), "Specifying", s)
	}
}

func TestCheckIdeas_FeatureReferencesDraftIdea(t *testing.T) {
	feature := `# Feature: Offline Sync

**Status:** Draft
**Source Ideas:** offline-mode

## Open Questions

None at this time.
`
	body := validIdeaBody("Offline Mode", "Draft", nil)
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":                 activeIndexWith("offline-mode"),
		"ideas/archived/README.md":        archivedIndex,
		"ideas/offline-mode.md":           body,
		"features/offline-sync/README.md": feature,
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-feature-cross-reference") {
		t.Errorf("expected idea-feature-cross-reference violation")
	}
}

// A cross-repo Source Idea (URL form / markdown link to another repo) must NOT
// fire idea-feature-cross-reference: it resolves via the linkage system, not the
// local spec tree.
func TestCheckIdeas_FeatureReferencesCrossRepoIdea(t *testing.T) {
	feature := `# Feature: pricing-ladder

**Status:** Draft
**Source Ideas:** [sourcer](https://github.com/sneat-co/backstage/blob/main/spec/ideas/sourcer.md)

## Open Questions

None at this time.
`
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":                   activeIndex, // empty — no local ideas
		"ideas/archived/README.md":          archivedIndex,
		"features/pricing-ladder/README.md": feature,
	})
	vs, _ := CheckIdeas(specRoot, false)
	if hasRule(vs, "idea-feature-cross-reference") {
		t.Errorf("cross-repo Source Idea must not fire idea-feature-cross-reference; got %+v", vs)
	}
}

// A bare unresolved slug (not a URL) still fires — cross-repo tolerance is
// URL-form only, so a typo'd local slug is still caught.
func TestCheckIdeas_FeatureReferencesUnknownLocalSlug(t *testing.T) {
	feature := `# Feature: pricing-ladder

**Status:** Draft
**Source Ideas:** no-such-local-idea

## Open Questions

None at this time.
`
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":                   activeIndex,
		"ideas/archived/README.md":          archivedIndex,
		"features/pricing-ladder/README.md": feature,
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-feature-cross-reference") {
		t.Errorf("an unknown local slug must still fire idea-feature-cross-reference")
	}
}

func TestCheckIdeas_UnlistedIdeaInIndex(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndex, // empty index
		"ideas/archived/README.md": archivedIndex,
		"ideas/offline-mode.md":    validIdeaBody("Offline Mode", "Approved", nil),
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-index-completeness") {
		t.Errorf("expected idea-index-completeness violation: %+v", vs)
	}

	// With --fix, index is regenerated.
	vs2, _ := CheckIdeas(specRoot, true)
	if hasRule(vs2, "idea-index-completeness") {
		t.Errorf("--fix did not regenerate index: %+v", vs2)
	}
	data, _ := os.ReadFile(filepath.Join(specRoot, "ideas", "README.md"))
	if !strings.Contains(string(data), "offline-mode") {
		t.Errorf("index not updated: %s", string(data))
	}
}

// TestCheckIdeas_IndexRowDriftDetectedAndFixed exercises the
// idea-index-row-sync rule. The Idea file declares Status="Approved",
// but the active-index row says "Draft". Without --fix, the linter
// reports drift; with --fix, the row is rewritten from the parsed Idea
// and the violation clears.
func TestCheckIdeas_IndexRowDriftDetectedAndFixed(t *testing.T) {
	staleIndex := `# SpecScore Ideas

## Index

| Idea | Status | Date | Owner | Promotes To |
|------|--------|------|-------|-------------|
| [offline-mode](offline-mode.md) | Draft | 2026-04-10 | alice | — |

## Open Questions

None at this time.
`
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          staleIndex,
		"ideas/archived/README.md": archivedIndex,
		"ideas/offline-mode.md":    validIdeaBody("Offline Mode", "Approved", nil),
	})

	// Without --fix: row-sync violation, no completeness violation.
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-index-row-sync") {
		t.Errorf("expected idea-index-row-sync violation: %+v", vs)
	}
	if hasRule(vs, "idea-index-completeness") {
		t.Errorf("idea-index-completeness should not fire when slug is present: %+v", vs)
	}

	// With --fix: row is rewritten and the violation is gone.
	vs2, _ := CheckIdeas(specRoot, true)
	if hasRule(vs2, "idea-index-row-sync") {
		t.Errorf("--fix did not resync row: %+v", vs2)
	}
	data, _ := os.ReadFile(filepath.Join(specRoot, "ideas", "README.md"))
	body := string(data)
	if !strings.Contains(body, "| [offline-mode](offline-mode.md) | Approved |") {
		t.Errorf("index row not rewritten with Approved status: %s", body)
	}
	if strings.Contains(body, "| Draft |") {
		t.Errorf("stale Draft cell still present: %s", body)
	}
}

func TestCheckIdeas_ArchivedIndexOutOfOrderAndFixed(t *testing.T) {
	older := validIdeaBody("Older", "Stale", map[string]string{"Archived": "true", "Archive Note": "pivoted", "Date": "2024-11-02"})
	newer := validIdeaBody("Newer", "Stale", map[string]string{"Archived": "true", "Archive Note": "pivoted", "Date": "2025-03-10"})
	// Date inside body: need to set Date via extraFields; validIdeaBody uses fixed default.
	// To override we rewrite the Date line.
	older = strings.Replace(older, "**Date:** 2026-04-10", "**Date:** 2024-11-02", 1)
	newer = strings.Replace(newer, "**Date:** 2026-04-10", "**Date:** 2025-03-10", 1)

	// Index with wrong order (newer first).
	badArchIndex := `# Archived Ideas

- 2025-03-10 — [newer](newer.md) — pivoted
- 2024-11-02 — [older](older.md) — pivoted

## Open Questions

None at this time.
`
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndex,
		"ideas/archived/README.md": badArchIndex,
		"ideas/archived/older.md":  older,
		"ideas/archived/newer.md":  newer,
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-archived-index-chronological") {
		t.Errorf("expected idea-archived-index-chronological: %+v", vs)
	}

	vs2, _ := CheckIdeas(specRoot, true)
	if hasRule(vs2, "idea-archived-index-chronological") {
		t.Errorf("--fix did not reorder: %+v", vs2)
	}
	data, _ := os.ReadFile(filepath.Join(specRoot, "ideas", "archived", "README.md"))
	body := string(data)
	iOlder := strings.Index(body, "older.md")
	iNewer := strings.Index(body, "newer.md")
	if iOlder == -1 || iNewer == -1 || iOlder > iNewer {
		t.Errorf("entries not reordered: %s", body)
	}
}

func TestCheckIdeas_IdeaWithIdField(t *testing.T) {
	body := validIdeaBody("X", "Draft", nil)
	body = strings.Replace(body, "**Status:** Draft\n", "**Status:** Draft\n**Id:** x\n", 1)
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndexWith("x"),
		"ideas/archived/README.md": archivedIndex,
		"ideas/x.md":               body,
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-id-is-slug") {
		t.Errorf("expected idea-id-is-slug violation")
	}
}

// featureBody returns a minimal Feature README with the given status and
// optional Source Ideas value (use "" to omit the field).
func featureBody(title, status, sourceIdeas string) string {
	var b strings.Builder
	b.WriteString("# Feature: " + title + "\n\n")
	b.WriteString("**Status:** " + status + "\n")
	if sourceIdeas != "" {
		b.WriteString("**Source Ideas:** " + sourceIdeas + "\n")
	}
	b.WriteString("\n## Summary\n\nStuff.\n\n## Open Questions\n\nNone at this time.\n")
	return b.String()
}

// TestCheckIdeas_DerivesSpecifying_WhenFeatureAtDraft verifies that
// an Idea referenced by a Draft Feature derives `Specifying`.
func TestCheckIdeas_DerivesSpecifying_WhenFeatureAtDraft(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":                 activeIndexWith("offline-mode"),
		"ideas/archived/README.md":        archivedIndex,
		"ideas/offline-mode.md":           validIdeaBody("Offline Mode", "Approved", nil),
		"features/offline-sync/README.md": featureBody("Offline Sync", "Draft", "offline-mode"),
	})
	// --fix should set the idea to Specifying.
	if _, err := CheckIdeas(specRoot, true); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(specRoot, "ideas", "offline-mode.md"))
	if !strings.Contains(string(data), "**Status:** Specifying") {
		t.Errorf("expected Specifying, got: %s", string(data))
	}
	// Subsequent lint passes.
	vs, _ := CheckIdeas(specRoot, false)
	if hasRule(vs, "idea-sync-lint-strict") {
		t.Errorf("subsequent lint reports drift: %+v", vs)
	}
}

// TestCheckIdeas_DerivesImplemented_WhenAllFeaturesStable verifies that an
// Idea referenced only by Stable Features derives `Implemented`.
func TestCheckIdeas_DerivesImplemented_WhenAllFeaturesStable(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":                 activeIndexWith("offline-mode"),
		"ideas/archived/README.md":        archivedIndex,
		"ideas/offline-mode.md":           validIdeaBody("Offline Mode", "Approved", nil),
		"features/offline-sync/README.md": featureBody("Offline Sync", "Stable", "offline-mode"),
	})
	if _, err := CheckIdeas(specRoot, true); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(specRoot, "ideas", "offline-mode.md"))
	if !strings.Contains(string(data), "**Status:** Implemented") {
		t.Errorf("expected Implemented, got: %s", string(data))
	}
	vs, _ := CheckIdeas(specRoot, false)
	if hasRule(vs, "idea-sync-lint-strict") {
		t.Errorf("subsequent lint reports drift: %+v", vs)
	}
}

// TestCheckIdeas_DerivesSpecifying_WhenMixed verifies an Idea referenced
// by one Stable AND one Draft Feature derives `Specifying` (any Draft/Under
// Review pulls it to Specifying).
func TestCheckIdeas_DerivesSpecifying_WhenMixed(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":           activeIndexWith("offline-mode"),
		"ideas/archived/README.md":  archivedIndex,
		"ideas/offline-mode.md":     validIdeaBody("Offline Mode", "Approved", nil),
		"features/feat-a/README.md": featureBody("Feat A", "Stable", "offline-mode"),
		"features/feat-b/README.md": featureBody("Feat B", "Draft", "offline-mode"),
	})
	if _, err := CheckIdeas(specRoot, true); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(specRoot, "ideas", "offline-mode.md"))
	if !strings.Contains(string(data), "**Status:** Specifying") {
		t.Errorf("expected Specifying, got: %s", string(data))
	}
}

// TestCheckIdeas_DeprecatedFeatureGivesImplemented verifies that a
// `Deprecated` Feature (a post-Stable, "done" state) derives `Implemented`,
// the same as `Stable` — never dragging the Idea backward.
func TestCheckIdeas_DeprecatedFeatureGivesImplemented(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":                 activeIndexWith("offline-mode"),
		"ideas/archived/README.md":        archivedIndex,
		"ideas/offline-mode.md":           validIdeaBody("Offline Mode", "Approved", nil),
		"features/offline-sync/README.md": featureBody("Offline Sync", "Deprecated", "offline-mode"),
	})
	if _, err := CheckIdeas(specRoot, true); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(specRoot, "ideas", "offline-mode.md"))
	if !strings.Contains(string(data), "**Status:** Implemented") {
		t.Errorf("expected Implemented (Deprecated is a done state, like Stable), got: %s", string(data))
	}
}

// TestCheckIdeas_MixedStableDeprecated_DerivesImplemented verifies that an
// Idea referenced by both a `Stable` and a `Deprecated` Feature derives
// `Implemented` — not `Specified` (the backward-transition bug).
func TestCheckIdeas_MixedStableDeprecated_DerivesImplemented(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":                  activeIndexWith("offline-mode"),
		"ideas/archived/README.md":         archivedIndex,
		"ideas/offline-mode.md":            validIdeaBody("Offline Mode", "Implementing", map[string]string{"Promotes To": "offline-sync, offline-cache"}),
		"features/offline-sync/README.md":  featureBody("Offline Sync", "Stable", "offline-mode"),
		"features/offline-cache/README.md": featureBody("Offline Cache", "Deprecated", "offline-mode"),
	})
	if _, err := CheckIdeas(specRoot, true); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(specRoot, "ideas", "offline-mode.md"))
	if !strings.Contains(string(data), "**Status:** Implemented") {
		t.Errorf("expected Implemented (must not go backward to Specified), got: %s", string(data))
	}
}

// TestCheckIdeas_DerivedRevertsToApproved_WhenRefsRemoved verifies
// that removing all Feature references reverts a derived-status Idea to
// Approved on --fix.
func TestCheckIdeas_DerivedRevertsToApproved_WhenRefsRemoved(t *testing.T) {
	for _, derivedStatus := range []string{"Specifying", "Specified", "Implementing", "Implemented"} {
		t.Run(derivedStatus, func(t *testing.T) {
			body := validIdeaBody("Offline Mode", derivedStatus, nil)
			body = strings.Replace(body, "**Promotes To:** —", "**Promotes To:** offline-sync", 1)
			specRoot := writeSpec(t, map[string]string{
				"ideas/README.md":          activeIndexWith("offline-mode"),
				"ideas/archived/README.md": archivedIndex,
				"ideas/offline-mode.md":    body,
				// no feature referencing it
			})
			// Without --fix: drift reported.
			vs, _ := CheckIdeas(specRoot, false)
			if !hasRule(vs, "idea-sync-lint-strict") {
				t.Fatalf("expected drift: %+v", vs)
			}
			// With --fix: revert to Approved with empty Promotes To.
			if _, err := CheckIdeas(specRoot, true); err != nil {
				t.Fatal(err)
			}
			data, _ := os.ReadFile(filepath.Join(specRoot, "ideas", "offline-mode.md"))
			s := string(data)
			if !strings.Contains(s, "**Status:** Approved") {
				t.Errorf("expected Approved after revert, got: %s", s)
			}
			if !strings.Contains(s, "**Promotes To:** —") {
				t.Errorf("expected empty Promotes To after revert, got: %s", s)
			}
		})
	}
}

// TestCheckIdeas_ImplementingRequiresPromotion verifies that a manual
// `Status: Implementing` with empty Promotes To is rejected.
func TestCheckIdeas_ImplementingRequiresPromotion(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndexWith("x"),
		"ideas/archived/README.md": archivedIndex,
		"ideas/x.md":               validIdeaBody("X", "Implementing", nil),
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-specified-requires-promotion") {
		t.Errorf("expected idea-specified-requires-promotion violation")
	}
}

// TestCheckIdeas_FeatureMayReferenceImplementingIdea verifies a Feature
// can reference an Idea whose status is Implementing.
func TestCheckIdeas_FeatureMayReferenceImplementingIdea(t *testing.T) {
	body := validIdeaBody("Offline Mode", "Implementing", nil)
	body = strings.Replace(body, "**Promotes To:** —", "**Promotes To:** feat-a, feat-b", 1)
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":           activeIndexWith("offline-mode"),
		"ideas/archived/README.md":  archivedIndex,
		"ideas/offline-mode.md":     body,
		"features/feat-a/README.md": featureBody("Feat A", "Draft", "offline-mode"),
		"features/feat-b/README.md": featureBody("Feat B", "Draft", "offline-mode"),
	})
	vs, _ := CheckIdeas(specRoot, false)
	if hasRule(vs, "idea-feature-cross-reference") {
		t.Errorf("Implementing should be an allowed cross-reference target: %+v", vs)
	}
}

// --- Idea type / change-request tests ---

// TestCheckIdeas_ValidFeatureRequest confirms that a standard idea (no Type
// field) still passes lint cleanly.
func TestCheckIdeas_ValidFeatureRequest(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndexWith("offline-mode"),
		"ideas/archived/README.md": archivedIndex,
		"ideas/offline-mode.md":    validIdeaBody("Offline Mode", "Approved", nil),
	})
	vs, err := CheckIdeas(specRoot, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range []string{
		"idea-type-values", "idea-type-title-consistency",
		"idea-targets-required", "idea-change-request-location",
	} {
		if hasRule(vs, rule) {
			t.Errorf("unexpected %s violation for vanilla feature-request: %+v", rule, vs)
		}
	}
}

// TestCheckIdeas_ValidChangeRequest confirms a properly-structured proposal
// at the correct location passes lint.
func TestCheckIdeas_ValidChangeRequest(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":                    activeIndex,
		"ideas/archived/README.md":           archivedIndex,
		"features/auth/README.md":            featureBody("Auth", "Approved", ""),
		"features/auth/proposals/add-mfa.md": validProposalBody("Add MFA", "Draft", "auth", nil),
	})
	vs, err := CheckIdeas(specRoot, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range []string{
		"idea-type-values", "idea-type-title-consistency",
		"idea-targets-required", "idea-targets-exists",
		"idea-change-request-location", "idea-title-format",
	} {
		if hasRule(vs, rule) {
			t.Errorf("unexpected %s violation for valid change-request: %+v", rule, vs)
		}
	}
}

// TestCheckIdeas_ProposalTitleWithoutChangeRequestType rejects a
// `# Proposal:` title when Type is not change-request.
func TestCheckIdeas_ProposalTitleWithoutChangeRequestType(t *testing.T) {
	// Build a body that uses # Proposal: but has Type: feature-request
	body := validProposalBody("Bad Proposal", "Draft", "auth", nil)
	body = strings.Replace(body, "**Type:** change-request", "**Type:** feature-request", 1)
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":                         activeIndex,
		"ideas/archived/README.md":                archivedIndex,
		"features/auth/README.md":                 featureBody("Auth", "Approved", ""),
		"features/auth/proposals/bad-proposal.md": body,
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-type-title-consistency") {
		t.Errorf("expected idea-type-title-consistency violation: %+v", vs)
	}
}

// TestCheckIdeas_IdeaTitleWithChangeRequestType rejects a `# Idea:` title
// when Type is change-request.
func TestCheckIdeas_IdeaTitleWithChangeRequestType(t *testing.T) {
	body := validIdeaBody("Bad Idea", "Draft", nil)
	// Insert Type: change-request and Targets: auth after Status line
	body = strings.Replace(body, "**Status:** Draft\n**Date:**",
		"**Status:** Draft\n**Type:** change-request\n**Targets:** auth\n**Date:**", 1)
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndexWith("bad-idea"),
		"ideas/archived/README.md": archivedIndex,
		"ideas/bad-idea.md":        body,
		"features/auth/README.md":  featureBody("Auth", "Approved", ""),
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-type-title-consistency") {
		t.Errorf("expected idea-type-title-consistency violation: %+v", vs)
	}
}

// TestCheckIdeas_ChangeRequestAtWrongLocation rejects a change-request
// idea that lives in spec/ideas/ instead of the feature proposals dir.
func TestCheckIdeas_ChangeRequestAtWrongLocation(t *testing.T) {
	body := validProposalBody("Wrong Location", "Draft", "auth", nil)
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndexWith("wrong-location"),
		"ideas/archived/README.md": archivedIndex,
		"ideas/wrong-location.md":  body,
		"features/auth/README.md":  featureBody("Auth", "Approved", ""),
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-change-request-location") {
		t.Errorf("expected idea-change-request-location violation: %+v", vs)
	}
}

// TestCheckIdeas_InvalidTypeValue rejects unknown Type values.
func TestCheckIdeas_InvalidTypeValue(t *testing.T) {
	body := validIdeaBody("Bad Type", "Draft", nil)
	body = strings.Replace(body, "**Status:** Draft\n**Date:**",
		"**Status:** Draft\n**Type:** unknown-type\n**Date:**", 1)
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndexWith("bad-type"),
		"ideas/archived/README.md": archivedIndex,
		"ideas/bad-type.md":        body,
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-type-values") {
		t.Errorf("expected idea-type-values violation: %+v", vs)
	}
}

// TestCheckIdeas_MissingTargetsForChangeRequest rejects a change-request
// with no Targets field.
func TestCheckIdeas_MissingTargetsForChangeRequest(t *testing.T) {
	body := validProposalBody("No Target", "Draft", "auth", nil)
	// Remove the Targets line entirely
	body = strings.Replace(body, "**Targets:** auth\n", "", 1)
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":                      activeIndex,
		"ideas/archived/README.md":             archivedIndex,
		"features/auth/README.md":              featureBody("Auth", "Approved", ""),
		"features/auth/proposals/no-target.md": body,
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-targets-required") {
		t.Errorf("expected idea-targets-required violation: %+v", vs)
	}
}

// TestCheckIdeas_TargetsNonexistentFeature rejects Targets referencing a
// feature directory that does not exist.
func TestCheckIdeas_TargetsNonexistentFeature(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":                    activeIndex,
		"ideas/archived/README.md":           archivedIndex,
		"features/ghost/proposals/fix-it.md": validProposalBody("Fix It", "Draft", "ghost", nil),
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-targets-exists") {
		t.Errorf("expected idea-targets-exists violation: %+v", vs)
	}
}

// TestCheckIdeas_PhaseFieldOptionalValid confirms Phase is accepted when
// present and non-empty.
func TestCheckIdeas_PhaseFieldOptionalValid(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndex,
		"ideas/archived/README.md": archivedIndex,
		"features/auth/README.md":  featureBody("Auth", "Approved", ""),
		"features/auth/proposals/add-mfa.md": validProposalBody(
			"Add MFA", "Draft", "auth", map[string]string{"Phase": "discovery"}),
	})
	vs, _ := CheckIdeas(specRoot, false)
	if hasRule(vs, "idea-phase-non-empty") {
		t.Errorf("unexpected idea-phase-non-empty for valid Phase: %+v", vs)
	}
}

// TestCheckIdeas_PhaseFieldEmpty rejects an empty Phase field.
func TestCheckIdeas_PhaseFieldEmpty(t *testing.T) {
	body := validProposalBody("Add MFA", "Draft", "auth", nil)
	// Insert Phase with empty value before Date
	body = strings.Replace(body, "**Date:**", "**Phase:** \n**Date:**", 1)
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":                    activeIndex,
		"ideas/archived/README.md":           archivedIndex,
		"features/auth/README.md":            featureBody("Auth", "Approved", ""),
		"features/auth/proposals/add-mfa.md": body,
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-phase-non-empty") {
		t.Errorf("expected idea-phase-non-empty violation: %+v", vs)
	}
}

// --- New lifecycle derivation and sync tests ---

// TestCheckIdeas_DerivesImplementing_WhenFeatureAtImplementing verifies that
// an Idea referenced by an Implementing Feature derives `Implementing`.
func TestCheckIdeas_DerivesImplementing_WhenFeatureAtImplementing(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":                 activeIndexWith("offline-mode"),
		"ideas/archived/README.md":        archivedIndex,
		"ideas/offline-mode.md":           validIdeaBody("Offline Mode", "Approved", nil),
		"features/offline-sync/README.md": featureBody("Offline Sync", "Implementing", "offline-mode"),
	})
	if _, err := CheckIdeas(specRoot, true); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(specRoot, "ideas", "offline-mode.md"))
	if !strings.Contains(string(data), "**Status:** Implementing") {
		t.Errorf("expected Implementing, got: %s", string(data))
	}
	vs, _ := CheckIdeas(specRoot, false)
	if hasRule(vs, "idea-sync-lint-strict") {
		t.Errorf("subsequent lint reports drift: %+v", vs)
	}
}

// TestCheckIdeas_DerivesSpecified_WhenAllFeaturesApproved verifies that
// an Idea referenced only by Approved Features derives `Specified`.
func TestCheckIdeas_DerivesSpecified_WhenAllFeaturesApproved(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":                 activeIndexWith("offline-mode"),
		"ideas/archived/README.md":        archivedIndex,
		"ideas/offline-mode.md":           validIdeaBody("Offline Mode", "Approved", nil),
		"features/offline-sync/README.md": featureBody("Offline Sync", "Approved", "offline-mode"),
	})
	if _, err := CheckIdeas(specRoot, true); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(specRoot, "ideas", "offline-mode.md"))
	if !strings.Contains(string(data), "**Status:** Specified") {
		t.Errorf("expected Specified, got: %s", string(data))
	}
	vs, _ := CheckIdeas(specRoot, false)
	if hasRule(vs, "idea-sync-lint-strict") {
		t.Errorf("subsequent lint reports drift: %+v", vs)
	}
}

// TestCheckIdeas_ChangeRequestSkipsDerivation verifies that change-request
// ideas are NOT subject to derivation — their status remains author-managed
// regardless of Feature references.
func TestCheckIdeas_ChangeRequestSkipsDerivation(t *testing.T) {
	// A change-request at Draft, referenced by a Feature. Without the
	// derivation bypass, the linter would try to derive Specifying.
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":                    activeIndex,
		"ideas/archived/README.md":           archivedIndex,
		"features/auth/README.md":            featureBody("Auth", "Draft", "add-mfa"),
		"features/auth/proposals/add-mfa.md": validProposalBody("Add MFA", "Draft", "auth", nil),
	})
	vs, _ := CheckIdeas(specRoot, false)
	if hasRule(vs, "idea-sync-lint-strict") {
		t.Errorf("change-request should not fire idea-sync-lint-strict: %+v", vs)
	}
}

// TestCheckIdeas_ChangeRequestNoPromotionRequired verifies that
// change-request ideas at derived statuses do NOT require Promotes To.
func TestCheckIdeas_ChangeRequestNoPromotionRequired(t *testing.T) {
	for _, status := range []string{"Specifying", "Specified", "Implementing", "Implemented"} {
		t.Run(status, func(t *testing.T) {
			specRoot := writeSpec(t, map[string]string{
				"ideas/README.md":                    activeIndex,
				"ideas/archived/README.md":           archivedIndex,
				"features/auth/README.md":            featureBody("Auth", "Approved", ""),
				"features/auth/proposals/add-mfa.md": validProposalBody("Add MFA", status, "auth", nil),
			})
			vs, _ := CheckIdeas(specRoot, false)
			if hasRule(vs, "idea-specified-requires-promotion") {
				t.Errorf("change-request at %s should not require Promotes To: %+v", status, vs)
			}
		})
	}
}

// TestCheckIdeas_FeatureCrossRefAcceptsSpecifying verifies that a Feature
// referencing an Idea at status Specifying does not fire idea-feature-cross-reference.
func TestCheckIdeas_FeatureCrossRefAcceptsSpecifying(t *testing.T) {
	body := validIdeaBody("Offline Mode", "Specifying", nil)
	body = strings.Replace(body, "**Promotes To:** —", "**Promotes To:** offline-sync", 1)
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":                 activeIndexWith("offline-mode"),
		"ideas/archived/README.md":        archivedIndex,
		"ideas/offline-mode.md":           body,
		"features/offline-sync/README.md": featureBody("Offline Sync", "Draft", "offline-mode"),
	})
	vs, _ := CheckIdeas(specRoot, false)
	if hasRule(vs, "idea-feature-cross-reference") {
		t.Errorf("Specifying should be accepted by feature-cross-reference: %+v", vs)
	}
}

// TestCheckIdeas_FeatureCrossRefAcceptsImplemented verifies that a Feature
// referencing an Idea at status Implemented does not fire idea-feature-cross-reference.
func TestCheckIdeas_FeatureCrossRefAcceptsImplemented(t *testing.T) {
	body := validIdeaBody("Offline Mode", "Implemented", nil)
	body = strings.Replace(body, "**Promotes To:** —", "**Promotes To:** offline-sync", 1)
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":                 activeIndexWith("offline-mode"),
		"ideas/archived/README.md":        archivedIndex,
		"ideas/offline-mode.md":           body,
		"features/offline-sync/README.md": featureBody("Offline Sync", "Stable", "offline-mode"),
	})
	vs, _ := CheckIdeas(specRoot, false)
	if hasRule(vs, "idea-feature-cross-reference") {
		t.Errorf("Implemented should be accepted by feature-cross-reference: %+v", vs)
	}
}

// TestCheckIdeas_SpecifyingRequiresPromotion verifies that a feature-request
// idea at Specifying without Promotes To fires the promotion rule.
func TestCheckIdeas_SpecifyingRequiresPromotion(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndexWith("x"),
		"ideas/archived/README.md": archivedIndex,
		"ideas/x.md":               validIdeaBody("X", "Specifying", nil),
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-specified-requires-promotion") {
		t.Errorf("expected idea-specified-requires-promotion for Specifying: %+v", vs)
	}
}

// TestCheckIdeas_ImplementedRequiresPromotion verifies that a feature-request
// idea at Implemented without Promotes To fires the promotion rule.
func TestCheckIdeas_ImplementedRequiresPromotion(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndexWith("x"),
		"ideas/archived/README.md": archivedIndex,
		"ideas/x.md":               validIdeaBody("X", "Implemented", nil),
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-specified-requires-promotion") {
		t.Errorf("expected idea-specified-requires-promotion for Implemented: %+v", vs)
	}
}

// --- Archival and cross-reference extension tests (Task 3) ---

// TestCheckIdeas_ArchivedChangeRequestStaysInPlace verifies that an archived
// change-request idea at its feature-scoped path passes lint (no
// idea-archived-location violation).
func TestCheckIdeas_ArchivedChangeRequestStaysInPlace(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndex,
		"ideas/archived/README.md": archivedIndex,
		"features/auth/README.md":  featureBody("Auth", "Approved", ""),
		"features/auth/proposals/add-mfa.md": validProposalBody(
			"Add MFA", "Stale", "auth",
			map[string]string{"Archived": "true", "Archive Note": "superseded by newer proposal"}),
	})
	vs, err := CheckIdeas(specRoot, false)
	if err != nil {
		t.Fatal(err)
	}
	if hasRule(vs, "idea-archived-location") {
		t.Errorf("archived change-request at feature-scoped path should NOT fire idea-archived-location: %+v", vs)
	}
}

// TestCheckIdeas_ArchivedFeatureRequestStillRequiresArchivedDir verifies
// that a feature-request idea flagged **Archived:** true but NOT in
// spec/ideas/archived/ still fires idea-archived-location.
func TestCheckIdeas_ArchivedFeatureRequestStillRequiresArchivedDir(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndexWith("x"),
		"ideas/archived/README.md": archivedIndex,
		"ideas/x.md":               validIdeaBody("X", "Stale", map[string]string{"Archived": "true", "Archive Note": "abandoned"}),
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-archived-location") {
		t.Errorf("archived feature-request outside archived/ MUST fire idea-archived-location: %+v", vs)
	}
}

// TestCheckIdeas_RelatedIdeasResolvesToProposal verifies that a Related
// Ideas entry referencing a change-request proposal under
// spec/features/*/proposals/ resolves successfully.
func TestCheckIdeas_RelatedIdeasResolvesToProposal(t *testing.T) {
	ideaBody := strings.Replace(
		validIdeaBody("Main Idea", "Approved", nil),
		"**Related Ideas:** —",
		"**Related Ideas:** extends:add-mfa",
		1)
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":                    activeIndexWith("main-idea"),
		"ideas/archived/README.md":           archivedIndex,
		"ideas/main-idea.md":                 ideaBody,
		"features/auth/README.md":            featureBody("Auth", "Approved", ""),
		"features/auth/proposals/add-mfa.md": validProposalBody("Add MFA", "Draft", "auth", nil),
	})
	vs, err := CheckIdeas(specRoot, false)
	if err != nil {
		t.Fatal(err)
	}
	if hasRule(vs, "idea-related-ideas-target-exists") {
		t.Errorf("related-ideas referencing a proposal should resolve: %+v", vs)
	}
}

// TestCheckIdeas_IndexCompletenessIncludesProposals verifies that the
// active index completeness check requires change-request proposals
// to be listed.
func TestCheckIdeas_IndexCompletenessIncludesProposals(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":                    activeIndex, // empty index — missing the proposal
		"ideas/archived/README.md":           archivedIndex,
		"features/auth/README.md":            featureBody("Auth", "Approved", ""),
		"features/auth/proposals/add-mfa.md": validProposalBody("Add MFA", "Draft", "auth", nil),
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-index-completeness") {
		t.Errorf("expected idea-index-completeness violation for unlisted proposal: %+v", vs)
	}
}

// TestCheckIdeas_ProposalTitleWithoutTypeField rejects `# Proposal:` title
// when Type field is absent (effective type = feature-request).
func TestCheckIdeas_ProposalTitleWithoutTypeField(t *testing.T) {
	// Build a body that uses "# Proposal:" prefix but has NO Type field at all.
	body := validIdeaBody("Bad Proposal", "Draft", nil)
	body = strings.Replace(body, "# Idea: Bad Proposal", "# Proposal: Bad Proposal", 1)
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndexWith("bad-proposal"),
		"ideas/archived/README.md": archivedIndex,
		"ideas/bad-proposal.md":    body,
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-type-title-consistency") {
		t.Errorf("expected idea-type-title-consistency for Proposal title without Type field: %+v", vs)
	}
}

// TestCheckIdeas_HeaderFieldOrderingTypeTargetsPhase verifies the ordering
// constraints for Type, Targets, and Phase fields relative to Status and Date.
func TestCheckIdeas_HeaderFieldOrderingTypeTargetsPhase(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		rule    string
		wantMsg string
	}{
		{
			name: "Type before Status",
			body: func() string {
				// Type must appear after Status, but we place it before.
				return "# Proposal: Bad Order\n\n" +
					"**Type:** change-request\n" +
					"**Status:** Draft\n" +
					"**Targets:** auth\n" +
					"**Date:** 2026-04-10\n" +
					"**Owner:** alice\n" +
					"**Promotes To:** —\n" +
					"**Supersedes:** —\n" +
					"**Related Ideas:** —\n\n" +
					sectionBody()
			}(),
			rule:    "idea-header-fields",
			wantMsg: "**Type:** must appear after **Status:**",
		},
		{
			name: "Type after Date",
			body: func() string {
				return "# Proposal: Bad Order\n\n" +
					"**Status:** Draft\n" +
					"**Date:** 2026-04-10\n" +
					"**Type:** change-request\n" +
					"**Targets:** auth\n" +
					"**Owner:** alice\n" +
					"**Promotes To:** —\n" +
					"**Supersedes:** —\n" +
					"**Related Ideas:** —\n\n" +
					sectionBody()
			}(),
			rule:    "idea-header-fields",
			wantMsg: "**Type:** must appear before **Date:**",
		},
		{
			name: "Targets before Type",
			body: func() string {
				return "# Proposal: Bad Order\n\n" +
					"**Status:** Draft\n" +
					"**Targets:** auth\n" +
					"**Type:** change-request\n" +
					"**Date:** 2026-04-10\n" +
					"**Owner:** alice\n" +
					"**Promotes To:** —\n" +
					"**Supersedes:** —\n" +
					"**Related Ideas:** —\n\n" +
					sectionBody()
			}(),
			rule:    "idea-header-fields",
			wantMsg: "**Targets:** must appear after **Type:**",
		},
		{
			name: "Targets after Date",
			body: func() string {
				return "# Proposal: Bad Order\n\n" +
					"**Status:** Draft\n" +
					"**Type:** change-request\n" +
					"**Date:** 2026-04-10\n" +
					"**Targets:** auth\n" +
					"**Owner:** alice\n" +
					"**Promotes To:** —\n" +
					"**Supersedes:** —\n" +
					"**Related Ideas:** —\n\n" +
					sectionBody()
			}(),
			rule:    "idea-header-fields",
			wantMsg: "**Targets:** must appear before **Date:**",
		},
		{
			name: "Phase before Targets",
			body: func() string {
				return "# Proposal: Bad Order\n\n" +
					"**Status:** Draft\n" +
					"**Type:** change-request\n" +
					"**Phase:** discovery\n" +
					"**Targets:** auth\n" +
					"**Date:** 2026-04-10\n" +
					"**Owner:** alice\n" +
					"**Promotes To:** —\n" +
					"**Supersedes:** —\n" +
					"**Related Ideas:** —\n\n" +
					sectionBody()
			}(),
			rule:    "idea-header-fields",
			wantMsg: "**Phase:** must appear after **Targets:**",
		},
		{
			name: "Phase after Date",
			body: func() string {
				return "# Proposal: Bad Order\n\n" +
					"**Status:** Draft\n" +
					"**Type:** change-request\n" +
					"**Targets:** auth\n" +
					"**Date:** 2026-04-10\n" +
					"**Phase:** discovery\n" +
					"**Owner:** alice\n" +
					"**Promotes To:** —\n" +
					"**Supersedes:** —\n" +
					"**Related Ideas:** —\n\n" +
					sectionBody()
			}(),
			rule:    "idea-header-fields",
			wantMsg: "**Phase:** must appear before **Date:**",
		},
		{
			name: "Phase before Type (no Targets)",
			body: func() string {
				// Phase without Targets, but must still be after Type.
				return "# Idea: Bad Order\n\n" +
					"**Status:** Draft\n" +
					"**Phase:** discovery\n" +
					"**Type:** feature-request\n" +
					"**Date:** 2026-04-10\n" +
					"**Owner:** alice\n" +
					"**Promotes To:** —\n" +
					"**Supersedes:** —\n" +
					"**Related Ideas:** —\n\n" +
					sectionBody()
			}(),
			rule:    "idea-header-fields",
			wantMsg: "**Phase:** must appear after **Type:**",
		},
		{
			name: "Targets before Status (no Type)",
			body: func() string {
				// Targets without Type field: must be after Status.
				return "# Idea: Bad Order\n\n" +
					"**Targets:** auth\n" +
					"**Status:** Draft\n" +
					"**Date:** 2026-04-10\n" +
					"**Owner:** alice\n" +
					"**Promotes To:** —\n" +
					"**Supersedes:** —\n" +
					"**Related Ideas:** —\n\n" +
					sectionBody()
			}(),
			rule:    "idea-header-fields",
			wantMsg: "**Targets:** must appear after **Status:**",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			specRoot := writeSpec(t, map[string]string{
				"ideas/README.md":                      activeIndex,
				"ideas/archived/README.md":             archivedIndex,
				"features/auth/README.md":              featureBody("Auth", "Approved", ""),
				"features/auth/proposals/bad-order.md": tc.body,
			})
			vs, _ := CheckIdeas(specRoot, false)
			found := false
			for _, v := range vs {
				if v.Rule == tc.rule && strings.Contains(v.Message, tc.wantMsg) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected %s with message containing %q, got: %+v", tc.rule, tc.wantMsg, vs)
			}
		})
	}
}

// sectionBody returns the required sections body for building test ideas.
func sectionBody() string {
	return `## Problem Statement
How Might We ship faster?

## Context
Triggering observation.

## Recommended Direction
Do it.

## Alternatives Considered
Nope.

## MVP Scope
Small.

## Not Doing (and Why)
- Thing — reason

## Key Assumptions to Validate
| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | Users want this | Survey |

## SpecScore Integration
- **New Features this would create:** TBD

## Open Questions
None at this time.
`
}

// TestCheckIdeas_TargetsOnFeatureRequestProhibited verifies that a feature-request
// idea with a non-empty Targets field fires idea-targets-required.
func TestCheckIdeas_TargetsOnFeatureRequestProhibited(t *testing.T) {
	body := validIdeaBody("Bad Targets", "Draft", nil)
	body = strings.Replace(body, "**Status:** Draft\n**Date:**",
		"**Status:** Draft\n**Targets:** auth\n**Date:**", 1)
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndexWith("bad-targets"),
		"ideas/archived/README.md": archivedIndex,
		"ideas/bad-targets.md":     body,
		"features/auth/README.md":  featureBody("Auth", "Approved", ""),
	})
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-targets-required") {
		t.Errorf("expected idea-targets-required for feature-request with Targets: %+v", vs)
	}
}

// TestCheckIdeas_ProposalIndexRowSync verifies that proposal-style links
// in the active index (../features/*/proposals/<slug>.md) are correctly
// parsed by readIndexRows and drift is detected + fixed.
func TestCheckIdeas_ProposalIndexRowSync(t *testing.T) {
	// Index with a proposal link whose status is stale.
	staleIndex := `# SpecScore Ideas

## Index

| Idea | Status | Date | Owner | Promotes To |
|------|--------|------|-------|-------------|
| [add-mfa](../features/auth/proposals/add-mfa.md) | Approved | 2026-04-10 | alice | — |

## Open Questions

None at this time.
`
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":                    staleIndex,
		"ideas/archived/README.md":           archivedIndex,
		"features/auth/README.md":            featureBody("Auth", "Approved", ""),
		"features/auth/proposals/add-mfa.md": validProposalBody("Add MFA", "Draft", "auth", nil),
	})

	// Without --fix: drift detected.
	vs, _ := CheckIdeas(specRoot, false)
	if !hasRule(vs, "idea-index-row-sync") {
		t.Errorf("expected idea-index-row-sync for stale proposal row: %+v", vs)
	}

	// With --fix: row is rewritten.
	vs2, _ := CheckIdeas(specRoot, true)
	if hasRule(vs2, "idea-index-row-sync") {
		t.Errorf("--fix did not resync proposal row: %+v", vs2)
	}
	data, _ := os.ReadFile(filepath.Join(specRoot, "ideas", "README.md"))
	body := string(data)
	if !strings.Contains(body, "| Draft |") {
		t.Errorf("index row should show Draft after fix: %s", body)
	}
}

// TestCheckIdeas_IndexFixIncludesProposalLink verifies that --fix
// generates proposal-style links for change-request ideas in the index.
func TestCheckIdeas_IndexFixIncludesProposalLink(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":                    activeIndex, // empty index
		"ideas/archived/README.md":           archivedIndex,
		"features/auth/README.md":            featureBody("Auth", "Approved", ""),
		"features/auth/proposals/add-mfa.md": validProposalBody("Add MFA", "Draft", "auth", nil),
	})

	// --fix should add the proposal to the index with the correct link format.
	vs, _ := CheckIdeas(specRoot, true)
	if hasRule(vs, "idea-index-completeness") {
		t.Errorf("--fix should have resolved index-completeness: %+v", vs)
	}
	data, _ := os.ReadFile(filepath.Join(specRoot, "ideas", "README.md"))
	body := string(data)
	if !strings.Contains(body, "../features/auth/proposals/add-mfa.md") {
		t.Errorf("index should contain proposal-style link: %s", body)
	}
}

// TestCheckIdeas_BothTypesPassFullValidation runs both a standard
// feature-request and a change-request through full idea validation to
// confirm existing rules pass cleanly for both types (regression check).
func TestCheckIdeas_BothTypesPassFullValidation(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/README.md":          activeIndexWith("offline-mode"),
		"ideas/archived/README.md": archivedIndex,
		"ideas/offline-mode.md":    validIdeaBody("Offline Mode", "Approved", nil),
		"features/auth/README.md":  featureBody("Auth", "Approved", ""),
		"features/auth/proposals/add-mfa.md": validProposalBody(
			"Add MFA", "Draft", "auth", nil),
	})
	// --fix first to reconcile index.
	if _, err := CheckIdeas(specRoot, true); err != nil {
		t.Fatal(err)
	}
	vs, err := CheckIdeas(specRoot, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vs {
		if v.Severity == "error" {
			t.Errorf("unexpected error: %+v", v)
		}
	}
}
