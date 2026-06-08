package lint

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/pkg/plan"
)

// parseFeatureACsMaxBuf is the scanner max-token size; tests may shrink it.
var parseFeatureACsMaxBuf = 1 << 20

// planRulesChecker implements the SpecStudio plan-Feature lint rules P-001
// through P-004 plus the parser-side validations they piggyback on
// (`**Mode:**` and `**Status:**` token validity covered by P-004). One
// checker emits violations for all four rule names; the linter framework
// dedupes by pointer identity so a single walk produces all findings.
//
// See spec/features/cli/spec/lint/plan-rules/README.md for the contract.
type planRulesChecker struct{}

func newPlanRulesChecker() checker {
	return &planRulesChecker{}
}

// name returns the primary rule name. The checker is registered under all
// four rule IDs in linter.go so that --rules / --ignore work per-rule.
func (c *planRulesChecker) name() string     { return "P-001" }
func (c *planRulesChecker) severity() string { return "error" }

func (c *planRulesChecker) check(specRoot string) ([]Violation, error) {
	plansDir := filepath.Join(specRoot, "plans")
	if info, err := os.Stat(plansDir); err != nil || !info.IsDir() {
		return nil, nil
	}

	entries, err := os.ReadDir(plansDir)
	if err != nil {
		return nil, fmt.Errorf("reading plans dir: %w", err)
	}

	featuresDir := filepath.Join(specRoot, "features")

	var violations []Violation
	parsedPlans := map[string]*plan.Plan{}
	relPaths := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "README.md" || !strings.HasSuffix(name, ".md") {
			continue
		}
		planPath := filepath.Join(plansDir, name)
		p, parseErr := plan.Parse(planPath)
		if parseErr != nil {
			return nil, fmt.Errorf("parsing plan %s: %w", planPath, parseErr)
		}
		if !p.HasPlanTitle {
			continue // not a SpecStudio single-file Plan
		}
		relPath, _ := filepath.Rel(specRoot, planPath)
		violations = append(violations, lintPlan(p, relPath, featuresDir)...)
		parsedPlans[p.Slug] = p
		relPaths[p.Slug] = relPath
	}
	// P-005 validates **Parent:** references across the full single-file plan
	// set (resolution + cycle detection need every plan's parent edge).
	violations = append(violations, lintP005(parsedPlans, relPaths)...)
	// Stable order: by file, line, rule name.
	sort.SliceStable(violations, func(i, j int) bool {
		if violations[i].File != violations[j].File {
			return violations[i].File < violations[j].File
		}
		if violations[i].Line != violations[j].Line {
			return violations[i].Line < violations[j].Line
		}
		return violations[i].Rule < violations[j].Rule
	})
	return violations, nil
}

// lintPlan runs all four rules against a single parsed Plan. relPath is the
// Plan's path relative to the spec root (used as Violation.File).
func lintPlan(p *plan.Plan, relPath, featuresDir string) []Violation {
	var v []Violation

	// P-004 schema-token validity runs first (Mode + per-task Status). These
	// findings do not depend on the source Feature, so they surface even when
	// the source Feature is missing.
	v = append(v, lintP004SchemaTokens(p, relPath)...)

	// P-003 dependency-graph validation. Self-contained — only needs the Plan.
	v = append(v, lintP003(p, relPath)...)

	// Source-Feature-dependent rules (P-001 and P-002).
	v = append(v, lintP001P002(p, relPath, featuresDir)...)

	// P-004 stub-mode placeholder-on-done validation. Depends only on the Plan.
	v = append(v, lintP004StubPlaceholder(p, relPath)...)

	return v
}

// ----- P-004 schema-token validity -----

