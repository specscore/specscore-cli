package issue

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// ---------------------------------------------------------------------------
// LegalTransitions
// ---------------------------------------------------------------------------

func TestLegalTransitions(t *testing.T) {
	m := LegalTransitions()

	// open can go to investigating, resolved, rejected.
	openTargets := m["open"]
	if len(openTargets) != 3 {
		t.Fatalf("open targets = %v; want 3 entries", openTargets)
	}
	wantOpen := map[string]bool{"investigating": true, "resolved": true, "rejected": true}
	for _, tgt := range openTargets {
		if !wantOpen[tgt] {
			t.Errorf("unexpected target for open: %q", tgt)
		}
	}

	// investigating can go to resolved, rejected.
	invTargets := m["investigating"]
	if len(invTargets) != 2 {
		t.Fatalf("investigating targets = %v; want 2 entries", invTargets)
	}
	wantInv := map[string]bool{"resolved": true, "rejected": true}
	for _, tgt := range invTargets {
		if !wantInv[tgt] {
			t.Errorf("unexpected target for investigating: %q", tgt)
		}
	}

	// Terminal states have no entry.
	for _, terminal := range []string{"resolved", "rejected"} {
		if targets, ok := m[terminal]; ok {
			t.Errorf("%q should not be in LegalTransitions map; got %v", terminal, targets)
		}
	}
}

// ---------------------------------------------------------------------------
// IsLegalTransition
// ---------------------------------------------------------------------------

func TestIsLegalTransition(t *testing.T) {
	valid := []struct {
		from, to string
	}{
		{"open", "investigating"},
		{"open", "resolved"},
		{"open", "rejected"},
		{"investigating", "resolved"},
		{"investigating", "rejected"},
	}
	for _, tc := range valid {
		if !IsLegalTransition(tc.from, tc.to) {
			t.Errorf("IsLegalTransition(%q, %q) = false; want true", tc.from, tc.to)
		}
	}

	invalid := []struct {
		from, to string
	}{
		{"resolved", "open"},
		{"resolved", "investigating"},
		{"rejected", "open"},
		{"rejected", "investigating"},
		{"open", "open"},
		{"investigating", "open"},
		{"unknown", "open"},
	}
	for _, tc := range invalid {
		if IsLegalTransition(tc.from, tc.to) {
			t.Errorf("IsLegalTransition(%q, %q) = true; want false", tc.from, tc.to)
		}
	}
}

// ---------------------------------------------------------------------------
// Helper: build a temp project with a single issue file.
// ---------------------------------------------------------------------------

