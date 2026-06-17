package decision

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateSlug(t *testing.T) {
	tests := []struct {
		slug string
		ok   bool
	}{
		{"postgres-over-mongo", true},
		{"a", true},
		{"test-slug-123", true},
		{"", false},
		{"CamelCase", false},
		{"-leading-hyphen", false},
		{"trailing-hyphen-", false},
		{"double--hyphen", false},
	}
	for _, tt := range tests {
		err := ValidateSlug(tt.slug)
		if tt.ok && err != nil {
			t.Errorf("ValidateSlug(%q) unexpected error: %v", tt.slug, err)
		}
		if !tt.ok && err == nil {
			t.Errorf("ValidateSlug(%q) expected error", tt.slug)
		}
	}
}

func TestNextNumber(t *testing.T) {
	t.Run("empty dir returns 1", func(t *testing.T) {
		root := t.TempDir()
		_ = os.MkdirAll(filepath.Join(root, "decisions"), 0o755)
		n, err := NextNumber(root)
		if err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("expected 1, got %d", n)
		}
	})

	t.Run("considers archived", func(t *testing.T) {
		root := t.TempDir()
		_ = os.MkdirAll(filepath.Join(root, "decisions", "archived"), 0o755)
		_ = os.WriteFile(filepath.Join(root, "decisions", "0001-a.md"), []byte("x"), 0o644)
		_ = os.WriteFile(filepath.Join(root, "decisions", "archived", "0003-c.md"), []byte("x"), 0o644)
		n, err := NextNumber(root)
		if err != nil {
			t.Fatal(err)
		}
		if n != 4 {
			t.Errorf("expected 4, got %d", n)
		}
	})
}

func TestScaffold(t *testing.T) {
	t.Run("produces valid content", func(t *testing.T) {
		body, err := Scaffold(ScaffoldOptions{
			Slug:  "test-decision",
			Title: "Test Decision",
			Owner: "test@example.com",
			Date:  "2026-05-26",
		})
		if err != nil {
			t.Fatal(err)
		}
		s := string(body)

		checks := []string{
			"# Decision: Test Decision",
			"**Status:** Draft",
			"**Date:** 2026-05-26",
			"**Owner:** test@example.com",
			"**Tags:** —",
			"**Source Idea:** —",
			"**Supersedes:** —",
			"**Superseded By:** —",
			"## Context",
			"## Decision",
			"## Rationale",
			"## Declined Alternatives",
			"## Consequences at Decision Time",
			"## Observed Consequences",
			"None observed yet.",
			"## Affected Features",
			"None at this time.",
			"https://specscore.md/decision-specification",
		}
		for _, check := range checks {
			if !strings.Contains(s, check) {
				t.Errorf("scaffold missing %q", check)
			}
		}
	})

	t.Run("title defaults from slug", func(t *testing.T) {
		body, err := Scaffold(ScaffoldOptions{
			Slug: "postgres-over-mongo",
			Date: "2026-05-26",
		})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), "# Decision: Postgres Over Mongo") {
			t.Error("expected title to be derived from slug")
		}
	})

	t.Run("flags injected", func(t *testing.T) {
		body, err := Scaffold(ScaffoldOptions{
			Slug:       "test",
			Title:      "Custom Title",
			Owner:      "owner@test.com",
			Date:       "2026-01-01",
			Tags:       "tag1, tag2",
			SourceIdea: "my-idea",
			Supersedes: "0001-old",
		})
		if err != nil {
			t.Fatal(err)
		}
		s := string(body)
		if !strings.Contains(s, "**Tags:** tag1, tag2") {
			t.Error("expected tags injection")
		}
		if !strings.Contains(s, "**Source Idea:** my-idea") {
			t.Error("expected source idea injection")
		}
		if !strings.Contains(s, "**Supersedes:** 0001-old") {
			t.Error("expected supersedes injection")
		}
	})

	t.Run("invalid slug rejected", func(t *testing.T) {
		_, err := Scaffold(ScaffoldOptions{Slug: "Bad-Slug"})
		if err == nil {
			t.Error("expected error for invalid slug")
		}
	})
}
