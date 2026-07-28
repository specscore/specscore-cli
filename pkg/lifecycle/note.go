package lifecycle

import (
	"fmt"
	"strings"
)

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
// This wrapper is retained for historical compensating callers. On a write,
// original holds its exact pre-invocation bytes, but that snapshot is not
// post-commit rollback authority. Transaction-profile Task/Plan writers call
// AppendResolutionNoteBytes inside their single artifact transaction.
func AppendResolutionNote(artifactPath, note string) (original []byte, wrote bool, err error) {
	return appendResolutionNoteAfterLine(artifactPath, note, 0)
}

// AppendResolutionNoteAfterLine writes note in the artifact body after the
// 1-based afterLine anchor. Resolution-heading and footer discovery are both
// limited to that body scope, so a Markdown example before a canonical title
// or header cannot receive or suppress a real audit note. Pass 0 when the
// whole document is the body; AppendResolutionNote does so for artifact kinds
// that do not provide a structural body anchor.
//
// The empty-note, verbatim-write, original-byte, and rollback semantics are
// identical to AppendResolutionNote. An anchor beyond the end of the file is
// rejected without mutating it.
func AppendResolutionNoteAfterLine(artifactPath, note string, afterLine int) (original []byte, wrote bool, err error) {
	return appendResolutionNoteAfterLine(artifactPath, note, afterLine)
}

func appendResolutionNoteAfterLine(artifactPath, note string, afterLine int) (original []byte, wrote bool, err error) {
	if strings.TrimSpace(note) == "" {
		return nil, false, nil
	}
	err = TransformArtifact(artifactPath, func(before []byte) ([]byte, error) {
		var updated []byte
		updated, wrote, err = AppendResolutionNoteAfterLineBytes(before, note, afterLine)
		if err != nil {
			return nil, err
		}
		original = append([]byte(nil), before...)
		return updated, err
	})
	return original, wrote, err
}

// AppendResolutionNoteBytes appends a resolution paragraph in memory.
func AppendResolutionNoteBytes(orig []byte, note string) ([]byte, bool, error) {
	return AppendResolutionNoteAfterLineBytes(orig, note, 0)
}

// AppendResolutionNoteAfterLineBytes is the snapshot-pure, body-scoped form
// used by compound artifact transactions. afterLine is a 1-based anchor.
func AppendResolutionNoteAfterLineBytes(orig []byte, note string, afterLine int) ([]byte, bool, error) {
	if strings.TrimSpace(note) == "" {
		return orig, false, nil
	}
	lines := splitKeepTerminators(orig)
	if afterLine < 0 || afterLine > len(lines) {
		return nil, false, fmt.Errorf("resolution-note body anchor line %d is outside 0..%d", afterLine, len(lines))
	}
	para := strings.TrimRight(note, "\n\r \t")
	structure := StructuralMarkdownMask(lines, "")
	return joinLines(insertResolutionNoteAfterLine(lines, structure, para, afterLine)), true, nil
}

// RestoreBody is the explicit legacy whole-body compensating writer. It is not
// part of the Task/Plan transaction profile and MUST NOT be used after their
// artifact commit or post-mutation callback failure.
func RestoreBody(artifactPath string, original []byte) error {
	return TransformArtifact(artifactPath, func([]byte) ([]byte, error) {
		return append([]byte(nil), original...), nil
	})
}

// insertResolutionNoteAfterLine is the body-scoped form of
// Resolution-note insertion. afterLine is a 1-based anchor: only lines
// strictly after it participate in Resolution-heading and footer lookup.
func insertResolutionNoteAfterLine(lines []string, structure []bool, para string, afterLine int) []string {
	if idx := findResolutionHeadingIndexAfterLine(lines, structure, afterLine); idx >= 0 {
		return appendParagraphToSection(lines, structure, idx, para)
	}
	return createResolutionSectionAfterLine(lines, structure, para, afterLine)
}

// findResolutionHeadingIndexAfterLine returns the first Resolution heading
// strictly after the 1-based afterLine anchor, or -1 when absent.
func findResolutionHeadingIndexAfterLine(lines []string, structure []bool, afterLine int) int {
	for i := afterLine; i < len(lines); i++ {
		if !isStructuralLine(structure, i) {
			continue
		}
		ln := lines[i]
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
func appendParagraphToSection(lines []string, structure []bool, headingIdx int, para string) []string {
	end := len(lines)
	for i := headingIdx + 1; i < len(lines); i++ {
		if !isStructuralLine(structure, i) {
			continue
		}
		body, _ := splitTerminator(lines[i])
		if isCanonicalUnindentedATXH2(body) || isFooterLine(body) {
			end = i
			break
		}
	}
	block := paragraphBlock(para, true)
	return insertAt(lines, end, block)
}

// createResolutionSectionAfterLine inserts a fresh Resolution section into
// the body after the 1-based afterLine anchor. Only a footer in that body can
// act as the insertion point; a pre-anchor footer is prose or an example.
func createResolutionSectionAfterLine(lines []string, structure []bool, para string, afterLine int) []string {
	heading := resolutionHeading + "\n\n"
	block := []string{heading}
	block = append(block, paragraphBlock(para, false)...)

	for i := afterLine; i < len(lines); i++ {
		if !isStructuralLine(structure, i) {
			continue
		}
		ln := lines[i]
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

func isStructuralLine(structure []bool, line int) bool {
	return line >= 0 && line < len(structure) && structure[line]
}

func isCanonicalUnindentedATXH2(body string) bool {
	return strings.HasPrefix(body, "## ") && strings.TrimSpace(body[len("## "):]) != ""
}