func setupIssueProject(t *testing.T, slug, status, severity string) string {
	t.Helper()
	root := t.TempDir()
	issuesDir := filepath.Join(root, "spec", "issues")
	if err := os.MkdirAll(issuesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	var sev string
	if severity != "" {
		sev = "severity: " + severity + "\n"
	}

	content := "---\ntype: issue\nslug: " + slug + "\nstatus: " + status + "\ncaptured_at: 2026-01-01T00:00:00Z\ncaptured_by: testuser\n" + sev + "---\n\n# Issue: Test Issue\n\n## Description\n\nSome description.\n\n## Steps to Reproduce\n\nStep 1.\n\n## Expected vs Actual\n\nExpected X, got Y.\n"

	path := filepath.Join(issuesDir, slug+".md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// ---------------------------------------------------------------------------
// ChangeStatus: happy path — open → investigating
// ---------------------------------------------------------------------------

func TestChangeStatus_HappyPath_OpenToInvestigating(t *testing.T) {
	root := setupIssueProject(t, "test-issue", "open", "high")

	result, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root,
		Slug:     "test-issue",
		To:       "investigating",
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	if result.From != "open" {
		t.Errorf("From = %q; want %q", result.From, "open")
	}
	if result.To != "investigating" {
		t.Errorf("To = %q; want %q", result.To, "investigating")
	}
	if result.Slug != "test-issue" {
		t.Errorf("Slug = %q; want %q", result.Slug, "test-issue")
	}
	if result.Path == "" {
		t.Error("Path should not be empty")
	}

	// Verify file was rewritten.
	content, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "status: investigating") {
		t.Error("file should contain status: investigating")
	}
}

// ---------------------------------------------------------------------------
// ChangeStatus: happy path — open → rejected (with reason and notes)
// ---------------------------------------------------------------------------

func TestChangeStatus_HappyPath_OpenToRejected(t *testing.T) {
	root := setupIssueProject(t, "reject-me", "open", "low")

	result, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root,
		Slug:     "reject-me",
		To:       "rejected",
		Reason:   "not-a-defect",
		Notes:    "This is expected behavior.",
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	if result.From != "open" {
		t.Errorf("From = %q; want %q", result.From, "open")
	}
	if result.To != "rejected" {
		t.Errorf("To = %q; want %q", result.To, "rejected")
	}

	content, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)
	if !strings.Contains(s, "status: rejected") {
		t.Error("file should contain status: rejected")
	}
	if !strings.Contains(s, "rejection_reason: not-a-defect") {
		t.Error("file should contain rejection_reason")
	}
	if !strings.Contains(s, "rejection_notes: This is expected behavior.") {
		t.Error("file should contain rejection_notes")
	}
}

// ---------------------------------------------------------------------------
// ChangeStatus: slug not found
// ---------------------------------------------------------------------------

func TestChangeStatus_SlugNotFound(t *testing.T) {
	root := setupIssueProject(t, "existing", "open", "high")

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root,
		Slug:     "nonexistent",
		To:       "investigating",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent slug")
	}
	var ecErr *exitcode.Error
	if !errors.As(err, &ecErr) {
		t.Fatalf("error type = %T; want *exitcode.Error", err)
	}
	if ecErr.ExitCode() != exitcode.NotFound {
		t.Errorf("exit code = %d; want %d (NotFound)", ecErr.ExitCode(), exitcode.NotFound)
	}
}

// ---------------------------------------------------------------------------
// ChangeStatus: illegal transition (resolved → open)
// ---------------------------------------------------------------------------

func TestChangeStatus_IllegalTransition(t *testing.T) {
	root := setupIssueProject(t, "resolved-issue", "resolved", "medium")

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root,
		Slug:     "resolved-issue",
		To:       "open",
	})
	if err == nil {
		t.Fatal("expected error for illegal transition")
	}
	var ecErr *exitcode.Error
	if !errors.As(err, &ecErr) {
		t.Fatalf("error type = %T; want *exitcode.Error", err)
	}
	if ecErr.ExitCode() != exitcode.InvalidState {
		t.Errorf("exit code = %d; want %d (InvalidState)", ecErr.ExitCode(), exitcode.InvalidState)
	}
	// Terminal state message (no legal targets).
	if !strings.Contains(err.Error(), "no legal targets") {
		t.Errorf("error = %q; want to contain 'no legal targets'", err.Error())
	}
}

