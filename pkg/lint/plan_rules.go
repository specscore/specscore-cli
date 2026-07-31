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
type planRulesChecker struct {
	// fixNoSource, when true, makes the fix pass rewrite a plan that declares
	// no source line at all into a source-less plan (`**Source:** none`). It is
	// opt-in (enabled via `--fix=no-source`) because a fully-absent source line
	// is more often a dropped `**Source Feature:**` than an intentional
	// source-less plan; the default fix pass leaves it as a P-002 error.
	fixNoSource bool

	// fixP007, when true, makes the fix pass reconcile drifting execution-band
	// plan statuses (P-007). Unlike no-source, P-007 is a standard fixer: it
	// runs on the unscoped pass and when `--fix=P-007` names it explicitly.
	fixP007 bool

	// fixP006Legacy, when true, makes the fix pass rewrite the closed set of
	// legacy plan **Status:** tokens (Completed→Implemented, Under Review→In
	// Review) to canonical. Standard fixer: unscoped pass or `--fix=P-006`.
	fixP006Legacy bool

	// fixP004Legacy, when true, makes the fix pass rewrite the closed set of
	// legacy per-task **Status:** tokens (pending→planning, done→complete,
	// in-progress→in_progress) to the canonical Task-status enum. Standard
	// fixer: unscoped pass or `--fix=P-004`.
	fixP004Legacy bool
}

func newPlanRulesChecker() *planRulesChecker {
	return &planRulesChecker{}
}

// name returns the primary rule name. The checker is registered under all
// four rule IDs in linter.go so that --rules / --ignore work per-rule.
func (c *planRulesChecker) name() string     { return "P-001" }
func (c *planRulesChecker) severity() string { return "error" }

// fixTargets declares the `--fix=<target>` actions this checker answers to: the
// opt-in no-source repair (named "no-source" rather than a P-00x rule ID) and
// the standard P-007 execution-band reconciliation (named by its rule ID).
func (c *planRulesChecker) fixTargets() []string {
	return []string{FixTargetNoSource, "P-004", "P-006", "P-007"}
}

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
	// P-009 validates same-repo cross-plan prerequisites as a separate graph;
	// Parent remains composition, while task Depends-On remains intra-plan.
	violations = append(violations, lintP009(parsedPlans, relPaths)...)
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

// fix implements the fixer interface. It performs two independent repairs,
// each gated by its own flag: the opt-in "no-source" repair and the standard
// P-007 execution-band reconciliation. Both are idempotent.
func (c *planRulesChecker) fix(specRoot string) error {
	if c.fixP004Legacy {
		if err := fixLegacyTaskStatuses(specRoot); err != nil {
			return err
		}
	}
	if c.fixP006Legacy {
		if err := fixLegacyStatusesInTree(filepath.Join(specRoot, "plans"), legacyPlanStatusMap, false); err != nil {
			return err
		}
	}
	if c.fixP007 {
		if err := fixP007(specRoot); err != nil {
			return err
		}
	}
	if c.fixNoSource {
		if err := c.fixNoSourceLines(specRoot); err != nil {
			return err
		}
	}
	return nil
}

// fixNoSourceLines is the opt-in "no-source" repair: every single-file plan that
// declares no source line at all gains a `**Source:** none` header line. A plan
// that already declares a source (Feature, Idea, or an unrecognized `**Source:**`
// value) is left untouched — only a fully-absent source line is repaired, and an
// unrecognized value stays a hard P-002 error. Idempotent.
func (c *planRulesChecker) fixNoSourceLines(specRoot string) error {
	plansDir := filepath.Join(specRoot, "plans")
	if info, err := os.Stat(plansDir); err != nil || !info.IsDir() {
		return nil
	}
	entries, err := os.ReadDir(plansDir)
	if err != nil {
		return fmt.Errorf("reading plans dir: %w", err)
	}
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
			return fmt.Errorf("parsing plan %s: %w", planPath, parseErr)
		}
		// Only a fully-absent source line is fixable: no `**Source Feature:**`
		// and no `**Source:**` line of any kind.
		if !p.HasPlanTitle || p.SourceFeature != "" || p.SourceLine != 0 {
			continue
		}
		if err := insertSourceNone(planPath, p); err != nil {
			return fmt.Errorf("fixing %s: %w", planPath, err)
		}
	}
	return nil
}

