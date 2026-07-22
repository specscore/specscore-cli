package feature

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

// writeFeatureFixture builds a temp features tree at <tmp>/spec/features
// with one feature `auth` at the given status. Returns featuresDir.
//
// This helper is the local equivalent of internal/cli/idea_test.go's
// setupSpecRoot for the feature kind, but trimmed to just what
// ChangeStatus needs (no `spec/README.md`, no features-index — lint is
// not invoked from this package's tests). The CLI-level integration
// tests in internal/cli/feature_test.go cover the lint dance.
func writeFeatureFixture(t *testing.T, status string) string {
	t.Helper()
	root := t.TempDir()
	featDir := filepath.Join(root, "spec", "features")
	if err := os.MkdirAll(filepath.Join(featDir, "auth"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := "# Feature: Auth\n\n**Status:** " + status + "\n\n## Summary\n\nPlaceholder.\n"
	if err := os.WriteFile(filepath.Join(featDir, "auth", "README.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return featDir
}

// readStatusLine returns the value of the **Status:** line in path, or
// "" if no such line is present. Used by tests that need to assert the
// rewrite landed correctly without round-tripping through the lifecycle
// package's internal parser.
func readStatusLine(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "**Status:**") {
			rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "**Status:**"))
			return rest
		}
	}
	return ""
}

// exitCodeOf returns the ExitCode() of err if err is a typed
// exitcode.Error; otherwise it returns -1. Centralizes the
// assertion pattern across the table-driven tests.
func exitCodeOf(err error) int {
	var ec *exitcode.Error
	if errors.As(err, &ec) {
		return ec.ExitCode()
	}
	return -1
}

// TestResolveFeatureID exercises the slug-resolution helper that
// underpins both the happy path (`auth` and `cli/idea/change-status`
// both work) and the exit-3 path (missing README).
func TestResolveFeatureID(t *testing.T) {
	root := t.TempDir()
	featDir := filepath.Join(root, "spec", "features")
	if err := os.MkdirAll(filepath.Join(featDir, "auth"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(featDir, "auth", "README.md"), []byte("# Auth\n**Status:** Draft\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Nested feature: cli/idea/change-status
	nested := filepath.Join(featDir, "cli", "idea", "change-status")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nested, "README.md"), []byte("# Nested\n**Status:** Draft\n"), 0o644); err != nil {
		t.Fatalf("write nested: %v", err)
	}

	t.Run("top-level resolves", func(t *testing.T) {
		got, err := resolveFeatureID(featDir, "auth")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		want := filepath.Join(featDir, "auth", "README.md")
		if got != want {
			t.Errorf("got %q want %q", got, want)
		}
	})

	t.Run("nested slash-bearing resolves", func(t *testing.T) {
		got, err := resolveFeatureID(featDir, "cli/idea/change-status")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		want := filepath.Join(featDir, "cli", "idea", "change-status", "README.md")
		if got != want {
			t.Errorf("got %q want %q", got, want)
		}
	})

	t.Run("missing exits NotFound", func(t *testing.T) {
		_, err := resolveFeatureID(featDir, "nonexistent")
		if err == nil {
			t.Fatal("expected error")
		}
		if got := exitCodeOf(err); got != exitcode.NotFound {
			t.Errorf("exit code = %d, want %d (NotFound)", got, exitcode.NotFound)
		}
	})

	t.Run("empty id exits InvalidArgs", func(t *testing.T) {
		_, err := resolveFeatureID(featDir, "")
		if err == nil {
			t.Fatal("expected error")
		}
		if got := exitCodeOf(err); got != exitcode.InvalidArgs {
			t.Errorf("exit code = %d, want %d (InvalidArgs)", got, exitcode.InvalidArgs)
		}
	})
}

// TestChangeStatus_HappyPaths covers every legal arc in the Feature
// matrix. Each sub-test seeds a fresh fixture so a transition in one
// arc can't leak into the next.
//
// Matches ACs:
//   - draft-to-under-review-happy-path
//   - draft-direct-to-approved-happy-path
//   - under-review-to-approved-happy-path
//   - implementing-to-stable-happy-path
//   - stable-to-deprecated-happy-path
//   - draft-to-deprecated-happy-path
//   - implementing-to-deprecated-happy-path
//   - approved-to-implementing-happy-path (not numbered in AC list but
//     part of the matrix — guarded so the matrix stays complete).
func TestChangeStatus_HappyPaths(t *testing.T) {
	cases := []struct {
		name string
		from string
		to   string
		want string
	}{
		{"draft → in review", "Draft", "in review", "In Review"},
		{"draft → approved (direct)", "Draft", "approved", "Approved"},
		{"in review → approved", "In Review", "approved", "Approved"},
		{"in review → rejected", "In Review", "rejected", "Rejected"},
		{"approved → implementing", "Approved", "implementing", "Implementing"},
		{"implementing → stable", "Implementing", "stable", "Stable"},
		{"stable → amending", "Stable", "amending", "Amending"},
		{"amending → stable", "Amending", "stable", "Stable"},
		{"stable → deprecated", "Stable", "deprecated", "Deprecated"},
		{"draft → deprecated", "Draft", "deprecated", "Deprecated"},
		{"implementing → deprecated", "Implementing", "deprecated", "Deprecated"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			featDir := writeFeatureFixture(t, tc.from)
			result, err := ChangeStatus(featDir, "auth", tc.to)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if string(result.From) != tc.from {
				t.Errorf("From = %q, want %q", result.From, tc.from)
			}
			if string(result.To) != tc.want {
				t.Errorf("To = %q, want %q", result.To, tc.want)
			}
			if got := readStatusLine(t, result.ReadmePath); got != tc.want {
				t.Errorf("on-disk Status = %q, want %q", got, tc.want)
			}
		})
	}
}

