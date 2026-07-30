package lifecycle

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Transition table: legal arcs per the Meta contract.
// Keep this in sync with transitionMatrix in lifecycle.go; the tests below
// derive their expected behavior from THIS table, not from the production
// table, so a drift between the two will be caught explicitly.
// ---------------------------------------------------------------------------

var expectedLegal = map[Kind][]transitionRow{
	KindIdea: {
		{From: IdeaDraft, To: IdeaInReview},
		{From: IdeaDraft, To: IdeaApproved},
		{From: IdeaDraft, To: IdeaStale},
		{From: IdeaInReview, To: IdeaApproved},
		{From: IdeaInReview, To: IdeaRejected},
		{From: IdeaInReview, To: IdeaStale},
		{From: IdeaApproved, To: IdeaSpecifying},
		{From: IdeaApproved, To: IdeaStale},
		{From: IdeaSpecifying, To: IdeaSpecified},
		{From: IdeaSpecifying, To: IdeaStale},
		{From: IdeaSpecified, To: IdeaImplementing},
		{From: IdeaSpecified, To: IdeaStale},
		{From: IdeaImplementing, To: IdeaImplemented},
	},
	KindFeature: {
		{From: FeatureDraft, To: FeatureInReview},
		{From: FeatureDraft, To: FeatureApproved},
		{From: FeatureDraft, To: FeatureDeprecated},
		{From: FeatureInReview, To: FeatureApproved},
		{From: FeatureInReview, To: FeatureRejected},
		{From: FeatureApproved, To: FeatureImplementing},
		{From: FeatureImplementing, To: FeatureStable},
		{From: FeatureImplementing, To: FeatureDeprecated},
		{From: FeatureStable, To: FeatureAmending},
		{From: FeatureAmending, To: FeatureStable},
		{From: FeatureStable, To: FeatureDeprecated},
	},
	KindPlan: {
		{From: PlanDraft, To: PlanInReview},
		{From: PlanDraft, To: PlanApproved},
		{From: PlanInReview, To: PlanDraft},
		{From: PlanInReview, To: PlanApproved},
		{From: PlanInReview, To: PlanRejected},

		{From: PlanApproved, To: PlanWithdrawn},
		{From: PlanExecuting, To: PlanWithdrawn},
		{From: PlanBlocked, To: PlanWithdrawn},
		{From: PlanImplemented, To: PlanWithdrawn},
		{From: PlanFailed, To: PlanWithdrawn},

		{From: PlanApproved, To: PlanSuperseded},
		{From: PlanExecuting, To: PlanSuperseded},
		{From: PlanBlocked, To: PlanSuperseded},
		{From: PlanImplemented, To: PlanSuperseded},
		{From: PlanFailed, To: PlanSuperseded},

		{From: PlanApproved, To: PlanDeprecated},
		{From: PlanExecuting, To: PlanDeprecated},
		{From: PlanBlocked, To: PlanDeprecated},
		{From: PlanImplemented, To: PlanDeprecated},
		{From: PlanFailed, To: PlanDeprecated},
	},
}

// allKindStatuses lists every status that should appear as either From or To
// in the kind's matrix. Used to exhaustively enumerate ILLEGAL transitions
// (every (from, to) where (from, to) is NOT in expectedLegal[kind]).
var allKindStatuses = map[Kind][]Status{
	KindIdea: {
		IdeaDraft,
		IdeaInReview,
		IdeaApproved,
		IdeaSpecifying,
		IdeaSpecified,
		IdeaImplementing,
		IdeaImplemented,
		IdeaRejected,
		IdeaStale,
	},
	KindFeature: {
		FeatureDraft,
		FeatureInReview,
		FeatureApproved,
		FeatureImplementing,
		FeatureStable,
		FeatureAmending,
		FeatureRejected,
		FeatureDeprecated,
	},
	KindPlan: {
		PlanDraft,
		PlanInReview,
		PlanApproved,
		PlanExecuting,
		PlanBlocked,
		PlanImplemented,
		PlanFailed,
		PlanRejected,
		PlanWithdrawn,
		PlanSuperseded,
		PlanDeprecated,
	},
}

// ---------------------------------------------------------------------------
// Tests for Transition: legal AND illegal triples.
// ---------------------------------------------------------------------------

