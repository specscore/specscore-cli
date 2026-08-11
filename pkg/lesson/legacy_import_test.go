package lesson

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func inventoryLegacyForApply(t *testing.T, source string) LegacyInventory {
	t.Helper()
	deps := defaultLegacyImportDeps()
	deps.sourceIdentity = func(path string, b []byte) (LegacySourceRef, error) {
		return LegacySourceRef{
			Repository:  "github.com/example/process",
			Path:        filepath.Base(path),
			Revision:    strings.Repeat("a", 40),
			CommittedAt: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
			SHA256:      shaString(b),
			ByteCount:   len(b),
		}, nil
	}
	inv, err := inventoryLegacyWithDeps(source, deps)
	if err != nil {
		t.Fatal(err)
	}
	return inv
}

type legacyImportTestFS struct {
	lessonFS
	link   func(string, string) error
	open   func(string) (lessonFile, error)
	remove func(string) error
}

func (fs legacyImportTestFS) Link(oldname, newname string) error {
	if fs.link != nil {
		return fs.link(oldname, newname)
	}
	return fs.lessonFS.Link(oldname, newname)
}

func (fs legacyImportTestFS) Open(path string) (lessonFile, error) {
	if fs.open != nil {
		return fs.open(path)
	}
	return fs.lessonFS.Open(path)
}

func (fs legacyImportTestFS) Remove(path string) error {
	if fs.remove != nil {
		return fs.remove(path)
	}
	return fs.lessonFS.Remove(path)
}

type legacyImportTestFile struct {
	lessonFile
	sync func() error
}

type legacyOwnershipTestFS struct {
	lessonFS
	lstat   func(string) (os.FileInfo, error)
	readDir func(string) ([]os.DirEntry, error)
	read    func(string) ([]byte, error)
}

func (fs legacyOwnershipTestFS) Lstat(path string) (os.FileInfo, error) {
	if fs.lstat != nil {
		return fs.lstat(path)
	}
	return fs.lessonFS.Lstat(path)
}

func (fs legacyOwnershipTestFS) ReadDir(path string) ([]os.DirEntry, error) {
	if fs.readDir != nil {
		return fs.readDir(path)
	}
	return fs.lessonFS.ReadDir(path)
}

func (fs legacyOwnershipTestFS) ReadFile(path string) ([]byte, error) {
	if fs.read != nil {
		return fs.read(path)
	}
	return fs.lessonFS.ReadFile(path)
}

func (f legacyImportTestFile) Sync() error {
	if f.sync != nil {
		return f.sync()
	}
	return f.lessonFile.Sync()
}

func legacyMapping(inv LegacyInventory, entries ...LegacyMappingEntry) LegacyMapping {
	return LegacyMapping{Source: inv.Source, Entries: entries}
}

func reviewedNew(key, slug string) LegacyMappingEntry {
	return LegacyMappingEntry{
		Key: key, Action: "new", Slug: slug, Status: "Recorded",
		Lesson:          "Apply the reviewed deterministic process control.",
		ProcessGap:      "The prior workflow lacked a deterministic check at the relevant boundary.",
		Classifications: []string{"process"},
	}
}

func TestInventoryLegacy_DuplicateIDsAreDistinctAndLossless(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "LESSONS-LEARNED.md")
	raw := "# log\n\n## L7 — first issue\n\n**Status:** Recorded\n\nbody\n\n## L7 — second issue\n\n**Status:** prose variant\n\nmore\n"
	if err := os.WriteFile(source, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	inv := inventoryLegacyForApply(t, source)
	if len(inv.Entries) != 2 || inv.Entries[0].Key != "L7#1" || inv.Entries[1].Key != "L7#2" {
		t.Fatalf("entries = %#v", inv.Entries)
	}
	if inv.Entries[0].Raw == "" || inv.Entries[0].BytesSHA256 == "" {
		t.Fatalf("lossless evidence missing: %#v", inv.Entries[0])
	}
	if len(inv.Entries[1].Warnings) == 0 {
		t.Fatal("duplicate legacy ID must be visible, not conflated")
	}
	if len(inv.Collisions) != 1 || inv.Collisions[0].Count != 2 {
		t.Fatalf("collision inventory = %#v", inv.Collisions)
	}
}

func TestPreflightLegacyApply_UsesTheSameWriteFreeValidation(t *testing.T) {
	dir := t.TempDir()
	lessonsDir := filepath.Join(dir, "spec", "lessons")
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "legacy.md")
	if err := os.WriteFile(source, []byte("## L1 — reviewed rule\n\n**Status:** Recorded\n\ntext\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inv := inventoryLegacyForApply(t, source)
	mapping := legacyMapping(inv, reviewedNew("L1#1", "reviewed-rule"))
	if err := PreflightLegacyApply(lessonsDir, []string{"process"}, inv, mapping); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(lessonsDir, "reviewed-rule")); !os.IsNotExist(err) {
		t.Fatalf("preflight wrote a target: %v", err)
	}
	if err := PreflightLegacyApply(lessonsDir, nil, inv, mapping); err == nil {
		t.Fatal("preflight accepted an empty classification vocabulary")
	}
}