func TestChangeStatus_IllegalTransition_WithTargets(t *testing.T) {
	// open → open is illegal but "open" has legal targets, so it should list them.
	root := setupIssueProject(t, "open-issue", "open", "high")

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root,
		Slug:     "open-issue",
		To:       "open",
	})
	if err == nil {
		t.Fatal("expected error for illegal transition")
	}
	var ecErr *exitcode.Error
	if !errors.As(err, &ecErr) {
		t.Fatalf("error type = %T; want *exitcode.Error", err)
	}
	if ecErr.ExitCode() != exitcode.InvalidState {
		t.Errorf("exit code = %d; want %d (InvalidState)", ecErr.ExitCode(), exitcode.InvalidState)
	}
	if !strings.Contains(err.Error(), "legal targets") {
		t.Errorf("error = %q; want to contain 'legal targets'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// ChangeStatus: severity gating — required
// ---------------------------------------------------------------------------

func TestChangeStatus_SeverityGating_Required(t *testing.T) {
	// Issue without severity.
	root := setupIssueProject(t, "no-severity", "open", "")

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root,
		Slug:     "no-severity",
		To:       "investigating",
	})
	if err == nil {
		t.Fatal("expected error when severity is unset and not provided")
	}
	var ecErr *exitcode.Error
	if !errors.As(err, &ecErr) {
		t.Fatalf("error type = %T; want *exitcode.Error", err)
	}
	if ecErr.ExitCode() != exitcode.InvalidArgs {
		t.Errorf("exit code = %d; want %d (InvalidArgs)", ecErr.ExitCode(), exitcode.InvalidArgs)
	}
	if !strings.Contains(err.Error(), "severity is required") {
		t.Errorf("error = %q; want to contain 'severity is required'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// ChangeStatus: severity gating — already set
// ---------------------------------------------------------------------------

func TestChangeStatus_SeverityGating_AlreadySet(t *testing.T) {
	root := setupIssueProject(t, "has-severity", "open", "critical")

	result, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root,
		Slug:     "has-severity",
		To:       "investigating",
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	if result.To != "investigating" {
		t.Errorf("To = %q; want investigating", result.To)
	}
}

// ---------------------------------------------------------------------------
// ChangeStatus: severity override
// ---------------------------------------------------------------------------

func TestChangeStatus_SeverityOverride(t *testing.T) {
	root := setupIssueProject(t, "override-sev", "open", "low")

	result, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root,
		Slug:     "override-sev",
		To:       "investigating",
		Severity: "critical",
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}

	content, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "severity: critical") {
		t.Error("severity should be overridden to critical")
	}
	// Ensure old severity is replaced, not duplicated.
	if strings.Count(string(content), "severity:") != 1 {
		t.Error("severity should appear exactly once")
	}
}

// ---------------------------------------------------------------------------
// ChangeStatus: rejection gating — reason required
// ---------------------------------------------------------------------------

func TestChangeStatus_RejectionGating_Required(t *testing.T) {
	root := setupIssueProject(t, "reject-no-reason", "open", "medium")

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root,
		Slug:     "reject-no-reason",
		To:       "rejected",
	})
	if err == nil {
		t.Fatal("expected error when rejecting without reason")
	}
	var ecErr *exitcode.Error
	if !errors.As(err, &ecErr) {
		t.Fatalf("error type = %T; want *exitcode.Error", err)
	}
	if ecErr.ExitCode() != exitcode.InvalidArgs {
		t.Errorf("exit code = %d; want %d (InvalidArgs)", ecErr.ExitCode(), exitcode.InvalidArgs)
	}
	if !strings.Contains(err.Error(), "--reason is required") {
		t.Errorf("error = %q; want to contain '--reason is required'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// ChangeStatus: rollback on PostMutation hook failure
// ---------------------------------------------------------------------------

func TestChangeStatus_Rollback_OnHookFailure(t *testing.T) {
	root := setupIssueProject(t, "rollback-test", "open", "high")

	issuePath := filepath.Join(root, "spec", "issues", "rollback-test.md")
	originalContent, err := os.ReadFile(issuePath)
	if err != nil {
		t.Fatal(err)
	}

	hookErr := errors.New("lint failed")
	_, err = ChangeStatus(ChangeStatusOptions{
		SpecRoot: root,
		Slug:     "rollback-test",
		To:       "investigating",
		PostMutation: func() error {
			return hookErr
		},
	})
	if err == nil {
		t.Fatal("expected hook error to propagate")
	}
	if err != hookErr {
		t.Errorf("error = %v; want hookErr", err)
	}

	// Verify file was rolled back.
	restored, err := os.ReadFile(issuePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != string(originalContent) {
		t.Error("file should be rolled back to original content after hook failure")
	}
}

// ---------------------------------------------------------------------------
// ChangeStatus: frontmatter preservation
// ---------------------------------------------------------------------------

func TestChangeStatus_FrontmatterPreservation(t *testing.T) {
	root := t.TempDir()
	issuesDir := filepath.Join(root, "spec", "issues")
	if err := os.MkdirAll(issuesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	content := `---
type: issue
slug: preserve-me
status: open
captured_at: 2026-01-01T00:00:00Z
captured_by: alice
severity: high
affected_component: auth
github_issue: https://github.com/org/repo/issues/99
---

# Issue: Preserve Me

## Description

Important description.

## Steps to Reproduce

1. Do something.

## Expected vs Actual

Expected A, got B.
`
	path := filepath.Join(issuesDir, "preserve-me.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root,
		Slug:     "preserve-me",
		To:       "investigating",
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}

	newContent, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(newContent)

	// Status should be updated.
	if !strings.Contains(s, "status: investigating") {
		t.Error("status should be updated to investigating")
	}
	// All other frontmatter fields should be preserved.
	checks := []string{
		"type: issue",
		"slug: preserve-me",
		"captured_at: 2026-01-01T00:00:00Z",
		"captured_by: alice",
		"severity: high",
		"affected_component: auth",
		"github_issue: https://github.com/org/repo/issues/99",
	}
	for _, chk := range checks {
		if !strings.Contains(s, chk) {
			t.Errorf("preserved field missing: %q", chk)
		}
	}
	// Body should be byte-identical.
	bodyMarker := "# Issue: Preserve Me"
	idx := strings.Index(s, bodyMarker)
	if idx < 0 {
		t.Fatal("body section not found")
	}
	originalBodyIdx := strings.Index(content, bodyMarker)
	originalBody := content[originalBodyIdx:]
	actualBody := s[idx:]
	if actualBody != originalBody {
		t.Errorf("body content changed:\n got:  %q\n want: %q", actualBody, originalBody)
	}
}

// ---------------------------------------------------------------------------
// ChangeStatus: empty SpecRoot
// ---------------------------------------------------------------------------

func TestChangeStatus_EmptySpecRoot(t *testing.T) {
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: "",
		Slug:     "test",
		To:       "investigating",
	})
	if err == nil {
		t.Fatal("expected error for empty SpecRoot")
	}
	var ecErr *exitcode.Error
	if !errors.As(err, &ecErr) {
		t.Fatalf("error type = %T; want *exitcode.Error", err)
	}
	if ecErr.ExitCode() != exitcode.Unexpected {
		t.Errorf("exit code = %d; want %d (Unexpected)", ecErr.ExitCode(), exitcode.Unexpected)
	}
}

// ---------------------------------------------------------------------------
// ChangeStatus: empty Slug
// ---------------------------------------------------------------------------

func TestChangeStatus_EmptySlug(t *testing.T) {
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: "/tmp",
		Slug:     "",
		To:       "investigating",
	})
	if err == nil {
		t.Fatal("expected error for empty Slug")
	}
	var ecErr *exitcode.Error
	if !errors.As(err, &ecErr) {
		t.Fatalf("error type = %T; want *exitcode.Error", err)
	}
	if ecErr.ExitCode() != exitcode.InvalidArgs {
		t.Errorf("exit code = %d; want %d (InvalidArgs)", ecErr.ExitCode(), exitcode.InvalidArgs)
	}
}

