package lint

import (
	"os"
	"path/filepath"
	"strings"
)

// readmeExistsChecker verifies that every spec directory has a README.md file.
type readmeExistsChecker struct{ plansDir string }

func newReadmeExistsChecker(plansDir ...string) checker {
	c := &readmeExistsChecker{}
	if len(plansDir) > 0 {
		c.plansDir = plansDir[0]
	}
	return c
}

func (c *readmeExistsChecker) name() string     { return "readme-exists" }
func (c *readmeExistsChecker) severity() string { return "error" }

func (c *readmeExistsChecker) check(specRoot string) ([]Violation, error) {
	var violations []Violation

	// `spec/ideas/seeds/` is created lazily on first sidekick-seed
	// capture (see upstream sidekick-capture Feature, REQ
	// seed-path-convention) and has no index README. Excluded from
	// the readme-exists rule.
	seedsRel := filepath.Join("ideas", "seeds")

	err := walkSpecDirs(specRoot, func(dirPath, relPath string) error {
		if c.plansDir != "" && (relPath == "plans" || strings.HasPrefix(filepath.ToSlash(relPath), "plans/")) {
			return nil
		}
		if relPath == seedsRel || isFeatureProposalsContainer(relPath) || isLessonOccurrencesContainer(relPath) {
			return nil
		}
		readmePath := filepath.Join(dirPath, "README.md")
		if _, err := os.Stat(readmePath); err != nil {
			violations = append(violations, Violation{
				File:     relPath,
				Line:     0,
				Severity: "error",
				Rule:     "readme-exists",
				Message:  "README.md not found",
			})
		}
		return nil
	})
	if err == nil && c.plansDir != "" {
		err = filepath.Walk(c.plansDir, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if !info.IsDir() {
				return nil
			}
			if path != c.plansDir && strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			if _, statErr := os.Stat(filepath.Join(path, "README.md")); statErr != nil {
				rel, _ := filepath.Rel(c.plansDir, path)
				violations = append(violations, Violation{File: filepath.Join("plans", rel), Severity: "error", Rule: c.name(), Message: "README.md not found"})
			}
			return nil
		})
	}

	return violations, err
}

// Occurrences are a reserved JSON-only child collection, not a document
// directory. Requiring a README here would contradict the append-only
// occurrence format and force a concurrent writer back into a shared file.
func isLessonOccurrencesContainer(relPath string) bool {
	parts := strings.Split(filepath.ToSlash(relPath), "/")
	return len(parts) == 3 && parts[0] == "lessons" && parts[2] == "occurrences"
}