// insertSourceNone writes a `**Source:** none` header line into a plan that
// lacks any source line. The line is inserted just after `**Status:**` (matching
// the canonical header order) or, absent that, just after the title.
func insertSourceNone(path string, p *plan.Plan) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	anchor := p.TitleLine // 1-based line to insert AFTER
	if p.StatusLine > 0 {
		anchor = p.StatusLine
	}
	if anchor <= 0 || anchor > len(lines) {
		anchor = len(lines)
	}
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:anchor]...)
	out = append(out, "**Source:** none")
	out = append(out, lines[anchor:]...)
	return os.WriteFile(path, []byte(strings.Join(out, "\n")), 0o644)
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

	// P-006 document-status vocabulary validation. Depends only on the Plan.
	v = append(v, lintP006(p, relPath)...)

	// P-007 execution-band derivation drift. Depends only on the Plan.
	v = append(v, lintP007(p, relPath)...)

	// P-008 implementation-commit-provenance ref-format. Syntactic only.
	v = append(v, lintP008(p, relPath)...)

	// P-010 coordination-branch reference format. Syntactic only.
	v = append(v, lintP010(p, relPath)...)

	return v
}

// ----- P-004 schema-token validity -----

// canonicalTaskStatuses is the sole legal per-task **Status:** vocabulary: the
// 7-value Task-status enum (unify-task-status-vocabulary#ac:enum-is-sole-vocabulary).
// P-004 validates every `### Task N:` block's **Status:** against this set.
var canonicalTaskStatuses = map[string]bool{
	"planning":    true,
	"queued":      true,
	"in_progress": true,
	"blocked":     true,
	"complete":    true,
	"failed":      true,
	"aborted":     true,
}

// canonicalTaskStatusList renders the enum for violation messages, in enum order.
const canonicalTaskStatusList = "planning, queued, in_progress, blocked, complete, failed, aborted"

// legacyTaskStatusMap is the CLOSED set of pre-enum legacy per-task **Status:**
// tokens and their canonical replacement. `lint` names the replacement in the
// violation; `lint --fix` rewrites the value in place
// (unify-task-status-vocabulary#ac:lint-flags-legacy / #ac:lint-fix-migrates-legacy).
var legacyTaskStatusMap = map[string]string{
	"pending":     "planning",
	"done":        "complete",
	"in-progress": "in_progress",
}

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
		if !t.StatusPresent || canonicalTaskStatuses[t.StatusRaw] {
			continue
		}
		if canonical, ok := legacyTaskStatusMap[t.StatusRaw]; ok {
			out = append(out, Violation{
				File:      relPath,
				Line:      t.StatusLine,
				Severity:  "error",
				Rule:      "P-004",
				FixTarget: "P-004",
				Message: fmt.Sprintf(
					"Task %d: legacy **Status:** value %q; use canonical %q (run --fix to migrate)",
					t.Number, t.StatusRaw, canonical,
				),
			})
			continue
		}
		out = append(out, Violation{
			File:     relPath,
			Line:     t.StatusLine,
			Severity: "error",
			Rule:     "P-004",
			Message: fmt.Sprintf(
				"Task %d: **Status:** value %q is not a valid task status (accepted: %s)",
				t.Number, t.StatusRaw, canonicalTaskStatusList,
			),
		})
	}
	return out
}

// fixLegacyTaskStatuses rewrites every legacy per-task **Status:** token to its
// canonical enum value across all single-file Plans, changing only the value on
// each task's **Status:** line. It mirrors lintP004SchemaTokens' legacy detection
// so a second pass is a no-op (the rewritten value is canonical, not legacy).
func fixLegacyTaskStatuses(specRoot string) error {
	plansDir := filepath.Join(specRoot, "plans")
	if info, err := os.Stat(plansDir); err != nil || !info.IsDir() {
		return nil
	}
	entries, err := os.ReadDir(plansDir)
	if err != nil {
		return fmt.Errorf("reading plans dir: %w", err)
	}
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
			return fmt.Errorf("parsing plan %s: %w", planPath, parseErr)
		}
		if !p.HasPlanTitle {
			continue
		}
		rewrites := map[int]string{}
		for _, t := range p.Tasks {
			if !t.StatusPresent {
				continue
			}
			if canonical, ok := legacyTaskStatusMap[t.StatusRaw]; ok {
				rewrites[t.StatusLine] = canonical
			}
		}
		if len(rewrites) == 0 {
			continue
		}
		if err := rewriteTaskStatusLines(planPath, rewrites); err != nil {
			return fmt.Errorf("fixing %s: %w", planPath, err)
		}
	}
	return nil
}

