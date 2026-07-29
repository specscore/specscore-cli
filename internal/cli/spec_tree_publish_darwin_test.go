//go:build darwin

package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func resetDarwinPublishSeams(t *testing.T) {
	t.Helper()
	mkdir, unlink := publishRecoveryMkdirTemp, publishRecoveryUnlinkat
	t.Cleanup(func() { publishRecoveryMkdirTemp, publishRecoveryUnlinkat = mkdir, unlink })
}

func TestPublishSpecTreeNoReplace_FailClosedSetupBranches(t *testing.T) {
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

	t.Run("recovery reservation failure", func(t *testing.T) {
		resetDarwinPublishSeams(t)
		parent := t.TempDir()
		publishRecoveryMkdirTemp = func(string, string) (string, error) { return "", errors.New("reserve failed") }
		if _, err := publishSpecTreeNoReplace(filepath.Join(parent, "spec"), filepath.Join(parent, "stage")); err == nil || !contains(err, "reserve failed") {
			t.Fatalf("publish error = %v", err)
		}
	})

	t.Run("recovery destination unlink failure", func(t *testing.T) {
		resetDarwinPublishSeams(t)
		parent := t.TempDir()
		publishRecoveryUnlinkat = func(int, string, int) error { return errors.New("unlink reserve failed") }
		if _, err := publishSpecTreeNoReplace(filepath.Join(parent, "spec"), filepath.Join(parent, "stage")); err == nil || !contains(err, "unlink reserve failed") {
			t.Fatalf("publish error = %v", err)
		}
		entries, err := os.ReadDir(parent)
		if err != nil || len(entries) != 1 || !entries[0].IsDir() {
			t.Fatalf("reserved recovery directory was not retained after unlink failure: entries=%v err=%v", entries, err)
		}
	})

	t.Run("claim failure preserves staged tree", func(t *testing.T) {
		parent := t.TempDir()
		stage := filepath.Join(parent, "stage")
		if err := os.Mkdir(stage, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(stage, "after.md"), []byte("after"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := publishSpecTreeNoReplace(filepath.Join(parent, "spec"), stage); err == nil || !contains(err, "claiming current") {
			t.Fatalf("publish error = %v", err)
		}
		if _, err := os.Stat(filepath.Join(stage, "after.md")); err != nil {
			t.Fatalf("staged tree was changed after claim failure: %v", err)
		}
	})
}
