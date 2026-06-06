package sidekick

import (
	"strings"
	"testing"
)

func TestDeriveSlug(t *testing.T) {
	cases := map[string]string{
		"Add Batch Mode!":               "add-batch-mode",
		"  specscore needs a seed verb": "specscore-needs-a-seed-verb",
		"Hello, World":                  "hello-world",
		"already-a-slug":                "already-a-slug",
		"multiple   spaces & symbols!!": "multiple-spaces-symbols",
		"UPPER lower 123":               "upper-lower-123",
	}
	for in, want := range cases {
		got, err := DeriveSlug(in)
		if err != nil {
			t.Errorf("DeriveSlug(%q) returned error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("DeriveSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDeriveSlug_EmptyWhenNoSlugSafeChars(t *testing.T) {
	for _, in := range []string{"", "   ", "!!!", "***///"} {
		if _, err := DeriveSlug(in); err == nil {
			t.Errorf("DeriveSlug(%q) = nil error, want error", in)
		}
	}
}

func TestValidateSlug(t *testing.T) {
	valid := []string{"a", "add-batch-mode", "add-batch-mode-2", "x1-y2", "abc123"}
	for _, s := range valid {
		if err := ValidateSlug(s); err != nil {
			t.Errorf("ValidateSlug(%q) = %v, want nil", s, err)
		}
	}
	invalid := []string{"", "Add-Batch", "add_batch", "add/batch", "-lead", "trail-", "a--b", strings.Repeat("a", MaxSlugChars+1)}
	for _, s := range invalid {
		if err := ValidateSlug(s); err == nil {
			t.Errorf("ValidateSlug(%q) = nil, want error", s)
		}
	}
}

func TestDeriveSlug_TruncatesAtHyphenBoundaryUnder60(t *testing.T) {
	in := "this is a very long one liner that should be truncated at a hyphen boundary well past sixty characters total"
	got, err := DeriveSlug(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > MaxSlugChars {
		t.Errorf("DeriveSlug len = %d (%q), want <= %d", len(got), got, MaxSlugChars)
	}
	if strings.HasSuffix(got, "-") || strings.HasPrefix(got, "-") {
		t.Errorf("DeriveSlug = %q, must not have leading/trailing hyphen", got)
	}
}

func TestScaffold_EmitsClosedFrontmatterAndH1(t *testing.T) {
	body, err := Scaffold(ScaffoldOptions{
		Slug:       "add-batch-mode",
		OneLiner:   "Add batch mode",
		CapturedAt: "2026-06-06T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, want := range []string{
		"---\n",
		"type: sidekick-seed\n",
		"slug: add-batch-mode\n",
		"captured_at: 2026-06-06T00:00:00Z\n",
		"captured_by: user\n",     // default
		"captured_during: null\n", // default
		"trigger: explicit\n",     // default
		"status: queued\n",
		"synchestra_task: null\n",
		"# Add batch mode\n",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("scaffold missing %q:\n%s", want, s)
		}
	}
}

func TestScaffold_OverridesAndOptionalBody(t *testing.T) {
	body, err := Scaffold(ScaffoldOptions{
		Slug:           "with-body",
		OneLiner:       "with body",
		CapturedBy:     "specstudio:specify",
		CapturedDuring: "spec/features/foo/README.md",
		Trigger:        "heuristic",
		Body:           "Some **markdown** detail.",
		CapturedAt:     "2026-06-06T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, want := range []string{
		"captured_by: specstudio:specify\n",
		"captured_during: spec/features/foo/README.md\n",
		"trigger: heuristic\n",
		"# with body\n\nSome **markdown** detail.\n",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("scaffold missing %q:\n%s", want, s)
		}
	}
}

func TestScaffold_RejectsBadInput(t *testing.T) {
	cases := []struct {
		name string
		opts ScaffoldOptions
	}{
		{"empty one-liner", ScaffoldOptions{Slug: "s", OneLiner: "   "}},
		{"empty slug", ScaffoldOptions{Slug: "", OneLiner: "x"}},
		{"bad trigger", ScaffoldOptions{Slug: "s", OneLiner: "x", Trigger: "maybe"}},
		{"one-liner too long", ScaffoldOptions{Slug: "s", OneLiner: strings.Repeat("a", MaxOneLinerChars+1)}},
		{"body too long", ScaffoldOptions{Slug: "s", OneLiner: "x", Body: strings.Repeat("a", MaxBodyChars+1)}},
	}
	for _, c := range cases {
		if _, err := Scaffold(c.opts); err == nil {
			t.Errorf("Scaffold(%s) = nil error, want error", c.name)
		}
	}
}
