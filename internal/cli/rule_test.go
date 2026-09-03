package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/rule"
	"github.com/spf13/cobra"
)

// runRule executes `specscore rule <args...>` against the given project root,
// returning stdout, stderr, and the error (which carries the exit code).
func runRule(t *testing.T, root string, args ...string) (string, string, error) {
	t.Helper()
	cmd := ruleCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(append(args, "--project", root))
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func setupRuleProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "spec", "features"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func readRuleIndex(t *testing.T, root string) string {
	t.Helper()
	b, err := os.ReadFile(rule.IndexPath(rule.RulesDir(root)))
	if err != nil {
		t.Fatalf("reading the rules index: %v", err)
	}
	return string(b)
}

func readRuleDetail(t *testing.T, root, slug string) string {
	t.Helper()
	b, err := os.ReadFile(rule.DetailPath(rule.RulesDir(root), slug))
	if err != nil {
		t.Fatalf("reading rule detail %s: %v", slug, err)
	}
	return string(b)
}

// writeCanonicalLesson materializes a canonical Lesson so promotion and
// reciprocity paths have something real to link against.
func writeCanonicalLesson(t *testing.T, root, slug, control string) string {
	t.Helper()
	dir := filepath.Join(root, "spec", "lessons", slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nformat: https://specscore.md/lesson-specification\nstatus: Recorded\n---\n\n" +
		"# Lesson: " + rule.TitleCaseFromSlug(slug) + "\n\n" +
		"**Status:** Recorded\n**Date:** 2026-09-03\n**Owner:** alex\n" +
		"**Classifications:** process\n**Legacy Provenance:** —\n**Duplicate Of:** —\n" +
		"**Supersedes:** —\n**Superseded By:** —\n\n" +
		"## Lesson\n\nThe durable rule.\n\n## Enforcement\n\n**Control:** " + control +
		"\n**Verification:** CI runs the conformance suite.\n**Evidence:** —\n\n" +
		"## Open Questions\n\nNone at this time.\n"
	path := filepath.Join(dir, "README.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeRuleSkillFile(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, rule.DefaultSkillsPath, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ----- group wiring -----

func TestRuleGroupPrintsHelp(t *testing.T) {
	cmd := ruleCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("rule group: %v", err)
	}
	for _, want := range []string{"new", "expand", "list", "show", "update", "delete", "promote", "lint"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("group help missing subcommand %q:\n%s", want, out.String())
		}
	}
}

// The singular `rule` group and the plural `rules` lint catalog are different
// commands; registering one must never shadow the other.
func TestRuleAndRulesAreDistinctCommands(t *testing.T) {
	root, _ := newRootCommand()
	var singular, plural bool
	for _, c := range root.Commands() {
		switch c.Name() {
		case "rule":
			singular = true
		case "rules":
			plural = true
		}
	}
	if !singular || !plural {
		t.Fatalf("expected both `rule` and `rules`; got singular=%v plural=%v", singular, plural)
	}
}

// Every rule verb must accept --format json, which is what makes the group
// usable from an agent rather than only from a terminal.
func TestEveryRuleVerbAcceptsFormatJSON(t *testing.T) {
	for _, sub := range ruleCommand().Commands() {
		if sub.Flags().Lookup("format") == nil {
			t.Errorf("`rule %s` has no --format flag", sub.Name())
		}
	}
}

// ----- new -----

// AC: an unflagged `rule new` records an inline rule: one index row, no
// directory. That is the friction argument the kind rests on.
func TestRuleNewIsInlineByDefault(t *testing.T) {
	root := setupRuleProject(t)
	out, _, err := runRule(t, root, "new", "never-mock-backends",
		"--statement", "Never ship a mocked extension backend.")
	if err != nil {
		t.Fatalf("rule new: %v", err)
	}
	if !strings.Contains(out, filepath.Join("rules", "README.md")) {
		t.Fatalf("an inline rule must point the caller at the index, got %q", out)
	}
	index := readRuleIndex(t, root)
	if !strings.Contains(index, "| never-mock-backends | Draft | fleet | Stated | — | — | Never ship a mocked extension backend. |") {
		t.Fatalf("row not recorded:\n%s", index)
	}
	if strings.Contains(index, "[never-mock-backends]") {
		t.Fatalf("an inline rule must not be linked:\n%s", index)
	}
	if _, err := os.Stat(filepath.Join(rule.RulesDir(root), "never-mock-backends")); !os.IsNotExist(err) {
		t.Fatal("an inline rule must not create a directory")
	}
	// The ancestor index is materialized too, so a fresh project is usable.
	if _, err := os.Stat(filepath.Join(root, "spec", "README.md")); err != nil {
		t.Fatalf("spec/README.md not materialized: %v", err)
	}
	if _, _, err := runRule(t, root, "lint"); err != nil {
		t.Fatalf("rule lint after new: %v", err)
	}
}

func TestRuleNewWithOnlyASlugIsLintClean(t *testing.T) {
	root := setupRuleProject(t)
	if _, _, err := runRule(t, root, "new", "x"); err != nil {
		t.Fatalf("rule new: %v", err)
	}
	if _, _, err := runRule(t, root, "lint"); err != nil {
		t.Fatalf("a slug-only rule is not lint-clean: %v", err)
	}
}

func TestRuleNewDetailed(t *testing.T) {
	root := setupRuleProject(t)
	out, _, err := runRule(t, root, "new", "gofmt-first",
		"--title", "Format Go First",
		"--statement", "Always run gofmt on changed Go files before any build.",
		"--scope", "fleet", "--scope", "path:**/*.go",
		"--enforcement", "Enforced", "--control", "wb pre-commit hook",
		"--status", "Active", "--owner", "alex", "--date", "2026-09-03",
		"--detailed", "--format", "json")
	if err != nil {
		t.Fatalf("rule new --detailed: %v", err)
	}
	var result ruleWriteResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out)
	}
	if result.Slug != "gofmt-first" || result.Form != "detailed" || !result.Detailed || result.Status != "Active" {
		t.Fatalf("result = %+v", result)
	}
	index := readRuleIndex(t, root)
	if !strings.Contains(index, "[gofmt-first](gofmt-first/README.md)") {
		t.Fatalf("a detailed rule's row must link to its document:\n%s", index)
	}
	body := readRuleDetail(t, root, "gofmt-first")
	for _, want := range []string{
		"status: Active", "# Rule: Format Go First",
		"**Scope:** fleet, path:**/*.go", "**Enforcement:** Enforced", "**Control:** wb pre-commit hook",
		"## Instructions", "### Compliant", "### Violation",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("detail document missing %q:\n%s", want, body)
		}
	}
	if _, _, err := runRule(t, root, "lint"); err != nil {
		t.Fatalf("a detailed rule is not lint-clean: %v", err)
	}
}

