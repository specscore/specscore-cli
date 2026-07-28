package lifecycle

import (
	"os"
	"strings"
	"testing"
)

// ideaFixtureFM is an Idea carrying the artifact-frontmatter-convention
// frontmatter (status: mirrors the body **Status:**), matching what the
// retrofitted scaffolders now emit.
const ideaFixtureFM = "---\nformat: https://specscore.md/idea-specification\nstatus: Draft\n---\n\n" +
	"# Idea: Sample\n\n**Status:** Draft\n**Date:** 2026-01-01\n**Owner:** alice\n\n## Problem Statement\n\nFix it.\n"

// AC:change-status-dual-write — Rewrite updates the body **Status:** and the
// frontmatter status: mirror together.
func TestRewrite_MirrorsFrontmatterStatus(t *testing.T) {
	path := writeFixture(t, ideaFixtureFM)
	if _, err := Rewrite(path, IdeaApproved); err != nil {
		t.Fatalf("Rewrite: %v", err)
	}
	got, _ := os.ReadFile(path)
	want := strings.NewReplacer(
		"status: Draft", "status: Approved",
		"**Status:** Draft", "**Status:** Approved",
	).Replace(ideaFixtureFM)
	if string(got) != want {
		t.Errorf("dual-write mismatch.\nGot:\n%s\nWant:\n%s", got, want)
	}
}

// A mid-flight rollback restores BOTH surfaces to the prior value, byte-for-byte.
func TestRewriteRollback_MirrorsFrontmatter(t *testing.T) {
	path := writeFixture(t, ideaFixtureFM)
	origLine, err := Rewrite(path, IdeaApproved)
	if err != nil {
		t.Fatalf("Rewrite: %v", err)
	}
	if err := Rollback(path, origLine); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != ideaFixtureFM {
		t.Errorf("rollback not byte-identical.\nGot:\n%s\nWant:\n%s", got, ideaFixtureFM)
	}
}

// A file with no frontmatter mirror is rewritten body-only (unchanged behavior).
func TestRewrite_NoFrontmatterBodyOnly(t *testing.T) {
	path := writeFixture(t, ideaFixture)
	if _, err := Rewrite(path, IdeaApproved); err != nil {
		t.Fatalf("Rewrite: %v", err)
	}
	got, _ := os.ReadFile(path)
	want := strings.Replace(ideaFixture, "**Status:** Draft", "**Status:** Approved", 1)
	if string(got) != want {
		t.Errorf("body-only rewrite changed unexpectedly:\n%s", got)
	}
}

func TestFindFrontmatterStatusLineIndex(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    int
	}{
		{"empty", "", -1},
		{"no opening fence", "# Title\n\n**Status:** Draft\n", -1},
		{"status before closing fence", "---\nformat: x\nstatus: Draft\n---\n", 2},
		{"status after closing fence is not the mirror", "---\nformat: x\n---\nstatus: Draft\n", -1},
		{"opening fence never closed, no status", "---\nformat: x\nbody\n", -1},
		{"BOM-prefixed opening fence", "\ufeff---\nformat: x\nstatus: Draft\n---\n", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := findFrontmatterStatusLineIndex(splitKeepTerminators([]byte(tc.content)))
			if got != tc.want {
				t.Errorf("findFrontmatterStatusLineIndex = %d, want %d", got, tc.want)
			}
		})
	}
}

// A BOM is part of the physical source bytes, not frontmatter syntax. A
// lifecycle rewrite must still recognize the leading block, preserve the BOM,
// and keep its status mirror atomically aligned with the body field.
func TestRewrite_BOMPrefixedFrontmatterMirrorsAndRollsBack(t *testing.T) {
	body := "\ufeff---\r\nformat: https://specscore.md/idea-specification\r\nstatus: Draft\r\n---\r\n\r\n" +
		"# Idea: Sample\r\n\r\n**Status:** Draft\r\n"
	path := writeFixture(t, body)
	origLine, err := Rewrite(path, IdeaApproved)
	if err != nil {
		t.Fatalf("Rewrite: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.NewReplacer(
		"status: Draft", "status: Approved",
		"**Status:** Draft", "**Status:** Approved",
	).Replace(body)
	if string(got) != want {
		t.Fatalf("BOM dual-write mismatch.\nwant:\n%s\ngot:\n%s", want, got)
	}
	if !strings.HasPrefix(string(got), "\ufeff---\r\n") {
		t.Fatalf("rewrite did not preserve BOM and CRLF opening bytes: %q", got)
	}
	if err := Rollback(path, origLine); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	restored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != body {
		t.Fatalf("BOM rollback was not byte-exact.\nwant:\n%s\ngot:\n%s", body, restored)
	}
}

func TestRewrite_PlanAndLessonUseCanonicalMaskedHeaderStatus(t *testing.T) {
	for _, tc := range []struct {
		name  string
		title string
	}{
		{name: "plan", title: "# Plan: Sample"},
		{name: "lesson", title: "# Lesson: Sample"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := "## Example before title\n\n**Status:** Fake\n\n" + tc.title +
				"\n\n**Status:** Draft\n```markdown\n**Status:** Fenced\n```\n\n## Summary\n\n**Status:** Body\n"
			path := writeFixture(t, body)
			originalLine, err := Rewrite(path, IdeaApproved)
			if err != nil {
				t.Fatalf("Rewrite: %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			want := strings.Replace(body, "**Status:** Draft", "**Status:** Approved", 1)
			if string(got) != want {
				t.Fatalf("only canonical header status may change:\nwant:\n%s\ngot:\n%s", want, got)
			}
			if err := Rollback(path, originalLine); err != nil {
				t.Fatalf("Rollback: %v", err)
			}
			restored, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(restored) != body {
				t.Fatalf("rollback must restore every sample byte:\nwant:\n%s\ngot:\n%s", body, restored)
			}
		})
	}
}

func TestBodyStatusValue(t *testing.T) {
	if got := bodyStatusValue("**Status:** Draft\n"); got != "Draft" {
		t.Errorf("bodyStatusValue = %q, want Draft", got)
	}
	if got := bodyStatusValue("not a status line\n"); got != "" {
		t.Errorf("bodyStatusValue = %q, want empty", got)
	}
}
