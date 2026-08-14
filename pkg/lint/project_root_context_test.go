package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLifecycleLintRulesDoNotDeriveProjectRoot(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") || entry.Name() == "lint.go" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(".", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "filepath.Dir(specRoot)") {
			t.Fatalf("%s derives a project root from specRoot; inject lint.Options.ProjectRoot instead", entry.Name())
		}
	}
}
