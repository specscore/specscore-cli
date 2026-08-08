package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// park requires --reason: rejected (exit 2) before any mutation.
func TestFeaturePark_MissingReasonRejected_CLI(t *testing.T) {
	root := setupFeatureSpec(t, "Approved")
	path := filepath.Join(root, "spec", "features", "auth", "README.md")
	before, _ := os.ReadFile(path)

	_, _, err := runFeature(t, "park", "auth", "--reason", "")
	if exitCodeOfErr(err) != 2 {
		t.Fatalf("expected exit 2, got %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Errorf("file mutated on rejected park")
	}
}

// Parking a Feature does not alter **Status:**, whatever it is — including
// Approved, which is deliberately NOT "queued toward Implementing" once
// parked.
func TestFeaturePark_HappyPath_CLI(t *testing.T) {
	root := setupFeatureSpec(t, "Approved")

	stdout, stderr, err := runFeature(t, "park", "auth", "--reason", "ratified, not v1")
	if err != nil {
		t.Fatalf("park: %v (stderr=%s)", err, stderr)
	}
	if strings.TrimSpace(stdout) != "auth: parked" {
		t.Errorf("stdout = %q, want %q", stdout, "auth: parked")
	}

	body, err := os.ReadFile(filepath.Join(root, "spec", "features", "auth", "README.md"))
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
	if !strings.Contains(s, "**Parked Reason:** ratified, not v1") {
		t.Errorf("missing **Parked Reason:**:\n%s", s)
	}
	if !strings.Contains(s, "**Parked Date:** ") {
		t.Errorf("missing **Parked Date:**:\n%s", s)
	}
}

// A Draft Feature can be parked too — parking is not gated on Status.
func TestFeaturePark_DraftCanBeParked_CLI(t *testing.T) {
	setupFeatureSpec(t, "Draft")
	if _, stderr, err := runFeature(t, "park", "auth", "--reason", "still fleshing this out"); err != nil {
		t.Fatalf("park: %v (stderr=%s)", err, stderr)
	}
}

// Round-trip: park then unpark restores the README byte-for-byte.
func TestFeatureParkUnpark_RoundTrips_CLI(t *testing.T) {
	root := setupFeatureSpec(t, "Approved")
	path := filepath.Join(root, "spec", "features", "auth", "README.md")
	before, _ := os.ReadFile(path)

	if _, stderr, err := runFeature(t, "park", "auth", "--reason", "not v1"); err != nil {
		t.Fatalf("park: %v (stderr=%s)", err, stderr)
	}
	stdout, stderr, err := runFeature(t, "unpark", "auth")
	if err != nil {
		t.Fatalf("unpark: %v (stderr=%s)", err, stderr)
	}
	if strings.TrimSpace(stdout) != "auth: unparked" {
		t.Errorf("stdout = %q, want %q", stdout, "auth: unparked")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("round-trip mismatch:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// Unparking a Feature that was never parked is rejected (exit 4).
func TestFeatureUnpark_NotParkedRejected_CLI(t *testing.T) {
	setupFeatureSpec(t, "Approved")
	_, _, err := runFeature(t, "unpark", "auth")
	if exitCodeOfErr(err) != 4 {
		t.Fatalf("expected exit 4, got %v", err)
	}
}

// A missing feature_id is a NotFound (exit 3) for both verbs.
func TestFeatureParkUnpark_NotFound_CLI(t *testing.T) {
	setupFeatureSpec(t, "Approved")
	for _, args := range [][]string{
		{"park", "ghost", "--reason", "x"},
		{"unpark", "ghost"},
	} {
		_, _, err := runFeature(t, args...)
		if exitCodeOfErr(err) != 3 {
			t.Errorf("%v: expected exit 3, got %v", args, err)
		}
	}
}

// The `feature list --parked` filter finds parked features and excludes
// unparked ones. Default listing (no --parked) still shows both — parked
// is a scheduling filter, not a visibility axis.
func TestFeatureList_ParkedFilter_CLI(t *testing.T) {
	root := setupFeatureSpec(t, "Approved")
	// Add a second, unparked feature alongside "auth".
	if err := os.MkdirAll(filepath.Join(root, "spec", "features", "billing"), 0o755); err != nil {
		t.Fatal(err)
	}
	billing := "# Feature: Billing\n\n**Status:** Draft\n**Source Ideas:** —\n\n## Summary\n\nx\n\n" +
		"## Open Questions\n\nNone at this time.\n\n---\n*This document follows the https://specscore.md/feature-specification*\n"
	if err := os.WriteFile(filepath.Join(root, "spec", "features", "billing", "README.md"), []byte(billing), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, _ := os.ReadFile(filepath.Join(root, "spec", "features", "README.md"))
	patched := strings.Replace(string(idx),
		"| [auth](auth/README.md) | Approved | Command | desc-auth |\n",
		"| [auth](auth/README.md) | Approved | Command | desc-auth |\n| [billing](billing/README.md) | Draft | Command | desc-billing |\n",
		1)
	if err := os.WriteFile(filepath.Join(root, "spec", "features", "README.md"), []byte(patched), 0o644); err != nil {
		t.Fatal(err)
	}
	migrateTree(t, root)

	if _, stderr, err := runFeature(t, "park", "auth", "--reason", "not v1"); err != nil {
		t.Fatalf("park: %v (stderr=%s)", err, stderr)
	}

	stdout, stderr, err := runFeature(t, "list", "--parked")
	if err != nil {
		t.Fatalf("list --parked: %v (stderr=%s)", err, stderr)
	}
	lines := nonEmptyLines(stdout)
	if len(lines) != 1 || lines[0] != "auth" {
		t.Fatalf("expected only [auth], got %v", lines)
	}

	stdoutAll, _, err := runFeature(t, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if allLines := nonEmptyLines(stdoutAll); len(allLines) != 2 {
		t.Fatalf("expected both features in default listing, got %v", allLines)
	}
}

// `feature list --fields=parked` surfaces the Parked bool via --format=yaml.
func TestFeatureList_ParkedField_CLI(t *testing.T) {
	setupFeatureSpec(t, "Approved")
	if _, stderr, err := runFeature(t, "park", "auth", "--reason", "not v1"); err != nil {
		t.Fatalf("park: %v (stderr=%s)", err, stderr)
	}
	stdout, _, err := runFeature(t, "list", "--fields", "parked")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(stdout, "parked: true") {
		t.Errorf("expected parked: true in output, got:\n%s", stdout)
	}
}