func lintP004SchemaTokens(p *plan.Plan, relPath string) []Violation {
	var out []Violation
	if p.ModeRawPresent && !p.ModeValueValid {
		out = append(out, Violation{
			File:     relPath,
			Line:     p.ModeLine,
			Severity: "error",
			Rule:     "P-004",
			Message: fmt.Sprintf(
				"invalid **Mode:** value %q (accepted: full, stub)",
				p.ModeRaw,
			),
		})
	}
	for _, t := range p.Tasks {
		if t.StatusPresent && !t.StatusValueValid {
			out = append(out, Violation{
				File:     relPath,
				Line:     t.StatusLine,
				Severity: "error",
				Rule:     "P-004",
				Message: fmt.Sprintf(
					"Task %d: invalid **Status:** value %q (accepted: pending, in-progress, done, blocked)",
					t.Number, t.StatusRaw,
				),
			})
		}
	}
	return out
}

// ----- P-003 dependency-graph -----

func lintP003(p *plan.Plan, relPath string) []Violation {
	var out []Violation

	// 1. Linear numbering 1..N. The parser preserves Number as-given; we
	//    detect gaps, duplicates, non-monotonic order, and non-positive
	//    numbers up-front so dangling/cycle detection can rely on the
	//    invariant.
	taskByNumber := make(map[int]plan.Task)
	expected := 1
	nonLinearReported := false
	for _, t := range p.Tasks {
		if t.Number != expected && !nonLinearReported {
			out = append(out, Violation{
				File:     relPath,
				Line:     t.HeadingLine,
				Severity: "error",
				Rule:     "P-003",
				Message: fmt.Sprintf(
					"Task numbering must be linear 1..N (expected Task %d, got Task %d)",
					expected, t.Number,
				),
			})
			nonLinearReported = true
		}
		if _, dup := taskByNumber[t.Number]; dup {
			out = append(out, Violation{
				File:     relPath,
				Line:     t.HeadingLine,
				Severity: "error",
				Rule:     "P-003",
				Message:  fmt.Sprintf("duplicate task number %d", t.Number),
			})
		}
		taskByNumber[t.Number] = t
		expected++
	}

	// 2. Self-references and dangling references.
	for _, t := range p.Tasks {
		for _, dep := range t.DependsOn {
			if dep == t.Number {
				out = append(out, Violation{
					File:     relPath,
					Line:     t.DependsOnLine,
					Severity: "error",
					Rule:     "P-003",
					Message:  fmt.Sprintf("Task %d depends on itself", t.Number),
				})
				continue
			}
			if _, ok := taskByNumber[dep]; !ok {
				out = append(out, Violation{
					File:     relPath,
					Line:     t.DependsOnLine,
					Severity: "error",
					Rule:     "P-003",
					Message: fmt.Sprintf(
						"Task %d depends on nonexistent task %d",
						t.Number, dep,
					),
				})
			}
		}
	}

	// 3. Cycle detection (only on the well-defined subgraph). DFS with
	//    parent tracking; report the first cycle found, citing the full
	//    path.
	if cycle := findCycle(p.Tasks); len(cycle) > 0 {
		// Cite the cycle on the dependency line of the first node in the
		// cycle so the user can navigate directly.
		var line int
		for _, t := range p.Tasks {
			if t.Number == cycle[0] {
				line = t.DependsOnLine
				if line == 0 {
					line = t.HeadingLine
				}
				break
			}
		}
		var labels []string
		for _, n := range cycle {
			labels = append(labels, fmt.Sprintf("Task %d", n))
		}
		// Close the loop visually by repeating the first node.
		labels = append(labels, fmt.Sprintf("Task %d", cycle[0]))
		out = append(out, Violation{
			File:     relPath,
			Line:     line,
			Severity: "error",
			Rule:     "P-003",
			Message: fmt.Sprintf(
				"Depends-On cycle: %s",
				strings.Join(labels, " → "),
			),
		})
	}

	return out
}

