package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/plan"
)

// TestPlanFieldValue_AllFields exercises every branch of planFieldValue,
// including the default case that is unreachable through the validated CLI
// path (parsePlanFields rejects unknown names before planFieldValue runs).
func TestPlanFieldValue_AllFields(t *testing.T) {
	p := &plan.Plan{
		Status:        "  Approved  ",
		SourceFeature: "cli/plan",
		Mode:          plan.ModeStub,
		Date:          "2026-06-04",
		Owner:         "alex",
	}
	cases := map[string]string{
		"status":         "Approved", // trimmed
		"source-feature": "cli/plan",
		"mode":           "stub",
		"date":           "2026-06-04",
		"owner":          "alex",
		"bogus":          "", // default branch
	}
	for field, want := range cases {
		if got := planFieldValue(p, field); got != want {
			t.Errorf("planFieldValue(%q) = %q, want %q", field, got, want)
		}
	}
}

// TestParsePlanFields_SkipsEmptyAndDedups covers the empty-part skip and the
// dedup (already-seen) branches of parsePlanFields.
func TestParsePlanFields_SkipsEmptyAndDedups(t *testing.T) {
	got, err := parsePlanFields("status, ,status,source-feature")
	if err != nil {
		t.Fatalf("parsePlanFields: %v", err)
	}
	want := []string{"status", "source-feature"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("parsePlanFields = %v, want %v", got, want)
	}
}

// TestPlanList_FieldsModeDateOwner covers planFieldValue's mode/date/owner
// branches through the command path.
func TestPlanList_FieldsModeDateOwner(t *testing.T) {
	plansDir := setupPlansSpec(t)
	content := "# Plan: full-meta\n\n**Status:** Approved\n\n**Mode:** stub\n\n" +
		"**Date:** 2026-06-04\n\n**Owner:** alex\n\n## Tasks\n\n### Task 1: x\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(plansDir, "full-meta.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	stdout, _, err := runPlan(t, "list", "--fields", "mode,date,owner")
	if err != nil {
		t.Fatalf("plan list --fields mode,date,owner: %v", err)
	}
	for _, want := range []string{"mode: stub", "date:", "2026-06-04", "owner: alex"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output missing %q: %s", want, stdout)
		}
	}
}

// TestPlanInfo_FormatJSON covers the json branch of runPlanInfo.
func TestPlanInfo_FormatJSON(t *testing.T) {
	plansDir := setupPlansSpec(t)
	writePlanWithSource(t, plansDir, "cli-rules", "Completed", "cli/rules")

	stdout, _, err := runPlan(t, "info", "cli-rules", "--format", "json")
	if err != nil {
		t.Fatalf("plan info --format json: %v", err)
	}
	if !strings.Contains(stdout, `"slug": "cli-rules"`) {
		t.Errorf("json missing slug: %s", stdout)
	}
	if !strings.Contains(stdout, `"source_feature": "cli/rules"`) {
		t.Errorf("json missing source_feature: %s", stdout)
	}
}

// TestPlanInfo_FormatText covers writePlanInfoText (the text branch).
func TestPlanInfo_FormatText(t *testing.T) {
	plansDir := setupPlansSpec(t)
	writePlanWithSource(t, plansDir, "cli-rules", "Completed", "cli/rules")

	stdout, _, err := runPlan(t, "info", "cli-rules", "--format", "text")
	if err != nil {
		t.Fatalf("plan info --format text: %v", err)
	}
	for _, want := range []string{
		"Slug:", "cli-rules", "Status:", "Completed",
		"Source Feature: cli/rules", "Tasks: 1 total",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("text output missing %q: %s", want, stdout)
		}
	}
}

// TestPlanInfo_TooManyArgsExits2 covers the >1 positional argument branch.
func TestPlanInfo_TooManyArgsExits2(t *testing.T) {
	setupPlansSpec(t)

	_, _, err := runPlan(t, "info", "a", "b")
	if err == nil {
		t.Fatal("expected error for too many args")
	}
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d (InvalidArgs)", got, exitcode.InvalidArgs)
	}
}

