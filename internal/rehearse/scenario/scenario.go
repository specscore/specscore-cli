// Package scenario parses markdown rehearse scenario files: body metadata
// (`**Verifies:**` / `**Status:**`) and fenced step blocks with info-string
// params (REQ: scenario-shape). Parsing only — no execution.
package scenario

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Block is one fenced block extracted from a scenario file.
type Block struct {
	// Kind is the first info-string token (e.g. "bash", "sql"). Empty for a
	// bare ``` fence.
	Kind string
	// Params holds the remaining info-string tokens as key=value pairs; a
	// bare token maps to the empty string.
	Params map[string]string
	// Body is the verbatim block content (without the fence lines).
	Body string
	// Line is the 1-based line number of the opening fence.
	Line int
}

// FileAssertion is one `### Assert: file` heading parsed from a scenario.
type FileAssertion struct {
	// Path is the file path (relative or absolute) extracted from the heading.
	Path string
	// Kind is one of: exists, missing, contains, not-contains, permissions.
	Kind string
	// Expected is the content of the code block immediately following the
	// heading, if any (empty for kinds that do not require one, e.g. exists).
	Expected string
}

// Scenario is one parsed scenario file.
type Scenario struct {
	// Path is the file the scenario was parsed from.
	Path string
	// Verifies lists the AC ids from the `**Verifies:**` body metadata,
	// parenthetical annotations stripped. Never nil.
	Verifies []string
	// Status is the `**Status:**` body metadata value (e.g. "pending").
	Status string
	// Expect is the `**Expect:**` body metadata value: "fail" when the scenario
	// is expected to fail, "pass" otherwise (the default, and the value for any
	// unrecognized directive). Feature: cli/rehearse/expected-fail.
	Expect string
	// Blocks are all fenced blocks in document order.
	Blocks []Block
	// FileAssertions are all `### Assert: file` headings in document order.
	// Never nil.
	FileAssertions []FileAssertion
	// Uses are all `**Use:**` reusable-check invocations in document order.
	// Feature: cli/rehearse/reusable-checks. Never nil.
	Uses []CheckUse
}

// CheckUse is one `**Use:** [label](url) with name=value …` invocation of a
// reusable check. Feature: cli/rehearse/reusable-checks.
type CheckUse struct {
	// Ref is the check URL from the Markdown link (relative to the scenario).
	Ref string
	// Params maps each `name=value` binding; values may be `{{context}}` refs.
	Params map[string]string
	// Line is the 1-based line number of the directive.
	Line int
}

// Check is a parsed reusable check (`<slug>.check.md`): a named, parameterized
// verification unit. Feature: cli/rehearse/reusable-checks.
type Check struct {
	// Slug is the check id from its `# Check: <slug>` heading (may be empty).
	Slug string
	// Params are the names declared on the `**Params:**` line. Never nil.
	Params []string
	// Body is the verbatim content of the check's first fenced block.
	Body string
}

// parenthetical matches one "(...)" annotation inside a Verifies value; the
// committed corpus writes `<ac-id> (REQ: <slug>)` and the annotation is
// stripped when parsing (REQ: scenario-shape).
var parenthetical = regexp.MustCompile(`\([^)]*\)`)

// Parse reads and parses the scenario file at path.
func Parse(path string) (*Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading scenario: %w", err)
	}
	return ParseBytes(path, data)
}

