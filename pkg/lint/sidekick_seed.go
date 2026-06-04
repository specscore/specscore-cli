package lint

// Sidekick-seed lint rule.
//
// specscore: https://specscore.studio/app/github.com/specscore/specstudio-skills/spec/features/sidekick-capture#req-seed-lint-rule
//
// Validates files matching `spec/ideas/seeds/*.md` against the
// `sidekick-seed` document-type contract defined in the upstream
// `sidekick-capture` Feature (REQ seed-frontmatter-schema, REQ
// seed-lint-rule). Each violation surfaces under the single rule name
// `sidekick-seed`.
//
// Promoted sidekick-seeds parked under `spec/ideas/archived/` are also in
// scope: any archived file whose frontmatter declares `type: sidekick-seed`
// is validated as a seed (and excluded from Idea discovery). Such seeds may
// carry the optional `promoted_to` frontmatter key. Archived files that are
// not seeds (genuine archived Ideas) are left untouched by this rule.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	sidekickSeedRule         = "sidekick-seed"
	sidekickSeedTypeValue    = "sidekick-seed"
	sidekickSeedMaxBodyChars = 2000
)

// sidekickSeedRequiredKeys names the exact 8 keys allowed in a seed's
// frontmatter. The list is closed: missing keys are rejected (3b) and
// extra keys are rejected (3a).
var sidekickSeedRequiredKeys = []string{
	"type",
	"slug",
	"captured_at",
	"captured_by",
	"captured_during",
	"trigger",
	"status",
	"synchestra_task",
}

var sidekickSeedRequiredKeySet = func() map[string]bool {
	m := make(map[string]bool, len(sidekickSeedRequiredKeys))
	for _, k := range sidekickSeedRequiredKeys {
		m[k] = true
	}
	return m
}()

// sidekickSeedOptionalKeys names keys permitted in a seed's frontmatter
// beyond the required set. A promoted seed (parked under
// `spec/ideas/archived/`) records `promoted_to: <slug>`, which must not be
// flagged as an unknown key.
var sidekickSeedOptionalKeys = map[string]bool{
	"promoted_to": true,
}

// sidekickSeedTriggerValues enumerates the values allowed for the
// `trigger` key per REQ seed-frontmatter-schema.
var sidekickSeedTriggerValues = map[string]bool{
	"heuristic": true,
	"explicit":  true,
}

type sidekickSeedChecker struct{}

func newSidekickSeedChecker() checker { return &sidekickSeedChecker{} }

func (c *sidekickSeedChecker) name() string     { return sidekickSeedRule }
func (c *sidekickSeedChecker) severity() string { return "error" }

func (c *sidekickSeedChecker) check(specRoot string) ([]Violation, error) {
	var violations []Violation

	// seeds/: every *.md is a seed by location.
	seedsDir := filepath.Join(specRoot, "ideas", "seeds")
	vs, err := scanSeedDir(specRoot, seedsDir, false)
	if err != nil {
		return nil, err
	}
	violations = append(violations, vs...)

	// archived/: only files whose frontmatter declares type: sidekick-seed
	// are promoted seeds; genuine archived Ideas are left to the Idea rules.
	archivedDir := filepath.Join(specRoot, "ideas", "archived")
	vs, err = scanSeedDir(specRoot, archivedDir, true)
	if err != nil {
		return nil, err
	}
	violations = append(violations, vs...)

	return violations, nil
}

// scanSeedDir validates seed files in dir. When seedsOnly is true, only
// files whose frontmatter declares `type: sidekick-seed` are considered
// (other files are skipped); otherwise every *.md is treated as a seed.
// A missing directory yields no violations.
func scanSeedDir(specRoot, dir string, seedsOnly bool) ([]Violation, error) {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var violations []Violation
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "README.md" || !strings.HasSuffix(name, ".md") {
			continue
		}
		path := filepath.Join(dir, name)
		rel, _ := filepath.Rel(specRoot, path)
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			violations = append(violations, Violation{
				File:     rel,
				Line:     0,
				Severity: "error",
				Rule:     sidekickSeedRule,
				Message:  fmt.Sprintf("cannot read seed file: %v", readErr),
			})
			continue
		}
		if seedsOnly && !frontmatterDeclaresSeed(string(content)) {
			continue
		}
		violations = append(violations, checkSidekickSeed(rel, string(content))...)
	}
	return violations, nil
}

// frontmatterDeclaresSeed reports whether content's leading YAML
// frontmatter sets `type` to the sidekick-seed literal. Files without a
// closing frontmatter fence, or with a non-seed type, return false.
func frontmatterDeclaresSeed(content string) bool {
	front, _, ok := splitFrontmatter(content)
	if !ok {
		return false
	}
	keys, _, err := parseFrontmatterKeys(front)
	if err != nil {
		return false
	}
	return keys["type"] == sidekickSeedTypeValue
}

