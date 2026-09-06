package sourceref

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// specscore:verifies https://specscore.org/github.com/specscore/specscore-cli/spec/features/cli/code/deps#ac:typed-implementation-and-test-links-checked
func TestLocalResolver_OfflineTypedFeatureCitations(t *testing.T) {
	root := t.TempDir()
	writeResolverConfig(t, root, "github.com", "acme", "checkout")
	writeResolverFeature(t, root, "checkout", "#### REQ: totals\n\n## Acceptance Criteria\n\n### AC: total-visible\n")
	resolver := NewLocalResolver(filepath.Join(root, "spec"))
	for _, raw := range []string{
		"specscore:feature/checkout#req:totals",
		"specscore:feature/checkout#ac:total-visible",
	} {
		ref, err := ParseReference(raw)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := resolver.ValidateFeatureCitation(ref); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
	}
	missing, _ := ParseReference("specscore:feature/checkout#ac:missing")
	if _, err := resolver.ValidateFeatureCitation(missing); err == nil || !strings.Contains(err.Error(), "missing exact heading") {
		t.Fatalf("missing AC must fail: %v", err)
	}
}

// specscore:verifies https://specscore.org/github.com/specscore/specscore-cli/spec/features/cli/code/deps#ac:typed-implementation-and-test-links-checked
func TestScanFilesTypedDirectives(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "checkout.go")
	content := "package checkout\n// specscore:" + "implements feature/checkout#req:totals\n// specscore:" +
		"verifies feature/checkout#ac:total-visible\n// specscore:" + "implements feature/checkout#ac:invalid\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := ScanFiles([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	refs := result.FileRefs[path]
	if len(refs) != 3 {
		t.Fatalf("typed directive targets = %d, want 3: %+v", len(refs), refs)
	}
	if len(result.ParseErrors[path]) != 1 || !strings.Contains(result.ParseErrors[path][0].Err.Error(), "implements target must be a REQ") {
		t.Fatalf("typed relation errors = %+v", result.ParseErrors[path])
	}
}

// specscore:verifies https://specscore.org/github.com/specscore/specscore-cli/spec/features/cli/code/deps#ac:typed-implementation-and-test-links-checked
func TestFeatureAnchorExistsAfterLeadingInlineCode(t *testing.T) {
	content := "`specscore code deps` validates links.\n\n#### REQ: links\n\n### AC: links-work\n"
	for _, fragment := range []string{"req:links", "ac:links-work"} {
		if err := featureAnchorExists(content, fragment); err != nil {
			t.Fatalf("%s after leading inline code: %v", fragment, err)
		}
	}
}