// A document-only flag has nowhere to live on an inline rule, so passing one
// implies --detailed rather than being silently dropped.
func TestRuleNewDetailFlagsImplyDetailed(t *testing.T) {
	for _, flag := range []string{"--why", "--exceptions", "--instructions", "--compliant", "--violation", "--supersedes"} {
		t.Run(flag, func(t *testing.T) {
			root := setupRuleProject(t)
			if _, _, err := runRule(t, root, "new", "x", flag, "some-value"); err != nil {
				t.Fatalf("rule new %s: %v", flag, err)
			}
			if _, err := os.Stat(rule.DetailPath(rule.RulesDir(root), "x")); err != nil {
				t.Fatalf("%s did not imply --detailed: %v", flag, err)
			}
		})
	}
	t.Run("--skill", func(t *testing.T) {
		root := setupRuleProject(t)
		if _, _, err := runRule(t, root, "new", "x", "--skill", "go-hygiene"); err != nil {
			t.Fatalf("rule new --skill: %v", err)
		}
		body := readRuleDetail(t, root, "x")
		if !strings.Contains(body, "skill:go-hygiene") {
			t.Fatalf("skill reference not recorded:\n%s", body)
		}
	})
}

func TestRuleNewRejects(t *testing.T) {
	root := setupRuleProject(t)
	if _, _, err := runRule(t, root, "new", "ok-rule"); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		args     []string
		wantCode int
	}{
		{name: "missing slug", args: []string{"new"}, wantCode: exitcode.InvalidArgs},
		{name: "two slugs", args: []string{"new", "a", "b"}, wantCode: exitcode.InvalidArgs},
		{name: "invalid slug", args: []string{"new", "Not A Slug"}, wantCode: exitcode.InvalidArgs},
		{name: "invalid format", args: []string{"new", "x", "--format", "toml"}, wantCode: exitcode.InvalidArgs},
		{name: "invalid scope", args: []string{"new", "x", "--scope", "team:platform"}, wantCode: exitcode.InvalidArgs},
		{name: "invalid source", args: []string{"new", "x", "--source", "bogus"}, wantCode: exitcode.InvalidArgs},
		{name: "invalid status", args: []string{"new", "x", "--status", "Pending"}, wantCode: exitcode.InvalidArgs},
		{name: "invalid skill", args: []string{"new", "x", "--skill", "Not A Skill"}, wantCode: exitcode.InvalidArgs},
		{name: "enforced without control", args: []string{"new", "x", "--enforcement", "Enforced"}, wantCode: exitcode.InvalidArgs},
		{name: "existing slug", args: []string{"new", "ok-rule"}, wantCode: exitcode.Conflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := runRule(t, root, tc.args...)
			if got := exitCodeOf(err); got != tc.wantCode {
				t.Fatalf("exit = %d, want %d (err=%v)", got, tc.wantCode, err)
			}
		})
	}
}

func TestRuleNewForceOverwrites(t *testing.T) {
	root := setupRuleProject(t)
	if _, _, err := runRule(t, root, "new", "x", "--statement", "First.", "--detailed"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runRule(t, root, "new", "x", "--statement", "Second.", "--detailed", "--force"); err != nil {
		t.Fatalf("rule new --force: %v", err)
	}
	if !strings.Contains(readRuleDetail(t, root, "x"), "**Statement:** Second.") {
		t.Fatal("--force did not overwrite the document")
	}
	if strings.Count(readRuleIndex(t, root), "| [x](x/README.md) |") != 1 {
		t.Fatalf("index row duplicated:\n%s", readRuleIndex(t, root))
	}
}

// ----- expand -----

func TestRuleExpand(t *testing.T) {
	root := setupRuleProject(t)
	if _, _, err := runRule(t, root, "new", "x",
		"--statement", "Always x.", "--scope", "path:**/*.go",
		"--enforcement", "Enforced", "--control", "wb hook", "--status", "Active"); err != nil {
		t.Fatal(err)
	}
	out, _, err := runRule(t, root, "expand", "x", "--why", "Because it matters.", "--format", "json")
	if err != nil {
		t.Fatalf("rule expand: %v", err)
	}
	var result ruleWriteResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out)
	}
	if result.Action != "expanded" || result.Form != "detailed" {
		t.Fatalf("result = %+v", result)
	}
	// Everything the row already said is carried over verbatim, so the new
	// document starts in agreement with it.
	body := readRuleDetail(t, root, "x")
	for _, want := range []string{
		"**Status:** Active", "**Statement:** Always x.", "**Scope:** path:**/*.go",
		"**Enforcement:** Enforced", "**Control:** wb hook", "**Why:** Because it matters.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expanded document missing %q:\n%s", want, body)
		}
	}
	if !strings.Contains(readRuleIndex(t, root), "[x](x/README.md)") {
		t.Fatalf("expand did not link the row:\n%s", readRuleIndex(t, root))
	}
	if _, _, err := runRule(t, root, "lint"); err != nil {
		t.Fatalf("expanded rule is not lint-clean: %v", err)
	}
}

func TestRuleExpandRejects(t *testing.T) {
	root := setupRuleProject(t)
	if _, _, err := runRule(t, root, "new", "detailed-already", "--detailed"); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		args     []string
		wantCode int
	}{
		{name: "missing slug", args: []string{"expand"}, wantCode: exitcode.InvalidArgs},
		{name: "two slugs", args: []string{"expand", "a", "b"}, wantCode: exitcode.InvalidArgs},
		{name: "bad format", args: []string{"expand", "detailed-already", "--format", "toml"}, wantCode: exitcode.InvalidArgs},
		{name: "unknown rule", args: []string{"expand", "ghost"}, wantCode: exitcode.NotFound},
		{name: "already detailed", args: []string{"expand", "detailed-already"}, wantCode: exitcode.Conflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := runRule(t, root, tc.args...)
			if got := exitCodeOf(err); got != tc.wantCode {
				t.Fatalf("exit = %d, want %d (err=%v)", got, tc.wantCode, err)
			}
		})
	}
}

// ----- list -----

