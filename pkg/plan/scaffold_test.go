package plan

import (
	"strings"
	"testing"
)

func TestScaffold_ParentEmittedAndOmitted(t *testing.T) {
	withParent, err := Scaffold(ScaffoldOptions{Slug: "sub", SourceFeature: "foo", Parent: "specscore:master"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(withParent), "**Supersedes:** —\n**Parent:** specscore:master\n") {
		t.Fatalf("Parent line not emitted after Supersedes:\n%s", withParent)
	}
	noParent, err := Scaffold(ScaffoldOptions{Slug: "root", SourceFeature: "foo", Parent: "  "})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(noParent), "**Parent:**") {
		t.Fatalf("did not expect a Parent line:\n%s", noParent)
	}
}

func TestValidateSlug(t *testing.T) {
	valid := []string{"a", "my-plan", "add-batch-mode", "x1-y2"}
	for _, s := range valid {
		if err := ValidateSlug(s); err != nil {
			t.Errorf("ValidateSlug(%q) = %v, want nil", s, err)
		}
	}
	invalid := []string{"", "My-Plan", "my_plan", "my/plan", "-lead", "trail-", "a--b"}
	for _, s := range invalid {
		if err := ValidateSlug(s); err == nil {
			t.Errorf("ValidateSlug(%q) = nil, want error", s)
		}
	}
}

func TestTitleCaseFromSlug(t *testing.T) {
	cases := map[string]string{
		"my-plan":        "My Plan",
		"add-batch-mode": "Add Batch Mode",
		"a--b":           "A  B", // empty middle part is skipped (not title-cased), leaving a double space
		"":               "",
	}
	for in, want := range cases {
		if got := titleCaseFromSlug(in); got != want {
			t.Errorf("titleCaseFromSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestScaffold_FeatureSourced(t *testing.T) {
	body, err := Scaffold(ScaffoldOptions{
		Slug:          "my-plan",
		Owner:         "alex",
		Date:          "2026-06-05",
		SourceFeature: "some-feature",
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	wants := []string{
		"---\nformat: https://specscore.md/plan-specification\nstatus: Draft\n---",
		"# Plan: My Plan",
		"**Status:** Draft",
		"**Source Feature:** some-feature",
		"**Date:** 2026-06-05",
		"**Owner:** alex",
		"**Supersedes:** —",
		"## Summary",
		"## Approach",
		"## Tasks",
		"## Open Questions",
		"*This document follows the https://specscore.md/plan-specification*",
	}
	for _, w := range wants {
		if !strings.Contains(s, w) {
			t.Errorf("scaffold missing %q:\n%s", w, s)
		}
	}
	if strings.Contains(s, "idea:") {
		t.Errorf("feature-sourced plan must not carry an idea source line:\n%s", s)
	}
}

func TestScaffold_IdeaSourced(t *testing.T) {
	body, err := Scaffold(ScaffoldOptions{Slug: "idea-plan", SourceIdea: "some-idea"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	if !strings.Contains(s, "**Source:** idea:some-idea") {
		t.Errorf("idea-sourced plan missing idea source line:\n%s", s)
	}
	if strings.Contains(s, "**Source Feature:**") {
		t.Errorf("idea-sourced plan must not carry a Source Feature line:\n%s", s)
	}
	// Title defaults to title-cased slug; owner defaults to "unknown".
	if !strings.Contains(s, "# Plan: Idea Plan") {
		t.Errorf("title not defaulted from slug:\n%s", s)
	}
	if !strings.Contains(s, "**Owner:** unknown") {
		t.Errorf("owner not defaulted to unknown:\n%s", s)
	}
}

func TestScaffold_DefaultDateIsToday(t *testing.T) {
	body, err := Scaffold(ScaffoldOptions{Slug: "p", Title: "P", SourceFeature: "f"})
	if err != nil {
		t.Fatal(err)
	}
	// Date defaulting just needs to emit a non-placeholder **Date:** line.
	if !strings.Contains(string(body), "**Date:** 20") {
		t.Errorf("default date not emitted:\n%s", body)
	}
}

func TestScaffold_Errors(t *testing.T) {
	if _, err := Scaffold(ScaffoldOptions{Slug: "Bad_Slug", SourceFeature: "f"}); err == nil {
		t.Error("expected error for invalid slug")
	}
	if _, err := Scaffold(ScaffoldOptions{Slug: "p"}); err == nil {
		t.Error("expected error when neither source is set")
	}
	if _, err := Scaffold(ScaffoldOptions{Slug: "p", SourceFeature: "f", SourceIdea: "i"}); err == nil {
		t.Error("expected error when both sources are set")
	}
}
