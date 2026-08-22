// Package issue — lifecycle transition orchestration for the Issue kind.
//
// This file hosts ChangeStatus, the kind-specific orchestrator invoked by
// `specscore issue change-status`. It performs line-based YAML frontmatter
// rewriting for the `status:` field (and optionally `severity:`,
// `rejection_reason:`, `rejection_notes:` fields).
//
// LINT INVOCATION lives in the cobra adapter (internal/cli/issue.go), NOT
// here, to avoid an import cycle: pkg/lint already imports pkg/issue for
// the issue-* lint rules, so pkg/issue cannot depend back on pkg/lint.
// The adapter passes a PostMutationHook into ChangeStatus; this package
// only knows "run the post-mutation hook, and roll back if it fails".
package issue

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// Testable indirections for file I/O.
var (
	readFile  = os.ReadFile
	writeFile = os.WriteFile
)

// PostMutationHook is called after a successful status rewrite.
type PostMutationHook func() error

// ChangeStatusOptions packages the inputs to ChangeStatus.
type ChangeStatusOptions struct {
	// SpecRoot is the project root that contains the `spec/` subtree
	// (NOT the `spec/` directory itself).
	SpecRoot string

	// Slug is the Issue slug.
	Slug string

	// To is the target status (lowercase): investigating, resolved, rejected.
	To string

	// Severity is an optional severity override. Required when transitioning
	// and current severity is absent.
	Severity string

	// Reason is required when To=rejected.
	Reason string

	// Notes is optional when To=rejected.
	Notes string

	// PostMutation is the post-rewrite hook (typically a spec-lint pass).
	PostMutation PostMutationHook
}

// ChangeStatusResult is the success payload returned by ChangeStatus.
type ChangeStatusResult struct {
	Slug string
	Path string
	From string
	To   string
}

// LegalTransitions returns the issue transition matrix.
func LegalTransitions() map[string][]string {
	return map[string][]string{
		"open":          {"investigating", "resolved", "rejected"},
		"investigating": {"resolved", "rejected"},
	}
}

// IsLegalTransition checks if from→to is a valid issue transition.
func IsLegalTransition(from, to string) bool {
	targets, ok := LegalTransitions()[from]
	if !ok {
		return false
	}
	for _, t := range targets {
		if t == to {
			return true
		}
	}
	return false
}

// ValidTargetStatuses returns valid --to values for issue change-status.
var ValidTargetStatuses = []string{"investigating", "resolved", "rejected"}

// ValidReasonValues returns valid --reason values for rejection.
var ValidReasonValues = []string{"not-a-defect", "wont-fix", "duplicate", "not-reproducible", "by-design", "deferred"}

