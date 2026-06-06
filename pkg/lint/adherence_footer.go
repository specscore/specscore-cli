package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// docTypeTarget describes one document type subject to the adherence-footer
// check. The walk function enumerates the consumer paths defined for that
// Kind by the document types registry.
//
// statusBearing records the Status-concept classification mandated by
// artifact-frontmatter-convention REQ:status-concept-by-type: status-bearing
// types (Idea, Feature, Plan, Task, Decision) MUST mirror their body
// `**Status:**` into frontmatter `status:`; status-less types (the *-index
// READMEs, scenarios, properties, entities) MUST NOT. This per-type flag is the
// single source of truth the status-mirror rule consumes. (Sidekick-seeds are
// also status-bearing but have their own checker and are gated on the
// seed-lifecycle CLI, so they are not represented here.)
type docTypeTarget struct {
	description   string
	url           string
	severity      string
	statusBearing bool
	walk          func(specRoot string, fn func(path string, content []byte)) error
}

// docTypeTargets mirrors the Document-Kind and Index-Kind rows in the
// document types registry at spec/features/README.md. New Document/Index
// Kinds must be added here.
//
// Severity policy during the MVP rollout: feature READMEs stay at "error"
// (pre-existing behavior), new consumer-layer checks ship at "warn" per
// the adherence-footer-and-doc-type-registry Idea's MVP gate, then
// transition to "error" after a cycle of clean runs.
var docTypeTargets = []docTypeTarget{
	{
		description:   "feature README",
		url:           "https://specscore.md/feature-specification",
		severity:      "error",
		statusBearing: true,
		walk:          walkFeatureReadmesExcludingIndex,
	},
	{
		description: "features-index README",
		url:         "https://specscore.md/features-index-specification",
		severity:    "warn",
		walk:        walkFeaturesIndex,
	},
	{
		description:   "Idea file",
		url:           "https://specscore.md/idea-specification",
		severity:      "warn",
		statusBearing: true,
		walk:          walkIdeaFiles,
	},
	{
		description: "plans-index README",
		url:         "https://specscore.md/plans-index-specification",
		severity:    "warn",
		walk:        walkPlansIndex,
	},
	{
		description: "ideas-index README",
		url:         "https://specscore.md/ideas-index-specification",
		severity:    "warn",
		walk:        walkIdeasIndex,
	},
	{
		description:   "Plan README",
		url:           "https://specscore.md/plan-specification",
		severity:      "warn",
		statusBearing: true,
		walk:          walkPlanReadmes,
	},
	{
		description:   "Task README",
		url:           "https://specscore.md/task-specification",
		severity:      "warn",
		statusBearing: true,
		walk:          walkTaskReadmes,
	},
	{
		description: "Scenario file",
		url:         "https://specscore.md/scenario-specification",
		severity:    "warn",
		walk:        walkScenarioFiles,
	},
	{
		description: "scenarios-index README",
		url:         "https://specscore.md/scenarios-index-specification",
		severity:    "warn",
		walk:        walkScenariosIndexes,
	},
	{
		description:   "Decision file",
		url:           "https://specscore.md/decision-specification",
		severity:      "error",
		statusBearing: true,
		walk:          walkDecisionFiles,
	},
	{
		description: "decisions-index README",
		url:         "https://specscore.md/decisions-index-specification",
		severity:    "warn",
		walk:        walkDecisionsIndex,
	},
	{
		description: "Property file",
		url:         "https://specscore.md/property-specification",
		severity:    "warn",
		walk:        walkPropertyFiles,
	},
	{
		description: "Entity file",
		url:         "https://specscore.md/entity-specification",
		severity:    "warn",
		walk:        walkEntityFiles,
	},
}

// adherenceFooterChecker verifies that every SpecScore document of a
// Document or Index Kind carries the adherence footer URL corresponding
// to its document type, as required by the Adherence Footer feature.
type adherenceFooterChecker struct{}

var adherenceFooterURLPattern = regexp.MustCompile(`^https://specscore\.md/[a-z0-9-]+-specification$`)

func newAdherenceFooterChecker() checker {
	return &adherenceFooterChecker{}
}

func (c *adherenceFooterChecker) name() string     { return "adherence-footer" }
func (c *adherenceFooterChecker) severity() string { return "error" }