func TestTransition_LegalArcs(t *testing.T) {
	t.Parallel()
	count := 0
	for kind, rows := range expectedLegal {
		for _, r := range rows {
			count++
			if err := Transition(kind, r.From, r.To); err != nil {
				t.Errorf("Transition(%q, %q → %q) returned error, want nil: %v",
					kind, r.From, r.To, err)
			}
		}
	}
	if count == 0 {
		t.Fatal("expectedLegal is empty; no legal arcs to test")
	}
	t.Logf("verified %d legal arcs across all kinds", count)
}

func TestTransition_IllegalArcs(t *testing.T) {
	t.Parallel()
	count := 0
	for kind, statuses := range allKindStatuses {
		legalSet := make(map[transitionRow]bool, len(expectedLegal[kind]))
		for _, r := range expectedLegal[kind] {
			legalSet[r] = true
		}
		for _, from := range statuses {
			for _, to := range statuses {
				if legalSet[transitionRow{From: from, To: to}] {
					continue
				}
				count++
				err := Transition(kind, from, to)
				if err == nil {
					t.Errorf("Transition(%q, %q → %q) returned nil, want ErrInvalidTransition",
						kind, from, to)
					continue
				}
				if !errors.Is(err, ErrInvalidTransition) {
					t.Errorf("Transition(%q, %q → %q): err does not wrap ErrInvalidTransition: %v",
						kind, from, to, err)
				}
				var ite *InvalidTransitionError
				if !errors.As(err, &ite) {
					t.Errorf("Transition(%q, %q → %q): err is not *InvalidTransitionError: %T",
						kind, from, to, err)
					continue
				}
				if ite.Kind != kind || ite.From != from || ite.To != to {
					t.Errorf("InvalidTransitionError context wrong: got (%q, %q, %q), want (%q, %q, %q)",
						ite.Kind, ite.From, ite.To, kind, from, to)
				}
			}
		}
	}
	if count == 0 {
		t.Fatal("no illegal arcs enumerated; test logic broken")
	}
	t.Logf("verified %d illegal arcs across all kinds", count)
}

