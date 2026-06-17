package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lint"
)

// setupFeatureSpec stages a lint-clean spec tree at a fresh t.TempDir,
// then chdirs into it so resolveFeaturesDir picks it up via the
// cwd-anchored FindSpecRepoRoot heuristic. Returns the repo root for
// path computations in assertions.
//
// "lint-clean" means: every artifact a default `spec lint` pass cares
// about is present and adherent. Hand-derived by running Lint against
// minimal fixtures and adding the bits that fired:
//
//   - spec/README.md with a Contents section linking features/
//   - spec/features/README.md with index header, the row for `auth`,
//     and the adherence footer
//   - spec/features/auth/README.md with Status, Summary, Outstanding
//     Questions, and adherence footer
//
// Tests that need additional features (e.g., the nested-feature-id
// case) layer files on top of this fixture after it's set up.
func setupFeatureSpec(t *testing.T, status string) string {
	t.Helper()
	root := t.TempDir()
	specDir := filepath.Join(root, "spec")
	featDir := filepath.Join(specDir, "features")
	if err := os.MkdirAll(filepath.Join(featDir, "auth"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	specReadme := "# Specifications\n\n## Contents\n\n- [features](features/README.md)\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(specDir, "README.md"), []byte(specReadme), 0o644); err != nil {
		t.Fatalf("write spec/README.md: %v", err)
	}

	idxBody := "# Features\n\n" +
		"| Feature | Status | Kind | Description |\n" +
		"|---------|--------|------|-------------|\n" +
		fmt.Sprintf("| [auth](auth/README.md) | %s | Command | desc-auth |\n", status) +
		"\n## Open Questions\n\nNone at this time.\n\n" +
		"---\n*This document follows the https://specscore.md/features-index-specification*\n"
	if err := os.WriteFile(filepath.Join(featDir, "README.md"), []byte(idxBody), 0o644); err != nil {
		t.Fatalf("write features/README.md: %v", err)
	}

	fBody := "# Feature: Auth\n\n**Status:** " + status + "\n**Source Ideas:** —\n\n## Summary\n\nPlaceholder.\n\n" +
		"## Open Questions\n\nNone at this time.\n\n" +
		"---\n*This document follows the https://specscore.md/feature-specification*\n"
	if err := os.WriteFile(filepath.Join(featDir, "auth", "README.md"), []byte(fBody), 0o644); err != nil {
		t.Fatalf("write auth/README.md: %v", err)
	}

	migrateTree(t, root)
	withCwd(t, root)
	return root
}

// runFeature invokes the `feature` cobra command tree in-process and
// captures stdout, stderr, and the returned error. Mirrors
// internal/cli/idea_test.go's runIdea helper.
func runFeature(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := featureCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

// exitCodeOfErr returns the ExitCode() carried by err if it is a typed
// exitcode.Error; otherwise -1. Used by every CLI-level test to assert
// the right code is mapped from the underlying error.
func exitCodeOfErr(err error) int {
	var ec *exitcode.Error
	if errors.As(err, &ec) {
		return ec.ExitCode()
	}
	return -1
}

// readAuthStatus returns the **Status:** value in the auth feature's
// README at <repoRoot>/spec/features/auth/README.md.
func readAuthStatus(t *testing.T, repoRoot string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot, "spec", "features", "auth", "README.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "**Status:**") {
			return strings.TrimSpace(strings.TrimPrefix(t, "**Status:**"))
		}
	}
	return ""
}

// readIndexStatus returns the Status cell value for `auth` in the
// features-index README.
func readIndexStatus(t *testing.T, repoRoot string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot, "spec", "features", "README.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, "[auth](auth/README.md)") {
			continue
		}
		// Row format: `| [auth](...) | Draft | Command | desc-auth |`
		cells := strings.Split(line, "|")
		if len(cells) < 3 {
			continue
		}
		return strings.TrimSpace(cells[2])
	}
	return ""
}

// AC: draft-to-under-review-happy-path
func TestFeatureChangeStatus_DraftToInReview(t *testing.T) {
	root := setupFeatureSpec(t, "Draft")

	out, errOut, err := runFeature(t, "change-status", "auth", "--to=in review")
	if err != nil {
		t.Fatalf("unexpected err: %v\nstderr=%s", err, errOut)
	}
	if got, want := out, "auth: Draft → In Review\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if got := readAuthStatus(t, root); got != "In Review" {
		t.Errorf("README Status = %q, want In Review", got)
	}
	if got := readIndexStatus(t, root); got != "In Review" {
		t.Errorf("index Status = %q, want In Review (feature-index-row-sync)", got)
	}
}

// AC: draft-direct-to-approved-happy-path
func TestFeatureChangeStatus_DraftDirectToApproved(t *testing.T) {
	root := setupFeatureSpec(t, "Draft")

	out, errOut, err := runFeature(t, "change-status", "auth", "--to=approved")
	if err != nil {
		t.Fatalf("unexpected err: %v\nstderr=%s", err, errOut)
	}
	if !strings.HasPrefix(out, "auth: Draft → Approved") {
		t.Errorf("stdout = %q, want prefix `auth: Draft → Approved`", out)
	}
	if got := readAuthStatus(t, root); got != "Approved" {
		t.Errorf("Status = %q, want Approved", got)
	}
}