func TestRuleList(t *testing.T) {
	root := setupRuleProject(t)
	if _, _, err := runRule(t, root, "new", "zeta", "--statement", "Never zeta.", "--scope", "path:**/*.go",
		"--enforcement", "Enforced", "--control", "wb hook", "--status", "Active", "--detailed"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runRule(t, root, "new", "alpha", "--statement", "Always alpha.", "--scope", "fleet"); err != nil {
		t.Fatal(err)
	}

	t.Run("sorted by slug", func(t *testing.T) {
		out, _, err := runRule(t, root, "list")
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if len(lines) != 2 || !strings.HasPrefix(lines[0], "alpha\t") || !strings.HasPrefix(lines[1], "zeta\t") {
			t.Fatalf("listing = %q", out)
		}
	})

	t.Run("json carries the row and its form", func(t *testing.T) {
		out, _, err := runRule(t, root, "list", "--format", "json")
		if err != nil {
			t.Fatal(err)
		}
		var entries []ruleListEntry
		if err := json.Unmarshal([]byte(out), &entries); err != nil {
			t.Fatalf("stdout is not JSON: %v\n%s", err, out)
		}
		if len(entries) != 2 || entries[0].Slug != "alpha" || entries[0].Detailed {
			t.Fatalf("entries = %+v", entries)
		}
		if !entries[1].Detailed || entries[1].Enforcement != "Enforced" {
			t.Fatalf("entries = %+v", entries)
		}
		// Absent lists render as [] rather than null, so a consumer can index
		// them without a nil check.
		if entries[0].Sources == nil || entries[0].Scope == nil {
			t.Fatalf("nil slices in JSON output: %+v", entries[0])
		}
	})

	t.Run("yaml", func(t *testing.T) {
		out, _, err := runRule(t, root, "list", "--format", "yaml")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "slug: alpha") {
			t.Fatalf("yaml listing = %q", out)
		}
	})

	t.Run("filters", func(t *testing.T) {
		cases := []struct {
			name string
			args []string
			want []string
		}{
			{name: "status", args: []string{"list", "--status", "active"}, want: []string{"zeta"}},
			{name: "enforcement", args: []string{"list", "--enforcement", "stated"}, want: []string{"alpha"}},
			{name: "scope exact", args: []string{"list", "--scope", "path:**/*.go"}, want: []string{"zeta"}},
			{name: "applies-to go file", args: []string{"list", "--applies-to", "internal/cli/x.go"}, want: []string{"alpha", "zeta"}},
			{name: "applies-to md file", args: []string{"list", "--applies-to", "docs/x.md"}, want: []string{"alpha"}},
			{name: "composed filters", args: []string{"list", "--applies-to", "internal/cli/x.go", "--status", "Active"}, want: []string{"zeta"}},
			{name: "no match is empty", args: []string{"list", "--status", "Superseded"}, want: nil},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				out, _, err := runRule(t, root, tc.args...)
				if err != nil {
					t.Fatalf("list: %v", err)
				}
				var got []string
				for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
					if line != "" {
						got = append(got, strings.SplitN(line, "\t", 2)[0])
					}
				}
				if strings.Join(got, ",") != strings.Join(tc.want, ",") {
					t.Fatalf("list = %v, want %v", got, tc.want)
				}
			})
		}
	})

	t.Run("rejects unknown filter values", func(t *testing.T) {
		for _, args := range [][]string{
			{"list", "--status", "bogus"},
			{"list", "--enforcement", "bogus"},
			{"list", "--scope", "team:platform"},
			{"list", "--format", "toml"},
		} {
			_, _, err := runRule(t, root, args...)
			if got := exitCodeOf(err); got != exitcode.InvalidArgs {
				t.Errorf("%v exit = %d, want %d", args, got, exitcode.InvalidArgs)
			}
		}
	})
}

// A malformed scope in the index must not make a filtered listing crash or
// silently include the row.
func TestRuleListSkipsRowsWithUnparsableScope(t *testing.T) {
	root := setupRuleProject(t)
	if _, _, err := runRule(t, root, "new", "x", "--statement", "s"); err != nil {
		t.Fatal(err)
	}
	index := strings.Replace(readRuleIndex(t, root), "| fleet |", "| team:platform |", 1)
	if err := os.WriteFile(rule.IndexPath(rule.RulesDir(root)), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"list", "--applies-to", "x.go"},
		{"list", "--scope", "fleet"},
	} {
		out, _, err := runRule(t, root, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if strings.TrimSpace(out) != "" {
			t.Fatalf("%v listed a row whose scope does not parse: %q", args, out)
		}
	}
}

