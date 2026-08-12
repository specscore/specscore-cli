package lesson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func withFlatSourceIdentity(t *testing.T) {
	t.Helper()
	// Fixtures are committed by writeFlatFixture. This deliberately leaves the
	// public lifecycle on its production immutable-Git identity path instead of
	// installing a process-global source-identity hook.
}

type flatMigrationTestFS struct {
	lessonFS
	link func(string, string) error
	open func(string) (lessonFile, error)
}

func (fs flatMigrationTestFS) Link(oldname, newname string) error {
	if fs.link != nil {
		return fs.link(oldname, newname)
	}
	return fs.lessonFS.Link(oldname, newname)
}

func (fs flatMigrationTestFS) Open(path string) (lessonFile, error) {
	if fs.open != nil {
		return fs.open(path)
	}
	return fs.lessonFS.Open(path)
}

type flatMigrationTestFile struct {
	lessonFile
	sync func() error
}

func (f flatMigrationTestFile) Sync() error {
	if f.sync != nil {
		return f.sync()
	}
	return f.lessonFile.Sync()
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
	root := filepath.Dir(filepath.Dir(lessonsDir))
	if _, err := os.Stat(filepath.Join(root, ".git")); os.IsNotExist(err) {
		for _, args := range [][]string{{"init"}, {"config", "user.email", "test@example.invalid"}, {"config", "user.name", "SpecScore test"}, {"remote", "add", "origin", "https://github.com/example/process.git"}} {
			if output, commandErr := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); commandErr != nil {
				t.Fatalf("git %v: %v: %s", args, commandErr, output)
			}
		}
	}
	if output, err := exec.Command("git", "-C", root, "add", ".").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, output)
	}
	if output, err := exec.Command("git", "-C", root, "commit", "-m", "fixture").CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, output)
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

