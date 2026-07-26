package lesson

import (
	"strings"
	"testing"
)

func TestValidateSlug(t *testing.T) {
	valid := []string{"a", "kinder-fake", "check-tags-before-tagging", "x1-y2"}
	for _, s := range valid {
		if err := ValidateSlug(s); err != nil {
			t.Errorf("ValidateSlug(%q) = %v, want nil", s, err)
		}
	}
	invalid := []string{"", "Kinder-Fake", "kinder_fake", "kinder/fake", "-lead", "trail-", "a--b"}
	for _, s := range invalid {
		if err := ValidateSlug(s); err == nil {
			t.Errorf("ValidateSlug(%q) = nil, want error", s)
		}
	}
}

func TestTitleCaseFromSlug(t *testing.T) {
	cases := map[string]string{
		"kinder-fake": "Kinder Fake",
		"a--b":        "A  B", // empty middle part is skipped, leaving a double space
		"single":      "Single",
		"":            "",
	}
	for in, want := range cases {
		if got := titleCaseFromSlug(in); got != want {
			t.Errorf("titleCaseFromSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestScaffold_Defaults(t *testing.T) {
	body, err := Scaffold(ScaffoldOptions{Slug: "kinder-fake"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	wants := []string{
		"---\nformat: https://specscore.md/lesson-specification\nstatus: Recorded\n---",
		"# Lesson: Kinder Fake",
		"**Status:** Recorded",
		"**Recurred:** 0",
		"**Owner:** unknown",
		"## Incident",
		"## Process gap",
		"## Check",
		"## Enforcement",
		"*This document follows the https://specscore.md/lesson-specification*",
	}
	for _, w := range wants {
		if !strings.Contains(s, w) {
			t.Errorf("scaffold missing %q:\n%s", w, s)
		}
	}
	// Default date is today's UTC date — just assert a non-placeholder value
	// starting with "20" was emitted.
	if !strings.Contains(s, "**Date:** 20") {
		t.Errorf("default date not emitted:\n%s", s)
	}
}

func TestScaffold_ExplicitFields(t *testing.T) {
	body, err := Scaffold(ScaffoldOptions{
		Slug:  "kinder-fake",
		Title: "A Kinder Fake",
		Owner: "alex",
		Date:  "2026-07-25",
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, w := range []string{"# Lesson: A Kinder Fake", "**Owner:** alex", "**Date:** 2026-07-25"} {
		if !strings.Contains(s, w) {
			t.Errorf("scaffold missing %q:\n%s", w, s)
		}
	}
}

func TestScaffold_InvalidSlug(t *testing.T) {
	if _, err := Scaffold(ScaffoldOptions{Slug: "Bad_Slug"}); err == nil {
		t.Error("expected error for invalid slug")
	}
}