// AC: under-review-to-approved-happy-path
func TestFeatureChangeStatus_InReviewToApproved(t *testing.T) {
	root := setupFeatureSpec(t, "In Review")

	out, errOut, err := runFeature(t, "change-status", "auth", "--to=approved")
	if err != nil {
		t.Fatalf("unexpected err: %v\nstderr=%s", err, errOut)
	}
	if got, want := out, "auth: In Review → Approved\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if got := readAuthStatus(t, root); got != "Approved" {
		t.Errorf("Status = %q, want Approved", got)
	}
}

// AC: implementing-to-stable-happy-path
func TestFeatureChangeStatus_ImplementingToStable(t *testing.T) {
	root := setupFeatureSpec(t, "Implementing")

	out, _, err := runFeature(t, "change-status", "auth", "--to=stable")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got, want := out, "auth: Implementing → Stable\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if got := readAuthStatus(t, root); got != "Stable" {
		t.Errorf("Status = %q, want Stable", got)
	}
}

// AC: stable-to-deprecated-happy-path
func TestFeatureChangeStatus_StableToDeprecated(t *testing.T) {
	root := setupFeatureSpec(t, "Stable")

	out, _, err := runFeature(t, "change-status", "auth", "--to=deprecated")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got, want := out, "auth: Stable → Deprecated\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if got := readAuthStatus(t, root); got != "Deprecated" {
		t.Errorf("Status = %q, want Deprecated", got)
	}
}

// AC: nested-feature-id-resolves — a sub-feature at
// spec/features/cli/idea/change-status/README.md transitions correctly
// via the slash-bearing id.
//
// The sub-feature is added on top of the canonical fixture; the
// features-index only lists top-level rows so no index update is
// needed for the sub-feature. Lint stays clean because the
// feature-index-row-sync rule scope is top-level-only.
func TestFeatureChangeStatus_NestedFeatureID(t *testing.T) {
	root := setupFeatureSpec(t, "Draft")

	nested := filepath.Join(root, "spec", "features", "cli", "idea", "change-status")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	body := "# Feature: Change Status\n\n**Status:** Draft\n\n## Summary\n\nPlaceholder.\n\n" +
		"## Open Questions\n\nNone at this time.\n\n" +
		"---\n*This document follows the https://specscore.md/feature-specification*\n"
	if err := os.WriteFile(filepath.Join(nested, "README.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// The parent `cli` and `cli/idea` directories also need READMEs
	// for forward-link checkers; minimal placeholders satisfy the
	// adherence-footer + OQ rules. Each parent must also link its
	// immediate child directory so `index-entries` stays bidirectionally
	// clean.
	parents := []struct {
		dir   string
		child string
	}{
		{filepath.Join(root, "spec", "features", "cli"), "idea"},
		{filepath.Join(root, "spec", "features", "cli", "idea"), "change-status"},
	}
	for _, p := range parents {
		readme := "# Feature: " + filepath.Base(p.dir) + "\n\n" +
			"**Status:** Approved\n\n## Summary\n\nPlaceholder.\n\n" +
			"## Contents\n\n" +
			"- [" + p.child + "](" + p.child + "/README.md)\n\n" +
			"## Open Questions\n\nNone at this time.\n\n" +
			"---\n*This document follows the https://specscore.md/feature-specification*\n"
		if err := os.WriteFile(filepath.Join(p.dir, "README.md"), []byte(readme), 0o644); err != nil {
			t.Fatalf("write parent: %v", err)
		}
	}
	// The top-level features-index README, seeded by setupFeatureSpec with
	// only `auth`, must also list `cli` now that we created that subtree.
	idxPath := filepath.Join(root, "spec", "features", "README.md")
	idxData, err := os.ReadFile(idxPath)
	if err != nil {
		t.Fatalf("read features index: %v", err)
	}
	idxWithCLI := strings.Replace(
		string(idxData),
		"| [auth](auth/README.md) |",
		"| [cli](cli/README.md) | Approved | Command | desc-cli |\n| [auth](auth/README.md) |",
		1,
	)
	if err := os.WriteFile(idxPath, []byte(idxWithCLI), 0o644); err != nil {
		t.Fatalf("rewrite features index: %v", err)
	}
	migrateTree(t, root)

	out, errOut, err := runFeature(t, "change-status", "cli/idea/change-status", "--to=approved")
	if err != nil {
		t.Fatalf("unexpected err: %v\nstderr=%s", err, errOut)
	}
	if got, want := out, "cli/idea/change-status: Draft → Approved\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}

	data, _ := os.ReadFile(filepath.Join(nested, "README.md"))
	if !strings.Contains(string(data), "**Status:** Approved") {
		t.Errorf("nested README not rewritten:\n%s", data)
	}
}

// AC: case-insensitive-to-flag — STABLE / Stable / stable transition
// an Implementing feature identically. File ends up with canonical
// title-case "Stable".
func TestFeatureChangeStatus_CaseInsensitiveTo(t *testing.T) {
	for _, raw := range []string{"STABLE", "Stable", "stable"} {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			root := setupFeatureSpec(t, "Implementing")
			out, _, err := runFeature(t, "change-status", "auth", "--to="+raw)
			if err != nil {
				t.Fatalf("unexpected err for --to=%q: %v", raw, err)
			}
			if !strings.HasSuffix(strings.TrimRight(out, "\n"), " Stable") {
				t.Errorf("stdout = %q, want suffix ` Stable`", out)
			}
			if got := readAuthStatus(t, root); got != "Stable" {
				t.Errorf("Status = %q, want canonical `Stable`", got)
			}
		})
	}
}