func (c *adherenceFooterChecker) check(specRoot string) ([]Violation, error) {
	var violations []Violation
	for _, t := range docTypeTargets {
		target := t
		err := target.walk(specRoot, func(path string, content []byte) {
			if strings.Contains(string(content), target.url) {
				return
			}
			relPath, _ := filepath.Rel(specRoot, path)
			violations = append(violations, Violation{
				File:     relPath,
				Line:     0,
				Severity: target.severity,
				Rule:     "adherence-footer",
				Message:  fmt.Sprintf("missing required adherence footer: URL %q not found in %s", target.url, target.description),
			})
		})
		if err != nil {
			return nil, err
		}
	}
	return violations, nil
}

// fix enforces a canonical adherence footer per document type.
// If a trailing adherence-footer block exists with a wrong
// https://specscore.md/*-specification URL, the URL is rewritten in-place.
// Otherwise, when the required URL is missing, the canonical footer is
// appended.
func (c *adherenceFooterChecker) fix(specRoot string) error {
	for _, t := range docTypeTargets {
		target := t
		var writeErr error
		err := target.walk(specRoot, func(path string, content []byte) {
			if writeErr != nil {
				return
			}
			s := string(content)

			rewritten, replaced := rewriteTrailingAdherenceFooterURL(s, target.url)
			if replaced {
				if rewritten != s {
					if err := os.WriteFile(path, []byte(rewritten), 0o644); err != nil {
						writeErr = err
					}
				}
				return
			}

			if strings.Contains(s, target.url) {
				return
			}
			if !strings.HasSuffix(s, "\n") {
				s += "\n"
			}
			s += "\n---\n*This document follows the " + target.url + "*\n"
			if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
				writeErr = err
			}
		})
		if err != nil {
			return err
		}
		if writeErr != nil {
			return writeErr
		}
	}
	return nil
}

func rewriteTrailingAdherenceFooterURL(content, targetURL string) (string, bool) {
	body, trailingNewlines := splitTrailingNewlines(content)
	lines := strings.Split(body, "\n")
	if len(lines) < 2 {
		return content, false
	}
	if lines[len(lines)-2] != "---" {
		return content, false
	}

	const prefix = "*This document follows the "
	const suffix = "*"

	last := lines[len(lines)-1]
	if !strings.HasPrefix(last, prefix) || !strings.HasSuffix(last, suffix) {
		return content, false
	}

	currentURL := strings.TrimSuffix(strings.TrimPrefix(last, prefix), suffix)
	if !adherenceFooterURLPattern.MatchString(currentURL) {
		return content, false
	}

	lines[len(lines)-1] = prefix + targetURL + suffix
	return strings.Join(lines, "\n") + trailingNewlines, true
}

func splitTrailingNewlines(s string) (body, trailingNewlines string) {
	i := len(s)
	for i > 0 && s[i-1] == '\n' {
		i--
	}
	return s[:i], s[i:]
}

// walkIdeaFiles invokes fn for every Idea file under specRoot/ideas/*.md,
// skipping README.md (the ideas index) and the archived/ subtree.
func walkIdeaFiles(specRoot string, fn func(path string, content []byte)) error {
	ideasDir := filepath.Join(specRoot, "ideas")
	return walkMatchingFiles(ideasDir, func(path string, depth int, name string) bool {
		if depth != 1 {
			return false
		}
		if name == "README.md" {
			return false
		}
		return strings.HasSuffix(name, ".md")
	}, fn)
}

// walkPlansIndex invokes fn for specRoot/plans/README.md if present.
func walkPlansIndex(specRoot string, fn func(path string, content []byte)) error {
	path := filepath.Join(specRoot, "plans", "README.md")
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	fn(path, content)
	return nil
}

// walkIdeasIndex invokes fn for specRoot/ideas/README.md if present.
func walkIdeasIndex(specRoot string, fn func(path string, content []byte)) error {
	path := filepath.Join(specRoot, "ideas", "README.md")
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	fn(path, content)
	return nil
}

// walkFeaturesIndex invokes fn for specRoot/features/README.md if present.
// This file is an Index-Kind document (features-index), not a Feature README,
// and is checked against the features-index-specification URL.
func walkFeaturesIndex(specRoot string, fn func(path string, content []byte)) error {
	path := filepath.Join(specRoot, "features", "README.md")
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	fn(path, content)
	return nil
}

