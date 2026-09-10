package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/rule"
)

// TestRuleRenderFixture is the brief's exact fixture: one rule with an
// explicit **Trigger:**, one whose Instructions' first line is `Trigger:
// <text>`, and one with neither — so it falls back to its Statement's first
// clause. It asserts the literal rendered output, not just membership.
func TestRuleRenderFixture(t *testing.T) {
	root := setupRuleProject(t)

	if _, _, err := runRule(t, root, "new", "explicit-trigger", "--detailed",
		"--statement", "Always qualify a version number with what it belongs to.",
		"--trigger", "about to write v2 without saying what it belongs to"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runRule(t, root, "new", "instructions-trigger", "--detailed",
		"--statement", "Always record an open question in specscore, never in a scratch doc.",
		"--instructions", "Trigger: about to park an open question in a scratch doc\n\nRecord it in specscore instead."); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runRule(t, root, "new", "no-trigger", "--detailed",
		"--statement", "Never ship a mocked backend, ship real routes.",
		"--instructions", "Ship real, dalgo-backed routes instead."); err != nil {
		t.Fatal(err)
	}

	out, _, err := runRule(t, root, "render")
	if err != nil {
		t.Fatalf("render: %v (stdout=%q)", err, out)
	}
	want := strings.Join([]string{
		"- Never ship a mocked backend → rule:no-trigger",
		"- about to park an open question in a scratch doc → rule:instructions-trigger",
		"- about to write v2 without saying what it belongs to → rule:explicit-trigger",
		"",
	}, "\n")
	if out != want {
		t.Fatalf("render output =\n%s\nwant\n%s", out, want)
	}
}

// TestRuleRenderInlineFallsBackToStatement covers the fourth shape the
// fixture above does not: an inline rule, which has no detail document at
// all and always renders from its row's Statement.
func TestRuleRenderInlineFallsBackToStatement(t *testing.T) {
	root := setupRuleProject(t)
	if _, _, err := runRule(t, root, "new", "gofmt-first",
		"--statement", "Always run gofmt before building, never skip it."); err != nil {
		t.Fatal(err)
	}
	out, _, err := runRule(t, root, "render")
	if err != nil {
		t.Fatal(err)
	}
	if want := "- Always run gofmt before building → rule:gofmt-first\n"; out != want {
		t.Fatalf("render output = %q, want %q", out, want)
	}
}

// TestRuleRenderSkipsSuperseded confirms a retired rule is never printed: it
// is not a live trigger for anything.
func TestRuleRenderSkipsSuperseded(t *testing.T) {
	root := setupRuleProject(t)
	if _, _, err := runRule(t, root, "new", "live", "--statement", "Always live."); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runRule(t, root, "new", "retired", "--detailed", "--status", "Superseded",
		"--statement", "Never retired.", "--supersedes", "live"); err != nil {
		t.Fatal(err)
	}
	out, _, err := runRule(t, root, "render")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "retired") {
		t.Fatalf("a Superseded rule must not render: %q", out)
	}
	if !strings.Contains(out, "rule:live") {
		t.Fatalf("the live rule must still render: %q", out)
	}
}

// TestRuleRenderScopeFilter mirrors `rule list --scope`'s exact filter.
func TestRuleRenderScopeFilter(t *testing.T) {
	root := setupRuleProject(t)
	if _, _, err := runRule(t, root, "new", "fleet-rule", "--scope", "fleet",
		"--statement", "Always fleet-wide."); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runRule(t, root, "new", "go-rule", "--scope", "path:**/*.go",
		"--statement", "Always gofmt go files."); err != nil {
		t.Fatal(err)
	}
	out, _, err := runRule(t, root, "render", "--scope", "path:**/*.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "rule:go-rule") || strings.Contains(out, "rule:fleet-rule") {
		t.Fatalf("scope filter did not select only go-rule: %q", out)
	}
}

// TestRuleRenderSectionGroupingAndFilter covers item 3: an optional
// **Section:** header field groups the md listing under a `## <Section>`
// heading per group, with ungrouped rules under `## Other`; --section
// restricts the listing to one section.
func TestRuleRenderSectionGroupingAndFilter(t *testing.T) {
	root := setupRuleProject(t)
	if _, _, err := runRule(t, root, "new", "explicit-trigger", "--detailed",
		"--statement", "Always qualify a version number with what it belongs to.",
		"--trigger", "about to write v2"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runRule(t, root, "new", "instructions-trigger", "--detailed",
		"--statement", "Always record an open question in specscore.",
		"--instructions", "Trigger: about to park an open question"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runRule(t, root, "new", "ungrouped", "--statement",
		"Never ship a mocked backend, ship real routes."); err != nil {
		t.Fatal(err)
	}

	// Section is not writable via a `rule new`/`rule promote` flag (only
	// --trigger is), so it is added the way a hand-authored header would be.
	addSectionField(t, root, "explicit-trigger", "Answering")
	addSectionField(t, root, "instructions-trigger", "Answering")

	t.Run("md groups by section, other last", func(t *testing.T) {
		out, _, err := runRule(t, root, "render", "--format", "md")
		if err != nil {
			t.Fatal(err)
		}
		want := strings.Join([]string{
			"## Answering",
			"",
			"- about to park an open question → rule:instructions-trigger",
			"- about to write v2 → rule:explicit-trigger",
			"",
			"## Other",
			"",
			"- Never ship a mocked backend → rule:ungrouped",
			"",
		}, "\n")
		if out != want {
			t.Fatalf("md render =\n%s\nwant\n%s", out, want)
		}
	})

	t.Run("--section restricts to one group", func(t *testing.T) {
		out, _, err := runRule(t, root, "render", "--section", "Answering")
		if err != nil {
			t.Fatal(err)
		}
		want := "- about to park an open question → rule:instructions-trigger\n" +
			"- about to write v2 → rule:explicit-trigger\n"
		if out != want {
			t.Fatalf("section-filtered render = %q, want %q", out, want)
		}
	})

	t.Run("--section with no match is empty", func(t *testing.T) {
		out, _, err := runRule(t, root, "render", "--section", "Nonexistent")
		if err != nil {
			t.Fatal(err)
		}
		if out != "" {
			t.Fatalf("render = %q, want empty", out)
		}
	})
}

func TestRuleRenderRejectsUnknownFormat(t *testing.T) {
	root := setupRuleProject(t)
	_, _, err := runRule(t, root, "render", "--format", "yaml")
	if got := exitCodeOf(err); got != exitcode.InvalidArgs {
		t.Fatalf("exit = %d, want %d", got, exitcode.InvalidArgs)
	}
}

// addSectionField inserts a **Section:** header line right after
// **Statement:** in a detailed rule's document, the way a hand-authored one
// would carry it — there is no `rule new`/`rule promote` flag for it.
func addSectionField(t *testing.T, root, slug, section string) {
	t.Helper()
	path := rule.DetailPath(rule.RulesDir(root), slug)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(body), "\n")
	out := make([]string, 0, len(lines)+1)
	inserted := false
	for _, line := range lines {
		out = append(out, line)
		if !inserted && strings.HasPrefix(strings.TrimSpace(line), "**Statement:**") {
			out = append(out, "**Section:** "+section)
			inserted = true
		}
	}
	if !inserted {
		t.Fatalf("no **Statement:** line found in %s", path)
	}
	if err := os.WriteFile(path, []byte(strings.Join(out, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
}
