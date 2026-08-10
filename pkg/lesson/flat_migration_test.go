package lesson

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func withFlatSourceIdentity(t *testing.T) {
	t.Helper()
	original := legacySourceIdentityFn
	legacySourceIdentityFn = func(path string, b []byte) (LegacySourceRef, error) {
		return LegacySourceRef{
			Repository:  "github.com/example/process",
			Path:        filepath.ToSlash(filepath.Join("spec", "lessons", filepath.Base(path))),
			Revision:    strings.Repeat("b", 40),
			CommittedAt: "2026-08-10T12:00:00Z",
			SHA256:      shaString(b),
			ByteCount:   len(b),
		}, nil
	}
	t.Cleanup(func() { legacySourceIdentityFn = original })
}

func flatFixture(status string, recurred int, recurrences string) string {
	return `---
format: https://specscore.md/lesson-specification
status: ` + status + `
---

# Lesson: Verify the durable boundary

**Status:** ` + status + `
**Date:** 2026-08-01
**Owner:** codex
**Recurred:** ` + fmtInt(recurred) + `

## Incident

The original incident included private evidence at /Users/private/work and person@example.com.

## Process gap

No check compared durable state with replay.

## Check

Validate the replayed state at the persistence boundary.

## Enforcement

The historical enforcement prose remains at the immutable source.

## Tracking

Historical tracking.

` + recurrences + `
---
*This document follows the https://specscore.md/lesson-specification*
`
}

func fmtInt(value int) string {
	if value == 0 {
		return "0"
	}
	if value == 1 {
		return "1"
	}
	return "2"
}

