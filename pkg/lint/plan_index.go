package lint

import (
	"os"
	"path/filepath"

	"github.com/specscore/specscore-cli/pkg/plan"
)

// planIndexChecker keeps the canonical plans index a derived projection of the
// single-file Plan artifacts. It deliberately owns rows only; prose sections
// such as Recently Closed remain author-maintained.
type planIndexChecker struct{}

func newPlanIndexChecker() *planIndexChecker { return &planIndexChecker{} }

func (c *planIndexChecker) name() string     { return "plan-index-sync" }
func (c *planIndexChecker) severity() string { return "error" }

func (c *planIndexChecker) check(specRoot string) ([]Violation, error) {
	plansDir := filepath.Join(specRoot, "plans")
	if info, err := os.Stat(plansDir); err != nil || !info.IsDir() {
		return nil, nil
	}
	indexPath := filepath.Join(plansDir, "README.md")
	content, err := os.ReadFile(indexPath)
	if os.IsNotExist(err) {
		return nil, nil // readme-exists owns a missing index document.
	}
	if err != nil {
		return nil, err
	}
	_, changed, err := plan.IndexContent(plansDir, content)
	if err != nil {
		return []Violation{{
			File:     filepath.Join("plans", "README.md"),
			Severity: "error",
			Rule:     c.name(),
			Message:  err.Error(),
		}}, nil
	}
	if !changed {
		return nil, nil
	}
	return []Violation{{
		File:      filepath.Join("plans", "README.md"),
		Severity:  "error",
		Rule:      c.name(),
		FixTarget: c.name(),
		Message:   "plans index rows are out of sync with spec/plans/*.md (run --fix)",
	}}, nil
}

func (c *planIndexChecker) fix(specRoot string) error {
	plansDir := filepath.Join(specRoot, "plans")
	if info, err := os.Stat(plansDir); err != nil || !info.IsDir() {
		return nil
	}
	_, err := plan.SyncIndex(plansDir)
	return err
}
