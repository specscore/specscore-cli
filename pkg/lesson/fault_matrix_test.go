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

// faultMatrixFS delegates to a real private filesystem and fails exactly one
// observed operation. The resulting tests exercise real directory trees and
// durable files; only the selected syscall boundary is synthetic.
type faultMatrixFS struct {
	lessonFS
	failAt int
	calls  int
	trace  []string
}

func (fs *faultMatrixFS) hit(op string) error {
	fs.calls++
	fs.trace = append(fs.trace, op)
	if fs.failAt == fs.calls {
		return fmt.Errorf("injected %s failure at operation %d", op, fs.calls)
	}
	return nil
}

func (fs *faultMatrixFS) ReadFile(path string) ([]byte, error) {
	if err := fs.hit("ReadFile"); err != nil {
		return nil, err
	}
	return fs.lessonFS.ReadFile(path)
}
func (fs *faultMatrixFS) ReadDir(path string) ([]os.DirEntry, error) {
	if err := fs.hit("ReadDir"); err != nil {
		return nil, err
	}
	return fs.lessonFS.ReadDir(path)
}
func (fs *faultMatrixFS) Stat(path string) (os.FileInfo, error) {
	if err := fs.hit("Stat"); err != nil {
		return nil, err
	}
	return fs.lessonFS.Stat(path)
}
func (fs *faultMatrixFS) Lstat(path string) (os.FileInfo, error) {
	if err := fs.hit("Lstat"); err != nil {
		return nil, err
	}
	return fs.lessonFS.Lstat(path)
}
func (fs *faultMatrixFS) Mkdir(path string, mode os.FileMode) error {
	if err := fs.hit("Mkdir"); err != nil {
		return err
	}
	return fs.lessonFS.Mkdir(path, mode)
}
func (fs *faultMatrixFS) MkdirAll(path string, mode os.FileMode) error {
	if err := fs.hit("MkdirAll"); err != nil {
		return err
	}
	return fs.lessonFS.MkdirAll(path, mode)
}
func (fs *faultMatrixFS) MkdirTemp(dir, pattern string) (string, error) {
	if err := fs.hit("MkdirTemp"); err != nil {
		return "", err
	}
	return fs.lessonFS.MkdirTemp(dir, pattern)
}
func (fs *faultMatrixFS) Remove(path string) error {
	if err := fs.hit("Remove"); err != nil {
		return err
	}
	return fs.lessonFS.Remove(path)
}
func (fs *faultMatrixFS) RemoveAll(path string) error {
	if err := fs.hit("RemoveAll"); err != nil {
		return err
	}
	return fs.lessonFS.RemoveAll(path)
}
func (fs *faultMatrixFS) CreateTemp(dir, pattern string) (lessonFile, error) {
	if err := fs.hit("CreateTemp"); err != nil {
		return nil, err
	}
	f, err := fs.lessonFS.CreateTemp(dir, pattern)
	if err != nil {
		return nil, err
	}
	return &faultMatrixFile{lessonFile: f, fs: fs}, nil
}
func (fs *faultMatrixFS) Open(path string) (lessonFile, error) {
	if err := fs.hit("Open"); err != nil {
		return nil, err
	}
	f, err := fs.lessonFS.Open(path)
	if err != nil {
		return nil, err
	}
	return &faultMatrixFile{lessonFile: f, fs: fs}, nil
}
func (fs *faultMatrixFS) OpenFile(path string, flag int, mode os.FileMode) (lessonFile, error) {
	if err := fs.hit("OpenFile"); err != nil {
		return nil, err
	}
	f, err := fs.lessonFS.OpenFile(path, flag, mode)
	if err != nil {
		return nil, err
	}
	return &faultMatrixFile{lessonFile: f, fs: fs}, nil
}
func (fs *faultMatrixFS) Link(oldname, newname string) error {
	if err := fs.hit("Link"); err != nil {
		return err
	}
	return fs.lessonFS.Link(oldname, newname)
}
func (fs *faultMatrixFS) Rename(oldname, newname string) error {
	if err := fs.hit("Rename"); err != nil {
		return err
	}
	return fs.lessonFS.Rename(oldname, newname)
}