// ChangeStatus performs an Issue-kind lifecycle transition end-to-end.
//
// Flow:
//  1. Find the issue file by slug using DiscoverAll.
//  2. Parse current status from frontmatter.
//  3. Validate the transition is legal.
//  4. Check severity gating.
//  5. Check rejection gating (require Reason when To=rejected).
//  6. Rewrite frontmatter fields.
//  7. Invoke PostMutation hook; rollback on failure.
func ChangeStatus(opts ChangeStatusOptions) (ChangeStatusResult, error) {
	if opts.SpecRoot == "" {
		return ChangeStatusResult{}, exitcode.UnexpectedErrorf("ChangeStatus: SpecRoot required")
	}
	if opts.Slug == "" {
		return ChangeStatusResult{}, exitcode.InvalidArgsErrorf("ChangeStatus: slug required")
	}
	if opts.To == "" {
		return ChangeStatusResult{}, exitcode.InvalidArgsErrorf("ChangeStatus: target status required")
	}

	// (1) Find the issue file by slug.
	specDir := filepath.Join(opts.SpecRoot, "spec")
	discovered, err := DiscoverAll(specDir)
	if err != nil {
		return ChangeStatusResult{}, exitcode.UnexpectedErrorf("discovering issues: %v", err)
	}
	var issuePath string
	for _, d := range discovered {
		if d.Slug == opts.Slug {
			issuePath = d.Path
			break
		}
	}
	if issuePath == "" {
		return ChangeStatusResult{}, exitcode.NotFoundErrorf("issue %q not found under %s", opts.Slug, specDir)
	}

	// (2) Parse current status from frontmatter.
	content, err := readFile(issuePath)
	if err != nil {
		return ChangeStatusResult{}, exitcode.UnexpectedErrorf("reading %s: %v", issuePath, err)
	}
	from := extractFrontmatterValue(string(content), "status")
	if from == "" {
		return ChangeStatusResult{}, exitcode.UnexpectedErrorf("issue %s has no status in frontmatter", issuePath)
	}

	// (3) Validate transition legality.
	if !IsLegalTransition(from, opts.To) {
		targets, ok := LegalTransitions()[from]
		if !ok || len(targets) == 0 {
			return ChangeStatusResult{}, exitcode.InvalidStateErrorf(
				"invalid transition: issue %q is in status %q; no legal targets from this state",
				opts.Slug, from)
		}
		return ChangeStatusResult{}, exitcode.InvalidStateErrorf(
			"invalid transition: issue %q is in status %q; legal targets: %s",
			opts.Slug, from, strings.Join(targets, ", "))
	}

	// (4) Severity gating: if transitioning and severity is absent, require it.
	currentSeverity := extractFrontmatterValue(string(content), "severity")
	if currentSeverity == "" || currentSeverity == "unset" {
		if opts.Severity == "" {
			return ChangeStatusResult{}, exitcode.InvalidArgsErrorf(
				"severity is required when transitioning issue %q (current severity is unset); use --severity",
				opts.Slug)
		}
	}

	// (5) Rejection gating.
	if opts.To == "rejected" && opts.Reason == "" {
		return ChangeStatusResult{}, exitcode.InvalidArgsErrorf(
			"--reason is required when transitioning issue %q to rejected", opts.Slug)
	}

	// (6) Rewrite frontmatter.
	newContent := rewriteFrontmatter(string(content), opts)
	if err := writeFile(issuePath, []byte(newContent), 0o644); err != nil {
		return ChangeStatusResult{}, exitcode.UnexpectedErrorf("writing %s: %v", issuePath, err)
	}

	// (7) PostMutation hook — rollback on failure.
	if opts.PostMutation != nil {
		if err := opts.PostMutation(); err != nil {
			// Rollback: restore original file content.
			_ = writeFile(issuePath, content, 0o644)
			return ChangeStatusResult{}, err
		}
	}

	// (8) Persistence check. The hook returning nil proves only that the
	// derived-index pass did not ERROR, not that the requested status is the
	// one on disk after its `--fix` rewrites. An Issue keeps its status in the
	// frontmatter only, so verify there. Never report a transition the file
	// does not carry.
	if persisted, readErr := readFile(issuePath); readErr != nil {
		_ = writeFile(issuePath, content, 0o644)
		return ChangeStatusResult{}, exitcode.UnexpectedErrorf(
			"re-reading %s after the post-mutation hook: %v (rolled back)", issuePath, readErr)
	} else if got := extractFrontmatterValue(string(persisted), "status"); got != string(opts.To) {
		_ = writeFile(issuePath, content, 0o644)
		return ChangeStatusResult{}, exitcode.UnexpectedErrorf(
			"transition did not persist: %s requested status %q but still reads %q "+
				"after the post-mutation index sync (rolled back)",
			issuePath, string(opts.To), got)
	}

	return ChangeStatusResult{
		Slug: opts.Slug,
		Path: issuePath,
		From: from,
		To:   opts.To,
	}, nil
}

// extractFrontmatterValue returns the value for a given key in the YAML
// frontmatter, or "" if not found.
func extractFrontmatterValue(content, key string) string {
	front, _, ok := splitFrontmatter(content)
	if !ok {
		return ""
	}
	prefix := key + ":"
	for _, line := range strings.Split(front, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
		}
	}
	return ""
}

// rewriteFrontmatter replaces the status line (and optionally adds/replaces
// severity, rejection_reason, rejection_notes) within the YAML frontmatter,
// preserving the rest of the file byte-identical.
func rewriteFrontmatter(content string, opts ChangeStatusOptions) string {
	lines := strings.Split(content, "\n")

	// Find frontmatter boundaries.
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != "---" {
		return content
	}
	closeIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == "---" {
			closeIdx = i
			break
		}
	}
	if closeIdx < 0 {
		return content
	}

	// Rewrite within frontmatter lines (indices 1..closeIdx-1).
	// Use a full-slice expression to cap capacity so append cannot
	// overwrite lines[closeIdx] (the closing "---").
	fmLines := lines[1:closeIdx:closeIdx]
	fmLines = setFrontmatterField(fmLines, "status", opts.To)
	if opts.Severity != "" {
		fmLines = setFrontmatterField(fmLines, "severity", opts.Severity)
	}
	if opts.To == "rejected" {
		fmLines = setFrontmatterField(fmLines, "rejection_reason", opts.Reason)
		if opts.Notes != "" {
			fmLines = setFrontmatterField(fmLines, "rejection_notes", opts.Notes)
		}
	}

	// Reassemble.
	var result []string
	result = append(result, lines[0])        // opening ---
	result = append(result, fmLines...)      // frontmatter body
	result = append(result, lines[closeIdx]) // closing ---
	result = append(result, lines[closeIdx+1:]...)
	return strings.Join(result, "\n")
}

// setFrontmatterField replaces the value of an existing key or appends the
// key at the end of the frontmatter lines.
func setFrontmatterField(fmLines []string, key, value string) []string {
	prefix := key + ":"
	for i, line := range fmLines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) {
			fmLines[i] = fmt.Sprintf("%s: %s", key, value)
			return fmLines
		}
	}
	// Key not found — append before end.
	fmLines = append(fmLines, fmt.Sprintf("%s: %s", key, value))
	return fmLines
}