// AC: nested-feature-id-resolves — `cli/idea/change-status` resolves
// through the nested directory path.
func TestChangeStatus_NestedFeatureID(t *testing.T) {
	root := t.TempDir()
	featDir := filepath.Join(root, "spec", "features")
	nested := filepath.Join(featDir, "cli", "idea", "change-status")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	body := "# Feature: Change Status\n\n**Status:** Draft\n\n## Summary\n\nPlaceholder.\n"
	if err := os.WriteFile(filepath.Join(nested, "README.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	result, err := ChangeStatus(featDir, "cli/idea/change-status", "approved")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if string(result.From) != "Draft" || string(result.To) != "Approved" {
		t.Errorf("transition = %q→%q, want Draft→Approved", result.From, result.To)
	}
	if got := readStatusLine(t, result.ReadmePath); got != "Approved" {
		t.Errorf("nested README Status = %q, want Approved", got)
	}
}

// AC: case-insensitive-to-flag — STABLE / Stable / stable all
// produce identical results on an Implementing source.
func TestChangeStatus_CaseInsensitiveFlag(t *testing.T) {
	for _, raw := range []string{"STABLE", "Stable", "stable", "  stable  "} {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			featDir := writeFeatureFixture(t, "Implementing")
			result, err := ChangeStatus(featDir, "auth", raw)
			if err != nil {
				t.Fatalf("unexpected err for %q: %v", raw, err)
			}
			if string(result.To) != "Stable" {
				t.Errorf("To = %q, want Stable (canonical title-case)", result.To)
			}
			if got := readStatusLine(t, result.ReadmePath); got != "Stable" {
				t.Errorf("on-disk Status = %q, want Stable", got)
			}
		})
	}
}

// AC: illegal-transition-rejected (matrix-level) — Draft → Implementing,
// Draft → Stable, Approved → Stable, Stable → Approved all exit 4.
// stderr must name the current status and the legal-target set.
// AC: reverse-transition-rejected — Stable → Implementing exits 4.
// AC: already-at-target-rejected — Approved → Approved exits 4 per
// REQ: not-idempotent (self-loops are forbidden in the matrix).
func TestChangeStatus_IllegalTransitions(t *testing.T) {
	cases := []struct {
		name string
		from string
		to   string
	}{
		{"draft → implementing (skips approved)", "Draft", "implementing"},
		{"draft → stable", "Draft", "stable"},
		{"approved → stable (skips implementing)", "Approved", "stable"},
		{"stable → approved (reverse)", "Stable", "approved"},
		{"stable → implementing (reverse)", "Stable", "implementing"},
		{"deprecated → stable (reverse)", "Deprecated", "stable"},
		{"approved → approved (self-loop)", "Approved", "approved"},
		{"draft → draft (self-loop, also blocked by --to=draft guard)", "Draft", "draft"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			featDir := writeFeatureFixture(t, tc.from)
			result, err := ChangeStatus(featDir, "auth", tc.to)
			if err == nil {
				t.Fatalf("expected error for %s → %s, got result %+v", tc.from, tc.to, result)
			}
			// --to=draft is intercepted at the flag layer (exit-2),
			// not the state-machine layer (exit-4). Both are valid
			// rejections; the matrix-strictness REQ is what we care
			// about — the artifact MUST stay unchanged either way.
			expectedExit := exitcode.InvalidState
			if strings.EqualFold(tc.to, "draft") {
				expectedExit = exitcode.InvalidArgs
			}
			if got := exitCodeOf(err); got != expectedExit {
				t.Errorf("exit code = %d, want %d (err = %v)", got, expectedExit, err)
			}
			// File on disk must be unchanged.
			path := filepath.Join(featDir, "auth", "README.md")
			if got := readStatusLine(t, path); got != tc.from {
				t.Errorf("file mutated unexpectedly: got %q, want %q", got, tc.from)
			}
			if expectedExit == exitcode.InvalidState {
				// The error message must surface both the current
				// status and the legal target set — that is the
				// state-machine-strictness REQ contract.
				msg := err.Error()
				if !strings.Contains(msg, tc.from) {
					t.Errorf("error message %q does not name current status %q", msg, tc.from)
				}
				legal := lifecycle.LegalTargets(lifecycle.KindFeature, lifecycle.Status(tc.from))
				if len(legal) > 0 && !strings.Contains(msg, string(legal[0])) {
					t.Errorf("error message %q does not name any legal target from %q", msg, tc.from)
				}
			}
		})
	}
}