// TestTransition_UnknownKind covers the defensive branch for a kind that has
// no entry in transitionMatrix.
func TestTransition_UnknownKind(t *testing.T) {
	t.Parallel()
	err := Transition(Kind("totally-bogus"), Status("X"), Status("Y"))
	if err == nil {
		t.Fatal("Transition with unknown kind returned nil error")
	}
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("err does not wrap ErrInvalidTransition: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Tests for LegalTargets / LegalSources / LegalStatuses.
// ---------------------------------------------------------------------------

func TestLegalTargets_AllSources(t *testing.T) {
	t.Parallel()
	for kind, rows := range expectedLegal {
		want := map[Status][]Status{}
		for _, r := range rows {
			want[r.From] = append(want[r.From], r.To)
		}
		for from, ts := range want {
			sort.Slice(ts, func(i, j int) bool { return string(ts[i]) < string(ts[j]) })
			got := LegalTargets(kind, from)
			if !reflect.DeepEqual(got, ts) {
				t.Errorf("LegalTargets(%q, %q) = %v, want %v", kind, from, got, ts)
			}
		}
	}
}

// Stale for Idea / Deprecated for Feature are terminal: no legal targets
// exist FROM them. Verify the function returns an empty (non-nil) slice.
func TestLegalTargets_Terminal(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind Kind
		from Status
	}{
		{KindIdea, IdeaStale},
		{KindFeature, FeatureDeprecated},
	}
	for _, c := range cases {
		got := LegalTargets(c.kind, c.from)
		if got == nil {
			t.Errorf("LegalTargets(%q, %q) returned nil, want empty []Status", c.kind, c.from)
		}
		if len(got) != 0 {
			t.Errorf("LegalTargets(%q, %q) = %v, want empty", c.kind, c.from, got)
		}
	}
}

func TestLegalSources(t *testing.T) {
	t.Parallel()
	for kind, rows := range expectedLegal {
		want := map[Status][]Status{}
		for _, r := range rows {
			want[r.To] = append(want[r.To], r.From)
		}
		for to, ss := range want {
			sort.Slice(ss, func(i, j int) bool { return string(ss[i]) < string(ss[j]) })
			got := LegalSources(kind, to)
			if !reflect.DeepEqual(got, ss) {
				t.Errorf("LegalSources(%q, %q) = %v, want %v", kind, to, got, ss)
			}
		}
	}
}

func TestLegalStatuses(t *testing.T) {
	t.Parallel()
	for kind, want := range allKindStatuses {
		sorted := append([]Status(nil), want...)
		sort.Slice(sorted, func(i, j int) bool { return string(sorted[i]) < string(sorted[j]) })
		got := LegalStatuses(kind)
		if !reflect.DeepEqual(got, sorted) {
			t.Errorf("LegalStatuses(%q) = %v, want %v", kind, got, sorted)
		}
	}
}

func TestLegalStatuses_UnknownKind(t *testing.T) {
	t.Parallel()
	got := LegalStatuses(Kind("totally-bogus"))
	if got == nil {
		t.Error("LegalStatuses(unknown) returned nil, want empty []Status")
	}
	if len(got) != 0 {
		t.Errorf("LegalStatuses(unknown) = %v, want empty", got)
	}
}

// TestLegalStatuses_ReturnsCopy guards against callers mutating the
// package-level slice via the return value.
func TestLegalStatuses_ReturnsCopy(t *testing.T) {
	t.Parallel()
	got := LegalStatuses(KindIdea)
	if len(got) == 0 {
		t.Fatal("LegalStatuses(KindIdea) returned empty slice")
	}
	got[0] = Status("MUTATED")
	got2 := LegalStatuses(KindIdea)
	if got2[0] == Status("MUTATED") {
		t.Error("LegalStatuses mutation leaked to package-level state")
	}
}

// ---------------------------------------------------------------------------
// ParseStatus: case-insensitive flag parsing.
// ---------------------------------------------------------------------------

func TestParseStatus_CaseInsensitive(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind  Kind
		raw   string
		want  Status
		ok    bool
		label string
	}{
		// Exact match.
		{KindIdea, "Draft", IdeaDraft, true, "exact"},
		{KindIdea, "Approved", IdeaApproved, true, "exact"},
		// Lower-case.
		{KindIdea, "draft", IdeaDraft, true, "lower"},
		{KindFeature, "stable", FeatureStable, true, "lower"},
		// Upper-case.
		{KindFeature, "DEPRECATED", FeatureDeprecated, true, "upper"},
		// Mixed.
		{KindIdea, "ApPrOvEd", IdeaApproved, true, "mixed"},
		// Multi-word.
		{KindIdea, "in review", IdeaInReview, true, "lower multi-word"},
		{KindIdea, "IN REVIEW", IdeaInReview, true, "upper multi-word"},
		{KindIdea, "In Review", IdeaInReview, true, "exact multi-word"},
		{KindFeature, "In Review", FeatureInReview, true, "feature multi-word"},
		// Whitespace tolerance.
		{KindIdea, "  Draft  ", IdeaDraft, true, "padded"},
		{KindIdea, "\tApproved\t", IdeaApproved, true, "tab-padded"},
		// Negative cases.
		{KindIdea, "", "", false, "empty"},
		{KindIdea, "Bogus", "", false, "unknown name"},
		{KindIdea, "Stable", "", false, "wrong kind: Stable is Feature-only"},
		{KindFeature, "Specified", "", false, "wrong kind: Specified is Idea-only"},
		{KindIdea, "Underreview", "", false, "missing space in multi-word"},
		{Kind("bogus-kind"), "Draft", "", false, "unknown kind"},
	}
	for _, c := range cases {
		got, ok := ParseStatus(c.kind, c.raw)
		if ok != c.ok || got != c.want {
			t.Errorf("ParseStatus(%q, %q) [%s] = (%q, %v), want (%q, %v)",
				c.kind, c.raw, c.label, got, ok, c.want, c.ok)
		}
	}
}

// ---------------------------------------------------------------------------
// validateMatrix: the not-idempotent init-time invariant.
//
// Per the task spec: "test asserts the panic by constructing a deliberately
// bad table in a test-internal helper — e.g. a helper that calls a private
// validateMatrix(matrix) function with a bad row, NOT by mutating the
// production matrix."
// ---------------------------------------------------------------------------