// rewriteTaskStatusLines rewrites each 1-based line in rewrites to
// `**Status:** <canonical>`, preserving every other line verbatim. Line numbers
// come from the parse pass over the same file, so they are always in range.
func rewriteTaskStatusLines(path string, rewrites map[int]string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	for lineNo, canonical := range rewrites {
		lines[lineNo-1] = "**Status:** " + canonical
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
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

	// Idea-sourced (`**Source:** idea:<slug>`) and source-less
	// (`**Source:** none`) plans have no source Feature, so the AC-coverage
	// (P-001) and AC-reference (P-002) rules do not apply.
	if p.SourceIdea != "" || p.SourceNone {
		return out
	}

	// Resolve the source-Feature path. If absent or missing on disk, emit a
	// single P-002 violation and bail — we cannot validate AC IDs without
	// the source AC list, and P-001 coverage is moot until the Feature
	// resolves.
	if p.SourceFeature == "" {
		// Distinguish "no source line at all" (likely a dropped Source
		// Feature) from an unrecognized `**Source:**` value. Only the former is
		// auto-fixable (via --fix=no-source); an unrecognized value needs a
		// human decision and carries no FixTarget.
		v := Violation{
			File:      relPath,
			Line:      p.TitleLine,
			Severity:  "error",
			Rule:      "P-002",
			Message:   "Plan is missing a source line: declare one of **Source Feature:** <slug>, **Source:** idea:<slug>, or **Source:** none",
			FixTarget: FixTargetNoSource,
		}
		if p.SourceLine != 0 {
			v.Line = p.SourceLine
			v.Message = fmt.Sprintf(
				"unrecognized **Source:** value %q (expected `idea:<slug>` or `none`)",
				p.SourceRaw,
			)
			v.FixTarget = ""
		}
		out = append(out, v)
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
		if t.Status == plan.StatusComplete && t.HasPlaceholder {
			out = append(out, Violation{
				File:     relPath,
				Line:     t.HeadingLine,
				Severity: "error",
				Rule:     "P-004",
				Message: fmt.Sprintf(
					"Task %d: **Status:** complete in a **Mode:** stub Plan must not have the placeholder body marker; either rerun specstudio:implement to write back the post-batch prose (REQ:posture-stub-placeholder, REQ:stub-placeholder-done-lint) or revert Status to planning",
					t.Number,
				),
			})
		}
	}
	return out
}

// ----- P-006 plan document-status vocabulary -----

// canonicalPlanStatuses is the legal Plan document-status set per the upstream
// Status Vocabulary Feature (REQ:per-artifact-status-sets, Plan row). Title
// Case, single ASCII space between words. This is the Plan's own **Status:**
// line, distinct from a task's lowercase **Status:** (validated by P-004).
var canonicalPlanStatuses = map[string]bool{
	"Draft":       true,
	"In Review":   true,
	"Approved":    true,
	"Executing":   true,
	"Blocked":     true,
	"Implemented": true,
	"Failed":      true,
	"Rejected":    true,
	"Withdrawn":   true,
	"Superseded":  true,
	"Deprecated":  true,
}

// canonicalPlanStatusList is the legal set rendered for violation messages, in
// lifecycle order.
const canonicalPlanStatusList = "Draft, In Review, Approved, Executing, Blocked, Implemented, Failed, Rejected, Withdrawn, Superseded, Deprecated"

// lintP006 validates the single-file Plan's body **Status:** value against the
// canonical Plan status set. An absent **Status:** line emits nothing (presence
// is governed by other rules); only a present-but-out-of-set value is flagged.
func lintP006(p *plan.Plan, relPath string) []Violation {
	if p.StatusLine == 0 || canonicalPlanStatuses[p.Status] {
		return nil
	}
	return []Violation{{
		File:     relPath,
		Line:     p.StatusLine,
		Severity: "error",
		Rule:     "P-006",
		Message: fmt.Sprintf(
			"invalid plan **Status:** value %q (accepted: %s)",
			p.Status, canonicalPlanStatusList,
		),
	}}
}

// ----- P-007 execution-band derivation -----

// derivationEligibleStatuses is the set of body **Status:** values from which
// `lint --fix` may derive an execution band: the approval gate plus the four
// execution-band statuses (plan#req:execution-status-derived). Prep states
// (Draft, In Review) and dispositions (Rejected, Withdrawn, Superseded,
// Deprecated) are human-authored and MUST NEVER be overwritten.
var derivationEligibleStatuses = map[string]bool{
	"Approved":    true,
	"Executing":   true,
	"Blocked":     true,
	"Implemented": true,
	"Failed":      true,
}

// lintP007 reports execution-band drift on a single-file Plan: when the body
// **Status:** is derivation-eligible and the task-status rollup resolves to a
// determinate band that differs from the current body status, the band is
// stale. An indeterminate rollup, a prep/disposition body status, or a matching
// band emits nothing. The rule reads task status only — `--fix` (below) rewrites
// just the body **Status:** line.
func lintP007(p *plan.Plan, relPath string) []Violation {
	if p.StatusLine == 0 || !derivationEligibleStatuses[p.Status] {
		return nil
	}
	band, ok := p.DeriveExecutionBand()
	if !ok || band == p.Status {
		return nil
	}
	return []Violation{{
		File:      relPath,
		Line:      p.StatusLine,
		Severity:  "error",
		Rule:      "P-007",
		FixTarget: "P-007",
		Message: fmt.Sprintf(
			"plan **Status:** %q is stale: the task-status rollup derives %q (run --fix to reconcile)",
			p.Status, band,
		),
	}}
}

// fixP007 rewrites the body **Status:** line of every drifting single-file Plan
// to its derived execution band. It mirrors lintP007's guards exactly
// (derivation-eligible body status + determinate rollup + actual drift) so a
// second pass is a no-op. Only the **Status:** line is rewritten; the rest of
// the file is byte-preserved.
func fixP007(specRoot string) error {
	plansDir := filepath.Join(specRoot, "plans")
	if info, err := os.Stat(plansDir); err != nil || !info.IsDir() {
		return nil
	}
	entries, err := os.ReadDir(plansDir)
	if err != nil {
		return fmt.Errorf("reading plans dir: %w", err)
	}
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
			return fmt.Errorf("parsing plan %s: %w", planPath, parseErr)
		}
		if !p.HasPlanTitle || p.StatusLine == 0 || !derivationEligibleStatuses[p.Status] {
			continue
		}
		band, ok := p.DeriveExecutionBand()
		if !ok || band == p.Status {
			continue
		}
		if err := rewritePlanStatusLine(planPath, p.StatusLine, band); err != nil {
			return fmt.Errorf("fixing %s: %w", planPath, err)
		}
	}
	return nil
}

