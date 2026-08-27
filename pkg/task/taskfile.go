package task

import (
	"bytes"
	"fmt"
	"strings"
)

// TaskFileData holds the parsed contents of a task README.md.
type TaskFileData struct {
	Title       string
	Status      string
	Description string
	DependsOn   []string
	Summary     string
}

// ParseTaskFile parses a task README.md into its structured fields.
//
// The expected format is:
//
//	# Title
//
//	**Status:** planning
//
//	Description text
//
//	## Dependencies
//
//	- dep1
//	- dep2
//
//	## Summary
//
//	Summary text
//
// The **Status:** line is OPTIONAL for backward compatibility with task
// files written before it existed (and with `specscore migrate`, which
// backfills it from the board index — see pkg/lint.migrateTaskBoardStatus).
// Status is "" when absent; callers needing a value fall back to the board,
// but once a file carries its own line, THAT line is authoritative for every
// subsequent read — see https://github.com/specscore/specscore/blob/main/spec/features/index/README.md#req-file-authoritative-over-index.
func ParseTaskFile(data []byte) (TaskFileData, error) {
	content := string(data)

	// Title must be the first line and start with "# ".
	if !strings.HasPrefix(content, "# ") {
		return TaskFileData{}, fmt.Errorf("missing task title: expected line starting with '# '")
	}

	firstNL := strings.Index(content, "\n")
	if firstNL == -1 {
		return TaskFileData{}, fmt.Errorf("missing Dependencies section")
	}
	title := strings.TrimSpace(content[2:firstNL])
	rest := content[firstNL+1:]

	// Split the remainder on "\n## " to locate H2 sections.
	parts := strings.Split(rest, "\n## ")

	// parts[0] is everything before the first H2: an optional leading
	// **Status:** line, then the description.
	pre := parts[0]
	status := ""
	trimmedPre := strings.TrimLeft(pre, "\n")
	if strings.HasPrefix(trimmedPre, "**Status:**") {
		if nl := strings.IndexByte(trimmedPre, '\n'); nl < 0 {
			status = strings.TrimSpace(strings.TrimPrefix(trimmedPre, "**Status:**"))
			trimmedPre = ""
		} else {
			status = strings.TrimSpace(strings.TrimPrefix(trimmedPre[:nl], "**Status:**"))
			trimmedPre = trimmedPre[nl+1:]
		}
		pre = trimmedPre
	}
	description := strings.TrimSpace(pre)

	sections := make(map[string]string, len(parts)-1)
	for _, p := range parts[1:] {
		idx := strings.Index(p, "\n")
		if idx == -1 {
			sections[strings.TrimSpace(p)] = ""
			continue
		}
		heading := strings.TrimSpace(p[:idx])
		body := strings.TrimSpace(p[idx+1:])
		sections[heading] = body
	}

	// Dependencies section is required.
	depBody, ok := sections["Dependencies"]
	if !ok {
		return TaskFileData{}, fmt.Errorf("missing Dependencies section")
	}

	var deps []string
	if depBody != "None" {
		for _, line := range strings.Split(depBody, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "- ") {
				deps = append(deps, strings.TrimSpace(line[2:]))
			}
		}
	}

	// Summary section is required.
	summaryBody, ok := sections["Summary"]
	if !ok {
		return TaskFileData{}, fmt.Errorf("missing Summary section")
	}
	summary := summaryBody
	if summary == "None" {
		summary = ""
	}

	return TaskFileData{
		Title:       title,
		Status:      status,
		Description: description,
		DependsOn:   deps,
		Summary:     summary,
	}, nil
}

// RenderTaskFile renders a TaskFileData to markdown bytes. When d.Status is
// non-empty it is written as a **Status:** line immediately after the title —
// callers creating a new task (which always starts `planning`) should always
// set it, so a task never again reaches the no-Status-line state that made
// `task change-status` unusable on existing boards.
func RenderTaskFile(d TaskFileData) []byte {
	var buf bytes.Buffer

	_, _ = fmt.Fprintf(&buf, "# %s\n", d.Title)

	if d.Status != "" {
		_, _ = fmt.Fprintf(&buf, "\n**Status:** %s\n", d.Status)
	}

	if d.Description != "" {
		_, _ = fmt.Fprintf(&buf, "\n%s\n", d.Description)
	}

	_, _ = buf.WriteString("\n## Dependencies\n\n")
	if len(d.DependsOn) == 0 {
		_, _ = buf.WriteString("None\n")
	} else {
		for _, dep := range d.DependsOn {
			_, _ = fmt.Fprintf(&buf, "- %s\n", dep)
		}
	}

	_, _ = buf.WriteString("\n## Summary\n\n")
	if d.Summary == "" {
		_, _ = buf.WriteString("None\n")
	} else {
		_, _ = fmt.Fprintf(&buf, "%s\n", d.Summary)
	}

	return buf.Bytes()
}