func TestRuleListEmptyProject(t *testing.T) {
	root := setupRuleProject(t)
	out, _, err := runRule(t, root, "list")
	if err != nil {
		t.Fatalf("list on an empty project must exit 0: %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Fatalf("stdout = %q, want empty", out)
	}
	out, _, err = runRule(t, root, "list", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Fatalf("json listing = %q, want []", out)
	}
}

// ----- show -----

func TestRuleShow(t *testing.T) {
	root := setupRuleProject(t)
	writeCanonicalLesson(t, root, "kinder-fake", "Assert against the real adapter.")
	if _, _, err := runRule(t, root, "promote", "--from-lesson", "kinder-fake", "no-fakes"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runRule(t, root, "new", "inline-one", "--statement", "Always inline."); err != nil {
		t.Fatal(err)
	}

	t.Run("json resolves links for a detailed rule", func(t *testing.T) {
		out, _, err := runRule(t, root, "show", "no-fakes", "--format", "json")
		if err != nil {
			t.Fatal(err)
		}
		var doc ruleShowDoc
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatalf("stdout is not JSON: %v\n%s", err, out)
		}
		if doc.Slug != "no-fakes" || doc.Form != "detailed" || doc.DetailPath == "" {
			t.Fatalf("doc = %+v", doc)
		}
		if len(doc.PromotingLessons) != 1 || doc.PromotingLessons[0] != "kinder-fake" {
			t.Fatalf("promoting lessons = %v", doc.PromotingLessons)
		}
		if len(doc.Sources) != 1 || doc.Sources[0] != "lesson:kinder-fake" {
			t.Fatalf("sources = %v", doc.Sources)
		}
		if len(doc.UnresolvedLinks) != 0 {
			t.Fatalf("unresolved = %v", doc.UnresolvedLinks)
		}
	})

	t.Run("json for an inline rule carries no document fields", func(t *testing.T) {
		out, _, err := runRule(t, root, "show", "inline-one", "--format", "json")
		if err != nil {
			t.Fatal(err)
		}
		var doc ruleShowDoc
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatal(err)
		}
		if doc.Form != "inline" || doc.DetailPath != "" || doc.Why != "" {
			t.Fatalf("an inline rule must report no document fields: %+v", doc)
		}
		if doc.Skills == nil || doc.Scope == nil {
			t.Fatalf("nil slices in JSON output: %+v", doc)
		}
	})

	t.Run("text summary", func(t *testing.T) {
		out, _, err := runRule(t, root, "show", "no-fakes", "--format", "text")
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"Slug:", "Form:          detailed", "Statement:", "Promoted from: kinder-fake", "Detail:"} {
			if !strings.Contains(out, want) {
				t.Errorf("text output missing %q:\n%s", want, out)
			}
		}
		inline, _, err := runRule(t, root, "show", "inline-one", "--format", "text")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(inline, "Detail:") {
			t.Fatalf("an inline rule must not print document fields:\n%s", inline)
		}
	})

	t.Run("yaml is the default", func(t *testing.T) {
		out, _, err := runRule(t, root, "show", "no-fakes")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "slug: no-fakes") {
			t.Fatalf("default output is not YAML:\n%s", out)
		}
	})

	t.Run("reports an unresolvable source", func(t *testing.T) {
		if _, _, err := runRule(t, root, "new", "ghosted", "--source", "lesson:ghost"); err != nil {
			t.Fatal(err)
		}
		out, _, err := runRule(t, root, "show", "ghosted", "--format", "json")
		if err != nil {
			t.Fatal(err)
		}
		var doc ruleShowDoc
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatal(err)
		}
		if len(doc.UnresolvedLinks) != 1 || doc.UnresolvedLinks[0] != "lesson:ghost" {
			t.Fatalf("unresolved = %v", doc.UnresolvedLinks)
		}
	})

	t.Run("reports a citing feature", func(t *testing.T) {
		featureDir := filepath.Join(root, "spec", "features", "some-feature")
		if err := os.MkdirAll(featureDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(featureDir, "README.md"),
			[]byte("# Feature: Some\n\nBound by rule:inline-one.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		out, _, err := runRule(t, root, "show", "inline-one", "--format", "json")
		if err != nil {
			t.Fatal(err)
		}
		var doc ruleShowDoc
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatal(err)
		}
		if len(doc.CitingFeatures) != 1 || doc.CitingFeatures[0] != "some-feature" {
			t.Fatalf("citing features = %v", doc.CitingFeatures)
		}
	})

	t.Run("rejects", func(t *testing.T) {
		cases := []struct {
			args     []string
			wantCode int
		}{
			{args: []string{"show"}, wantCode: exitcode.InvalidArgs},
			{args: []string{"show", "a", "b"}, wantCode: exitcode.InvalidArgs},
			{args: []string{"show", "no-fakes", "--format", "toml"}, wantCode: exitcode.InvalidArgs},
			{args: []string{"show", "missing"}, wantCode: exitcode.NotFound},
		}
		for _, tc := range cases {
			out, _, err := runRule(t, root, tc.args...)
			if got := exitCodeOf(err); got != tc.wantCode {
				t.Errorf("%v exit = %d, want %d", tc.args, got, tc.wantCode)
			}
			if tc.wantCode == exitcode.NotFound && out != "" {
				t.Errorf("a missing rule must print nothing to stdout, got %q", out)
			}
		}
	})
}

// ----- update -----

func TestRuleUpdate(t *testing.T) {
	root := setupRuleProject(t)
	if _, _, err := runRule(t, root, "new", "x", "--statement", "Always x.", "--why", "Because."); err != nil {
		t.Fatal(err)
	}
	// A hand-written note must survive every edit.
	path := rule.DetailPath(rule.RulesDir(root), "x")
	body := strings.Replace(readRuleDetail(t, root, "x"), "## Open Questions", "<!-- keep me -->\n\n## Open Questions", 1)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("row edits mirror into the document", func(t *testing.T) {
		if _, _, err := runRule(t, root, "update", "x", "--status", "active"); err != nil {
			t.Fatalf("update: %v", err)
		}
		if !strings.Contains(readRuleIndex(t, root), "| Active |") {
			t.Fatalf("index row not updated:\n%s", readRuleIndex(t, root))
		}
		got := readRuleDetail(t, root, "x")
		if !strings.Contains(got, "status: Active") || !strings.Contains(got, "**Status:** Active") {
			t.Fatalf("document mirror not updated:\n%s", got)
		}
		if _, _, err := runRule(t, root, "lint"); err != nil {
			t.Fatalf("the pair is not lint-clean after an update: %v", err)
		}
	})

	t.Run("edits preserve unrelated bytes", func(t *testing.T) {
		if _, _, err := runRule(t, root, "update", "x", "--statement", "Never x.", "--title", "Renamed X"); err != nil {
			t.Fatalf("update: %v", err)
		}
		got := readRuleDetail(t, root, "x")
		if !strings.Contains(got, "<!-- keep me -->") {
			t.Fatalf("update discarded a hand-written note:\n%s", got)
		}
		if !strings.Contains(got, "# Rule: Renamed X") || !strings.Contains(got, "**Statement:** Never x.") {
			t.Fatalf("update did not apply:\n%s", got)
		}
	})

	t.Run("sources are incremental", func(t *testing.T) {
		if _, _, err := runRule(t, root, "update", "x", "--add-source", "decision:0001"); err != nil {
			t.Fatalf("add-source: %v", err)
		}
		if !strings.Contains(readRuleIndex(t, root), "decision:0001") {
			t.Fatalf("source not added:\n%s", readRuleIndex(t, root))
		}
		if _, _, err := runRule(t, root, "update", "x", "--remove-source", "decision:0001"); err != nil {
			t.Fatalf("remove-source: %v", err)
		}
		if !strings.Contains(readRuleDetail(t, root, "x"), "**Sources:** "+rule.Sentinel) {
			t.Fatalf("empty source list did not fall back to the sentinel:\n%s", readRuleDetail(t, root, "x"))
		}
	})

	t.Run("enforcement and control are validated together", func(t *testing.T) {
		if _, _, err := runRule(t, root, "update", "x", "--enforcement", "Enforced"); exitCodeOf(err) != exitcode.InvalidArgs {
			t.Fatalf("raising the tier without a control must exit 2, got %v", err)
		}
		if !strings.Contains(readRuleDetail(t, root, "x"), "**Enforcement:** Stated") {
			t.Fatal("a rejected update must leave the artifact untouched")
		}
		if _, _, err := runRule(t, root, "update", "x", "--enforcement", "Enforced", "--control", "wb hook"); err != nil {
			t.Fatalf("update: %v", err)
		}
		// Once a control exists, raising the tier alone is fine.
		if _, _, err := runRule(t, root, "update", "x", "--enforcement", "Automated"); err != nil {
			t.Fatalf("update with an existing control: %v", err)
		}
		// Clearing the control under a control-requiring tier is refused.
		if _, _, err := runRule(t, root, "update", "x", "--control", ""); exitCodeOf(err) != exitcode.InvalidArgs {
			t.Fatal("clearing the control of an Automated rule must exit 2")
		}
	})

	t.Run("scope replacement", func(t *testing.T) {
		if _, _, err := runRule(t, root, "update", "x", "--scope", "product:sneat", "--scope", "path:**/*.go"); err != nil {
			t.Fatalf("update: %v", err)
		}
		if !strings.Contains(readRuleIndex(t, root), "product:sneat, path:**/*.go") {
			t.Fatalf("scope not replaced:\n%s", readRuleIndex(t, root))
		}
	})

	t.Run("document-only edits", func(t *testing.T) {
		if _, _, err := runRule(t, root, "update", "x", "--why", "A better reason.", "--exceptions", ""); err != nil {
			t.Fatalf("update: %v", err)
		}
		got := readRuleDetail(t, root, "x")
		if !strings.Contains(got, "**Why:** A better reason.") || !strings.Contains(got, "**Exceptions:** none") {
			t.Fatalf("document-only edits not applied:\n%s", got)
		}
	})

	t.Run("document-only edits are refused on an inline rule", func(t *testing.T) {
		if _, _, err := runRule(t, root, "new", "inline-two", "--statement", "s"); err != nil {
			t.Fatal(err)
		}
		_, _, err := runRule(t, root, "update", "inline-two", "--why", "x")
		if got := exitCodeOf(err); got != exitcode.InvalidState {
			t.Fatalf("exit = %d, want %d (err=%v)", got, exitcode.InvalidState, err)
		}
		if !strings.Contains(err.Error(), "rule expand") {
			t.Fatalf("the error should name the repair verb: %v", err)
		}
	})

	t.Run("rejects", func(t *testing.T) {
		cases := []struct {
			name     string
			args     []string
			wantCode int
		}{
			{name: "missing slug", args: []string{"update"}, wantCode: exitcode.InvalidArgs},
			{name: "two slugs", args: []string{"update", "a", "b"}, wantCode: exitcode.InvalidArgs},
			{name: "no edit flags", args: []string{"update", "x"}, wantCode: exitcode.InvalidArgs},
			{name: "empty statement", args: []string{"update", "x", "--statement", ""}, wantCode: exitcode.InvalidArgs},
			{name: "empty why", args: []string{"update", "x", "--why", ""}, wantCode: exitcode.InvalidArgs},
			{name: "bad status", args: []string{"update", "x", "--status", "Pending"}, wantCode: exitcode.InvalidArgs},
			{name: "bad enforcement", args: []string{"update", "x", "--enforcement", "Mandatory"}, wantCode: exitcode.InvalidArgs},
			{name: "bad scope", args: []string{"update", "x", "--scope", "team:platform"}, wantCode: exitcode.InvalidArgs},
			{name: "duplicate source", args: []string{"update", "x", "--add-source", "decision:0002", "--add-source", "decision:0002"}, wantCode: exitcode.InvalidArgs},
			{name: "absent source removal", args: []string{"update", "x", "--remove-source", "decision:9999"}, wantCode: exitcode.InvalidArgs},
			{name: "bad format", args: []string{"update", "x", "--status", "Draft", "--format", "toml"}, wantCode: exitcode.InvalidArgs},
			{name: "missing rule", args: []string{"update", "ghost", "--status", "Draft"}, wantCode: exitcode.NotFound},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, _, err := runRule(t, root, tc.args...)
				if got := exitCodeOf(err); got != tc.wantCode {
					t.Fatalf("exit = %d, want %d (err=%v)", got, tc.wantCode, err)
				}
			})
		}
	})
}