// rewritePlanStatusLine rewrites the 1-based statusLine of a plan file to
// `**Status:** <band>`, preserving every other line verbatim. statusLine comes
// from the parse pass over the same file, so it is always in range.
func rewritePlanStatusLine(path string, statusLine int, band string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	lines[statusLine-1] = "**Status:** " + band
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

// ----- P-008 implementation-commit-provenance ref format -----

// p008ShaRe matches a git commit hash: a 7–40 char hex string
// (implementation-commit-provenance#req:provenance-ref-format).
var p008ShaRe = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)

// p008BranchRe strips an OPTIONAL trailing ` (<branch>)` suffix from an
// **Implemented-by:** value. Group 1 captures the ref core (`<repo>@<sha>` or a
// bare `<sha>`); group 2 captures the non-empty branch.
var p008BranchRe = regexp.MustCompile(`^(.*\S)\s+\(([^()]+)\)$`)

// validImplementedByRef reports whether val is a well-formed implementation-commit
// provenance reference, validated SYNTACTICALLY ONLY. The referenced repo is never
// scanned and the filesystem/git is never touched (mirroring the P-005 cross-repo
// precedent). Accepted shapes: `<repo>@<sha>`, `<repo>@<sha> (<branch>)`, bare
// `<sha>`, and `<sha> (<branch>)`, where `<repo>` is a repo slug or a full clone
// URL (the LAST `@<sha>` segment is the sha; everything before it is the repo) and
// `<sha>` is 7–40 hex chars.
func validImplementedByRef(val string) bool {
	v := strings.TrimSpace(val)
	if v == "" {
		return false
	}
	// Strip an optional trailing ` (<branch>)` suffix; the regex guarantees a
	// non-empty branch (and a non-empty ref core, since group 1 ends in `\S`), so
	// an empty `()` leaves the suffix in place and fails the sha check below.
	if m := p008BranchRe.FindStringSubmatch(v); m != nil {
		v = strings.TrimSpace(m[1])
	}
	// The LAST `@` separates an optional repo from the sha. A bare sha has none.
	if idx := strings.LastIndex(v, "@"); idx >= 0 {
		if strings.TrimSpace(v[:idx]) == "" {
			return false // `@<sha>` with no repo before it
		}
		return p008ShaRe.MatchString(v[idx+1:])
	}
	return p008ShaRe.MatchString(v)
}

