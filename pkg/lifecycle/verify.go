package lifecycle

import (
	"fmt"
	"os"
	"strings"
)

// StatusNotPersistedError reports that an artifact does NOT carry the status a
// transition claimed to write. It is raised by VerifyPersistedStatus after the
// post-mutation hook (`spec lint --fix` + verify) has run, so it catches the
// case the plain write path cannot see: the rewrite itself succeeded, and then
// derived-index work rewrote the same line back to a different value.
//
// Without this check a verb prints its success line — `<slug>: <from> → <to>`
// — and exits 0 while the file on disk is byte-identical to its pre-invocation
// state. That failure is self-concealing: the success line even quotes the
// correct from → to pair. Callers MUST treat this error as a failed
// transition, restore the pre-invocation state, and exit non-zero.
type StatusNotPersistedError struct {
	// Path is the artifact whose status did not stick.
	Path string
	// Want is the status the transition requested.
	Want Status
	// GotBody is the body `**Status:**` value actually on disk.
	GotBody Status
	// GotFrontmatter is the YAML frontmatter `status:` mirror actually on
	// disk, or "" when the artifact carries no frontmatter mirror.
	GotFrontmatter string
	// HasFrontmatter reports whether a frontmatter `status:` mirror exists.
	HasFrontmatter bool
}

// Error implements the error interface. The message names the requested status
// and every surface that disagrees with it, so the operator can tell a
// re-derived status apart from a half-written one (body advanced, mirror stale).
func (e *StatusNotPersistedError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "transition did not persist: %s requested %q but still reads **Status:** %q",
		e.Path, string(e.Want), string(e.GotBody))
	if e.HasFrontmatter && e.GotFrontmatter != string(e.Want) {
		fmt.Fprintf(&b, " (frontmatter status: %q)", e.GotFrontmatter)
	}
	b.WriteString(" after the post-mutation index sync")
	return b.String()
}

// PersistedStatus reads artifactPath and returns the two status surfaces that
// MUST agree after a transition: the body `**Status:**` value and the YAML
// frontmatter `status:` mirror. hasFrontmatter is false (and frontmatter is
// "") for artifacts that carry no frontmatter mirror — those are unaffected by
// the mirror half of the check.
func PersistedStatus(artifactPath string) (body Status, frontmatter string, hasFrontmatter bool, err error) {
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		return "", "", false, err
	}
	lines := splitKeepTerminators(data)

	idx := findStatusLineIndex(lines)
	if idx < 0 {
		return "", "", false, ErrStatusLineNotFound
	}
	bodyLine, _ := splitTerminator(lines[idx])
	m := statusLineRe.FindStringSubmatch(bodyLine)
	body = Status(strings.TrimSpace(m[2]))

	if fmIdx := findFrontmatterStatusLineIndex(lines); fmIdx >= 0 {
		fmLine, _ := splitTerminator(lines[fmIdx])
		fm := fmStatusLineRe.FindStringSubmatch(fmLine)
		return body, strings.TrimSpace(fm[2]), true, nil
	}
	return body, "", false, nil
}

// VerifyPersistedStatus confirms that artifactPath actually carries want on
// BOTH status surfaces — the body `**Status:**` line and, when present, the
// frontmatter `status:` mirror.
//
// It is the last step of a lifecycle transition, run after the post-mutation
// hook returns success, and it is what stands between a verb and the
// self-concealing failure described on StatusNotPersistedError. A verb MUST
// call it before printing its success line: the hook returning nil only proves
// the derived-index pass did not ERROR, never that it left the requested status
// in place.
//
// It returns *StatusNotPersistedError on disagreement, or the underlying
// os/parse error when the artifact cannot be read.
func VerifyPersistedStatus(artifactPath string, want Status) error {
	body, frontmatter, hasFrontmatter, err := PersistedStatus(artifactPath)
	if err != nil {
		return err
	}
	if body == want && (!hasFrontmatter || frontmatter == string(want)) {
		return nil
	}
	return &StatusNotPersistedError{
		Path:           artifactPath,
		Want:           want,
		GotBody:        body,
		GotFrontmatter: frontmatter,
		HasFrontmatter: hasFrontmatter,
	}
}