// TestResolvePlansDir_ProjectFlagSuccess covers the --project success path.
func TestResolvePlansDir_ProjectFlagSuccess(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "spec", "features"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	got, err := resolvePlansDir(root)
	if err != nil {
		t.Fatalf("resolvePlansDir(project): %v", err)
	}
	if !strings.HasSuffix(got, filepath.Join("spec", "plans")) {
		t.Errorf("got %q, want a path ending in spec/plans", got)
	}
}

// TestResolvePlansDir_ProjectAbsError covers the filepathAbsFn error branch.
func TestResolvePlansDir_ProjectAbsError(t *testing.T) {
	old := filepathAbsFn
	filepathAbsFn = func(string) (string, error) { return "", errors.New("abs boom") }
	t.Cleanup(func() { filepathAbsFn = old })

	_, err := resolvePlansDir("whatever")
	if err == nil {
		t.Fatal("expected error from abs failure")
	}
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d (InvalidArgs)", got, exitcode.InvalidArgs)
	}
}

// TestResolvePlansDir_GetwdError covers the osGetwdFn error branch.
func TestResolvePlansDir_GetwdError(t *testing.T) {
	old := osGetwdFn
	osGetwdFn = func() (string, error) { return "", errors.New("getwd boom") }
	t.Cleanup(func() { osGetwdFn = old })

	_, err := resolvePlansDir("")
	if err == nil {
		t.Fatal("expected error from getwd failure")
	}
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Errorf("exit code = %d, want %d (Unexpected)", got, exitcode.Unexpected)
	}
}

// TestResolvePlansDir_NoRepoRootError covers the FindSpecRepoRoot error branch
// (a working directory with no spec repo above it).
func TestResolvePlansDir_NoRepoRootError(t *testing.T) {
	dir := t.TempDir() // no spec/features marker anywhere above
	old := osGetwdFn
	osGetwdFn = func() (string, error) { return dir, nil }
	t.Cleanup(func() { osGetwdFn = old })

	if _, err := resolvePlansDir(""); err == nil {
		t.Fatal("expected error when no spec repo root is found")
	}
}

// TestPlanList_ResolvePlansDirError covers runPlanList's resolvePlansDir error
// branch (the caller's propagation of the error).
func TestPlanList_ResolvePlansDirError(t *testing.T) {
	old := osGetwdFn
	osGetwdFn = func() (string, error) { return "", errors.New("getwd boom") }
	t.Cleanup(func() { osGetwdFn = old })

	if _, _, err := runPlan(t, "list"); err == nil {
		t.Fatal("expected error when plans dir cannot be resolved")
	}
}

// TestPlanInfo_ResolvePlansDirError covers runPlanInfo's resolvePlansDir error
// branch.
func TestPlanInfo_ResolvePlansDirError(t *testing.T) {
	old := osGetwdFn
	osGetwdFn = func() (string, error) { return "", errors.New("getwd boom") }
	t.Cleanup(func() { osGetwdFn = old })

	if _, _, err := runPlan(t, "info", "any"); err == nil {
		t.Fatal("expected error when plans dir cannot be resolved")
	}
}

