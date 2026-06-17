package sidekick

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// seedFixture writes a queued seed at spec/ideas/seeds/<slug>.md under a fresh
// temp specRoot and returns (specRoot, seedPath).
func seedFixture(t *testing.T, slug, body string) (string, string) {
	t.Helper()
	specRoot := t.TempDir()
	seedsDir := filepath.Join(specRoot, "spec", "ideas", "seeds")
	if err := os.MkdirAll(seedsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	seedPath := filepath.Join(seedsDir, slug+".md")
	if err := os.WriteFile(seedPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return specRoot, seedPath
}

const queuedSeedBody = "---\ncaptured_by: user\nstatus: queued\n---\n# A sideline idea\n\nSome context.\n"

func TestRelocate_happy_rewritesMovesAndTags(t *testing.T) {
	// AC: implemented-relocate-and-tag (terminal-relocation).
	specRoot, seedPath := seedFixture(t, "foo", queuedSeedBody)

	if err := Relocate(RelocateOptions{
		SpecRoot:  specRoot,
		Slug:      "foo",
		SeedPath:  seedPath,
		NewStatus: SeedImplemented,
	}); err != nil {
		t.Fatalf("Relocate() = %v; want nil", err)
	}

	if _, err := os.Stat(seedPath); !os.IsNotExist(err) {
		t.Errorf("seed still present at seeds/ (err=%v); want gone", err)
	}
	archived := filepath.Join(specRoot, "spec", "ideas", "archived", "foo.md")
	got, err := os.ReadFile(archived)
	if err != nil {
		t.Fatalf("reading archived seed: %v", err)
	}
	s := string(got)
	if !strings.Contains(s, "status: Implemented") {
		t.Errorf("archived seed missing rewritten status; got:\n%s", s)
	}
	if !strings.Contains(s, "type: sidekick-seed") {
		t.Errorf("archived seed missing type tag; got:\n%s", s)
	}
	if strings.Contains(s, "status: queued") {
		t.Errorf("archived seed still carries queued status; got:\n%s", s)
	}
	// Body preserved.
	if !strings.Contains(s, "# A sideline idea") || !strings.Contains(s, "Some context.") {
		t.Errorf("archived seed body not preserved; got:\n%s", s)
	}
}

func TestRelocate_collision_exit1_sourceUntouched(t *testing.T) {
	// AC: archive-path-collision.
	specRoot, seedPath := seedFixture(t, "foo", queuedSeedBody)
	archivedDir := filepath.Join(specRoot, "spec", "ideas", "archived")
	if err := os.MkdirAll(archivedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	archived := filepath.Join(archivedDir, "foo.md")
	const existing = "PRE-EXISTING CONTENT\n"
	if err := os.WriteFile(archived, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	srcBefore, _ := os.ReadFile(seedPath)

	err := Relocate(RelocateOptions{
		SpecRoot:  specRoot,
		Slug:      "foo",
		SeedPath:  seedPath,
		NewStatus: SeedImplemented,
	})
	if err == nil {
		t.Fatal("Relocate() = nil; want exit-1 Conflict")
	}
	if got := exitCodeOf(t, err); got != exitcode.Conflict {
		t.Errorf("exit code = %d; want %d (Conflict)", got, exitcode.Conflict)
	}

	// Source seed byte-identical (status: queued, no type).
	srcAfter, readErr := os.ReadFile(seedPath)
	if readErr != nil {
		t.Fatalf("source seed gone after collision: %v", readErr)
	}
	if string(srcAfter) != string(srcBefore) {
		t.Errorf("source seed mutated after collision:\nbefore:\n%s\nafter:\n%s", srcBefore, srcAfter)
	}
	if strings.Contains(string(srcAfter), "type:") {
		t.Errorf("source seed gained a type key after collision; got:\n%s", srcAfter)
	}

	// Archived file untouched.
	archAfter, _ := os.ReadFile(archived)
	if string(archAfter) != existing {
		t.Errorf("pre-existing archived file mutated; got %q want %q", archAfter, existing)
	}
}

func TestRelocate_postMoveFailure_fullRollback(t *testing.T) {
	// AC: rollback-includes-relocation — an injected post-move failure rolls
	// back to the original seeds/ state (original status, no type, no note).
	specRoot, seedPath := seedFixture(t, "foo", queuedSeedBody)
	srcBefore, _ := os.ReadFile(seedPath)

	// Simulate a caller-applied body note: the RollbackHook must undo it.
	hookCalled := false
	rollbackHook := func() error {
		hookCalled = true
		return os.WriteFile(seedPath, srcBefore, 0o644) // restore body verbatim
	}

	boom := errors.New("injected lint failure")
	err := Relocate(RelocateOptions{
		SpecRoot:      specRoot,
		Slug:          "foo",
		SeedPath:      seedPath,
		NewStatus:     SeedImplemented,
		RollbackHook:  rollbackHook,
		afterMoveHook: func() error { return boom },
	})
	if err == nil {
		t.Fatal("Relocate() = nil; want post-move failure error")
	}
	if got := exitCodeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit code = %d; want %d (Unexpected)", got, exitcode.Unexpected)
	}
	if !hookCalled {
		t.Error("RollbackHook not invoked during rollback")
	}

	// File restored to seeds/ in its exact original form.
	if _, statErr := os.Stat(seedPath); statErr != nil {
		t.Fatalf("seed not restored to seeds/: %v", statErr)
	}
	archived := filepath.Join(specRoot, "spec", "ideas", "archived", "foo.md")
	if _, statErr := os.Stat(archived); !os.IsNotExist(statErr) {
		t.Errorf("archived file lingers after rollback (err=%v); want gone", statErr)
	}
	after, _ := os.ReadFile(seedPath)
	if string(after) != string(srcBefore) {
		t.Errorf("seed not restored byte-for-byte:\nbefore:\n%s\nafter:\n%s", srcBefore, after)
	}
	if strings.Contains(string(after), "type:") {
		t.Errorf("restored seed still carries a type key; got:\n%s", after)
	}
	if strings.Contains(string(after), "## Resolution") {
		t.Errorf("restored seed still carries a Resolution section; got:\n%s", after)
	}
}

func TestAddSidekickSeedType_idempotentRewrite(t *testing.T) {
	_, seedPath := seedFixture(t, "foo",
		"---\ntype: stale\ncaptured_by: user\nstatus: queued\n---\n# h\n")
	if err := addSidekickSeedType(seedPath); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(seedPath)
	if c := strings.Count(string(got), "type:"); c != 1 {
		t.Errorf("type: appears %d times; want 1\n%s", c, got)
	}
	if !strings.Contains(string(got), "type: sidekick-seed") {
		t.Errorf("existing type key not rewritten; got:\n%s", got)
	}
}
