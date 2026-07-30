//go:build windows

package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunLifecycleTransactionFailsClosedOnWindowsBeforeOperation(t *testing.T) {
	project := t.TempDir()
	spec := filepath.Join(project, "spec", "README.md")
	if err := os.MkdirAll(filepath.Dir(spec), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	called := false
	if _, err := RunLifecycleTransaction(project, func(string) error {
		called = true
		return os.WriteFile(spec, []byte("after\n"), 0o600)
	}); err == nil {
		t.Fatal("Windows lifecycle transaction unexpectedly ran")
	}
	if called {
		t.Fatal("Windows lifecycle operation ran despite fail-closed contract")
	}
	content, err := os.ReadFile(spec)
	if err != nil || string(content) != "before\n" {
		t.Fatalf("live tree changed: %q, %v", content, err)
	}
}
