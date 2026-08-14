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
// artifact's header block, mirroring the Decision "Superseded By" convention.
// It is retained as a legacy single-field wrapper. Transaction-profile
// Task/Plan writers call SetSupersededByBytes inside their one artifact
// transaction and do not stitch this wrapper into a multi-write sequence.
//
// Semantics:
//
//   - An empty or whitespace-only successor is treated as absent: the file is
//     left untouched, wrote is false, and original is nil.
//   - If a `**Superseded By:**` line already exists, its value is rewritten in
//     place (indentation and trailing whitespace preserved).
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

	// Rewrite an existing line in place.
	for i, ln := range lines {
		body, terminator := splitTerminator(ln)
		if m := supersededByLineRe.FindStringSubmatch(body); m != nil {
			lines[i] = fmt.Sprintf("%s**Superseded By:** %s%s", m[1], successor, m[3]) + terminator
			return joinLines(lines), true, nil
		}
	}

	// Insert after the Supersedes line, else after the Status line.
	anchor := findSupersedesLineIndex(lines)
	if anchor < 0 {
		anchor = findStatusLineIndex(lines)
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

// findSupersedesLineIndex returns the index of the `**Supersedes:**` header
// line, or -1 when absent.
func findSupersedesLineIndex(lines []string) int {
	for i, ln := range lines {
		body, _ := splitTerminator(ln)
		if supersedesLineRe.MatchString(body) {
			return i
		}
	}
	return -1
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
