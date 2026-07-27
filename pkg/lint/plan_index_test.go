package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/plan"
)

func TestPlanIndexChecker_FlagsAndFixesPlanRowDrift(t *testing.T) {
	root := t.TempDir()
	plansDir := filepath.Join(root, "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	index := "# Plans\n\n| Plan | Status | Source | Date | Owner |\n|---|---|---|---|---|\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(plansDir, "README.md"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, slug := range []string{"alpha", "beta"} {
		body, err := plan.Scaffold(plan.ScaffoldOptions{Slug: slug})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(plansDir, slug+".md"), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// alpha is missing; beta is both stale and duplicated. The checker reports
	// one derived-index drift violation and the fixer reconstructs the rows.
	stale := "# Plans\n\n| Plan | Status | Source | Date | Owner |\n|---|---|---|---|---|\n| [beta](beta.md) | Approved | none | — | — |\n| [beta](beta.md) | Approved | none | — | — |\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(plansDir, "README.md"), []byte(stale), 0o644); err != nil {
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
	content := string(got)
	for _, slug := range []string{"alpha", "beta"} {
		if strings.Count(content, "["+slug+"]("+slug+".md)") != 1 {
			t.Errorf("fixed index must contain exactly one %s row:\n%s", slug, content)
		}
	}
	if strings.Contains(content, "| [beta](beta.md) | Approved |") {
		t.Errorf("fixed index retained stale beta metadata:\n%s", content)
	}
}

func TestPlanIndexChecker_AbsentAndMalformedIndexes(t *testing.T) {
	c := newPlanIndexChecker()
	if c.name() != "plan-index-sync" || c.severity() != "error" {
		t.Fatalf("unexpected checker identity: name=%q severity=%q", c.name(), c.severity())
	}

	root := t.TempDir()
	if vs, err := c.check(root); err != nil || len(vs) != 0 {
		t.Fatalf("absent plans dir: violations=%+v err=%v", vs, err)
	}
	if err := c.fix(root); err != nil {
		t.Fatalf("fix must ignore an absent plans dir: %v", err)
	}

	plansDir := filepath.Join(root, "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if vs, err := c.check(root); err != nil || len(vs) != 0 {
		t.Fatalf("missing index is owned by readme-exists, got violations=%+v err=%v", vs, err)
	}
	malformed := "# Plans\n\n| Plan | Status | Source | Date | Owner |\n| not a separator |\n"
	if err := os.WriteFile(filepath.Join(plansDir, "README.md"), []byte(malformed), 0o644); err != nil {
		t.Fatal(err)
	}
	vs, err := c.check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 1 || !strings.Contains(vs[0].Message, "canonical table") {
		t.Fatalf("malformed index must surface as one lint violation: %+v", vs)
	}
}
