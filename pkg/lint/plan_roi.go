package lint

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// planROIChecker validates ROI metadata values in plan README headers.
// When present, Effort must be S/M/L/XL and Impact must be low/medium/high/critical.
type planROIChecker struct {
	plansDir string
	// routeBroken, when true, means Plan routing is configured but failed to
	// resolve: check() must skip entirely, without ever touching the local
	// spec/plans tree that an empty plansDir would otherwise default to
	// (finding 1 / repo-config#req:plan-route-required).
	routeBroken bool
}

func newPlanROIChecker(plansDir ...string) checker {
	c := &planROIChecker{}
	if len(plansDir) > 0 {
		c.plansDir = plansDir[0]
	}
	return c
}

// newPlanROICheckerRouteError returns a plan-roi-metadata checker configured
// for a configured-but-broken Plan route: see routeBroken.
func newPlanROICheckerRouteError() checker {
	return &planROIChecker{routeBroken: true}
}

func (c *planROIChecker) name() string     { return "plan-roi-metadata" }
func (c *planROIChecker) severity() string { return "warning" }

// routeErrorChecker implements planOwnedChecker: see linter.go's
// registerPlanOwned, the ONE place that decides which checker to register
// when Plan routing is configured but broken.
func (c *planROIChecker) routeErrorChecker() checker { return newPlanROICheckerRouteError() }

var validEffort = map[string]bool{
	"S": true, "M": true, "L": true, "XL": true,
}

var validImpact = map[string]bool{
	"low": true, "medium": true, "high": true, "critical": true,
}

func (c *planROIChecker) check(specRoot string) ([]Violation, error) {
	if c.routeBroken {
		return nil, nil
	}
	plansDir := effectivePlansDir(specRoot, c.plansDir)
	info, err := os.Stat(plansDir)
	if err != nil || !info.IsDir() {
		return nil, nil
	}

	var violations []Violation

	err = filepath.Walk(plansDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !fi.IsDir() {
			return nil
		}
		if strings.HasPrefix(fi.Name(), ".") {
			return filepath.SkipDir
		}
		// Skip the plans/ directory itself (its README is the index).
		if path == plansDir {
			return nil
		}

		readmePath := filepath.Join(path, "README.md")
		if _, statErr := os.Stat(readmePath); statErr != nil {
			return nil
		}

		relReadme, _ := filepath.Rel(specRoot, readmePath)

		v, scanErr := scanROIMetadata(readmePath, relReadme)
		if scanErr != nil {
			return scanErr
		}
		violations = append(violations, v...)

		return nil
	})

	if err != nil {
		return nil, err
	}

	return violations, nil
}

// scanROIMetadata reads the header of a plan README (lines before the first ## heading)
// and validates Effort/Impact values if present.
func scanROIMetadata(readmePath, relPath string) ([]Violation, error) {
	file, err := os.Open(readmePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	var violations []Violation
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Stop at the first ## heading — we only check the header.
		if strings.HasPrefix(trimmed, "## ") {
			break
		}

		if value, ok := strings.CutPrefix(trimmed, "**Effort:**"); ok {
			value = strings.TrimSpace(value)
			if !validEffort[value] {
				violations = append(violations, Violation{
					File:     relPath,
					Line:     lineNum,
					Severity: "warning",
					Rule:     "plan-roi-metadata",
					Message:  "Effort value must be one of S, M, L, XL; got " + value,
				})
			}
		}

		if value, ok := strings.CutPrefix(trimmed, "**Impact:**"); ok {
			value = strings.TrimSpace(value)
			if !validImpact[value] {
				violations = append(violations, Violation{
					File:     relPath,
					Line:     lineNum,
					Severity: "warning",
					Rule:     "plan-roi-metadata",
					Message:  "Impact value must be one of low, medium, high, critical; got " + value,
				})
			}
		}
	}

	return violations, nil
}
