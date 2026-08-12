package lifecycle

import "strings"

// resolutionHeading is the H2 heading under which an optional transition
// --note is written, per lifecycle-transitions#REQ:optional-transition-note.
const resolutionHeading = "## Resolution"

// AppendResolutionNote writes the supplied markdown into the artifact body as
// a `## Resolution` section, implementing
// lifecycle-transitions#REQ:optional-transition-note.
//
// Semantics:
//
//   - An empty or whitespace-only note is treated as absent: the file is left
//     untouched, wrote is false, and original is nil.
//   - If a `## Resolution` H2 section already exists, the note is appended as a
//     new trailing paragraph within it (the section is never relocated).
//   - If absent, the section is created immediately before the artifact footer
//     line (`*This document follows the …*`) when one is present, else at EOF.
//
// The markdown is written verbatim except for trailing-newline normalization;
// it is never reflowed, wrapped, truncated, or sanitized.
//
// On a write, original holds the exact pre-invocation file bytes so the caller
// can roll back via RestoreBody as part of the surrounding atomic transition.
func AppendResolutionNote(artifactPath, note string) (original []byte, wrote bool, err error) {
	if strings.TrimSpace(note) == "" {
		return nil, false, nil
	}
	err = TransformArtifact(artifactPath, func(before []byte) ([]byte, error) {
		original = append([]byte(nil), before...)
		var updated []byte
		updated, wrote, err = AppendResolutionNoteBytes(before, note)
		return updated, err
	})
	return original, wrote, err
}

// AppendResolutionNoteBytes appends a resolution paragraph in memory.
func AppendResolutionNoteBytes(orig []byte, note string) ([]byte, bool, error) {
	if strings.TrimSpace(note) == "" {
		return orig, false, nil
	}
	para := strings.TrimRight(note, "\n\r \t")
	return joinLines(insertResolutionNote(splitKeepTerminators(orig), para)), true, nil
}

// RestoreBody rewrites the artifact with original (the bytes returned by a
// prior AppendResolutionNote call), restoring it byte-for-byte. It is the
// body-mutation counterpart to lifecycle.Rollback and participates in the same
// rollback path.
func RestoreBody(artifactPath string, original []byte) error {
	return TransformArtifact(artifactPath, func([]byte) ([]byte, error) {
		return append([]byte(nil), original...), nil
	})
}

// insertResolutionNote returns the lines with the note inserted: a new
// paragraph appended to an existing `## Resolution` section, or a freshly
// created section when none exists.
func insertResolutionNote(lines []string, para string) []string {
	if idx := findResolutionHeadingIndex(lines); idx >= 0 {
		return appendParagraphToSection(lines, idx, para)
	}
	return createResolutionSection(lines, para)
}

// findResolutionHeadingIndex returns the index of the `## Resolution` H2
// heading line, or -1 when absent.
func findResolutionHeadingIndex(lines []string) int {
	for i, ln := range lines {
		body, _ := splitTerminator(ln)
		if strings.TrimRight(body, " \t") == resolutionHeading {
			return i
		}
	}
	return -1
}

// appendParagraphToSection appends para as a new trailing paragraph to the
// `## Resolution` section that starts at headingIdx. The new paragraph is
// inserted just before the next H2 (`## `) heading at or after the section, or
// before the footer line, or at EOF — whichever comes first — so existing
// sections are never relocated.
func appendParagraphToSection(lines []string, headingIdx int, para string) []string {
	end := len(lines)
	for i := headingIdx + 1; i < len(lines); i++ {
		body, _ := splitTerminator(lines[i])
		if strings.HasPrefix(body, "## ") || isFooterLine(body) {
			end = i
			break
		}
	}
	block := paragraphBlock(para, true)
	return insertAt(lines, end, block)
}

// createResolutionSection inserts a fresh `## Resolution` section carrying
// para, immediately before the footer line when present, else at EOF.
func createResolutionSection(lines []string, para string) []string {
	heading := resolutionHeading + "\n\n"
	block := []string{heading}
	block = append(block, paragraphBlock(para, false)...)

	for i, ln := range lines {
		body, _ := splitTerminator(ln)
		if isFooterLine(body) {
			// Insert before the footer, ensuring a blank separator line
			// precedes the new section.
			withLead := ensureLeadingBlank(lines, i, block)
			return insertAt(lines, i, withLead)
		}
	}
	// No footer — append at EOF.
	tail := ensureLeadingBlank(lines, len(lines), block)
	return append(lines, tail...)
}

// paragraphBlock renders para as a terminated block of lines. When leading is
// true a blank separator line precedes the paragraph (used when appending a
// further paragraph inside an existing section).
func paragraphBlock(para string, leading bool) []string {
	var out []string
	if leading {
		out = append(out, "\n")
	}
	for _, l := range strings.Split(para, "\n") {
		out = append(out, l+"\n")
	}
	return out
}

// ensureLeadingBlank prepends a blank line to block when the line immediately
// before insertAtIdx is not already blank, so the inserted section is visually
// separated from preceding content.
func ensureLeadingBlank(lines []string, insertAtIdx int, block []string) []string {
	if insertAtIdx == 0 {
		return block
	}
	prevBody, _ := splitTerminator(lines[insertAtIdx-1])
	if strings.TrimSpace(prevBody) == "" {
		return block
	}
	return append([]string{"\n"}, block...)
}

// insertAt returns lines with block spliced in at index i.
func insertAt(lines []string, i int, block []string) []string {
	out := make([]string, 0, len(lines)+len(block))
	out = append(out, lines[:i]...)
	out = append(out, block...)
	out = append(out, lines[i:]...)
	return out
}

// isFooterLine reports whether body is the artifact footer line
// (`*This document follows the …*`).
func isFooterLine(body string) bool {
	return strings.HasPrefix(strings.TrimSpace(body), "*This document follows")
}
