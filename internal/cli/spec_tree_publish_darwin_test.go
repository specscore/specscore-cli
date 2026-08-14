//go:build darwin

package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublishSpecTreeNoReplace_AtomicExchangeBranches(t *testing.T) {
	t.Run("stage must be sibling", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "spec")
		if _, err := publishSpecTreeNoReplace(root, t.TempDir()); err == nil || !contains(err, "must be a sibling") {
			t.Fatalf("publish error = %v", err)
		}
	})

	t.Run("parent open failure", func(t *testing.T) {
		parent := filepath.Join(t.TempDir(), "missing")
		if _, err := publishSpecTreeNoReplace(filepath.Join(parent, "spec"), filepath.Join(parent, "stage")); err == nil || !contains(err, "opening spec parent") {
			t.Fatalf("publish error = %v", err)
		}
	})

	t.Run("exchange failure preserves staged tree", func(t *testing.T) {
		parent := t.TempDir()
		stage := filepath.Join(parent, "stage")
		if err := os.Mkdir(stage, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(stage, "after.md"), []byte("after"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := publishSpecTreeNoReplace(filepath.Join(parent, "spec"), stage); err == nil || !contains(err, "atomically exchanging") {
			t.Fatalf("publish error = %v", err)
		}
		if _, err := os.Stat(filepath.Join(stage, "after.md")); err != nil {
			t.Fatalf("staged tree was changed after claim failure: %v", err)
		}
	})

	t.Run("exchanges whole trees without a missing spec window", func(t *testing.T) {
		parent := t.TempDir()
		spec, stage := filepath.Join(parent, "spec"), filepath.Join(parent, "stage")
		for _, path := range []string{spec, stage} {
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(spec, "before.md"), []byte("before"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(stage, "after.md"), []byte("after"), 0o644); err != nil {
			t.Fatal(err)
		}
		oldPath, err := publishSpecTreeNoReplace(spec, stage)
		if err != nil {
			t.Fatal(err)
		}
		if oldPath != stage {
			t.Fatalf("old tree path = %q, want stage path %q", oldPath, stage)
		}
		if got, err := os.ReadFile(filepath.Join(spec, "after.md")); err != nil || string(got) != "after" {
			t.Fatalf("published tree = %q, %v", got, err)
		}
		if got, err := os.ReadFile(filepath.Join(stage, "before.md")); err != nil || string(got) != "before" {
			t.Fatalf("prior tree = %q, %v", got, err)
		}
	})
}
