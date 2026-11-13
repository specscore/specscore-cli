package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Tests for the opt-in feature-storage-layout rules
// (cli/spec/lint/storage-layout).

const slGuide = "https://example.com/storage-design"

const slOptIn = "project:\n  title: T\nstorage_layout:\n  required: true\n  guide_url: " + slGuide + "\n"

const slOptInExisting = slOptIn + "  existing_until: 2026-01-31\n"

func slFeature(status, section string) string {
	return slFeatureDated(status, "", section)
}

// slFeatureDated is slFeature with a `**Date:**` header line (none when date
// is empty). The header replaces the Source Ideas line so line numbers of the
// body stay the same as slFeature's.
func slFeatureDated(status, date, section string) string {
	meta := "**Source Ideas:** —"
	if date != "" {
		meta = "**Date:** " + date
	}
	return "---\nformat: https://specscore.md/feature-specification\nstatus: " + status + "\n---\n\n" +
		"# Feature: X\n\n**Status:** " + status + "\n" + meta + "\n\n" +
		"## Summary\n\nx\n\n## Behavior\n\nx\n\n" + section +
		"## Acceptance Criteria\n\nNot defined yet.\n\n## Open Questions\n\nNone at this time.\n\n" +
		"---\n*This document follows the https://specscore.md/feature-specification*\n"
}

// slLayout is a section listing one record with the given approval value.
func slLayout(approval string) string {
	return "## Storage layout\n\n- `spaces/{s}/ext/rooms/{id}` one room record\n\n**Approved by:** " + approval + "\n\n"
}

func slCheck(t *testing.T, specRoot string, explicit bool) []Violation {
	t.Helper()
	vs, err := newStorageLayoutChecker("", explicit, false).check(specRoot)
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
	if v.File != "specscore.yaml" && !strings.HasSuffix(v.Message, "See "+slGuide) {
		t.Fatalf("message %q does not end with the configured guide link", v.Message)
	}
	if strings.Contains(v.Message, "\n") {
		t.Fatalf("message must be one line: %q", v.Message)
	}
	return v
}

func slOne(t *testing.T, cfg, status, section string) []Violation {
	t.Helper()
	sr := gradeSpecRoot(t, cfg)
	writeMD(t, sr, "features/foo/README.md", slFeature(status, section))
	return slCheck(t, sr, false)
}

// --- grandfathering: off unless opted in -----------------------------------

func TestStorageLayout_OffWithoutConfig(t *testing.T) {
	if vs := slOne(t, "project:\n  title: T\n", "Stable", ""); len(vs) != 0 {
		t.Fatalf("rule must be off without storage_layout.required, got %v", vs)
	}
}

func TestStorageLayout_OffWithUnreadableConfig(t *testing.T) {
	if vs := slOne(t, "", "Stable", ""); len(vs) != 0 {
		t.Fatalf("rule must be off when specscore.yaml is unreadable, got %v", vs)
	}
}