// lintP008 validates each task's optional `**Implemented-by:**` provenance value
// against the provenance-ref-format requirement. An absent field is valid
// (provenance is never required); a present-but-malformed value is flagged. The
// rule is purely syntactic — it never resolves or scans the referenced repo.
func lintP008(p *plan.Plan, relPath string) []Violation {
	var out []Violation
	for _, t := range p.Tasks {
		if !t.ImplementedByPresent {
			continue
		}
		if validImplementedByRef(t.ImplementationCommit) {
			continue
		}
		out = append(out, Violation{
			File:     relPath,
			Line:     t.ImplementedByLine,
			Severity: "error",
			Rule:     "P-008",
			Message: fmt.Sprintf(
				"Task %d: malformed **Implemented-by:** value %q (expected provenance-ref-format <repo>@<sha> or bare <sha>, optional trailing (<branch>); <sha> = 7-40 hex chars)",
				t.Number, t.ImplementationCommit,
			),
		})
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

// ----- P-009 cross-plan prerequisite validity -----

// lintP009 validates the optional `**Prerequisite Plans:**` header as a
// same-repository dependency graph. It is intentionally separate from P-005:
// Parent expresses composition, whereas prerequisites express execution order.
func lintP009(parsedPlans map[string]*plan.Plan, relPaths map[string]string) []Violation {
	var out []Violation
	edges := make(map[string][]string, len(parsedPlans))

	slugs := make([]string, 0, len(parsedPlans))
	for slug := range parsedPlans {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)

	for _, slug := range slugs {
		p := parsedPlans[slug]
		if p.PrerequisiteLine == 0 {
			continue
		}
		raw := strings.TrimSpace(p.PrerequisiteRaw)
		if raw == "" {
			out = append(out, Violation{
				File: relPaths[slug], Line: p.PrerequisiteLine, Severity: "error", Rule: "P-009",
				Message: "**Prerequisite Plans:** must be a comma-separated list of plan slugs or —",
			})
			continue
		}
		if raw == "—" {
			continue
		}

		seen := map[string]bool{}
		for _, part := range strings.Split(raw, ",") {
			prerequisite := strings.TrimSpace(part)
			if prerequisite == "" {
				out = append(out, Violation{
					File: relPaths[slug], Line: p.PrerequisiteLine, Severity: "error", Rule: "P-009",
					Message: "**Prerequisite Plans:** must not contain an empty list entry",
				})
				continue
			}
			if err := plan.ValidateSlug(prerequisite); err != nil {
				out = append(out, Violation{
					File: relPaths[slug], Line: p.PrerequisiteLine, Severity: "error", Rule: "P-009",
					Message: fmt.Sprintf("invalid prerequisite plan slug %q: %v", prerequisite, err),
				})
				continue
			}
			if seen[prerequisite] {
				out = append(out, Violation{
					File: relPaths[slug], Line: p.PrerequisiteLine, Severity: "error", Rule: "P-009",
					Message: fmt.Sprintf("**Prerequisite Plans:** lists %q more than once", prerequisite),
				})
				continue
			}
			seen[prerequisite] = true
			if prerequisite == slug {
				out = append(out, Violation{
					File: relPaths[slug], Line: p.PrerequisiteLine, Severity: "error", Rule: "P-009",
					Message: fmt.Sprintf("**Prerequisite Plans:** %q references the plan itself", slug),
				})
				continue
			}
			if _, ok := parsedPlans[prerequisite]; !ok {
				out = append(out, Violation{
					File: relPaths[slug], Line: p.PrerequisiteLine, Severity: "error", Rule: "P-009",
					Message: fmt.Sprintf("prerequisite plan %q does not resolve to spec/plans/%s.md", prerequisite, prerequisite),
				})
				continue
			}
			edges[slug] = append(edges[slug], prerequisite)
		}
	}

	if cycle := findPrerequisiteCycle(edges); len(cycle) > 0 {
		first := cycleFirst(cycle)
		out = append(out, Violation{
			File: relPaths[first], Line: parsedPlans[first].PrerequisiteLine, Severity: "error", Rule: "P-009",
			Message: fmt.Sprintf("**Prerequisite Plans:** graph forms a cycle: %s", renderCyclePath(cycle)),
		})
	}
	return out
}

func findPrerequisiteCycle(edges map[string][]string) []string {
	const (
		white = iota
		gray
		black
	)
	color := map[string]int{}
	stack := []string{}
	position := map[string]int{}
	var cycle []string

	var visit func(string) bool
	visit = func(slug string) bool {
		color[slug] = gray
		position[slug] = len(stack)
		stack = append(stack, slug)
		next := append([]string(nil), edges[slug]...)
		sort.Strings(next)
		for _, prerequisite := range next {
			switch color[prerequisite] {
			case white:
				if visit(prerequisite) {
					return true
				}
			case gray:
				cycle = append([]string(nil), stack[position[prerequisite]:]...)
				return true
			}
		}
		stack = stack[:len(stack)-1]
		delete(position, slug)
		color[slug] = black
		return false
	}

	starts := make([]string, 0, len(edges))
	for slug := range edges {
		starts = append(starts, slug)
	}
	sort.Strings(starts)
	for _, slug := range starts {
		if color[slug] == white && visit(slug) {
			return cycle
		}
	}
	return nil
}

// ----- P-010 coordination-branch reference format -----

// coordinationRe matches `<owner>/<repo>@<branch>`
// (plan#req:coordination-branch-format): `<owner>` and `<repo>` are simple
// GitHub-style identifiers (no '/' or '@' within either); `<branch>` is any
// non-empty, whitespace-free git ref name run to the end of the value (it MAY
// itself contain '/', e.g. `feature/foo`).
var coordinationRe = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9._-]*)/([A-Za-z0-9][A-Za-z0-9._-]*)@(\S+)$`)

// ParseCoordinationRef parses a **Coordination:** value into its
// owner/repo/branch components, validated SYNTACTICALLY ONLY — mirroring the
// P-005/P-008 cross-repo precedent, it never resolves or scans the named
// repository or checks whether the branch exists. ok is false for an empty or
// malformed value. Exported so the CLI's coordination-branch enforcement
// (`plan change-status`/`reconcile`, `task change-status --plan`) can parse
// the same field this rule validates, without duplicating the grammar.
func ParseCoordinationRef(val string) (owner, repo, branch string, ok bool) {
	m := coordinationRe.FindStringSubmatch(strings.TrimSpace(val))
	if m == nil {
		return "", "", "", false
	}
	return m[1], m[2], m[3], true
}

// lintP010 validates a plan's optional **Coordination:** header field against
// coordination-branch-format. An absent field is valid — the field is fully
// optional, so `lintP010` reports nothing when `p.CoordinationLine == 0` (the
// field never appeared at all). A present-but-malformed value is flagged.
// Purely syntactic: the named repo/branch is never resolved.
func lintP010(p *plan.Plan, relPath string) []Violation {
	if p.CoordinationLine == 0 {
		return nil
	}
	if _, _, _, ok := ParseCoordinationRef(p.Coordination); ok {
		return nil
	}
	return []Violation{{
		File:     relPath,
		Line:     p.CoordinationLine,
		Severity: "error",
		Rule:     "P-010",
		Message: fmt.Sprintf(
			"malformed **Coordination:** value %q (expected coordination-branch-format <owner>/<repo>@<branch>)",
			p.Coordination),
	}}
}
