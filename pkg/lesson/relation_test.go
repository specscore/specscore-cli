package lesson

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

type relationTestFS struct {
	lessonFS
	rename func(string, string) error
	open   func(string) (lessonFile, error)
}

func (fs relationTestFS) Rename(oldname, newname string) error {
	if fs.rename != nil {
		return fs.rename(oldname, newname)
	}
	return fs.lessonFS.Rename(oldname, newname)
}

func (fs relationTestFS) Open(path string) (lessonFile, error) {
	if fs.open != nil {
		return fs.open(path)
	}
	return fs.lessonFS.Open(path)
}

type relationTestFile struct {
	lessonFile
	sync func() error
}

func (f relationTestFile) Sync() error {
	if f.sync != nil {
		return f.sync()
	}
	return f.lessonFile.Sync()
}

func relationFixture(t *testing.T, slugs ...string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "spec", "lessons")
	for _, slug := range slugs {
		path := filepath.Join(dir, slug, "README.md")
		if err := os.MkdirAll(filepath.Join(filepath.Dir(path), "occurrences"), 0o755); err != nil {
			t.Fatal(err)
		}
		body, err := ScaffoldCanonical(ScaffoldOptions{Slug: slug}, []string{"process"})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestAddRelation_SupersedesRefusesCycle(t *testing.T) {
	lessons := relationFixture(t, "a", "b")
	if err := AddRelation(lessons, "a", "supersedes", "b"); err != nil {
		t.Fatal(err)
	}
	beforeA, _ := os.ReadFile(filepath.Join(lessons, "a", "README.md"))
	beforeB, _ := os.ReadFile(filepath.Join(lessons, "b", "README.md"))
	if err := AddRelation(lessons, "b", "supersedes", "a"); err == nil {
		t.Fatal("cycle must be refused before mutation")
	}
	afterA, _ := os.ReadFile(filepath.Join(lessons, "a", "README.md"))
	afterB, _ := os.ReadFile(filepath.Join(lessons, "b", "README.md"))
	if !bytes.Equal(beforeA, afterA) || !bytes.Equal(beforeB, afterB) {
		t.Fatal("cycle rejection mutated a Lesson")
	}
}

func TestAddRelation_DuplicateRetiresOnlyRetainedLessonAndIsVisibleFromTarget(t *testing.T) {
	lessons := relationFixture(t, "retained", "canonical")
	canonicalPath := filepath.Join(lessons, "canonical", "README.md")
	canonicalBefore, _ := os.ReadFile(canonicalPath)
	if err := AddRelation(lessons, "retained", "duplicates", "canonical"); err != nil {
		t.Fatal(err)
	}
	canonicalAfter, _ := os.ReadFile(canonicalPath)
	if !bytes.Equal(canonicalBefore, canonicalAfter) {
		t.Fatal("canonical duplicate target changed")
	}
	retained, _ := os.ReadFile(filepath.Join(lessons, "retained", "README.md"))
	for _, want := range []string{"status: Superseded", "**Status:** Superseded", "**Duplicate Of:** canonical", "**Superseded By:** canonical"} {
		if !strings.Contains(string(retained), want) {
			t.Fatalf("retained duplicate missing %q:\n%s", want, retained)
		}
	}
	relations, err := ListRelations(lessons, "canonical")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, relation := range relations {
		found = found || (relation.From == "retained" && relation.Type == "duplicates" && relation.To == "canonical")
	}
	if !found {
		t.Fatalf("canonical target lacks inverse visibility: %#v", relations)
	}
}

func TestAddRelation_EnforcedDuplicateAndConflictingTargetAreWriteFree(t *testing.T) {
	lessons := relationFixture(t, "a", "b", "c")
	aPath := filepath.Join(lessons, "a", "README.md")
	raw, _ := os.ReadFile(aPath)
	enforced := strings.Replace(string(raw), "status: Recorded", "status: Enforced", 1)
	enforced = strings.Replace(enforced, "**Status:** Recorded", "**Status:** Enforced", 1)
	if err := os.WriteFile(aPath, []byte(enforced), 0o644); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(aPath)
	if err := AddRelation(lessons, "a", "duplicates", "b"); err == nil {
		t.Fatal("Enforced duplicate must be rejected")
	}
	after, _ := os.ReadFile(aPath)
	if !bytes.Equal(before, after) {
		t.Fatal("Enforced duplicate rejection mutated source")
	}

	recorded := strings.Replace(enforced, "status: Enforced", "status: Recorded", 1)
	recorded = strings.Replace(recorded, "**Status:** Enforced", "**Status:** Recorded", 1)
	recorded = strings.Replace(recorded, "**Duplicate Of:** —", "**Duplicate Of:** c", 1)
	if err := os.WriteFile(aPath, []byte(recorded), 0o644); err != nil {
		t.Fatal(err)
	}
	before, _ = os.ReadFile(aPath)
	if err := AddRelation(lessons, "a", "duplicates", "b"); err == nil {
		t.Fatal("conflicting duplicate target must be rejected")
	}
	after, _ = os.ReadFile(aPath)
	if !bytes.Equal(before, after) {
		t.Fatal("conflict rejection mutated source")
	}
}

func TestAddRelation_MalformedSidecarIsFatalAndWriteFree(t *testing.T) {
	lessons := relationFixture(t, "a", "b")
	sidecar := filepath.Join(lessons, relatedRelationsFile)
	if err := os.WriteFile(sidecar, []byte(`[{"from":"a","type":"related","to":"b","ignored":true}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(sidecar)
	if err := AddRelation(lessons, "a", "related", "b"); err == nil {
		t.Fatal("malformed sidecar must be fatal")
	}
	after, _ := os.ReadFile(sidecar)
	if !bytes.Equal(before, after) {
		t.Fatal("malformed sidecar was overwritten")
	}
}

func TestAddRelation_FirstRelatedEdgeCreatesSidecar(t *testing.T) {
	lessons := relationFixture(t, "a", "b")
	if err := AddRelation(lessons, "a", "related", "b"); err != nil {
		t.Fatalf("first related edge: %v", err)
	}
	relations, err := readRelatedRelations(lessons)
	if err != nil {
		t.Fatal(err)
	}
	if len(relations) != 1 || relations[0] != (Relation{From: "a", Type: "related", To: "b"}) {
		t.Fatalf("related sidecar = %#v", relations)
	}
}

func TestRelationLockPathUsesPhysicalProjectRoot(t *testing.T) {
	lessons := relationFixture(t, "a", "b")
	root := filepath.Dir(filepath.Dir(lessons))
	alias := filepath.Join(t.TempDir(), "project-alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	physical, err := relationLockPath(lessons)
	if err != nil {
		t.Fatal(err)
	}
	viaAlias, err := relationLockPath(filepath.Join(alias, "spec", "lessons"))
	if err != nil {
		t.Fatal(err)
	}
	if physical != viaAlias {
		t.Fatalf("symlink aliases must coordinate on one advisory lock: physical=%s alias=%s", physical, viaAlias)
	}
}

func TestAddRelation_SecondPublishFailureRollsBackFirstLesson(t *testing.T) {
	lessons := relationFixture(t, "successor", "prior")
	from := filepath.Join(lessons, "successor", "README.md")
	to := filepath.Join(lessons, "prior", "README.md")
	beforeFrom, _ := os.ReadFile(from)
	beforeTo, _ := os.ReadFile(to)
	calls := 0
	deps := defaultRelationDeps()
	deps.fs = relationTestFS{lessonFS: osLessonFS{}, rename: func(old, new string) error {
		calls++
		if calls == 2 {
			return errors.New("injected second publish failure")
		}
		return os.Rename(old, new)
	}}
	if err := addRelationWithDeps(lessons, "successor", "supersedes", "prior", deps); err == nil {
		t.Fatal("expected injected failure")
	}
	afterFrom, _ := os.ReadFile(from)
	afterTo, _ := os.ReadFile(to)
	if !bytes.Equal(beforeFrom, afterFrom) || !bytes.Equal(beforeTo, afterTo) {
		t.Fatal("failed two-Lesson publication was not rolled back")
	}
}

func TestAddRelation_SecondPostRenameSyncFailureRemainsUncertain(t *testing.T) {
	lessons := relationFixture(t, "successor", "prior")
	from := filepath.Join(lessons, "successor", "README.md")
	to := filepath.Join(lessons, "prior", "README.md")
	calls := 0
	deps := defaultRelationDeps()
	deps.fs = relationTestFS{lessonFS: osLessonFS{}, open: func(path string) (lessonFile, error) {
		file, err := osLessonFS{}.Open(path)
		if err != nil {
			return nil, err
		}
		return relationTestFile{lessonFile: file, sync: func() error {
			calls++
			if calls == 2 {
				return errors.New("injected second post-rename directory sync failure")
			}
			return file.Sync()
		}}, nil
	}}

	err := addRelationWithDeps(lessons, "successor", "supersedes", "prior", deps)
	if MutationOutcomeOf(err) != MutationUncertain {
		t.Fatalf("outcome=%v err=%v; second renamed file can remain", MutationOutcomeOf(err), err)
	}
	fromBytes, readErr := os.ReadFile(from)
	if readErr != nil {
		t.Fatal(readErr)
	}
	toBytes, readErr := os.ReadFile(to)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(fromBytes), "**Supersedes:** prior") || !strings.Contains(string(toBytes), "**Superseded By:** successor") {
		t.Fatalf("both renamed files must be treated as possibly published:\nfrom=%s\nto=%s", fromBytes, toBytes)
	}
}

func TestAddRelation_AdversarialOverwriteRacesAreWriteFree(t *testing.T) {
	tests := []struct {
		name string
		typ  string
		from string
		to   string
		hook func(t *testing.T, lessons string)
	}{
		{
			name: "duplicate", typ: "duplicates", from: "a", to: "b",
			hook: func(t *testing.T, lessons string) {
				path := filepath.Join(lessons, "a", "README.md")
				raw, _ := os.ReadFile(path)
				if err := os.WriteFile(path, append(raw, []byte("\nexternal writer\n")...), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "supersedes-two-files", typ: "supersedes", from: "a", to: "b",
			hook: func(t *testing.T, lessons string) {
				path := filepath.Join(lessons, "a", "README.md")
				raw, _ := os.ReadFile(path)
				if err := os.WriteFile(path, append(raw, []byte("\nexternal writer\n")...), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "related-sidecar", typ: "related", from: "a", to: "b",
			hook: func(t *testing.T, lessons string) {
				if err := os.WriteFile(filepath.Join(lessons, relatedRelationsFile), []byte("[]\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lessons := relationFixture(t, "a", "b")
			beforeB, _ := os.ReadFile(filepath.Join(lessons, "b", "README.md"))
			fired := false
			deps := defaultRelationDeps()
			deps.beforePublish = func(kind string) error {
				if !fired && (kind == tc.typ || (tc.typ == "supersedes" && kind == "supersedes")) {
					fired = true
					tc.hook(t, lessons)
				}
				return nil
			}
			if err := addRelationWithDeps(lessons, tc.from, tc.typ, tc.to, deps); err == nil {
				t.Fatal("race must refuse publication")
			}
			if !fired {
				t.Fatal("test hook did not exercise the publication boundary")
			}
			afterB, _ := os.ReadFile(filepath.Join(lessons, "b", "README.md"))
			if !bytes.Equal(beforeB, afterB) {
				t.Fatal("racing writer caused a clobber of an unrelated target")
			}
		})
	}
}

func TestAddRelation_ValidationFailuresAreClassifiedPrePublication(t *testing.T) {
	lessons := relationFixture(t, "a", "b")
	if err := AddRelation(lessons, "a", "duplicates", "missing"); err == nil {
		t.Fatal("missing endpoint was accepted")
	} else if got := MutationOutcomeOf(err); got != MutationPrePublication {
		t.Fatalf("missing endpoint outcome = %v, want pre-publication: %v", got, err)
	}

	if err := AddRelation(lessons, "a", "supersedes", "b"); err != nil {
		t.Fatal(err)
	}
	if err := AddRelation(lessons, "b", "supersedes", "a"); err == nil {
		t.Fatal("cycle was accepted")
	} else if got := MutationOutcomeOf(err); got != MutationPrePublication {
		t.Fatalf("cycle outcome = %v, want pre-publication: %v", got, err)
	}
}

// TestRelationLockHelper is run in a subprocess by the overlapping-writer
// test below. It must wait for the parent transaction's endpoint lock rather
// than publishing concurrently.
func TestRelationLockHelper(t *testing.T) {
	if os.Getenv("SPECSCORE_RELATION_LOCK_HELPER") != "1" {
		return
	}
	lessons := os.Getenv("SPECSCORE_RELATION_LESSONS")
	_ = AddRelation(lessons, "a", "duplicates", "b")
}

func TestAddRelation_CrossProcessAdvisoryLockRefusesOverlappingWriter(t *testing.T) {
	lessons := relationFixture(t, "a", "b")
	path := filepath.Join(lessons, "a", "README.md")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	locked := make(chan struct{})
	release := make(chan struct{})
	deps := defaultRelationDeps()
	deps.lockAcquired = func() {
		close(locked)
		<-release
	}
	first := make(chan error, 1)
	go func() { first <- addRelationWithDeps(lessons, "a", "duplicates", "b", deps) }()
	<-locked

	child := exec.Command(os.Args[0], "-test.run=^TestRelationLockHelper$")
	child.Env = append(os.Environ(), "SPECSCORE_RELATION_LOCK_HELPER=1", "SPECSCORE_RELATION_LESSONS="+lessons)
	var childOutput bytes.Buffer
	child.Stdout = &childOutput
	child.Stderr = &childOutput
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	childResult := make(chan error, 1)
	go func() { childResult <- child.Wait() }()
	select {
	case err := <-childResult:
		t.Fatalf("competing process escaped endpoint serialization: %v\n%s", err, childOutput.String())
	case <-time.After(100 * time.Millisecond):
	}
	during, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, during) {
		t.Fatal("overlapping writer modified the Lesson while the first call held the lock")
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatalf("first relation writer failed: %v", err)
	}
	select {
	case err := <-childResult:
		if err != nil {
			t.Fatalf("serialized competing process failed: %v\n%s", err, childOutput.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serialized competing process did not finish after endpoint release")
	}
	if _, err := os.Stat(filepath.Join(lessons, ".relations.lock")); !os.IsNotExist(err) {
		t.Fatalf("relation lock polluted tracked Lesson tree: %v", err)
	}
}

func TestAddRelation_HoldsEndpointLocksThroughPostMutation(t *testing.T) {
	lessons := relationFixture(t, "retained", "canonical")
	root := filepath.Dir(filepath.Dir(lessons))
	postEntered := make(chan struct{})
	releasePost := make(chan struct{})
	relationResult := make(chan error, 1)
	go func() {
		relationResult <- AddRelationWithPostMutation(lessons, "retained", "duplicates", "canonical", func() error {
			close(postEntered)
			<-releasePost
			return nil
		})
	}()
	<-postEntered

	lifecycleResult := make(chan error, 1)
	go func() {
		_, err := ChangeStatus(ChangeStatusOptions{SpecRoot: root, Slug: "retained", To: lifecycle.LessonStated, PostMutation: func() error { return nil }})
		lifecycleResult <- err
	}()
	select {
	case err := <-lifecycleResult:
		t.Fatalf("lifecycle escaped the relation endpoint lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(releasePost)
	if err := <-relationResult; err != nil {
		t.Fatal(err)
	}
	if err := <-lifecycleResult; err == nil {
		t.Fatal("relation and incompatible lifecycle mutation both succeeded")
	}
	retained, err := Parse(filepath.Join(lessons, "retained", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if retained.Status != "Superseded" || retained.DuplicateOf != "canonical" {
		t.Fatalf("serial relation state was lost: %#v", retained)
	}
}