// AC: illegal-transition-rejected — Draft → implementing exits 4
// with stderr naming the current status and legal targets. Also
// exercise a couple of other illegal pairs end-to-end.
func TestFeatureChangeStatus_IllegalTransitionRejected(t *testing.T) {
	cases := []struct {
		name string
		from string
		to   string
	}{
		{"draft → implementing", "Draft", "implementing"},
		{"draft → stable", "Draft", "stable"},
		{"stable → approved", "Stable", "approved"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			root := setupFeatureSpec(t, tc.from)
			out, _, err := runFeature(t, "change-status", "auth", "--to="+tc.to)
			if err == nil {
				t.Fatalf("expected error for %s → %s", tc.from, tc.to)
			}
			if got := exitCodeOfErr(err); got != exitcode.InvalidState {
				t.Errorf("exit code = %d, want %d (InvalidState)", got, exitcode.InvalidState)
			}
			if out != "" {
				t.Errorf("stdout must be empty on failure, got %q", out)
			}
			if got := readAuthStatus(t, root); got != tc.from {
				t.Errorf("file mutated unexpectedly: got %q, want %q", got, tc.from)
			}
			// Error message names the current status and at least
			// one legal target — surfaced via the wrapped
			// InvalidTransitionError's Error() string.
			if !strings.Contains(err.Error(), tc.from) {
				t.Errorf("err %q does not name current status %q", err, tc.from)
			}
		})
	}
}

// AC: reverse-transition-rejected — Stable → Implementing exits 4.
func TestFeatureChangeStatus_ReverseTransitionRejected(t *testing.T) {
	root := setupFeatureSpec(t, "Stable")
	_, _, err := runFeature(t, "change-status", "auth", "--to=implementing")
	if err == nil {
		t.Fatal("expected error for Stable → Implementing")
	}
	if got := exitCodeOfErr(err); got != exitcode.InvalidState {
		t.Errorf("exit code = %d, want %d", got, exitcode.InvalidState)
	}
	if got := readAuthStatus(t, root); got != "Stable" {
		t.Errorf("file mutated unexpectedly: got %q, want Stable", got)
	}
}

// AC: already-at-target-rejected — Approved → Approved exits 4 per
// REQ: not-idempotent.
func TestFeatureChangeStatus_AlreadyAtTargetRejected(t *testing.T) {
	root := setupFeatureSpec(t, "Approved")
	_, _, err := runFeature(t, "change-status", "auth", "--to=approved")
	if err == nil {
		t.Fatal("expected error for Approved → Approved")
	}
	if got := exitCodeOfErr(err); got != exitcode.InvalidState {
		t.Errorf("exit code = %d, want %d", got, exitcode.InvalidState)
	}
	if got := readAuthStatus(t, root); got != "Approved" {
		t.Errorf("file mutated unexpectedly: got %q, want Approved", got)
	}
}

// AC: unrecognized-to-value-rejected — --to=banana exits 2 BEFORE the
// state-machine check. --to=archived (Idea-only) and --to=draft (no
// arc INTO Draft) MUST also exit 2.
func TestFeatureChangeStatus_UnrecognizedToValue(t *testing.T) {
	for _, raw := range []string{"banana", "archived", "specified", "draft"} {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			root := setupFeatureSpec(t, "Draft")
			_, _, err := runFeature(t, "change-status", "auth", "--to="+raw)
			if err == nil {
				t.Fatalf("expected error for --to=%q", raw)
			}
			if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
				t.Errorf("exit code = %d, want %d (InvalidArgs)", got, exitcode.InvalidArgs)
			}
			if got := readAuthStatus(t, root); got != "Draft" {
				t.Errorf("file mutated unexpectedly: got %q, want Draft", got)
			}
		})
	}
}

// AC: missing-feature-id-rejected — no positional argument exits 2.
func TestFeatureChangeStatus_MissingFeatureID(t *testing.T) {
	setupFeatureSpec(t, "Draft")
	_, _, err := runFeature(t, "change-status", "--to=approved")
	if err == nil {
		t.Fatal("expected error for missing feature_id")
	}
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d (InvalidArgs)", got, exitcode.InvalidArgs)
	}
}

// AC: missing-to-flag-rejected — no --to flag exits 2.
func TestFeatureChangeStatus_MissingToFlag(t *testing.T) {
	setupFeatureSpec(t, "Draft")
	_, _, err := runFeature(t, "change-status", "auth")
	if err == nil {
		t.Fatal("expected error for missing --to")
	}
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d (InvalidArgs)", got, exitcode.InvalidArgs)
	}
	if !strings.Contains(err.Error(), "--to") {
		t.Errorf("error message does not mention --to: %v", err)
	}
}