// Updating supersession pointers writes both documents' fields.
func TestRuleUpdateSupersession(t *testing.T) {
	root := setupRuleProject(t)
	for _, slug := range []string{"old", "new"} {
		if _, _, err := runRule(t, root, "new", slug, "--statement", "s", "--detailed"); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := runRule(t, root, "update", "new", "--supersedes", "old"); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, _, err := runRule(t, root, "update", "old", "--superseded-by", "new", "--status", "Superseded"); err != nil {
		t.Fatalf("update: %v", err)
	}
	if !strings.Contains(readRuleDetail(t, root, "new"), "**Supersedes:** old") {
		t.Fatalf("supersedes not written:\n%s", readRuleDetail(t, root, "new"))
	}
	if _, _, err := runRule(t, root, "lint"); err != nil {
		t.Fatalf("a consistent supersession pair is not lint-clean: %v", err)
	}
}

// ----- promote -----

func TestRulePromote(t *testing.T) {
	root := setupRuleProject(t)
	writeCanonicalLesson(t, root, "kinder-fake", "Assert against the real adapter, never a hand-rolled fake.")

	out, _, err := runRule(t, root, "promote", "--from-lesson", "kinder-fake", "no-fakes", "--format", "json")
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	var result rulePromoteResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out)
	}
	// A promoted rule is detailed by default: the Lesson already carries the
	// reason a rule needs.
	if result.Slug != "no-fakes" || result.FromLesson != "kinder-fake" || result.Action != "promoted" || !result.Detailed {
		t.Fatalf("result = %+v", result)
	}
	body := readRuleDetail(t, root, "no-fakes")
	if !strings.Contains(body, "**Statement:** Assert against the real adapter, never a hand-rolled fake.") {
		t.Fatalf("statement not pre-filled from the Lesson's Control:\n%s", body)
	}
	if !strings.Contains(readRuleIndex(t, root), "lesson:kinder-fake") {
		t.Fatalf("promoted Lesson not recorded as a source:\n%s", readRuleIndex(t, root))
	}
	lessonBody, err := os.ReadFile(filepath.Join(root, "spec", "lessons", "kinder-fake", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(lessonBody), "**Promotes To:** rule:no-fakes") {
		t.Fatalf("Lesson not pointed at the rule:\n%s", lessonBody)
	}
	if _, _, err := runRule(t, root, "lint"); err != nil {
		t.Fatalf("promoted pair is not lint-clean: %v", err)
	}
}

func TestRulePromoteInline(t *testing.T) {
	root := setupRuleProject(t)
	writeCanonicalLesson(t, root, "kinder-fake", "Assert against the real adapter.")
	if _, _, err := runRule(t, root, "promote", "--from-lesson", "kinder-fake", "no-fakes", "--inline"); err != nil {
		t.Fatalf("promote --inline: %v", err)
	}
	if _, err := os.Stat(rule.DetailPath(rule.RulesDir(root), "no-fakes")); !os.IsNotExist(err) {
		t.Fatal("--inline must not write a detail document")
	}
	if _, _, err := runRule(t, root, "lint"); err != nil {
		t.Fatalf("an inline promotion is not lint-clean: %v", err)
	}
}

