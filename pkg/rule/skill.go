package rule

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Rules and skills are two halves of the same instruction surface: a skill
// tells an agent how to do a job, a rule tells it what it must never do while
// doing that job. Linking them in both directions is what stops a skill from
// silently outliving the rule that constrains it — the same reciprocity the
// lesson<->rule pair enforces.

// DefaultSkillsPath is the repository-relative directory scanned for skills
// when specscore.yaml declares no override.
const DefaultSkillsPath = "ai/skills"

// SkillRulesHeading is the H2 a skill uses to declare the rules that bind it.
const SkillRulesHeading = "## Rules"

// Skill is a discovered agent skill and the rule slugs it declares.
type Skill struct {
	Name string // the containing directory name, which is also the skill name
	Path string // absolute path to SKILL.md
	// RuleRefs are the `rule:<slug>` references listed under `## Rules`,
	// deduplicated and sorted.
	RuleRefs []string
	// RulesHeadingLine is the 1-based line of `## Rules`, or 0 when absent.
	RulesHeadingLine int
}

// ruleRefRe matches a `rule:<slug>` reference.
var ruleRefRe = regexp.MustCompile(`\brule:([a-z0-9]+(?:-[a-z0-9]+)*)\b`)

// SkillsDir resolves the skills directory for a project root, honouring the
// optional `rules.skills_path` override.
func SkillsDir(projectRoot, configuredPath string) string {
	rel := strings.TrimSpace(configuredPath)
	if rel == "" {
		rel = DefaultSkillsPath
	}
	if filepath.IsAbs(rel) {
		return filepath.Clean(rel)
	}
	return filepath.Join(projectRoot, filepath.FromSlash(rel))
}

// DiscoverSkills reads every <skillsDir>/<name>/SKILL.md. A missing directory
// is not an error: a repository with no skills simply has no pairs to check.
func DiscoverSkills(skillsDir string) ([]*Skill, error) {
	entries, err := osReadDir(skillsDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(skillsDir, e.Name(), "SKILL.md")
		content, readErr := osReadFile(path)
		if readErr != nil {
			continue
		}
		out = append(out, parseSkill(e.Name(), path, string(content)))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// SkillsByName is DiscoverSkills keyed by skill name.
func SkillsByName(skillsDir string) (map[string]*Skill, error) {
	skills, err := DiscoverSkills(skillsDir)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*Skill, len(skills))
	for _, s := range skills {
		out[s.Name] = s
	}
	return out, nil
}

// parseSkill extracts the rule references a skill declares. Only references
// under the `## Rules` heading count: a `rule:` token in prose elsewhere is
// discussion, not a declaration, and treating it as one would make the
// reciprocity check fire on documentation.
func parseSkill(name, path, content string) *Skill {
	s := &Skill{Name: name, Path: path}
	inRules := false
	seen := map[string]bool{}
	for i, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == SkillRulesHeading {
			inRules = true
			s.RulesHeadingLine = i + 1
			continue
		}
		if inRules && strings.HasPrefix(line, "## ") {
			break
		}
		if !inRules {
			continue
		}
		for _, m := range ruleRefRe.FindAllStringSubmatch(line, -1) {
			seen[m[1]] = true
		}
	}
	for slug := range seen {
		s.RuleRefs = append(s.RuleRefs, slug)
	}
	sort.Strings(s.RuleRefs)
	return s
}

// SetSkillRules rewrites a skill's `## Rules` section to list exactly slugs,
// appending the section when absent. It preserves every other byte of the
// skill, because a skill file is hand-written instruction text that a rule verb
// has no business reformatting.
func SetSkillRules(skillPath string, slugs []string) error {
	content, err := osReadFile(skillPath)
	if err != nil {
		return err
	}
	ordered := append([]string(nil), slugs...)
	sort.Strings(ordered)
	refs := make([]string, 0, len(ordered))
	for _, slug := range ordered {
		refs = append(refs, "- rule:"+slug)
	}

	lines := strings.Split(string(content), "\n")
	start, end := -1, len(lines)
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == SkillRulesHeading && start == -1 {
			start = i
			continue
		}
		if start != -1 && strings.HasPrefix(line, "## ") {
			end = i
			break
		}
	}

	block := append([]string{SkillRulesHeading, ""}, refs...)
	block = append(block, "")

	var out []string
	if start == -1 {
		out = append(out, lines...)
		for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
			out = out[:len(out)-1]
		}
		out = append(out, "")
		out = append(out, block...)
	} else {
		out = append(out, lines[:start]...)
		out = append(out, block...)
		out = append(out, lines[end:]...)
	}
	return WriteFileAtomic(skillPath, []byte(strings.Join(out, "\n")))
}