// AC: feature-not-found — a missing feature directory exits 3.
func TestFeatureChangeStatus_FeatureNotFound(t *testing.T) {
	setupFeatureSpec(t, "Draft")
	_, _, err := runFeature(t, "change-status", "nonexistent", "--to=approved")
	if err == nil {
		t.Fatal("expected error for missing feature")
	}
	if got := exitCodeOfErr(err); got != exitcode.NotFound {
		t.Errorf("exit code = %d, want %d (NotFound)", got, exitcode.NotFound)
	}
}

// --- feature list tests ---

func TestFeatureList_Text(t *testing.T) {
	setupFeatureSpec(t, "Approved")

	out, _, err := runFeature(t, "list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "auth") {
		t.Errorf("stdout = %q, want it to contain 'auth'", out)
	}
}

func TestFeatureList_YAML(t *testing.T) {
	setupFeatureSpec(t, "Approved")

	out, _, err := runFeature(t, "list", "--format=yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "path: auth") {
		t.Errorf("stdout = %q, want it to contain 'path: auth'", out)
	}
}

func TestFeatureList_JSON(t *testing.T) {
	setupFeatureSpec(t, "Approved")

	out, _, err := runFeature(t, "list", "--format=json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"path"`) {
		t.Errorf("stdout = %q, want it to contain '\"path\"'", out)
	}
	if !strings.Contains(out, "auth") {
		t.Errorf("stdout = %q, want it to contain 'auth'", out)
	}
}

func TestFeatureList_WithFields(t *testing.T) {
	setupFeatureSpec(t, "Approved")

	out, _, err := runFeature(t, "list", "--fields=status")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// --fields auto-switches to yaml format
	if !strings.Contains(out, "status: Approved") {
		t.Errorf("stdout = %q, want it to contain 'status: Approved'", out)
	}
}

func TestFeatureList_InvalidFormat(t *testing.T) {
	setupFeatureSpec(t, "Approved")

	_, _, err := runFeature(t, "list", "--format=csv")
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d (InvalidArgs)", got, exitcode.InvalidArgs)
	}
}

// --- feature info tests ---

func TestFeatureInfo_YAML(t *testing.T) {
	setupFeatureSpec(t, "Approved")

	out, _, err := runFeature(t, "info", "auth")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "path: auth") {
		t.Errorf("stdout = %q, want it to contain 'path: auth'", out)
	}
	if !strings.Contains(out, "status: Approved") {
		t.Errorf("stdout = %q, want it to contain 'status: Approved'", out)
	}
}

func TestFeatureInfo_JSON(t *testing.T) {
	setupFeatureSpec(t, "Approved")

	out, _, err := runFeature(t, "info", "auth", "--format=json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"path"`) {
		t.Errorf("stdout = %q, want it to contain '\"path\"'", out)
	}
	if !strings.Contains(out, `"auth"`) {
		t.Errorf("stdout = %q, want it to contain '\"auth\"'", out)
	}
}

func TestFeatureInfo_Text(t *testing.T) {
	setupFeatureSpec(t, "Approved")

	out, _, err := runFeature(t, "info", "auth", "--format=text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Feature: auth") {
		t.Errorf("stdout = %q, want it to contain 'Feature: auth'", out)
	}
	if !strings.Contains(out, "Status:  Approved") {
		t.Errorf("stdout = %q, want it to contain 'Status:  Approved'", out)
	}
}

func TestFeatureInfo_NotFound(t *testing.T) {
	setupFeatureSpec(t, "Draft")

	_, _, err := runFeature(t, "info", "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent feature")
	}
	if got := exitCodeOfErr(err); got != exitcode.NotFound {
		t.Errorf("exit code = %d, want %d (NotFound)", got, exitcode.NotFound)
	}
}

// --- feature tree tests ---

func TestFeatureTree_FullTree(t *testing.T) {
	setupFeatureSpec(t, "Approved")

	out, _, err := runFeature(t, "tree")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "auth") {
		t.Errorf("stdout = %q, want it to contain 'auth'", out)
	}
}

func TestFeatureTree_Focused(t *testing.T) {
	setupFeatureSpec(t, "Approved")

	out, _, err := runFeature(t, "tree", "auth")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "* auth") {
		t.Errorf("stdout = %q, want it to contain '* auth' (focused marker)", out)
	}
}

func TestFeatureTree_DirectionDown(t *testing.T) {
	setupFeatureSpec(t, "Approved")

	out, _, err := runFeature(t, "tree", "auth", "--direction=down")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "auth") {
		t.Errorf("stdout = %q, want it to contain 'auth'", out)
	}
}

func TestFeatureTree_InvalidDirection(t *testing.T) {
	setupFeatureSpec(t, "Approved")

	_, _, err := runFeature(t, "tree", "auth", "--direction=sideways")
	if err == nil {
		t.Fatal("expected error for invalid direction")
	}
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d (InvalidArgs)", got, exitcode.InvalidArgs)
	}
}

func TestFeatureTree_DirectionWithoutID(t *testing.T) {
	setupFeatureSpec(t, "Approved")

	_, _, err := runFeature(t, "tree", "--direction=down")
	if err == nil {
		t.Fatal("expected error for --direction without feature_id")
	}
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d (InvalidArgs)", got, exitcode.InvalidArgs)
	}
}

// --- feature deps/refs helpers ---