func TestRulePromoteOverridesAndTiers(t *testing.T) {
	root := setupRuleProject(t)
	writeCanonicalLesson(t, root, "kinder-fake", "Assert against the real adapter.")
	if _, _, err := runRule(t, root, "promote", "--from-lesson", "kinder-fake", "no-fakes",
		"--statement", "Never hand-roll a fake.", "--why", "Custom reason.",
		"--scope", "repo:specscore/specscore-cli", "--enforcement", "Enforced",
		"--source", "decision:0001", "--status", "Active"); err != nil {
		t.Fatalf("promote: %v", err)
	}
	body := readRuleDetail(t, root, "no-fakes")
	for _, want := range []string{
		"**Statement:** Never hand-roll a fake.",
		"**Why:** Custom reason.",
		"**Scope:** repo:specscore/specscore-cli",
		"**Enforcement:** Enforced",
		// The control defaults to the Lesson's Control when the tier needs one.
		"**Control:** Assert against the real adapter.",
		"**Sources:** lesson:kinder-fake, decision:0001",
		"**Status:** Active",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("promoted rule missing %q:\n%s", want, body)
		}
	}
}

func TestRulePromoteRejects(t *testing.T) {
	root := setupRuleProject(t)
	writeCanonicalLesson(t, root, "kinder-fake", "Assert against the real adapter.")
	writeCanonicalLesson(t, root, "other-lesson", "—")
	if _, _, err := runRule(t, root, "promote", "--from-lesson", "kinder-fake", "no-fakes"); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name     string
		args     []string
		wantCode int
	}{
		{name: "missing rule slug", args: []string{"promote", "--from-lesson", "kinder-fake"}, wantCode: exitcode.InvalidArgs},
		{name: "two rule slugs", args: []string{"promote", "--from-lesson", "kinder-fake", "a", "b"}, wantCode: exitcode.InvalidArgs},
		{name: "missing --from-lesson", args: []string{"promote", "x"}, wantCode: exitcode.InvalidArgs},
		{name: "invalid rule slug", args: []string{"promote", "--from-lesson", "kinder-fake", "Not A Slug"}, wantCode: exitcode.InvalidArgs},
		{name: "bad format", args: []string{"promote", "--from-lesson", "kinder-fake", "x", "--format", "toml"}, wantCode: exitcode.InvalidArgs},
		{name: "bad scope", args: []string{"promote", "--from-lesson", "other-lesson", "x", "--scope", "team:x"}, wantCode: exitcode.InvalidArgs},
		{name: "missing lesson", args: []string{"promote", "--from-lesson", "ghost", "x"}, wantCode: exitcode.NotFound},
		// A Lesson carries exactly one promotion pointer.
		{name: "lesson already promoted", args: []string{"promote", "--from-lesson", "kinder-fake", "another-rule"}, wantCode: exitcode.InvalidState},
		// And an existing rule slug is a conflict, not a silent overwrite.
		{name: "existing rule slug", args: []string{"promote", "--from-lesson", "other-lesson", "no-fakes"}, wantCode: exitcode.Conflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := runRule(t, root, tc.args...)
			if got := exitCodeOf(err); got != tc.wantCode {
				t.Fatalf("exit = %d, want %d (err=%v)", got, tc.wantCode, err)
			}
		})
	}
}

