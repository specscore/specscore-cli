package lint

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func setupGitRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for relPath, content := range files {
		fullPath := filepath.Join(root, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Init git repo and commit
	cmds := [][]string{
		{"git", "init"},
		{"git", "add", "-A"},
		{"git", "commit", "-m", "initial"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git command %v failed: %v\n%s", args, err, out)
		}
	}
	return root
}

func TestImmutabilityFrozenSectionModified(t *testing.T) {
	original := acceptedDecisionContent()

	root := setupGitRepo(t, map[string]string{
		"decisions/0001-test.md": original,
	})

	// Modify a frozen section
	modified := strings.Replace(original, "Some context.", "Modified context.", 1)
	_ = os.WriteFile(filepath.Join(root, "decisions/0001-test.md"), []byte(modified), 0o644)

	vs, err := checkDecisionImmutability(root)
	if err != nil {
		t.Fatal(err)
	}
	if !hasDecisionViolation(vs, "D-immutability-once-accepted", "Context") {
		t.Error("expected D-immutability-once-accepted violation for modified Context section")
	}
}

func TestImmutabilityTitleModified(t *testing.T) {
	original := acceptedDecisionContent()

	root := setupGitRepo(t, map[string]string{
		"decisions/0001-test.md": original,
	})

	modified := strings.Replace(original, "# Decision: Accepted Decision", "# Decision: Changed Title", 1)
	_ = os.WriteFile(filepath.Join(root, "decisions/0001-test.md"), []byte(modified), 0o644)

	vs, err := checkDecisionImmutability(root)
	if err != nil {
		t.Fatal(err)
	}
	if !hasDecisionViolation(vs, "D-immutability-once-accepted", "title changed") {
		t.Error("expected D-immutability-once-accepted violation for title change")
	}
}

// Regression: a frontmatter-carrying Accepted decision (post artifact-
// frontmatter-convention migration) must not falsely trip immutability. The
// committed version read via `git show HEAD:` also carries the frontmatter, so
// the shared parser must skip the leading `---` block to read the real title —
// otherwise it reads "" and reports a spurious "title changed from ”".
func TestImmutabilityFrontmatterTolerated(t *testing.T) {
	fm := "---\nformat: https://specscore.md/decision-specification\nstatus: Accepted\n---\n\n" + acceptedDecisionContent()
	root := setupGitRepo(t, map[string]string{
		"decisions/0001-test.md": fm,
	})
	// Committed == working tree (both carry the frontmatter); nothing changed.
	vs, err := checkDecisionImmutability(root)
	if err != nil {
		t.Fatal(err)
	}
	if hasDecisionViolation(vs, "D-immutability-once-accepted", "title changed") {
		t.Errorf("frontmatter-carrying committed decision falsely flagged: %+v", vs)
	}
}

func TestImmutabilityFieldModified(t *testing.T) {
	original := acceptedDecisionContent()

	root := setupGitRepo(t, map[string]string{
		"decisions/0001-test.md": original,
	})

	modified := strings.Replace(original, "**Owner:** test@example.com", "**Owner:** other@example.com", 1)
	_ = os.WriteFile(filepath.Join(root, "decisions/0001-test.md"), []byte(modified), 0o644)

	vs, err := checkDecisionImmutability(root)
	if err != nil {
		t.Fatal(err)
	}
	if !hasDecisionViolation(vs, "D-immutability-once-accepted", "Owner") {
		t.Error("expected D-immutability-once-accepted violation for Owner field change")
	}
}

func TestImmutabilityStatusChangeAllowed(t *testing.T) {
	original := acceptedDecisionContent()

	root := setupGitRepo(t, map[string]string{
		"decisions/0001-test.md": original,
	})

	// Change Status and Superseded By — these are allowed
	modified := strings.Replace(original, "**Status:** Accepted", "**Status:** Superseded", 1)
	modified = strings.Replace(modified, "**Superseded By:** —", "**Superseded By:** 0002-new", 1)
	_ = os.WriteFile(filepath.Join(root, "decisions/0001-test.md"), []byte(modified), 0o644)

	vs, err := checkDecisionImmutability(root)
	if err != nil {
		t.Fatal(err)
	}
	// Should NOT flag Status or Superseded By changes.
	// But the status changed FROM Accepted, so the immutability check
	// skips it (committed version has Accepted, but current has Superseded —
	// however, we only check files where current status == Accepted).
	// Actually, the checker only checks current status == Accepted,
	// so a transition to Superseded would skip the check entirely.
	for _, v := range vs {
		if v.Rule == "D-immutability-once-accepted" && strings.Contains(v.Message, "Status") {
			t.Error("Status changes should be allowed for supersession")
		}
	}
}

func TestImmutabilityObservedConsequencesAppendOnly(t *testing.T) {
	t.Run("append allowed", func(t *testing.T) {
		original := acceptedDecisionContent()

		root := setupGitRepo(t, map[string]string{
			"decisions/0001-test.md": original,
		})

		modified := strings.Replace(original, "None observed yet.", "None observed yet.\n\n2026-05-27 — Something was observed.", 1)
		_ = os.WriteFile(filepath.Join(root, "decisions/0001-test.md"), []byte(modified), 0o644)

		vs, err := checkDecisionImmutability(root)
		if err != nil {
			t.Fatal(err)
		}
		// The committed version has "None observed yet." — replacing with real
		// entries is allowed (the placeholder removal is handled).
		if hasDecisionViolation(vs, "D-observed-consequences-append-only", "") {
			t.Error("appending to Observed Consequences should be allowed")
		}
	})

	t.Run("modification rejected", func(t *testing.T) {
		content := `# Decision: Test

**Status:** Accepted
**Date:** 2026-05-20
**Owner:** test@example.com
**Tags:** —
**Source Idea:** —
**Supersedes:** —
**Superseded By:** —

## Context

Some context.

## Decision

We chose A.

## Rationale

Good reasons.

## Declined Alternatives

### Option B

Not good enough.

## Consequences at Decision Time

Expected positive outcomes.

## Observed Consequences

2026-05-22 — First observation.
2026-05-23 — Second observation.

## Affected Features

None at this time.

---
*This document follows the https://specscore.md/decision-specification*
`
		root := setupGitRepo(t, map[string]string{
			"decisions/0001-test.md": content,
		})

		// Modify an existing observation
		modified := strings.Replace(content, "2026-05-22 — First observation.", "2026-05-22 — Modified first observation.", 1)
		_ = os.WriteFile(filepath.Join(root, "decisions/0001-test.md"), []byte(modified), 0o644)

		vs, err := checkDecisionImmutability(root)
		if err != nil {
			t.Fatal(err)
		}
		if !hasDecisionViolation(vs, "D-observed-consequences-append-only", "modified") {
			t.Error("expected D-observed-consequences-append-only violation for modified entry")
		}
	})

	t.Run("removal rejected", func(t *testing.T) {
		content := `# Decision: Test

**Status:** Accepted
**Date:** 2026-05-20
**Owner:** test@example.com
**Tags:** —
**Source Idea:** —
**Supersedes:** —
**Superseded By:** —

## Context

Some context.

## Decision

We chose A.

## Rationale

Good reasons.

## Declined Alternatives

### Option B

Not good enough.

## Consequences at Decision Time

Expected positive outcomes.

## Observed Consequences

2026-05-22 — First observation.
2026-05-23 — Second observation.

## Affected Features

None at this time.

---
*This document follows the https://specscore.md/decision-specification*
`
		root := setupGitRepo(t, map[string]string{
			"decisions/0001-test.md": content,
		})

		// Remove an observation
		modified := strings.Replace(content, "2026-05-22 — First observation.\n", "", 1)
		_ = os.WriteFile(filepath.Join(root, "decisions/0001-test.md"), []byte(modified), 0o644)

		vs, err := checkDecisionImmutability(root)
		if err != nil {
			t.Fatal(err)
		}
		if !hasDecisionViolation(vs, "D-observed-consequences-append-only", "") {
			t.Error("expected D-observed-consequences-append-only violation for removed entry")
		}
	})
}

func TestImmutabilityProposedNotChecked(t *testing.T) {
	root := setupGitRepo(t, map[string]string{
		"decisions/0001-test.md": validDecisionContent(),
	})

	// Modify the Proposed decision — should not trigger immutability
	modified := strings.Replace(validDecisionContent(), "Some context here.", "Completely rewritten context.", 1)
	_ = os.WriteFile(filepath.Join(root, "decisions/0001-test.md"), []byte(modified), 0o644)

	vs, err := checkDecisionImmutability(root)
	if err != nil {
		t.Fatal(err)
	}
	if hasDecisionViolation(vs, "D-immutability-once-accepted", "") {
		t.Error("Proposed decisions should not be checked for immutability")
	}
}

func TestImmutabilityNewFileNotChecked(t *testing.T) {
	// File not yet committed — should not be checked
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "decisions"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "decisions/0001-test.md"), []byte(acceptedDecisionContent()), 0o644)

	// Init git but don't commit
	cmd := exec.Command("git", "init")
	cmd.Dir = root
	_ = cmd.Run()

	vs, err := checkDecisionImmutability(root)
	if err != nil {
		t.Fatal(err)
	}
	if hasDecisionViolation(vs, "D-immutability-once-accepted", "") {
		t.Error("new uncommitted files should not be checked for immutability")
	}
}

func TestImmutabilityUnchangedPasses(t *testing.T) {
	root := setupGitRepo(t, map[string]string{
		"decisions/0001-test.md": acceptedDecisionContent(),
	})

	// Don't modify anything
	vs, err := checkDecisionImmutability(root)
	if err != nil {
		t.Fatal(err)
	}
	if hasDecisionViolation(vs, "D-immutability-once-accepted", "") {
		t.Error("unchanged Accepted decision should not have violations")
	}
	if hasDecisionViolation(vs, "D-observed-consequences-append-only", "") {
		t.Error("unchanged Observed Consequences should not have violations")
	}
}
