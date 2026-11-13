package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Tests for the opt-in feature-storage-layout / feature-storage-layout-pending
// rules (cli/spec/lint/storage-layout).

const slGuide = "https://example.com/storage-design"

const slOptIn = "project:\n  title: T\nstorage_layout:\n  required: true\n  guide_url: " + slGuide + "\n"

func slFeature(status, section string) string {
	return "---\nformat: https://specscore.md/feature-specification\nstatus: " + status + "\n---\n\n" +
		"# Feature: X\n\n**Status:** " + status + "\n**Source Ideas:** —\n\n" +
		"## Summary\n\nx\n\n## Behavior\n\nx\n\n" + section +
		"## Acceptance Criteria\n\nNot defined yet.\n\n## Open Questions\n\nNone at this time.\n\n" +
		"---\n*This document follows the https://specscore.md/feature-specification*\n"
}

func slCheck(t *testing.T, specRoot string, explicit bool) []Violation {
	t.Helper()
	vs, err := newStorageLayoutChecker("", explicit).check(specRoot)
	if err != nil {
		t.Fatalf("check error: %v", err)
	}
	return vs
}

func slOnly(t *testing.T, vs []Violation, rule, severity, msgPart string) Violation {
	t.Helper()
	if len(vs) != 1 {
		t.Fatalf("expected exactly one violation, got %d: %+v", len(vs), vs)
	}
	v := vs[0]
	if v.Rule != rule || v.Severity != severity {
		t.Fatalf("got rule=%s severity=%s, want %s/%s (%s)", v.Rule, v.Severity, rule, severity, v.Message)
	}
	if !strings.Contains(v.Message, msgPart) {
		t.Fatalf("message %q does not contain %q", v.Message, msgPart)
	}
	if !strings.HasSuffix(v.Message, "See "+slGuide) {
		t.Fatalf("message %q does not end with the configured guide link", v.Message)
	}
	if strings.Contains(v.Message, "\n") {
		t.Fatalf("message must be one line: %q", v.Message)
	}
	return v
}

// --- grandfathering: off unless opted in -----------------------------------

func TestStorageLayout_OffWithoutConfig(t *testing.T) {
	sr := gradeSpecRoot(t, "project:\n  title: T\n")
	writeMD(t, sr, "features/foo/README.md", slFeature("Stable", ""))
	if vs := slCheck(t, sr, false); len(vs) != 0 {
		t.Fatalf("rule must be off without storage_layout.required, got %v", vs)
	}
}

func TestStorageLayout_OffWithUnreadableConfig(t *testing.T) {
	sr := gradeSpecRoot(t, "")
	writeMD(t, sr, "features/foo/README.md", slFeature("Stable", ""))
	if vs := slCheck(t, sr, false); len(vs) != 0 {
		t.Fatalf("rule must be off when specscore.yaml is unreadable, got %v", vs)
	}
}

func TestStorageLayout_OffInDefaultLintRun(t *testing.T) {
	sr := gradeSpecRoot(t, "project:\n  title: T\n")
	writeMD(t, sr, "features/foo/README.md", slFeature("Stable", ""))
	vs, err := Lint(Options{SpecRoot: sr, Severity: "info"})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vs {
		if strings.HasPrefix(v.Rule, "feature-storage-layout") {
			t.Fatalf("default run must not report %s without opt-in: %+v", v.Rule, v)
		}
	}
}

