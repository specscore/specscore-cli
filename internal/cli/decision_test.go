package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/decision"
	"github.com/specscore/specscore-cli/pkg/lint"
)

// setupDecisionRoot stages a temp spec repo with the minimal layout the
// `decision new` verb expects: a `specscore.yaml` anchor at the root and
// pre-populated decisions/active/archived index files so the post-scaffold
// `specscore spec lint --fix` is lint-clean.
func setupDecisionRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	specDir := filepath.Join(root, "spec")
	if err := os.MkdirAll(filepath.Join(specDir, "features"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(specDir, "decisions", "archived"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "specscore.yaml"), []byte("name: test\n"), 0o644); err != nil {
		t.Fatalf("write specscore.yaml: %v", err)
	}
	// Lint-clean active index.
	activeIdx := `# Decisions

## Decisions

| # | Decision | Status | Date | Tags | Affected |
|---|----------|--------|------|------|----------|

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/decisions-index-specification*
`
	if err := os.WriteFile(filepath.Join(specDir, "decisions", "README.md"), []byte(activeIdx), 0o644); err != nil {
		t.Fatalf("write active index: %v", err)
	}
	// Lint-clean archived index.
	archivedIdx := `# Archived Decisions

_No archived decisions yet._

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/decisions-index-specification*
`
	if err := os.WriteFile(filepath.Join(specDir, "decisions", "archived", "README.md"), []byte(archivedIdx), 0o644); err != nil {
		t.Fatalf("write archived index: %v", err)
	}
	migrateTree(t, root)
	return root
}

// runDecision invokes the decision cobra command tree in-process.
func runDecision(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := decisionCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

// =============================================================================
// runDecisionNew happy path — bare invocation produces a lint-clean Decision.
// =============================================================================

func TestDecisionNew_BareInvocation(t *testing.T) {
	root := setupDecisionRoot(t)
	withCwd(t, root)

	out, _, err := runDecision(t, "new", "demo")
	if err != nil {
		t.Fatalf("decision new failed: %v", err)
	}
	// Stdout should print the path of the created file.
	if !strings.Contains(out, "0001-demo.md") {
		t.Errorf("expected stdout to mention 0001-demo.md; got %q", out)
	}
	// File must exist.
	target := filepath.Join(root, "spec", "decisions", "0001-demo.md")
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected scaffolded file at %s; got %v", target, err)
	}
}

// =============================================================================
// runDecisionNew with flags — flags must be propagated to the scaffolded file.
// =============================================================================