func TestStorageLayout_OffInDefaultLintRun(t *testing.T) {
	sr := gradeSpecRoot(t, "project:\n  title: T\n")
	p := writeMD(t, sr, "features/foo/README.md", slFeature("Stable", ""))
	res, err := LintWithResult(Options{SpecRoot: sr, Severity: "info", Fix: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range res.Violations {
		if strings.HasPrefix(v.Rule, "feature-storage-layout") {
			t.Fatalf("default run must not report %s without opt-in: %+v", v.Rule, v)
		}
	}
	if b, _ := os.ReadFile(p); strings.Contains(string(b), "Storage layout") {
		t.Fatal("bare --fix must not insert the section")
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
	if strings.Contains(vs[0].Message, "See ") {
		t.Fatalf("no guide_url configured, message must not link one: %q", vs[0].Message)
	}
	if vs[0].FixTarget != "feature-storage-layout" || !strings.Contains(vs[0].Message, "--fix=feature-storage-layout") {
		t.Fatalf("missing-section finding must name its opt-in fix, got %+v", vs[0])
	}
}

func TestStorageLayout_NamedFixTargetIsExplicitSelection(t *testing.T) {
	sr := gradeSpecRoot(t, "project:\n  title: T\n")
	p := writeMD(t, sr, "features/foo/README.md", slFeature("Draft", ""))
	if _, err := LintWithResult(Options{SpecRoot: sr, Fix: true, FixTargets: []string{"feature-storage-layout"}}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); !strings.Contains(string(b), "## Storage layout") {
		t.Fatal("--fix=feature-storage-layout must work without the config opt-in")
	}
}

// --- config shape -----------------------------------------------------------

func TestStorageLayout_MalformedConfigIsVisibleError(t *testing.T) {
	cases := map[string]string{
		"storage_layout:\n  required: \"true\"\n":                           "error",
		"storage_layout: true\n":                                            "error",
		"storage_layout:\n  requird: true\n":                                "error",
		"storage_layout:\n  required: true\n  guide_url: docs/storage.md\n": "error",
		// The repository said it is not opted in: a stray key is a warning,
		// so a default (--severity error) run is unchanged.
		"storage_layout:\n  required: false\n  note: legacy\n": "warning",
	}
	for block, severity := range cases {
		sr := gradeSpecRoot(t, "project:\n  title: T\n"+block)
		p := writeMD(t, sr, "features/foo/README.md", slFeature("Draft", ""))
		v := slOnly(t, slCheck(t, sr, false), "feature-storage-layout", severity, "invalid `storage_layout` block")
		if v.File != "specscore.yaml" {
			t.Fatalf("config error must cite specscore.yaml, got %s", v.File)
		}
		c := newStorageLayoutChecker("", false, true)
		if err := c.fix(sr); err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(p); strings.Contains(string(b), "Storage layout") {
			t.Fatal("--fix must not act on a malformed block")
		}
	}
}

// --- accepted forms ---------------------------------------------------------

func TestStorageLayout_NoneStatementAccepted(t *testing.T) {
	for _, stmt := range []string{
		"None — this feature stores nothing.",
		"None - this feature stores nothing.",
		"none -- this feature stores nothing",
		"NONE – this feature stores nothing.",
	} {
		if vs := slOne(t, slOptIn, "Stable", "## Storage layout\n\n<!-- prompts -->\n"+stmt+"\n\n"); len(vs) != 0 {
			t.Fatalf("%q must be accepted, got %v", stmt, vs)
		}
	}
}

func TestStorageLayout_ApprovedLayoutAccepted(t *testing.T) {
	for _, line := range []string{"**Approved by:** \"yes, build it\" — owner, 2026-11-13", "- **Approved by:** owner, 2026-11-13"} {
		vs := slOne(t, slOptIn, "Stable", "## STORAGE LAYOUT\n\n<!-- prompts -->\n| Record | Path |\n|---|---|\n| room | rooms/{id} |\n\n"+line+"\n\n")
		if len(vs) != 0 {
			t.Fatalf("approved layout %q must be accepted, got %v", line, vs)
		}
	}
}

// --- errors -----------------------------------------------------------------

func TestStorageLayout_MissingSection(t *testing.T) {
	v := slOnly(t, slOne(t, slOptIn, "Draft", ""), "feature-storage-layout", "error", "no `## Storage layout` section")
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

func TestStorageLayout_HiddenSectionsIgnored(t *testing.T) {
	for name, section := range map[string]string{
		"code fence":   "```markdown\n## Storage layout\n\nNone — this feature stores nothing.\n```\n\n",
		"html comment": "<!--\n## Storage layout\n\nNone — this feature stores nothing.\n-->\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			slOnly(t, slOne(t, slOptIn, "Approved", section), "feature-storage-layout", "error", "no `## Storage layout` section")
		})
	}
}

func TestStorageLayout_DuplicateSection(t *testing.T) {
	vs := slOne(t, slOptIn, "Stable", "## Storage layout\n\nNone — this feature stores nothing.\n\n"+slLayout("pending")+"## Storage layout\n\nx\n\n")
	v := slOnly(t, vs, "feature-storage-layout", "error", "more than one `## Storage layout` section")
	if v.Line != 23 {
		t.Fatalf("duplicate finding must cite the second heading, got line %d", v.Line)
	}
}

func TestStorageLayout_EmptySection(t *testing.T) {
	slOnly(t, slOne(t, slOptIn, "Draft", "## Storage layout\n\n<!-- nothing\nyet -->\n\n"), "feature-storage-layout", "error", "`## Storage layout` is empty")
}

func TestStorageLayout_NoneWithContentIsContradiction(t *testing.T) {
	for name, section := range map[string]string{
		"records after none":  "## Storage layout\n\nNone — this feature stores nothing.\n\n- `rooms/{id}` 40 fields\n\n",
		"approval after none": "## Storage layout\n\nNone — this feature stores nothing.\n\n**Approved by:** owner\n\n",
		"none after records":  "## Storage layout\n\n- `rooms/{id}`\n\nNone - this feature stores nothing\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			slOnly(t, slOne(t, slOptIn, "Stable", section), "feature-storage-layout", "error", "says the Feature stores nothing but also lists content")
		})
	}
}

