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

// Scenario is one parsed scenario file.
type Scenario struct {
	// Path is the file the scenario was parsed from.
	Path string
	// Verifies lists the AC ids from the `**Verifies:**` body metadata,
	// parenthetical annotations stripped. Never nil.
	Verifies []string
	// Status is the `**Status:**` body metadata value (e.g. "pending").
	Status string
	// Blocks are all fenced blocks in document order.
	Blocks []Block
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
	sc := &Scenario{Path: path, Verifies: []string{}}

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
	}
	if inFence {
		return nil, fmt.Errorf("%s: unclosed fenced block opened at line %d", path, openLine)
	}
	return sc, nil
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
