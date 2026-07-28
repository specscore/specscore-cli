package lifecycle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeNoteFixture(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	return path
}

const footerLine = "*This document follows the https://specscore.md/idea-specification*"

// absent + footer present → insert the section immediately before the footer.
func TestAppendResolutionNote_CreateBeforeFooter(t *testing.T) {
	t.Parallel()
	body := "# Idea: Baz\n\n**Status:** Queued\n\nSome body text.\n\n" + footerLine + "\n"
	path := writeNoteFixture(t, body)

	orig, wrote, err := AppendResolutionNote(path, "Shipped in skills/implement step 4b")
	if err != nil {
		t.Fatalf("AppendResolutionNote: %v", err)
	}
	if !wrote {
		t.Fatal("expected wrote=true")
	}
	if string(orig) != body {
		t.Fatalf("original bytes not captured verbatim:\n%q", string(orig))
	}

	got := readFile(t, path)
	if !strings.Contains(got, "## Resolution") {
		t.Fatalf("missing ## Resolution section:\n%s", got)
	}
	if !strings.Contains(got, "Shipped in skills/implement step 4b") {
		t.Fatalf("missing note content:\n%s", got)
	}
	// Section must appear before the footer.
	resIdx := strings.Index(got, "## Resolution")
	footIdx := strings.Index(got, footerLine)
	if resIdx < 0 || footIdx < 0 || resIdx > footIdx {
		t.Fatalf("## Resolution must precede the footer:\n%s", got)
	}
}

// absent + no footer → append at EOF.
func TestAppendResolutionNote_CreateAtEOF(t *testing.T) {
	t.Parallel()
	body := "# Idea: Baz\n\n**Status:** Queued\n\nSome body text.\n"
	path := writeNoteFixture(t, body)

	_, wrote, err := AppendResolutionNote(path, "note at eof")
	if err != nil {
		t.Fatalf("AppendResolutionNote: %v", err)
	}
	if !wrote {
		t.Fatal("expected wrote=true")
	}
	got := readFile(t, path)
	if !strings.HasPrefix(got, body) {
		t.Fatalf("existing body not preserved as prefix:\n%s", got)
	}
	if !strings.Contains(got, "## Resolution") || !strings.Contains(got, "note at eof") {
		t.Fatalf("missing section/content at EOF:\n%s", got)
	}
}

// existing section → append a new trailing paragraph within it.
func TestAppendResolutionNote_AppendToExisting(t *testing.T) {
	t.Parallel()
	body := "# Idea: Baz\n\n**Status:** Queued\n\n## Resolution\n\nfirst reason\n\n" + footerLine + "\n"
	path := writeNoteFixture(t, body)

	_, wrote, err := AppendResolutionNote(path, "second reason")
	if err != nil {
		t.Fatalf("AppendResolutionNote: %v", err)
	}
	if !wrote {
		t.Fatal("expected wrote=true")
	}
	got := readFile(t, path)
	if strings.Count(got, "## Resolution") != 1 {
		t.Fatalf("section must not be duplicated:\n%s", got)
	}
	if !strings.Contains(got, "first reason") || !strings.Contains(got, "second reason") {
		t.Fatalf("both paragraphs must be present:\n%s", got)
	}
	// New paragraph appended after the first, still before the footer.
	firstIdx := strings.Index(got, "first reason")
	secondIdx := strings.Index(got, "second reason")
	footIdx := strings.Index(got, footerLine)
	if firstIdx >= secondIdx || secondIdx >= footIdx {
		t.Fatalf("new paragraph must follow existing content and precede footer:\n%s", got)
	}
}

// A caller with a structural title/header anchor can limit Resolution lookup
// and footer insertion to the canonical body. Examples before that anchor are
// left byte-for-byte alone.
func TestAppendResolutionNoteAfterLine_IgnoresPreAnchorExamples(t *testing.T) {
	prelude := "## Resolution\n\npre-title example\n\n" + footerLine + "\n\n"
	body := prelude + "# Plan: Auth\n\n**Status:** Draft\n\n## Summary\n\nBody.\n\n" + footerLine + "\n"
	path := writeNoteFixture(t, body)
	afterTitle := strings.Count(prelude, "\n") + 1

	original, wrote, err := AppendResolutionNoteAfterLine(path, "canonical audit note", afterTitle)
	if err != nil {
		t.Fatalf("AppendResolutionNoteAfterLine: %v", err)
	}
	if !wrote || string(original) != body {
		t.Fatalf("result = (wrote=%t, original=%q), want (true, original bytes)", wrote, string(original))
	}

	want := prelude + "# Plan: Auth\n\n**Status:** Draft\n\n## Summary\n\nBody.\n\n" +
		"## Resolution\n\ncanonical audit note\n" + footerLine + "\n"
	if got := readFile(t, path); got != want {
		t.Fatalf("body-scoped note mutation mismatch:\nwant:\n%s\ngot:\n%s", want, got)
	}
}

// empty / whitespace-only note → no-op.
func TestAppendResolutionNote_EmptyIsNoOp(t *testing.T) {
	t.Parallel()
	for _, note := range []string{"", "   ", "\n\t \n"} {
		body := "# Idea: Baz\n\n**Status:** Queued\n\nbody\n"
		path := writeNoteFixture(t, body)
		orig, wrote, err := AppendResolutionNote(path, note)
		if err != nil {
			t.Fatalf("note=%q: %v", note, err)
		}
		if wrote {
			t.Fatalf("note=%q: expected wrote=false", note)
		}
		if orig != nil {
			t.Fatalf("note=%q: expected nil original on no-op", note)
		}
		if got := readFile(t, path); got != body {
			t.Fatalf("note=%q: file mutated on no-op:\n%s", note, got)
		}
	}
}

// markdown written verbatim (multi-line, markdown markup preserved, no reflow).
func TestAppendResolutionNote_Verbatim(t *testing.T) {
	t.Parallel()
	body := "# Idea: Baz\n\n**Status:** Queued\n\nbody\n"
	path := writeNoteFixture(t, body)
	note := "Line one with `code`.\n\n- bullet **bold**\n- second bullet"

	_, _, err := AppendResolutionNote(path, note)
	if err != nil {
		t.Fatalf("AppendResolutionNote: %v", err)
	}
	got := readFile(t, path)
	if !strings.Contains(got, note) {
		t.Fatalf("note must be written verbatim:\n%s", got)
	}
}

// RestoreBody restores the exact pre-invocation bytes.
func TestRestoreBody_RoundTrip(t *testing.T) {
	t.Parallel()
	body := "# Idea: Baz\n\n**Status:** Queued\n\nbody\n\n" + footerLine + "\n"
	path := writeNoteFixture(t, body)

	orig, _, err := AppendResolutionNote(path, "a note")
	if err != nil {
		t.Fatalf("AppendResolutionNote: %v", err)
	}
	if got := readFile(t, path); got == body {
		t.Fatal("precondition: file should have changed before restore")
	}
	if err := RestoreBody(path, orig); err != nil {
		t.Fatalf("RestoreBody: %v", err)
	}
	if got := readFile(t, path); got != body {
		t.Fatalf("RestoreBody did not produce byte-identical content:\n%s", got)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(b)
}