// checkSidekickSeed validates one seed's content. relPath is included in
// every returned violation. The function emits at most one violation per
// rejection path (a–f) so that a fully-malformed file produces a focused
// punch list rather than a cascade of duplicates.
func checkSidekickSeed(relPath, content string) []Violation {
	var vs []Violation

	front, body, ok := splitFrontmatter(content)
	if !ok {
		vs = append(vs, Violation{
			File:     relPath,
			Line:     0,
			Severity: "error",
			Rule:     sidekickSeedRule,
			Message:  "missing YAML frontmatter (file must begin with `---` and contain a closing `---`)",
		})
		return vs
	}

	keys, keyOrder, parseErr := parseFrontmatterKeys(front)
	if parseErr != nil {
		vs = append(vs, Violation{
			File:     relPath,
			Line:     0,
			Severity: "error",
			Rule:     sidekickSeedRule,
			Message:  fmt.Sprintf("invalid YAML frontmatter: %v", parseErr),
		})
		return vs
	}

	// (a) unknown frontmatter keys.
	var unknown []string
	for _, k := range keyOrder {
		if !sidekickSeedRequiredKeySet[k] && !sidekickSeedOptionalKeys[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		vs = append(vs, Violation{
			File:     relPath,
			Line:     0,
			Severity: "error",
			Rule:     sidekickSeedRule,
			Message:  fmt.Sprintf("unknown frontmatter key(s): %s", strings.Join(unknown, ", ")),
		})
	}

	// (b) missing required keys.
	var missing []string
	for _, k := range sidekickSeedRequiredKeys {
		if _, ok := keys[k]; !ok {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		vs = append(vs, Violation{
			File:     relPath,
			Line:     0,
			Severity: "error",
			Rule:     sidekickSeedRule,
			Message:  fmt.Sprintf("missing required frontmatter key(s): %s", strings.Join(missing, ", ")),
		})
	}

	// (c) `type` must be the literal `sidekick-seed`.
	if t, present := keys["type"]; present && t != sidekickSeedTypeValue {
		vs = append(vs, Violation{
			File:     relPath,
			Line:     0,
			Severity: "error",
			Rule:     sidekickSeedRule,
			Message:  fmt.Sprintf("type must be %q; got %q", sidekickSeedTypeValue, t),
		})
	}

	// (d) `trigger` must be heuristic|explicit.
	if tr, present := keys["trigger"]; present && !sidekickSeedTriggerValues[tr] {
		vs = append(vs, Violation{
			File:     relPath,
			Line:     0,
			Severity: "error",
			Rule:     sidekickSeedRule,
			Message:  fmt.Sprintf("trigger must be one of {heuristic, explicit}; got %q", tr),
		})
	}

	// (e) body's first non-blank line must be an H1.
	if !bodyFirstLineIsH1(body) {
		vs = append(vs, Violation{
			File:     relPath,
			Line:     0,
			Severity: "error",
			Rule:     sidekickSeedRule,
			Message:  "body's first non-blank line must be an H1 heading (`# <text>`)",
		})
	}

	// (f) body length cap.
	if len(body) > sidekickSeedMaxBodyChars {
		vs = append(vs, Violation{
			File:     relPath,
			Line:     0,
			Severity: "error",
			Rule:     sidekickSeedRule,
			Message:  fmt.Sprintf("body exceeds %d characters (got %d)", sidekickSeedMaxBodyChars, len(body)),
		})
	}

	return vs
}

// splitFrontmatter extracts a leading YAML frontmatter block delimited by
// `---` lines. Returns (frontmatterBody, bodyAfter, ok). `ok` is false
// when the file does not start with `---` or when no closing `---` is
// found. `body` is everything after the closing `---` line (without a
// leading newline removal), so its length matches the contract phrasing
// "after the frontmatter, inclusive of the H1 line".
func splitFrontmatter(content string) (front, body string, ok bool) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != "---" {
		return "", "", false
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == "---" {
			front = strings.Join(lines[1:i], "\n")
			body = strings.Join(lines[i+1:], "\n")
			return front, body, true
		}
	}
	return "", "", false
}

// parseFrontmatterKeys parses a top-level YAML mapping and returns the
// key/value pairs (values stringified) along with the order in which keys
// appeared. An empty frontmatter is treated as a valid empty mapping.
func parseFrontmatterKeys(front string) (map[string]string, []string, error) {
	keys := map[string]string{}
	var order []string

	if strings.TrimSpace(front) == "" {
		return keys, order, nil
	}

	var node yaml.Node
	if err := yaml.Unmarshal([]byte(front), &node); err != nil {
		return nil, nil, err
	}
	if len(node.Content) == 0 {
		return keys, order, nil
	}
	root := node.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("frontmatter must be a YAML mapping")
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		k := root.Content[i].Value
		v := root.Content[i+1].Value
		keys[k] = v
		order = append(order, k)
	}
	return keys, order, nil
}

// bodyFirstLineIsH1 returns true when the first non-blank line of body
// begins with `# ` (an ATX H1). Blank lines are lines that contain only
// whitespace.
func bodyFirstLineIsH1(body string) bool {
	for _, ln := range strings.Split(body, "\n") {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		return strings.HasPrefix(ln, "# ")
	}
	return false
}
