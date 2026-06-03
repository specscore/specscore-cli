package lint

import (
	"os"
	"path/filepath"
	"testing"
)

// Task 7 — the single-repository special case: a Capability and its CLI and Web
// Implementations co-located under one feature folder. `spec lint` (scoped to
// the capability-family rules) reports no violations, and each Implementation's
// same-repo "**Implements:**" reference resolves to the co-located Capability
// (capability-and-platform-implementations#ac:colocated-layout-valid).
func TestColocatedLayout_Valid(t *testing.T) {
	tmp := t.TempDir()

	// Capability at spec/features/dashboards.
	capability := "# Feature: Dashboards\n\n**Status:** Approved\n\n" +
		"## Implementation Matrix\n\n" +
		"| Platform | Status | Brief | Link |\n" +
		"| --- | --- | --- | --- |\n" +
		"| CLI | Partial | Data-only, no graph | specscore:feature/dashboards/cli |\n" +
		"| Web | Full | Rich interactive graph | specscore:feature/dashboards/web |\n"
	writeColocatedReadme(t, tmp, "dashboards", capability)

	// CLI and Web Implementations co-located under the same feature folder.
	cli := "# Feature: Dashboards (CLI)\n\n**Status:** Approved\n" +
		"**Implements:** specscore:feature/dashboards\n\n## Summary\n"
	web := "# Feature: Dashboards (Web)\n\n**Status:** Approved\n" +
		"**Implements:** specscore:feature/dashboards\n\n## Summary\n"
	writeColocatedReadme(t, tmp, filepath.Join("dashboards", "cli"), cli)
	writeColocatedReadme(t, tmp, filepath.Join("dashboards", "web"), web)

	capabilityRules := []string{"implements-reference", "implementation-matrix", "other-platforms-links-only"}
	res, err := LintWithResult(Options{SpecRoot: tmp, Rules: capabilityRules})
	if err != nil {
		t.Fatalf("LintWithResult: %v", err)
	}
	if len(res.Violations) != 0 {
		t.Fatalf("expected a lint-clean co-located layout, got %d violations: %+v", len(res.Violations), res.Violations)
	}

	// Each Implementation's reference resolves same-repo to the co-located
	// Capability (no cross-repo suffix, slug points at the Capability folder).
	for _, impl := range []string{cli, web} {
		role := classifyFeatureRole(impl)
		if !role.isImplementation {
			t.Fatalf("expected an Implementation role for:\n%s", impl)
		}
		ref, err := parseImplementsRef(role.implementsLine)
		if err != nil {
			t.Fatalf("parseImplementsRef: %v", err)
		}
		if ref.crossRepo {
			t.Errorf("expected same-repo reference, got cross-repo for:\n%s", impl)
		}
		if ref.featureSlug() != "dashboards" {
			t.Errorf("featureSlug = %q, want dashboards", ref.featureSlug())
		}
		target := filepath.Join(tmp, "features", ref.featureSlug(), "README.md")
		data, readErr := os.ReadFile(target)
		if readErr != nil {
			t.Fatalf("co-located Capability not found at %s: %v", target, readErr)
		}
		if !classifyFeatureRole(string(data)).isCapability {
			t.Errorf("reference target at %s is not a Capability", target)
		}
	}
}

// writeColocatedReadme writes specRoot/features/<relDir>/README.md.
func writeColocatedReadme(t *testing.T, specRoot, relDir, content string) {
	t.Helper()
	dir := filepath.Join(specRoot, "features", relDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