func TestStorageLayout_NoApprovedByLine(t *testing.T) {
	slOnly(t, slOne(t, slOptIn, "Draft", "## Storage layout\n\n- rooms/{id}\n\n```\n**Approved by:** fenced\n```\n\n"), "feature-storage-layout", "error", "no `**Approved by:**` line")
}

func TestStorageLayout_ApprovalWithoutLayout(t *testing.T) {
	slOnly(t, slOne(t, slOptIn, "Draft", "## Storage layout\n\n**Approved by:** owner, 2026-11-13\n\n"), "feature-storage-layout", "error", "describes no layout")
}

// M1/M1': an approval needs a YYYY-MM-DD date and must not start with a
// negative word; placeholders count as not approved.
func TestStorageLayout_PlaceholdersAreNotApproval(t *testing.T) {
	for _, value := range []string{
		"", "—", "–", "-", "N/A", "na", "none", "TBD", "tba", "todo", "self", "?", "unknown", "nobody", "TBD.",
		"pending", "Pending review", "not reviewed",
		"not approved", "Not approved, 2026-11-13", "not yet", "none yet", "no one", "No, 2026-11-13",
		"awaiting review", "waiting for owner 2026-11-13", "TBD (owner to review) 2026-11-13", "todo: 2026-11-13",
		"Ada", "Ada, yesterday", "Ada, 2026-02-30", "Ada, 13/11/2026",
	} {
		v := slOnly(t, slOne(t, slOptIn, "Approved", slLayout(value)), "feature-storage-layout", "error", "is Approved but its storage layout is not approved")
		if v.Line != 23 {
			t.Fatalf("%q: finding must cite the approval line, got %d", value, v.Line)
		}
		if !strings.Contains(v.Message, "who approved it and the date") {
			t.Fatalf("%q: message must say what an approval looks like: %q", value, v.Message)
		}
		slOnly(t, slOne(t, slOptIn, "Draft", slLayout(value)), "feature-storage-layout-pending", "warning", "storage layout not yet approved")
	}
	// Real approvals: a name and a date, optionally quoted words.
	for _, value := range []string{"Tbdavid, 2026-11-13", "Ada, 2026-11-13: \"yes, build it\"", "\"approved\" — owner (2026-11-13)", "Nora 2026-11-13"} {
		if vs := slOne(t, slOptIn, "Approved", slLayout(value)); len(vs) != 0 {
			t.Fatalf("approval %q must pass, got %v", value, vs)
		}
	}
}

func TestStorageLayout_EmptyApprovalMessage(t *testing.T) {
	slOnly(t, slOne(t, slOptIn, "Stable", slLayout("  ")), "feature-storage-layout", "error", "(`**Approved by:**` is empty)")
}

func TestStorageLayout_StatusesFailClosed(t *testing.T) {
	for _, status := range []string{"Approved", "implementing", "Stable"} {
		slOnly(t, slOne(t, slOptIn, status, slLayout("pending")), "feature-storage-layout", "error", "is "+status+" but its storage layout is not approved")
	}
	for _, status := range []string{"Aproved", "Stable (since 2026-01)"} {
		slOnly(t, slOne(t, slOptIn, status, slLayout("pending")), "feature-storage-layout", "error", "`"+status+"` is not a recognized pre-approval status (treated as approved)")
	}
	sr := gradeSpecRoot(t, slOptIn)
	writeMD(t, sr, "features/foo/README.md", "# Feature: X\n\n"+slLayout("pending"))
	slOnly(t, slCheck(t, sr, false), "feature-storage-layout", "error", "status `` is not a recognized pre-approval status")
}

// --- warning ----------------------------------------------------------------