// ---------------------------------------------------------------------------
// ChangeStatus: empty To
// ---------------------------------------------------------------------------

func TestChangeStatus_EmptyTo(t *testing.T) {
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: "/tmp",
		Slug:     "test",
		To:       "",
	})
	if err == nil {
		t.Fatal("expected error for empty To")
	}
	var ecErr *exitcode.Error
	if !errors.As(err, &ecErr) {
		t.Fatalf("error type = %T; want *exitcode.Error", err)
	}
	if ecErr.ExitCode() != exitcode.InvalidArgs {
		t.Errorf("exit code = %d; want %d (InvalidArgs)", ecErr.ExitCode(), exitcode.InvalidArgs)
	}
}

// ---------------------------------------------------------------------------
// ChangeStatus: severity "unset" treated as absent
// ---------------------------------------------------------------------------

func TestChangeStatus_SeverityUnsetTreatedAsAbsent(t *testing.T) {
	root := t.TempDir()
	issuesDir := filepath.Join(root, "spec", "issues")
	if err := os.MkdirAll(issuesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\ntype: issue\nslug: unset-sev\nstatus: open\ncaptured_at: 2026-01-01T00:00:00Z\ncaptured_by: tester\nseverity: unset\n---\n\n# Issue: Unset Sev\n\n## Description\n\nX.\n"
	if err := os.WriteFile(filepath.Join(issuesDir, "unset-sev.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Without severity flag, should fail.
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root,
		Slug:     "unset-sev",
		To:       "investigating",
	})
	if err == nil {
		t.Fatal("expected error when severity is 'unset' and no --severity flag")
	}

	// With severity flag, should succeed.
	result, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root,
		Slug:     "unset-sev",
		To:       "investigating",
		Severity: "medium",
	})
	if err != nil {
		t.Fatalf("ChangeStatus with severity: %v", err)
	}
	if result.To != "investigating" {
		t.Errorf("To = %q; want investigating", result.To)
	}
}

