package lifecycle

import (
	"fmt"
	"regexp"
	"strings"
)

// supersededByLineRe matches a `**Superseded By:** <value>` header line,
// mirroring statusLineRe's tolerance for leading indent and trailing
// whitespace/CR. Capture groups: [1] indent, [2] value, [3] trailing.
var supersededByLineRe = regexp.MustCompile(`^([ \t]*)\*\*Superseded By:\*\*[ \t]+([^\r\n]*?)([ \t]*\r?)$`)

// supersedesLineRe matches the `**Supersedes:**` header line, used as the
// insertion anchor for a freshly written `**Superseded By:**` line.
var supersedesLineRe = regexp.MustCompile(`^([ \t]*)\*\*Supersedes:\*\*`)

// SetSupersededBy writes a `**Superseded By:** <successor>` reference into the
// canonical lifecycle-artifact header, mirroring the Decision convention.
// It is retained as a legacy single-field wrapper. Transaction-profile
// Task/Plan writers call SetSupersededByBytes inside their one artifact
// transaction and do not stitch this wrapper into a multi-write sequence.
//
// Semantics:
//
//   - An empty or whitespace-only successor is treated as absent: the file is
//     left untouched, wrote is false, and original is nil.
//   - Only fields in the canonical header after the first structural Plan or
//     Lesson title and before its first structural H2 participate.
//   - If a `**Superseded By:**` line already exists there, its value is
//     rewritten in place (indentation and trailing whitespace preserved).
//   - Otherwise the line is inserted immediately after the `**Supersedes:**`
//     header line when present, else immediately after the `**Status:**` line.
//     A file with neither anchor returns ErrStatusLineNotFound and is left
//     untouched.
//
// On a write, original holds the exact pre-invocation bytes for historical
// compensating callers only; it is not a post-commit rollback authority.
func SetSupersededBy(artifactPath, successor string) (original []byte, wrote bool, err error) {
	if strings.TrimSpace(successor) == "" {
		return nil, false, nil
	}
	err = TransformArtifact(artifactPath, func(before []byte) ([]byte, error) {
		original = append([]byte(nil), before...)
		var updated []byte
		updated, wrote, err = SetSupersededByBytes(before, successor)
		return updated, err
	})
	return original, wrote, err
}

// SetSupersededByBytes applies the successor header transform in memory.
func SetSupersededByBytes(orig []byte, successor string) ([]byte, bool, error) {
	successor = strings.TrimSpace(successor)
	if successor == "" {
		return orig, false, nil
	}
	lines := splitKeepTerminators(orig)
	structure := StructuralMarkdownMask(lines, "")
	headerStart, headerEnd := canonicalLifecycleHeaderRange(lines, structure)
	if headerStart < 0 {
		return nil, false, ErrStatusLineNotFound
	}

	// Rewrite an existing canonical-header line in place.
	for i := headerStart; i < headerEnd; i++ {
		if !isStructuralLine(structure, i) {
			continue
		}
		ln := lines[i]
		body, terminator := splitTerminator(ln)
		if m := supersededByLineRe.FindStringSubmatch(body); m != nil {
			lines[i] = fmt.Sprintf("%s**Superseded By:** %s%s", m[1], successor, m[3]) + terminator
			return joinLines(lines), true, nil
		}
	}

	// Insert after the canonical-header Supersedes line, else its Status line.
	anchor := findSupersedesLineIndexInHeader(lines, structure, headerStart, headerEnd)
	if anchor < 0 {
		anchor = findStatusLineIndexInHeader(lines, structure, headerStart, headerEnd)
	}
	if anchor < 0 {
		return nil, false, ErrStatusLineNotFound
	}

	// Match the anchor line's terminator so the inserted line agrees with the
	// file's line-ending convention. When the anchor is the final line with no
	// terminator (EOF), give it one first so the two lines do not run together.
	anchorBody, terminator := splitTerminator(lines[anchor])
	if terminator == "" {
		terminator = "\n"
		lines[anchor] = anchorBody + terminator
	}
	newLine := fmt.Sprintf("**Superseded By:** %s%s", successor, terminator)
	lines = insertAt(lines, anchor+1, []string{newLine})

	return joinLines(lines), true, nil
}

// findSupersedesLineIndex returns the canonical lifecycle-header field.
func findSupersedesLineIndex(lines []string) int {
	structure := StructuralMarkdownMask(lines, "")
	start, end := canonicalLifecycleHeaderRange(lines, structure)
	return findSupersedesLineIndexInHeader(lines, structure, start, end)
}

func findSupersedesLineIndexInHeader(lines []string, structure []bool, start, end int) int {
	if start < 0 {
		return -1
	}
	for i := start; i < end; i++ {
		if !isStructuralLine(structure, i) {
			continue
		}
		ln := lines[i]
		body, _ := splitTerminator(ln)
		if supersedesLineRe.MatchString(body) {
			return i
		}
	}
	return -1
}

func findStatusLineIndexInHeader(lines []string, structure []bool, start, end int) int {
	if start < 0 {
		return -1
	}
	for i := start; i < end; i++ {
		if !isStructuralLine(structure, i) {
			continue
		}
		body, _ := splitTerminator(lines[i])
		if statusLineRe.MatchString(body) {
			return i
		}
	}
	return -1
}