func TestStorageLayout_PendingOnDraftIsWarning(t *testing.T) {
	for _, status := range []string{"Draft", "In Review", "amending", "Under Review", "Planned"} {
		v := slOnly(t, slOne(t, slOptIn, status, "## Storage layout\n\n**Approved by:** Pending (asked 2026-11-13)\n\n"),
			"feature-storage-layout-pending", "warning", "storage layout not yet approved (`**Approved by:** Pending (asked 2026-11-13)`)")
		if v.Line != 21 {
			t.Fatalf("pending finding must cite the Approved by line, got %d", v.Line)
		}
	}
}

func TestStorageLayout_PendingWarningHiddenAtDefaultSeverity(t *testing.T) {
	sr := gradeSpecRoot(t, slOptIn)
	writeMD(t, sr, "features/foo/README.md", slFeature("Draft", "## Storage layout\n\n**Approved by:** pending\n\n"))
	vs, err := Lint(Options{SpecRoot: sr, Severity: "error", Rules: storageLayoutRuleNames})
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 0 {
		t.Fatalf("a pending draft must not fail the default --severity error run, got %v", vs)
	}
}

// --- existing layouts (storage_layout.existing_until) -----------------------

func slOneDated(t *testing.T, cfg, status, date, section string) []Violation {
	t.Helper()
	sr := gradeSpecRoot(t, cfg)
	writeMD(t, sr, "features/foo/README.md", slFeatureDated(status, date, section))
	return slCheck(t, sr, false)
}

func TestStorageLayout_ExistingLayoutAdmittedWithCutoff(t *testing.T) {
	for _, value := range []string{
		"not reviewed — existing layout, recorded 2026-01-31",
		"Not reviewed - existing layout, recorded 2026-01-01.",
		"not reviewed -- existing layout, recorded 2025-06-01",
	} {
		for _, status := range []string{"Stable", "Draft"} {
			v := slOnly(t, slOneDated(t, slOptInExisting, status, "2026-01-31", slLayout(value)), "feature-storage-layout-existing", "info",
				"existing storage layout recorded, not reviewed by a human")
			if !v.Advisory || CountFailing([]Violation{v}) != 0 {
				t.Fatalf("existing-layout finding must be advisory and never fail a run: %+v", v)
			}
		}
	}
}

// M2': the existing form is admitted only for Features that predate the
// cutoff; a new Feature typing a past recorded date must fail, at any status.
func TestStorageLayout_ExistingLayoutRejectedOtherwise(t *testing.T) {
	old := slLayout("not reviewed — existing layout, recorded 2026-01-01")
	cases := map[string]struct{ cfg, status, date, section string }{
		"new feature back-dates the record": {slOptInExisting, "Approved", "2026-12-25", old},
		"new draft back-dates the record":   {slOptInExisting, "Draft", "2026-12-25", old},
		"feature without a Date header":     {slOptInExisting, "Stable", "", old},
		"feature Date not a date":           {slOptInExisting, "Stable", "January", old},
		"recorded after the cutoff":         {slOptInExisting, "Stable", "2026-01-15", slLayout("not reviewed — existing layout, recorded 2026-02-01")},
		"impossible recorded date":          {slOptInExisting, "Stable", "2026-01-15", slLayout("not reviewed — existing layout, recorded 2026-02-30")},
		"no cutoff configured":              {slOptIn, "Stable", "2025-01-01", old},
		"wrong wording":                     {slOptInExisting, "Stable", "2026-01-15", slLayout("not reviewed — existing layouts recorded 2026-01-01")},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			slOnly(t, slOneDated(t, tc.cfg, tc.status, tc.date, tc.section), "feature-storage-layout", "error",
				"only for layouts that predate this rule")
		})
	}
	// The form still needs a described layout.
	slOnly(t, slOneDated(t, slOptInExisting, "Stable", "2026-01-15", "## Storage layout\n\n**Approved by:** not reviewed — existing layout, recorded 2026-01-01\n\n"),
		"feature-storage-layout", "error", "describes no layout")
}