// walkFeatureReadmesExcludingIndex walks feature READMEs but skips the root
// spec/features/README.md (which is an Index-Kind document, not a Feature).
// Used by the feature-specification adherence-footer check so the root index
// isn't flagged for missing feature-specification — its footer is
// features-index-specification, handled by walkFeaturesIndex.
func walkFeatureReadmesExcludingIndex(specRoot string, fn func(path string, content []byte)) error {
	rootIndex := filepath.Join(specRoot, "features", "README.md")
	return walkFeatureReadmes(specRoot, func(path string, content []byte) {
		if path == rootIndex {
			return
		}
		fn(path, content)
	})
}

// walkPlanReadmes invokes fn for every Plan README under specRoot/plans/**/README.md,
// excluding specRoot/plans/README.md (which is the plans-index, walked separately)
// and any README.md inside a reserved _-prefixed directory.
func walkPlanReadmes(specRoot string, fn func(path string, content []byte)) error {
	plansDir := filepath.Join(specRoot, "plans")
	info, err := os.Stat(plansDir)
	if err != nil || !info.IsDir() {
		return nil
	}
	return filepath.Walk(plansDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if path != plansDir && strings.HasPrefix(info.Name(), "_") {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Name() != "README.md" {
			return nil
		}
		// Skip the plans-index itself (handled by walkPlansIndex).
		if path == filepath.Join(plansDir, "README.md") {
			return nil
		}
		// Skip task READMEs — tasks/<slug>/README.md is a Task, not a Plan.
		if filepath.Base(filepath.Dir(filepath.Dir(path))) == "tasks" {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		fn(path, content)
		return nil
	})
}

// walkTaskReadmes invokes fn for every Task README under
// specRoot/plans/**/tasks/*/README.md.
func walkTaskReadmes(specRoot string, fn func(path string, content []byte)) error {
	plansDir := filepath.Join(specRoot, "plans")
	info, err := os.Stat(plansDir)
	if err != nil || !info.IsDir() {
		return nil
	}
	return filepath.Walk(plansDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || info.Name() != "README.md" {
			return nil
		}
		// A Task README lives at plans/**/tasks/<slug>/README.md. Check that
		// the grandparent directory is "tasks".
		if filepath.Base(filepath.Dir(filepath.Dir(path))) != "tasks" {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		fn(path, content)
		return nil
	})
}

// walkScenariosIndexes invokes fn for every scenarios index at
// specRoot/features/**/_tests/README.md.
func walkScenariosIndexes(specRoot string, fn func(path string, content []byte)) error {
	featuresDir := filepath.Join(specRoot, "features")
	info, err := os.Stat(featuresDir)
	if err != nil || !info.IsDir() {
		return nil
	}
	return filepath.Walk(featuresDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || info.Name() != "README.md" {
			return nil
		}
		// README.md must be directly inside a `_tests` directory.
		if filepath.Base(filepath.Dir(path)) != "_tests" {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		fn(path, content)
		return nil
	})
}

// walkScenarioFiles invokes fn for every scenario markdown file under
// specRoot/features/**/_tests/*.md, excluding README.md (which is an index,
// not a scenario).
func walkScenarioFiles(specRoot string, fn func(path string, content []byte)) error {
	featuresDir := filepath.Join(specRoot, "features")
	info, err := os.Stat(featuresDir)
	if err != nil || !info.IsDir() {
		return nil
	}
	return filepath.Walk(featuresDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}
		if info.Name() == "README.md" {
			return nil
		}
		// Path must include a segment named _tests.
		parent := filepath.Base(filepath.Dir(path))
		if parent != "_tests" {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		fn(path, content)
		return nil
	})
}

// walkPropertyFiles invokes fn for every *.property.md file under
// specRoot/features/, skipping hidden and underscore-prefixed directories.
func walkPropertyFiles(specRoot string, fn func(path string, content []byte)) error {
	featuresDir := filepath.Join(specRoot, "features")
	return walkMatchingFiles(featuresDir, func(_ string, _ int, name string) bool {
		return strings.HasSuffix(name, ".property.md")
	}, fn)
}

// walkMatchingFiles enumerates files under root, invoking fn for each file
// where match returns true. depth is measured relative to root (1 = direct
// child). Subdirectories are walked but match is only consulted for files.
func walkMatchingFiles(root string, match func(path string, depth int, name string) bool, fn func(path string, content []byte)) error {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil
	}
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if path != root && strings.HasPrefix(info.Name(), "_") {
				return filepath.SkipDir
			}
			// Also skip archived/ for ideas.
			if info.Name() == "archived" && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		depth := strings.Count(rel, string(filepath.Separator)) + 1
		if !match(path, depth, info.Name()) {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		fn(path, content)
		return nil
	})
}