// ---------------------------------------------------------------------------
// ChangeStatus: PostMutation nil (no hook) succeeds
// ---------------------------------------------------------------------------

func TestChangeStatus_NoPostMutationHook(t *testing.T) {
	root := setupIssueProject(t, "no-hook", "open", "high")

	result, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot:     root,
		Slug:         "no-hook",
		To:           "investigating",
		PostMutation: nil,
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	if result.To != "investigating" {
		t.Errorf("To = %q; want investigating", result.To)
	}
}

// ---------------------------------------------------------------------------
// ChangeStatus: rejected without notes (notes optional)
// ---------------------------------------------------------------------------

func TestChangeStatus_RejectedWithoutNotes(t *testing.T) {
	root := setupIssueProject(t, "reject-no-notes", "open", "low")

	result, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root,
		Slug:     "reject-no-notes",
		To:       "rejected",
		Reason:   "wont-fix",
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}

	content, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)
	if !strings.Contains(s, "rejection_reason: wont-fix") {
		t.Error("file should contain rejection_reason")
	}
	if strings.Contains(s, "rejection_notes:") {
		t.Error("file should NOT contain rejection_notes when notes empty")
	}
}

// ---------------------------------------------------------------------------
// ChangeStatus: investigating → resolved
// ---------------------------------------------------------------------------

func TestChangeStatus_InvestigatingToResolved(t *testing.T) {
	root := setupIssueProject(t, "inv-to-res", "investigating", "high")

	result, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root,
		Slug:     "inv-to-res",
		To:       "resolved",
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	if result.From != "investigating" {
		t.Errorf("From = %q; want investigating", result.From)
	}
	if result.To != "resolved" {
		t.Errorf("To = %q; want resolved", result.To)
	}
}

// ---------------------------------------------------------------------------
// ChangeStatus: issue with no status in frontmatter
// ---------------------------------------------------------------------------

