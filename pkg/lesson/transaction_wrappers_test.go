package lesson

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicTransactionWrappers(t *testing.T) {
	root := t.TempDir()
	called := false
	if err := WithMutationLock(root, "wrapped", func() error { called = true; return nil }); err != nil || !called {
		t.Fatalf("WithMutationLock = %v, called=%v", err, called)
	}
	if err := WithMutationLock(root, "Bad Slug", func() error { return nil }); err == nil || MutationOutcomeOf(err) != MutationPrePublication {
		t.Fatalf("invalid lock slug = %v, outcome=%v", err, MutationOutcomeOf(err))
	}
	called = false
	if err := WithMutationLocks(root, []string{"wrapped-b", "wrapped-a", "wrapped-b"}, func() error { called = true; return nil }); err != nil || !called {
		t.Fatalf("WithMutationLocks = %v, called=%v", err, called)
	}
	if err := WithMutationLocks(root, []string{"wrapped", "Bad Slug"}, func() error { return nil }); err == nil || MutationOutcomeOf(err) != MutationPrePublication {
		t.Fatalf("invalid multi-lock slug = %v, outcome=%v", err, MutationOutcomeOf(err))
	}
	path := filepath.Join(root, "README.md")
	if err := os.WriteFile(path, []byte("before\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := RewriteFileAtomic(path, []byte("after\n")); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "after\n" {
		t.Fatalf("RewriteFileAtomic = %q, %v", got, err)
	}
}

func TestAddRelationPostMutationAndNewValidationBranches(t *testing.T) {
	lessons := relationFixture(t, "a", "b")
	if err := AddRelationWithPostMutation(lessons, "a", "bogus", "b", nil); err == nil || MutationOutcomeOf(err) != MutationPrePublication {
		t.Fatalf("invalid relation = %v, outcome=%v", err, MutationOutcomeOf(err))
	}
	postErr := errors.New("post mutation")
	if err := AddRelationWithPostMutation(lessons, "a", "related", "b", func() error { return postErr }); !errors.Is(err, postErr) || MutationOutcomeOf(err) != MutationUncertain {
		t.Fatalf("post mutation error = %v, outcome=%v", err, MutationOutcomeOf(err))
	}

	deps := defaultRelationDeps()
	deps.abs = func(string) (string, error) { return "", errors.New("abs") }
	if err := withRelationLockWithDeps(lessons, deps, func() error { return nil }); err == nil || MutationOutcomeOf(err) != MutationPrePublication {
		t.Fatalf("relation root error = %v, outcome=%v", err, MutationOutcomeOf(err))
	}

	bPath := filepath.Join(lessons, "b", "README.md")
	raw, err := os.ReadFile(bPath)
	if err != nil {
		t.Fatal(err)
	}
	withdrawn := strings.Replace(string(raw), "status: Recorded", "status: Withdrawn", 1)
	withdrawn = strings.Replace(withdrawn, "**Status:** Recorded", "**Status:** Withdrawn", 1)
	if err := os.WriteFile(bPath, []byte(withdrawn), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AddRelation(lessons, "a", "supersedes", "b"); err == nil || !strings.Contains(err.Error(), "cannot transition") {
		t.Fatalf("terminal supersedes target = %v", err)
	}
}

func TestPublicInspectionAndRelationTransactionWrappersClassifyNoops(t *testing.T) {
	root := t.TempDir()
	lessonsDir := filepath.Join(root, "spec", "lessons")
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "legacy.md")
	if err := os.WriteFile(source, []byte("## L1 — reviewed rule\n\n**Status:** Recorded\n\ntext\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inv := inventoryLegacyForApply(t, source)
	mapping := legacyMapping(inv, reviewedNew("L1#1", "reviewed-rule"))
	inspection, err := InspectLegacyApply(lessonsDir, []string{"process"}, inv, mapping)
	if err != nil || !inspection.MutationRequired {
		t.Fatalf("initial legacy inspection = %#v, %v", inspection, err)
	}
	if _, err := InspectLegacyApply(lessonsDir, nil, inv, mapping); err == nil {
		t.Fatal("legacy inspection accepted an empty classification vocabulary")
	}
	if _, err := ApplyLegacy(lessonsDir, []string{"process"}, inv, mapping); err != nil {
		t.Fatal(err)
	}
	inspection, err = InspectLegacyApply(lessonsDir, []string{"process"}, inv, mapping)
	if err != nil || inspection.MutationRequired || len(inspection.Result.Skipped) != 1 {
		t.Fatalf("completed legacy inspection = %#v, %v", inspection, err)
	}
	if err := os.Remove(filepath.Join(lessonsDir, "reviewed-rule", ".legacy-import-owner")); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectLegacyApply(lessonsDir, []string{"process"}, inv, mapping); err == nil {
		t.Fatal("legacy inspection accepted a completed target without its ownership marker")
	}

	for _, typ := range []string{"related", "duplicates", "supersedes"} {
		t.Run(typ, func(t *testing.T) {
			dir := relationFixture(t, "from", "to")
			beforeCalls, postCalls := 0, 0
			hooks := RelationTransactionHooks{
				BeforeMutation: func() error { beforeCalls++; return nil },
				PostMutation:   func() error { postCalls++; return nil },
			}
			mutated, err := AddRelationTransaction(dir, "from", typ, "to", hooks)
			if err != nil || !mutated || beforeCalls != 1 || postCalls != 1 {
				t.Fatalf("first transaction = mutated %v, before %d, post %d, err %v", mutated, beforeCalls, postCalls, err)
			}
			mutated, err = AddRelationTransaction(dir, "from", typ, "to", hooks)
			if err != nil || mutated || beforeCalls != 1 || postCalls != 1 {
				t.Fatalf("second transaction = mutated %v, before %d, post %d, err %v", mutated, beforeCalls, postCalls, err)
			}
		})
	}
}

func TestRelationTransactionsRefusePreparationBeforePublication(t *testing.T) {
	prepareErr := errors.New("prepare")
	for _, typ := range []string{"related", "duplicates", "supersedes"} {
		t.Run(typ, func(t *testing.T) {
			dir := relationFixture(t, "from", "to")
			beforeFrom, err := os.ReadFile(filepath.Join(dir, "from", "README.md"))
			if err != nil {
				t.Fatal(err)
			}
			beforeTo, err := os.ReadFile(filepath.Join(dir, "to", "README.md"))
			if err != nil {
				t.Fatal(err)
			}
			mutated, err := AddRelationTransaction(dir, "from", typ, "to", RelationTransactionHooks{
				BeforeMutation: func() error { return prepareErr },
			})
			if mutated || !errors.Is(err, prepareErr) || MutationOutcomeOf(err) != MutationPrePublication {
				t.Fatalf("preparation failure = mutated %v, outcome %v, err %v", mutated, MutationOutcomeOf(err), err)
			}
			afterFrom, readErr := os.ReadFile(filepath.Join(dir, "from", "README.md"))
			if readErr != nil {
				t.Fatal(readErr)
			}
			afterTo, readErr := os.ReadFile(filepath.Join(dir, "to", "README.md"))
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(afterFrom) != string(beforeFrom) || string(afterTo) != string(beforeTo) {
				t.Fatal("preparation failure changed a Lesson")
			}
			if typ == "related" {
				if _, statErr := os.Stat(filepath.Join(dir, relatedRelationsFile)); !os.IsNotExist(statErr) {
					t.Fatalf("preparation failure published a relation sidecar: %v", statErr)
				}
			}
		})
	}
}
