package rule

import (
	"reflect"
	"testing"
)

func TestResolveTriggerFallbackChain(t *testing.T) {
	cases := []struct {
		name      string
		statement string
		detail    *Detail
		want      string
	}{
		{
			name:      "explicit trigger wins",
			statement: "Never ship a mocked backend, ship real routes.",
			detail:    &Detail{Trigger: "about to mock an extension backend"},
			want:      "about to mock an extension backend",
		},
		{
			name:      "instructions Trigger line wins over statement",
			statement: "Never ship a mocked backend, ship real routes.",
			detail:    &Detail{InstructionsFirstLine: "Trigger: about to mock an extension backend"},
			want:      "about to mock an extension backend",
		},
		{
			name:      "a non-Trigger instructions line is not the trigger",
			statement: "Never ship a mocked backend, ship real routes.",
			detail:    &Detail{InstructionsFirstLine: "Ship real, dalgo-backed routes instead."},
			want:      "Never ship a mocked backend",
		},
		{
			name:      "statement first clause, comma",
			statement: "Never ship a mocked backend, ship real routes.",
			detail:    nil,
			want:      "Never ship a mocked backend",
		},
		{
			name:      "statement first clause, colon",
			statement: "About to mock a backend: never do it.",
			detail:    nil,
			want:      "About to mock a backend",
		},
		{
			name:      "statement with no clause separator is returned whole",
			statement: "Always run gofmt before building.",
			detail:    nil,
			want:      "Always run gofmt before building.",
		},
		{
			name:      "sentinel trigger falls through to statement",
			statement: "Never ship a mocked backend, ship real routes.",
			detail:    &Detail{Trigger: Sentinel},
			want:      "Never ship a mocked backend",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveTrigger(tc.statement, tc.detail)
			if got != tc.want {
				t.Errorf("ResolveTrigger() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildRenderEntriesSortsBySectionThenTrigger(t *testing.T) {
	rows := []Row{
		NewRow("zeta", false, "Active", "Zeta does one thing, always.", []string{"fleet"}, "Stated", "", nil),
		NewRow("alpha", false, "Active", "Alpha does another thing, always.", []string{"fleet"}, "Stated", "", nil),
		NewRow("beta", true, "Draft", "Beta needs a document, always.", []string{"fleet"}, "Stated", "", nil),
	}
	details := map[string]*Detail{
		"beta": {Trigger: "about to beta", Section: "Tooling"},
	}
	entries := BuildRenderEntries(rows, details)
	want := []RenderEntry{
		{Trigger: "Alpha does another thing", Slug: "alpha", Section: ""},
		{Trigger: "Zeta does one thing", Slug: "zeta", Section: ""},
		{Trigger: "about to beta", Slug: "beta", Section: "Tooling"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("entries = %#v, want %#v", entries, want)
	}
}

// TestBuildRenderEntriesBreaksTiesBySlug covers the third sort key: two rows
// whose Section and Trigger both happen to agree must still sort
// deterministically, by slug.
func TestBuildRenderEntriesBreaksTiesBySlug(t *testing.T) {
	rows := []Row{
		NewRow("zeta", false, "Active", "Always the same thing.", []string{"fleet"}, "Stated", "", nil),
		NewRow("alpha", false, "Active", "Always the same thing.", []string{"fleet"}, "Stated", "", nil),
	}
	entries := BuildRenderEntries(rows, nil)
	if len(entries) != 2 || entries[0].Slug != "alpha" || entries[1].Slug != "zeta" {
		t.Fatalf("entries = %#v, want alpha before zeta on a tied Section and Trigger", entries)
	}
}

func TestRenderEntryRenderLine(t *testing.T) {
	e := RenderEntry{Trigger: "about to write v2", Slug: "always-qualify-version-numbers"}
	if got, want := e.RenderLine(), "- about to write v2 → rule:always-qualify-version-numbers"; got != want {
		t.Fatalf("RenderLine() = %q, want %q", got, want)
	}
}
