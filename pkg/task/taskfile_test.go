package task

import (
	"strings"
	"testing"
)

func TestTaskFileRoundTrip(t *testing.T) {
	original := TaskFileData{
		Title:       "Implement API endpoints",
		Description: "Description paragraph here.",
		DependsOn:   []string{"setup-db", "define-schema"},
		Summary:     "Some summary text",
	}

	rendered := RenderTaskFile(original)
	parsed, err := ParseTaskFile(rendered)
	if err != nil {
		t.Fatalf("round-trip parse failed: %v", err)
	}

	if parsed.Title != original.Title {
		t.Errorf("title: got %q, want %q", parsed.Title, original.Title)
	}
	if parsed.Description != original.Description {
		t.Errorf("description: got %q, want %q", parsed.Description, original.Description)
	}
	if len(parsed.DependsOn) != len(original.DependsOn) {
		t.Fatalf("deps length: got %d, want %d", len(parsed.DependsOn), len(original.DependsOn))
	}
	for i, dep := range parsed.DependsOn {
		if dep != original.DependsOn[i] {
			t.Errorf("dep[%d]: got %q, want %q", i, dep, original.DependsOn[i])
		}
	}
	if parsed.Summary != original.Summary {
		t.Errorf("summary: got %q, want %q", parsed.Summary, original.Summary)
	}
}

// TestTaskFileRoundTrip_WithStatus proves a **Status:** line survives a
// render/parse round trip and is written immediately after the title (the
// exact gap `specscore task change-status` needs closed — see
// pkg/lint.migrateTaskBoardStatus and internal/cli.runTaskNew).
func TestTaskFileRoundTrip_WithStatus(t *testing.T) {
	original := TaskFileData{
		Title:       "Implement API endpoints",
		Status:      "planning",
		Description: "Description paragraph here.",
		DependsOn:   []string{"setup-db"},
		Summary:     "Some summary text",
	}

	rendered := RenderTaskFile(original)
	if !strings.Contains(string(rendered), "# Implement API endpoints\n\n**Status:** planning\n") {
		t.Fatalf("rendered file does not carry **Status:** immediately after the title:\n%s", rendered)
	}

	parsed, err := ParseTaskFile(rendered)
	if err != nil {
		t.Fatalf("round-trip parse failed: %v", err)
	}
	if parsed.Status != original.Status {
		t.Errorf("status: got %q, want %q", parsed.Status, original.Status)
	}
	if parsed.Description != original.Description {
		t.Errorf("description: got %q, want %q", parsed.Description, original.Description)
	}
}

// TestTaskFileRoundTrip_WithStatusNoDescription covers a task with a Status
// line but no description paragraph, so the Status line's trailing blank line
// is not mistaken for description content.
func TestTaskFileRoundTrip_WithStatusNoDescription(t *testing.T) {
	original := TaskFileData{Title: "Bare task", Status: "queued"}
	rendered := RenderTaskFile(original)
	parsed, err := ParseTaskFile(rendered)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if parsed.Status != "queued" {
		t.Errorf("status: got %q, want %q", parsed.Status, "queued")
	}
	if parsed.Description != "" {
		t.Errorf("description: got %q, want empty", parsed.Description)
	}
}

// TestTaskFileParse_NoStatusLineIsBackwardCompatible proves a pre-existing
// task README with no **Status:** line at all (every board written before
// this fix, or any hand-authored file) still parses cleanly, with Status
// reported as "" rather than an error — this is the exact shape that made
// `task change-status` exit 10 on an existing board.
func TestTaskFileParse_NoStatusLineIsBackwardCompatible(t *testing.T) {
	md := "# My Task\n\nSome description.\n\n## Dependencies\n\nNone\n\n## Summary\n\nNone\n"
	parsed, err := ParseTaskFile([]byte(md))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if parsed.Status != "" {
		t.Errorf("status: got %q, want empty (no **Status:** line present)", parsed.Status)
	}
	if parsed.Description != "Some description." {
		t.Errorf("description: got %q, want %q", parsed.Description, "Some description.")
	}
}

