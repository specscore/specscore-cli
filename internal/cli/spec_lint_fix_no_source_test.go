package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/projectdef"
)

// sourcelessPlanContent is a lint-clean plan except that it declares no source
// line, so it fails P-002 with a fixable (no-source) violation.
const sourcelessPlanContent = `---
format: https://specscore.md/plan-specification
status: Draft
---

# Plan: P

**Status:** Draft
**Date:** 2026-01-01
**Owner:** t
**Supersedes:** —

## Summary

x

## Approach

x

## Tasks

### Task 1: Do

**Status:** pending

Do it.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
`

func setupSourcelessPlanProject(t *testing.T) (root, planPath string) {
	t.Helper()
	root = t.TempDir()
	if err := projectdef.WriteSpecConfig(root, projectdef.SpecConfig{}); err != nil {
		t.Fatalf("WriteSpecConfig: %v", err)
	}
	plansDir := filepath.Join(root, "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatalf("mkdir plans: %v", err)
	}
	planPath = filepath.Join(plansDir, "p.md")
	if err := os.WriteFile(planPath, []byte(sourcelessPlanContent), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	return root, planPath
}

// AC: no-source is opt-in — a bare --fix must NOT add `**Source:** none`, while
// --fix=no-source repairs it. Plain lint advertises the fix.
func TestSpecLint_NoSourceFixIsOptIn(t *testing.T) {
	root, planPath := setupSourcelessPlanProject(t)

	// Plain lint: violation remains and the How-to-fix hint is on stderr.
	_, stderr, err := runSpec(t, "lint", "--project", root, "--rules=P-002")
	if err == nil {
		t.Fatal("expected a violation from plain lint")
	}
	if !strings.Contains(stderr, "How to fix") || !strings.Contains(stderr, "--fix=no-source") {
		t.Errorf("plain lint should advertise --fix=no-source, stderr: %q", stderr)
	}

	// Bare --fix must NOT touch the sourceless plan.
	if _, _, _ = runSpec(t, "lint", "--project", root, "--rules=P-002", "--fix"); true {
		data, _ := os.ReadFile(planPath)
		if strings.Contains(string(data), "**Source:** none") {
			t.Errorf("bare --fix must not add **Source:** none:\n%s", data)
		}
	}

	// --fix=no-source repairs it and lint then passes.
	_, _, err = runSpec(t, "lint", "--project", root, "--rules=P-002", "--fix=no-source")
	if err != nil {
		t.Fatalf("--fix=no-source should leave a clean tree, got: %v", err)
	}
	data, _ := os.ReadFile(planPath)
	if !strings.Contains(string(data), "**Source:** none") {
		t.Errorf("--fix=no-source should add **Source:** none:\n%s", data)
	}
}

// AC: --fix=<target> rejects unknown targets with an invalid-args exit.
func TestSpecLint_FixUnknownTargetRejected(t *testing.T) {
	root, _ := setupSourcelessPlanProject(t)
	_, _, err := runSpec(t, "lint", "--project", root, "--fix=bogus")
	if err == nil {
		t.Fatal("expected error for unknown --fix target")
	}
	if got := exitCodeOf(err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d", got, exitcode.InvalidArgs)
	}
}

// AC: a scoped --fix to an unrelated target must NOT trigger the no-source fix.
func TestSpecLint_ScopedFixExcludesNoSource(t *testing.T) {
	root, planPath := setupSourcelessPlanProject(t)
	_, _, _ = runSpec(t, "lint", "--project", root, "--fix=adherence-footer")
	data, _ := os.ReadFile(planPath)
	if strings.Contains(string(data), "**Source:** none") {
		t.Errorf("--fix=adherence-footer must not add **Source:** none:\n%s", data)
	}
}
