package lint

import (
	"fmt"

	"github.com/specscore/specscore-cli/pkg/config"
	"github.com/specscore/specscore-cli/pkg/projectdef"
)

// configScopeChecker enforces that user-scoped config keys (per-user/per-machine
// values such as recaps.repo or journal.repo) are not set in the committed
// specscore.yaml. They belong in specscore.local.yaml or ~/.specscore.yaml.
type configScopeChecker struct{ projectRoot string }

func newConfigScopeChecker(projectRoots ...string) *configScopeChecker {
	var projectRoot string
	if len(projectRoots) > 0 {
		projectRoot = projectRoots[0]
	}
	return &configScopeChecker{projectRoot: projectRoot}
}

func (c *configScopeChecker) name() string     { return "config-user-scoped-key" }
func (c *configScopeChecker) severity() string { return "error" }

func (c *configScopeChecker) check(specRoot string) ([]Violation, error) {
	scopeViolations, err := config.CheckCommittedScope(lintProjectRoot(c.projectRoot, specRoot))
	if err != nil {
		// A missing or malformed specscore.yaml is surfaced by other rules;
		// this rule stays silent rather than double-reporting.
		return nil, nil
	}
	var violations []Violation
	for _, v := range scopeViolations {
		violations = append(violations, Violation{
			File:     projectdef.SpecConfigFile,
			Line:     0,
			Severity: "error",
			Rule:     "config-user-scoped-key",
			Message: fmt.Sprintf(
				"%s is a per-user/per-machine value and MUST NOT be set in committed %s; move it to %s or ~/%s",
				v.Key, config.ProjectFile, config.LocalFile, config.HomeFile,
			),
		})
	}
	return violations, nil
}
