package rule

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeSkill(t *testing.T, root, name, body string) string {
	t.Helper()
	dir := filepath.Join(root, DefaultSkillsPath, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const skillWithRules = `---
name: go-hygiene
description: Keep Go changes formatted.
---

# Go hygiene

Prose that mentions rule:not-a-declaration outside the section.

## Rules

- rule:gofmt-first
- rule:no-vendor-edits

## Notes

rule:also-not-a-declaration
`

func TestSkillsDirHonoursOverride(t *testing.T) {
	cases := []struct {
		name       string
		configured string
		want       string
	}{
		{name: "default", configured: "", want: filepath.Join("/proj", "ai", "skills")},
		{name: "relative override", configured: "tools/skills", want: filepath.Join("/proj", "tools", "skills")},
		{name: "slash-separated override", configured: "a/b/c", want: filepath.Join("/proj", "a", "b", "c")},
		{name: "absolute override", configured: "/shared/skills", want: filepath.Clean("/shared/skills")},
		{name: "whitespace is not a path", configured: "   ", want: filepath.Join("/proj", "ai", "skills")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SkillsDir("/proj", tc.configured); got != tc.want {
				t.Fatalf("SkillsDir(%q) = %q, want %q", tc.configured, got, tc.want)
			}
		})
	}
}

// Only references under `## Rules` are declarations. A `rule:` token in prose
// is discussion, and treating it as a declaration would make the reciprocity
// check fire on documentation.
func TestParseSkillReadsOnlyTheRulesSection(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "go-hygiene", skillWithRules)
	skills, err := DiscoverSkills(SkillsDir(root, ""))
	if err != nil {
		t.Fatalf("DiscoverSkills: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("DiscoverSkills = %v", skills)
	}
	s := skills[0]
	if s.Name != "go-hygiene" || s.RulesHeadingLine == 0 {
		t.Fatalf("skill = %+v", s)
	}
	if want := []string{"gofmt-first", "no-vendor-edits"}; !reflect.DeepEqual(s.RuleRefs, want) {
		t.Fatalf("rule refs = %v, want %v", s.RuleRefs, want)
	}
}

func TestDiscoverSkillsIgnoresNoise(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "zebra", "---\nname: zebra\n---\n\n# Zebra\n")
	writeSkill(t, root, "alpha", skillWithRules)
	// A directory with no SKILL.md, and a stray file, are both skipped.
	if err := os.MkdirAll(filepath.Join(root, DefaultSkillsPath, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, DefaultSkillsPath, "stray.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	skills, err := DiscoverSkills(SkillsDir(root, ""))
	if err != nil {
		t.Fatalf("DiscoverSkills: %v", err)
	}
	if len(skills) != 2 || skills[0].Name != "alpha" || skills[1].Name != "zebra" {
		t.Fatalf("DiscoverSkills = %v", skills)
	}
	byName, err := SkillsByName(SkillsDir(root, ""))
	if err != nil || byName["alpha"] == nil {
		t.Fatalf("SkillsByName = %v, %v", byName, err)
	}
}

// A repository with no skills has no pairs to check, which is not an error.
func TestDiscoverSkillsMissingDirectory(t *testing.T) {
	skills, err := DiscoverSkills(filepath.Join(t.TempDir(), "ai", "skills"))
	if err != nil || len(skills) != 0 {
		t.Fatalf("DiscoverSkills = %v, %v", skills, err)
	}
	byName, err := SkillsByName(filepath.Join(t.TempDir(), "ai", "skills"))
	if err != nil || len(byName) != 0 {
		t.Fatalf("SkillsByName = %v, %v", byName, err)
	}
}

func TestSetSkillRules(t *testing.T) {
	t.Run("rewrites an existing section", func(t *testing.T) {
		root := t.TempDir()
		path := writeSkill(t, root, "go-hygiene", skillWithRules)
		if err := SetSkillRules(path, []string{"zeta-rule", "alpha-rule"}); err != nil {
			t.Fatalf("SetSkillRules: %v", err)
		}
		got := string(mustRead(t, path))
		if !strings.Contains(got, "- rule:alpha-rule\n- rule:zeta-rule\n") {
			t.Fatalf("rules section not rewritten in sorted order:\n%s", got)
		}
		if strings.Contains(got, "gofmt-first") {
			t.Fatalf("stale reference retained:\n%s", got)
		}
		// Everything outside the section survives.
		for _, want := range []string{"# Go hygiene", "## Notes", "rule:also-not-a-declaration"} {
			if !strings.Contains(got, want) {
				t.Errorf("skill lost %q:\n%s", want, got)
			}
		}
	})

	t.Run("appends when absent", func(t *testing.T) {
		root := t.TempDir()
		path := writeSkill(t, root, "plain", "---\nname: plain\n---\n\n# Plain\n\nBody.\n")
		if err := SetSkillRules(path, []string{"x"}); err != nil {
			t.Fatalf("SetSkillRules: %v", err)
		}
		got := string(mustRead(t, path))
		if !strings.Contains(got, SkillRulesHeading) || !strings.Contains(got, "- rule:x") {
			t.Fatalf("rules section not appended:\n%s", got)
		}
		if !strings.Contains(got, "Body.") {
			t.Fatalf("skill body lost:\n%s", got)
		}
		// The appended section is re-read as declarations.
		skills, err := DiscoverSkills(SkillsDir(root, ""))
		if err != nil || len(skills) != 1 || !reflect.DeepEqual(skills[0].RuleRefs, []string{"x"}) {
			t.Fatalf("round trip = %v, %v", skills, err)
		}
	})

	t.Run("clearing empties the section", func(t *testing.T) {
		root := t.TempDir()
		path := writeSkill(t, root, "go-hygiene", skillWithRules)
		if err := SetSkillRules(path, nil); err != nil {
			t.Fatalf("SetSkillRules: %v", err)
		}
		skills, err := DiscoverSkills(SkillsDir(root, ""))
		if err != nil || len(skills) != 1 || len(skills[0].RuleRefs) != 0 {
			t.Fatalf("refs after clearing = %v, %v", skills, err)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		if err := SetSkillRules(filepath.Join(t.TempDir(), "SKILL.md"), []string{"x"}); err == nil {
			t.Fatal("SetSkillRules on a missing file should error")
		}
	})
}
