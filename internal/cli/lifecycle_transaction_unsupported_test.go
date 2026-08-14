//go:build !darwin && !linux && !windows

package cli

import "testing"

func TestRunLifecycleTransactionFailsClosedOnUnsupportedPlatform(t *testing.T) {
	called := false
	if _, err := RunLifecycleTransaction(t.TempDir(), []string{"README.md"}, func(string) error {
		called = true
		return nil
	}); err == nil {
		t.Fatal("unsupported platform lifecycle transaction unexpectedly ran")
	}
	if called {
		t.Fatal("unsupported platform operation ran despite fail-closed contract")
	}
}