func TestDecisionNew_FlagsInjected(t *testing.T) {
	root := setupDecisionRoot(t)
	withCwd(t, root)

	// Pre-create an idea so source-idea passes lint.
	ideasDir := filepath.Join(root, "spec", "ideas")
	if err := os.MkdirAll(ideasDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ideasDir, "my-idea.md"), []byte("# Idea: My Idea\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := runDecision(t, "new", "demo-flag",
		"--title", "Demo Flag",
		"--owner", "alice",
		"--source-idea", "my-idea",
		"--tags", "alpha, beta",
	)
	if err != nil {
		t.Fatalf("decision new failed: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, "spec", "decisions", "0001-demo-flag.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, want := range []string{
		"# Decision: Demo Flag",
		"**Owner:** alice",
		"**Source Idea:** my-idea",
		"**Tags:** alpha, beta",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("expected %q in scaffolded body; got:\n%s", want, s)
		}
	}
}

// =============================================================================
// Owner defaults to $USER when --owner is omitted.
// =============================================================================

func TestDecisionNew_OwnerDefaultsToUSER(t *testing.T) {
	root := setupDecisionRoot(t)
	withCwd(t, root)

	t.Setenv("USER", "from-env")
	_, _, err := runDecision(t, "new", "user-owner")
	if err != nil {
		t.Fatalf("decision new failed: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, "spec", "decisions", "0001-user-owner.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "**Owner:** from-env") {
		t.Errorf("expected Owner injected from $USER; got:\n%s", body)
	}
}

// =============================================================================
// Invalid slug — error path on line 50-52.
// =============================================================================

func TestDecisionNew_InvalidSlug(t *testing.T) {
	root := setupDecisionRoot(t)
	withCwd(t, root)

	_, _, err := runDecision(t, "new", "Bad-Slug-Camel")
	if err == nil {
		t.Fatal("expected error for invalid slug")
	}
	if got := exitCodeOfErr(err); got != 2 {
		t.Errorf("expected exit code 2 (InvalidArgs), got %d", got)
	}
}

// =============================================================================
// `--project` flag pointing at a missing dir — resolveSpecRoot error
// =============================================================================

func TestDecisionNew_ProjectFlagInvalid(t *testing.T) {
	// No setup — cwd is whatever the test runner is in. Use --project to
	// drive resolveSpecRoot down the failing path.
	_, _, err := runDecision(t, "new", "demo", "--project", "/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error when --project doesn't exist")
	}
}

// =============================================================================
// Pre-existing file without --force triggers ConflictError.
// =============================================================================

func TestDecisionNew_ConflictWithoutForce(t *testing.T) {
	root := setupDecisionRoot(t)
	withCwd(t, root)

	// Pin NextNumber to 1 so target = "0001-conflict.md", then pre-place
	// that file so the os.Stat in the conflict-check returns nil.
	// Production code never produces this state through normal use
	// (NextNumber would skip the pre-placed file's number), so we use the
	// seam to drive the conflict-check branch deterministically.
	orig := decisionNextNumberFn
	decisionNextNumberFn = func(_ string) (int, error) { return 1, nil }
	t.Cleanup(func() { decisionNextNumberFn = orig })

	target := filepath.Join(root, "spec", "decisions", "0001-conflict.md")
	if err := os.WriteFile(target, []byte(validDecisionContent_DecisionCLI()), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := runDecision(t, "new", "conflict")
	if err == nil {
		t.Fatal("expected ConflictError for pre-existing file")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' in error message; got %q", err.Error())
	}
}

// validDecisionContent_DecisionCLI returns the same shape as the pkg/lint
// helper but inlined here because internal/cli tests cannot import test-only
// helpers from another package.
func validDecisionContent_DecisionCLI() string {
	return `# Decision: Stub

**Status:** Proposed
**Date:** 2026-05-26
**Owner:** test@example.com
**Tags:** —
**Source Idea:** —
**Supersedes:** —
**Superseded By:** —

## Context

C.

## Decision

D.

## Rationale

R.

## Declined Alternatives

### Alt

No.

## Consequences at Decision Time

C.

## Observed Consequences

None observed yet.

## Affected Features

None at this time.

---
*This document follows the https://specscore.md/decision-specification*
`
}

// =============================================================================
// Pre-existing file WITH --force overwrites silently.
// =============================================================================

func TestDecisionNew_ForceOverwrites(t *testing.T) {
	root := setupDecisionRoot(t)
	withCwd(t, root)

	// Same seam pattern as the conflict test — pin NextNumber to 1 and
	// pre-place 0001-force.md. With --force the conflict-check must be
	// bypassed and the file overwritten.
	orig := decisionNextNumberFn
	decisionNextNumberFn = func(_ string) (int, error) { return 1, nil }
	t.Cleanup(func() { decisionNextNumberFn = orig })

	target := filepath.Join(root, "spec", "decisions", "0001-force.md")
	if err := os.WriteFile(target, []byte(validDecisionContent_DecisionCLI()), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := runDecision(t, "new", "force", "--force", "--title", "Force Override")
	if err != nil {
		t.Fatalf("decision new --force failed: %v", err)
	}
	body, _ := os.ReadFile(target)
	if !strings.Contains(string(body), "# Decision: Force Override") {
		t.Errorf("expected overwrite to scaffold new content; got:\n%s", body)
	}
}

// =============================================================================
// decisionScaffoldFn returns an error — error path on line 102-104.
// =============================================================================

func TestDecisionNew_ScaffoldFnError(t *testing.T) {
	root := setupDecisionRoot(t)
	withCwd(t, root)

	orig := decisionScaffoldFn
	decisionScaffoldFn = func(_ decision.ScaffoldOptions) ([]byte, error) {
		return nil, errors.New("synthetic scaffold error")
	}
	t.Cleanup(func() { decisionScaffoldFn = orig })

	_, _, err := runDecision(t, "new", "demo")
	if err == nil {
		t.Fatal("expected scaffolding error to surface")
	}
	if !strings.Contains(err.Error(), "scaffolding decision") {
		t.Errorf("expected wrapped scaffolding error; got %q", err.Error())
	}
}

// =============================================================================
// lintLintFn first call (Fix=true) fails — error path on line 110-113.
// =============================================================================

func TestDecisionNew_LintFixFailureRemovesTarget(t *testing.T) {
	root := setupDecisionRoot(t)
	withCwd(t, root)

	orig := lintLintFn
	lintLintFn = func(opts lint.Options) ([]lint.Violation, error) {
		return nil, errors.New("synthetic lint fix failure")
	}
	t.Cleanup(func() { lintLintFn = orig })

	_, _, err := runDecision(t, "new", "demo")
	if err == nil {
		t.Fatal("expected lint-fix error to surface")
	}
	if !strings.Contains(err.Error(), "lint fix") {
		t.Errorf("expected wrapped lint-fix error; got %q", err.Error())
	}
	// The scaffolded file must have been rolled back.
	target := filepath.Join(root, "spec", "decisions", "0001-demo.md")
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected target to be removed on lint-fix failure; got err=%v", err)
	}
}

// =============================================================================
// lintLintFn second call (verification) fails — error path on line 114-117.
// =============================================================================

func TestDecisionNew_LintVerifyFailure(t *testing.T) {
	root := setupDecisionRoot(t)
	withCwd(t, root)

	orig := lintLintFn
	call := 0
	lintLintFn = func(opts lint.Options) ([]lint.Violation, error) {
		call++
		if call == 1 {
			// First call: --fix succeeds.
			return nil, nil
		}
		return nil, errors.New("synthetic verify failure")
	}
	t.Cleanup(func() { lintLintFn = orig })

	_, _, err := runDecision(t, "new", "demo-verify")
	if err == nil {
		t.Fatal("expected lint-verify error to surface")
	}
	if !strings.Contains(err.Error(), "running lint") {
		t.Errorf("expected wrapped lint error; got %q", err.Error())
	}
}

// =============================================================================
// Generated file fails lint (error violations) — error path on lines 125-132.
// =============================================================================

func TestDecisionNew_GeneratedFileFailsLint(t *testing.T) {
	root := setupDecisionRoot(t)
	withCwd(t, root)

	orig := lintLintFn
	call := 0
	lintLintFn = func(opts lint.Options) ([]lint.Violation, error) {
		call++
		if call == 1 {
			return nil, nil
		}
		// Second call (verify): synthesize a fatal violation against the
		// scaffolded file.
		return []lint.Violation{
			{
				File:     "decisions/0001-lint-fail.md",
				Line:     7,
				Severity: "error",
				Rule:     "D-some-rule",
				Message:  "synthetic violation",
			},
		}, nil
	}
	t.Cleanup(func() { lintLintFn = orig })

	_, _, err := runDecision(t, "new", "lint-fail")
	if err == nil {
		t.Fatal("expected lint-violation error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "generated decision failed lint") {
		t.Errorf("expected 'generated decision failed lint' prefix; got %q", msg)
	}
	if !strings.Contains(msg, "D-some-rule") {
		t.Errorf("expected the violation rule to be included; got %q", msg)
	}
}

// =============================================================================
// runDecisionNew NextNumber error path — line 76-78.
// =============================================================================
//
// NextNumber never returns a non-nil error in production (pkg/decision
// swallows os.ReadDir errors). The branch is defensive. We drive it via
// the seam to verify the wrapping behavior.

func TestDecisionNew_NextNumberError(t *testing.T) {
	root := setupDecisionRoot(t)
	withCwd(t, root)

	orig := decisionNextNumberFn
	decisionNextNumberFn = func(_ string) (int, error) {
		return 0, errors.New("synthetic next-number failure")
	}
	t.Cleanup(func() { decisionNextNumberFn = orig })

	_, _, err := runDecision(t, "new", "demo")
	if err == nil {
		t.Fatal("expected NextNumber error to surface")
	}
	if !strings.Contains(err.Error(), "determining next number") {
		t.Errorf("expected 'determining next number'; got %q", err.Error())
	}
}

// =============================================================================
// runDecisionNew MkdirAll error path — line 82-84.
// =============================================================================
//
// Stage a spec tree where `spec/decisions` is a regular file (not a dir).
// MkdirAll on it returns ENOTDIR.

func TestDecisionNew_MkdirAllError(t *testing.T) {
	root := t.TempDir()
	specDir := filepath.Join(root, "spec")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "specscore.yaml"), []byte("name: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Block `spec/decisions` by making it a file.
	if err := os.WriteFile(filepath.Join(specDir, "decisions"), []byte("blocker"), 0o644); err != nil {
		t.Fatal(err)
	}
	withCwd(t, root)

	_, _, err := runDecision(t, "new", "demo")
	if err == nil {
		t.Fatal("expected MkdirAll error")
	}
	if !strings.Contains(err.Error(), "creating") {
		t.Errorf("expected 'creating' in error; got %q", err.Error())
	}
}

// =============================================================================
// runDecisionNew os.WriteFile error path — line 105-107.
// =============================================================================
//
// Make `spec/decisions/0001-write-blocked.md` a directory so WriteFile
// fails with "is a directory".

func TestDecisionNew_WriteFileError(t *testing.T) {
	root := setupDecisionRoot(t)
	withCwd(t, root)

	// Create a directory at the target file path.
	blocker := filepath.Join(root, "spec", "decisions", "0001-blocked.md")
	if err := os.MkdirAll(blocker, 0o755); err != nil {
		t.Fatal(err)
	}

	_, _, err := runDecision(t, "new", "blocked", "--force")
	if err == nil {
		t.Fatal("expected WriteFile error")
	}
	if !strings.Contains(err.Error(), "writing") {
		t.Errorf("expected 'writing' in error; got %q", err.Error())
	}
}

// =============================================================================
// runDecisionNew NextNumber error path via seam — line 76-78.
// =============================================================================
//
// We can't trigger NextNumber error through pkg/decision alone (it
// swallows all errors). We do NOT introduce a CLI seam for the scaffold
// helper's internals — instead we note the branch is defensive and
// effectively unreachable in production. Verify the happy path returns
// nextNum > 0 transitively via the BareInvocation test above.

// =============================================================================
// runDecisionNew incremental numbering — second decision gets 0002.
// =============================================================================

func TestDecisionNew_IncrementsNumber(t *testing.T) {
	root := setupDecisionRoot(t)
	withCwd(t, root)

	if _, _, err := runDecision(t, "new", "first"); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, _, err := runDecision(t, "new", "second"); err != nil {
		t.Fatalf("second: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "spec", "decisions", "0002-second.md")); err != nil {
		t.Errorf("expected 0002-second.md; got %v", err)
	}
}

// =============================================================================
// runDecisionNew with --supersedes — flag is propagated even though we
// don't validate the target here.
// =============================================================================

func TestDecisionNew_SupersedesFlag(t *testing.T) {
	root := setupDecisionRoot(t)
	withCwd(t, root)

	// We need an existing archived decision as the supersession target,
	// otherwise the post-scaffold lint will reject the new file.
	archivedTarget := filepath.Join(root, "spec", "decisions", "archived", "0001-old.md")
	old := `# Decision: Old

**Status:** Superseded
**Date:** 2026-05-20
**Owner:** test
**Tags:** —
**Source Idea:** —
**Supersedes:** —
**Superseded By:** 0002-new

## Context

C.

## Decision

D.

## Rationale

R.

## Declined Alternatives

### Alt

No.

## Consequences at Decision Time

C.

## Observed Consequences

None observed yet.

## Affected Features

None at this time.

---
*This document follows the https://specscore.md/decision-specification*
`
	if err := os.WriteFile(archivedTarget, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	archivedIndexPath := filepath.Join(root, "spec", "decisions", "archived", "README.md")
	archivedIndex, err := os.ReadFile(archivedIndexPath)
	if err != nil {
		t.Fatal(err)
	}
	archivedRow := "- 2026-05-20 — [0001-old](0001-old.md) — Superseded — superseded by 0002-new"
	updatedArchivedIndex := string(archivedIndex)
	switch {
	case strings.Contains(updatedArchivedIndex, "_No archived decisions yet._"):
		updatedArchivedIndex = strings.Replace(updatedArchivedIndex, "_No archived decisions yet._", archivedRow, 1)
	case strings.Contains(updatedArchivedIndex, archivedRow):
		// already present
	case strings.Contains(updatedArchivedIndex, "## Open Questions"):
		updatedArchivedIndex = strings.Replace(updatedArchivedIndex, "## Open Questions", archivedRow+"\n\n## Open Questions", 1)
	default:
		updatedArchivedIndex = strings.TrimRight(updatedArchivedIndex, "\n") + "\n\n" + archivedRow + "\n"
	}
	if err := os.WriteFile(archivedIndexPath, []byte(updatedArchivedIndex), 0o644); err != nil {
		t.Fatal(err)
	}

	migrateTree(t, root)
	_, _, err = runDecision(t, "new", "new", "--supersedes", "0001-old")
	if err != nil {
		t.Fatalf("decision new --supersedes failed: %v", err)
	}
	body, _ := os.ReadFile(filepath.Join(root, "spec", "decisions", "0002-new.md"))
	if !strings.Contains(string(body), "**Supersedes:** 0001-old") {
		t.Errorf("expected --supersedes injection; got:\n%s", body)
	}
}

// Compile-time sanity: ensure the helper signature we depend on is
// stable. (No runtime assertion.)
var _ = fmt.Sprintf
