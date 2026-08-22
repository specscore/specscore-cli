package idea

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

// These tests cover the two halves of the self-concealing transition failure:
//
//   - the derived-band precondition, which refuses a transition that could
//     never survive the post-mutation index sync, BEFORE writing anything; and
//   - the persistence check, which refuses to report success when the
//     post-mutation hook left a different status on disk.
//
// Before both existed, `idea change-status <slug> --to=Specifying` printed
// `<slug>: Approved → Specifying` and exited 0 over a byte-identical file.

// patchIdeaHeader rewrites a single `**Field:** value` header line in the
// staged Idea, so a test can stage promotion/type facts the scaffold does not
// take as options.
func patchIdeaHeader(t *testing.T, root, slug, field, value string) {
	t.Helper()
	path := filepath.Join(root, "spec", "ideas", slug+".md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read idea: %v", err)
	}
	lines := strings.Split(string(raw), "\n")
	prefix := "**" + field + ":**"
	found := false
	for i, line := range lines {
		if strings.HasPrefix(line, prefix) {
			lines[i] = prefix + " " + value
			found = true
			break
		}
	}
	if !found {
		// Absent optional field (e.g. **Type:**) — insert it into the header
		// block, directly after the **Status:** line.
		for i, line := range lines {
			if strings.HasPrefix(line, "**Status:**") {
				lines = append(lines[:i+1], append([]string{prefix + " " + value}, lines[i+1:]...)...)
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatalf("idea %s has no header block to patch:\n%s", slug, raw)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatalf("write idea: %v", err)
	}
}

// revertingHook simulates the real post-mutation surface that caused the bug:
// `spec lint --fix` runs, reports no error, and rewrites the very status line
// the transition just wrote.
func revertingHook(t *testing.T, root, slug, statusToWrite string) PostMutationHook {
	t.Helper()
	return func() error {
		path := filepath.Join(root, "spec", "ideas", slug+".md")
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines := strings.Split(string(raw), "\n")
		for i, line := range lines {
			if strings.HasPrefix(line, "**Status:**") {
				lines[i] = "**Status:** " + statusToWrite
			}
			if strings.HasPrefix(line, "status:") {
				lines[i] = "status: " + statusToWrite
			}
		}
		return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
	}
}

// The reported bug, at the package boundary: no Feature promotes the Idea, so
// `Specifying` cannot survive the index sync. Refuse it up front (exit 4) and
// leave the file untouched.
func TestChangeStatus_DerivedBandRefusedWithoutPromotion(t *testing.T) {
	root := stageIdeaTree(t, "unpromoted", "Approved")
	before := readIdea(t, root, "unpromoted")

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot:     root,
		Slug:         "unpromoted",
		To:           lifecycle.IdeaSpecifying,
		PostMutation: noopLint,
	})
	assertExitCode(t, err, exitcode.InvalidState)
	if !strings.Contains(err.Error(), "Promotes To") {
		t.Errorf("message does not name the empty field: %v", err)
	}
	if got := readIdea(t, root, "unpromoted"); got != before {
		t.Errorf("refused transition still wrote to the file:\n%s", got)
	}
}

// With a promoting Feature named in **Promotes To:**, the same transition is
// allowed through — the precondition gates on promotion, not on the status.
func TestChangeStatus_DerivedBandAllowedWithPromotion(t *testing.T) {
	root := stageIdeaTree(t, "promoted", "Approved")
	patchIdeaHeader(t, root, "promoted", "Promotes To", "demo")

	result, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot:     root,
		Slug:         "promoted",
		To:           lifecycle.IdeaSpecifying,
		PostMutation: noopLint,
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	if result.To != lifecycle.IdeaSpecifying {
		t.Errorf("result.To = %q; want Specifying", result.To)
	}
	if body := readIdea(t, root, "promoted"); !strings.Contains(body, "**Status:** Specifying") {
		t.Errorf("status not persisted:\n%s", body)
	}
}

// Change-request Ideas are author-managed — the derivation rules skip them, so
// the precondition must skip them too.
func TestChangeStatus_DerivedBandSkippedForChangeRequest(t *testing.T) {
	root := stageIdeaTree(t, "cr", "Approved")
	patchIdeaHeader(t, root, "cr", "Type", "change-request")

	if _, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot:     root,
		Slug:         "cr",
		To:           lifecycle.IdeaSpecifying,
		PostMutation: noopLint,
	}); err != nil {
		t.Fatalf("change-request transition refused: %v", err)
	}
	if body := readIdea(t, root, "cr"); !strings.Contains(body, "**Status:** Specifying") {
		t.Errorf("status not persisted:\n%s", body)
	}
}

// An artifact the precondition cannot parse is not a rejection: the state
// machine already read it, and the persistence check catches whatever the
// post-mutation hook leaves behind.
func TestChangeStatus_DerivedBandSkippedWhenUnparseable(t *testing.T) {
	root := stageIdeaTree(t, "unparseable", "Approved")
	orig := parseIdeaFn
	parseIdeaFn = func(string) (*Idea, error) { return nil, fmt.Errorf("injected parse failure") }
	t.Cleanup(func() { parseIdeaFn = orig })

	if _, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot:     root,
		Slug:         "unparseable",
		To:           lifecycle.IdeaSpecifying,
		PostMutation: noopLint,
	}); err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	if body := readIdea(t, root, "unparseable"); !strings.Contains(body, "**Status:** Specifying") {
		t.Errorf("status not persisted:\n%s", body)
	}
}

// The general defect: the hook succeeds but rewrites the status. The verb MUST
// roll back and fail (exit 10) rather than return a success result.
func TestChangeStatus_PostMutationRevertFailsLoudly(t *testing.T) {
	root := stageIdeaTree(t, "reverted", "Draft")
	before := readIdea(t, root, "reverted")

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot:     root,
		Slug:         "reverted",
		To:           lifecycle.IdeaRejected,
		PostMutation: revertingHook(t, root, "reverted", "Draft"),
	})
	assertExitCode(t, err, exitcode.Unexpected)
	if !strings.Contains(err.Error(), "did not persist") {
		t.Errorf("message does not name the failure: %v", err)
	}
	if strings.Contains(err.Error(), "idea-sync-lint-strict") {
		t.Errorf("non-derived target should not carry the derivation hint: %v", err)
	}
	if got := readIdea(t, root, "reverted"); got != before {
		t.Errorf("file not restored after a failed transition:\n%s", got)
	}
}

// Same failure inside the derived band carries the actionable hint: advance
// the promoting Feature, not the Idea.
func TestChangeStatus_PostMutationRevertInDerivedBandExplainsDerivation(t *testing.T) {
	root := stageIdeaTree(t, "derived-revert", "Approved")
	patchIdeaHeader(t, root, "derived-revert", "Promotes To", "demo")

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot:     root,
		Slug:         "derived-revert",
		To:           lifecycle.IdeaSpecifying,
		PostMutation: revertingHook(t, root, "derived-revert", "Approved"),
	})
	assertExitCode(t, err, exitcode.Unexpected)
	for _, want := range []string{"did not persist", "idea-sync-lint-strict"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message %q does not mention %q", err.Error(), want)
		}
	}
	if body := readIdea(t, root, "derived-revert"); !strings.Contains(body, "**Status:** Approved") {
		t.Errorf("file not restored after a failed transition:\n%s", body)
	}
}
