package lint

import (
	"os"
	"path/filepath"

	"github.com/specscore/specscore-cli/pkg/plan"
)

// planIndexChecker keeps the canonical plans index a derived projection of the
// single-file Plan artifacts. It deliberately owns rows only; prose sections
// such as Recently Closed remain author-maintained.
type planIndexChecker struct {
	plansDir string
	// routeBroken, when true, means Plan routing is configured but failed to
	// resolve: check() and fix() must both no-op, without ever touching the
	// local spec/plans tree that an empty plansDir would otherwise default
	// to (finding 1 / repo-config#req:plan-route-required).
	routeBroken bool
}

func newPlanIndexChecker(plansDir ...string) *planIndexChecker {
	c := &planIndexChecker{}
	if len(plansDir) > 0 {
		c.plansDir = plansDir[0]
	}
	return c
}

// newPlanIndexCheckerRouteError returns a plan-index-sync checker configured
// for a configured-but-broken Plan route: see routeBroken.
func newPlanIndexCheckerRouteError() *planIndexChecker {
	return &planIndexChecker{routeBroken: true}
}

func (c *planIndexChecker) name() string     { return "plan-index-sync" }
func (c *planIndexChecker) severity() string { return "error" }

func (c *planIndexChecker) check(specRoot string) ([]Violation, error) {
	if c.routeBroken {
		return nil, nil
	}
	plansDir := effectivePlansDir(specRoot, c.plansDir)
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
	if c.routeBroken {
		return nil
	}
	plansDir := effectivePlansDir(specRoot, c.plansDir)
	if info, err := os.Stat(plansDir); err != nil || !info.IsDir() {
		return nil
	}
	// A missing plans/README.md is not this fixer's concern (readme-exists owns
	// a missing index document, mirroring check() above) — a Plan mutation that
	// runs before the namespace's index has been materialized must not hard-fail
	// the whole post-mutation lint pass over an absent derived file.
	if _, err := os.Stat(filepath.Join(plansDir, "README.md")); os.IsNotExist(err) {
		return nil
	}
	_, err := plan.SyncIndex(plansDir)
	return err
}
