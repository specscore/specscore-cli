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
**Status:** planning

<!-- implement: pending -->

### Task 2: B
**Verifies:** f#ac:b
**Status:** complete

<!-- implement: pending -->

### Task 3: C
**Verifies:** f#ac:c
**Status:** planning

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
**Status:** planning

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
	if !strings.Contains(got.Message, "in_progress") || !strings.Contains(got.Message, "planning") {
		t.Fatalf("message must cite the canonical enum; got: %s", got.Message)
	}
}

// AC enum-is-sole-vocabulary: a non-enum, non-legacy token (`shipped`) is
// rejected as not a valid task status, with no canonical mapping offered.
func TestP004_NonEnumStatusRejectedNoMapping(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writeFeature(t, "f", "a")
	e.writePlan(t, "p", `# Plan: P

**Source Feature:** f

## Tasks

### Task 1: A
**Verifies:** f#ac:a
**Status:** shipped
`)
	got := hasViolation(runRules(t, e), "P-004", `"shipped"`)
	if got == nil {
		t.Fatal("expected P-004 for non-enum status `shipped`")
	}
	if !strings.Contains(got.Message, "not a valid task status") {
		t.Fatalf("message must state value is not valid; got: %s", got.Message)
	}
	if got.FixTarget != "" {
		t.Fatalf("non-legacy token must offer no fix target; got: %q", got.FixTarget)
	}
	// No canonical mapping should be offered (no "use canonical").
	if strings.Contains(got.Message, "use canonical") {
		t.Fatalf("non-legacy token must not offer a mapping; got: %s", got.Message)
	}
}

// AC lint-flags-legacy: each legacy token is flagged with its canonical
// replacement, and a fix target is offered.
func TestP004_LegacyStatusFlaggedWithReplacement(t *testing.T) {
	cases := []struct{ legacy, canonical string }{
		{"pending", "planning"},
		{"done", "complete"},
		{"in-progress", "in_progress"},
	}
	for _, tc := range cases {
		e := newPlanRulesEnv(t)
		e.writeFeature(t, "f", "a")
		e.writePlan(t, "p", `# Plan: P

**Source Feature:** f

## Tasks

### Task 1: A
**Verifies:** f#ac:a
**Status:** `+tc.legacy+`
`)
		got := hasViolation(runRules(t, e), "P-004", `"`+tc.legacy+`"`)
		if got == nil {
			t.Fatalf("expected P-004 for legacy token %q", tc.legacy)
		}
		if !strings.Contains(got.Message, `"`+tc.canonical+`"`) {
			t.Fatalf("message must name canonical %q; got: %s", tc.canonical, got.Message)
		}
		if got.FixTarget != "P-004" {
			t.Fatalf("legacy token must offer P-004 fix target; got: %q", got.FixTarget)
		}
	}
}

// AC enum-is-sole-vocabulary: every canonical enum value is accepted with no
// P-004 status violation.
func TestP004_CanonicalStatusAccepted(t *testing.T) {
	for _, st := range []string{"planning", "queued", "in_progress", "blocked", "complete", "failed", "aborted"} {
		e := newPlanRulesEnv(t)
		e.writeFeature(t, "f", "a")
		e.writePlan(t, "p", `# Plan: P

**Source Feature:** f

## Tasks

### Task 1: A
**Verifies:** f#ac:a
**Status:** `+st+`
`)
		if got := hasViolation(runRules(t, e), "P-004", "Status"); got != nil {
			t.Fatalf("canonical status %q must not be flagged; got: %+v", st, got)
		}
	}
}

