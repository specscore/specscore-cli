package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func contains(err error, want string) bool {
	return err != nil && strings.Contains(err.Error(), want)
}

func findLifecycleRecoveryTree(t *testing.T, parent string) string {
	t.Helper()
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() && (strings.HasPrefix(entry.Name(), ".specscore-lifecycle-recovery-") || strings.HasPrefix(entry.Name(), ".specscore-lint-stage-")) {
			return filepath.Join(parent, entry.Name())
		}
	}
	return ""
}
