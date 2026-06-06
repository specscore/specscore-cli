package cli

import (
	"path/filepath"
	"testing"

	"github.com/specscore/specscore-cli/pkg/lint"
)

// migrateTree backfills the artifact-frontmatter-convention frontmatter into a
// fixture tree at root/spec, using the same engine the `spec migrate` verb runs.
// Shared setup helpers call it so their hand-written, frontmatter-less artifacts
// conform to the now-error frontmatter rules.
func migrateTree(t *testing.T, root string) {
	t.Helper()
	if _, err := lint.Migrate(filepath.Join(root, "spec")); err != nil {
		t.Fatalf("migrating fixture tree: %v", err)
	}
}