func TestChangeStatus_NoStatusInFrontmatter(t *testing.T) {
	root := t.TempDir()
	issuesDir := filepath.Join(root, "spec", "issues")
	if err := os.MkdirAll(issuesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Issue without status field.
	content := "---\ntype: issue\nslug: no-status\ncaptured_at: 2026-01-01T00:00:00Z\ncaptured_by: tester\n---\n\n# Issue: No Status\n\n## Description\n\nX.\n"
	if err := os.WriteFile(filepath.Join(issuesDir, "no-status.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root,
		Slug:     "no-status",
		To:       "investigating",
	})
	if err == nil {
		t.Fatal("expected error for issue with no status field")
	}
	var ecErr *exitcode.Error
	if !errors.As(err, &ecErr) {
		t.Fatalf("error type = %T; want *exitcode.Error", err)
	}
	if ecErr.ExitCode() != exitcode.Unexpected {
		t.Errorf("exit code = %d; want %d (Unexpected)", ecErr.ExitCode(), exitcode.Unexpected)
	}
}

// ---------------------------------------------------------------------------
// ChangeStatus: DiscoverAll error (non-existent spec dir handled)
// ---------------------------------------------------------------------------

func TestChangeStatus_SpecDirDoesNotExist(t *testing.T) {
	root := t.TempDir()
	// Don't create spec/ dir — DiscoverAll returns empty, slug not found.
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root,
		Slug:     "anything",
		To:       "investigating",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var ecErr *exitcode.Error
	if !errors.As(err, &ecErr) {
		t.Fatalf("error type = %T; want *exitcode.Error", err)
	}
	if ecErr.ExitCode() != exitcode.NotFound {
		t.Errorf("exit code = %d; want %d (NotFound)", ecErr.ExitCode(), exitcode.NotFound)
	}
}

// ---------------------------------------------------------------------------
// ChangeStatus: PostMutation succeeds
// ---------------------------------------------------------------------------

func TestChangeStatus_PostMutationSucceeds(t *testing.T) {
	root := setupIssueProject(t, "hook-ok", "open", "medium")

	hookCalled := false
	result, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root,
		Slug:     "hook-ok",
		To:       "investigating",
		PostMutation: func() error {
			hookCalled = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	if !hookCalled {
		t.Error("PostMutation hook was not called")
	}
	if result.To != "investigating" {
		t.Errorf("To = %q; want investigating", result.To)
	}
}

// ---------------------------------------------------------------------------
// extractFrontmatterValue: edge cases
// ---------------------------------------------------------------------------

func TestExtractFrontmatterValue_NoFrontmatter(t *testing.T) {
	got := extractFrontmatterValue("no frontmatter here", "status")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestExtractFrontmatterValue_KeyNotPresent(t *testing.T) {
	content := "---\ntype: issue\nslug: foo\n---\nBody.\n"
	got := extractFrontmatterValue(content, "status")
	if got != "" {
		t.Errorf("expected empty for missing key, got %q", got)
	}
}

func TestExtractFrontmatterValue_KeyPresent(t *testing.T) {
	content := "---\ntype: issue\nstatus: open\n---\nBody.\n"
	got := extractFrontmatterValue(content, "status")
	if got != "open" {
		t.Errorf("expected %q, got %q", "open", got)
	}
}

// ---------------------------------------------------------------------------
// rewriteFrontmatter: edge cases
// ---------------------------------------------------------------------------

func TestRewriteFrontmatter_NoFrontmatter(t *testing.T) {
	content := "no frontmatter at all"
	got := rewriteFrontmatter(content, ChangeStatusOptions{To: "investigating"})
	if got != content {
		t.Error("rewriteFrontmatter should return content unchanged if no frontmatter")
	}
}

func TestRewriteFrontmatter_NoClosingDelimiter(t *testing.T) {
	content := "---\nstatus: open\nno closing"
	got := rewriteFrontmatter(content, ChangeStatusOptions{To: "investigating"})
	if got != content {
		t.Error("rewriteFrontmatter should return content unchanged if no closing ---")
	}
}

func TestRewriteFrontmatter_EmptyContent(t *testing.T) {
	got := rewriteFrontmatter("", ChangeStatusOptions{To: "investigating"})
	if got != "" {
		t.Error("rewriteFrontmatter should return empty for empty content")
	}
}

func TestRewriteFrontmatter_AppendsNewField(t *testing.T) {
	content := "---\ntype: issue\nstatus: open\n---\nBody.\n"
	opts := ChangeStatusOptions{
		To:       "rejected",
		Reason:   "wont-fix",
		Notes:    "Deferred.",
		Severity: "low",
	}
	got := rewriteFrontmatter(content, opts)
	if !strings.Contains(got, "status: rejected") {
		t.Error("status should be rewritten")
	}
	if !strings.Contains(got, "rejection_reason: wont-fix") {
		t.Error("rejection_reason should be appended")
	}
	if !strings.Contains(got, "rejection_notes: Deferred.") {
		t.Error("rejection_notes should be appended")
	}
	if !strings.Contains(got, "severity: low") {
		t.Error("severity should be appended")
	}
}

// ---------------------------------------------------------------------------
// setFrontmatterField: replaces existing vs appends
// ---------------------------------------------------------------------------

func TestSetFrontmatterField_ReplacesExisting(t *testing.T) {
	lines := []string{"type: issue", "status: open", "slug: foo"}
	result := setFrontmatterField(lines, "status", "investigating")
	found := false
	for _, l := range result {
		if l == "status: investigating" {
			found = true
		}
		if l == "status: open" {
			t.Error("old value should be replaced")
		}
	}
	if !found {
		t.Error("new value not found")
	}
}

func TestSetFrontmatterField_AppendsNew(t *testing.T) {
	lines := []string{"type: issue", "slug: foo"}
	result := setFrontmatterField(lines, "severity", "high")
	if len(result) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(result))
	}
	if result[2] != "severity: high" {
		t.Errorf("last line = %q; want %q", result[2], "severity: high")
	}
}

// ---------------------------------------------------------------------------
// ChangeStatus: readFile error (using testable indirection)
// ---------------------------------------------------------------------------

func TestChangeStatus_ReadFileError(t *testing.T) {
	root := setupIssueProject(t, "read-err", "open", "high")

	// Override readFile to fail after DiscoverAll succeeds (DiscoverAll uses
	// os.ReadFile directly via Parse, not the indirection var).
	origRead := readFile
	readFile = func(name string) ([]byte, error) {
		return nil, errors.New("simulated read error")
	}
	t.Cleanup(func() { readFile = origRead })

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root,
		Slug:     "read-err",
		To:       "investigating",
	})
	if err == nil {
		t.Fatal("expected error from readFile failure")
	}
	var ecErr *exitcode.Error
	if !errors.As(err, &ecErr) {
		t.Fatalf("error type = %T; want *exitcode.Error", err)
	}
	if ecErr.ExitCode() != exitcode.Unexpected {
		t.Errorf("exit code = %d; want %d (Unexpected)", ecErr.ExitCode(), exitcode.Unexpected)
	}
}

