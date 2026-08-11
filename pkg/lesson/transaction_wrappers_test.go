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