// ParseBytes parses scenario content already in memory. path is used for
// error messages only.
func ParseBytes(path string, data []byte) (*Scenario, error) {
	sc := &Scenario{Path: path, Verifies: []string{}, FileAssertions: []FileAssertion{}, Uses: []CheckUse{}, Expect: "pass"}

	var (
		inFence   bool
		openLine  int
		current   Block
		bodyLines []string
	)
	for i, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if inFence {
			if isClosingFence(trimmed) {
				current.Body = strings.Join(bodyLines, "\n")
				sc.Blocks = append(sc.Blocks, current)
				inFence = false
				continue
			}
			bodyLines = append(bodyLines, line)
			continue
		}
		if strings.HasPrefix(trimmed, "```") {
			inFence = true
			openLine = i + 1
			kind, params := parseInfoString(strings.TrimLeft(trimmed, "`"))
			current = Block{Kind: kind, Params: params, Line: openLine}
			bodyLines = nil
			continue
		}
		if v, ok := metaValue(trimmed, "Verifies"); ok && len(sc.Verifies) == 0 {
			sc.Verifies = parseVerifies(v)
		}
		if v, ok := metaValue(trimmed, "Status"); ok && sc.Status == "" {
			sc.Status = v
		}
		if v, ok := metaValue(trimmed, "Expect"); ok {
			// Only "fail" is meaningful; anything else keeps the "pass" default,
			// so the directive is forward-compatible with unknown values.
			if strings.EqualFold(strings.TrimSpace(v), "fail") {
				sc.Expect = "fail"
			}
		}
		if v, ok := metaValue(trimmed, "Use"); ok {
			use, uerr := parseUse(v)
			if uerr != nil {
				return nil, fmt.Errorf("%s:%d: %v", path, i+1, uerr)
			}
			use.Line = i + 1
			sc.Uses = append(sc.Uses, use)
		}
	}
	if inFence {
		return nil, fmt.Errorf("%s: unclosed fenced block opened at line %d", path, openLine)
	}
	sc.FileAssertions = parseFileAssertions(string(data))
	return sc, nil
}

// fileAssertionHeading matches `### Assert: file \`<path>\` <kind>`.
// Group 1 = path (contents of backticks), group 2 = kind (remaining token).
var fileAssertionHeading = regexp.MustCompile("(?m)^### Assert: file `([^`]+)` (\\S+)")

// parseFileAssertions scans raw markdown for `### Assert: file` H3 headings
// and extracts path, kind, and (optionally) the code block that immediately
// follows each heading. It stops collecting code-block content at the next
// `###` heading or at end of file.
func parseFileAssertions(content string) []FileAssertion {
	assertions := []FileAssertion{}
	lines := strings.Split(content, "\n")
	n := len(lines)
	i := 0
	for i < n {
		line := lines[i]
		m := fileAssertionHeading.FindStringSubmatch(line)
		if m == nil {
			i++
			continue
		}
		fa := FileAssertion{Path: m[1], Kind: m[2]}

		// Scan forward past blank lines to find an optional code fence.
		j := i + 1
		for j < n && strings.TrimSpace(lines[j]) == "" {
			j++
		}
		// If the next non-blank line is a `###` heading (or end), no code block.
		if j < n && strings.HasPrefix(strings.TrimSpace(lines[j]), "```") {
			// Collect fence body until closing fence or next ### heading.
			var bodyLines []string
			j++ // skip opening fence
			for j < n {
				t := strings.TrimSpace(lines[j])
				if strings.HasPrefix(t, "###") || isClosingFence(t) {
					break
				}
				bodyLines = append(bodyLines, lines[j])
				j++
			}
			fa.Expected = strings.Join(bodyLines, "\n")
			// Trim a trailing newline that strings.Join may have included if
			// the last body line was empty.
			fa.Expected = strings.TrimRight(fa.Expected, "\n")
			if isClosingFence(strings.TrimSpace(lines[j])) {
				j++ // skip closing fence
			}
		}
		assertions = append(assertions, fa)
		i = j
	}
	return assertions
}

// isClosingFence reports whether a trimmed line is a closing code fence: at
// least three characters, all backticks.
func isClosingFence(trimmed string) bool {
	return len(trimmed) >= 3 && strings.Trim(trimmed, "`") == ""
}