// ---------------------------------------------------------------------------
// ChangeStatus: writeFile error (using testable indirection)
// ---------------------------------------------------------------------------

func TestChangeStatus_WriteFileError(t *testing.T) {
	root := setupIssueProject(t, "write-err", "open", "high")

	origWrite := writeFile
	writeFile = func(name string, data []byte, perm os.FileMode) error {
		return errors.New("simulated write error")
	}
	t.Cleanup(func() { writeFile = origWrite })

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root,
		Slug:     "write-err",
		To:       "investigating",
	})
	if err == nil {
		t.Fatal("expected error from writeFile failure")
	}
	var ecErr *exitcode.Error
	if !errors.As(err, &ecErr) {
		t.Fatalf("error type = %T; want *exitcode.Error", err)
	}
	if ecErr.ExitCode() != exitcode.Unexpected {
		t.Errorf("exit code = %d; want %d (Unexpected)", ecErr.ExitCode(), exitcode.Unexpected)
	}
}

// ---------------------------------------------------------------------------
// ChangeStatus: DiscoverAll returns error (using unreadable spec dir)
// ---------------------------------------------------------------------------

func TestChangeStatus_DiscoverAllError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root")
	}
	root := t.TempDir()
	specDir := filepath.Join(root, "spec")
	if err := os.MkdirAll(filepath.Join(specDir, "issues", "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Put a file inside subdir.
	if err := os.WriteFile(filepath.Join(specDir, "issues", "subdir", "x.md"), []byte(minimalIssue("x")), 0o644); err != nil {
		t.Fatal(err)
	}
	// Make subdir unreadable to cause walk error.
	if err := os.Chmod(filepath.Join(specDir, "issues", "subdir"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(specDir, "issues", "subdir"), 0o755) })

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root,
		Slug:     "anything",
		To:       "investigating",
	})
	if err == nil {
		t.Fatal("expected error from DiscoverAll failure")
	}
	var ecErr *exitcode.Error
	if !errors.As(err, &ecErr) {
		t.Fatalf("error type = %T; want *exitcode.Error", err)
	}
	if ecErr.ExitCode() != exitcode.Unexpected {
		t.Errorf("exit code = %d; want %d (Unexpected)", ecErr.ExitCode(), exitcode.Unexpected)
	}
}

// ---------------------------------------------------------------------------
// ValidTargetStatuses and ValidReasonValues: verify expected values
// ---------------------------------------------------------------------------