// setupFeatureWithDeps extends the base fixture with a second feature
// `billing` that depends on `auth`.
func setupFeatureWithDeps(t *testing.T) string {
	t.Helper()
	root := setupFeatureSpec(t, "Approved")

	billingDir := filepath.Join(root, "spec", "features", "billing")
	if err := os.MkdirAll(billingDir, 0o755); err != nil {
		t.Fatalf("mkdir billing: %v", err)
	}
	body := "# Feature: Billing\n\n**Status:** Draft\n\n## Summary\n\nBilling.\n\n## Dependencies\n\n- auth\n\n## Open Questions\n\nNone at this time.\n\n---\n*This document follows the https://specscore.md/feature-specification*\n"
	if err := os.WriteFile(filepath.Join(billingDir, "README.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write billing/README.md: %v", err)
	}

	// Update features index to include billing
	idxPath := filepath.Join(root, "spec", "features", "README.md")
	idxData, err := os.ReadFile(idxPath)
	if err != nil {
		t.Fatalf("read features index: %v", err)
	}
	updated := strings.Replace(string(idxData),
		"## Open Questions",
		"| [billing](billing/README.md) | Draft | Command | desc-billing |\n\n## Open Questions", 1)
	if err := os.WriteFile(idxPath, []byte(updated), 0o644); err != nil {
		t.Fatalf("rewrite features index: %v", err)
	}
	return root
}

// --- feature deps tests ---

func TestFeatureDeps_NoDeps(t *testing.T) {
	setupFeatureSpec(t, "Approved")

	out, _, err := runFeature(t, "deps", "auth")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// auth has no dependencies — output should be empty
	if strings.TrimSpace(out) != "" {
		t.Errorf("stdout = %q, want empty (no deps)", out)
	}
}

func TestFeatureDeps_WithDeps(t *testing.T) {
	setupFeatureWithDeps(t)

	out, _, err := runFeature(t, "deps", "billing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "auth") {
		t.Errorf("stdout = %q, want it to contain 'auth'", out)
	}
}

func TestFeatureDeps_NotFound(t *testing.T) {
	setupFeatureSpec(t, "Draft")

	_, _, err := runFeature(t, "deps", "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent feature")
	}
	if got := exitCodeOfErr(err); got != exitcode.NotFound {
		t.Errorf("exit code = %d, want %d (NotFound)", got, exitcode.NotFound)
	}
}

// --- feature refs tests ---

func TestFeatureRefs_NoRefs(t *testing.T) {
	setupFeatureSpec(t, "Approved")

	out, _, err := runFeature(t, "refs", "auth")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// auth has no incoming references — output should be empty
	if strings.TrimSpace(out) != "" {
		t.Errorf("stdout = %q, want empty (no refs)", out)
	}
}

func TestFeatureRefs_WithRefs(t *testing.T) {
	setupFeatureWithDeps(t)

	out, _, err := runFeature(t, "refs", "auth")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "billing") {
		t.Errorf("stdout = %q, want it to contain 'billing'", out)
	}
}

// AC: lint-failure-rolls-back — when a post-rewrite lint pass reports
// an error-severity violation, the original Status line MUST be
// restored and the command MUST exit 10.
//
// We engineer the failure by introducing a pre-existing error-severity
// violation OUTSIDE the auth feature: a second feature `broken` whose
// README declares Approved while the features-index row says Draft.
// feature-index-row-sync's --fix path WILL repair the broken row when
// it runs (the lint rule rewrites every drifted top-level row in one
// pass), so this fixture won't actually leave a violation behind.
//
// To get a deterministic rollback, we plant a DIFFERENT kind of
// error-severity violation: a feature with NO `**Status:**` line at
// all — which feature-index-row-sync silently skips (it cannot
// determine a wanted status), but the `oq-section` rule reports as
// an error on the missing Open Questions section. Even though
// the oq-section rule reports on a totally unrelated file, the Meta
// REQ rollback-on-lint-failure is explicit: ANY error-severity
// violation in the spec tree after --fix triggers rollback.
func TestFeatureChangeStatus_LintFailureRollsBack(t *testing.T) {
	root := setupFeatureSpec(t, "Draft")

	// Plant a broken feature that the lint pass will flag as
	// error-severity but feature-index-row-sync's --fix cannot
	// silence. A README missing the Open Questions section
	// fires oq-section (error). The features-index does NOT list
	// this feature, so feature-index-row-sync stays silent.
	broken := filepath.Join(root, "spec", "features", "broken")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	brokenBody := "# Feature: Broken\n\n**Status:** Draft\n\n## Summary\n\nNo OQ section here.\n"
	if err := os.WriteFile(filepath.Join(broken, "README.md"), []byte(brokenBody), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	beforeAuth, _ := os.ReadFile(filepath.Join(root, "spec", "features", "auth", "README.md"))

	out, _, err := runFeature(t, "change-status", "auth", "--to=approved")
	if err == nil {
		t.Fatal("expected error due to pre-existing lint violation")
	}
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Errorf("exit code = %d, want %d (Unexpected/10)", got, exitcode.Unexpected)
	}
	if out != "" {
		t.Errorf("stdout must be empty on failure, got %q", out)
	}

	// Rollback: auth/README.md byte-identical to the pre-invocation
	// snapshot, including the **Status:** Draft line.
	afterAuth, _ := os.ReadFile(filepath.Join(root, "spec", "features", "auth", "README.md"))
	if string(beforeAuth) != string(afterAuth) {
		t.Errorf("rollback did not restore auth/README.md:\nbefore=%q\nafter =%q", beforeAuth, afterAuth)
	}
}