func TestFlatMigration_PublicFailureEdgesRemainWriteFree(t *testing.T) {
	root := t.TempDir()
	lessons := filepath.Join(root, "spec", "lessons")
	if err := os.MkdirAll(lessons, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := PreflightFlatMigration(FlatMigrationOptions{}); err == nil {
		t.Fatal("invalid options accepted")
	}
	if _, err := MigrateFlat(FlatMigrationOptions{LessonsDir: lessons, Slug: "rule", Classifications: []string{"process"}}); err == nil {
		t.Fatal("missing flat source without proof accepted")
	}

	markerPath := filepath.Join(lessons, ".flat-migration-rule.json")
	if err := os.WriteFile(markerPath, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := PreflightFlatMigration(FlatMigrationOptions{LessonsDir: lessons, Slug: "rule", Classifications: []string{"process"}}); err == nil {
		t.Fatal("malformed marker accepted")
	}
	if err := os.Remove(markerPath); err != nil {
		t.Fatal(err)
	}
	invalidMarker, err := json.Marshal(flatMigrationMarker{SchemaVersion: 1, Slug: "rule"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(markerPath, invalidMarker, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := MigrateFlat(FlatMigrationOptions{LessonsDir: lessons, Slug: "rule", Classifications: []string{"process"}}); err == nil {
		t.Fatal("invalid marker accepted")
	}
	if err := os.Remove(markerPath); err != nil {
		t.Fatal(err)
	}

	ref := coverageLegacySource()
	marker := flatMigrationMarker{SchemaVersion: 1, Source: ref, Slug: "rule", Classifications: []string{"process"}, EventUUID: "01234567-89ab-4def-8123-456789abcdef"}
	markerBytes, err := json.Marshal(marker)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(markerPath, markerBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := PreflightFlatMigration(FlatMigrationOptions{LessonsDir: lessons, Slug: "rule", Classifications: []string{"validation"}}); err == nil {
		t.Fatal("changed retry classifications accepted")
	}
	if err := os.Remove(markerPath); err != nil {
		t.Fatal(err)
	}

	flat := writeFlatFixture(t, lessons, "rule", flatFixture("Recorded", 0, ""))
	if err := os.WriteFile(filepath.Join(lessons, "rule"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	withFlatSourceIdentity(t)
	if _, err := MigrateFlat(FlatMigrationOptions{LessonsDir: lessons, Slug: "rule", Classifications: []string{"process"}}); err == nil {
		t.Fatal("regular canonical target accepted")
	}
	if b, err := os.ReadFile(flat); err != nil || !bytes.Contains(b, []byte("# Lesson:")) {
		t.Fatalf("failed migration mutated source: %q err=%v", b, err)
	}
}

func TestFinalizeFlatMigration_RejectsMismatchedOrIncompleteTransaction(t *testing.T) {
	withFlatSourceIdentity(t)
	lessons := filepath.Join(t.TempDir(), "spec", "lessons")
	body := flatFixture("Recorded", 0, "")
	writeFlatFixture(t, lessons, "finalize-boundary", body)
	opts := FlatMigrationOptions{LessonsDir: lessons, Slug: "finalize-boundary", Classifications: []string{"process"}}
	result, err := MigrateFlat(opts)
	if err != nil || !result.PendingFinalize {
		t.Fatalf("migration=%#v err=%v", result, err)
	}
	if err := FinalizeFlatMigration(opts, "01234567-89ab-4def-8123-456789abcdef"); err == nil {
		t.Fatal("finalization accepted a different event UUID")
	}
	if err := FinalizeFlatMigration(FlatMigrationOptions{LessonsDir: lessons, Slug: opts.Slug, Classifications: []string{"validation"}}, FlatMigrationEventUUID(shaString([]byte(body)), opts.Slug)); err == nil {
		t.Fatal("finalization accepted different classifications")
	}
	flat := filepath.Join(lessons, opts.Slug+".md")
	if err := os.WriteFile(flat, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := FinalizeFlatMigration(opts, FlatMigrationEventUUID(shaString([]byte(body)), opts.Slug)); err == nil {
		t.Fatal("finalization accepted a remaining flat source")
	}
	if err := os.Remove(flat); err != nil {
		t.Fatal(err)
	}
	if err := FinalizeFlatMigration(opts, FlatMigrationEventUUID(shaString([]byte(body)), opts.Slug)); err == nil {
		t.Fatal("finalization accepted an absent exact index row")
	}
	writeFlatMigrationIndex(t, lessons, result.CanonicalPath)
	if err := FinalizeFlatMigration(opts, FlatMigrationEventUUID(shaString([]byte(body)), opts.Slug)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(lessons, ".flat-migration-"+opts.Slug+".json")); !os.IsNotExist(err) {
		t.Fatalf("finalization retained marker: %v", err)
	}
}

func TestFlatMigration_DurableMarkerIsTheOnlyResumeAuthority(t *testing.T) {
	lessons := filepath.Join(t.TempDir(), "spec", "lessons")
	if err := os.MkdirAll(lessons, 0o755); err != nil {
		t.Fatal(err)
	}
	ref := coverageLegacySource()
	marker := flatMigrationMarker{
		SchemaVersion:   1,
		Source:          ref,
		Slug:            "resume-boundary",
		Classifications: []string{"process"},
		EventUUID:       "01234567-89ab-4def-8123-456789abcdef",
	}
	b, err := json.Marshal(marker)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lessons, ".flat-migration-resume-boundary.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	opts := FlatMigrationOptions{LessonsDir: lessons, Slug: "resume-boundary", Classifications: []string{"process"}}
	preflight, err := PreflightFlatMigration(opts)
	if err != nil || !preflight.PendingTransaction || preflight.EventUUID != marker.EventUUID {
		t.Fatalf("preflight=%#v err=%v", preflight, err)
	}
	if _, err := MigrateFlat(FlatMigrationOptions{LessonsDir: lessons, Slug: opts.Slug, Classifications: opts.Classifications, EventUUID: "01234567-89ab-4def-8123-456789abcdea"}); err == nil {
		t.Fatal("resume accepted a different caller event UUID")
	}
	if _, err := MigrateFlat(opts); err == nil {
		t.Fatal("resume accepted a marker without the canonical proof")
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
	injected := false
	deps := defaultFlatMigrationDeps()
	deps.fs = flatMigrationTestFS{lessonFS: osLessonFS{}, open: func(path string) (lessonFile, error) {
		file, err := osLessonFS{}.Open(path)
		if err != nil {
			return nil, err
		}
		return flatMigrationTestFile{lessonFile: file, sync: func() error {
			if path == lessonsDir {
				if _, statErr := os.Stat(flatPath); os.IsNotExist(statErr) && !injected {
					injected = true
					return errors.New("injected source-remove directory sync failure")
				}
			}
			return file.Sync()
		}}, nil
	}}
	if _, err := migrateFlatWithDeps(opts, deps); err == nil {
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
		deps := defaultFlatMigrationDeps()
		deps.fs = flatMigrationTestFS{lessonFS: osLessonFS{}, link: func(old, new string) error {
			if strings.HasSuffix(new, filepath.Join("rollback", "README.md")) {
				return errors.New("injected publication failure")
			}
			return os.Link(old, new)
		}}
		if _, err := migrateFlatWithDeps(FlatMigrationOptions{LessonsDir: lessonsDir, Slug: "rollback", Classifications: []string{"process"}}, deps); err == nil {
			t.Fatal("expected injected failure")
		}
		if _, err := os.Stat(filepath.Join(lessonsDir, ".flat-migration-rollback.json")); err != nil {
			t.Fatalf("post-marker failure did not retain recovery marker: %v", err)
		}
	})
}

func TestMigrateFlatWithDeps_PublishAndDurabilityFailuresRestoreFreshTree(t *testing.T) {
	for _, failLinkCall := range []int{1, 2, 3, 4} {
		t.Run(fmt.Sprintf("link-%d", failLinkCall), func(t *testing.T) {
			lessonsDir := filepath.Join(t.TempDir(), "spec", "lessons")
			writeFlatFixture(t, lessonsDir, "rollback", flatFixture("Recorded", 0, ""))
			calls := 0
			failedTarget := ""
			deps := defaultFlatMigrationDeps()
			deps.fs = flatMigrationTestFS{lessonFS: osLessonFS{}, link: func(old, new string) error {
				calls++
				if calls == failLinkCall {
					failedTarget = new
					return errors.New("injected link boundary failure")
				}
				return os.Link(old, new)
			}}
			if _, err := migrateFlatWithDeps(FlatMigrationOptions{LessonsDir: lessonsDir, Slug: "rollback", Classifications: []string{"process"}}, deps); err == nil {
				t.Fatal("expected injected link failure")
			}
			if calls != failLinkCall {
				t.Fatalf("link calls=%d, want %d", calls, failLinkCall)
			}
			marker := filepath.Join(lessonsDir, ".flat-migration-rollback.json")
			if failedTarget == marker {
				if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
					t.Fatalf("pre-publication failure retained marker: %v", statErr)
				}
			} else if _, statErr := os.Stat(marker); statErr != nil && !strings.Contains(failedTarget, ".flat-migration-stage-") {
				t.Fatalf("post-publication failure lost marker: %v", statErr)
			}
		})
	}

	for _, failSyncCall := range []int{1, 2, 3, 4} {
		t.Run(fmt.Sprintf("sync-%d", failSyncCall), func(t *testing.T) {
			lessonsDir := filepath.Join(t.TempDir(), "spec", "lessons")
			writeFlatFixture(t, lessonsDir, "rollback", flatFixture("Recorded", 0, ""))
			calls := 0
			deps := defaultFlatMigrationDeps()
			deps.fs = flatMigrationTestFS{lessonFS: osLessonFS{}, open: func(path string) (lessonFile, error) {
				file, err := osLessonFS{}.Open(path)
				if err != nil {
					return nil, err
				}
				return flatMigrationTestFile{lessonFile: file, sync: func() error {
					calls++
					if calls == failSyncCall {
						return errors.New("injected directory sync failure")
					}
					return file.Sync()
				}}, nil
			}}
			if _, err := migrateFlatWithDeps(FlatMigrationOptions{LessonsDir: lessonsDir, Slug: "rollback", Classifications: []string{"process"}}, deps); err == nil {
				t.Fatal("expected injected sync failure")
			}
			if calls < failSyncCall {
				t.Fatalf("sync calls=%d, want at least %d", calls, failSyncCall)
			}
			marker := filepath.Join(lessonsDir, ".flat-migration-rollback.json")
			if _, statErr := os.Stat(marker); statErr != nil && failSyncCall > 1 {
				t.Fatalf("post-publication sync failure lost marker: %v", statErr)
			}
		})
	}
}

func TestMigrateFlatRetainsConcurrentForeignChildrenAndCleanRetry(t *testing.T) {
	lessonsDir := filepath.Join(t.TempDir(), "spec", "lessons")
	writeFlatFixture(t, lessonsDir, "foreign-child", flatFixture("Recorded", 0, ""))
	foreignID := "f1234567-89ab-4def-8123-456789abcdef"
	foreignPath := filepath.Join(lessonsDir, "foreign-child", "occurrences", foreignID+".json")
	var foreign []byte
	deps := defaultFlatMigrationDeps()
	base := osLessonFS{}
	deps.fs = flatMigrationTestFS{lessonFS: base, link: func(old, new string) error {
		if strings.Contains(new, filepath.Join(".legacy-import", "flat-")) {
			if _, err := AddOccurrence(AddOccurrenceOptions{
				LessonPath: filepath.Join(lessonsDir, "foreign-child", "README.md"),
				ID:         foreignID, Summary: "Concurrent independently owned occurrence.",
				Context: map[string]any{"writer": "foreign"}, Evidence: Evidence{Kind: "none"}, Now: time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC),
			}); err != nil {
				return err
			}
			foreign, _ = os.ReadFile(foreignPath)
			return errors.New("manifest publication failed after foreign child appeared")
		}
		return base.Link(old, new)
	}}
	opts := FlatMigrationOptions{LessonsDir: lessonsDir, Slug: "foreign-child", Classifications: []string{"process"}}
	if _, err := migrateFlatWithDeps(opts, deps); err == nil || MutationOutcomeOf(err) != MutationUncertain {
		t.Fatalf("post-visible failure = %v, outcome=%v", err, MutationOutcomeOf(err))
	}
	if got, err := os.ReadFile(foreignPath); err != nil || !bytes.Equal(got, foreign) {
		t.Fatalf("foreign child was deleted or changed: %q, %v", got, err)
	}
	result, err := MigrateFlat(opts)
	if err != nil {
		t.Fatalf("clean retry: %v", err)
	}
	writeFlatMigrationIndex(t, lessonsDir, result.CanonicalPath)
	if err := FinalizeFlatMigration(opts, FlatMigrationEventUUID(resultSourceSHA(t, lessonsDir, opts.Slug), opts.Slug)); err != nil {
		t.Fatalf("finalize retry: %v", err)
	}
	if got, err := os.ReadFile(foreignPath); err != nil || !bytes.Equal(got, foreign) {
		t.Fatalf("clean retry changed foreign child: %q, %v", got, err)
	}
}

func resultSourceSHA(t *testing.T, lessonsDir, slug string) string {
	t.Helper()
	markerBytes, err := os.ReadFile(filepath.Join(lessonsDir, ".flat-migration-"+slug+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var marker flatMigrationMarker
	if err := decodeStrictJSON(markerBytes, &marker); err != nil {
		t.Fatal(err)
	}
	return marker.Source.SHA256
}

func TestMigrateFlatNoReplaceMovesPreserveConcurrentSourceAndRecoveryTarget(t *testing.T) {
	t.Run("source replacement", func(t *testing.T) {
		lessonsDir := filepath.Join(t.TempDir(), "spec", "lessons")
		flatPath := writeFlatFixture(t, lessonsDir, "source-race", flatFixture("Recorded", 0, ""))
		foreign := []byte("foreign replacement\n")
		deps := defaultFlatMigrationDeps()
		realMove := deps.renameNoReplace
		deps.renameNoReplace = func(old, new string) error {
			if old == flatPath {
				if err := os.WriteFile(old, foreign, 0o600); err != nil {
					return err
				}
			}
			return realMove(old, new)
		}
		opts := FlatMigrationOptions{LessonsDir: lessonsDir, Slug: "source-race", Classifications: []string{"process"}}
		_, err := migrateFlatWithDeps(opts, deps)
		if err == nil || MutationOutcomeOf(err) != MutationUncertain {
			t.Fatalf("source race = %v", err)
		}
		markerBytes, readErr := os.ReadFile(filepath.Join(lessonsDir, ".flat-migration-source-race.json"))
		if readErr != nil {
			t.Fatal(readErr)
		}
		var marker flatMigrationMarker
		if err := decodeStrictJSON(markerBytes, &marker); err != nil {
			t.Fatal(err)
		}
		got, readErr := os.ReadFile(filepath.Join(flatMigrationRecoveryDir(lessonsDir, marker.EventUUID), "source.md"))
		if readErr != nil || !bytes.Equal(got, foreign) {
			t.Fatalf("foreign source replacement was not retained: %q, %v", got, readErr)
		}
	})

	t.Run("recovery target", func(t *testing.T) {
		lessonsDir := filepath.Join(t.TempDir(), "spec", "lessons")
		flatPath := writeFlatFixture(t, lessonsDir, "target-race", flatFixture("Recorded", 0, ""))
		foreign := []byte("foreign recovery target\n")
		deps := defaultFlatMigrationDeps()
		realMove := deps.renameNoReplace
		deps.renameNoReplace = func(old, new string) error {
			if old == flatPath {
				if err := os.WriteFile(new, foreign, 0o600); err != nil {
					return err
				}
			}
			return realMove(old, new)
		}
		opts := FlatMigrationOptions{LessonsDir: lessonsDir, Slug: "target-race", Classifications: []string{"process"}}
		_, err := migrateFlatWithDeps(opts, deps)
		if err == nil || MutationOutcomeOf(err) != MutationUncertain {
			t.Fatalf("target race = %v", err)
		}
		if got, readErr := os.ReadFile(flatPath); readErr != nil || bytes.Equal(got, foreign) {
			t.Fatalf("owned source was lost: %q, %v", got, readErr)
		}
		markerBytes, readErr := os.ReadFile(filepath.Join(lessonsDir, ".flat-migration-target-race.json"))
		if readErr != nil {
			t.Fatal(readErr)
		}
		var marker flatMigrationMarker
		if err := decodeStrictJSON(markerBytes, &marker); err != nil {
			t.Fatal(err)
		}
		got, readErr := os.ReadFile(filepath.Join(flatMigrationRecoveryDir(lessonsDir, marker.EventUUID), "source.md"))
		if readErr != nil || !bytes.Equal(got, foreign) {
			t.Fatalf("foreign recovery target was overwritten: %q, %v", got, readErr)
		}
	})
}

func TestFinalizeFlatMigrationPreservesConcurrentMarkerReplacement(t *testing.T) {
	lessonsDir := filepath.Join(t.TempDir(), "spec", "lessons")
	writeFlatFixture(t, lessonsDir, "marker-race", flatFixture("Recorded", 0, ""))
	opts := FlatMigrationOptions{LessonsDir: lessonsDir, Slug: "marker-race", Classifications: []string{"process"}}
	result, err := MigrateFlat(opts)
	if err != nil {
		t.Fatal(err)
	}
	writeFlatMigrationIndex(t, lessonsDir, result.CanonicalPath)
	markerPath := filepath.Join(lessonsDir, ".flat-migration-marker-race.json")
	markerBytes, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatal(err)
	}
	var marker flatMigrationMarker
	if err := decodeStrictJSON(markerBytes, &marker); err != nil {
		t.Fatal(err)
	}
	foreign := []byte("foreign marker replacement\n")
	deps := defaultFlatMigrationDeps()
	realMove := deps.renameNoReplace
	deps.renameNoReplace = func(old, new string) error {
		if old == markerPath {
			if err := os.WriteFile(old, foreign, 0o600); err != nil {
				return err
			}
		}
		return realMove(old, new)
	}
	err = finalizeFlatMigrationWithDeps(opts, marker.EventUUID, deps)
	if err == nil || !strings.Contains(err.Error(), "marker changed") {
		t.Fatalf("marker race = %v", err)
	}
	completed := filepath.Join(flatMigrationRecoveryDir(lessonsDir, marker.EventUUID), "transaction.complete.json")
	if got, readErr := os.ReadFile(completed); readErr != nil || !bytes.Equal(got, foreign) {
		t.Fatalf("foreign marker replacement was deleted: %q, %v", got, readErr)
	}
}

func TestMigrateFlat_ResumesDurableMarkerWithoutOverwriting(t *testing.T) {
	withFlatSourceIdentity(t)
	lessonsDir := filepath.Join(t.TempDir(), "spec", "lessons")
	flatPath := writeFlatFixture(t, lessonsDir, "resume", flatFixture("Recorded", 1, "## Recurrences\n\n- 2026-08-02 — Again.\n"))
	sourceBytes, _ := os.ReadFile(flatPath)
	source, err := resolveLegacySourceIdentity(flatPath, sourceBytes)
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