func TestValidTargetStatuses(t *testing.T) {
	expected := []string{"investigating", "resolved", "rejected"}
	if len(ValidTargetStatuses) != len(expected) {
		t.Fatalf("len(ValidTargetStatuses) = %d; want %d", len(ValidTargetStatuses), len(expected))
	}
	for i, v := range expected {
		if ValidTargetStatuses[i] != v {
			t.Errorf("ValidTargetStatuses[%d] = %q; want %q", i, ValidTargetStatuses[i], v)
		}
	}
}

func TestValidReasonValues(t *testing.T) {
	expected := []string{"not-a-defect", "wont-fix", "duplicate", "not-reproducible", "by-design", "deferred"}
	if len(ValidReasonValues) != len(expected) {
		t.Fatalf("len(ValidReasonValues) = %d; want %d", len(ValidReasonValues), len(expected))
	}
	for i, v := range expected {
		if ValidReasonValues[i] != v {
			t.Errorf("ValidReasonValues[%d] = %q; want %q", i, ValidReasonValues[i], v)
		}
	}
}

// ---------------------------------------------------------------------------
// ChangeStatus: severity provided fills in missing severity
// ---------------------------------------------------------------------------

func TestChangeStatus_SeverityProvidedWhenMissing(t *testing.T) {
	root := setupIssueProject(t, "fill-severity", "open", "")

	result, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root,
		Slug:     "fill-severity",
		To:       "investigating",
		Severity: "high",
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}

	content, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "severity: high") {
		t.Error("severity should be written when provided")
	}
}

// TestChangeStatus_PostMutationRevertFailsLoudly covers the shared persistence
// guarantee for the Issue kind, whose status lives in the frontmatter only:
// when the post-mutation hook succeeds but leaves a different status on disk,
// the verb rolls back and fails rather than reporting a transition the file
// does not carry.
func TestChangeStatus_PostMutationRevertFailsLoudly(t *testing.T) {
	root := setupIssueProject(t, "revert-test", "open", "high")
	issuePath := filepath.Join(root, "spec", "issues", "revert-test.md")
	original, err := os.ReadFile(issuePath)
	if err != nil {
		t.Fatal(err)
	}

	_, err = ChangeStatus(ChangeStatusOptions{
		SpecRoot: root,
		Slug:     "revert-test",
		To:       "investigating",
		// A --fix pass that puts the old status back and reports success.
		PostMutation: func() error { return os.WriteFile(issuePath, original, 0o644) },
	})
	if err == nil {
		t.Fatal("expected a failure when the post-mutation pass reverts the status")
	}
	if !strings.Contains(err.Error(), "did not persist") {
		t.Errorf("error does not name the failure: %v", err)
	}
	restored, err := os.ReadFile(issuePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != string(original) {
		t.Errorf("issue not restored after a failed transition:\n%s", restored)
	}
}

// The re-read itself can fail (the artifact vanished under us). That is also a
// failed transition, not a success.
func TestChangeStatus_PostMutationReReadFailureFailsLoudly(t *testing.T) {
	root := setupIssueProject(t, "reread-test", "open", "high")

	origRead := readFile
	readFile = func(path string) ([]byte, error) {
		if strings.HasSuffix(path, "reread-test.md") && changeStatusReReadFailed {
			return nil, errors.New("injected re-read failure")
		}
		return origRead(path)
	}
	changeStatusReReadFailed = false
	t.Cleanup(func() { readFile = origRead; changeStatusReReadFailed = false })

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root,
		Slug:     "reread-test",
		To:       "investigating",
		// Arm the injected failure only for the post-hook re-read, so the
		// verb's earlier reads still see the real file.
		PostMutation: func() error { changeStatusReReadFailed = true; return nil },
	})
	if err == nil {
		t.Fatal("expected a failure when the artifact cannot be re-read")
	}
	if !strings.Contains(err.Error(), "re-reading") {
		t.Errorf("error does not name the failure: %v", err)
	}
}

// changeStatusReReadFailed arms the injected re-read failure above.
var changeStatusReReadFailed bool
