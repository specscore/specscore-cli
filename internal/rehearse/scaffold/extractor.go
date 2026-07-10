package scaffold

import "strings"

// ExtractedAC holds the result of extracting Given/When/Then text from an AC body.
type ExtractedAC struct {
	HasGWT bool     // true if Given/When/Then lines were found
	Text   []string // extracted lines (Given/When/Then, or fallback raw)
}

// Extract parses acBody for Given/When/Then lines, stripping ** bold markers.
// An optional leading "Scenario: <name>" line is included. If no GWT lines
// are found the raw body is returned as-is with HasGWT false. Extraction
// stops at the next ### or ## heading.
func Extract(acBody []string) *ExtractedAC {
	var lines []string
	var scenarioLine string

	for _, line := range acBody {
		// Stop at any heading.
		if strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "### ") {
			break
		}
		// Capture optional Scenario: line.
		if strings.HasPrefix(line, "Scenario: ") {
			scenarioLine = line
			continue
		}
		// Strip ** bold markers and check first word.
		stripped := strings.ReplaceAll(line, "**", "")
		first := firstWord(stripped)
		if first == "Given" || first == "When" || first == "Then" {
			lines = append(lines, line)
		}
	}

	if len(lines) == 0 {
		return &ExtractedAC{HasGWT: false, Text: acBody}
	}

	var text []string
	if scenarioLine != "" {
		text = append(text, scenarioLine)
	}
	text = append(text, lines...)
	return &ExtractedAC{HasGWT: true, Text: text}
}

// firstWord returns the first whitespace-delimited token in s, or "".
func firstWord(s string) string {
	s = strings.TrimSpace(s)
	idx := strings.IndexByte(s, ' ')
	if idx < 0 {
		return s
	}
	return s[:idx]
}