func TestTaskFileParseNoneDepsAndSummary(t *testing.T) {
	md := "# My Task\n\nSome description.\n\n## Dependencies\n\nNone\n\n## Summary\n\nNone\n"
	parsed, err := ParseTaskFile([]byte(md))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if parsed.Title != "My Task" {
		t.Errorf("title: got %q", parsed.Title)
	}
	if len(parsed.DependsOn) != 0 {
		t.Errorf("expected no deps, got %v", parsed.DependsOn)
	}
	if parsed.Summary != "" {
		t.Errorf("expected empty summary, got %q", parsed.Summary)
	}
}

func TestTaskFileParseMultipleDeps(t *testing.T) {
	md := "# Task\n\nDesc.\n\n## Dependencies\n\n- alpha\n- bravo\n- charlie\n\n## Summary\n\nNone\n"
	parsed, err := ParseTaskFile([]byte(md))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	want := []string{"alpha", "bravo", "charlie"}
	if len(parsed.DependsOn) != len(want) {
		t.Fatalf("deps length: got %d, want %d", len(parsed.DependsOn), len(want))
	}
	for i, dep := range parsed.DependsOn {
		if dep != want[i] {
			t.Errorf("dep[%d]: got %q, want %q", i, dep, want[i])
		}
	}
}

func TestTaskFileParseMultiParagraphDescription(t *testing.T) {
	md := "# Task\n\nFirst paragraph.\n\nSecond paragraph.\n\n## Dependencies\n\nNone\n\n## Summary\n\nNone\n"
	parsed, err := ParseTaskFile([]byte(md))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if !strings.Contains(parsed.Description, "First paragraph.") {
		t.Errorf("description missing first paragraph: %q", parsed.Description)
	}
	if !strings.Contains(parsed.Description, "Second paragraph.") {
		t.Errorf("description missing second paragraph: %q", parsed.Description)
	}
}

func TestTaskFileRenderEmptyDescription(t *testing.T) {
	d := TaskFileData{
		Title:     "No Desc",
		DependsOn: []string{"dep-a"},
		Summary:   "Done",
	}
	rendered := string(RenderTaskFile(d))
	if !strings.HasPrefix(rendered, "# No Desc\n") {
		t.Errorf("unexpected start: %q", rendered[:40])
	}
	if strings.Contains(rendered, "\n\n\n") {
		t.Errorf("should not have triple newline for empty description")
	}
}

func TestTaskFileRenderNoDeps(t *testing.T) {
	d := TaskFileData{
		Title:       "Simple",
		Description: "A task.",
	}
	rendered := string(RenderTaskFile(d))
	if !strings.Contains(rendered, "## Dependencies\n\nNone\n") {
		t.Errorf("expected 'None' under Dependencies, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "## Summary\n\nNone\n") {
		t.Errorf("expected 'None' under Summary, got:\n%s", rendered)
	}
}

func TestTaskFileParseErrorMissingTitle(t *testing.T) {
	md := "No title here\n\n## Dependencies\n\nNone\n\n## Summary\n\nNone\n"
	_, err := ParseTaskFile([]byte(md))
	if err == nil {
		t.Fatal("expected error for missing title")
	}
	if !strings.Contains(err.Error(), "title") {
		t.Errorf("error should mention title: %v", err)
	}
}

func TestTaskFileParseErrorMissingDeps(t *testing.T) {
	md := "# Task\n\nDesc.\n\n## Summary\n\nNone\n"
	_, err := ParseTaskFile([]byte(md))
	if err == nil {
		t.Fatal("expected error for missing Dependencies section")
	}
	if !strings.Contains(err.Error(), "Dependencies") {
		t.Errorf("error should mention Dependencies: %v", err)
	}
}

func TestTaskFileParseErrorMissingSummary(t *testing.T) {
	md := "# Task\n\nDesc.\n\n## Dependencies\n\nNone\n"
	_, err := ParseTaskFile([]byte(md))
	if err == nil {
		t.Fatal("expected error for missing Summary section")
	}
	if !strings.Contains(err.Error(), "Summary") {
		t.Errorf("error should mention Summary: %v", err)
	}
}