// --- setupFeatureChain helper ---

// setupFeatureChain creates a 3-feature chain: auth (Approved, no deps),
// billing (Draft, depends on auth), payments (Draft, depends on billing).
// Returns the repo root.
func setupFeatureChain(t *testing.T) string {
	t.Helper()
	root := setupFeatureSpec(t, "Approved")
	featDir := filepath.Join(root, "spec", "features")

	// Add billing depending on auth
	billingDir := filepath.Join(featDir, "billing")
	if err := os.MkdirAll(billingDir, 0o755); err != nil {
		t.Fatalf("mkdir billing: %v", err)
	}
	billingBody := "# Feature: Billing\n\n**Status:** Draft\n\n## Summary\n\nBilling.\n\n## Dependencies\n\n- auth\n\n## Open Questions\n\nNone at this time.\n\n---\n*This document follows the https://specscore.md/feature-specification*\n"
	if err := os.WriteFile(filepath.Join(billingDir, "README.md"), []byte(billingBody), 0o644); err != nil {
		t.Fatalf("write billing/README.md: %v", err)
	}

	// Add payments depending on billing
	paymentsDir := filepath.Join(featDir, "payments")
	if err := os.MkdirAll(paymentsDir, 0o755); err != nil {
		t.Fatalf("mkdir payments: %v", err)
	}
	paymentsBody := "# Feature: Payments\n\n**Status:** Draft\n\n## Summary\n\nPayments.\n\n## Dependencies\n\n- billing\n\n## Open Questions\n\nNone at this time.\n\n---\n*This document follows the https://specscore.md/feature-specification*\n"
	if err := os.WriteFile(filepath.Join(paymentsDir, "README.md"), []byte(paymentsBody), 0o644); err != nil {
		t.Fatalf("write payments/README.md: %v", err)
	}

	// Update features index
	idxPath := filepath.Join(featDir, "README.md")
	idxData, err := os.ReadFile(idxPath)
	if err != nil {
		t.Fatalf("read features index: %v", err)
	}
	updated := strings.Replace(string(idxData),
		"## Open Questions",
		"| [billing](billing/README.md) | Draft | Command | desc-billing |\n| [payments](payments/README.md) | Draft | Command | desc-payments |\n\n## Open Questions", 1)
	if err := os.WriteFile(idxPath, []byte(updated), 0o644); err != nil {
		t.Fatalf("rewrite features index: %v", err)
	}
	return root
}

// --- feature list: fields + text format ---

func TestFeatureList_FieldsText(t *testing.T) {
	setupFeatureSpec(t, "Draft")

	out, _, err := runFeature(t, "list", "--fields=status", "--format=text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "auth") {
		t.Errorf("stdout = %q, want it to contain 'auth'", out)
	}
	if !strings.Contains(out, "status=Draft") {
		t.Errorf("stdout = %q, want it to contain 'status=Draft'", out)
	}
}

// --- feature list: fields + JSON ---

func TestFeatureList_FieldsJSON(t *testing.T) {
	setupFeatureSpec(t, "Approved")

	out, _, err := runFeature(t, "list", "--fields=status", "--format=json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json unmarshal: %v\noutput=%q", err, out)
	}
	if len(result) == 0 {
		t.Fatal("expected at least one feature in JSON output")
	}
	if result[0]["path"] != "auth" {
		t.Errorf("path = %v, want auth", result[0]["path"])
	}
	if result[0]["status"] != "Approved" {
		t.Errorf("status = %v, want Approved", result[0]["status"])
	}
}

// --- feature tree: fields + text ---

func TestFeatureTree_FieldsText(t *testing.T) {
	setupFeatureSpec(t, "Approved")

	out, _, err := runFeature(t, "tree", "--fields=status", "--format=text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "auth") {
		t.Errorf("stdout = %q, want it to contain 'auth'", out)
	}
	if !strings.Contains(out, "status=Approved") {
		t.Errorf("stdout = %q, want it to contain 'status=Approved'", out)
	}
}

// --- feature tree: fields + YAML ---

func TestFeatureTree_FieldsYAML(t *testing.T) {
	setupFeatureSpec(t, "Approved")

	out, _, err := runFeature(t, "tree", "--fields=status", "--format=yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "path: auth") {
		t.Errorf("stdout = %q, want it to contain 'path: auth'", out)
	}
	if !strings.Contains(out, "status: Approved") {
		t.Errorf("stdout = %q, want it to contain 'status: Approved'", out)
	}
}

// --- feature tree: fields + JSON ---

func TestFeatureTree_FieldsJSON(t *testing.T) {
	setupFeatureSpec(t, "Approved")

	out, _, err := runFeature(t, "tree", "--fields=status", "--format=json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json unmarshal: %v\noutput=%q", err, out)
	}
	if len(result) == 0 {
		t.Fatal("expected at least one feature in JSON output")
	}
	if result[0]["path"] != "auth" {
		t.Errorf("path = %v, want auth", result[0]["path"])
	}
	if result[0]["status"] != "Approved" {
		t.Errorf("status = %v, want Approved", result[0]["status"])
	}
}

// --- feature deps: transitive ---

