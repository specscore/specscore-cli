package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/plan"
)

func TestPlanIndexChecker_FlagsAndFixesMissingPlanRow(t *testing.T) {
	root := t.TempDir()
	plansDir := filepath.Join(root, "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	index := "# Plans\n\n| Plan | Status | Source | Date | Owner |\n|---|---|---|---|---|\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(plansDir, "README.md"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	body, err := plan.Scaffold(plan.ScaffoldOptions{Slug: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plansDir, "alpha.md"), body, 0o644); err != nil {
		t.Fatal(err)
	}

	c := newPlanIndexChecker()
	vs, err := c.check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 1 || vs[0].Rule != "plan-index-sync" || vs[0].FixTarget != "plan-index-sync" {
		t.Fatalf("unexpected violations: %+v", vs)
	}
	if err := c.fix(root); err != nil {
		t.Fatal(err)
	}
	vs, err = c.check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 0 {
		t.Fatalf("fix must leave index clean: %+v", vs)
	}
	got, _ := os.ReadFile(filepath.Join(plansDir, "README.md"))
	if !strings.Contains(string(got), "| [alpha](alpha.md) | Draft | none |") {
		t.Errorf("fixed index missing plan row:\n%s", got)
	}
}
