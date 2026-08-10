package lesson

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func coverageOccurrence() Occurrence {
	return Occurrence{
		SchemaVersion: 1,
		ID:            "01234567-89ab-4def-8123-456789abcdef",
		OccurredAt:    time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC),
		Summary:       "A bounded safe summary.",
		Context:       map[string]any{},
		Evidence:      Evidence{Kind: "none"},
	}
}

func TestCoverageValidationAndMutationEdges(t *testing.T) {
	for _, value := range []string{
		"authorization: value", "ghp_abcdefghijklmnopqrst", "AKIA0123456789ABCDEF",
		"-----BEGIN PRIVATE KEY-----", "person@example.test", "+353 123 4567",
		"/Users/example/private", "raw transcript: secret", "safe\x00value",
	} {
		if err := ValidateSafeContent("field", value); err == nil {
			t.Fatalf("unsafe content %q accepted", value)
		}
	}
	if err := ValidateSafeContent("field", "ordinary bounded text"); err != nil {
		t.Fatal(err)
	}

	for _, value := range []string{"", "line\nnext", "token=secret", strings.Repeat("x", 501)} {
		if err := validateBounded("field", value); err == nil {
			t.Fatalf("invalid bounded value %q accepted", value)
		}
	}
	if err := validateBounded("field", "safe"); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"", "/absolute", "C:/drive", "a//b", "a/./b", "a/../b", "a\\b"} {
		if err := validateRepoRelativePath(path); err == nil {
			t.Fatalf("invalid repo path %q accepted", path)
		}
	}
	if err := validateRepoRelativePath("spec/lessons/rule/README.md"); err != nil {
		t.Fatal(err)
	}

	for _, context := range []map[string]any{
		{"prompt": "no"}, {"unknown": "no"}, {"git": "not-an-object"},
		{"repository": "not/a"}, {"git": map[string]any{"commit": "BAD"}},
		{"worktree": map[string]any{"path_hint": "../outside"}},
		{"execution": map[string]any{"kind": "daemon"}},
		{"execution": map[string]any{"id": 7}},
	} {
		if err := validateContext(context); err == nil {
			t.Fatalf("invalid context %#v accepted", context)
		}
	}
	if err := validateContext(map[string]any{
		"repository": "github.com/specscore/specscore-cli",
		"git":        map[string]any{"commit": "abcdef1", "branch": "main"},
		"worktree":   map[string]any{"path_hint": "pkg/lesson", "id": "coverage"},
		"execution":  map[string]any{"kind": "ci", "id": "run-1"},
	}); err != nil {
		t.Fatal(err)
	}

	ref := "spec/lessons/rule/README.md"
	badURL := "ftp://example.test/evidence"
	for _, evidence := range []Evidence{
		{Kind: "invalid"}, {Kind: "none", Ref: &ref}, {Kind: "path"},
		{Kind: "path", Ref: stringPtr("../outside")}, {Kind: "url", Ref: &badURL},
	} {
		if err := validateEvidence(evidence); err == nil {
			t.Fatalf("invalid evidence %#v accepted", evidence)
		}
	}
	if err := validateEvidence(Evidence{Kind: "path", Ref: &ref}); err != nil {
		t.Fatal(err)
	}

	base := coverageOccurrence()
	cases := []Occurrence{
		func() Occurrence { o := base; o.SchemaVersion = 2; return o }(),
		func() Occurrence { o := base; o.ID = "not-a-uuid"; return o }(),
		func() Occurrence { o := base; o.OccurredAt = time.Time{}; return o }(),
		func() Occurrence { o := base; o.Context = nil; return o }(),
		func() Occurrence { o := base; o.Evidence = Evidence{Kind: "url"}; return o }(),
		func() Occurrence { o := base; o.Redactions = []string{"same", "same"}; return o }(),
	}
	for _, o := range cases {
		if err := ValidateOccurrence(o); err == nil {
			t.Fatalf("invalid occurrence accepted: %#v", o)
		}
	}
	if err := ValidateOccurrence(base); err != nil {
		t.Fatal(err)
	}

	cause := errors.New("cause")
	if mutationFailure(MutationPrePublication, nil) != nil {
		t.Fatal("nil mutation failure was not nil")
	}
	wrapped := mutationFailure(MutationCompensated, cause)
	if !errors.Is(wrapped, cause) || MutationOutcomeOf(wrapped) != MutationCompensated {
		t.Fatalf("wrapped mutation=%v outcome=%v", wrapped, MutationOutcomeOf(wrapped))
	}
	if MutationOutcomeOf(nil) != MutationPrePublication || MutationOutcomeOf(cause) != MutationUncertain {
		t.Fatal("mutation outcome fallback is not conservative")
	}
	if got := CompensatePublication(func() error { return nil }, cause); MutationOutcomeOf(got) != MutationCompensated {
		t.Fatalf("compensated outcome=%v", MutationOutcomeOf(got))
	}
	if got := CompensatePublication(func() error { return errors.New("sync failed") }, cause); MutationOutcomeOf(got) != MutationUncertain {
		t.Fatalf("uncertain outcome=%v", MutationOutcomeOf(got))
	}
}