func TestFeatureDeps_Transitive(t *testing.T) {
	setupFeatureChain(t)

	out, _, err := runFeature(t, "deps", "payments", "--transitive")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "billing") {
		t.Errorf("stdout = %q, want it to contain 'billing'", out)
	}
	if !strings.Contains(out, "auth") {
		t.Errorf("stdout = %q, want it to contain 'auth' (transitive dep)", out)
	}
}

func TestFeatureDeps_TransitiveText(t *testing.T) {
	setupFeatureChain(t)

	out, _, err := runFeature(t, "deps", "payments", "--transitive", "--format=text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Transitive text uses indentation: billing at depth 0, auth at depth 1
	if !strings.Contains(out, "billing") {
		t.Errorf("stdout = %q, want it to contain 'billing'", out)
	}
	if !strings.Contains(out, "\tauth") {
		t.Errorf("stdout = %q, want it to contain indented 'auth'", out)
	}
}

func TestFeatureDeps_WithFieldsYAML(t *testing.T) {
	setupFeatureChain(t)

	out, _, err := runFeature(t, "deps", "billing", "--fields=status", "--format=yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "path: auth") {
		t.Errorf("stdout = %q, want it to contain 'path: auth'", out)
	}
	if !strings.Contains(out, "status: Approved") {
		t.Errorf("stdout = %q, want it to contain 'status: Approved'", out)
	}
}

func TestFeatureDeps_FormatJSON(t *testing.T) {
	setupFeatureChain(t)

	out, _, err := runFeature(t, "deps", "billing", "--format=json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json unmarshal: %v\noutput=%q", err, out)
	}
	if len(result) == 0 {
		t.Fatal("expected at least one dep in JSON output")
	}
	if result[0]["path"] != "auth" {
		t.Errorf("path = %v, want auth", result[0]["path"])
	}
}

func TestFeatureDeps_TransitiveWithFields(t *testing.T) {
	setupFeatureChain(t)

	out, _, err := runFeature(t, "deps", "payments", "--transitive", "--fields=status", "--format=text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "billing") {
		t.Errorf("stdout = %q, want it to contain 'billing'", out)
	}
	if !strings.Contains(out, "status=Draft") {
		t.Errorf("stdout = %q, want it to contain 'status=Draft'", out)
	}
}

func TestFeatureDeps_TransitiveYAML(t *testing.T) {
	setupFeatureChain(t)

	out, _, err := runFeature(t, "deps", "payments", "--transitive", "--format=yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "path: billing") {
		t.Errorf("stdout = %q, want it to contain 'path: billing'", out)
	}
	if !strings.Contains(out, "path: auth") {
		t.Errorf("stdout = %q, want it to contain 'path: auth'", out)
	}
}

func TestFeatureDeps_TransitiveJSON(t *testing.T) {
	setupFeatureChain(t)

	out, _, err := runFeature(t, "deps", "payments", "--transitive", "--format=json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json unmarshal: %v\noutput=%q", err, out)
	}
	if len(result) == 0 {
		t.Fatal("expected at least one node in JSON output")
	}
	if result[0]["path"] != "billing" {
		t.Errorf("path = %v, want billing", result[0]["path"])
	}
}

// --- feature refs: transitive ---

func TestFeatureRefs_Transitive(t *testing.T) {
	setupFeatureChain(t)

	out, _, err := runFeature(t, "refs", "auth", "--transitive")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "billing") {
		t.Errorf("stdout = %q, want it to contain 'billing'", out)
	}
	if !strings.Contains(out, "payments") {
		t.Errorf("stdout = %q, want it to contain 'payments' (transitive ref)", out)
	}
}

func TestFeatureRefs_FormatYAML(t *testing.T) {
	setupFeatureWithDeps(t)

	out, _, err := runFeature(t, "refs", "auth", "--format=yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "path: billing") {
		t.Errorf("stdout = %q, want it to contain 'path: billing'", out)
	}
}

func TestFeatureRefs_FormatJSON(t *testing.T) {
	setupFeatureWithDeps(t)

	out, _, err := runFeature(t, "refs", "auth", "--format=json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json unmarshal: %v\noutput=%q", err, out)
	}
	if len(result) == 0 {
		t.Fatal("expected at least one ref in JSON output")
	}
	if result[0]["path"] != "billing" {
		t.Errorf("path = %v, want billing", result[0]["path"])
	}
}

func TestFeatureRefs_WithFields(t *testing.T) {
	setupFeatureWithDeps(t)

	out, _, err := runFeature(t, "refs", "auth", "--fields=status")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// --fields auto-selects yaml format
	if !strings.Contains(out, "path: billing") {
		t.Errorf("stdout = %q, want it to contain 'path: billing'", out)
	}
	if !strings.Contains(out, "status: Draft") {
		t.Errorf("stdout = %q, want it to contain 'status: Draft'", out)
	}
}

func TestFeatureRefs_TransitiveYAML(t *testing.T) {
	setupFeatureChain(t)

	out, _, err := runFeature(t, "refs", "auth", "--transitive", "--format=yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "path: billing") {
		t.Errorf("stdout = %q, want it to contain 'path: billing'", out)
	}
}