// findCycle returns the task-number sequence that forms the first cycle in
// the dependency graph, or nil when the graph is acyclic. Only edges to
// existing tasks contribute to the search; dangling edges are handled
// separately.
func findCycle(tasks []plan.Task) []int {
	nums := make(map[int]bool, len(tasks))
	edges := make(map[int][]int, len(tasks))
	for _, t := range tasks {
		nums[t.Number] = true
	}
	for _, t := range tasks {
		for _, dep := range t.DependsOn {
			if dep == t.Number {
				continue
			}
			if !nums[dep] {
				continue
			}
			edges[t.Number] = append(edges[t.Number], dep)
		}
	}

	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[int]int, len(tasks))
	parent := make(map[int]int, len(tasks))

	var (
		cycle   []int
		dfs     func(n int) bool
		ordered []int
	)
	for n := range nums {
		ordered = append(ordered, n)
	}
	sort.Ints(ordered)

	dfs = func(n int) bool {
		color[n] = gray
		for _, m := range edges[n] {
			switch color[m] {
			case white:
				parent[m] = n
				if dfs(m) {
					return true
				}
			case gray:
				// Reconstruct cycle: walk parents from n back to m.
				path := []int{m}
				for cur := n; cur != m; cur = parent[cur] {
					path = append(path, cur)
				}
				// path is in reverse order (m, n, parent(n), …, m's
				// ancestor right before m). Reverse to read forward.
				for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
					path[i], path[j] = path[j], path[i]
				}
				cycle = path
				return true
			}
		}
		color[n] = black
		return false
	}

	for _, n := range ordered {
		if color[n] == white {
			if dfs(n) {
				return cycle
			}
		}
	}
	return nil
}

// ----- P-001 and P-002 (source-Feature-dependent) -----

