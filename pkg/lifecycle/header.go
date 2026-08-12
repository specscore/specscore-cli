package lifecycle

import (
	"fmt"
	"os"
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
// It is the structured counterpart to the `--note` text and participates in
// the same atomic-transition + rollback path as AppendResolutionNote.
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
// On a write, original holds the exact pre-invocation file bytes so the caller
// can roll back via RestoreBody as part of the surrounding atomic transition.
func SetSupersededBy(artifactPath, successor string) (original []byte, wrote bool, err error) {
	err = WithArtifactMutationLock(artifactPath, func() error {
		original, wrote, err = setSupersededByUnderLock(artifactPath, successor)
		return err
	})
	return original, wrote, err
}

func setSupersededByUnderLock(artifactPath, successor string) (original []byte, wrote bool, err error) {
	successor = strings.TrimSpace(successor)
	if successor == "" {
		return nil, false, nil
	}

	orig, err := os.ReadFile(artifactPath)
	if err != nil {
		return nil, false, err
	}
	updated, wrote, err := SetSupersededByBytes(orig, successor)
	if err != nil || !wrote {
		return orig, wrote, err
	}
	if err := writeFileAtomic(artifactPath, updated); err != nil {
		return nil, false, err
	}
	return orig, true, nil
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
	err = WithArtifactMutationLock(artifactPath, func() error {
		original, wrote, err = setSupersedesUnderLock(artifactPath, target)
		return err
	})
	return original, wrote, err
}

func setSupersedesUnderLock(artifactPath, target string) (original []byte, wrote bool, err error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, false, nil
	}

	orig, err := os.ReadFile(artifactPath)
	if err != nil {
		return nil, false, err
	}
	lines := splitKeepTerminators(orig)

	// Rewrite an existing line in place.
	for i, ln := range lines {
		body, terminator := splitTerminator(ln)
		if m := fullSupersedesLineRe.FindStringSubmatch(body); m != nil {
			lines[i] = fmt.Sprintf("%s**Supersedes:** %s%s", m[1], target, m[3]) + terminator
			if err := writeFileAtomic(artifactPath, joinLines(lines)); err != nil {
				return nil, false, err
			}
			return orig, true, nil
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

	if err := writeFileAtomic(artifactPath, joinLines(lines)); err != nil {
		return nil, false, err
	}
	return orig, true, nil
}