func TestStorageLayout_ExplicitRulesSelectionRunsWithoutConfig(t *testing.T) {
	sr := gradeSpecRoot(t, "project:\n  title: T\n")
	writeMD(t, sr, "features/foo/README.md", slFeature("Draft", ""))
	vs, err := Lint(Options{SpecRoot: sr, Rules: []string{"feature-storage-layout"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 1 || vs[0].Rule != "feature-storage-layout" {
		t.Fatalf("explicit --rules must run the rule, got %+v", vs)
	}
	// No guide URL configured: the message carries no "See" suffix.
	if strings.Contains(vs[0].Message, "See ") {
		t.Fatalf("no guide_url configured, message must not link one: %q", vs[0].Message)
	}
	if vs[0].FixTarget != "feature-storage-layout" {
		t.Fatalf("missing-section finding must carry its fix target, got %q", vs[0].FixTarget)
	}
}

func TestStorageLayout_OptedInDefaultLintRun(t *testing.T) {
	sr := gradeSpecRoot(t, slOptIn)
	writeMD(t, sr, "features/foo/README.md", slFeature("Draft", ""))
	vs, err := Lint(Options{SpecRoot: sr, Rules: []string{"feature-storage-layout", "feature-storage-layout-pending"}})
	if err != nil {
		t.Fatal(err)
	}
	slOnly(t, vs, "feature-storage-layout", "error", "no `## Storage layout` section")
}

// --- accepted forms ---------------------------------------------------------

func TestStorageLayout_NoneStatementAccepted(t *testing.T) {
	for _, stmt := range []string{
		"None — this feature stores nothing.",
		"None - this feature stores nothing.",
		"none -- this feature stores nothing",
		"NONE – this feature stores nothing.",
	} {
		sr := gradeSpecRoot(t, slOptIn)
		writeMD(t, sr, "features/foo/README.md", slFeature("Stable", "## Storage layout\n\n"+stmt+"\n\n"))
		if vs := slCheck(t, sr, false); len(vs) != 0 {
			t.Fatalf("%q must be accepted, got %v", stmt, vs)
		}
	}
}

func TestStorageLayout_ApprovedLayoutAccepted(t *testing.T) {
	sr := gradeSpecRoot(t, slOptIn)
	writeMD(t, sr, "features/foo/README.md", slFeature("Stable",
		"## STORAGE LAYOUT\n\n<!-- prompts -->\n| Record | Path |\n|---|---|\n| room | spaces/{s}/ext/rooms/{id} |\n\n"+
			"**Approved by:** \"yes, build it\" — owner, 2026-11-13\n\n"))
	if vs := slCheck(t, sr, false); len(vs) != 0 {
		t.Fatalf("approved layout must be accepted, got %v", vs)
	}
}

// --- errors -----------------------------------------------------------------

func TestStorageLayout_MissingSection(t *testing.T) {
	sr := gradeSpecRoot(t, slOptIn)
	writeMD(t, sr, "features/foo/README.md", slFeature("Draft", ""))
	v := slOnly(t, slCheck(t, sr, false), "feature-storage-layout", "error", "no `## Storage layout` section")
	if v.File != filepath.Join("features", "foo", "README.md") || v.Line != 8 {
		t.Fatalf("finding must cite the Status line of the feature, got %s:%d", v.File, v.Line)
	}
}

func TestStorageLayout_MissingSectionNoStatusLine(t *testing.T) {
	sr := gradeSpecRoot(t, slOptIn)
	writeMD(t, sr, "features/foo/README.md", "# Feature: X\n\n## Summary\n\nx\n")
	v := slOnly(t, slCheck(t, sr, false), "feature-storage-layout", "error", "no `## Storage layout` section")
	if v.Line != 1 {
		t.Fatalf("no Status line: finding must cite line 1, got %d", v.Line)
	}
}

func TestStorageLayout_HeadingInsideCodeFenceIgnored(t *testing.T) {
	sr := gradeSpecRoot(t, slOptIn)
	writeMD(t, sr, "features/foo/README.md", slFeature("Draft",
		"```markdown\n## Storage layout\n\nNone — this feature stores nothing.\n```\n\n"))
	slOnly(t, slCheck(t, sr, false), "feature-storage-layout", "error", "no `## Storage layout` section")
}

func TestStorageLayout_EmptySection(t *testing.T) {
	sr := gradeSpecRoot(t, slOptIn)
	writeMD(t, sr, "features/foo/README.md", slFeature("Draft", "## Storage layout\n\n<!-- nothing\nyet -->\n\n"))
	slOnly(t, slCheck(t, sr, false), "feature-storage-layout", "error", "`## Storage layout` is empty")
}

func TestStorageLayout_NoApprovedByLine(t *testing.T) {
	sr := gradeSpecRoot(t, slOptIn)
	writeMD(t, sr, "features/foo/README.md", slFeature("Draft", "## Storage layout\n\n- rooms/{id}\n\n"))
	slOnly(t, slCheck(t, sr, false), "feature-storage-layout", "error", "no `**Approved by:**` line")
}

func TestStorageLayout_EmptyApprovedBy(t *testing.T) {
	sr := gradeSpecRoot(t, slOptIn)
	writeMD(t, sr, "features/foo/README.md", slFeature("Draft", "## Storage layout\n\n- rooms/{id}\n\n**Approved by:**   \n\n"))
	slOnly(t, slCheck(t, sr, false), "feature-storage-layout", "error", "`**Approved by:**` is empty")
}

func TestStorageLayout_ApprovalWithoutLayout(t *testing.T) {
	sr := gradeSpecRoot(t, slOptIn)
	writeMD(t, sr, "features/foo/README.md", slFeature("Draft", "## Storage layout\n\n**Approved by:** owner, 2026-11-13\n\n"))
	slOnly(t, slCheck(t, sr, false), "feature-storage-layout", "error", "describes no layout")
}

func TestStorageLayout_PendingOnApprovedOrLaterIsError(t *testing.T) {
	for _, status := range []string{"Approved", "implementing", "Stable"} {
		sr := gradeSpecRoot(t, slOptIn)
		writeMD(t, sr, "features/foo/README.md", slFeature(status, "## Storage layout\n\n- rooms/{id}\n\n**Approved by:** pending\n\n"))
		slOnly(t, slCheck(t, sr, false), "feature-storage-layout", "error", "is "+status+" but its storage layout is not approved")
	}
}

// --- warning ----------------------------------------------------------------

func TestStorageLayout_PendingOnDraftIsWarning(t *testing.T) {
	for _, status := range []string{"Draft", "In Review", "Amending"} {
		sr := gradeSpecRoot(t, slOptIn)
		writeMD(t, sr, "features/foo/README.md", slFeature(status, "## Storage layout\n\n**Approved by:** Pending (asked 2026-11-13)\n\n"))
		v := slOnly(t, slCheck(t, sr, false), "feature-storage-layout-pending", "warning", "storage layout not yet approved")
		if v.Line != 21 {
			t.Fatalf("pending finding must cite the Approved by line, got %d", v.Line)
		}
	}
}

func TestStorageLayout_PendingWarningHiddenAtDefaultSeverity(t *testing.T) {
	sr := gradeSpecRoot(t, slOptIn)
	writeMD(t, sr, "features/foo/README.md", slFeature("Draft", "## Storage layout\n\n**Approved by:** pending\n\n"))
	vs, err := Lint(Options{SpecRoot: sr, Severity: "error", Rules: []string{"feature-storage-layout", "feature-storage-layout-pending"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 0 {
		t.Fatalf("a pending draft must not fail the default --severity error run, got %v", vs)
	}
}

// --- exemptions -------------------------------------------------------------

func TestStorageLayout_RejectedAndDeprecatedExempt(t *testing.T) {
	for _, status := range []string{"Rejected", "Deprecated"} {
		sr := gradeSpecRoot(t, slOptIn)
		writeMD(t, sr, "features/foo/README.md", slFeature(status, ""))
		if vs := slCheck(t, sr, false); len(vs) != 0 {
			t.Fatalf("%s feature must be exempt, got %v", status, vs)
		}
	}
}

func TestStorageLayout_NonFeatureReadmeIgnored(t *testing.T) {
	sr := gradeSpecRoot(t, slOptIn)
	writeMD(t, sr, "features/README.md", "# Features\n\nindex\n")
	writeMD(t, sr, "features/foo/_tests/README.md", "# Feature: in reserved dir\n")
	if vs := slCheck(t, sr, false); len(vs) != 0 {
		t.Fatalf("non-Feature READMEs must be ignored, got %v", vs)
	}
}

func TestStorageLayout_SectionEndsAtNextHeadingOrRule(t *testing.T) {
	// The Approved by line after the next H2 does not belong to the section.
	sr := gradeSpecRoot(t, slOptIn)
	writeMD(t, sr, "features/foo/README.md",
		"# Feature: X\n\n**Status:** Draft\n\n## Storage layout\n\n- rooms/{id}\n\n---\n\n**Approved by:** pending\n")
	slOnly(t, slCheck(t, sr, false), "feature-storage-layout", "error", "no `**Approved by:**` line")

	sr2 := gradeSpecRoot(t, slOptIn)
	writeMD(t, sr2, "features/foo/README.md",
		"# Feature: X\n\n**Status:** Draft\n\n## Storage layout\n\n- rooms/{id}\n\n# Other\n\n**Approved by:** pending\n")
	slOnly(t, slCheck(t, sr2, false), "feature-storage-layout", "error", "no `**Approved by:**` line")
}

func TestStorageLayout_SortedByFile(t *testing.T) {
	sr := gradeSpecRoot(t, slOptIn)
	writeMD(t, sr, "features/bbb/README.md", slFeature("Draft", ""))
	writeMD(t, sr, "features/aaa/README.md", slFeature("Draft", ""))
	vs := slCheck(t, sr, false)
	if len(vs) != 2 || vs[0].File > vs[1].File {
		t.Fatalf("expected two violations sorted by file, got %+v", vs)
	}
}

// --- --fix ------------------------------------------------------------------

func TestStorageLayout_FixInsertsSkeletonBeforeAcceptanceCriteria(t *testing.T) {
	sr := gradeSpecRoot(t, slOptIn)
	p := writeMD(t, sr, "features/foo/README.md", slFeature("Draft", ""))
	c := newStorageLayoutChecker("", false)
	if err := c.fix(sr); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	got := string(b)
	sl := strings.Index(got, "## Storage layout\n")
	ac := strings.Index(got, "## Acceptance Criteria")
	if sl < 0 || sl > ac {
		t.Fatalf("skeleton must sit right before Acceptance Criteria:\n%s", got)
	}
	for _, want := range []string{"**Approved by:** pending", "Example records", "considered for reuse", "Reads and writes", slGuide, "None — this feature stores nothing."} {
		if !strings.Contains(got, want) {
			t.Fatalf("skeleton missing %q:\n%s", want, got)
		}
	}
	// After the fix the draft carries only the pending warning.
	slOnly(t, slCheck(t, sr, false), "feature-storage-layout-pending", "warning", "storage layout not yet approved")
	// Idempotent.
	if err := c.fix(sr); err != nil {
		t.Fatal(err)
	}
	b2, _ := os.ReadFile(p)
	if string(b2) != got {
		t.Fatal("second --fix run changed the file")
	}
}

func TestStorageLayout_FixPlacementFallbacks(t *testing.T) {
	cases := map[string]struct{ body, before string }{
		"open-questions": {"# Feature: X\n\n**Status:** Draft\n\n## Open Questions\n\nNone.\n", "## Open Questions"},
		"footer":         {"# Feature: X\n\n**Status:** Draft\n\n## Behavior\n\nx\n\n---\n*This document follows the https://specscore.md/feature-specification*\n", "---\n*This document follows"},
		"eof":            {"# Feature: X\n\n**Status:** Draft\n\n## Behavior\n\nx", ""},
		"fenced-ac":      {"# Feature: X\n\n**Status:** Draft\n\n```\n## Acceptance Criteria\n```\n", ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			sr := gradeSpecRoot(t, "project:\n  title: T\nstorage_layout:\n  required: true\n")
			p := writeMD(t, sr, "features/foo/README.md", tc.body)
			if err := newStorageLayoutChecker("", false).fix(sr); err != nil {
				t.Fatal(err)
			}
			b, _ := os.ReadFile(p)
			got := string(b)
			sl := strings.Index(got, "## Storage layout\n")
			if sl < 0 {
				t.Fatalf("skeleton not inserted:\n%s", got)
			}
			if tc.before != "" && sl > strings.Index(got, tc.before) {
				t.Fatalf("skeleton must precede %q:\n%s", tc.before, got)
			}
			if tc.before == "" && !strings.HasSuffix(got, "**Approved by:** pending\n") {
				t.Fatalf("skeleton must be appended at end of file:\n%s", got)
			}
			if strings.Contains(got, "Principles:") {
				t.Fatalf("no guide_url configured; skeleton must not cite one:\n%s", got)
			}
		})
	}
}

func TestStorageLayout_FixLeavesPresentExemptAndOffAlone(t *testing.T) {
	present := slFeature("Draft", "## Storage layout\n\nNone — this feature stores nothing.\n\n")
	rejected := slFeature("Rejected", "")
	sr := gradeSpecRoot(t, slOptIn)
	p1 := writeMD(t, sr, "features/present/README.md", present)
	p2 := writeMD(t, sr, "features/rejected/README.md", rejected)
	p3 := writeMD(t, sr, "features/README.md", "# Features\n")
	if err := newStorageLayoutChecker("", false).fix(sr); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]string{p1: present, p2: rejected, p3: "# Features\n"} {
		if b, _ := os.ReadFile(p); string(b) != want {
			t.Fatalf("%s must be untouched, got:\n%s", p, b)
		}
	}

	off := gradeSpecRoot(t, "project:\n  title: T\n")
	missing := slFeature("Draft", "")
	p4 := writeMD(t, off, "features/foo/README.md", missing)
	if err := newStorageLayoutChecker("", false).fix(off); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p4); string(b) != missing {
		t.Fatal("--fix must not touch a repository that has not opted in")
	}
}

func TestStorageLayout_FixThroughLinterAndTargets(t *testing.T) {
	sr := gradeSpecRoot(t, slOptIn)
	p := writeMD(t, sr, "features/foo/README.md", slFeature("Draft", ""))
	res, err := LintWithResult(Options{SpecRoot: sr, Fix: true, FixTargets: []string{"feature-storage-layout"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Fixed) != 1 || res.Fixed[0] != filepath.Join("features", "foo", "README.md") {
		t.Fatalf("fix report must list the feature README, got %v", res.Fixed)
	}
	if b, _ := os.ReadFile(p); !strings.Contains(string(b), "## Storage layout") {
		t.Fatal("scoped --fix=feature-storage-layout must insert the skeleton")
	}
	found := false
	for _, n := range FixTargetNames() {
		if n == "feature-storage-layout" {
			found = true
		}
		if n == "feature-storage-layout-pending" {
			t.Fatal("the pending rule has no fix of its own")
		}
	}
	if !found {
		t.Fatal("feature-storage-layout must be a valid --fix target")
	}
}

func TestStorageLayout_RulesRegistered(t *testing.T) {
	want := map[string]string{"feature-storage-layout": "error", "feature-storage-layout-pending": "warning"}
	for _, r := range AllRules() {
		if sev, ok := want[r.ID]; ok {
			if r.Severity != sev || r.Family != "feature" {
				t.Fatalf("%s registered as %s/%s, want feature/%s", r.ID, r.Family, r.Severity, sev)
			}
			delete(want, r.ID)
		}
	}
	if len(want) != 0 {
		t.Fatalf("rules not registered: %v", want)
	}
	if err := ValidateRuleNames([]string{"feature-storage-layout", "feature-storage-layout-pending"}); err != nil {
		t.Fatal(err)
	}
}

func TestStorageLayout_CheckerIdentity(t *testing.T) {
	c := newStorageLayoutChecker("", false)
	if c.name() != "feature-storage-layout" || c.severity() != "error" {
		t.Fatalf("checker identity = %s/%s", c.name(), c.severity())
	}
}

func TestStorageLayout_WalkErrorSurfaces(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, chmod tests skipped")
	}
	sr := gradeSpecRoot(t, slOptIn)
	locked := filepath.Join(sr, "features", "locked")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(locked, 0o755) }()
	if _, err := newStorageLayoutChecker("", false).check(sr); err == nil {
		t.Fatal("an unreadable features subtree must fail the check, not pass silently")
	}
}