func TestValidateMatrix_RejectsSelfLoop(t *testing.T) {
	t.Parallel()
	bad := []transitionRow{
		{From: Status("Draft"), To: Status("Approved")},
		{From: Status("Approved"), To: Status("Approved")}, // self-loop!
	}
	err := validateMatrix(bad)
	if err == nil {
		t.Fatal("validateMatrix accepted a self-loop row")
	}
	if !strings.Contains(err.Error(), "self-loop") {
		t.Errorf("validateMatrix error does not mention self-loop: %v", err)
	}
}

func TestValidateMatrix_AcceptsLegalTable(t *testing.T) {
	t.Parallel()
	for kind, rows := range expectedLegal {
		if err := validateMatrix(rows); err != nil {
			t.Errorf("validateMatrix(%q rows) returned error on production table: %v", kind, err)
		}
	}
}

// TestValidateMatrix_PanicSimulation demonstrates that init() would panic if a
// bad table were registered. We simulate the exact recover() + panic()
// signature init uses, without touching the production map.
func TestValidateMatrix_PanicSimulation(t *testing.T) {
	t.Parallel()
	bad := []transitionRow{
		{From: Status("Draft"), To: Status("Draft")},
	}
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("simulated init did not panic on self-loop")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic value is not string: %T = %v", r, r)
		}
		if !strings.Contains(msg, "self-loop") {
			t.Errorf("panic message does not mention self-loop: %s", msg)
		}
	}()

	// Reproduce the init-time check exactly.
	if err := validateMatrix(bad); err != nil {
		panic("lifecycle: transition matrix for kind \"simulated\" is invalid: " + err.Error())
	}
	t.Fatal("validateMatrix did not return an error for self-loop row")
}