// Re-promoting the same Lesson to the same rule with --force is the idempotent
// repair path, not a second promotion.
func TestRulePromoteSameTargetWithForce(t *testing.T) {
	root := setupRuleProject(t)
	writeCanonicalLesson(t, root, "kinder-fake", "Assert against the real adapter.")
	if _, _, err := runRule(t, root, "promote", "--from-lesson", "kinder-fake", "no-fakes"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runRule(t, root, "promote", "--from-lesson", "kinder-fake", "no-fakes", "--force"); err != nil {
		t.Fatalf("re-promote with --force: %v", err)
	}
	if _, _, err := runRule(t, root, "lint"); err != nil {
		t.Fatalf("re-promoted pair is not lint-clean: %v", err)
	}
}

func TestRulePromoteYAMLAndText(t *testing.T) {
	for _, format := range []string{"yaml", "text"} {
		t.Run(format, func(t *testing.T) {
			root := setupRuleProject(t)
			writeCanonicalLesson(t, root, "kinder-fake", "Assert against the real adapter.")
			out, _, err := runRule(t, root, "promote", "--from-lesson", "kinder-fake", "no-fakes", "--format", format)
			if err != nil {
				t.Fatalf("promote: %v", err)
			}
			if !strings.Contains(out, "no-fakes") {
				t.Fatalf("%s output = %q", format, out)
			}
		})
	}
}

// ----- delete -----

func TestRuleDelete(t *testing.T) {
	t.Run("unlinked inline rule deletes cleanly", func(t *testing.T) {
		root := setupRuleProject(t)
		if _, _, err := runRule(t, root, "new", "x"); err != nil {
			t.Fatal(err)
		}
		if _, _, err := runRule(t, root, "delete", "x"); err != nil {
			t.Fatalf("delete: %v", err)
		}
		index := readRuleIndex(t, root)
		if strings.Contains(index, "| x |") {
			t.Fatalf("index row not removed:\n%s", index)
		}
		if !strings.Contains(index, rule.IndexEmptyPlaceholder) {
			t.Fatalf("placeholder not restored:\n%s", index)
		}
	})

	t.Run("detailed rule removes its directory", func(t *testing.T) {
		root := setupRuleProject(t)
		if _, _, err := runRule(t, root, "new", "x", "--detailed"); err != nil {
			t.Fatal(err)
		}
		if _, _, err := runRule(t, root, "delete", "x"); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := os.Stat(filepath.Join(rule.RulesDir(root), "x")); !os.IsNotExist(err) {
			t.Fatal("rule directory not removed")
		}
	})

	t.Run("refuses while a lesson still links", func(t *testing.T) {
		root := setupRuleProject(t)
		writeCanonicalLesson(t, root, "kinder-fake", "c")
		if _, _, err := runRule(t, root, "promote", "--from-lesson", "kinder-fake", "no-fakes"); err != nil {
			t.Fatal(err)
		}
		_, _, err := runRule(t, root, "delete", "no-fakes")
		if got := exitCodeOf(err); got != exitcode.InvalidState {
			t.Fatalf("exit = %d, want %d", got, exitcode.InvalidState)
		}
		if !strings.Contains(err.Error(), "lesson:kinder-fake") {
			t.Fatalf("error should name the blocker: %v", err)
		}
		if _, statErr := os.Stat(rule.DetailPath(rule.RulesDir(root), "no-fakes")); statErr != nil {
			t.Fatal("a refused delete must leave the artifact in place")
		}
	})

	t.Run("refuses while a skill still links", func(t *testing.T) {
		root := setupRuleProject(t)
		if _, _, err := runRule(t, root, "new", "x", "--detailed"); err != nil {
			t.Fatal(err)
		}
		writeRuleSkillFile(t, root, "go-hygiene", "---\nname: go-hygiene\n---\n\n# Go\n\n## Rules\n\n- rule:x\n")
		_, _, err := runRule(t, root, "delete", "x")
		if got := exitCodeOf(err); got != exitcode.InvalidState || !strings.Contains(err.Error(), "skill:go-hygiene") {
			t.Fatalf("exit = %d err = %v", got, err)
		}
	})

	t.Run("supersede-with repoints every incoming link", func(t *testing.T) {
		root := setupRuleProject(t)
		writeCanonicalLesson(t, root, "kinder-fake", "c")
		if _, _, err := runRule(t, root, "promote", "--from-lesson", "kinder-fake", "no-fakes"); err != nil {
			t.Fatal(err)
		}
		if _, _, err := runRule(t, root, "new", "successor", "--statement", "Never fake.", "--detailed"); err != nil {
			t.Fatal(err)
		}
		out, _, err := runRule(t, root, "delete", "no-fakes", "--supersede-with", "successor", "--format", "json")
		if err != nil {
			t.Fatalf("delete --supersede-with: %v", err)
		}
		var result ruleDeleteResult
		if err := json.Unmarshal([]byte(out), &result); err != nil {
			t.Fatalf("stdout is not JSON: %v\n%s", err, out)
		}
		if len(result.RewrittenLinks) != 1 || result.RewrittenLinks[0] != "lesson:kinder-fake" {
			t.Fatalf("rewritten = %v", result.RewrittenLinks)
		}
		lessonBody, _ := os.ReadFile(filepath.Join(root, "spec", "lessons", "kinder-fake", "README.md"))
		if !strings.Contains(string(lessonBody), "**Promotes To:** rule:successor") {
			t.Fatalf("lesson not repointed:\n%s", lessonBody)
		}
		if !strings.Contains(readRuleIndex(t, root), "lesson:kinder-fake") {
			t.Fatalf("successor did not inherit the source:\n%s", readRuleIndex(t, root))
		}
		if !strings.Contains(readRuleDetail(t, root, "successor"), "**Sources:** lesson:kinder-fake") {
			t.Fatalf("successor's document mirror not updated:\n%s", readRuleDetail(t, root, "successor"))
		}
		if _, _, err := runRule(t, root, "lint"); err != nil {
			t.Fatalf("repointed tree is not lint-clean: %v", err)
		}
	})

	t.Run("supersede-with repoints an inline successor", func(t *testing.T) {
		root := setupRuleProject(t)
		writeCanonicalLesson(t, root, "kinder-fake", "c")
		if _, _, err := runRule(t, root, "promote", "--from-lesson", "kinder-fake", "no-fakes", "--inline"); err != nil {
			t.Fatal(err)
		}
		if _, _, err := runRule(t, root, "new", "successor", "--statement", "Never fake."); err != nil {
			t.Fatal(err)
		}
		if _, _, err := runRule(t, root, "delete", "no-fakes", "--supersede-with", "successor"); err != nil {
			t.Fatalf("delete --supersede-with: %v", err)
		}
		if _, _, err := runRule(t, root, "lint"); err != nil {
			t.Fatalf("repointed tree is not lint-clean: %v", err)
		}
	})

	t.Run("supersede-with repoints rule relations", func(t *testing.T) {
		root := setupRuleProject(t)
		for _, slug := range []string{"old", "successor", "newer"} {
			if _, _, err := runRule(t, root, "new", slug, "--detailed"); err != nil {
				t.Fatal(err)
			}
		}
		if _, _, err := runRule(t, root, "update", "newer", "--supersedes", "old"); err != nil {
			t.Fatal(err)
		}
		if _, _, err := runRule(t, root, "delete", "old"); exitCodeOf(err) != exitcode.InvalidState {
			t.Fatal("a rule referenced by another rule must block deletion")
		}
		if _, _, err := runRule(t, root, "delete", "old", "--supersede-with", "successor"); err != nil {
			t.Fatalf("delete --supersede-with: %v", err)
		}
		if !strings.Contains(readRuleDetail(t, root, "newer"), "**Supersedes:** successor") {
			t.Fatalf("relation not repointed:\n%s", readRuleDetail(t, root, "newer"))
		}
	})

	t.Run("warns about prose citations it cannot rewrite", func(t *testing.T) {
		root := setupRuleProject(t)
		if _, _, err := runRule(t, root, "new", "x", "--detailed"); err != nil {
			t.Fatal(err)
		}
		if _, _, err := runRule(t, root, "new", "successor"); err != nil {
			t.Fatal(err)
		}
		featureDir := filepath.Join(root, "spec", "features", "some-feature")
		if err := os.MkdirAll(featureDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(featureDir, "README.md"),
			[]byte("# Feature: Some\n\nBound by rule:x.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, errOut, err := runRule(t, root, "delete", "x", "--supersede-with", "successor")
		if err != nil {
			t.Fatalf("delete: %v", err)
		}
		if !strings.Contains(errOut, "feature:some-feature") {
			t.Fatalf("a prose citation must be reported on stderr, got %q", errOut)
		}
	})

	t.Run("yaml output", func(t *testing.T) {
		root := setupRuleProject(t)
		if _, _, err := runRule(t, root, "new", "x"); err != nil {
			t.Fatal(err)
		}
		out, _, err := runRule(t, root, "delete", "x", "--format", "yaml")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "slug: x") {
			t.Fatalf("yaml output = %q", out)
		}
	})

	t.Run("rejects", func(t *testing.T) {
		root := setupRuleProject(t)
		if _, _, err := runRule(t, root, "new", "x"); err != nil {
			t.Fatal(err)
		}
		cases := []struct {
			name     string
			args     []string
			wantCode int
		}{
			{name: "missing slug", args: []string{"delete"}, wantCode: exitcode.InvalidArgs},
			{name: "two slugs", args: []string{"delete", "a", "b"}, wantCode: exitcode.InvalidArgs},
			{name: "self supersede", args: []string{"delete", "x", "--supersede-with", "x"}, wantCode: exitcode.InvalidArgs},
			{name: "unknown successor", args: []string{"delete", "x", "--supersede-with", "ghost"}, wantCode: exitcode.InvalidArgs},
			{name: "bad format", args: []string{"delete", "x", "--format", "toml"}, wantCode: exitcode.InvalidArgs},
			{name: "missing rule", args: []string{"delete", "ghost"}, wantCode: exitcode.NotFound},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, _, err := runRule(t, root, tc.args...)
				if got := exitCodeOf(err); got != tc.wantCode {
					t.Fatalf("exit = %d, want %d (err=%v)", got, tc.wantCode, err)
				}
			})
		}
	})
}

