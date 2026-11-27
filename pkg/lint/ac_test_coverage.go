package lint

// Features implemented: cli/spec/lint/ac-test-coverage

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/specscore/specscore-cli/pkg/plan"
	"github.com/specscore/specscore-cli/pkg/projectdef"
	"github.com/specscore/specscore-cli/pkg/sourceref"
)

// acTestCoverageRule is the opt-in rule that requires every acceptance
// criterion of an Implementing or Stable Feature to be named by an annotation
// in a test file, or explicitly deferred (cli/spec/lint/ac-test-coverage).
// It reports nothing unless the repository sets
// `ac_test_coverage.required: true` in specscore.yaml or the caller names
// the rule in --rules (cli/spec/lint/ac-test-coverage#req:opt-in).
const acTestCoverageRule = "feature-ac-test-coverage"

// acTestCoverageStatuses are the Feature statuses the rule applies to
// (cli/spec/lint/ac-test-coverage#req:scope), compared lower-cased.
var acTestCoverageStatuses = []string{"implementing", "stable"}

// acTestCoverageSkipDirs are directory names never descended into while
// looking for test files: version-control metadata and installed
// JavaScript dependencies (cli/spec/lint/ac-test-coverage#req:test-files).
var acTestCoverageSkipDirs = []string{".git", "node_modules"}

// acDeferredKey is the AC-level deferral field
// (cli/spec/lint/ac-test-coverage#req:deferral).
const acDeferredKey = "**Deferred:**"

type acTestCoverageChecker struct {
	projectRoot string
	plansDir    string
	// explicit is true when the caller named the rule in --rules: an explicit
	// selection runs the rule without the config opt-in.
	explicit bool
	// routeBroken means Plan routing is configured but failed to resolve: the
	// checker must not read any Plan tree, so it reports nothing (the
	// plan-route-unresolved finding fails the run instead).
	routeBroken bool
}

func newACTestCoverageChecker(projectRoot, plansDir string, explicit bool) *acTestCoverageChecker {
	return &acTestCoverageChecker{projectRoot: projectRoot, plansDir: plansDir, explicit: explicit}
}

func (c *acTestCoverageChecker) name() string     { return acTestCoverageRule }
func (c *acTestCoverageChecker) severity() string { return "error" }

// routeErrorChecker implements planOwnedChecker: the rule reads Plans'
// `## Deferred AC Coverage` lists, so a broken Plan route makes it a no-op.
func (c *acTestCoverageChecker) routeErrorChecker() checker {
	return &acTestCoverageChecker{routeBroken: true}
}

// acFeature is what the rule needs from one Feature README.
type acFeature struct {
	rel    string // README path relative to specRoot, slash-separated
	id     string // Feature path relative to specRoot/features, e.g. cli/code/deps
	status string
	acs    []acCriterion
}

type acCriterion struct {
	id   string
	line int // 1-based line of the `### AC:` heading
	// deferred is true when the AC block carries `**Deferred:** <reason>`.
	deferred bool
	// emptyDeferral is true when the block has a `**Deferred:**` line with no
	// reason, which does not count as a deferral.
	emptyDeferral bool
}

