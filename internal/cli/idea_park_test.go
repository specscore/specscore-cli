package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// park requires --reason: a missing/empty reason is rejected (exit 2)
// BEFORE any mutation.
func TestIdeaPark_MissingReasonRejected_CLI(t *testing.T) {
	root := stageActiveIdea(t, "foo", "Approved", "")
	path := filepath.Join(root, "spec", "ideas", "foo.md")
	before, _ := os.ReadFile(path)

	_, _, err := runIdea(t, "park", "foo", "--reason", "")
	if exitCodeOfErr(err) != 2 {
		t.Fatalf("expected exit 2, got %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Errorf("file mutated on rejected park")
	}
}

// Parking does not alter **Status:** and writes the reason/date pair.
func TestIdeaPark_HappyPath_CLI(t *testing.T) {
	root := stageActiveIdea(t, "foo", "Approved", "")

	stdout, stderr, err := runIdea(t, "park", "foo", "--reason", "good idea, not v1")
	if err != nil {
		t.Fatalf("park: %v (stderr=%s)", err, stderr)
	}
	if strings.TrimSpace(stdout) != "foo: parked" {
		t.Errorf("stdout = %q, want %q", stdout, "foo: parked")
	}

	body, err := os.ReadFile(filepath.Join(root, "spec", "ideas", "foo.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	s := string(body)
	if !strings.Contains(s, "**Status:** Approved") {
		t.Errorf("Status must be preserved:\n%s", s)
	}
	if !strings.Contains(s, "**Parked:** true") {
		t.Errorf("missing **Parked:** true:\n%s", s)
	}
	if !strings.Contains(s, "**Parked Reason:** good idea, not v1") {
		t.Errorf("missing **Parked Reason:**:\n%s", s)
	}
	if !strings.Contains(s, "**Parked Date:** ") {
		t.Errorf("missing **Parked Date:**:\n%s", s)
	}
}

// Unpark round-trips: park then unpark restores the file to its pre-park
// state (Status untouched throughout, no Parked* lines left behind).
func TestIdeaParkUnpark_RoundTrips_CLI(t *testing.T) {
	root := stageActiveIdea(t, "foo", "Draft", "")
	path := filepath.Join(root, "spec", "ideas", "foo.md")
	before, _ := os.ReadFile(path)

	if _, stderr, err := runIdea(t, "park", "foo", "--reason", "not now"); err != nil {
		t.Fatalf("park: %v (stderr=%s)", err, stderr)
	}
	stdout, stderr, err := runIdea(t, "unpark", "foo")
	if err != nil {
		t.Fatalf("unpark: %v (stderr=%s)", err, stderr)
	}
	if strings.TrimSpace(stdout) != "foo: unparked" {
		t.Errorf("stdout = %q, want %q", stdout, "foo: unparked")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("round-trip mismatch:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// Unparking an idea that was never parked is rejected (exit 4) rather than
// silently succeeding.
func TestIdeaUnpark_NotParkedRejected_CLI(t *testing.T) {
	stageActiveIdea(t, "foo", "Draft", "")
	_, _, err := runIdea(t, "unpark", "foo")
	if exitCodeOfErr(err) != 4 {
		t.Fatalf("expected exit 4, got %v", err)
	}
}

// A missing idea is a NotFound (exit 3) for both verbs.
func TestIdeaParkUnpark_NotFound_CLI(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	for _, args := range [][]string{
		{"park", "ghost", "--reason", "x"},
		{"unpark", "ghost"},
	} {
		_, _, err := runIdea(t, args...)
		if exitCodeOfErr(err) != 3 {
			t.Errorf("%v: expected exit 3, got %v", args, err)
		}
	}
}

// An invalid slug is rejected (exit 2) before any filesystem work.
func TestIdeaParkUnpark_InvalidSlug_CLI(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	for _, args := range [][]string{
		{"park", "Bad_Slug", "--reason", "x"},
		{"unpark", "Bad_Slug"},
	} {
		_, _, err := runIdea(t, args...)
		if exitCodeOfErr(err) != 2 {
			t.Errorf("%v: expected exit 2, got %v", args, err)
		}
	}
}

// Re-parking overwrites the reason rather than duplicating the block.
func TestIdeaPark_ReParkOverwritesReason_CLI(t *testing.T) {
	root := stageActiveIdea(t, "foo", "Approved", "")
	if _, _, err := runIdea(t, "park", "foo", "--reason", "first reason"); err != nil {
		t.Fatalf("first park: %v", err)
	}
	if _, _, err := runIdea(t, "park", "foo", "--reason", "second reason"); err != nil {
		t.Fatalf("second park: %v", err)
	}
	body, _ := os.ReadFile(filepath.Join(root, "spec", "ideas", "foo.md"))
	s := string(body)
	if strings.Count(s, "**Parked:**") != 1 {
		t.Errorf("expected exactly one **Parked:** line, got:\n%s", s)
	}
	if strings.Contains(s, "first reason") || !strings.Contains(s, "second reason") {
		t.Errorf("expected reason overwritten to 'second reason', got:\n%s", s)
	}
}

// The `idea list --parked` filter finds parked ideas and excludes unparked
// ones.
func TestIdeaList_ParkedFilter_CLI(t *testing.T) {
	root := stageActiveIdea(t, "kept-active", "Draft", "")
	withCwd(t, root)
	if _, _, err := runIdea(t, "new", "deferred", "--owner", "tester"); err != nil {
		t.Fatalf("idea new: %v", err)
	}
	if _, _, err := runIdea(t, "park", "deferred", "--reason", "not v1"); err != nil {
		t.Fatalf("park: %v", err)
	}

	stdout, stderr, err := runIdea(t, "list", "--parked")
	if err != nil {
		t.Fatalf("list --parked: %v (stderr=%s)", err, stderr)
	}
	lines := nonEmptyLines(stdout)
	if len(lines) != 1 || lines[0] != "deferred" {
		t.Fatalf("expected only [deferred], got %v", lines)
	}

	// Default listing (no --parked) still shows BOTH — parked is not a
	// visibility axis like archived.
	stdoutAll, _, err := runIdea(t, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	allLines := nonEmptyLines(stdoutAll)
	if len(allLines) != 2 {
		t.Fatalf("expected both ideas in default listing, got %v", allLines)
	}
}

// yaml output surfaces the parked field.
func TestIdeaList_ParkedFieldInYAML_CLI(t *testing.T) {
	root := stageActiveIdea(t, "foo", "Approved", "")
	withCwd(t, root)
	if _, _, err := runIdea(t, "park", "foo", "--reason", "not v1"); err != nil {
		t.Fatalf("park: %v", err)
	}
	stdout, _, err := runIdea(t, "list", "--format", "yaml")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(stdout, "parked: true") {
		t.Errorf("expected parked: true in yaml output, got:\n%s", stdout)
	}
}