// canonicalLifecycleHeaderRange returns the [start,end) range eligible for
// Plan/Lesson lifecycle fields. An earlier ordinary or Setext H1 makes a later
// lifecycle-looking sample non-canonical and therefore fails closed.
func canonicalLifecycleHeaderRange(lines []string, structure []bool) (start, end int) {
	for i, line := range lines {
		if !isStructuralLine(structure, i) {
			continue
		}
		if isSetextH1Line(lines, structure, i) {
			return -1, -1
		}
		heading, _ := splitTerminator(line)
		if i == 0 {
			heading = strings.TrimPrefix(heading, "\ufeff")
		}
		if !isATXH1(heading) {
			continue
		}
		if !isCanonicalLifecycleTitle(heading) {
			return -1, -1
		}
		end = len(lines)
		for j := i + 1; j < len(lines); j++ {
			if !isStructuralLine(structure, j) {
				continue
			}
			body, _ := splitTerminator(lines[j])
			if isCanonicalUnindentedATXH2(body) {
				end = j
				break
			}
		}
		return i + 1, end
	}
	return -1, -1
}

func isCanonicalLifecycleTitle(line string) bool {
	for _, prefix := range []string{"# Plan: ", "# Lesson: "} {
		if strings.HasPrefix(line, prefix) && strings.TrimSpace(line[len(prefix):]) != "" {
			return true
		}
	}
	return false
}

func isATXH1(line string) bool {
	indent := 0
	for indent < len(line) && line[indent] == ' ' && indent < 3 {
		indent++
	}
	if indent == len(line) || (indent < len(line) && line[indent] == ' ') || line[indent] != '#' {
		return false
	}
	return indent+1 == len(line) || line[indent+1] == ' ' || line[indent+1] == '\t'
}

func isSetextH1Line(lines []string, structure []bool, i int) bool {
	if i <= 0 || !isStructuralLine(structure, i-1) {
		return false
	}
	previous, _ := splitTerminator(lines[i-1])
	if strings.TrimSpace(previous) == "" || IsIndentedCode(previous) {
		return false
	}
	line, _ := splitTerminator(lines[i])
	if IsIndentedCode(line) {
		return false
	}
	indent := 0
	for indent < len(line) && line[indent] == ' ' && indent < 3 {
		indent++
	}
	begin := indent
	for indent < len(line) && line[indent] == '=' {
		indent++
	}
	return indent-begin >= 1 && strings.TrimSpace(line[indent:]) == ""
}

// fullSupersedesLineRe matches a complete `**Supersedes:** <value>` header
// line (as opposed to supersedesLineRe, which anchors only the prefix for use
// as an insertion marker). Capture groups mirror supersededByLineRe: [1]
// indent, [2] value, [3] trailing.
var fullSupersedesLineRe = regexp.MustCompile(`^([ \t]*)\*\*Supersedes:\*\*[ \t]+([^\r\n]*?)([ \t]*\r?)$`)

// SetSupersedes writes a `**Supersedes:** <target>` reference into the
// artifact's header block — the other half of the bidirectional link
// completed by SetSupersededBy on the target artifact. It is the Decision-kind
// counterpart: `decision change-status --to=superseded` calls SetSupersededBy
// on the OLD decision and SetSupersedes on the NEW (successor) decision in the
// same atomic transition, per D-supersedes-bidirectional.
//
// Semantics mirror SetSupersededBy exactly:
//
//   - An empty or whitespace-only target is treated as absent: the file is
//     left untouched, wrote is false, and original is nil.
//   - If a `**Supersedes:**` line already exists, its value is rewritten in
//     place (indentation and trailing whitespace preserved). This is the
//     common case for a Decision, whose scaffold always emits the field
//     (defaulted to `—`).
//   - Otherwise the line is inserted immediately after the `**Status:**`
//     header line. A file with no `**Status:**` line returns
//     ErrStatusLineNotFound and is left untouched.
//
// On a write, original holds the exact pre-invocation file bytes so the
// caller can roll back via RestoreBody as part of the surrounding atomic
// transition.
func SetSupersedes(artifactPath, target string) (original []byte, wrote bool, err error) {
	if strings.TrimSpace(target) == "" {
		return nil, false, nil
	}
	err = TransformArtifact(artifactPath, func(before []byte) ([]byte, error) {
		original = append([]byte(nil), before...)
		var updated []byte
		updated, wrote, err = SetSupersedesBytes(before, target)
		return updated, err
	})
	return original, wrote, err
}

// SetSupersedesBytes applies the predecessor header transform in memory.
func SetSupersedesBytes(original []byte, target string) ([]byte, bool, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return original, false, nil
	}

	lines := splitKeepTerminators(original)

	// Rewrite an existing line in place.
	for i, ln := range lines {
		body, terminator := splitTerminator(ln)
		if m := fullSupersedesLineRe.FindStringSubmatch(body); m != nil {
			lines[i] = fmt.Sprintf("%s**Supersedes:** %s%s", m[1], target, m[3]) + terminator
			return joinLines(lines), true, nil
		}
	}

	// Insert after the Status line.
	anchor := findStatusLineIndex(lines)
	if anchor < 0 {
		return nil, false, ErrStatusLineNotFound
	}

	anchorBody, terminator := splitTerminator(lines[anchor])
	if terminator == "" {
		terminator = "\n"
		lines[anchor] = anchorBody + terminator
	}
	newLine := fmt.Sprintf("**Supersedes:** %s%s", target, terminator)
	lines = insertAt(lines, anchor+1, []string{newLine})

	return joinLines(lines), true, nil
}
