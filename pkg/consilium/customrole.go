package consilium

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// customRoleSchemaVersion is the only Output Schema Version a custom-role
// markdown file may declare, per REQ:custom-role-markdown-contract.
const customRoleSchemaVersion = "1"

// metaLineRE matches a body-metadata line of the form `**Key:** value`,
// capturing the key and the trimmed value. The value may be empty.
var metaLineRE = regexp.MustCompile(`(?m)^\s*\*\*([^:*]+):\*\*\s*(.*?)\s*$`)

// fencedYAMLRE captures the contents of the first fenced code block (``` or
// ```yaml) within a section. Tilde fences are not part of the contract.
var fencedYAMLRE = regexp.MustCompile("(?s)```[a-zA-Z]*\\n(.*?)```")

// ParseCustomRole reads the markdown file at path and validates it against
// REQ:custom-role-markdown-contract: body-metadata `**Name:**` (matching the
// filename without `.md`), `**Group:**` in the three-value enum,
// `**Output Schema Version:** 1`; a `## Role Prompt` H2; and a `## Example
// Vote` H2 carrying one valid vote (parsed via ParseVotes). On success it
// returns the role as a RosterEntry whose Path is the input path. Any
// violation yields a single error naming the missing/invalid field AND the
// file path.
func ParseCustomRole(path string) (RosterEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RosterEntry{}, fmt.Errorf("custom role %s: read: %w", path, err)
	}
	body := string(data)

	meta := parseMeta(body)

	name, ok := meta["Name"]
	if !ok || name == "" {
		return RosterEntry{}, fieldErr(path, "Name", "missing required body-metadata line `**Name:** <kebab-slug>`")
	}
	wantName := strings.TrimSuffix(filepath.Base(path), ".md")
	if name != wantName {
		return RosterEntry{}, fieldErr(path, "Name",
			fmt.Sprintf("value %q must match the filename without .md (%q)", name, wantName))
	}

	group, ok := meta["Group"]
	if !ok || group == "" {
		return RosterEntry{}, fieldErr(path, "Group", "missing required body-metadata line `**Group:** builders|customers|adversaries`")
	}
	if !validGroup(group) {
		return RosterEntry{}, fieldErr(path, "Group",
			fmt.Sprintf("out-of-enum value %q; must be one of %s", group, groupEnumList()))
	}

	version, ok := meta["Output Schema Version"]
	if !ok || version == "" {
		return RosterEntry{}, fieldErr(path, "Output Schema Version", "missing required body-metadata line `**Output Schema Version:** 1`")
	}
	if version != customRoleSchemaVersion {
		return RosterEntry{}, fieldErr(path, "Output Schema Version",
			fmt.Sprintf("value %q; only %q is supported", version, customRoleSchemaVersion))
	}

	if !hasH2(body, "Role Prompt") {
		return RosterEntry{}, fieldErr(path, "Role Prompt", "missing required `## Role Prompt` section")
	}

	exampleVote := sectionBody(body, "Example Vote")
	if exampleVote == "" {
		return RosterEntry{}, fieldErr(path, "Example Vote", "missing required `## Example Vote` section")
	}
	m := fencedYAMLRE.FindStringSubmatch(exampleVote)
	if m == nil {
		return RosterEntry{}, fieldErr(path, "Example Vote", "section must contain a fenced YAML vote block")
	}
	if _, err := ParseVotes([]byte(m[1])); err != nil {
		return RosterEntry{}, fieldErr(path, "Example Vote", fmt.Sprintf("invalid vote: %v", err))
	}

	return RosterEntry{Name: name, Group: Group(group), Path: path}, nil
}

// fieldErr builds the single, deterministic error for a custom-role contract
// violation. It always names the offending field and the file path so the
// loader-rejection ACs can string-match on both.
func fieldErr(path, field, rule string) error {
	return fmt.Errorf("custom role %s: %s: %s", path, field, rule)
}

// parseMeta extracts every `**Key:** value` body-metadata line into a map. A
// key seen more than once keeps its first value; that ambiguity is not part of
// the contract and is not reported here.
func parseMeta(body string) map[string]string {
	out := make(map[string]string)
	for _, m := range metaLineRE.FindAllStringSubmatch(body, -1) {
		key := strings.TrimSpace(m[1])
		if _, seen := out[key]; seen {
			continue
		}
		out[key] = m[2]
	}
	return out
}

// hasH2 reports whether body contains an H2 heading (`## <title>`) with the
// given title (exact, after trimming surrounding whitespace).
func hasH2(body, title string) bool {
	return h2Index(body, title) >= 0
}

// sectionBody returns the text of the H2 section with the given title: the
// content from just after the heading line up to the next H2 (or EOF). An
// absent section yields "".
func sectionBody(body, title string) string {
	start := h2Index(body, title)
	if start < 0 {
		return ""
	}
	// Advance past the heading line.
	rest := body[start:]
	nl := strings.IndexByte(rest, '\n')
	if nl < 0 {
		return ""
	}
	rest = rest[nl+1:]
	// Truncate at the next H2 heading, if any.
	if next := nextH2Index(rest); next >= 0 {
		rest = rest[:next]
	}
	return rest
}

// h2Index returns the byte offset of the `## <title>` heading line in body, or
// -1 if absent. Matching is line-anchored and title-exact.
func h2Index(body, title string) int {
	re := regexp.MustCompile(`(?m)^##\s+` + regexp.QuoteMeta(title) + `\s*$`)
	loc := re.FindStringIndex(body)
	if loc == nil {
		return -1
	}
	return loc[0]
}

// nextH2Index returns the byte offset of the first H2 heading line in s, or -1.
var anyH2RE = regexp.MustCompile(`(?m)^##\s+`)

func nextH2Index(s string) int {
	loc := anyH2RE.FindStringIndex(s)
	if loc == nil {
		return -1
	}
	return loc[0]
}