func TestStorageLayout_FutureCutoffIsConfigError(t *testing.T) {
	sr := gradeSpecRoot(t, slOptIn+"  existing_until: 2999-01-01\n")
	writeMD(t, sr, "features/foo/README.md", slFeatureDated("Stable", "2026-01-15", slLayout("not reviewed — existing layout, recorded 2026-01-01")))
	v := slOnly(t, slCheck(t, sr, false), "feature-storage-layout", "error", "`storage_layout.existing_until` (2999-01-01) is in the future")
	if v.File != "specscore.yaml" {
		t.Fatalf("config error must cite specscore.yaml, got %s", v.File)
	}
}

func TestStorageLayout_ExistingListedAtInfoSeverity(t *testing.T) {
	sr := gradeSpecRoot(t, slOptInExisting)
	writeMD(t, sr, "features/foo/README.md", slFeatureDated("Stable", "2026-01-15", slLayout("not reviewed — existing layout, recorded 2026-01-15")))
	vs, err := Lint(Options{SpecRoot: sr, Severity: "info", Rules: storageLayoutRuleNames})
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 1 || vs[0].Rule != "feature-storage-layout-existing" || CountFailing(vs) != 0 {
		t.Fatalf("--severity info must list the existing layout as advisory, got %+v", vs)
	}
	if vs, _ := Lint(Options{SpecRoot: sr, Severity: "error", Rules: storageLayoutRuleNames}); len(vs) != 0 {
		t.Fatalf("default severity must not report it, got %+v", vs)
	}
}

// --- exemptions -------------------------------------------------------------