func TestP009_PrerequisitePlansResolveAndExposeCycles(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		e := newPlanRulesEnv(t)
		e.writePlan(t, "foundation", "# Plan: Foundation\n\n**Source:** none\n")
		e.writePlan(t, "application", "# Plan: Application\n\n**Source:** none\n**Prerequisite Plans:** foundation\n")
		if got := hasViolation(runRules(t, e), "P-009", ""); got != nil {
			t.Fatalf("valid prerequisite must not be flagged: %+v", got)
		}
	})

	t.Run("dangling", func(t *testing.T) {
		e := newPlanRulesEnv(t)
		e.writePlan(t, "application", "# Plan: Application\n\n**Source:** none\n**Prerequisite Plans:** missing\n")
		if got := hasViolation(runRules(t, e), "P-009", "does not resolve"); got == nil {
			t.Fatal("expected dangling prerequisite violation")
		}
	})

	t.Run("self", func(t *testing.T) {
		e := newPlanRulesEnv(t)
		e.writePlan(t, "application", "# Plan: Application\n\n**Source:** none\n**Prerequisite Plans:** application\n")
		if got := hasViolation(runRules(t, e), "P-009", "itself"); got == nil {
			t.Fatal("expected self prerequisite violation")
		}
	})

	t.Run("cycle", func(t *testing.T) {
		e := newPlanRulesEnv(t)
		e.writePlan(t, "alpha", "# Plan: Alpha\n\n**Source:** none\n**Prerequisite Plans:** beta\n")
		e.writePlan(t, "beta", "# Plan: Beta\n\n**Source:** none\n**Prerequisite Plans:** alpha\n")
		if got := hasViolation(runRules(t, e), "P-009", "alpha → beta → alpha"); got == nil {
			t.Fatal("expected prerequisite cycle violation with the full cycle path")
		}
	})
}

// AC lint-fix-migrates-legacy: --fix rewrites each legacy task **Status:**
// value-for-value to canonical, leaving all other content untouched, and is
// idempotent.
func TestP004_FixMigratesLegacyTokens(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writeFeature(t, "f", "a", "b")
	path := e.writePlan(t, "p", `# Plan: P

**Source Feature:** f

## Tasks

### Task 1: A
**Verifies:** f#ac:a
**Status:** done

### Task 2: B
**Verifies:** f#ac:b
**Depends-On:** 1
**Status:** pending
`)
	before, _ := os.ReadFile(path)

	c := newPlanRulesChecker()
	c.fixP004Legacy = true
	if err := c.fix(e.specRoot); err != nil {
		t.Fatalf("fix: %v", err)
	}
	after, _ := os.ReadFile(path)

	want := strings.Replace(string(before), "**Status:** done", "**Status:** complete", 1)
	want = strings.Replace(want, "**Status:** pending", "**Status:** planning", 1)
	if string(after) != want {
		t.Fatalf("fix changed more than the status values:\nwant:\n%q\ngot:\n%q", want, after)
	}

	// Post-fix: no legacy P-004 violations remain.
	if got := hasViolation(runRules(t, e), "P-004", "Status"); got != nil {
		t.Fatalf("post-fix lint still reports a status P-004: %+v", got)
	}
	// Idempotent: a second pass changes nothing.
	if err := c.fix(e.specRoot); err != nil {
		t.Fatalf("fix (2nd): %v", err)
	}
	after2, _ := os.ReadFile(path)
	if string(after2) != string(after) {
		t.Fatalf("fix not idempotent:\n%s", after2)
	}
}

