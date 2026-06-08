package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// planRulesEnv is a minimal lint environment: spec root with features/ and
// plans/ subdirs.
type planRulesEnv struct {
	specRoot string
}

func newPlanRulesEnv(t *testing.T) *planRulesEnv {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"features", "plans"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return &planRulesEnv{specRoot: root}
}

func (e *planRulesEnv) writeFeature(t *testing.T, slug string, acSlugs ...string) {
	t.Helper()
	dir := filepath.Join(e.specRoot, "features", filepath.FromSlash(slug))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString("# Feature: " + slug + "\n\n")
	b.WriteString("**Status:** Approved\n\n")
	b.WriteString("## Behavior\n\n### Topic\n\n#### REQ: r\n\nrequirement.\n\n")
	b.WriteString("## Acceptance Criteria\n\n")
	for _, ac := range acSlugs {
		b.WriteString("### AC: " + ac + " (verifies REQ:r)\n\n")
		b.WriteString("**Given** g **When** w **Then** t\n\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (e *planRulesEnv) writePlan(t *testing.T, slug, body string) string {
	t.Helper()
	path := filepath.Join(e.specRoot, "plans", slug+".md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// runRules dispatches the plan-rules checker against the env.
func runRules(t *testing.T, e *planRulesEnv) []Violation {
	t.Helper()
	c := newPlanRulesChecker()
	v, err := c.check(e.specRoot)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// hasViolation returns the first violation matching rule whose message
// contains substr (or with substr==""). Returns nil when none match.
func hasViolation(vs []Violation, rule, substr string) *Violation {
	for i := range vs {
		if vs[i].Rule != rule {
			continue
		}
		if substr == "" || strings.Contains(vs[i].Message, substr) {
			return &vs[i]
		}
	}
	return nil
}

// AC: coverage-gap-flagged
func TestP001_CoverageGap(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writeFeature(t, "sample", "alpha", "beta", "gamma")
	e.writePlan(t, "sample", `# Plan: Sample

**Source Feature:** sample
**Mode:** full

## Tasks

### Task 1: First
**Verifies:** sample#ac:alpha

### Task 2: Second
**Verifies:** sample#ac:beta
**Depends-On:** 1
`)
	v := runRules(t, e)
	got := hasViolation(v, "P-001", "sample#ac:gamma")
	if got == nil {
		t.Fatalf("expected P-001 violation citing gamma, got %d violations: %+v", len(v), v)
	}
}

// AC: deferred-ac-counts-as-covered
func TestP001_DeferredCountsAsCovered(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writeFeature(t, "sample", "alpha", "beta", "gamma")
	e.writePlan(t, "sample", `# Plan: Sample

**Source Feature:** sample
**Mode:** full

## Tasks

### Task 1: First
**Verifies:** sample#ac:alpha

### Task 2: Second
**Verifies:** sample#ac:beta

## Deferred AC Coverage

- sample#ac:gamma — post-MVP scope
`)
	v := runRules(t, e)
	if got := hasViolation(v, "P-001", ""); got != nil {
		t.Fatalf("unexpected P-001 violation: %+v", got)
	}
}

// AC: stale-ac-flagged
func TestP002_StaleAC(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writeFeature(t, "sample", "alpha")
	e.writePlan(t, "sample", `# Plan: Sample

**Source Feature:** sample

## Tasks

### Task 1: First
**Verifies:** sample#ac:alpha

### Task 2: Second
**Verifies:** sample#ac:typo-slug
`)
	v := runRules(t, e)
	if got := hasViolation(v, "P-002", "typo-slug"); got == nil {
		t.Fatalf("expected P-002 with typo-slug; got: %+v", v)
	}
}

// AC: missing-source-feature
func TestP002_MissingSourceFeatureSingleViolation(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writePlan(t, "p", `# Plan: P

**Source Feature:** does/not/exist

## Tasks

### Task 1: A
**Verifies:** does/not/exist#ac:a

### Task 2: B
**Verifies:** does/not/exist#ac:b

### Task 3: C
**Verifies:** does/not/exist#ac:c
`)
	v := runRules(t, e)
	count := 0
	for _, x := range v {
		if x.Rule == "P-002" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 P-002 violation, got %d: %+v", count, v)
	}
}

// AC: cycle-detected-and-cited
func TestP003_CycleCited(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writeFeature(t, "f", "a", "b", "c")
	e.writePlan(t, "cycle", `# Plan: Cycle

**Source Feature:** f

## Tasks

### Task 1: A
**Verifies:** f#ac:a
**Depends-On:** 3

### Task 2: B
**Verifies:** f#ac:b
**Depends-On:** 1

### Task 3: C
**Verifies:** f#ac:c
**Depends-On:** 2
`)
	v := runRules(t, e)
	got := hasViolation(v, "P-003", "→")
	if got == nil {
		t.Fatalf("expected P-003 cycle violation citing path; got: %+v", v)
	}
	if !strings.Contains(got.Message, "cycle") {
		t.Fatalf("cycle word missing: %s", got.Message)
	}
}

// AC: dangling-depends-on
func TestP003_Dangling(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writeFeature(t, "f", "a", "b", "c", "d")
	e.writePlan(t, "p", `# Plan: P

**Source Feature:** f

## Tasks

### Task 1: A
**Verifies:** f#ac:a

### Task 2: B
**Verifies:** f#ac:b

### Task 3: C
**Verifies:** f#ac:c
**Depends-On:** 7

### Task 4: D
**Verifies:** f#ac:d
`)
	v := runRules(t, e)
	if got := hasViolation(v, "P-003", "Task 3 depends on nonexistent task 7"); got == nil {
		t.Fatalf("expected dangling violation; got: %+v", v)
	}
}

// AC: self-reference-flagged
func TestP003_SelfReference(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writeFeature(t, "f", "a", "b")
	e.writePlan(t, "p", `# Plan: P

**Source Feature:** f

## Tasks

### Task 1: A
**Verifies:** f#ac:a

### Task 2: B
**Verifies:** f#ac:b
**Depends-On:** 2
`)
	v := runRules(t, e)
	if got := hasViolation(v, "P-003", "Task 2 depends on itself"); got == nil {
		t.Fatalf("expected self-ref violation; got: %+v", v)
	}
}

// AC: non-linear-numbering-flagged
func TestP003_NonLinearNumbering(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writeFeature(t, "f", "a", "b", "c")
	e.writePlan(t, "p", `# Plan: P

**Source Feature:** f

## Tasks

### Task 1: A
**Verifies:** f#ac:a

### Task 3: B
**Verifies:** f#ac:b

### Task 5: C
**Verifies:** f#ac:c
`)
	v := runRules(t, e)
	got := hasViolation(v, "P-003", "linear")
	if got == nil {
		t.Fatalf("expected non-linear violation; got: %+v", v)
	}
	if !strings.Contains(got.Message, "Task 3") {
		t.Fatalf("message should cite Task 3; got: %s", got.Message)
	}
}

// AC: stub-done-placeholder-flagged
func TestP004_StubDonePlaceholder(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writeFeature(t, "f", "a", "b", "c")
	e.writePlan(t, "p", `# Plan: P

**Source Feature:** f
**Mode:** stub

## Tasks

### Task 1: A
**Verifies:** f#ac:a
**Status:** pending

<!-- implement: pending -->

### Task 2: B
**Verifies:** f#ac:b
**Status:** done

<!-- implement: pending -->

### Task 3: C
**Verifies:** f#ac:c
**Status:** pending

<!-- implement: pending -->
`)
	v := runRules(t, e)
	got := hasViolation(v, "P-004", "Task 2")
	if got == nil {
		t.Fatalf("expected P-004 on task 2; got: %+v", v)
	}
	if strings.Contains(got.Message, "Task 1") || strings.Contains(got.Message, "Task 3") {
		t.Fatalf("P-004 should only cite Task 2, got: %s", got.Message)
	}
	if !strings.Contains(got.Message, "posture-stub-placeholder") || !strings.Contains(got.Message, "stub-placeholder-done-lint") {
		t.Fatalf("P-004 message must reference both upstream REQs; got: %s", got.Message)
	}
}

// AC: stub-pending-placeholder-permitted
func TestP004_StubPendingPermitted(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writeFeature(t, "f", "a")
	e.writePlan(t, "p", `# Plan: P

**Source Feature:** f
**Mode:** stub

## Tasks

### Task 1: A
**Verifies:** f#ac:a
**Status:** pending

<!-- implement: pending -->
`)
	v := runRules(t, e)
	if got := hasViolation(v, "P-004", ""); got != nil {
		t.Fatalf("unexpected P-004: %+v", got)
	}
}

// AC: invalid-mode-value-flagged
func TestP004_InvalidModeValue(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writeFeature(t, "f", "a")
	e.writePlan(t, "p", `# Plan: P

**Source Feature:** f
**Mode:** sketch

## Tasks

### Task 1: A
**Verifies:** f#ac:a
`)
	v := runRules(t, e)
	got := hasViolation(v, "P-004", `"sketch"`)
	if got == nil {
		t.Fatalf("expected invalid-mode P-004; got: %+v", v)
	}
	if !strings.Contains(got.Message, "full") || !strings.Contains(got.Message, "stub") {
		t.Fatalf("message must cite accepted set; got: %s", got.Message)
	}
}

// AC: invalid-status-value-flagged
func TestP004_InvalidStatusValue(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writeFeature(t, "f", "a")
	e.writePlan(t, "p", `# Plan: P

**Source Feature:** f

## Tasks

### Task 1: A
**Verifies:** f#ac:a
**Status:** waiting
`)
	v := runRules(t, e)
	got := hasViolation(v, "P-004", `"waiting"`)
	if got == nil {
		t.Fatalf("expected invalid-status P-004; got: %+v", v)
	}
	if !strings.Contains(got.Message, "pending") || !strings.Contains(got.Message, "in-progress") {
		t.Fatalf("message must cite accepted set; got: %s", got.Message)
	}
}

// AC: defaults-when-fields-absent
func TestDefaults_NoFalsePositives(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writeFeature(t, "f", "a")
	e.writePlan(t, "p", `# Plan: P

**Source Feature:** f

## Tasks

### Task 1: A
**Verifies:** f#ac:a
`)
	v := runRules(t, e)
	for _, x := range v {
		if x.Rule == "P-003" || x.Rule == "P-004" {
			t.Fatalf("unexpected %s on default Plan: %+v", x.Rule, x)
		}
	}
}

// AC: skip-non-plan-files (parts of)
func TestSkip_NonPlanAndDirectoryPlans(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writeFeature(t, "f", "a")
	// Random non-plan markdown
	if err := os.WriteFile(filepath.Join(e.specRoot, "plans", "notes.md"),
		[]byte("# Random notes\n\nstuff\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Directory-form plan
	dir := filepath.Join(e.specRoot, "plans", "legacy")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"),
		[]byte("# Plan: Legacy\n\n**Status:** draft\n\n## Steps\n\n- do thing\n"),
		0o644); err != nil {
		t.Fatal(err)
	}
	v := runRules(t, e)
	for _, x := range v {
		if x.Rule == "P-001" || x.Rule == "P-002" || x.Rule == "P-003" || x.Rule == "P-004" {
			t.Fatalf("unexpected plan-rules violation on non-Plan path: %+v", x)
		}
	}
}

// AC: rules-in-default-suite (rule names + filter behavior)
func TestRulesRegisteredInAllRuleNames(t *testing.T) {
	all := AllRuleNames()
	for _, n := range []string{"P-001", "P-002", "P-003", "P-004"} {
		if !all[n] {
			t.Errorf("rule %s missing from allRuleNames", n)
		}
	}
}

func TestRulesFilteringByName(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writeFeature(t, "f", "a", "b")
	e.writePlan(t, "p", `# Plan: P

**Source Feature:** f

## Tasks

### Task 1: A
**Verifies:** f#ac:a
**Depends-On:** 1
`)
	// --rules P-003 should produce the self-ref violation and exclude the
	// P-001 coverage gap on `b`.
	vs, err := Lint(Options{SpecRoot: e.specRoot, Rules: []string{"P-003"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vs {
		if v.Rule != "P-003" {
			t.Fatalf("filter leaked %s: %+v", v.Rule, v)
		}
	}
	if len(vs) == 0 {
		t.Fatal("expected at least one P-003 violation under --rules P-003")
	}
}

// AC: the no-source fix is opt-in — the default fix pass (fixNoSource=false)
// must NOT rewrite a sourceless plan, leaving it as a P-002 error.
func TestPlanRulesNoSourceFixIsOptIn(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "plans"))
	planPath := filepath.Join(root, "plans", "p.md")
	writeFile(t, planPath, "# Plan: P\n\n**Status:** Draft\n\n## Tasks\n\n### Task 1: Do\n\n**Status:** pending\n")

	c := newPlanRulesChecker() // fixNoSource defaults to false
	if err := c.fix(root); err != nil {
		t.Fatalf("fix: %v", err)
	}
	data, _ := os.ReadFile(planPath)
	if strings.Contains(string(data), "**Source:** none") {
		t.Errorf("default fix pass must not add **Source:** none (no-source is opt-in):\n%s", data)
	}
}

// AC: with the opt-in enabled, a sourceless plan gains `**Source:** none`,
// inserted after the Status line, and the fix is idempotent.
func TestPlanRulesNoSourceFixOptIn(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "plans"))
	planPath := filepath.Join(root, "plans", "p.md")
	writeFile(t, planPath, "# Plan: P\n\n**Status:** Draft\n\n## Tasks\n\n### Task 1: Do\n\n**Status:** pending\n")

	c := newPlanRulesChecker()
	c.fixNoSource = true
	if err := c.fix(root); err != nil {
		t.Fatalf("fix: %v", err)
	}
	data, _ := os.ReadFile(planPath)
	if !strings.Contains(string(data), "**Status:** Draft\n**Source:** none") {
		t.Errorf("expected **Source:** none after Status line:\n%s", data)
	}
	if err := c.fix(root); err != nil {
		t.Fatalf("fix (2nd pass): %v", err)
	}
	data2, _ := os.ReadFile(planPath)
	if strings.Count(string(data2), "**Source:** none") != 1 {
		t.Errorf("fix not idempotent (expected exactly one source line):\n%s", data2)
	}
}

// AC: an unrecognized **Source:** value is NOT a fully-absent source line, so
// the opt-in fix leaves it untouched (it stays a hard P-002 error).
func TestPlanRulesNoSourceFixSkipsUnrecognizedValue(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "plans"))
	planPath := filepath.Join(root, "plans", "p.md")
	writeFile(t, planPath, "# Plan: P\n\n**Status:** Draft\n**Source:** garbage\n\n## Tasks\n\n### Task 1: Do\n\n**Status:** pending\n")

	c := newPlanRulesChecker()
	c.fixNoSource = true
	if err := c.fix(root); err != nil {
		t.Fatalf("fix: %v", err)
	}
	data, _ := os.ReadFile(planPath)
	if strings.Contains(string(data), "**Source:** none") {
		t.Errorf("unrecognized **Source:** value must not be rewritten to none:\n%s", data)
	}
}