func writeFlatFixture(t *testing.T, lessonsDir, slug, body string) string {
	t.Helper()
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(lessonsDir, slug+".md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeFlatMigrationIndex(t *testing.T, lessonsDir, canonicalPath string) {
	t.Helper()
	l, err := Parse(canonicalPath)
	if err != nil {
		t.Fatal(err)
	}
	occurrences, err := DiscoverOccurrences(canonicalPath)
	if err != nil {
		t.Fatal(err)
	}
	last := ""
	if len(occurrences) > 0 {
		last = occurrences[len(occurrences)-1].OccurredAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	control := strings.TrimSpace(l.Control)
	if control == "" {
		control = "—"
	}
	row := fmt.Sprintf("| [%s](%s/README.md) | %s | %s | %d | %s | %s |", l.Slug, l.Slug, l.Status, strings.Join(l.Classifications, ", "), len(occurrences), last, control)
	index := "# Lessons\n\n## Lessons\n\n| Lesson | Status | Classifications | Occurrences | Last Occurred | Enforcement |\n|---|---|---|---:|---|---|\n" + row + "\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(lessonsDir, "README.md"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateFlat_PreservesImmutableProvenanceAndEveryStructuredObservation(t *testing.T) {
	withFlatSourceIdentity(t)
	lessonsDir := filepath.Join(t.TempDir(), "spec", "lessons")
	body := flatFixture("Enforced", 2, `## Recurrences

- 2026-08-02 — First recurrence.

- 2026-08-03 — Second recurrence.
`)
	flatPath := writeFlatFixture(t, lessonsDir, "durable-boundary", body)

	opts := FlatMigrationOptions{
		LessonsDir: lessonsDir, Slug: "durable-boundary", Classifications: []string{"validation"},
		Control: "Run the durable replay boundary check.", Verification: "go test ./pkg/event -run TestReplayBoundary", Evidence: "pkg/event/outbox_test.go",
	}
	first, err := MigrateFlat(opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.CreatedOccurrences) != 3 {
		t.Fatalf("provider plus two recurrences = %d, want 3: %#v", len(first.CreatedOccurrences), first)
	}
	if _, err := os.Stat(flatPath); !os.IsNotExist(err) {
		t.Fatalf("exact legacy source should be removed: %v", err)
	}
	readme, err := os.ReadFile(first.CanonicalPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"/Users/private/work", "person@example.com", "## Incident", "## Recurrences"} {
		if bytes.Contains(readme, []byte(forbidden)) {
			t.Errorf("canonical README republished forbidden legacy prose %q", forbidden)
		}
	}
	for _, want := range []string{"status: Enforced", "**Status:** Enforced", "**Classifications:** validation", "**Legacy Provenance:** github.com/example/process@", "**Control:** Run the durable replay boundary check.", "**Verification:** go test ./pkg/event -run TestReplayBoundary", "**Evidence:** pkg/event/outbox_test.go"} {
		if !bytes.Contains(readme, []byte(want)) {
			t.Errorf("canonical README lacks %q:\n%s", want, readme)
		}
	}
	if bytes.Contains(readme, []byte("git show")) || bytes.Contains(readme, []byte("**Evidence:** sha256:")) {
		t.Fatal("migration fabricated enforcement verification or evidence from provenance")
	}
	manifest, err := os.ReadFile(first.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(manifest, []byte("private evidence")) || bytes.Contains(manifest, []byte("person@example.com")) {
		t.Fatal("manifest copied raw legacy prose")
	}
	occurrences, err := DiscoverOccurrences(first.CanonicalPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(occurrences) != 3 || !occurrences[0].OccurredAt.Equal(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("deterministic occurrences = %#v", occurrences)
	}
	writeFlatMigrationIndex(t, lessonsDir, first.CanonicalPath)
	if err := FinalizeFlatMigration(opts, FlatMigrationEventUUID(shaString([]byte(body)), opts.Slug)); err != nil {
		t.Fatal(err)
	}

	before := snapshotTree(t, lessonsDir)
	second, err := MigrateFlat(opts)
	if err != nil {
		t.Fatal(err)
	}
	if !second.AlreadyMigrated || !bytes.Equal(before, snapshotTree(t, lessonsDir)) {
		t.Fatalf("second run was not byte-identical/no-op: %#v", second)
	}
}

func TestMigrateFlat_CompletedRetryIsReceiptBackedAndByteStable(t *testing.T) {
	withFlatSourceIdentity(t)
	lessonsDir := filepath.Join(t.TempDir(), "spec", "lessons")
	writeFlatFixture(t, lessonsDir, "retry-boundary", flatFixture("Enforced", 0, ""))
	opts := FlatMigrationOptions{LessonsDir: lessonsDir, Slug: "retry-boundary", Classifications: []string{"process"}, Control: "Run durable migration checks.", Verification: "go test ./pkg/lesson", Evidence: "pkg/lesson/flat_migration_test.go"}
	first, err := MigrateFlat(opts)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(first.CanonicalPath)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MigrateFlat(opts)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(second.CanonicalPath)
	if err != nil {
		t.Fatal(err)
	}
	if first.CanonicalPath != second.CanonicalPath || !bytes.Equal(before, after) {
		t.Fatalf("completed retry changed durable projection: %#v %#v", first, second)
	}
}

func TestMigrateFlat_SourceRemoveDirectorySyncFailureResumesSameTransaction(t *testing.T) {
	withFlatSourceIdentity(t)
	lessonsDir := filepath.Join(t.TempDir(), "spec", "lessons")
	body := flatFixture("Recorded", 0, "")
	flatPath := writeFlatFixture(t, lessonsDir, "sync-resume", body)
	opts := FlatMigrationOptions{LessonsDir: lessonsDir, Slug: "sync-resume", Classifications: []string{"process"}}
	preflight, err := PreflightFlatMigration(opts)
	if err != nil {
		t.Fatal(err)
	}
	orig := flatSyncDirectory
	injected := false
	flatSyncDirectory = func(path string) error {
		if path == lessonsDir {
			if _, statErr := os.Stat(flatPath); os.IsNotExist(statErr) && !injected {
				injected = true
				return errors.New("injected source-remove directory sync failure")
			}
		}
		return orig(path)
	}
	t.Cleanup(func() { flatSyncDirectory = orig })
	if _, err := MigrateFlat(opts); err == nil {
		t.Fatal("expected source-remove directory sync failure")
	}
	if !injected {
		t.Fatal("source-remove sync seam did not run")
	}
	if _, err := os.Stat(flatPath); !os.IsNotExist(err) {
		t.Fatalf("source removal was not observable: %v", err)
	}
	marker := filepath.Join(lessonsDir, ".flat-migration-sync-resume.json")
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("resume marker missing: %v", err)
	}

	flatSyncDirectory = orig
	retryPreflight, err := PreflightFlatMigration(opts)
	if err != nil {
		t.Fatal(err)
	}
	if !retryPreflight.PendingTransaction || retryPreflight.EventUUID != preflight.EventUUID {
		t.Fatalf("retry did not retain deterministic transaction: %#v", retryPreflight)
	}
	retry, err := MigrateFlat(FlatMigrationOptions{LessonsDir: lessonsDir, Slug: opts.Slug, Classifications: opts.Classifications, EventUUID: retryPreflight.EventUUID})
	if err != nil || !retry.PendingFinalize {
		t.Fatalf("resume=%#v err=%v", retry, err)
	}
	writeFlatMigrationIndex(t, lessonsDir, retry.CanonicalPath)
	if err := FinalizeFlatMigration(opts, preflight.EventUUID); err != nil {
		t.Fatal(err)
	}
	completed, err := MigrateFlat(opts)
	if err != nil || !completed.AlreadyMigrated {
		t.Fatalf("final retry=%#v err=%v", completed, err)
	}
}

func TestPreflightFlatMigration_SourceAbsentWithoutMarkerRequiresManifestAndExactIndex(t *testing.T) {
	withFlatSourceIdentity(t)
	for name, breakProof := range map[string]func(t *testing.T, lessonsDir string, result FlatMigrationResult){
		"missing manifest": func(t *testing.T, _ string, result FlatMigrationResult) {
			if err := os.Remove(result.ManifestPath); err != nil {
				t.Fatal(err)
			}
		},
		"missing exact index row": func(t *testing.T, lessonsDir string, _ FlatMigrationResult) {
			if err := os.WriteFile(filepath.Join(lessonsDir, "README.md"), []byte("# Lessons\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			lessonsDir := filepath.Join(t.TempDir(), "spec", "lessons")
			writeFlatFixture(t, lessonsDir, "proof-required", flatFixture("Recorded", 0, ""))
			opts := FlatMigrationOptions{LessonsDir: lessonsDir, Slug: "proof-required", Classifications: []string{"process"}}
			result, err := MigrateFlat(opts)
			if err != nil {
				t.Fatal(err)
			}
			writeFlatMigrationIndex(t, lessonsDir, result.CanonicalPath)
			marker := filepath.Join(lessonsDir, ".flat-migration-proof-required.json")
			if err := os.Remove(marker); err != nil {
				t.Fatal(err)
			}
			breakProof(t, lessonsDir, result)
			if _, err := PreflightFlatMigration(opts); err == nil {
				t.Fatal("source-absent manual/incomplete state was accepted as migrated")
			}
		})
	}
}

func TestMigrateFlat_EnforcedSourceRequiresReviewedEvidenceWithoutFabrication(t *testing.T) {
	withFlatSourceIdentity(t)
	lessonsDir := filepath.Join(t.TempDir(), "spec", "lessons")
	// This is the historical Backstage Enforced shape: narrative Enforcement
	// and Tracking sections, but no canonical control/verification/evidence.
	body := `---
format: https://specscore.md/lesson-specification
status: Enforced
---

# Lesson: Aggregate validation must prove replayable history

**Status:** Enforced
**Date:** 2026-07-28
**Owner:** codex
**Recurred:** 0

## Incident

Persisted aggregates could replay a different semantic outcome.

## Process gap

Validation checked shape without proving canonical replay.

## Check

Reconstruct every durable receipt from canonical commands.

## Enforcement

Enforced in a historical implementation commit with adversarial tests.

## Tracking

Historical implementation commit.

---
*This document follows the https://specscore.md/lesson-specification*
`
	writeFlatFixture(t, lessonsDir, "aggregate-validation", body)
	before := snapshotTree(t, lessonsDir)
	_, err := MigrateFlat(FlatMigrationOptions{LessonsDir: lessonsDir, Slug: "aggregate-validation", Classifications: []string{"validation"}})
	if err == nil || !strings.Contains(err.Error(), "requires reviewed control, verification, and evidence") {
		t.Fatalf("expected explicit enforcement mapping requirement, got %v", err)
	}
	if !bytes.Equal(before, snapshotTree(t, lessonsDir)) {
		t.Fatal("failed Enforced migration changed the Lesson tree")
	}
}

func TestMigrateFlat_RecurredMismatchIsWriteFree(t *testing.T) {
	withFlatSourceIdentity(t)
	lessonsDir := filepath.Join(t.TempDir(), "spec", "lessons")
	flatPath := writeFlatFixture(t, lessonsDir, "mismatch", flatFixture("Recorded", 2, ""))
	original, _ := os.ReadFile(flatPath)
	before := snapshotTree(t, lessonsDir)
	_, err := MigrateFlat(FlatMigrationOptions{LessonsDir: lessonsDir, Slug: "mismatch", Classifications: []string{"process"}})
	if err == nil || !strings.Contains(err.Error(), "refusing to invent or drop history") {
		t.Fatalf("expected count mismatch: %v", err)
	}
	if !bytes.Equal(before, snapshotTree(t, lessonsDir)) {
		t.Fatal("failed preflight changed the Lesson tree")
	}
	if current, _ := os.ReadFile(flatPath); !bytes.Equal(current, original) {
		t.Fatal("failed preflight changed source bytes")
	}
}

func TestMigrateFlat_RefusesCanonicalSiblingAndRollsBackPublicationFailure(t *testing.T) {
	withFlatSourceIdentity(t)
	t.Run("sibling collision", func(t *testing.T) {
		lessonsDir := filepath.Join(t.TempDir(), "spec", "lessons")
		writeFlatFixture(t, lessonsDir, "collision", flatFixture("Recorded", 0, ""))
		if err := os.Mkdir(filepath.Join(lessonsDir, "collision"), 0o755); err != nil {
			t.Fatal(err)
		}
		before := snapshotTree(t, lessonsDir)
		if _, err := MigrateFlat(FlatMigrationOptions{LessonsDir: lessonsDir, Slug: "collision", Classifications: []string{"process"}}); err == nil {
			t.Fatal("expected canonical sibling collision")
		}
		if !bytes.Equal(before, snapshotTree(t, lessonsDir)) {
			t.Fatal("collision preflight changed bytes")
		}
	})

	t.Run("publish rollback", func(t *testing.T) {
		lessonsDir := filepath.Join(t.TempDir(), "spec", "lessons")
		writeFlatFixture(t, lessonsDir, "rollback", flatFixture("Recorded", 0, ""))
		before := snapshotTree(t, lessonsDir)
		original := flatPublishLink
		calls := 0
		flatPublishLink = func(old, new string) error {
			calls++
			if calls == 2 {
				return errors.New("injected publication failure")
			}
			return os.Link(old, new)
		}
		t.Cleanup(func() { flatPublishLink = original })
		if _, err := MigrateFlat(FlatMigrationOptions{LessonsDir: lessonsDir, Slug: "rollback", Classifications: []string{"process"}}); err == nil {
			t.Fatal("expected injected failure")
		}
		if !bytes.Equal(before, snapshotTree(t, lessonsDir)) {
			t.Fatal("failed publication did not roll back exactly")
		}
	})
}

func TestMigrateFlat_ResumesDurableMarkerWithoutOverwriting(t *testing.T) {
	withFlatSourceIdentity(t)
	lessonsDir := filepath.Join(t.TempDir(), "spec", "lessons")
	flatPath := writeFlatFixture(t, lessonsDir, "resume", flatFixture("Recorded", 1, "## Recurrences\n\n- 2026-08-02 — Again.\n"))
	sourceBytes, _ := os.ReadFile(flatPath)
	source, err := legacySourceIdentityFn(flatPath, sourceBytes)
	if err != nil {
		t.Fatal(err)
	}
	opts := FlatMigrationOptions{LessonsDir: lessonsDir, Slug: "resume", Classifications: []string{"process"}}
	eventUUID := FlatMigrationEventUUID(source.SHA256, opts.Slug)
	opts.EventUUID = eventUUID
	stage, _, _, err := stageFlatMigration(opts, sourceBytes, flatPath, source)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(stage) }()
	manifestPath := flatManifestPath(lessonsDir, source, "resume")
	expected, err := collectFlatExpectedFiles(stage, lessonsDir, "resume", manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	marker, err := marshalSafeJSON("marker", flatMigrationMarker{SchemaVersion: 1, Source: source, Slug: "resume", Classifications: []string{"process"}, EventUUID: eventUUID, Files: expected})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lessonsDir, ".flat-migration-resume.json"), marker, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(lessonsDir, "resume", "occurrences"), 0o755); err != nil {
		t.Fatal(err)
	}
	readme, _ := os.ReadFile(filepath.Join(stage, "README.md"))
	if err := os.WriteFile(filepath.Join(lessonsDir, "resume", "README.md"), readme, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := MigrateFlat(opts)
	if err != nil {
		t.Fatal(err)
	}
	if result.AlreadyMigrated || len(result.CreatedOccurrences) != 2 {
		t.Fatalf("resume result = %#v", result)
	}
	if _, err := os.Stat(flatPath); !os.IsNotExist(err) {
		t.Fatalf("resume did not remove exact flat source: %v", err)
	}
	markerPath := filepath.Join(lessonsDir, ".flat-migration-resume.json")
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("resume did not retain marker through index/event boundary: %v", err)
	}
	writeFlatMigrationIndex(t, lessonsDir, result.CanonicalPath)
	if err := FinalizeFlatMigration(opts, eventUUID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("finalization did not retire marker: %v", err)
	}
}

func TestMigrateFlat_CrashAfterSourceRemovalResumesSameTransaction(t *testing.T) {
	withFlatSourceIdentity(t)
	lessonsDir := filepath.Join(t.TempDir(), "spec", "lessons")
	body := flatFixture("Recorded", 0, "")
	writeFlatFixture(t, lessonsDir, "crash-resume", body)
	opts := FlatMigrationOptions{LessonsDir: lessonsDir, Slug: "crash-resume", Classifications: []string{"process"}}
	preflight, err := PreflightFlatMigration(opts)
	if err != nil {
		t.Fatal(err)
	}
	opts.EventUUID = preflight.EventUUID
	first, err := MigrateFlat(opts)
	if err != nil {
		t.Fatal(err)
	}
	if !first.PendingFinalize {
		t.Fatal("published files must remain in a durable pending-finalize transaction")
	}

	retryPreflight, err := PreflightFlatMigration(FlatMigrationOptions{LessonsDir: lessonsDir, Slug: opts.Slug, Classifications: opts.Classifications})
	if err != nil {
		t.Fatal(err)
	}
	if !retryPreflight.PendingTransaction || retryPreflight.EventUUID != preflight.EventUUID || retryPreflight.Source != preflight.Source {
		t.Fatalf("retry did not recover exact transaction: %#v", retryPreflight)
	}
	retry, err := MigrateFlat(opts)
	if err != nil {
		t.Fatal(err)
	}
	if !retry.PendingFinalize || len(retry.CreatedOccurrences) != 1 {
		t.Fatalf("retry result = %#v", retry)
	}
	writeFlatMigrationIndex(t, lessonsDir, retry.CanonicalPath)
	if err := FinalizeFlatMigration(opts, preflight.EventUUID); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, lessonsDir)
	completed, err := MigrateFlat(FlatMigrationOptions{LessonsDir: lessonsDir, Slug: opts.Slug, Classifications: opts.Classifications})
	if err != nil || !completed.AlreadyMigrated || !bytes.Equal(before, snapshotTree(t, lessonsDir)) {
		t.Fatalf("completed retry was not read-only: %#v err=%v", completed, err)
	}
}