// AC enum-is-sole-vocabulary: --fix leaves a non-legacy invalid token alone.
func TestP004_FixSkipsNonLegacyToken(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writeFeature(t, "f", "a")
	path := e.writePlan(t, "p", `# Plan: P

**Source Feature:** f

## Tasks

### Task 1: A
**Verifies:** f#ac:a
**Status:** shipped
`)
	before, _ := os.ReadFile(path)
	c := newPlanRulesChecker()
	c.fixP004Legacy = true
	if err := c.fix(e.specRoot); err != nil {
		t.Fatalf("fix: %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatalf("fix must not touch a non-legacy token:\n%s", after)
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
	writeFile(t, planPath, "# Plan: P\n\n**Status:** Draft\n\n## Tasks\n\n### Task 1: Do\n\n**Status:** planning\n")

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
	writeFile(t, planPath, "# Plan: P\n\n**Status:** Draft\n\n## Tasks\n\n### Task 1: Do\n\n**Status:** planning\n")

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
	writeFile(t, planPath, "# Plan: P\n\n**Status:** Draft\n**Source:** garbage\n\n## Tasks\n\n### Task 1: Do\n\n**Status:** planning\n")

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

// ----- P-007 execution-band derivation -----

// p007Plan writes a single-file plan whose body Status is bodyStatus and whose
// tasks carry the given per-task statuses (one `### Task N:` per status).
func (e *planRulesEnv) p007Plan(t *testing.T, slug, bodyStatus string, taskStatuses ...string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("# Plan: " + slug + "\n\n")
	b.WriteString("**Status:** " + bodyStatus + "\n")
	b.WriteString("**Source:** none\n\n")
	b.WriteString("## Tasks\n\n")
	for i, s := range taskStatuses {
		b.WriteString("### Task " + itoa(i+1) + ": T\n\n")
		b.WriteString("**Status:** " + s + "\n\n")
	}
	return e.writePlan(t, slug, b.String())
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// AC: derive Executing from an in-progress task when body Status drifts.
func TestP007_DeriveExecuting(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.p007Plan(t, "p", "Approved", "in_progress", "planning")
	v := hasViolation(runRules(t, e), "P-007", "Executing")
	if v == nil {
		t.Fatal("expected a P-007 violation deriving Executing")
	}
	if v.Severity != "error" {
		t.Errorf("severity = %q, want error", v.Severity)
	}
}

// AC: derive Blocked when a task is blocked and none in-progress/failed.
func TestP007_DeriveBlocked(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.p007Plan(t, "p", "Approved", "complete", "blocked")
	if hasViolation(runRules(t, e), "P-007", "Blocked") == nil {
		t.Fatal("expected a P-007 violation deriving Blocked")
	}
}

// AC: derive Implemented when all tasks are done.
func TestP007_DeriveImplemented(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.p007Plan(t, "p", "Approved", "complete", "complete")
	if hasViolation(runRules(t, e), "P-007", "Implemented") == nil {
		t.Fatal("expected a P-007 violation deriving Implemented")
	}
}

// AC: derive Failed from a failed/aborted task (precedence over all).
func TestP007_DeriveFailed(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.p007Plan(t, "p", "Executing", "failed", "in_progress")
	if hasViolation(runRules(t, e), "P-007", "Failed") == nil {
		t.Fatal("expected a P-007 violation deriving Failed")
	}
}

// AC: an indeterminate rollup (a pending task) emits nothing even from a
// derivation-eligible body status.
func TestP007_IndeterminateNoOp(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.p007Plan(t, "p", "Approved", "complete", "planning")
	if v := hasViolation(runRules(t, e), "P-007", ""); v != nil {
		t.Fatalf("indeterminate rollup must emit no P-007: %+v", v)
	}
}

// AC: no tasks at all is indeterminate.
func TestP007_NoTasksNoOp(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.p007Plan(t, "p", "Approved")
	if v := hasViolation(runRules(t, e), "P-007", ""); v != nil {
		t.Fatalf("a task-less plan must emit no P-007: %+v", v)
	}
}

// AC: when the derived band already equals the body status, no drift.
func TestP007_NoDriftNoOp(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.p007Plan(t, "p", "Implemented", "complete", "complete")
	if v := hasViolation(runRules(t, e), "P-007", ""); v != nil {
		t.Fatalf("matching band must emit no P-007: %+v", v)
	}
}

// AC: prep states (Draft, In Review) are human-authored and never overwritten,
// even when the rollup is determinate.
func TestP007_PrepStatesNeverDerived(t *testing.T) {
	for _, st := range []string{"Draft", "In Review"} {
		e := newPlanRulesEnv(t)
		e.p007Plan(t, "p", st, "complete", "complete")
		if v := hasViolation(runRules(t, e), "P-007", ""); v != nil {
			t.Fatalf("body status %q must not be derived: %+v", st, v)
		}
	}
}

// AC: disposition states are human-authored and never overwritten.
func TestP007_DispositionStatesNeverDerived(t *testing.T) {
	for _, st := range []string{"Rejected", "Withdrawn", "Superseded", "Deprecated"} {
		e := newPlanRulesEnv(t)
		e.p007Plan(t, "p", st, "complete", "complete")
		if v := hasViolation(runRules(t, e), "P-007", ""); v != nil {
			t.Fatalf("body status %q must not be derived: %+v", st, v)
		}
	}
}

// AC: --fix rewrites only the body **Status:** line to the derived band and is
// idempotent (a second pass is a no-op).
func TestP007_FixRewritesAndIdempotent(t *testing.T) {
	e := newPlanRulesEnv(t)
	path := e.p007Plan(t, "p", "Approved", "complete", "complete")
	before, _ := os.ReadFile(path)

	c := newPlanRulesChecker()
	c.fixP007 = true
	if err := c.fix(e.specRoot); err != nil {
		t.Fatalf("fix: %v", err)
	}
	after, _ := os.ReadFile(path)
	if !strings.Contains(string(after), "**Status:** Implemented\n") {
		t.Fatalf("expected body status rewritten to Implemented:\n%s", after)
	}
	if strings.Contains(string(after), "**Status:** Approved\n") {
		t.Fatalf("old body status must be gone:\n%s", after)
	}
	// Byte-preserving otherwise: only the status word changed.
	wantAfter := strings.Replace(string(before), "**Status:** Approved\n", "**Status:** Implemented\n", 1)
	if string(after) != wantAfter {
		t.Fatalf("fix changed more than the status line:\nwant:\n%q\ngot:\n%q", wantAfter, after)
	}
	// No drift now -> no violation.
	if v := hasViolation(runRules(t, e), "P-007", ""); v != nil {
		t.Fatalf("post-fix lint still reports P-007: %+v", v)
	}
	// Idempotent: a second fix pass changes nothing.
	if err := c.fix(e.specRoot); err != nil {
		t.Fatalf("fix (2nd): %v", err)
	}
	after2, _ := os.ReadFile(path)
	if string(after2) != string(after) {
		t.Fatalf("fix not idempotent:\n%s", after2)
	}
}

// AC: --fix never touches a prep/disposition body status even with a
// determinate rollup.
func TestP007_FixSkipsPrepAndDisposition(t *testing.T) {
	e := newPlanRulesEnv(t)
	path := e.p007Plan(t, "p", "Draft", "complete", "complete")
	before, _ := os.ReadFile(path)
	c := newPlanRulesChecker()
	c.fixP007 = true
	if err := c.fix(e.specRoot); err != nil {
		t.Fatalf("fix: %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatalf("fix must not touch a Draft plan:\n%s", after)
	}
}

// AC: --fix on an indeterminate plan changes nothing.
func TestP007_FixIndeterminateNoOp(t *testing.T) {
	e := newPlanRulesEnv(t)
	path := e.p007Plan(t, "p", "Approved", "complete", "planning")
	before, _ := os.ReadFile(path)
	c := newPlanRulesChecker()
	c.fixP007 = true
	if err := c.fix(e.specRoot); err != nil {
		t.Fatalf("fix: %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatalf("fix must not touch an indeterminate plan:\n%s", after)
	}
}

// AC: P-007 is registered in the canonical rule-name set and is filterable.
func TestP007_RegisteredAndFilterable(t *testing.T) {
	if !AllRuleNames()["P-007"] {
		t.Fatal("P-007 missing from AllRuleNames()")
	}
}

// p008Plan writes a minimal, otherwise-clean single-file plan whose sole task
// carries the given **Implemented-by:** value (omitted entirely when impl == "").
func (e *planRulesEnv) p008Plan(t *testing.T, impl string) {
	t.Helper()
	e.writeFeature(t, "sample", "alpha")
	var b strings.Builder
	b.WriteString("# Plan: Sample\n\n**Source Feature:** sample\n**Mode:** full\n\n## Tasks\n\n### Task 1: First\n**Verifies:** sample#ac:alpha\n**Status:** complete\n")
	if impl != "" {
		b.WriteString("**Implemented-by:** " + impl + "\n")
	}
	e.writePlan(t, "sample", b.String())
}

// AC: well-formed-ref-accepted — a `<repo>@<sha> (<branch>)` ref is accepted on
// shape alone and the linter does NOT scan the referenced repo.
func TestP008_WellFormedRefAccepted(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.p008Plan(t, "backstage@a1b2c3d (feature/foo)")
	// No `backstage` repo exists anywhere under the env; acceptance must rely on
	// shape alone, never a filesystem/git scan.
	if _, err := os.Stat(filepath.Join(e.specRoot, "backstage")); !os.IsNotExist(err) {
		t.Fatalf("test setup: backstage path should not exist")
	}
	v := runRules(t, e)
	if got := hasViolation(v, "P-008", ""); got != nil {
		t.Fatalf("unexpected P-008 violation: %+v", got)
	}
}

// AC: malformed-ref-rejected — `not a commit ref` is reported citing the
// provenance-ref-format requirement and naming the offending value.
func TestP008_MalformedRefRejected(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.p008Plan(t, "not a commit ref")
	v := runRules(t, e)
	got := hasViolation(v, "P-008", "not a commit ref")
	if got == nil {
		t.Fatalf("expected P-008 violation naming the value, got %d violations: %+v", len(v), v)
	}
	if !strings.Contains(got.Message, "provenance-ref-format") {
		t.Fatalf("violation should cite provenance-ref-format, got: %q", got.Message)
	}
}

// AC: same-repo-bare-sha — a bare `<sha>` with no repo prefix is accepted.
func TestP008_SameRepoBareSha(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.p008Plan(t, "a1b2c3d")
	v := runRules(t, e)
	if got := hasViolation(v, "P-008", ""); got != nil {
		t.Fatalf("unexpected P-008 violation for bare sha: %+v", got)
	}
}

func TestP008_AbsentFieldNoViolation(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.p008Plan(t, "") // no **Implemented-by:** line at all
	v := runRules(t, e)
	if got := hasViolation(v, "P-008", ""); got != nil {
		t.Fatalf("absent provenance must not violate, got: %+v", got)
	}
}

func TestP008_EmptyAfterLabelRejected(t *testing.T) {
	e := newPlanRulesEnv(t)
	// `**Implemented-by:**` present with an empty value.
	e.writeFeature(t, "sample", "alpha")
	e.writePlan(t, "sample", "# Plan: Sample\n\n**Source Feature:** sample\n**Mode:** full\n\n## Tasks\n\n### Task 1: First\n**Verifies:** sample#ac:alpha\n**Status:** complete\n**Implemented-by:**\n")
	v := runRules(t, e)
	if got := hasViolation(v, "P-008", ""); got == nil {
		t.Fatalf("empty-after-label must violate, got %d violations: %+v", len(v), v)
	}
}

// TestValidImplementedByRef exercises the purely-syntactic ref validator over
// every accepted and malformed shape. The validator takes only a string and
// touches no filesystem/git, which is the structural guarantee that P-008 never
// scans the referenced repo.
func TestValidImplementedByRef(t *testing.T) {
	accepted := []string{
		"a1b2c3d",                                  // bare sha (min length)
		"a1b2c3d (feature/foo)",                    // bare sha + branch
		"backstage@a1b2c3d",                        // repo slug + sha
		"backstage@a1b2c3d (feature/foo)",          // repo slug + sha + branch
		"https://github.com/org/repo@a1b2c3d",      // https clone URL + sha
		"git@host:org/repo.git@a1b2c3d (main)",     // ssh clone URL + sha + branch
		"abcdefabcdefabcdefabcdefabcdefabcdef0123", // 40-char sha
	}
	for _, ref := range accepted {
		if !validImplementedByRef(ref) {
			t.Errorf("expected accepted: %q", ref)
		}
	}
	malformed := []string{
		"",                 // empty after label
		"   ",              // whitespace only
		"not a commit ref", // free text
		"a1b2c3",           // too short (6 hex)
		"abcdefabcdefabcdefabcdefabcdefabcdef01234", // too long (41 hex)
		"ghijklm",           // non-hex
		"backstage@",        // repo, missing sha
		"@a1b2c3d",          // missing repo before sha
		"backstage@xyz123g", // repo + non-hex sha
		"a1b2c3d ()",        // empty branch parens
	}
	for _, ref := range malformed {
		if validImplementedByRef(ref) {
			t.Errorf("expected malformed: %q", ref)
		}
	}
}