func lintP001P002(p *plan.Plan, relPath, featuresDir string) []Violation {
	var out []Violation

	// Resolve the source-Feature path. If absent or missing on disk, emit a
	// single P-002 violation and bail — we cannot validate AC IDs without
	// the source AC list, and P-001 coverage is moot until the Feature
	// resolves.
	if p.SourceFeature == "" {
		out = append(out, Violation{
			File:     relPath,
			Line:     p.TitleLine,
			Severity: "error",
			Rule:     "P-002",
			Message:  "Plan is missing **Source Feature:** body metadata",
		})
		return out
	}
	featReadme := filepath.Join(featuresDir, filepath.FromSlash(p.SourceFeature), "README.md")
	acs, err := parseFeatureACs(featReadme)
	if err != nil || acs == nil {
		out = append(out, Violation{
			File:     relPath,
			Line:     p.SourceFeatureLine,
			Severity: "error",
			Rule:     "P-002",
			Message: fmt.Sprintf(
				"**Source Feature:** %s does not resolve to a Feature README at %s",
				p.SourceFeature, filepath.Join("features", filepath.FromSlash(p.SourceFeature), "README.md"),
			),
		})
		return out
	}

	// Build the set of valid AC IDs `<feature-slug>#ac:<ac-slug>` from the
	// source Feature.
	validIDs := make(map[string]int, len(acs))
	for slug, line := range acs {
		validIDs[fmt.Sprintf("%s#ac:%s", p.SourceFeature, slug)] = line
	}

	// P-002 pass: every Verifies / Deferred reference must resolve.
	for _, t := range p.Tasks {
		if t.VerifiesPresent && len(t.Verifies) == 0 {
			out = append(out, Violation{
				File:     relPath,
				Line:     t.VerifiesLine,
				Severity: "error",
				Rule:     "P-002",
				Message:  fmt.Sprintf("Task %d: empty **Verifies:** line", t.Number),
			})
			continue
		}
		for _, ref := range t.Verifies {
			if _, ok := validIDs[ref]; !ok {
				out = append(out, Violation{
					File:     relPath,
					Line:     t.VerifiesLine,
					Severity: "error",
					Rule:     "P-002",
					Message: fmt.Sprintf(
						"Task %d: stale AC reference %s (no such AC in source Feature %s)",
						t.Number, ref, p.SourceFeature,
					),
				})
			}
		}
	}
	for _, d := range p.DeferredACs {
		if _, ok := validIDs[d.ACID]; !ok {
			out = append(out, Violation{
				File:     relPath,
				Line:     d.Line,
				Severity: "error",
				Rule:     "P-002",
				Message: fmt.Sprintf(
					"stale AC reference %s in ## Deferred AC Coverage (no such AC in source Feature %s)",
					d.ACID, p.SourceFeature,
				),
			})
		}
	}

	// P-001 pass: every AC in the source Feature must be covered or deferred.
	covered := make(map[string]bool, len(validIDs))
	for _, t := range p.Tasks {
		for _, ref := range t.Verifies {
			covered[ref] = true
		}
	}
	for _, d := range p.DeferredACs {
		covered[d.ACID] = true
	}
	// Stable iteration: alphabetical AC slug order.
	var slugs []string
	for slug := range acs {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	for _, slug := range slugs {
		id := fmt.Sprintf("%s#ac:%s", p.SourceFeature, slug)
		if covered[id] {
			continue
		}
		out = append(out, Violation{
			File:     relPath,
			Line:     p.TitleLine,
			Severity: "error",
			Rule:     "P-001",
			Message: fmt.Sprintf(
				"AC coverage gap: %s is neither covered by any task's **Verifies:** nor listed under ## Deferred AC Coverage",
				id,
			),
		})
	}

	return out
}

// acHeadingRe matches `### AC: <ac-slug>` headings in a Feature README. The
// slug captures up to the first whitespace or `(` (the verifies-parenthetical).
var acHeadingRe = regexp.MustCompile(`^###\s+AC:\s+(\S+?)(?:\s|\(|$)`)

// parseFeatureACs reads the Feature README at path and returns a map of
// AC slug → 1-based line number of the heading. Returns (nil, nil) when the
// file does not exist (the caller treats nil as "Feature missing").
func parseFeatureACs(path string) (map[string]int, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	out := make(map[string]int)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), parseFeatureACsMaxBuf)
	inACs := false
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		trimmed := strings.TrimSpace(scanner.Text())
		if title, ok := strings.CutPrefix(trimmed, "## "); ok {
			inACs = strings.TrimSpace(title) == "Acceptance Criteria"
			continue
		}
		if !inACs {
			continue
		}
		if m := acHeadingRe.FindStringSubmatch(trimmed); m != nil {
			slug := strings.TrimRight(m[1], ":")
			if slug != "" {
				out[slug] = lineNum
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ----- P-004 stub-mode placeholder-on-done -----

func lintP004StubPlaceholder(p *plan.Plan, relPath string) []Violation {
	if p.Mode != plan.ModeStub {
		return nil
	}
	var out []Violation
	for _, t := range p.Tasks {
		if t.Status == plan.StatusDone && t.HasPlaceholder {
			out = append(out, Violation{
				File:     relPath,
				Line:     t.HeadingLine,
				Severity: "error",
				Rule:     "P-004",
				Message: fmt.Sprintf(
					"Task %d: **Status:** done in a **Mode:** stub Plan must not have the placeholder body marker; either rerun specstudio:implement to write back the post-batch prose (REQ:posture-stub-placeholder, REQ:stub-placeholder-done-lint) or revert Status to pending",
					t.Number,
				),
			})
		}
	}
	return out
}

// ----- P-005 parent reference validity -----

// crossRepoParentRe matches a cross-repo plan reference `<repo-slug>:<plan-slug>`
// where both sides are lowercase, hyphen-separated, URL-safe slugs and there is
// exactly one ':' separator (cli/spec/lint/plan-rules#req:rule-p-005-cross-repo-syntactic-only).
var crossRepoParentRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*:[a-z0-9]+(?:-[a-z0-9]+)*$`)

// lintP005 validates the optional **Parent:** reference on every single-file
// Plan. Same-repo parents (no ':') must resolve to another single-file Plan,
// must not be self-references, and must not form a cycle. Cross-repo parents
// (`<repo-slug>:<plan-slug>`) are validated syntactically only — never resolved,
// never scanned for across sibling repos.
func lintP005(parsedPlans map[string]*plan.Plan, relPaths map[string]string) []Violation {
	var out []Violation

	slugs := make([]string, 0, len(parsedPlans))
	for slug := range parsedPlans {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)

	// sameRepoParent maps slug -> resolvable same-repo parent slug, used for
	// cycle detection. Only populated for non-self, resolvable, same-repo edges.
	sameRepoParent := map[string]string{}

	for _, slug := range slugs {
		p := parsedPlans[slug]
		parent := strings.TrimSpace(p.Parent)
		if parent == "" {
			continue // root plan
		}
		relPath := relPaths[slug]
		line := p.ParentLine

		if strings.Contains(parent, ":") {
			if !crossRepoParentRe.MatchString(parent) {
				out = append(out, Violation{
					File: relPath, Line: line, Severity: "error", Rule: "P-005",
					Message: fmt.Sprintf("malformed cross-repo **Parent:** %q (expected <repo-slug>:<plan-slug>, each lowercase, hyphen-separated, URL-safe)", parent),
				})
			}
			continue
		}

		if parent == slug {
			out = append(out, Violation{
				File: relPath, Line: line, Severity: "error", Rule: "P-005",
				Message: fmt.Sprintf("**Parent:** %q references the plan itself", slug),
			})
			continue
		}
		if _, ok := parsedPlans[parent]; !ok {
			out = append(out, Violation{
				File: relPath, Line: line, Severity: "error", Rule: "P-005",
				Message: fmt.Sprintf("**Parent:** %q does not resolve to a single-file Plan at spec/plans/%s.md", parent, parent),
			})
			continue
		}
		sameRepoParent[slug] = parent
	}

	out = append(out, detectParentCycles(sameRepoParent, parsedPlans, relPaths)...)
	return out
}

// detectParentCycles finds cycles in the same-repo parent graph and emits one
// P-005 violation per distinct cycle, reported on the lexicographically
// smallest member for stable output.
func detectParentCycles(edges map[string]string, parsedPlans map[string]*plan.Plan, relPaths map[string]string) []Violation {
	var out []Violation
	reported := map[string]bool{}

	starts := make([]string, 0, len(edges))
	for slug := range edges {
		starts = append(starts, slug)
	}
	sort.Strings(starts)

	for _, start := range starts {
		seen := map[string]int{}
		var path []string
		cur := start
		for {
			next, ok := edges[cur]
			if !ok {
				break
			}
			if idx, dup := seen[cur]; dup {
				cycle := append([]string{}, path[idx:]...)
				key := canonicalCycleKey(cycle)
				if !reported[key] {
					reported[key] = true
					first := cycleFirst(cycle)
					p := parsedPlans[first]
					out = append(out, Violation{
						File: relPaths[first], Line: p.ParentLine, Severity: "error", Rule: "P-005",
						Message: fmt.Sprintf("**Parent:** chain forms a cycle: %s", renderCyclePath(cycle)),
					})
				}
				break
			}
			seen[cur] = len(path)
			path = append(path, cur)
			cur = next
		}
	}
	return out
}

// canonicalCycleKey returns an order-independent key so each cycle reports once.
func canonicalCycleKey(cycle []string) string {
	sorted := append([]string{}, cycle...)
	sort.Strings(sorted)
	return strings.Join(sorted, "\x00")
}

// cycleFirst returns the lexicographically smallest slug in the cycle.
func cycleFirst(cycle []string) string {
	first := cycle[0]
	for _, s := range cycle[1:] {
		if s < first {
			first = s
		}
	}
	return first
}

// renderCyclePath renders a cycle as `a → b → … → a`, rotated to start at the
// lexicographically smallest member for determinism.
func renderCyclePath(cycle []string) string {
	start := 0
	for i, s := range cycle {
		if s < cycle[start] {
			start = i
		}
	}
	rotated := make([]string, 0, len(cycle)+1)
	for i := range cycle {
		rotated = append(rotated, cycle[(start+i)%len(cycle)])
	}
	rotated = append(rotated, cycle[start])
	return strings.Join(rotated, " → ")
}