type faultMatrixFile struct {
	lessonFile
	fs *faultMatrixFS
}

func (f *faultMatrixFile) Chmod(mode os.FileMode) error {
	if err := f.fs.hit("Chmod"); err != nil {
		return err
	}
	return f.lessonFile.Chmod(mode)
}
func (f *faultMatrixFile) Write(data []byte) (int, error) {
	if err := f.fs.hit("Write"); err != nil {
		return 0, err
	}
	return f.lessonFile.Write(data)
}
func (f *faultMatrixFile) Sync() error {
	if err := f.fs.hit("Sync"); err != nil {
		return err
	}
	return f.lessonFile.Sync()
}
func (f *faultMatrixFile) Close() error {
	if err := f.fs.hit("Close"); err != nil {
		_ = f.lessonFile.Close()
		return err
	}
	return f.lessonFile.Close()
}

func flatMatrixFixture(t *testing.T, slug string) (string, FlatMigrationOptions, flatMigrationDeps) {
	t.Helper()
	lessons := filepath.Join(t.TempDir(), "spec", "lessons")
	if err := os.MkdirAll(lessons, 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte(flatFixture("Recorded", 1, "## Recurrences\n\n- 2026-08-02 — Again.\n"))
	path := filepath.Join(lessons, slug+".md")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	source := coverageLegacySource()
	source.Path = "spec/lessons/" + slug + ".md"
	source.SHA256 = shaString(body)
	source.ByteCount = len(body)
	deps := defaultFlatMigrationDeps()
	deps.sourceIdentity = func(string, []byte) (LegacySourceRef, error) { return source, nil }
	return lessons, FlatMigrationOptions{LessonsDir: lessons, Slug: slug, Classifications: []string{"process"}}, deps
}

func TestMigrateFlatEveryFilesystemFailureHasACompleteOutcome(t *testing.T) {
	_, baselineOpts, baselineDeps := flatMatrixFixture(t, "baseline")
	baselineFS := &faultMatrixFS{lessonFS: osLessonFS{}}
	baselineDeps.fs = baselineFS
	if _, err := migrateFlatWithDeps(baselineOpts, baselineDeps); err != nil {
		t.Fatal(err)
	}
	for failAt := 1; failAt <= baselineFS.calls; failAt++ {
		failAt := failAt
		op := baselineFS.trace[failAt-1]
		t.Run(fmt.Sprintf("%03d-%s", failAt, op), func(t *testing.T) {
			lessons, opts, deps := flatMatrixFixture(t, "rule")
			before := snapshotTree(t, lessons)
			fs := &faultMatrixFS{lessonFS: osLessonFS{}, failAt: failAt}
			deps.fs = fs
			_, err := migrateFlatWithDeps(opts, deps)
			after := snapshotTree(t, lessons)
			if err == nil {
				// Deferred cleanup is deliberately best-effort after a completed
				// transaction; its injected failure cannot invalidate the result.
				if op != "RemoveAll" && op != "Remove" && op != "Close" {
					t.Fatalf("injected %s failure was ignored", op)
				}
				return
			}
			if bytes.Equal(before, after) {
				return
			}
			// Once the marker is visible, every later failure retains durable
			// partial state. Every visible owned file must match the marker; an
			// absent file remains resumable and no path is rolled back.
			markerBytes, readErr := os.ReadFile(filepath.Join(lessons, ".flat-migration-rule.json"))
			if readErr != nil {
				t.Fatalf("changed tree lacks durable marker: %v; err=%v", readErr, err)
			}
			var marker flatMigrationMarker
			if decodeErr := decodeStrictJSON(markerBytes, &marker); decodeErr != nil {
				t.Fatalf("changed tree has malformed marker: %v", decodeErr)
			}
			if verifyErr := verifyExpectedFiles(lessons, marker.Files, true); verifyErr != nil {
				t.Fatalf("marker does not prove retained tree: %v", verifyErr)
			}
			if _, statErr := os.Stat(filepath.Join(lessons, "rule.md")); os.IsNotExist(statErr) {
				if verifyErr := verifyFlatMigrationRecoverySourceWithFS(lessons, marker.Source, marker.EventUUID, osLessonFS{}); verifyErr != nil {
					t.Fatalf("retired source lacks private recovery proof: %v", verifyErr)
				}
			}
		})
	}
}

func legacyMatrixFixture(t *testing.T) (string, LegacyInventory, LegacyMapping) {
	t.Helper()
	root := t.TempDir()
	lessons := filepath.Join(root, "spec", "lessons")
	if err := os.MkdirAll(lessons, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "LESSONS-LEARNED.md")
	if err := os.WriteFile(source, []byte("## L1 — reviewed rule\n\n**Status:** Recorded\n\ntext\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inv := inventoryLegacyForApply(t, source)
	return lessons, inv, legacyMapping(inv, reviewedNew("L1#1", "reviewed-rule"))
}

func TestApplyLegacyEveryFilesystemFailurePreservesOrClassifiesTheCompleteTree(t *testing.T) {
	_, inv, mapping := legacyMatrixFixture(t)
	baselineLessons := filepath.Join(t.TempDir(), "spec", "lessons")
	if err := os.MkdirAll(baselineLessons, 0o755); err != nil {
		t.Fatal(err)
	}
	baselineFS := &faultMatrixFS{lessonFS: osLessonFS{}}
	baselineDeps := defaultLegacyImportDeps()
	baselineDeps.fs = baselineFS
	if _, err := applyLegacyWithDeps(baselineLessons, []string{"process"}, inv, mapping, baselineDeps); err != nil {
		t.Fatal(err)
	}
	for failAt := 1; failAt <= baselineFS.calls; failAt++ {
		failAt := failAt
		op := baselineFS.trace[failAt-1]
		t.Run(fmt.Sprintf("%03d-%s", failAt, op), func(t *testing.T) {
			lessons, freshInv, freshMapping := legacyMatrixFixture(t)
			before := snapshotTree(t, lessons)
			fs := &faultMatrixFS{lessonFS: osLessonFS{}, failAt: failAt}
			deps := defaultLegacyImportDeps()
			deps.fs = fs
			_, err := applyLegacyWithDeps(lessons, []string{"process"}, freshInv, freshMapping, deps)
			after := snapshotTree(t, lessons)
			if err == nil {
				if op != "RemoveAll" && op != "Remove" && op != "Close" {
					t.Fatalf("injected %s failure was ignored", op)
				}
				return
			}
			if bytes.Equal(before, after) {
				return
			}
			if MutationOutcomeOf(err) != MutationUncertain {
				t.Fatalf("changed tree reported %v: %v", MutationOutcomeOf(err), err)
			}
			if entries, readErr := os.ReadDir(lessons); readErr != nil {
				t.Fatal(readErr)
			} else {
				for _, entry := range entries {
					if strings.HasPrefix(entry.Name(), ".legacy-import-stage-") {
						t.Fatalf("rollback leaked staging directory %s", entry.Name())
					}
				}
			}
		})
	}
}

type matrixLocker struct {
	locked bool
	err    error
}

func (l matrixLocker) TryLock() (bool, error) { return l.locked, l.err }
func (matrixLocker) Unlock() error            { return errors.New("ignored unlock failure") }

func TestRelationPerCallDependenciesCoverLockAndPublicationFailures(t *testing.T) {
	lessons := relationFixture(t, "a", "b")
	base := defaultRelationDeps()
	for name, mutate := range map[string]func(*relationDeps){
		"absolute path": func(d *relationDeps) { d.abs = func(string) (string, error) { return "", errors.New("abs") } },
		"physical root": func(d *relationDeps) { d.evalSymlinks = func(string) (string, error) { return "", errors.New("eval") } },
		"lock error": func(d *relationDeps) {
			d.newLock = func(string) relationLocker { return matrixLocker{err: errors.New("lock")} }
		},
		"lock busy": func(d *relationDeps) { d.newLock = func(string) relationLocker { return matrixLocker{} } },
	} {
		t.Run(name, func(t *testing.T) {
			deps := base
			mutate(&deps)
			before := snapshotTree(t, lessons)
			if err := addRelationWithDeps(lessons, "a", "related", "b", deps); err == nil || MutationOutcomeOf(err) != MutationPrePublication {
				t.Fatalf("error=%v outcome=%v", err, MutationOutcomeOf(err))
			}
			if !bytes.Equal(before, snapshotTree(t, lessons)) {
				t.Fatal("lock failure changed Lesson tree")
			}
		})
	}

	for _, typ := range []string{"related", "duplicates", "supersedes"} {
		t.Run(typ, func(t *testing.T) {
			baselineLessons := relationFixture(t, "from", "to")
			baselineFS := &faultMatrixFS{lessonFS: osLessonFS{}}
			baselineDeps := defaultRelationDeps()
			baselineDeps.fs = baselineFS
			if err := addRelationWithDeps(baselineLessons, "from", typ, "to", baselineDeps); err != nil {
				t.Fatal(err)
			}
			for failAt := 1; failAt <= baselineFS.calls; failAt++ {
				lessons := relationFixture(t, "from", "to")
				before := snapshotTree(t, lessons)
				fs := &faultMatrixFS{lessonFS: osLessonFS{}, failAt: failAt}
				deps := defaultRelationDeps()
				deps.fs = fs
				err := addRelationWithDeps(lessons, "from", typ, "to", deps)
				after := snapshotTree(t, lessons)
				if err == nil {
					op := baselineFS.trace[failAt-1]
					if op != "Remove" && op != "Close" {
						t.Fatalf("operation %d %s failure ignored", failAt, op)
					}
					continue
				}
				if !bytes.Equal(before, after) && MutationOutcomeOf(err) != MutationUncertain {
					t.Fatalf("partial relation tree reported %v: %v", MutationOutcomeOf(err), err)
				}
			}
		})
	}
}

func TestOccurrenceRemainingBehaviorAndFilesystemErrors(t *testing.T) {
	o := coverageOccurrence()
	o.Redactions = []string{""}
	if err := ValidateOccurrence(o); err == nil {
		t.Fatal("invalid redaction accepted")
	}
	if err := validateContext(map[string]any{"repository": "person@example.test"}); err == nil {
		t.Fatal("unsafe repository context accepted")
	}
	if err := validateContext(map[string]any{"git": map[string]any{"branch": nil}}); err != nil {
		t.Fatal(err)
	}
	if err := validateContextObject("git", map[string]any{"branch": "token=secret"}); err == nil {
		t.Fatal("unsafe nested context accepted")
	}
	badRef := "token=secret"
	if err := validateEvidence(Evidence{Kind: "command", Ref: &badRef}); err == nil {
		t.Fatal("unsafe evidence accepted")
	}

	root := t.TempDir()
	lessonPath := filepath.Join(root, "spec", "lessons", "rule", "README.md")
	body, err := ScaffoldCanonical(ScaffoldOptions{Slug: "rule"}, []string{"process"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(lessonPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lessonPath, body, 0o644); err != nil {
		t.Fatal(err)
	}
	opts := AddOccurrenceOptions{LessonPath: lessonPath, Summary: "", Now: time.Time{}}
	created, err := AddOccurrence(opts)
	if err != nil || created.ID == "" || created.Summary != "Lesson gap observed." {
		t.Fatalf("defaults=%#v err=%v", created, err)
	}
	if err := removeOccurrenceWithFS(filepath.Join(filepath.Dir(created.Path), "missing.json"), osLessonFS{}); err != nil {
		t.Fatal(err)
	}

	for _, raw := range []string{
		`{"context":{"nested":["person@example.test"]},"occurred_at":"2026-08-10T12:00:00Z"}`,
		`{"occurred_at":7}`,
		`{"occurred_at":"` + strings.Repeat("1", 41) + `"}`,
	} {
		if err := validateOccurrenceRaw([]byte(raw)); err == nil {
			t.Fatalf("invalid raw occurrence accepted: %s", raw)
		}
	}
}
