package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/lint"
)

func initPlanStoreRepo(t *testing.T, root, remote string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-b", "main"}, {"config", "user.email", "test@example.com"}, {"config", "user.name", "Test"}, {"remote", "add", "origin", remote}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func TestExternalPlanMutationChangesOnlyPlanStoreAndKeepsPrerequisiteGuard(t *testing.T) {
	base := t.TempDir()
	source, store := filepath.Join(base, "source"), filepath.Join(base, "store")
	initPlanStoreRepo(t, source, "git@github.com:acme/product.git")
	initPlanStoreRepo(t, store, "git@github.com:acme/workbench.git")
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(source, "specscore.yaml"), "project:\n  host: github.com\n  org: acme\n  repo: product\nplans_repo: acme/workbench\n")
	write(filepath.Join(source, "specscore.local.yaml"), "repo_checkouts:\n  acme/workbench: "+store+"\n")
	// Deliberately fixable source drift proves a Plan-only mutation never runs
	// broad source fixers.
	write(filepath.Join(source, "spec", "features", "drift", "README.md"), "# Feature: Drift\n\n**Status:** Draft\n")
	ns := filepath.Join(store, "spec", "plans", "github.com", "acme", "product")
	index := "# Plans\n\n| Plan | Status | Source | Date | Owner |\n|------|--------|--------|------|-------|\n"
	write(filepath.Join(ns, "README.md"), index)
	planBody := func(title, status, prerequisite, taskStatus string) string {
		return "---\nformat: https://specscore.md/plan-specification\nstatus: " + status + "\n---\n\n# Plan: " + title + "\n\n**Status:** " + status + "\n**Source:** none\n**Prerequisite Plans:** " + prerequisite + "\n\n## Tasks\n\n### Task 1: Execute\n\n**Id:** execute\n**Verifies:** —\n**Status:** " + taskStatus + "\n\n## Open Questions\n\nNone.\n\n---\n*This document follows the https://specscore.md/plan-specification*\n"
	}
	write(filepath.Join(ns, "ready", "README.md"), planBody("Ready", "Approved", "—", "queued"))
	write(filepath.Join(ns, "prereq", "README.md"), planBody("Prereq", "Draft", "—", "planning"))
	write(filepath.Join(ns, "blocked", "README.md"), planBody("Blocked", "Draft", "prereq", "queued"))
	write(filepath.Join(ns, "preview", "README.md"), planBody("Preview", "Draft", "—", "planning"))

	sourceBefore, err := os.ReadFile(filepath.Join(source, "spec", "features", "drift", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	originalLint := lintLintFn
	lintLintFn = func(opts lint.Options) ([]lint.Violation, error) {
		if opts.Fix {
			if strings.Join(opts.Rules, ",") != "P-007,plan-index-sync" {
				t.Fatalf("Plan mutation fix rules = %v", opts.Rules)
			}
			return lint.Lint(opts)
		}
		return nil, nil
	}
	t.Cleanup(func() { lintLintFn = originalLint })
	previewPath := filepath.Join(ns, "preview", "README.md")
	previewBefore, _ := os.ReadFile(previewPath)
	if _, _, err := runPlan(t, "change-status", "preview", "--to", "Approved", "--dry-run", "--project", source); err == nil {
		t.Fatal("invalid source fixture should fail preflight")
	}
	previewAfter, _ := os.ReadFile(previewPath)
	if string(previewAfter) != string(previewBefore) {
		t.Fatal("dry-run mutated destination Plan")
	}
	if _, err := os.Stat(filepath.Join(store, ".specscore-lifecycle.lock")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created real destination lock: %v", err)
	}
	driftAfterPreview, _ := os.ReadFile(filepath.Join(source, "spec", "features", "drift", "README.md"))
	if string(driftAfterPreview) != string(sourceBefore) {
		t.Fatal("dry-run mutated source Feature tree")
	}

	if _, _, err := runTask(t, "change-status", "execute", "--plan", "ready", "--to", "in_progress", "--project", source); err != nil {
		t.Fatal(err)
	}
	ready, err := os.ReadFile(filepath.Join(ns, "ready", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ready), "**Status:** Executing") || !strings.Contains(string(ready), "**Status:** in_progress") {
		t.Fatalf("derived Plan/task status not written:\n%s", ready)
	}
	sourceAfter, err := os.ReadFile(filepath.Join(source, "spec", "features", "drift", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(sourceAfter) != string(sourceBefore) {
		t.Fatal("Plan mutation changed source Feature tree")
	}

	blockedPath := filepath.Join(ns, "blocked", "README.md")
	blockedBefore, _ := os.ReadFile(blockedPath)
	if _, _, err := runTask(t, "change-status", "execute", "--plan", "blocked", "--to", "in_progress", "--project", source); err == nil || !strings.Contains(err.Error(), "unmet prerequisite") {
		t.Fatalf("prerequisite guard error = %v", err)
	}
	blockedAfter, _ := os.ReadFile(blockedPath)
	if string(blockedAfter) != string(blockedBefore) {
		t.Fatal("prerequisite refusal mutated destination Plan")
	}
	if _, err := os.Stat(filepath.Join(source, "spec", "plans")); !os.IsNotExist(err) {
		t.Fatalf("source gained local plans path: %v", err)
	}
}
