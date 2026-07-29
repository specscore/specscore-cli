package lint

import "strings"

// isFrontmatterFence recognizes either YAML document marker accepted as a
// frontmatter fence. A frontmatter block starts with `---`; YAML also permits
// `...` as its closing marker. Keeping this recognition shared prevents a
// body field after a dotted closer from being parsed as frontmatter.
func isFrontmatterFence(line string) bool {
	trimmed := strings.TrimSpace(line)
	return trimmed == "---" || trimmed == "..."
}

// isLeadingFrontmatterFence additionally accepts a UTF-8 BOM on the physical
// first line. The BOM is source encoding, not part of the fence syntax.
func isLeadingFrontmatterFence(line string) bool {
	return isFrontmatterFence(strings.TrimPrefix(line, "\ufeff"))
}

// parseLeadingFrontmatter extracts a leading YAML-frontmatter block at the
// very top of content and returns its top-level scalar key/value pairs. The
// opening fence is `---`; the closer may be `---` or `...`. present is false
// when content has no complete leading frontmatter block.
//
// Only top-level `key: value` scalar lines are captured; indented (nested)
// lines, blank lines, and `#` comments are skipped. The
// artifact-frontmatter-convention mirrored fields this package reads —
// `format:` and `status:` — are top-level scalars, so a minimal parser is
// sufficient and avoids a YAML dependency in the hot lint path. Surrounding
// single or double quotes on a value are stripped.
func parseLeadingFrontmatter(content []byte) (fields map[string]string, present bool) {
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 || !isLeadingFrontmatterFence(lines[0]) {
		return nil, false
	}

	closing := -1
	for i := 1; i < len(lines); i++ {
		if isFrontmatterFence(lines[i]) {
			closing = i
			break
		}
	}
	if closing == -1 {
		return nil, false
	}

	fields = map[string]string{}
	for i := 1; i < closing; i++ {
		line := strings.TrimRight(lines[i], "\r")
		if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '#' {
			continue
		}
		idx := strings.Index(line, ":")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		fields[key] = stripQuotes(val)
	}
	return fields, true
}

// frontmatterEndIndex returns the index of the first content line after a
// complete leading frontmatter block, or 0 when there is no opening fence or no
// closing fence. Parsers anchor their title/header scan to this so a leading
// frontmatter block does not displace the title.
func frontmatterEndIndex(lines []string) int {
	if len(lines) == 0 || !isLeadingFrontmatterFence(lines[0]) {
		return 0
	}
	for i := 1; i < len(lines); i++ {
		if isFrontmatterFence(lines[i]) {
			return i + 1
		}
	}
	return 0
}

// bodyAfterFrontmatter returns content with any complete leading YAML
// frontmatter block (an opening `---`, intervening lines, and a `---` or `...`
// closer) removed, so callers inspecting the human-visible body do not match
// lines inside frontmatter. When there is no opening fence, or the opening
// fence is never closed, the full content is returned unchanged.
func bodyAfterFrontmatter(content []byte) string {
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 || !isLeadingFrontmatterFence(lines[0]) {
		return string(content)
	}
	for i := 1; i < len(lines); i++ {
		if isFrontmatterFence(lines[i]) {
			return strings.Join(lines[i+1:], "\n")
		}
	}
	return string(content)
}

// stripQuotes removes a single matching pair of surrounding single or double
// quotes from s. Unbalanced or absent quotes leave s unchanged.
func stripQuotes(s string) string {
	if len(s) >= 2 {
		first, last := s[0], s[len(s)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