func TestStorageLayout_RejectedAndDeprecatedExempt(t *testing.T) {
	for _, status := range []string{"Rejected", "deprecated"} {
		if vs := slOne(t, slOptIn, status, ""); len(vs) != 0 {
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

// --- fix (opt-in: --fix=feature-storage-layout only) ------------------------

func TestStorageLayout_FixInsertsSkeletonBeforeAcceptanceCriteria(t *testing.T) {
	sr := gradeSpecRoot(t, slOptIn)
	p := writeMD(t, sr, "features/foo/README.md", slFeature("Draft", ""))
	c := newStorageLayoutChecker("", false, true)
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
	if strings.Contains(got, "not reviewed") {
		t.Fatal("the fix must never insert the existing-layout form")
	}
	slOnly(t, slCheck(t, sr, false), "feature-storage-layout-pending", "warning", "storage layout not yet approved")
	if err := c.fix(sr); err != nil {
		t.Fatal(err)
	}
	if b2, _ := os.ReadFile(p); string(b2) != got {
		t.Fatal("second --fix run changed the file")
	}
}

func TestStorageLayout_BareFixNeverInserts(t *testing.T) {
	sr := gradeSpecRoot(t, slOptIn)
	body := slFeature("Draft", "")
	p := writeMD(t, sr, "features/foo/README.md", body)
	for _, targets := range [][]string{nil, {FixTargetAll}} {
		if _, err := LintWithResult(Options{SpecRoot: sr, Fix: true, FixTargets: targets}); err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(p); string(b) != body {
			t.Fatalf("--fix with targets %v must not insert the section:\n%s", targets, b)
		}
	}
	if err := newStorageLayoutChecker("", true, false).fix(sr); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != body {
		t.Fatal("the fixer must be inert unless its target is named")
	}
}

func TestStorageLayout_FixPlacementFallbacks(t *testing.T) {
	cases := map[string]struct{ body, before string }{
		// CommonMark: a ``` line inside a ~~~ fence does not close it, and a
		// shorter fence does not close a longer one.
		"mixed-fences":   {"# Feature: X\n\n**Status:** Draft\n\n~~~md\n```\n## Acceptance Criteria\n~~~\n\n````\n```\n## Open Questions\n````\n\n## Acceptance Criteria\n\nx\n", "\n## Acceptance Criteria\n\nx"},
		"open-questions": {"# Feature: X\n\n**Status:** Draft\n\n## Open Questions\n\nNone.\n", "## Open Questions"},
		"footer":         {"# Feature: X\n\n**Status:** Draft\n\n## Behavior\n\nx\n\n---\n*This document follows the https://specscore.md/feature-specification*\n", "---\n*This document follows"},
		"eof":            {"# Feature: X\n\n**Status:** Draft\n\n## Behavior\n\nx", ""},
		"fenced-ac":      {"# Feature: X\n\n**Status:** Draft\n\n```\n## Acceptance Criteria\n```\n", ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, changed := AddStorageLayoutSkeleton(tc.body, "")
			if !changed {
				t.Fatal("skeleton not inserted")
			}
			sl := strings.Index(got, "## Storage layout\n")
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

func TestStorageLayout_SkeletonSkipsUnclosedFenceAndKeepsCRLF(t *testing.T) {
	unclosed := "# Feature: X\n\n**Status:** Draft\n\n```go\nfunc x() {}\n\n## Acceptance Criteria\n\nx\n"
	if got, changed := AddStorageLayoutSkeleton(unclosed, ""); changed || got != unclosed {
		t.Fatal("an unclosed fence must leave the file unchanged (an insert inside it would repeat on every run)")
	}
	sr := gradeSpecRoot(t, slOptIn)
	writeMD(t, sr, "features/foo/README.md", unclosed)
	v := slOnly(t, slCheck(t, sr, false), "feature-storage-layout", "error", "a fenced code block is never closed")
	if v.FixTarget != "" {
		t.Fatal("a finding the fix cannot repair must not advertise the fix target")
	}
	crlf := "# Feature: X\r\n\r\n**Status:** Draft\r\n\r\n## Acceptance Criteria\r\n\r\nx\r\n"
	got, changed := AddStorageLayoutSkeleton(crlf, "")
	if !changed || strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n") {
		t.Fatalf("a CRLF file must keep CRLF line endings only:\n%q", got)
	}
	if _, again := AddStorageLayoutSkeleton(got, ""); again {
		t.Fatal("CRLF insert must be idempotent")
	}
}

func TestStorageLayout_SkeletonLeavesPresentExemptAndNonFeatureAlone(t *testing.T) {
	for _, body := range []string{
		slFeature("Draft", "## Storage layout\n\nNone — this feature stores nothing.\n\n"),
		slFeature("Rejected", ""),
		"# Features\n",
	} {
		if got, changed := AddStorageLayoutSkeleton(body, slGuide); changed || got != body {
			t.Fatalf("must be untouched:\n%s", body)
		}
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
		if n == "feature-storage-layout-pending" || n == "feature-storage-layout-existing" {
			t.Fatalf("%s has no fix of its own", n)
		}
	}
	if !found {
		t.Fatal("feature-storage-layout must be a valid --fix target")
	}
}

func TestStorageLayout_RulesRegistered(t *testing.T) {
	want := map[string]string{"feature-storage-layout": "error", "feature-storage-layout-pending": "warning", "feature-storage-layout-existing": "info"}
	for _, r := range AllRules() {
		if sev, ok := want[r.ID]; ok {
			if r.Severity != sev || r.Family != "feature" {
				t.Fatalf("%s registered as %s/%s, want feature/%s", r.ID, r.Family, r.Severity, sev)
			}
			if r.Advisory != (r.ID == "feature-storage-layout-existing") {
				t.Fatalf("%s advisory = %v", r.ID, r.Advisory)
			}
			delete(want, r.ID)
		}
	}
	if len(want) != 0 {
		t.Fatalf("rules not registered: %v", want)
	}
	if err := ValidateRuleNames(storageLayoutRuleNames); err != nil {
		t.Fatal(err)
	}
}

func TestStorageLayout_CheckerIdentity(t *testing.T) {
	c := newStorageLayoutChecker("", false, false)
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
	if _, err := newStorageLayoutChecker("", false, false).check(sr); err == nil {
		t.Fatal("an unreadable features subtree must fail the check, not pass silently")
	}
}

// Minor 3: the skeleton lands after both mixed fences, before the real
// Acceptance Criteria heading.
func TestStorageLayout_SkeletonRespectsMixedFences(t *testing.T) {
	body := "# Feature: X\n\n**Status:** Draft\n\n``not a fence``\n\n~~~md\n```\n## Acceptance Criteria\n~~~\n\n````\n```\n## Open Questions\n````\n\n## Acceptance Criteria\n\nx\n"
	got, changed := AddStorageLayoutSkeleton(body, "")
	if !changed {
		t.Fatal("balanced mixed fences must not block the insert")
	}
	sl := strings.Index(got, "## Storage layout")
	if sl < strings.LastIndex(got, "````") || sl > strings.LastIndex(got, "## Acceptance Criteria") {
		t.Fatalf("skeleton must sit after the fences and before the real heading:\n%s", got)
	}
}