// TestPlanList_DiscoverError covers runPlanList's plan.Discover error branch by
// making spec/plans a regular file so os.ReadDir fails inside Discover.
func TestPlanList_DiscoverError(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "spec", "features"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// spec/plans is a file, not a directory.
	if err := os.WriteFile(filepath.Join(root, "spec", "plans"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	withCwd(t, root)

	_, _, err := runPlan(t, "list")
	if err == nil {
		t.Fatal("expected error when plans dir is unreadable")
	}
	if !strings.Contains(err.Error(), "discovering plans") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestPlanList_FieldsStatusFilterExcludes covers the matched()==false continue
// branch inside the --fields output path.
func TestPlanList_FieldsStatusFilterExcludes(t *testing.T) {
	plansDir := setupPlansSpec(t)
	writePlanInDir(t, plansDir, "approved-one", "Approved")
	writePlanInDir(t, plansDir, "draft-one", "Draft")

	stdout, _, err := runPlan(t, "list", "--fields", "status", "--status", "Approved")
	if err != nil {
		t.Fatalf("plan list --fields status --status Approved: %v", err)
	}
	if !strings.Contains(stdout, "slug: approved-one") {
		t.Errorf("expected approved-one in output: %s", stdout)
	}
	if strings.Contains(stdout, "draft-one") {
		t.Errorf("draft-one should be filtered out: %s", stdout)
	}
}

// TestPlanList_FieldsFormatJSON covers the json branch of the --fields path.
func TestPlanList_FieldsFormatJSON(t *testing.T) {
	plansDir := setupPlansSpec(t)
	writePlanWithSource(t, plansDir, "fielded", "Approved", "cli/plan")

	stdout, _, err := runPlan(t, "list", "--fields", "status", "--format", "json")
	if err != nil {
		t.Fatalf("plan list --fields status --format json: %v", err)
	}
	if !strings.Contains(stdout, `"slug": "fielded"`) {
		t.Errorf("json missing slug: %s", stdout)
	}
	if !strings.Contains(stdout, `"status": "Approved"`) {
		t.Errorf("json missing status: %s", stdout)
	}
}

// TestPlanList_FieldsYAMLEncodeError covers the yaml.Encode error branch of the
// --fields path using a writer that always fails.
func TestPlanList_FieldsYAMLEncodeError(t *testing.T) {
	plansDir := setupPlansSpec(t)
	writePlanInDir(t, plansDir, "any", "Draft")

	cmd := planCommand()
	cmd.SetOut(&errWriter{})
	cmd.SetErr(&errWriter{})
	cmd.SetArgs([]string{"list", "--fields", "status"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when writer fails during yaml encode")
	}
}

// TestPlanList_YAMLEncodeError covers the yaml.Encode error branch of the
// non-fields path using a failing writer.
func TestPlanList_YAMLEncodeError(t *testing.T) {
	plansDir := setupPlansSpec(t)
	writePlanInDir(t, plansDir, "any", "Draft")

	cmd := planCommand()
	cmd.SetOut(&errWriter{})
	cmd.SetErr(&errWriter{})
	cmd.SetArgs([]string{"list", "--format", "yaml"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when writer fails during yaml encode")
	}
}

// TestPlanInfo_YAMLEncodeError covers runPlanInfo's yaml.Encode error branch.
func TestPlanInfo_YAMLEncodeError(t *testing.T) {
	plansDir := setupPlansSpec(t)
	writePlanInDir(t, plansDir, "any", "Draft")

	cmd := planCommand()
	cmd.SetOut(&errWriter{})
	cmd.SetErr(&errWriter{})
	cmd.SetArgs([]string{"info", "any"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when writer fails during yaml encode")
	}
}

// TestPlanInfo_ParseError covers runPlanInfo's plan.Parse error branch: the
// plan file exists (so resolvePlanPath's stat succeeds) but is unreadable, so
// Parse's os.Open fails. chmod-based, so skip when running as root.
func TestPlanInfo_ParseError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping as root: chmod won't restrict access")
	}
	plansDir := setupPlansSpec(t)
	writePlanInDir(t, plansDir, "locked", "Draft")
	locked := filepath.Join(plansDir, "locked.md")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o644) })

	_, _, err := runPlan(t, "info", "locked")
	if err == nil {
		t.Fatal("expected error when plan file is unreadable")
	}
	if !strings.Contains(err.Error(), "parsing plan") {
		t.Errorf("unexpected error: %v", err)
	}
}