// ----- lint -----

func TestRuleLintCommand(t *testing.T) {
	root := setupRuleProject(t)
	if _, _, err := runRule(t, root, "new", "x", "--source", "lesson:ghost"); err != nil {
		t.Fatal(err)
	}

	t.Run("reports violations and exits nonzero", func(t *testing.T) {
		out, _, err := runRule(t, root, "lint")
		if got := exitCodeOf(err); got != exitcode.Conflict {
			t.Fatalf("exit = %d, want %d", got, exitcode.Conflict)
		}
		if !strings.Contains(out, "R-007") {
			t.Fatalf("stdout should name the failing rule:\n%s", out)
		}
	})

	t.Run("json output", func(t *testing.T) {
		out, _, _ := runRule(t, root, "lint", "--format", "json")
		var violations []map[string]any
		if err := json.Unmarshal([]byte(out), &violations); err != nil {
			t.Fatalf("stdout is not JSON: %v\n%s", err, out)
		}
		if len(violations) == 0 {
			t.Fatal("expected at least one violation in JSON output")
		}
	})

	t.Run("yaml output", func(t *testing.T) {
		out, _, _ := runRule(t, root, "lint", "--format", "yaml")
		if !strings.Contains(out, "R-007") {
			t.Fatalf("yaml output = %q", out)
		}
	})

	t.Run("clean tree exits zero", func(t *testing.T) {
		if _, _, err := runRule(t, root, "update", "x", "--remove-source", "lesson:ghost"); err != nil {
			t.Fatal(err)
		}
		out, _, err := runRule(t, root, "lint")
		if err != nil {
			t.Fatalf("lint: %v", err)
		}
		if strings.TrimSpace(out) != "" {
			t.Fatalf("clean lint should print nothing, got %q", out)
		}
	})

	t.Run("json on a clean tree is an empty list", func(t *testing.T) {
		out, _, err := runRule(t, root, "lint", "--format", "json")
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(out) != "[]" {
			t.Fatalf("json = %q, want []", out)
		}
	})

	t.Run("fix repairs a drifted index", func(t *testing.T) {
		if _, _, err := runRule(t, root, "new", "y", "--detailed"); err != nil {
			t.Fatal(err)
		}
		index := strings.Replace(readRuleIndex(t, root), "[y](y/README.md)", "y", 1)
		if err := os.WriteFile(rule.IndexPath(rule.RulesDir(root)), []byte(index), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := runRule(t, root, "lint"); exitCodeOf(err) != exitcode.Conflict {
			t.Fatal("a missing link must fail lint")
		}
		if _, _, err := runRule(t, root, "lint", "--fix"); err != nil {
			t.Fatalf("lint --fix: %v", err)
		}
		if !strings.Contains(readRuleIndex(t, root), "[y](y/README.md)") {
			t.Fatalf("--fix did not restore the link:\n%s", readRuleIndex(t, root))
		}
	})

	t.Run("bad format", func(t *testing.T) {
		if _, _, err := runRule(t, root, "lint", "--format", "toml"); exitCodeOf(err) != exitcode.InvalidArgs {
			t.Fatal("invalid --format must exit 2")
		}
	})
}

// ----- shared behaviour -----

// Read verbs must never mutate: listing or showing a rule leaves the tree
// byte-identical.
func TestRuleReadVerbsDoNotMutate(t *testing.T) {
	root := setupRuleProject(t)
	if _, _, err := runRule(t, root, "new", "x", "--detailed"); err != nil {
		t.Fatal(err)
	}
	before := readRuleDetail(t, root, "x") + readRuleIndex(t, root)
	for _, args := range [][]string{{"list"}, {"show", "x"}} {
		if _, _, err := runRule(t, root, args...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	if after := readRuleDetail(t, root, "x") + readRuleIndex(t, root); after != before {
		t.Fatal("a read verb mutated the tree")
	}
}

// Every verb resolves its project the same way, so a directory outside a
// SpecScore project fails identically everywhere.
func TestRuleVerbsRejectAnUnresolvableProject(t *testing.T) {
	outside := t.TempDir()
	for _, args := range [][]string{
		{"new", "x"}, {"expand", "x"}, {"list"}, {"show", "x"},
		{"update", "x", "--status", "Draft"}, {"delete", "x"},
		{"promote", "--from-lesson", "l", "x"}, {"lint"},
	} {
		if _, _, err := runRule(t, outside, args...); err == nil {
			t.Errorf("%v should fail outside a SpecScore project", args)
		}
	}
}

func TestResolveRulesDirUsesProjectFlag(t *testing.T) {
	root := setupRuleProject(t)
	got, err := resolveRulesDir(root)
	if err != nil {
		t.Fatalf("resolveRulesDir: %v", err)
	}
	if got != rule.RulesDir(root) {
		t.Fatalf("resolveRulesDir = %q, want %q", got, rule.RulesDir(root))
	}
	if _, err := resolveRulesDir(t.TempDir()); err == nil {
		t.Fatal("resolveRulesDir should reject a directory outside any SpecScore project")
	}
}

func TestWriteRuleResultFormats(t *testing.T) {
	result := ruleWriteResult{Slug: "x", Form: "inline", Path: "/p", Status: "Draft", Action: "created"}
	for _, tc := range []struct{ format, want string }{
		{"text", "/p\n"},
		{"json", `"slug": "x"`},
		{"yaml", "slug: x"},
	} {
		cmd := &cobra.Command{}
		var out bytes.Buffer
		cmd.SetOut(&out)
		if err := writeRuleResult(cmd, tc.format, result); err != nil {
			t.Fatalf("writeRuleResult(%s): %v", tc.format, err)
		}
		if !strings.Contains(out.String(), tc.want) {
			t.Errorf("writeRuleResult(%s) = %q, want it to contain %q", tc.format, out.String(), tc.want)
		}
	}
}

func TestRuleFormName(t *testing.T) {
	if ruleFormName(true) != "detailed" || ruleFormName(false) != "inline" {
		t.Fatal("ruleFormName is wrong")
	}
}