// parseInfoString splits a fence info string into the block kind (first
// token) and key=value params (remaining tokens; bare tokens map to "").
func parseInfoString(info string) (string, map[string]string) {
	params := map[string]string{}
	fields := strings.Fields(info)
	if len(fields) == 0 {
		return "", params
	}
	for _, f := range fields[1:] {
		key, value, _ := strings.Cut(f, "=")
		params[key] = value
	}
	return fields[0], params
}

// metaValue extracts the value of a `**<key>:** <value>` body-metadata line.
func metaValue(line, key string) (string, bool) {
	prefix := "**" + key + ":**"
	if !strings.HasPrefix(line, prefix) {
		return "", false
	}
	return strings.TrimSpace(line[len(prefix):]), true
}

// parseVerifies parses the Verifies metadata value: parenthetical
// annotations are stripped, then the remainder is a comma-separated list of
// AC ids. The em-dash placeholder ("—") means "none".
func parseVerifies(value string) []string {
	ids := []string{}
	for _, part := range strings.Split(parenthetical.ReplaceAllString(value, ""), ",") {
		id := strings.TrimSpace(part)
		if id == "" || id == "—" {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

// useLink matches the Markdown link in a `**Use:**` directive; group 1 is the
// check URL. Feature: cli/rehearse/reusable-checks (REQ: use-directive).
var useLink = regexp.MustCompile(`\[[^\]]*\]\(([^)]+)\)`)

// parseUse parses a `**Use:** [label](url) with a=1 b=2` directive value into a
// CheckUse. The link target is the check ref; `with` params are space-separated
// name=value pairs (values may be `{{context}}` refs). A value with no link is
// malformed (REQ: use-validation).
func parseUse(value string) (CheckUse, error) {
	loc := useLink.FindStringSubmatchIndex(value)
	if loc == nil {
		return CheckUse{}, fmt.Errorf("malformed **Use:** directive (need a [label](url) link): %q", strings.TrimSpace(value))
	}
	use := CheckUse{Ref: value[loc[2]:loc[3]], Params: map[string]string{}}
	rest := value[loc[1]:]
	if i := strings.Index(rest, "with"); i >= 0 {
		for _, tok := range strings.Fields(rest[i+len("with"):]) {
			if k, v, ok := strings.Cut(tok, "="); ok && k != "" {
				use.Params[k] = v
			}
		}
	}
	return use, nil
}

// checkHeading matches a check file's `# Check: <slug>` heading; group 1 is the
// slug. Feature: cli/rehearse/reusable-checks (REQ: check-artifact).
var checkHeading = regexp.MustCompile(`(?m)^#\s*Check:\s*(\S+)`)

// ParseCheck reads and parses the reusable-check file at path.
func ParseCheck(path string) (*Check, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading check: %w", err)
	}
	return ParseCheckBytes(path, data)
}

// ParseCheckBytes parses check content already in memory: the `# Check:` slug,
// the `**Params:**` declaration, and the body of the first fenced block (the
// verification). A check with no fenced block is an error.
func ParseCheckBytes(path string, data []byte) (*Check, error) {
	ch := &Check{Params: []string{}}
	if m := checkHeading.FindSubmatch(data); m != nil {
		ch.Slug = string(m[1])
	}
	var (
		inFence bool
		body    []string
		hasBody bool
	)
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if inFence {
			if isClosingFence(trimmed) {
				break
			}
			body = append(body, line)
			continue
		}
		if strings.HasPrefix(trimmed, "```") {
			inFence = true
			hasBody = true
			continue
		}
		if v, ok := metaValue(trimmed, "Params"); ok && len(ch.Params) == 0 {
			for _, p := range strings.Split(v, ",") {
				if p = strings.TrimSpace(p); p != "" {
					ch.Params = append(ch.Params, p)
				}
			}
		}
	}
	if !hasBody {
		return nil, fmt.Errorf("%s: check has no verification block", path)
	}
	ch.Body = strings.Join(body, "\n")
	return ch, nil
}
