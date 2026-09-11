package lint

import "fmt"

// planRouteErrorChecker reports the single designated ERROR-severity finding
// for a project whose Plan routing is configured but failed to resolve
// (missing checkout, wrong origin, a nested or ambiguous mapping, a
// misplaced key, ...). It is registered unconditionally (so the rule stays
// in registry/checker parity — see CheckRegistryParity) but only ever emits
// when Options.PlanRouteError is non-empty; every other Plan-owned checker
// (readme-exists' plans walk, plan-hierarchy, plan-roi-metadata,
// plan-index-sync, P-001..P-010, and the plan-owned targets inside
// adherence-footer/status-mirror) is disabled for the same run by
// newLinter, so this is the only signal a broken route leaves behind.
//
// Lead assumption (repo-config's Plan Repository Routing section is silent
// on lint under routing): `spec lint` is not itself one of the dedicated
// Plan verbs repo-config#req:plan-route-required binds to, so a route that
// resolves cleanly, or is simply absent, must not change lint's behavior —
// but a route that IS configured and fails must never let lint silently read
// or write whatever happens to sit at the local spec/plans path, since that
// path may be exactly the stale artifact routing was configured to route
// away from (finding 1 of the PR #199 adversarial review).
type planRouteErrorChecker struct{ routeError string }

func newPlanRouteErrorChecker(routeError string) checker {
	return &planRouteErrorChecker{routeError: routeError}
}

func (c *planRouteErrorChecker) name() string     { return "plan-route-unresolved" }
func (c *planRouteErrorChecker) severity() string { return "error" }

func (c *planRouteErrorChecker) check(_ string) ([]Violation, error) {
	if c.routeError == "" {
		return nil, nil
	}
	return []Violation{{
		File:     "plans",
		Line:     0,
		Severity: "error",
		Rule:     c.name(),
		Message: fmt.Sprintf(
			"plan routing is configured but could not be resolved, so every Plan-owned lint rule was skipped (spec/plans was neither read nor written): %s",
			c.routeError),
	}}, nil
}
