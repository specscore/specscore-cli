package lesson

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// recurringHeading is the H2 heading under which each recurrence is logged.
const recurringHeading = "## Recurrences"

// recurredLineRe matches a `**Recurred:** <n>` header line, tolerating
// leading indent and trailing whitespace — mirroring the shape of
// pkg/lifecycle's own field-line matchers.
var recurredLineRe = regexp.MustCompile(`^([ \t]*)\*\*Recurred:\*\*[ \t]*([^\r\n]*?)([ \t]*\r?)$`)

// statusHeaderLineRe locates the `**Status:**` line as the fallback
// insertion anchor when a Lesson predates the `**Recurred:**` field.
var statusHeaderLineRe = regexp.MustCompile(`^([ \t]*)\*\*Status:\*\*`)

// Recur records that a Lesson's gap manifested again: it increments the
// `**Recurred:** N` header count (inserting the field, defaulted to 0 then
// incremented to 1, when a pre-existing Lesson predates it) and appends a
// dated bullet — with the optional note — to a `## Recurrences` section
// (created immediately before the adherence footer when absent). It does
// NOT change `**Status:**`: a recurrence is a signal that a lesson needs to
// graduate, not a graduation itself — the decision to promote stays a
// deliberate `change-status` call. Returns the new recurrence count.
func Recur(path, note string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	lines := strings.Split(string(data), "\n")

	newCount, lines, err := incrementRecurredLine(lines)
	if err != nil {
		return 0, err
	}

	lines = appendRecurrenceEntry(lines, note)

	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return 0, err
	}
	return newCount, nil
}

// incrementRecurredLine finds the `**Recurred:**` line and rewrites its value
// to (current + 1), or — when absent — inserts a fresh `**Recurred:** 1` line
// immediately after `**Status:**`. A present-but-unparsable value is treated
// as 0 before incrementing, so the field self-heals rather than propagating
// a bad value forever.
func incrementRecurredLine(lines []string) (int, []string, error) {
	for i, ln := range lines {
		if m := recurredLineRe.FindStringSubmatch(ln); m != nil {
			current, _ := strconv.Atoi(strings.TrimSpace(m[2]))
			if current < 0 {
				current = 0
			}
			next := current + 1
			lines[i] = fmt.Sprintf("%s**Recurred:** %d%s", m[1], next, m[3])
			return next, lines, nil
		}
	}
	for i, ln := range lines {
		if statusHeaderLineRe.MatchString(ln) {
			out := make([]string, 0, len(lines)+1)
			out = append(out, lines[:i+1]...)
			out = append(out, "**Recurred:** 1")
			out = append(out, lines[i+1:]...)
			return 1, out, nil
		}
	}
	return 0, lines, fmt.Errorf("lesson has no **Status:** line to anchor a new **Recurred:** field")
}

// appendRecurrenceEntry appends a dated bullet to the `## Recurrences`
// section, creating the section immediately before the adherence footer
// (or at EOF absent one) when it does not already exist.
func appendRecurrenceEntry(lines []string, note string) []string {
	entry := "- " + time.Now().UTC().Format("2006-01-02")
	note = strings.TrimSpace(note)
	if note != "" {
		entry += " — " + note
	}

	if idx := findRecurrencesHeading(lines); idx >= 0 {
		end := len(lines)
		for i := idx + 1; i < len(lines); i++ {
			t := strings.TrimSpace(lines[i])
			if strings.HasPrefix(t, "## ") || isFooterLine(t) {
				end = i
				break
			}
		}
		out := make([]string, 0, len(lines)+1)
		out = append(out, lines[:end]...)
		out = append(out, entry)
		out = append(out, lines[end:]...)
		return out
	}

	block := []string{"", recurringHeading, "", entry, ""}
	for i, ln := range lines {
		if isFooterLine(strings.TrimSpace(ln)) {
			out := make([]string, 0, len(lines)+len(block))
			out = append(out, lines[:i]...)
			out = append(out, block...)
			out = append(out, lines[i:]...)
			return out
		}
	}
	return append(append([]string{}, lines...), block...)
}

func findRecurrencesHeading(lines []string) int {
	for i, ln := range lines {
		if strings.TrimRight(ln, " \t\r") == recurringHeading {
			return i
		}
	}
	return -1
}

// isFooterLine reports whether body is the artifact footer line
// (`*This document follows the …*`).
func isFooterLine(body string) bool {
	return strings.HasPrefix(strings.TrimSpace(body), "*This document follows")
}