func TestApplyLegacy_ReviewedMappingIdempotent(t *testing.T) {
	dir := t.TempDir()
	lessonsDir := filepath.Join(dir, "spec", "lessons")
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "legacy.md")
	if err := os.WriteFile(source, []byte("## L1 — use a check\n\n**Status:** Recorded\n\ntext\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inv := inventoryLegacyForApply(t, source)
	mapping := legacyMapping(inv, reviewedNew("L1#1", "use-a-check"))
	first, err := ApplyLegacy(lessonsDir, []string{"process"}, inv, mapping)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.CreatedLessons) != 1 || len(first.CreatedOccurrences) != 1 {
		t.Fatalf("first = %#v", first)
	}
	if len(first.StatusDecisions) != 1 || first.StatusDecisions[0].ImportedStatus != "Recorded" {
		t.Fatalf("status downgrade was not reported: %#v", first.StatusDecisions)
	}
	readme, err := os.ReadFile(filepath.Join(lessonsDir, "use-a-check", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Apply the reviewed deterministic process control.", "The prior workflow lacked a deterministic check"} {
		if !bytes.Contains(readme, []byte(want)) {
			t.Errorf("compact migrated Lesson lacks reviewed content %q", want)
		}
	}
	if bytes.Contains(readme, []byte("<!-- TODO:")) {
		t.Fatal("compact migrated Lesson retained scaffold placeholders")
	}
	providerPath := filepath.Join(lessonsDir, "use-a-check", "occurrences", first.CreatedOccurrences[0]+".json")
	providerBytes, err := os.ReadFile(providerPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(providerPath); err != nil {
		t.Fatal(err)
	}
	resumed, err := ApplyLegacy(lessonsDir, []string{"process"}, inv, mapping)
	if err != nil || len(resumed.CreatedLessons) != 0 || len(resumed.CreatedOccurrences) != 1 {
		t.Fatalf("provider clean resume = %#v err=%v", resumed, err)
	}
	resumedBytes, err := os.ReadFile(providerPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(providerBytes, resumedBytes) {
		t.Fatal("provider occurrence changed bytes on clean resume")
	}
	second, err := ApplyLegacy(lessonsDir, []string{"process"}, inv, mapping)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.CreatedLessons) != 0 || len(second.Skipped) != 1 {
		t.Fatalf("second = %#v", second)
	}
	if _, err := os.Stat(filepath.Join(lessonsDir, "use-a-check", "legacy")); !os.IsNotExist(err) {
		t.Fatalf("raw historical prose must not be republished: %v", err)
	}
}

func TestApplyLegacy_NewRequiresSafeReviewedCompactContentAndConfiguredClassification(t *testing.T) {
	tests := map[string]struct {
		allowed []string
		mutate  func(*LegacyMappingEntry)
	}{
		"missing vocabulary":          {allowed: nil},
		"missing classification":      {allowed: []string{"process"}, mutate: func(m *LegacyMappingEntry) { m.Classifications = nil }},
		"unconfigured classification": {allowed: []string{"validation"}},
		"missing Lesson":              {allowed: []string{"process"}, mutate: func(m *LegacyMappingEntry) { m.Lesson = "" }},
		"missing Process Gap":         {allowed: []string{"process"}, mutate: func(m *LegacyMappingEntry) { m.ProcessGap = "" }},
		"unsafe compact text":         {allowed: []string{"process"}, mutate: func(m *LegacyMappingEntry) { m.Lesson = "Contact person@example.com" }},
		"placeholder compact text":    {allowed: []string{"process"}, mutate: func(m *LegacyMappingEntry) { m.ProcessGap = "<!-- TODO: decide -->" }},
		"unreviewed status":           {allowed: []string{"process"}, mutate: func(m *LegacyMappingEntry) { m.Status = "Enforced" }},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			lessonsDir := filepath.Join(dir, "spec", "lessons")
			if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(dir, "legacy.md")
			if err := os.WriteFile(source, []byte("## L1 — reviewed rule\n\n**Status:** Enforced\n\ntext\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			inv := inventoryLegacyForApply(t, source)
			entry := reviewedNew("L1#1", "reviewed-rule")
			if tc.mutate != nil {
				tc.mutate(&entry)
			}
			before := snapshotTree(t, lessonsDir)
			if _, err := ApplyLegacy(lessonsDir, tc.allowed, inv, legacyMapping(inv, entry)); err == nil {
				t.Fatal("invalid reviewed mapping was accepted")
			}
			if !bytes.Equal(before, snapshotTree(t, lessonsDir)) {
				t.Fatal("invalid reviewed mapping changed the Lesson tree")
			}
		})
	}
}

func TestApplyLegacy_InvalidMappingIsWriteFree(t *testing.T) {
	dir := t.TempDir()
	lessonsDir := filepath.Join(dir, "spec", "lessons")
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "legacy.md")
	if err := os.WriteFile(source, []byte("## L1 — rule\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inv := inventoryLegacyForApply(t, source)
	invalid := reviewedNew("L1#1", "Bad_Slug")
	_, err := ApplyLegacy(lessonsDir, []string{"process"}, inv, legacyMapping(inv, invalid))
	if err == nil {
		t.Fatal("expected invalid mapping")
	}
	if _, err := os.Stat(filepath.Join(lessonsDir, ".legacy-import")); !os.IsNotExist(err) {
		t.Fatalf("invalid mapping wrote manifest: %v", err)
	}
}

func TestApplyLegacy_ManualDispositionFailsClosedBeforeAnyWrite(t *testing.T) {
	dir := t.TempDir()
	lessonsDir := filepath.Join(dir, "spec", "lessons")
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "legacy.md")
	if err := os.WriteFile(source, []byte("## L1 — decided\n\nbody\n\n## L2 — unresolved\n\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inv := inventoryLegacyForApply(t, source)
	mapping := legacyMapping(inv,
		reviewedNew("L1#1", "decided"),
		LegacyMappingEntry{Key: "L2#1", Action: "manual"},
	)
	if _, err := ApplyLegacy(lessonsDir, []string{"process"}, inv, mapping); err == nil || !strings.Contains(err.Error(), "resolve every row") {
		t.Fatalf("manual mapping should fail closed: %v", err)
	}
	entries, err := os.ReadDir(lessonsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("manual mapping partially applied: %#v", entries)
	}
}

func TestInventoryLegacy_AuthoritativeMonolithShape(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "LESSONS-LEARNED.md")
	raw := "# Log\n\n## L2026-01-01-0000: template only\n\n## Lessons\n\n### L1 — first\n\nbody\n\n**Recurred:** at least twice in one run\ncontinued detail\n\n## L56 (2026-07-31): dated title\n\nbody\n"
	if err := os.WriteFile(source, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	inv := inventoryLegacyForApply(t, source)
	if inv.EntryProjectionSHA256 != "7885a71fa422a7b08eb421d6abb08bcddeb0298527eeebbf946b05512a656c85" {
		t.Fatalf("authoritative-shape entry projection = %s", inv.EntryProjectionSHA256)
	}
	if len(inv.Entries) != 3 || inv.LessonCount != 2 || inv.RecurrenceMarkerCount != 1 || inv.Entries[0].LegacyID != "L1" || inv.Entries[1].Kind != "recurrence-marker" || inv.Entries[1].ParentKey != "L1#1" || inv.Entries[2].LegacyID != "L56" {
		t.Fatalf("authoritative inventory = %#v", inv.Entries)
	}
	marker := inv.Entries[1]
	if marker.Raw != raw[marker.StartByte:marker.EndByte] || shaString([]byte(marker.Raw)) != marker.BytesSHA256 || !containsString(marker.Warnings, "aggregate-count-ambiguous") {
		t.Fatalf("marker span/provenance = %#v", marker)
	}
}

func TestInventoryLegacy_AuthoritativeAuditProjectionFixture(t *testing.T) {
	type sourceFixture struct {
		Repository  string `json:"repository"`
		Path        string `json:"path"`
		Revision    string `json:"revision"`
		CommittedAt string `json:"committed_at"`
		SHA256      string `json:"sha256"`
		ByteCount   int    `json:"byte_count"`
	}
	type collisionFixture struct {
		LegacyID string   `json:"legacy_id"`
		Keys     []string `json:"keys"`
	}
	type auditFixture struct {
		SchemaVersion           int                `json:"schema_version"`
		Source                  sourceFixture      `json:"source"`
		LessonCount             int                `json:"lesson_count"`
		RecurrenceMarkerCount   int                `json:"recurrence_marker_count"`
		EntryCount              int                `json:"entry_count"`
		EntryProjectionSHA256   string             `json:"entry_projection_sha256"`
		EntryFixtureSHA256      string             `json:"entry_fixture_sha256"`
		Collisions              []collisionFixture `json:"collisions"`
		UnmatchedCandidateCount int                `json:"unmatched_candidate_count"`
	}
	type entryFixture struct {
		Key         string  `json:"key"`
		Kind        string  `json:"kind"`
		ParentKey   *string `json:"parent_key"`
		StartLine   int     `json:"start_line"`
		EndLine     int     `json:"end_line"`
		StartByte   int     `json:"start_byte"`
		EndByte     int     `json:"end_byte"`
		BytesSHA256 string  `json:"bytes_sha256"`
	}
	b, err := os.ReadFile(filepath.Join("testdata", "backstage-legacy-audit-91238f86.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture auditFixture
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		t.Fatalf("audit fixture has trailing JSON: %v", err)
	}
	if fixture.SchemaVersion != 1 || fixture.Source.Repository != "github.com/sneat-co/backstage" || fixture.Source.Path != "LESSONS-LEARNED.md" || fixture.Source.Revision != "91238f86dbe6b8effdbaa8f034d58e1ccf94c9aa" || fixture.Source.CommittedAt != "2026-08-10T12:20:12Z" || fixture.Source.SHA256 != "4641e3561d8db3c50e919a23c9d2975d06dd51058f1e4838c6b65dfc4d9838a8" || fixture.Source.ByteCount != 478835 {
		t.Fatalf("authoritative source fixture drifted: %#v", fixture.Source)
	}
	if fixture.LessonCount != 175 || fixture.RecurrenceMarkerCount != 35 || fixture.EntryCount != 210 || fixture.UnmatchedCandidateCount != 0 || fixture.EntryProjectionSHA256 != "cb19d7c1ed6dc432ff21fe0998bb77fe651fc29cc076517b0cc7ab1c750b9e93" {
		t.Fatalf("authoritative inventory fixture drifted: %#v", fixture)
	}
	entriesBytes, err := os.ReadFile(filepath.Join("testdata", "backstage-legacy-audit-91238f86.entries.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := shaString(entriesBytes); got != fixture.EntryFixtureSHA256 {
		t.Fatalf("authoritative exact-entry fixture digest drifted: %s", got)
	}
	var entries []entryFixture
	entriesDecoder := json.NewDecoder(bytes.NewReader(entriesBytes))
	entriesDecoder.DisallowUnknownFields()
	if err := entriesDecoder.Decode(&entries); err != nil {
		t.Fatal(err)
	}
	if err := entriesDecoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("exact-entry fixture has trailing JSON: %v", err)
	}
	if len(entries) != fixture.EntryCount {
		t.Fatalf("exact-entry fixture count = %d", len(entries))
	}
	seenEntries := map[string]entryFixture{}
	lessons, markers := 0, 0
	for i, entry := range entries {
		if entry.Key == "" || entry.StartLine < 1 || entry.EndLine < entry.StartLine || entry.StartByte < 0 || entry.EndByte <= entry.StartByte || entry.EndByte > fixture.Source.ByteCount || len(entry.BytesSHA256) != 64 {
			t.Fatalf("invalid exact source projection at row %d: %#v", i, entry)
		}
		if _, exists := seenEntries[entry.Key]; exists {
			t.Fatalf("duplicate exact source key %q", entry.Key)
		}
		switch entry.Kind {
		case "lesson":
			lessons++
			if entry.ParentKey != nil {
				t.Fatalf("Lesson %q unexpectedly has a parent", entry.Key)
			}
		case "recurrence-marker":
			markers++
			if entry.ParentKey == nil {
				t.Fatalf("marker %q lacks a parent", entry.Key)
			}
			parent, exists := seenEntries[*entry.ParentKey]
			if !exists || parent.Kind != "lesson" || entry.StartByte < parent.StartByte || entry.EndByte > parent.EndByte {
				t.Fatalf("marker %q range is not nested in parent %q", entry.Key, *entry.ParentKey)
			}
		default:
			t.Fatalf("unknown exact source entry kind %q", entry.Kind)
		}
		seenEntries[entry.Key] = entry
	}
	if lessons != fixture.LessonCount || markers != fixture.RecurrenceMarkerCount {
		t.Fatalf("exact source projection counts = lessons:%d markers:%d", lessons, markers)
	}
	wantCollisions := []string{"L157:L157#1,L157#2", "L22:L22#1,L22#2", "L34:L34#1,L34#2", "L35:L35#1,L35#2", "L51:L51#1,L51#2,L51#3"}
	var gotCollisions []string
	for _, collision := range fixture.Collisions {
		gotCollisions = append(gotCollisions, collision.LegacyID+":"+strings.Join(collision.Keys, ","))
	}
	if strings.Join(gotCollisions, "|") != strings.Join(wantCollisions, "|") {
		t.Fatalf("authoritative collision fixture drifted: %v", gotCollisions)
	}
}

func TestApplyLegacy_CreatesReviewedMarkerOccurrenceUnderNewLesson(t *testing.T) {
	dir := t.TempDir()
	lessonsDir := filepath.Join(dir, "spec", "lessons")
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "legacy.md")
	raw := "## L1 — durable rule\n\nbody\n\n**Recurred:** at least twice; aggregate count retained for review\n\n"
	if err := os.WriteFile(source, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	inv := inventoryLegacyForApply(t, source)
	if len(inv.Entries) != 2 {
		t.Fatalf("inventory = %#v", inv.Entries)
	}
	mapping := legacyMapping(inv,
		reviewedNew("L1#1", "durable-rule"),
		LegacyMappingEntry{Key: "L1#1/recurrence#1", Action: "occurrence", Slug: "durable-rule"},
	)
	first, err := ApplyLegacy(lessonsDir, []string{"process"}, inv, mapping)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.CreatedLessons) != 1 || len(first.CreatedOccurrences) != 2 {
		t.Fatalf("first = %#v", first)
	}
	for _, id := range first.CreatedOccurrences {
		o, err := FindOccurrence(filepath.Join(lessonsDir, "durable-rule", "README.md"), id)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(o.Summary, "recurrence#1") && !containsString(o.Redactions, "aggregate-count-ambiguous") {
			t.Fatal("aggregate marker occurrence lost its ambiguity tag")
		}
	}
	before := snapshotTree(t, lessonsDir)
	second, err := ApplyLegacy(lessonsDir, []string{"process"}, inv, mapping)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Skipped) != 2 || !bytes.Equal(before, snapshotTree(t, lessonsDir)) {
		t.Fatalf("idempotent second = %#v", second)
	}
}

func TestApplyLegacy_OccurrenceSecondRunIsByteIdentical(t *testing.T) {
	dir := t.TempDir()
	lessonsDir := filepath.Join(dir, "spec", "lessons")
	target := filepath.Join(lessonsDir, "canonical", "README.md")
	body, err := ScaffoldCanonical(ScaffoldOptions{Slug: "canonical", Owner: "tester"}, []string{"process"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(filepath.Dir(target), "occurrences"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, body, 0o644); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "legacy.md")
	if err := os.WriteFile(source, []byte("## L9 — happened again\n\n**Status:** Recorded\n\ntext\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inv := inventoryLegacyForApply(t, source)
	mapping := legacyMapping(inv, LegacyMappingEntry{Key: "L9#1", Action: "occurrence", Slug: "canonical"})
	first, err := ApplyLegacy(lessonsDir, []string{"process"}, inv, mapping)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.CreatedOccurrences) != 1 {
		t.Fatalf("first = %#v", first)
	}
	occurrencePath := filepath.Join(filepath.Dir(target), "occurrences", first.CreatedOccurrences[0]+".json")
	firstBytes, err := os.ReadFile(occurrencePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(occurrencePath); err != nil {
		t.Fatal(err)
	}
	recreated, err := ApplyLegacy(lessonsDir, []string{"process"}, inv, mapping)
	if err != nil || len(recreated.CreatedOccurrences) != 1 {
		t.Fatalf("clean retry = %#v err=%v", recreated, err)
	}
	recreatedBytes, err := os.ReadFile(occurrencePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBytes, recreatedBytes) {
		t.Fatal("clean retry changed deterministic occurrence bytes")
	}
	before := snapshotTree(t, lessonsDir)
	second, err := ApplyLegacy(lessonsDir, []string{"process"}, inv, mapping)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.CreatedOccurrences) != 0 || len(second.Skipped) != 1 {
		t.Fatalf("second = %#v", second)
	}
	after := snapshotTree(t, lessonsDir)
	if !bytes.Equal(before, after) {
		t.Fatal("second apply changed repository bytes")
	}
}

func TestApplyLegacy_DuplicateNewTargetPreflightIsWriteFree(t *testing.T) {
	dir := t.TempDir()
	lessonsDir := filepath.Join(dir, "spec", "lessons")
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "legacy.md")
	if err := os.WriteFile(source, []byte("## L1 — one\n\na\n## L2 — two\n\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inv := inventoryLegacyForApply(t, source)
	mapping := legacyMapping(inv, reviewedNew("L1#1", "same"), reviewedNew("L2#1", "same"))
	if _, err := ApplyLegacy(lessonsDir, []string{"process"}, inv, mapping); err == nil {
		t.Fatal("expected duplicate target rejection")
	}
	entries, err := os.ReadDir(lessonsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("preflight wrote %#v", entries)
	}
}

func TestApplyLegacy_PublicationFailureRollsBackEveryArtifact(t *testing.T) {
	dir := t.TempDir()
	lessonsDir := filepath.Join(dir, "spec", "lessons")
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "legacy.md")
	if err := os.WriteFile(source, []byte("## L1 — one\n\na\n## L2 — two\n\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inv := inventoryLegacyForApply(t, source)
	mapping := legacyMapping(inv, reviewedNew("L1#1", "one"), reviewedNew("L2#1", "two"))
	calls := 0
	deps := defaultLegacyImportDeps()
	deps.fs = legacyImportTestFS{lessonFS: osLessonFS{}, link: func(old, new string) error {
		calls++
		if calls == 2 {
			return errors.New("injected publish failure")
		}
		return os.Link(old, new)
	}}
	if _, err := applyLegacyWithDeps(lessonsDir, []string{"process"}, inv, mapping, deps); err == nil || MutationOutcomeOf(err) != MutationUncertain {
		t.Fatal("expected injected failure")
	}
	// Historical name retained for deletion-audit continuity. The safe policy
	// now retains every post-publication artifact and reconciles it on retry.
	resumed, err := ApplyLegacy(lessonsDir, []string{"process"}, inv, mapping)
	if err != nil || len(resumed.CreatedOccurrences) != 2 {
		t.Fatalf("retained publication did not resume: result=%#v err=%v", resumed, err)
	}
}

func TestApplyLegacy_RollbackPreservesConcurrentForeignOccurrenceAndRetryCompletes(t *testing.T) {
	dir := t.TempDir()
	lessonsDir := filepath.Join(dir, "spec", "lessons")
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "legacy.md")
	if err := os.WriteFile(source, []byte("## L1 — one\n\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inv := inventoryLegacyForApply(t, source)
	mapping := legacyMapping(inv, reviewedNew("L1#1", "one"))
	foreignID := "12345678-1234-4123-8123-123456789abc"
	removeCalls := 0
	deps := defaultLegacyImportDeps()
	deps.fs = legacyImportTestFS{lessonFS: osLessonFS{}, remove: func(path string) error {
		removeCalls++
		return os.Remove(path)
	}, link: func(old, new string) error {
		if !strings.Contains(new, string(filepath.Separator)+".legacy-import"+string(filepath.Separator)) {
			return os.Link(old, new)
		}
		if _, err := AddOccurrence(AddOccurrenceOptions{
			LessonPath: filepath.Join(lessonsDir, "one", "README.md"),
			ID:         foreignID,
			Summary:    "Concurrent independently owned occurrence.",
			Context:    map[string]any{},
			Evidence:   Evidence{Kind: "none"},
			Now:        time.Date(2026, 8, 10, 13, 0, 0, 0, time.UTC),
		}); err != nil {
			t.Fatalf("adding concurrent occurrence: %v", err)
		}
		return errors.New("injected manifest publication failure")
	}}

	_, err := applyLegacyWithDeps(lessonsDir, []string{"process"}, inv, mapping, deps)
	if got := MutationOutcomeOf(err); got != MutationUncertain {
		t.Fatalf("outcome = %v, want uncertain: %v", got, err)
	}
	if !strings.Contains(err.Error(), "retained for durable retry") {
		t.Fatalf("error does not explain refused ownership rollback: %v", err)
	}
	if removeCalls != 0 {
		t.Fatalf("post-publication failure attempted %d destructive removes", removeCalls)
	}
	foreignPath := filepath.Join(lessonsDir, "one", "occurrences", foreignID+".json")
	foreignBefore, err := os.ReadFile(foreignPath)
	if err != nil {
		t.Fatalf("foreign occurrence was deleted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(lessonsDir, "one", "README.md")); err != nil {
		t.Fatalf("published Lesson was partially removed: %v", err)
	}

	resumed, err := ApplyLegacy(lessonsDir, []string{"process"}, inv, mapping)
	if err != nil {
		t.Fatalf("retry did not reconcile the preserved publication: %v", err)
	}
	if len(resumed.CreatedLessons) != 0 || len(resumed.CreatedOccurrences) != 1 {
		t.Fatalf("retry result = %#v", resumed)
	}
	foreignAfter, err := os.ReadFile(foreignPath)
	if err != nil || !bytes.Equal(foreignBefore, foreignAfter) {
		t.Fatalf("retry changed foreign occurrence: err=%v", err)
	}
	second, err := ApplyLegacy(lessonsDir, []string{"process"}, inv, mapping)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.CreatedLessons) != 0 || len(second.CreatedOccurrences) != 0 || len(second.Skipped) != 1 {
		t.Fatalf("completed retry is not idempotent: %#v", second)
	}
}

func TestLegacyPublishedSet_VerificationRejectsEveryOwnershipDrift(t *testing.T) {
	root := t.TempDir()
	stage := filepath.Join(root, "stage")
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(stage, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeDurableStageFile(filepath.Join(stage, "README.md"), []byte("owned\n")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(target, "foreign.json")
	if err := os.WriteFile(foreign, []byte("foreign\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := publishStagedLesson(stage, target); err == nil || MutationOutcomeOf(err) != MutationPrePublication {
		t.Fatalf("foreign target err=%v outcome=%v", err, MutationOutcomeOf(err))
	}
	if got, err := os.ReadFile(foreign); err != nil || string(got) != "foreign\n" {
		t.Fatalf("foreign target was changed: %q err=%v", got, err)
	}
}

func TestPublishStagedLessonRetryAndCollisionEdges(t *testing.T) {
	newStage := func(t *testing.T, root string) (string, []byte) {
		t.Helper()
		stage := filepath.Join(root, "stage")
		if err := os.MkdirAll(stage, 0o755); err != nil {
			t.Fatal(err)
		}
		readme := []byte("owned\n")
		if err := writeDurableStageFile(filepath.Join(stage, "README.md"), readme); err != nil {
			t.Fatal(err)
		}
		return stage, []byte("specscore-legacy-import:" + shaString(readme) + "\n")
	}

	t.Run("complete retry is byte-identical", func(t *testing.T) {
		root := t.TempDir()
		stage, _ := newStage(t, root)
		target := filepath.Join(root, "target")
		if err := publishStagedLesson(stage, target); err != nil {
			t.Fatal(err)
		}
		if err := publishStagedLesson(stage, target); err != nil {
			t.Fatalf("retry: %v", err)
		}
	})

	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, root, target string, marker []byte) lessonFS
	}{
		{name: "different existing marker", setup: func(t *testing.T, _, target string, _ []byte) lessonFS {
			t.Helper()
			if err := os.Mkdir(target, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(target, ".legacy-import-owner"), []byte("different"), 0o644); err != nil {
				t.Fatal(err)
			}
			return osLessonFS{}
		}},
		{name: "partial target unreadable", setup: func(t *testing.T, _, target string, _ []byte) lessonFS {
			t.Helper()
			if err := os.Mkdir(target, 0o755); err != nil {
				t.Fatal(err)
			}
			return legacyOwnershipTestFS{lessonFS: osLessonFS{}, readDir: func(string) ([]os.DirEntry, error) { return nil, errors.New("readdir") }}
		}},
		{name: "marker unreadable", setup: func(t *testing.T, _, target string, _ []byte) lessonFS {
			t.Helper()
			if err := os.MkdirAll(filepath.Join(target, ".legacy-import-owner"), 0o755); err != nil {
				t.Fatal(err)
			}
			return osLessonFS{}
		}},
		{name: "marker link collision", setup: func(t *testing.T, _, target string, _ []byte) lessonFS {
			t.Helper()
			return legacyImportTestFS{lessonFS: osLessonFS{}, link: func(old, new string) error {
				if filepath.Base(new) == ".legacy-import-owner" {
					if err := os.WriteFile(new, []byte("racer"), 0o644); err != nil {
						t.Fatal(err)
					}
					return os.ErrExist
				}
				return os.Link(old, new)
			}}
		}},
		{name: "occurrence store collision", setup: func(t *testing.T, _, target string, marker []byte) lessonFS {
			t.Helper()
			if err := os.Mkdir(target, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(target, ".legacy-import-owner"), marker, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(target, "occurrences"), []byte("foreign"), 0o644); err != nil {
				t.Fatal(err)
			}
			return osLessonFS{}
		}},
		{name: "README collision", setup: func(t *testing.T, _, target string, marker []byte) lessonFS {
			t.Helper()
			if err := os.MkdirAll(filepath.Join(target, "occurrences"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(target, ".legacy-import-owner"), marker, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(target, "README.md"), []byte("foreign"), 0o644); err != nil {
				t.Fatal(err)
			}
			return osLessonFS{}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			stage, marker := newStage(t, root)
			target := filepath.Join(root, "target")
			fs := tc.setup(t, root, target, marker)
			if err := publishStagedLessonWithFS(stage, target, fs); err == nil {
				t.Fatal("collision/fault was accepted")
			}
		})
	}
}

func TestLegacyPreflightSurfacesUnreadableRetryDirectory(t *testing.T) {
	lessons, inv, mapping := legacyMatrixFixture(t)
	target := filepath.Join(lessons, mapping.Entries[0].Slug)
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	fs := legacyOwnershipTestFS{lessonFS: osLessonFS{}, readDir: func(path string) ([]os.DirEntry, error) {
		if path == target {
			return nil, errors.New("unreadable partial target")
		}
		return os.ReadDir(path)
	}}
	if _, err := preflightLegacyApplyWithFS(lessons, []string{"process"}, inv, mapping, fs); err == nil || !strings.Contains(err.Error(), "unreadable partial target") {
		t.Fatalf("preflight error = %v", err)
	}
}

func TestApplyLegacy_PostLinkDurabilityFailureRollsBackOwnedArtifacts(t *testing.T) {
	for name, failCall := range map[string]int{"Lesson directory sync": 2, "manifest directory sync": 4} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			lessonsDir := filepath.Join(dir, "spec", "lessons")
			if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(dir, "legacy.md")
			if err := os.WriteFile(source, []byte("## L1 — one\n\n**Status:** Recorded\n\nbody\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			inv := inventoryLegacyForApply(t, source)
			mapping := legacyMapping(inv, reviewedNew("L1#1", "one"))
			calls := 0
			deps := defaultLegacyImportDeps()
			deps.fs = legacyImportTestFS{lessonFS: osLessonFS{}, open: func(path string) (lessonFile, error) {
				file, err := osLessonFS{}.Open(path)
				if err != nil {
					return nil, err
				}
				return legacyImportTestFile{lessonFile: file, sync: func() error {
					calls++
					if calls == failCall {
						return errors.New("injected post-link sync failure")
					}
					return file.Sync()
				}}, nil
			}}

			if _, err := applyLegacyWithDeps(lessonsDir, []string{"process"}, inv, mapping, deps); err == nil || MutationOutcomeOf(err) != MutationUncertain {
				t.Fatal("expected injected durability failure")
			}
			resumed, err := ApplyLegacy(lessonsDir, []string{"process"}, inv, mapping)
			if err != nil || len(resumed.CreatedOccurrences) != 1 {
				t.Fatalf("durability retry result=%#v err=%v", resumed, err)
			}
		})
	}
}

func TestApplyLegacy_CompensationFailureAfterPublicationRemainsUncertain(t *testing.T) {
	dir := t.TempDir()
	lessonsDir := filepath.Join(dir, "spec", "lessons")
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "legacy.md")
	if err := os.WriteFile(source, []byte("## L1 — one\n\na\n## L2 — two\n\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inv := inventoryLegacyForApply(t, source)
	mapping := legacyMapping(inv, reviewedNew("L1#1", "one"), reviewedNew("L2#1", "two"))
	calls := 0
	deps := defaultLegacyImportDeps()
	deps.fs = legacyImportTestFS{lessonFS: osLessonFS{}, open: func(path string) (lessonFile, error) {
		file, openErr := osLessonFS{}.Open(path)
		if openErr != nil {
			return nil, openErr
		}
		return legacyImportTestFile{lessonFile: file, sync: func() error {
			calls++
			// Call four is a post-publication durability fence. No later call
			// may attempt destructive compensation.
			if calls == 4 {
				return errors.New("injected publication directory sync failure")
			}
			return file.Sync()
		}}, nil
	}}

	_, err := applyLegacyWithDeps(lessonsDir, []string{"process"}, inv, mapping, deps)
	if MutationOutcomeOf(err) != MutationUncertain {
		t.Fatalf("outcome=%v err=%v; post-publication compensation was not proven durable", MutationOutcomeOf(err), err)
	}
	if calls != 4 {
		t.Fatalf("post-publication failure attempted extra destructive fences: calls=%d", calls)
	}
}

func TestApplyLegacy_UnsafeHistoricalSourceIsReferencedNotRepublished(t *testing.T) {
	dir := t.TempDir()
	lessonsDir := filepath.Join(dir, "spec", "lessons")
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	secret := "ghp_" + strings.Repeat("1", 30)
	source := filepath.Join(dir, "legacy.md")
	if err := os.WriteFile(source, []byte("## L1 — unsafe\n\n"+secret+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inv := inventoryLegacyForApply(t, source)
	result, err := ApplyLegacy(lessonsDir, []string{"process"}, inv, legacyMapping(inv, reviewedNew("L1#1", "unsafe")))
	if err != nil {
		t.Fatalf("safe reference-only import rejected historical source: %v", err)
	}
	tree := snapshotTree(t, lessonsDir)
	if bytes.Contains(tree, []byte(secret)) || bytes.Contains(tree, []byte("source_bytes_base64")) {
		t.Fatal("committed import artifacts republished unsafe historical bytes")
	}
	manifest, err := os.ReadFile(result.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{inv.Source.Repository, inv.Source.Path, inv.Source.Revision, inv.Source.SHA256, `"start_byte"`, `"end_byte"`, inv.Entries[0].BytesSHA256} {
		if !bytes.Contains(manifest, []byte(want)) {
			t.Errorf("manifest missing immutable provenance %q", want)
		}
	}
}

func snapshotTree(t *testing.T, root string) []byte {
	t.Helper()
	var paths []string
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	var out bytes.Buffer
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(root, path)
		out.WriteString(filepath.ToSlash(rel))
		out.WriteByte(0)
		out.Write(b)
		out.WriteByte(0)
	}
	return out.Bytes()
}