func TestFeatureRefs_TransitiveJSON(t *testing.T) {
	setupFeatureChain(t)

	out, _, err := runFeature(t, "refs", "auth", "--transitive", "--format=json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json unmarshal: %v\noutput=%q", err, out)
	}
	if len(result) == 0 {
		t.Fatal("expected at least one node in JSON output")
	}
	if result[0]["path"] != "billing" {
		t.Errorf("path = %v, want billing", result[0]["path"])
	}
}

func TestFeatureRefs_TransitiveWithFields(t *testing.T) {
	setupFeatureChain(t)

	out, _, err := runFeature(t, "refs", "auth", "--transitive", "--fields=status", "--format=text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "billing") {
		t.Errorf("stdout = %q, want it to contain 'billing'", out)
	}
	if !strings.Contains(out, "status=Draft") {
		t.Errorf("stdout = %q, want it to contain 'status=Draft'", out)
	}
}

// --- feature new ---

func TestFeatureNew_Basic(t *testing.T) {
	setupFeatureSpec(t, "Draft")

	out, _, err := runFeature(t, "new", "--title=New Feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "new-feature") {
		t.Errorf("stdout = %q, want it to contain 'new-feature' (generated slug)", out)
	}
}

// TestFeatureNew_LintClean verifies that a freshly scaffolded feature
// README passes a read-only lint without any follow-up --fix pass — i.e.
// the scaffolder backfills the project-specific studio-toolbar and the
// **Source Ideas:** sentinel itself. Regression guard for the
// scaffolder↔lint mismatch where `feature new` emitted a header the
// 0.10.0 linter rejected (missing studio-toolbar + Source Ideas lines).
func TestFeatureNew_LintClean(t *testing.T) {
	root := t.TempDir()
	withCwd(t, root)

	if _, errOut, err := runInitCmd(t, nil,
		"--project", root,
		"--host", "github.com",
		"--org", "acme",
		"--repo", "widget",
	); err != nil {
		t.Fatalf("init failed: %v\nstderr=%s", err, errOut)
	}

	if _, errOut, err := runFeature(t, "new",
		"--title", "Billing",
		"--description", "Billing system",
	); err != nil {
		t.Fatalf("feature new failed: %v\nstderr=%s", err, errOut)
	}

	// Read-only lint (no Fix) — the scaffold must already be clean.
	violations, err := lint.Lint(lint.Options{SpecRoot: filepath.Join(root, "spec")})
	if err != nil {
		t.Fatalf("lint invocation failed: %v", err)
	}
	for _, v := range violations {
		if v.Severity == "error" {
			t.Errorf("scaffolded feature is not lint-clean: %s:%d [%s] %s",
				v.File, v.Line, v.Rule, v.Message)
		}
	}
}

func TestFeatureNew_WithParent(t *testing.T) {
	setupFeatureSpec(t, "Draft")

	out, _, err := runFeature(t, "new", "--title=Sub Feature", "--parent=auth")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "auth/sub-feature") {
		t.Errorf("stdout = %q, want it to contain 'auth/sub-feature'", out)
	}
}

func TestFeatureNew_MissingTitle(t *testing.T) {
	setupFeatureSpec(t, "Draft")

	_, _, err := runFeature(t, "new")
	if err == nil {
		t.Fatal("expected error for missing --title")
	}
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d (InvalidArgs)", got, exitcode.InvalidArgs)
	}
}

func TestFeatureNew_InvalidFormat(t *testing.T) {
	setupFeatureSpec(t, "Draft")

	_, _, err := runFeature(t, "new", "--title=Foo", "--format=csv")
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d (InvalidArgs)", got, exitcode.InvalidArgs)
	}
}

func TestFeatureNew_JSONOutput(t *testing.T) {
	setupFeatureSpec(t, "Draft")

	out, _, err := runFeature(t, "new", "--title=JSON Feature", "--format=json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json unmarshal: %v\noutput=%q", err, out)
	}
	if result["path"] != "json-feature" {
		t.Errorf("path = %v, want json-feature", result["path"])
	}
}

func TestFeatureNew_TextOutput(t *testing.T) {
	setupFeatureSpec(t, "Draft")

	out, _, err := runFeature(t, "new", "--title=Text Feature", "--format=text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Feature: text-feature") {
		t.Errorf("stdout = %q, want it to contain 'Feature: text-feature'", out)
	}
	if !strings.Contains(out, "Status:  Draft") {
		t.Errorf("stdout = %q, want it to contain 'Status:  Draft'", out)
	}
}

func TestFeatureNew_ParentNotFound(t *testing.T) {
	setupFeatureSpec(t, "Draft")

	_, _, err := runFeature(t, "new", "--title=Orphan", "--parent=nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent parent")
	}
	if got := exitCodeOfErr(err); got != exitcode.NotFound {
		t.Errorf("exit code = %d, want %d (NotFound)", got, exitcode.NotFound)
	}
}

func TestFeatureNew_DuplicateSlug(t *testing.T) {
	setupFeatureSpec(t, "Draft")

	// auth already exists
	_, _, err := runFeature(t, "new", "--title=Auth")
	if err == nil {
		t.Fatal("expected error for duplicate feature slug")
	}
	if got := exitCodeOfErr(err); got != exitcode.InvalidState {
		t.Errorf("exit code = %d, want %d (InvalidState)", got, exitcode.InvalidState)
	}
}
