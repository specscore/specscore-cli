package cli

// Tests for the read-only `<kind> transitions [<id>]` query verb — the
// founder's second ask: "a command/flag that shows list of available
// transitions from current status" plus a full bidirectional (previous/next)
// view of the status vocabulary. Purely additive; never mutates.

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFeatureTransitions_NoArgPrintsFullMatrix(t *testing.T) {
	setupFeatureSpec(t, "Draft")
	stdout, stderr, err := runFeature(t, "transitions")
	if err != nil {
		t.Fatalf("transitions: %v\nstderr=%s", err, stderr)
	}
	for _, want := range []string{"Draft", "Approved", "Rejected", "Deprecated", "previous:", "next:", "terminal status"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
}

func TestFeatureTransitions_WithID_ReportsCurrentStatusAndNext(t *testing.T) {
	setupFeatureSpec(t, "Approved")
	stdout, stderr, err := runFeature(t, "transitions", "auth")
	if err != nil {
		t.Fatalf("transitions auth: %v\nstderr=%s", err, stderr)
	}
	if !strings.HasPrefix(stdout, "auth: Approved\n") {
		t.Errorf("stdout = %q, want it to start with %q", stdout, "auth: Approved\n")
	}
	// Approved's legal next set per the Feature matrix.
	for _, want := range []string{"Amending", "Deprecated", "Implementing", "Rejected"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing legal next status %q:\n%s", want, stdout)
		}
	}
}

func TestFeatureTransitions_UnknownID_Exit3(t *testing.T) {
	setupFeatureSpec(t, "Draft")
	_, _, err := runFeature(t, "transitions", "does-not-exist")
	if err == nil {
		t.Fatal("expected an error for an unknown feature_id")
	}
	if got := exitCodeOfErr(err); got != 3 {
		t.Errorf("exit code = %d, want 3 (NotFound)", got)
	}
}

func TestFeatureTransitions_JSONFormat(t *testing.T) {
	setupFeatureSpec(t, "Approved")
	stdout, stderr, err := runFeature(t, "transitions", "auth", "--format=json")
	if err != nil {
		t.Fatalf("transitions --format=json: %v\nstderr=%s", err, stderr)
	}
	var got struct {
		ID       string   `json:"id"`
		Status   string   `json:"status"`
		Previous []string `json:"previous"`
		Next     []string `json:"next"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("invalid JSON: %v\noutput=%s", err, stdout)
	}
	if got.ID != "auth" || got.Status != "Approved" {
		t.Errorf("decoded = %+v, want id=auth status=Approved", got)
	}
	if len(got.Next) == 0 {
		t.Errorf("decoded Next is empty: %+v", got)
	}
}

func TestFeatureTransitions_DoesNotMutateTheArtifact(t *testing.T) {
	root := setupFeatureSpec(t, "Approved")
	before := snapshotDir(t, root)
	if _, _, err := runFeature(t, "transitions", "auth"); err != nil {
		t.Fatalf("transitions: %v", err)
	}
	after := snapshotDir(t, root)
	if diff := diffSnapshotPaths(before, after); len(diff) != 0 {
		t.Fatalf("transitions (read-only query) changed files: %v", diff)
	}
}

func TestPlanTransitions_WithID(t *testing.T) {
	stagePlan(t, "auth", "Draft")
	stdout, stderr, err := runPlan(t, "transitions", "auth")
	if err != nil {
		t.Fatalf("transitions auth: %v\nstderr=%s", err, stderr)
	}
	if !strings.HasPrefix(stdout, "auth: Draft\n") {
		t.Errorf("stdout = %q, want prefix %q", stdout, "auth: Draft\n")
	}
}

func TestIssueTransitions_NoArgAndWithSlug(t *testing.T) {
	root := setupIssueSpecRoot(t)
	withCwd(t, root)
	if _, _, err := runIssue(t, "new", "timeout-bug", "--severity=high"); err != nil {
		t.Fatalf("issue new: %v", err)
	}

	matrixOut, _, err := runIssue(t, "transitions")
	if err != nil {
		t.Fatalf("issue transitions: %v", err)
	}
	for _, want := range []string{"open", "investigating", "resolved", "rejected"} {
		if !strings.Contains(matrixOut, want) {
			t.Errorf("matrix output missing %q:\n%s", want, matrixOut)
		}
	}

	single, _, err := runIssue(t, "transitions", "timeout-bug")
	if err != nil {
		t.Fatalf("issue transitions timeout-bug: %v", err)
	}
	if !strings.HasPrefix(single, "timeout-bug: open\n") {
		t.Errorf("stdout = %q, want prefix %q", single, "timeout-bug: open\n")
	}
}

func TestSidekickTransitions_NoArgAndWithSlug(t *testing.T) {
	stageQueuedSeed(t, "foo")
	matrixOut, _, err := runSidekick(t, "transitions")
	if err != nil {
		t.Fatalf("sidekick transitions: %v", err)
	}
	if !strings.Contains(matrixOut, "Queued") || !strings.Contains(matrixOut, "Implemented") {
		t.Errorf("matrix output missing expected statuses:\n%s", matrixOut)
	}

	single, _, err := runSidekick(t, "transitions", "foo")
	if err != nil {
		t.Fatalf("sidekick transitions foo: %v", err)
	}
	if !strings.HasPrefix(single, "foo: Queued\n") {
		t.Errorf("stdout = %q, want prefix %q", single, "foo: Queued\n")
	}
}

func TestTransitions_InvalidFormatRejected(t *testing.T) {
	setupFeatureSpec(t, "Draft")
	_, _, err := runFeature(t, "transitions", "--format=xml")
	if err == nil {
		t.Fatal("expected an error for an unsupported --format value")
	}
	if got := exitCodeOfErr(err); got != 2 {
		t.Errorf("exit code = %d, want 2 (InvalidArgs)", got)
	}
}