func (c *acTestCoverageChecker) check(specRoot string) ([]Violation, error) {
	if c.routeBroken {
		return nil, nil
	}
	root := lintProjectRoot(c.projectRoot, specRoot)
	cfg, err := projectdef.ReadSpecConfig(root)
	if err != nil {
		// No readable specscore.yaml: lint's repo-config gate reports that;
		// the rule stays off unless explicitly selected.
		if !c.explicit {
			return nil, nil
		}
		cfg = projectdef.SpecConfig{}
	}
	if shape := cfg.ACTestCoverageShapeError(); shape != "" {
		severity := "error"
		if cfg.ACTestCoverageExplicitlyOff() {
			severity = "warning"
		}
		return []Violation{{File: projectdef.SpecConfigFile, Severity: severity, Rule: acTestCoverageRule,
			Message: "invalid `ac_test_coverage` block in specscore.yaml: " + shape + "; fix the block (see `specscore rules`)."}}, nil
	}
	if !c.explicit && !cfg.ACTestCoverageRequired() {
		return nil, nil
	}

	var features []acFeature
	err = walkFeatureReadmes(specRoot, func(readmePath string, content []byte) {
		rel, _ := filepath.Rel(specRoot, readmePath)
		f := parseACFeature(string(content))
		if f == nil || !slices.Contains(acTestCoverageStatuses, strings.ToLower(f.status)) || len(f.acs) == 0 {
			return
		}
		f.rel = filepath.ToSlash(rel)
		f.id = strings.TrimSuffix(strings.TrimPrefix(f.rel, "features/"), "/README.md")
		features = append(features, *f)
	})
	if err != nil {
		return nil, err
	}
	if len(features) == 0 {
		return nil, nil
	}

	self, _ := cfg.ProjectIdentity()
	named, err := acNamedByTests(root, cfg.ACTestCoverageTestFiles(), self)
	if err != nil {
		return nil, err
	}
	deferred := acDeferredByPlans(effectivePlansDir(specRoot, c.plansDir))

	var out []Violation
	for _, f := range features {
		for _, ac := range f.acs {
			key := f.id + "#ac:" + ac.id
			if ac.deferred || named[key] || deferred[key] {
				continue
			}
			msg := "Feature is " + f.status + " but `" + key + "` is named by no test; add `specscore:verifies feature/" + key +
				"` to a test file (ac_test_coverage.test_files), or defer it with `**Deferred:** <reason>` under the AC or a `## Deferred AC Coverage` entry in a Plan."
			if ac.emptyDeferral {
				msg = "Feature is " + f.status + " but `" + key + "` has a `**Deferred:**` line with no reason, which is not a deferral; write the reason after `**Deferred:**`, or add `specscore:verifies feature/" + key + "` to a test file (ac_test_coverage.test_files)."
			}
			out = append(out, Violation{File: f.rel, Line: ac.line, Severity: "error", Rule: acTestCoverageRule, Message: msg})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out, nil
}

// parseACFeature returns the status and acceptance criteria of a Feature
// README, or nil when content is not a Feature (no `# Feature: ` H1). Only
// `### AC:` headings under `## Acceptance Criteria` count; headings and
// deferral lines inside fenced code blocks or HTML comments are ignored.
func parseACFeature(content string) *acFeature {
	var f acFeature
	isFeature, inACs := false, false
	var fence mdFence
	inComment := false
	var cur *acCriterion
	for i, line := range strings.Split(content, "\n") {
		if fence.open() {
			fence.feed(line)
			continue
		}
		var visible string
		visible, inComment = stripHTMLComments(line, inComment)
		t := strings.TrimSpace(visible)
		if t == "" || fence.feed(t) {
			continue
		}
		if !isFeature && strings.HasPrefix(t, "# Feature: ") {
			isFeature = true
		}
		if f.status == "" && strings.HasPrefix(t, "**Status:**") {
			f.status = strings.TrimSpace(strings.TrimPrefix(t, "**Status:**"))
		}
		if strings.HasPrefix(t, "# ") || strings.HasPrefix(t, "## ") || t == "---" {
			inACs = strings.HasPrefix(t, "## ") && strings.TrimSpace(strings.TrimPrefix(t, "## ")) == "Acceptance Criteria"
			cur = nil
			continue
		}
		if !inACs {
			continue
		}
		if strings.HasPrefix(t, "### ") {
			cur = nil
			if m := acHeadingRe.FindStringSubmatch(t); m != nil {
				f.acs = append(f.acs, acCriterion{id: strings.TrimRight(m[1], ":"), line: i + 1})
				cur = &f.acs[len(f.acs)-1]
			}
			continue
		}
		if isATXHeading(t) {
			cur = nil // any other heading ends the AC block
			continue
		}
		if cur != nil && strings.HasPrefix(t, acDeferredKey) {
			reason := strings.Trim(strings.TrimSpace(strings.TrimPrefix(t, acDeferredKey)), "—–- ")
			if reason != "" {
				cur.deferred = true
			} else {
				cur.emptyDeferral = true
			}
		}
	}
	if !isFeature {
		return nil
	}
	return &f
}

// acNamedByTests returns the set of `<feature-id>#ac:<ac-id>` keys named by
// an annotation in a test file under root. Annotations are parsed with the
// `specscore code` parser (sourceref.ScanDirective); only valid directives
// count, so `specscore:implements` (which must target a REQ) never covers an
// AC. A reference counts when it targets this repository: no authority, or
// the authority equals self (`host/org/repo` from specscore.yaml).
func acNamedByTests(root string, globs []string, self string) (map[string]bool, error) {
	named := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if path != root && slices.Contains(acTestCoverageSkipDirs, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if !slices.ContainsFunc(globs, func(g string) bool { return doublestar.MatchUnvalidated(g, rel) }) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(data), "\n") {
			if key, ok := acKeyFromLine(line, self); ok {
				named[key] = true
			}
		}
		return nil
	})
	return named, err
}

// acKeyFromLine returns the `<feature-id>#ac:<ac-id>` key a source line's
// annotation names, when it names an AC of a Feature in this repository.
func acKeyFromLine(line, self string) (string, bool) {
	d := sourceref.ScanDirective(line)
	if d == nil || sourceref.ValidateDirective(d) != nil {
		return "", false
	}
	ref := d.Target
	if ref.Type != "feature" || (ref.CrossRepoSuffix != "" && strings.TrimPrefix(ref.CrossRepoSuffix, "@") != self) {
		return "", false
	}
	kind, id, ok := strings.Cut(ref.Fragment, ":")
	if !ok || !strings.EqualFold(kind, "ac") || id == "" {
		return "", false
	}
	feature := strings.TrimPrefix(ref.ResolvedPath, "spec/features/")
	return feature + "#ac:" + id, true
}

// acDeferredByPlans returns the set of AC keys listed under any Plan's
// `## Deferred AC Coverage`. An unreadable or ambiguous Plan tree defers
// nothing (the plan rules report it); the rule then fails closed.
func acDeferredByPlans(plansDir string) map[string]bool {
	deferred := map[string]bool{}
	plans, _ := plan.Discover(plansDir)
	for _, p := range plans {
		for _, d := range p.DeferredACs {
			deferred[d.ACID] = true
		}
	}
	return deferred
}

// isATXHeading reports whether a trimmed line is a Markdown ATX heading: one
// to six `#` followed by a space or the end of the line.
func isATXHeading(t string) bool {
	rest := strings.TrimLeft(t, "#")
	n := len(t) - len(rest)
	return n >= 1 && n <= 6 && (rest == "" || rest[0] == ' ' || rest[0] == '\t')
}