// TestProductionMatrix_NoSelfLoops is the belt-and-suspenders direct
// verification of REQ: not-idempotent against the actual production table.
func TestProductionMatrix_NoSelfLoops(t *testing.T) {
	t.Parallel()
	for kind, rows := range transitionMatrix {
		for _, r := range rows {
			if r.From == r.To {
				t.Errorf("production matrix for kind %q contains self-loop %q → %q",
					kind, r.From, r.To)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Validate: combines readStatus + Transition. Uses a synthetic Idea file.
// ---------------------------------------------------------------------------

const ideaFixture = "# Idea: Sample\n\n**Status:** Draft\n**Date:** 2026-01-01\n**Owner:** alice\n\n## Problem Statement\n\nSomething to fix.\n"

func writeFixture(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	return path
}

func TestValidate_LegalTransition(t *testing.T) {
	t.Parallel()
	path := writeFixture(t, ideaFixture)
	from, err := Validate(KindIdea, path, IdeaApproved)
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if from != IdeaDraft {
		t.Errorf("Validate returned from=%q, want %q", from, IdeaDraft)
	}
}

func TestValidate_IllegalTransition(t *testing.T) {
	t.Parallel()
	path := writeFixture(t, ideaFixture)
	// Idea: Draft → Stable is bogus (Stable doesn't exist for Idea).
	from, err := Validate(KindIdea, path, Status("Stable"))
	if err == nil {
		t.Fatal("Validate accepted illegal transition Draft → Stable for Idea")
	}
	if from != IdeaDraft {
		t.Errorf("Validate returned from=%q, want %q (even on failure)", from, IdeaDraft)
	}
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("err does not wrap ErrInvalidTransition: %v", err)
	}
}

func TestValidate_MissingStatusLine(t *testing.T) {
	t.Parallel()
	path := writeFixture(t, "# Idea: Bogus\n\nNo status here.\n")
	_, err := Validate(KindIdea, path, IdeaApproved)
	if !errors.Is(err, ErrStatusLineNotFound) {
		t.Errorf("Validate err = %v, want ErrStatusLineNotFound", err)
	}
}

func TestValidate_MissingFile(t *testing.T) {
	t.Parallel()
	_, err := Validate(KindIdea, filepath.Join(t.TempDir(), "does-not-exist.md"), IdeaApproved)
	if err == nil {
		t.Fatal("Validate accepted missing file")
	}
	if !os.IsNotExist(err) {
		t.Errorf("Validate err = %v, want os.IsNotExist", err)
	}
}

// ---------------------------------------------------------------------------
// Rewrite + Rollback round-trip.
// ---------------------------------------------------------------------------

func TestRewrite_DraftToApproved(t *testing.T) {
	t.Parallel()
	path := writeFixture(t, ideaFixture)
	origLine, err := Rewrite(path, IdeaApproved)
	if err != nil {
		t.Fatalf("Rewrite: %v", err)
	}
	if !strings.Contains(origLine, "Draft") {
		t.Errorf("returned originalStatusLine missing Draft: %q", origLine)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after Rewrite: %v", err)
	}
	want := strings.Replace(ideaFixture, "**Status:** Draft", "**Status:** Approved", 1)
	if string(got) != want {
		t.Errorf("file after Rewrite is not byte-identical except for status line.\nGot:\n%q\nWant:\n%q", got, want)
	}
}

func TestRewriteRollback_RoundTrip(t *testing.T) {
	t.Parallel()
	path := writeFixture(t, ideaFixture)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	origLine, err := Rewrite(path, IdeaApproved)
	if err != nil {
		t.Fatalf("Rewrite: %v", err)
	}

	// Sanity-check the file actually changed.
	mutated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(before, mutated) {
		t.Fatal("Rewrite did not change the file")
	}

	if err := Rollback(path, origLine); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("Rollback did not restore byte-identical content.\nBefore:\n%q\nAfter:\n%q", before, after)
	}
}

// TestRewrite_PreservesCRLF guarantees REQ: status-line-rewrite holds for
// files using Windows-style line endings.
func TestRewrite_PreservesCRLF(t *testing.T) {
	t.Parallel()
	content := "# Idea: Sample\r\n\r\n**Status:** Draft\r\n**Owner:** alice\r\n"
	path := writeFixture(t, content)
	origLine, err := Rewrite(path, IdeaApproved)
	if err != nil {
		t.Fatalf("Rewrite: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(content, "**Status:** Draft", "**Status:** Approved", 1)
	if string(got) != want {
		t.Errorf("CRLF file not preserved byte-for-byte except for status value.\nGot:\n%q\nWant:\n%q", got, want)
	}
	if !strings.HasSuffix(origLine, "\r\n") {
		t.Errorf("originalStatusLine lost CRLF terminator: %q", origLine)
	}
}

// TestRewrite_PreservesTrailingWhitespace ensures the rewrite preserves
// trailing whitespace on the status line (a subtle byte-identity case).
func TestRewrite_PreservesTrailingWhitespace(t *testing.T) {
	t.Parallel()
	// Use spaces after the status value AND a leading indent (atypical but
	// the contract says we preserve it).
	content := "# Idea: Sample\n\n  **Status:** Draft   \n**Owner:** alice\n"
	path := writeFixture(t, content)
	if _, err := Rewrite(path, IdeaApproved); err != nil {
		t.Fatalf("Rewrite: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "# Idea: Sample\n\n  **Status:** Approved   \n**Owner:** alice\n"
	if string(got) != want {
		t.Errorf("Rewrite did not preserve indent/trailing-space.\nGot:\n%q\nWant:\n%q", got, want)
	}
}

func TestRewrite_NoStatusLine(t *testing.T) {
	t.Parallel()
	content := "# Idea: Sample\n\nNo header.\n"
	path := writeFixture(t, content)
	_, err := Rewrite(path, IdeaApproved)
	if !errors.Is(err, ErrStatusLineNotFound) {
		t.Errorf("Rewrite err = %v, want ErrStatusLineNotFound", err)
	}
	// File must be untouched.
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Errorf("Rewrite mutated file despite missing status line.\nGot:\n%q\nWant:\n%q", got, content)
	}
}

func TestRollback_NoStatusLine(t *testing.T) {
	t.Parallel()
	content := "# Idea: Sample\n\nNo header.\n"
	path := writeFixture(t, content)
	err := Rollback(path, "**Status:** Draft\n")
	if !errors.Is(err, ErrStatusLineNotFound) {
		t.Errorf("Rollback err = %v, want ErrStatusLineNotFound", err)
	}
}

// TestRewrite_FeatureFixture covers a Feature README (no kind dependency in
// Rewrite, but we want to be sure the regex matches the canonical Feature
// README shape).
func TestRewrite_FeatureFixture(t *testing.T) {
	t.Parallel()
	content := "# Feature: Sample\n\n**Status:** Draft\n**Deps:** —\n\n## Summary\n"
	path := writeFixture(t, content)
	if _, err := Rewrite(path, FeatureInReview); err != nil {
		t.Fatalf("Rewrite: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(content, "**Status:** Draft", "**Status:** In Review", 1)
	if string(got) != want {
		t.Errorf("Feature Rewrite not byte-clean.\nGot:\n%q\nWant:\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// InvalidTransitionError.Error(): both message paths.
// ---------------------------------------------------------------------------

func TestInvalidTransitionError_ErrorWithTargets(t *testing.T) {
	t.Parallel()
	e := &InvalidTransitionError{
		Kind:         KindIdea,
		From:         IdeaDraft,
		To:           Status("Bogus"),
		LegalTargets: []Status{IdeaApproved, IdeaStale},
	}
	msg := e.Error()
	if !strings.Contains(msg, "idea") {
		t.Errorf("error message should contain kind: %s", msg)
	}
	if !strings.Contains(msg, "Draft") {
		t.Errorf("error message should contain from status: %s", msg)
	}
	if !strings.Contains(msg, "Bogus") {
		t.Errorf("error message should contain to status: %s", msg)
	}
	if !strings.Contains(msg, "Approved") {
		t.Errorf("error message should contain legal targets: %s", msg)
	}
}

func TestInvalidTransitionError_ErrorNoTargets(t *testing.T) {
	t.Parallel()
	e := &InvalidTransitionError{
		Kind:         KindIdea,
		From:         IdeaStale,
		To:           Status("Bogus"),
		LegalTargets: []Status{},
	}
	msg := e.Error()
	if !strings.Contains(msg, "no legal targets") {
		t.Errorf("error message should mention 'no legal targets': %s", msg)
	}
	if !strings.Contains(msg, "Stale") {
		t.Errorf("error message should contain from status: %s", msg)
	}
}

func TestInvalidTransitionError_Unwrap(t *testing.T) {
	t.Parallel()
	e := &InvalidTransitionError{
		Kind: KindIdea,
		From: IdeaDraft,
		To:   Status("Bogus"),
	}
	if !errors.Is(e, ErrInvalidTransition) {
		t.Error("Unwrap should expose ErrInvalidTransition")
	}
}

// ---------------------------------------------------------------------------
// LegalTargets / LegalSources: unknown kind branches.
// ---------------------------------------------------------------------------

func TestLegalTargets_UnknownKind(t *testing.T) {
	t.Parallel()
	got := LegalTargets(Kind("unknown"), Status("X"))
	if got == nil {
		t.Error("LegalTargets(unknown) returned nil, want empty []Status")
	}
	if len(got) != 0 {
		t.Errorf("LegalTargets(unknown) = %v, want empty", got)
	}
}

func TestLegalSources_UnknownKind(t *testing.T) {
	t.Parallel()
	got := LegalSources(Kind("unknown"), Status("X"))
	if got == nil {
		t.Error("LegalSources(unknown) returned nil, want empty []Status")
	}
	if len(got) != 0 {
		t.Errorf("LegalSources(unknown) = %v, want empty", got)
	}
}

func TestLegalSources_NoSources(t *testing.T) {
	t.Parallel()
	// IdeaDraft has no sources (nothing transitions INTO Draft).
	got := LegalSources(KindIdea, IdeaDraft)
	if got == nil {
		t.Error("LegalSources returned nil, want empty []Status")
	}
	if len(got) != 0 {
		t.Errorf("LegalSources(KindIdea, IdeaDraft) = %v, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// Rewrite / Rollback: file-not-found error paths.
// ---------------------------------------------------------------------------

func TestRewrite_MissingFile(t *testing.T) {
	t.Parallel()
	_, err := Rewrite(filepath.Join(t.TempDir(), "nope.md"), IdeaApproved)
	if err == nil {
		t.Fatal("Rewrite should error on missing file")
	}
}

func TestRollback_MissingFile(t *testing.T) {
	t.Parallel()
	err := Rollback(filepath.Join(t.TempDir(), "nope.md"), "**Status:** Draft\n")
	if err == nil {
		t.Fatal("Rollback should error on missing file")
	}
}

// ---------------------------------------------------------------------------
// splitKeepTerminators: edge cases.
// ---------------------------------------------------------------------------

func TestSplitKeepTerminators_Empty(t *testing.T) {
	t.Parallel()
	got := splitKeepTerminators(nil)
	if got != nil {
		t.Errorf("splitKeepTerminators(nil) = %v, want nil", got)
	}
	got = splitKeepTerminators([]byte{})
	if got != nil {
		t.Errorf("splitKeepTerminators(empty) = %v, want nil", got)
	}
}

func TestSplitKeepTerminators_NoTrailingNewline(t *testing.T) {
	t.Parallel()
	got := splitKeepTerminators([]byte("line1\nline2"))
	if len(got) != 2 {
		t.Fatalf("len = %d; want 2; got %q", len(got), got)
	}
	if got[0] != "line1\n" {
		t.Errorf("got[0] = %q; want %q", got[0], "line1\n")
	}
	if got[1] != "line2" {
		t.Errorf("got[1] = %q; want %q", got[1], "line2")
	}
}

func TestSplitKeepTerminators_SingleLineNoNewline(t *testing.T) {
	t.Parallel()
	got := splitKeepTerminators([]byte("hello"))
	if len(got) != 1 {
		t.Fatalf("len = %d; want 1; got %q", len(got), got)
	}
	if got[0] != "hello" {
		t.Errorf("got[0] = %q; want %q", got[0], "hello")
	}
}

func TestSplitKeepTerminators_RoundTrip(t *testing.T) {
	t.Parallel()
	original := []byte("line1\r\nline2\nline3")
	lines := splitKeepTerminators(original)
	reassembled := joinLines(lines)
	if !bytes.Equal(original, reassembled) {
		t.Errorf("round-trip failed.\nOriginal: %q\nReassembled: %q", original, reassembled)
	}
}

// ---------------------------------------------------------------------------
// splitTerminator: edge cases.
// ---------------------------------------------------------------------------

func TestSplitTerminator_LF(t *testing.T) {
	t.Parallel()
	body, term := splitTerminator("hello\n")
	if body != "hello" || term != "\n" {
		t.Errorf("splitTerminator(LF) = (%q, %q), want (%q, %q)", body, term, "hello", "\n")
	}
}

func TestSplitTerminator_CRLF(t *testing.T) {
	t.Parallel()
	body, term := splitTerminator("hello\r\n")
	if body != "hello" || term != "\r\n" {
		t.Errorf("splitTerminator(CRLF) = (%q, %q), want (%q, %q)", body, term, "hello", "\r\n")
	}
}

func TestSplitTerminator_NoTerminator(t *testing.T) {
	t.Parallel()
	body, term := splitTerminator("hello")
	if body != "hello" || term != "" {
		t.Errorf("splitTerminator(bare) = (%q, %q), want (%q, %q)", body, term, "hello", "")
	}
}

// ---------------------------------------------------------------------------
// dirOf: edge cases.
// ---------------------------------------------------------------------------

func TestDirOf_SimpleDir(t *testing.T) {
	t.Parallel()
	if got := dirOf("/foo/bar/baz.txt"); got != "/foo/bar" {
		t.Errorf("dirOf(/foo/bar/baz.txt) = %q; want %q", got, "/foo/bar")
	}
}

func TestDirOf_RootPath(t *testing.T) {
	t.Parallel()
	if got := dirOf("/baz.txt"); got != "/" {
		t.Errorf("dirOf(/baz.txt) = %q; want %q", got, "/")
	}
}

func TestDirOf_NoSlash(t *testing.T) {
	t.Parallel()
	if got := dirOf("baz.txt"); got != "." {
		t.Errorf("dirOf(baz.txt) = %q; want %q", got, ".")
	}
}

func TestDirOf_Backslash(t *testing.T) {
	t.Parallel()
	if got := dirOf(`foo\bar\baz.txt`); got != `foo\bar` {
		t.Errorf(`dirOf(foo\bar\baz.txt) = %q; want %q`, got, `foo\bar`)
	}
}

// ---------------------------------------------------------------------------
// joinLines: edge case.
// ---------------------------------------------------------------------------

func TestJoinLines_Empty(t *testing.T) {
	t.Parallel()
	got := joinLines(nil)
	if len(got) != 0 {
		t.Errorf("joinLines(nil) = %q; want empty", got)
	}
}

// ---------------------------------------------------------------------------
// computeStatusUnion: directly tested for correctness.
// ---------------------------------------------------------------------------

func TestComputeStatusUnion_Deduplication(t *testing.T) {
	t.Parallel()
	rows := []transitionRow{
		{From: Status("A"), To: Status("B")},
		{From: Status("A"), To: Status("C")},
		{From: Status("B"), To: Status("C")},
	}
	got := computeStatusUnion(rows)
	want := []Status{"A", "B", "C"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("computeStatusUnion = %v, want %v", got, want)
	}
}

// ---------------------------------------------------------------------------
// Rewrite: file ending without trailing newline.
// ---------------------------------------------------------------------------

func TestRewrite_NoTrailingNewline(t *testing.T) {
	t.Parallel()
	content := "# Idea\n\n**Status:** Draft"
	path := writeFixture(t, content)
	origLine, err := Rewrite(path, IdeaApproved)
	if err != nil {
		t.Fatalf("Rewrite: %v", err)
	}
	if !strings.Contains(origLine, "Draft") {
		t.Errorf("origLine missing Draft: %q", origLine)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(content, "**Status:** Draft", "**Status:** Approved", 1)
	if string(got) != want {
		t.Errorf("file after Rewrite:\nGot:  %q\nWant: %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// Rollback after Rewrite with CRLF: round-trip byte identity.
// ---------------------------------------------------------------------------

func TestRollback_CRLFRoundTrip(t *testing.T) {
	t.Parallel()
	content := "# Idea\r\n\r\n**Status:** Draft\r\n**Owner:** alice\r\n"
	path := writeFixture(t, content)
	before, _ := os.ReadFile(path)

	origLine, err := Rewrite(path, IdeaApproved)
	if err != nil {
		t.Fatalf("Rewrite: %v", err)
	}
	if err := Rollback(path, origLine); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Errorf("CRLF round-trip failed.\nBefore: %q\nAfter:  %q", before, after)
	}
}

// ---------------------------------------------------------------------------
// writeFileAtomic: error paths.
// ---------------------------------------------------------------------------

func TestWriteFileAtomic_StatFails(t *testing.T) {
	t.Parallel()
	// Write to a path that doesn't exist - os.Stat will fail.
	err := writeFileAtomic(filepath.Join(t.TempDir(), "does-not-exist.md"), []byte("content"))
	if err == nil {
		t.Fatal("writeFileAtomic should error when file doesn't exist (Stat fails)")
	}
}

func TestWriteFileAtomic_UnwritableDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "file.md")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Make the directory unwritable so CreateTemp fails.
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	err := writeFileAtomic(path, []byte("new content"))
	if err == nil {
		t.Fatal("writeFileAtomic should error when directory is not writable")
	}
}

// ---------------------------------------------------------------------------
// Rewrite: writeFileAtomic fails (directory made unwritable after read).
// ---------------------------------------------------------------------------

func TestRewrite_WriteFailure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := "# Idea\n\n**Status:** Draft\n"
	path := filepath.Join(dir, "sample.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	// Make dir unwritable so the atomic write (CreateTemp) fails.
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	_, err := Rewrite(path, IdeaApproved)
	if err == nil {
		t.Fatal("Rewrite should fail when directory is not writable")
	}
}

// ---------------------------------------------------------------------------
// readStatus: scanner error path is hard to trigger naturally.
// We test it indirectly by verifying readStatus works on a valid file.
// The scanner error requires an I/O failure mid-read, which is not
// realistically testable without mocking. Instead we verify the function
// signature handles the file-not-found case correctly.
// ---------------------------------------------------------------------------

func TestReadStatus_FileNotFound(t *testing.T) {
	t.Parallel()
	_, err := readStatus(filepath.Join(t.TempDir(), "nonexistent.md"))
	if err == nil {
		t.Fatal("readStatus should error on nonexistent file")
	}
}

func TestReadStatus_FoundValue(t *testing.T) {
	t.Parallel()
	content := "# Idea\n\n**Status:** Under Review\n"
	path := writeFixture(t, content)
	got, err := readStatus(path)
	if err != nil {
		t.Fatalf("readStatus: %v", err)
	}
	if got != Status("Under Review") {
		t.Errorf("readStatus = %q; want %q", got, "Under Review")
	}
}
