package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Verifies: cli/rules#ac:catalog-deterministic-render
func TestRenderCatalogDeterministic(t *testing.T) {
	first := RenderCatalog()
	second := RenderCatalog()
	if first != second {
		t.Fatalf("RenderCatalog() is not deterministic: two calls produced different output")
	}
}

// Verifies: cli/rules#ac:catalog-at-canonical-path
func TestCatalogPathCanonical(t *testing.T) {
	if CatalogPath != "docs/lint-rules.md" {
		t.Fatalf("CatalogPath = %q, want %q", CatalogPath, "docs/lint-rules.md")
	}
}

// Verifies: cli/rules#ac:catalog-at-canonical-path
func TestRenderCatalogGroupedByFamily(t *testing.T) {
	out := RenderCatalog()

	// Family headings appear as level-2 sections.
	for _, family := range []string{"core", "idea", "decision", "entity"} {
		if !strings.Contains(out, "## "+family+"\n") {
			t.Errorf("rendered catalog missing family heading %q", family)
		}
	}

	// A known rule shows its id and description.
	if !strings.Contains(out, "readme-exists") {
		t.Errorf("rendered catalog missing rule id %q", "readme-exists")
	}
	if !strings.Contains(out, "Requires a README.md in every spec directory.") {
		t.Errorf("rendered catalog missing description for readme-exists")
	}

	// The known rule's id and its description live in the same line, under core.
	coreIdx := strings.Index(out, "## core\n")
	ideaIdx := strings.Index(out, "## idea\n")
	if coreIdx < 0 || ideaIdx < 0 {
		t.Fatalf("expected both core and idea sections")
	}
	if coreIdx > ideaIdx {
		t.Errorf("families not in sorted order: core (%d) should precede idea (%d)", coreIdx, ideaIdx)
	}
	coreSection := out[coreIdx:ideaIdx]
	if !strings.Contains(coreSection, "readme-exists") {
		t.Errorf("readme-exists not listed under core family")
	}
}

func TestRenderCatalogTrailingNewline(t *testing.T) {
	out := RenderCatalog()
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("rendered catalog must end with a trailing newline")
	}
	if strings.HasSuffix(out, "\n\n") {
		t.Errorf("rendered catalog must end with a single trailing newline")
	}
}

func TestWriteCatalog(t *testing.T) {
	tmp := t.TempDir()
	if err := WriteCatalog(tmp); err != nil {
		t.Fatalf("WriteCatalog() error: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(tmp, CatalogPath))
	if err != nil {
		t.Fatalf("reading written catalog: %v", err)
	}
	if string(got) != RenderCatalog() {
		t.Errorf("written file bytes do not match RenderCatalog() output")
	}
}