// AC: unrecognized-to-value-rejected — `--to=banana` exits 2 BEFORE
// any state-machine check. Same for `--to=archived` (Idea-only) and
// `--to=draft` (no arc INTO Draft, treated as not a legal target).
func TestChangeStatus_UnrecognizedToValue(t *testing.T) {
	for _, raw := range []string{"banana", "archived", "specified", "DRAFT", "draft"} {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			featDir := writeFeatureFixture(t, "Draft")
			_, err := ChangeStatus(featDir, "auth", raw)
			if err == nil {
				t.Fatalf("expected error for --to=%q", raw)
			}
			if got := exitCodeOf(err); got != exitcode.InvalidArgs {
				t.Errorf("exit code = %d, want %d (err = %v)", got, exitcode.InvalidArgs, err)
			}
			// File must be unchanged — exit-2 occurs before any
			// file mutation.
			path := filepath.Join(featDir, "auth", "README.md")
			if got := readStatusLine(t, path); got != "Draft" {
				t.Errorf("file mutated unexpectedly: got %q, want Draft", got)
			}
		})
	}
}

// AC: feature-not-found — a missing README.md exits 3.
func TestChangeStatus_FeatureNotFound(t *testing.T) {
	root := t.TempDir()
	featDir := filepath.Join(root, "spec", "features")
	if err := os.MkdirAll(featDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	_, err := ChangeStatus(featDir, "nonexistent", "approved")
	if err == nil {
		t.Fatal("expected error")
	}
	if got := exitCodeOf(err); got != exitcode.NotFound {
		t.Errorf("exit code = %d, want %d (NotFound); err = %v", got, exitcode.NotFound, err)
	}
}

// AC: no-status-line — a feature README without a **Status:** line should
// fail with an Unexpected error (line 96).
func TestChangeStatus_NoStatusLine(t *testing.T) {
	root := t.TempDir()
	featDir := filepath.Join(root, "spec", "features")
	authDir := filepath.Join(featDir, "auth")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write a README without a **Status:** line.
	body := "# Feature: Auth\n\n## Summary\n\nNo status here.\n"
	if err := os.WriteFile(filepath.Join(authDir, "README.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ChangeStatus(featDir, "auth", "approved")
	if err == nil {
		t.Fatal("expected error for missing status line")
	}
	if got := exitCodeOf(err); got != exitcode.Unexpected {
		t.Errorf("exit code = %d, want %d (Unexpected)", got, exitcode.Unexpected)
	}
}

// Rewrite error (line 100-102) — read-only directory prevents atomic write.
func TestChangeStatus_RewriteError(t *testing.T) {
	root := t.TempDir()
	featDir := filepath.Join(root, "spec", "features")
	authDir := filepath.Join(featDir, "auth")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "# Feature: Auth\n\n**Status:** Draft\n\n## Summary\n\nPlaceholder.\n"
	if err := os.WriteFile(filepath.Join(authDir, "README.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	// Make the directory read-only so the atomic-write (temp file + rename) fails.
	if err := os.Chmod(authDir, 0o555); err != nil {
		t.Skip("cannot change permissions")
	}
	t.Cleanup(func() { _ = os.Chmod(authDir, 0o755) })

	_, err := ChangeStatus(featDir, "auth", "approved")
	if err == nil {
		t.Fatal("expected error for read-only directory (rewrite failure)")
	}
	if got := exitCodeOf(err); got != exitcode.Unexpected {
		t.Errorf("exit code = %d, want %d (Unexpected)", got, exitcode.Unexpected)
	}
}

// joinStatuses with empty slice returns "".
func TestJoinStatuses_Empty(t *testing.T) {
	got := joinStatuses(nil)
	if got != "" {
		t.Errorf("joinStatuses(nil) = %q, want empty", got)
	}
}

// Restore closure round-trips correctly: after a successful rewrite,
// calling Restore returns the file to its pre-rewrite content.
// Verifies the contract the CLI handler relies on for
// rollback-on-lint-failure.
func TestChangeStatus_RestoreUndoesRewrite(t *testing.T) {
	featDir := writeFeatureFixture(t, "Draft")
	path := filepath.Join(featDir, "auth", "README.md")
	before, _ := os.ReadFile(path)

	result, err := ChangeStatus(featDir, "auth", "approved")
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	if got := readStatusLine(t, path); got != "Approved" {
		t.Fatalf("expected post-rewrite status Approved, got %q", got)
	}

	if err := result.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Errorf("Restore did not produce byte-identical content:\nbefore=%q\nafter =%q", before, after)
	}
}