func TestCoverageLayoutEdges(t *testing.T) {
	if got := RequiredSectionsFor(nil); len(got) != len(RequiredSections) {
		t.Fatalf("legacy sections=%v", got)
	}
	canonical := &Lesson{Canonical: true, SectionLines: map[string]int{"Lesson": 1}}
	if missing := canonical.MissingRequiredSectionsForLayout(); len(missing) != 4 {
		t.Fatalf("canonical missing=%v", missing)
	}
	if missing := (&Lesson{SectionLines: map[string]int{}}).MissingRequiredSectionsForLayout(); len(missing) != len(RequiredSections) {
		t.Fatalf("legacy missing=%v", missing)
	}
}

func TestCoverageFlatMigrationHelperEdges(t *testing.T) {
	for _, opts := range []FlatMigrationOptions{
		{}, {Slug: "Rule", Classifications: []string{"process"}},
		{Slug: "rule", Classifications: []string{"process", "process"}},
	} {
		if err := validateFlatMigrationOptions(opts); err == nil {
			t.Fatalf("invalid migration options accepted: %#v", opts)
		}
	}
	if err := validateFlatMigrationOptions(FlatMigrationOptions{Slug: "rule", Classifications: []string{"process", "validation"}}); err != nil {
		t.Fatal(err)
	}

	redacted := []string{}
	if got := safeFlatSection("Check", "", "fallback", &redacted); got != "fallback" || len(redacted) != 1 {
		t.Fatalf("empty section=%q redactions=%v", got, redacted)
	}
	if got := safeFlatSection("Check", "person@example.test", "fallback", &redacted); got != "fallback" || len(redacted) != 2 {
		t.Fatalf("unsafe section=%q redactions=%v", got, redacted)
	}
	if got := safeFlatSection("Check", "safe body", "fallback", &redacted); got != "safe body" {
		t.Fatalf("safe section=%q", got)
	}
	if got := replaceCanonicalSection("no heading", "Lesson", "body"); got != "no heading" {
		t.Fatalf("missing heading replaced: %q", got)
	}
	if got := replaceCanonicalSection("## Lesson\nonly", "Lesson", "body"); got != "## Lesson\nonly" {
		t.Fatalf("terminal heading replaced: %q", got)
	}
	if got := replaceCanonicalSection("## Lesson\nold\n## Next\nnext", "Lesson", "new"); !strings.Contains(got, "## Lesson\n\nnew\n\n## Next") {
		t.Fatalf("section replacement=%q", got)
	}

	sections := splitFlatSections([]byte("## Recurrences\n- 2026-08-01 — first\n- 2026-08-02 — second\n---\nignored\n"))
	observations := flatRecurrenceObservations([]byte("## Recurrences\n- 2026-08-01 — first\n- 2026-08-02 — second\n---\nignored\n"), sections["Recurrences"])
	if len(observations) != 2 || !strings.Contains(observations[0].text, "first") || flatRecurrenceObservations([]byte("x"), flatSection{}) != nil {
		t.Fatalf("observations=%#v", observations)
	}

	root := t.TempDir()
	stage := filepath.Join(root, "stage")
	if err := os.MkdirAll(filepath.Join(stage, "occurrences"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := collectFlatExpectedFiles(stage, root, "rule", filepath.Join(root, "manifest.json")); err == nil {
		t.Fatal("missing staged README accepted")
	}
	for _, file := range []string{filepath.Join(stage, "README.md"), filepath.Join(stage, "manifest.json")} {
		if err := os.WriteFile(file, []byte("safe"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(stage, "occurrences", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := collectFlatExpectedFiles(stage, root, "rule", filepath.Join(root, "manifest.json")); err == nil {
		t.Fatal("staged occurrence directory accepted")
	}
	if err := os.Remove(filepath.Join(stage, "occurrences", "nested")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "occurrences", "one.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := collectFlatExpectedFiles(stage, root, "rule", filepath.Join(root, "manifest.json"))
	if err != nil || len(files) != 3 {
		t.Fatalf("files=%#v err=%v", files, err)
	}
	if err := ensureFreshMigrationTargetsAbsent(root, files); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "rule"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "rule", "README.md"), []byte("collision"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureFreshMigrationTargetsAbsent(root, files); err == nil {
		t.Fatal("target collision accepted")
	}
	if err := verifyExpectedFiles(root, []flatExpectedFile{{Path: "../outside", SHA256: strings.Repeat("0", 64)}}, false); err == nil {
		t.Fatal("invalid expected metadata accepted")
	}
	if err := verifyExpectedFiles(root, []flatExpectedFile{{Path: "missing", SHA256: strings.Repeat("0", 64)}}, true); err != nil {
		t.Fatal(err)
	}
	if err := verifyExpectedFiles(root, []flatExpectedFile{{Path: "missing", SHA256: strings.Repeat("0", 64)}}, false); err == nil {
		t.Fatal("missing expected file accepted")
	}
	if err := verifyExpectedFiles(root, []flatExpectedFile{{Path: "rule/README.md", SHA256: strings.Repeat("0", 64)}}, false); err == nil {
		t.Fatal("mismatched expected file accepted")
	}

	if err := validateCompletedFlatMigration(filepath.Join(root, "missing", "README.md")); err == nil {
		t.Fatal("missing canonical lesson accepted")
	}
	if _, err := marshalSafeJSON("unsafe", map[string]string{"person": "person@example.test"}); err == nil {
		t.Fatal("unsafe json was accepted")
	}
	var marker flatMigrationMarker
	if err := decodeStrictJSON([]byte("{"), &marker); err == nil {
		t.Fatal("malformed json accepted")
	}
	if err := decodeStrictJSON([]byte(`{"schema_version":1} {}`), &marker); err == nil {
		t.Fatal("trailing json accepted")
	}
	valid, err := json.Marshal(flatMigrationMarker{SchemaVersion: 1})
	if err != nil || decodeStrictJSON(append(valid, '\n'), &marker) != nil {
		t.Fatalf("valid strict json err=%v", err)
	}
	if got := FlatMigrationEventUUID(strings.Repeat("a", 64), "rule"); got != FlatMigrationEventUUID(strings.Repeat("a", 64), "rule") || bytes.Equal([]byte(got), nil) {
		t.Fatal("deterministic event uuid failed")
	}
}

func coverageLegacySource() LegacySourceRef {
	return LegacySourceRef{
		Repository:  "github.com/example/repository",
		Path:        "history/LESSONS-LEARNED.md",
		Revision:    strings.Repeat("a", 40),
		CommittedAt: "2026-08-10T12:00:00Z",
		SHA256:      strings.Repeat("b", 64),
		ByteCount:   1,
	}
}

func TestCoverageLegacyHelpersAndInventoryEdges(t *testing.T) {
	for _, ref := range []LegacySourceRef{
		{}, func() LegacySourceRef { r := coverageLegacySource(); r.Repository = "two/parts"; return r }(),
		func() LegacySourceRef { r := coverageLegacySource(); r.Path = "../outside"; return r }(),
		func() LegacySourceRef { r := coverageLegacySource(); r.Revision = "bad"; return r }(),
		func() LegacySourceRef {
			r := coverageLegacySource()
			r.CommittedAt = "2026-08-10T12:00:00+01:00"
			return r
		}(),
		func() LegacySourceRef { r := coverageLegacySource(); r.SHA256 = "bad"; return r }(),
	} {
		if err := validateLegacySourceRef(ref); err == nil {
			t.Fatalf("invalid legacy ref accepted: %#v", ref)
		}
	}
	if err := validateLegacySourceRef(coverageLegacySource()); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveLegacySourceIdentity(filepath.Join(t.TempDir(), "not-git.md"), []byte("x")); err == nil {
		t.Fatal("non-Git source identity accepted")
	}

	for _, value := range []string{"", strings.Repeat("x", 2001), "<!-- TODO -->", "TODO: refine", "# heading", "person@example.test"} {
		if err := validateReviewedCompactText("Lesson", value); err == nil {
			t.Fatalf("invalid reviewed compact text %q accepted", value)
		}
	}
	if err := validateReviewedCompactText("Lesson", "A reviewed compact statement."); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	source := filepath.Join(root, "legacy.md")
	text := "# Outside\n\n## Lessons\n\n## L1 — First\n**Status:** Recorded\n**Recurred:** twice\nnotes\n\n### L1: \n**Occurrence:** loose marker\n\n## L2 malformed\n"
	if err := os.WriteFile(source, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	oldIdentity := legacySourceIdentityFn
	legacySourceIdentityFn = func(string, []byte) (LegacySourceRef, error) {
		return LegacySourceRef{}, errors.New("identity unavailable")
	}
	t.Cleanup(func() { legacySourceIdentityFn = oldIdentity })
	inv, err := InventoryLegacy(source)
	if err != nil {
		t.Fatal(err)
	}
	if inv.LessonCount != 2 || inv.RecurrenceMarkerCount != 2 || len(inv.Collisions) != 1 || len(inv.UnmatchedCandidates) != 1 || len(inv.Warnings) != 1 {
		t.Fatalf("inventory=%#v", inv)
	}
	if inv.EntryProjectionSHA256 == "" || legacyEntryProjectionSHA256(inv.Entries) != inv.EntryProjectionSHA256 {
		t.Fatal("inventory projection is not stable")
	}
	if got := legacySlug("L1", "Title — punctuation!"); got != "l1-title-punctuation" {
		t.Fatalf("legacy slug=%q", got)
	}

	entry := LegacyEntry{Key: "L1#1", Warnings: []string{"aggregate-count-ambiguous"}, StartByte: 5}
	validSource := coverageLegacySource()
	opts := legacyOccurrenceOptions("", legacyOccurrenceID(validSource.SHA256, entry.Key, "rule"), LegacyInventory{Source: validSource}, entry)
	if opts.Evidence.Ref == nil || len(opts.Redactions) != 2 || opts.Now.Location() != time.UTC {
		t.Fatalf("legacy occurrence options=%#v", opts)
	}
	o := coverageOccurrence()
	o.ID = opts.ID
	o.OccurredAt = opts.Now
	o.Summary = opts.Summary
	o.Context = opts.Context
	o.Evidence = opts.Evidence
	o.Redactions = opts.Redactions
	if err := validateLegacyOccurrence(o, LegacyInventory{Source: validSource}, entry, "rule"); err != nil {
		t.Fatal(err)
	}
	o.Summary = "different"
	if err := validateLegacyOccurrence(o, LegacyInventory{Source: validSource}, entry, "rule"); err == nil {
		t.Fatal("mismatched legacy occurrence accepted")
	}
	if !containsString([]string{"a", "b"}, "b") || containsString([]string{"a"}, "b") {
		t.Fatal("containsString incorrect")
	}

	stage := filepath.Join(root, "stage")
	if err := os.MkdirAll(stage, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(stage, "durable")
	if err := writeDurableStageFile(file, []byte("safe")); err != nil {
		t.Fatal(err)
	}
	if err := writeDurableStageFile(file, []byte("again")); err == nil {
		t.Fatal("exclusive stage write overwrote existing file")
	}
	if !pathIsDirectory(stage) || pathIsDirectory(file) || pathIsDirectory(filepath.Join(root, "missing")) {
		t.Fatal("pathIsDirectory incorrect")
	}
	provenance := legacyProvenance(LegacyInventory{Source: validSource}, entry)
	if !strings.Contains(provenance, "github.com/example/repository@") || hasLegacyProvenance(file, provenance) {
		t.Fatal("legacy provenance helper incorrect")
	}
	if err := os.WriteFile(file, []byte("**Legacy Provenance:** "+provenance), 0o644); err != nil {
		t.Fatal(err)
	}
	if !hasLegacyProvenance(file, provenance) || validateImportedLesson(file, entry, LegacyInventory{Source: validSource}) == nil {
		t.Fatal("imported lesson validation did not require occurrence store")
	}

	manifest, err := legacyManifestBytes(LegacyInventory{Source: validSource, Entries: []LegacyEntry{entry}, LessonCount: 1, EntryProjectionSHA256: "projection"}, LegacyMapping{Source: validSource})
	if err != nil || !bytes.Contains(manifest, []byte(`"schema_version": 1`)) {
		t.Fatalf("manifest=%s err=%v", manifest, err)
	}
}
