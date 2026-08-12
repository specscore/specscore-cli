package lifecycle

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// Testable indirections for OS operations. Tests inject failures via these.
var (
	osCreateTemp       = os.CreateTemp
	ioCopy             = io.Copy
	osChmod            = os.Chmod
	osRename           = os.Rename
	osReadBeforeRename = os.ReadFile
	osOpenDir          = func(path string) (*os.File, error) { return os.Open(path) }
	fileSync           = func(f *os.File) error { return f.Sync() }
	fileClose          = func(f *os.File) error { return f.Close() }
)

// statusLineRe matches a header line of the form `**Status:** <value>`,
// allowing leading horizontal whitespace before the `**` marker and tolerating
// trailing whitespace after the value. The matcher operates on a single line
// (caller has already split on newlines).
//
// Capture groups:
//
//	[1] indent (any leading horizontal whitespace before "**Status:**")
//	[2] value (the status text, without surrounding whitespace)
//	[3] trailing (any trailing horizontal whitespace AND optional CR)
//
// Note the explicit `\r?` in the trailing group: when a file uses CRLF line
// endings, Rewrite MUST preserve the carriage return byte (REQ:
// status-line-rewrite). The line-splitter below also preserves the original
// terminator on each line, so Rewrite reassembles the file byte-for-byte
// outside of the single mutated value.
var statusLineRe = regexp.MustCompile(`^([ \t]*)\*\*Status:\*\*[ \t]+([^\r\n]*?)([ \t]*\r?)$`)

// fmStatusLineRe matches a YAML-frontmatter `status:` mirror line (lowercase
// key, no bold markers), distinguishing it from the body `**Status:**` line.
// Capture groups mirror statusLineRe: [1] indent, [2] value, [3] trailing.
var fmStatusLineRe = regexp.MustCompile(`^([ \t]*)status:[ \t]*([^\r\n]*?)([ \t]*\r?)$`)

// ErrStatusLineNotFound is returned by Validate/Rewrite when the artifact
// does not contain a recognizable `**Status:**` line.
var ErrStatusLineNotFound = errors.New("lifecycle: artifact has no **Status:** line")

// Validate is a legacy convenience that reads artifactPath, extracts its
// current Status, and checks that the transition is legal. It does not mutate.
// Transaction-profile writers MUST instead parse and call Transition from the
// exact bytes supplied by TransformArtifact; stitching Validate and Rewrite
// together would validate outside the artifact lock.
//
// It returns the from status on success. On failure it returns one of:
//
//   - an os error if the file cannot be opened or read
//   - ErrStatusLineNotFound if the file has no recognizable **Status:** line
//   - an *InvalidTransitionError if the transition is illegal in kind's matrix
func Validate(kind Kind, artifactPath string, to Status) (Status, error) {
	from, err := readStatus(artifactPath)
	if err != nil {
		return "", err
	}
	if err := Transition(kind, from, to); err != nil {
		return from, err
	}
	return from, nil
}

// Rewrite mutates the artifact's `**Status:**` line in place, replacing only
// the value text. Every other byte of the file (line ordering, indentation,
// line endings, trailing whitespace) is preserved (REQ: status-line-rewrite).
//
// Rewrite is retained for historical single-field callers. The returned
// string is the original line content for their explicitly documented legacy
// compensation path. Transaction-profile Task/Plan writers use RewriteBytes
// inside one TransformArtifact callback and never perform a late rollback.
//
// If the file has no `**Status:**` line, Rewrite returns ErrStatusLineNotFound
// and the file is left untouched.
func Rewrite(artifactPath string, newStatus Status) (string, error) {
	var originalLine string
	err := TransformArtifact(artifactPath, func(original []byte) ([]byte, error) {
		updated, line, transformErr := RewriteBytes(original, newStatus)
		originalLine = line
		return updated, transformErr
	})
	return originalLine, err
}

// RewriteBytes rewrites Status (and its frontmatter mirror) in memory. It is
// the pure transform used by compound artifact transactions.
func RewriteBytes(original []byte, newStatus Status) ([]byte, string, error) {
	lines := splitKeepTerminators(original)

	idx := findStatusLineIndex(lines)
	if idx < 0 {
		return nil, "", ErrStatusLineNotFound
	}

	originalLine := lines[idx]
	body, terminator := splitTerminator(originalLine)
	m := statusLineRe.FindStringSubmatch(body)
	indent := m[1]
	trailing := m[3]
	newBody := fmt.Sprintf("%s**Status:** %s%s", indent, string(newStatus), trailing)
	lines[idx] = newBody + terminator

	// artifact-frontmatter-convention REQ:scaffold-and-change-status — the body
	// `**Status:**` and the frontmatter `status:` mirror are rewritten together
	// in this one atomic write, so a mid-flight failure leaves both at the prior
	// value. Files with no frontmatter mirror are unaffected.
	if fmIdx := findFrontmatterStatusLineIndex(lines); fmIdx >= 0 {
		setFrontmatterStatusLine(lines, fmIdx, string(newStatus))
	}

	return joinLines(lines), originalLine, nil
}

// StatusFromBytes returns the body Status parsed from bytes held by a compound
// transaction. It never reads from disk.
func StatusFromBytes(original []byte) (Status, error) {
	lines := splitKeepTerminators(original)
	idx := findStatusLineIndex(lines)
	if idx < 0 {
		return "", ErrStatusLineNotFound
	}
	body, _ := splitTerminator(lines[idx])
	m := statusLineRe.FindStringSubmatch(body)
	if m == nil {
		return "", ErrStatusLineNotFound
	}
	return Status(strings.TrimSpace(m[2])), nil
}

// Rollback is the explicit legacy compensating status-line writer. It is not
// part of the Task/Plan transaction profile and MUST NOT be used after their
// artifact commit or post-mutation callback failure.
//
// Rollback locates the file's current `**Status:**` line (which is now the
// MUTATED value), replaces that single line with originalStatusLine, and
// writes the file back. After Rollback returns nil, the file content is
// byte-identical to its pre-Rewrite state.
//
// If the file has been mutated externally between Rewrite and Rollback such
// that no `**Status:**` line remains, Rollback returns ErrStatusLineNotFound.
// It does not prove ownership of unrelated current bytes; callers that have
// migrated to ArtifactTransaction must retain committed state instead.
func Rollback(artifactPath string, originalStatusLine string) error {
	return TransformArtifact(artifactPath, func(current []byte) ([]byte, error) {
		return RollbackBytes(current, originalStatusLine)
	})
}

// RollbackBytes is the pure in-memory legacy status-line restoration transform.
func RollbackBytes(current []byte, originalStatusLine string) ([]byte, error) {
	lines := splitKeepTerminators(current)
	idx := findStatusLineIndex(lines)
	if idx < 0 {
		return nil, ErrStatusLineNotFound
	}
	lines[idx] = originalStatusLine
	// Re-mirror the frontmatter `status:` from the restored body value so the
	// rollback leaves both surfaces at the prior value, in lockstep with the
	// dual-write in Rewrite.
	if fmIdx := findFrontmatterStatusLineIndex(lines); fmIdx >= 0 {
		if v := bodyStatusValue(originalStatusLine); v != "" {
			setFrontmatterStatusLine(lines, fmIdx, v)
		}
	}
	return joinLines(lines), nil
}

// findFrontmatterStatusLineIndex returns the index of the `status:` line inside
// a leading `---`-fenced YAML frontmatter block, or -1 when the file has no
// such block or the block carries no `status:` key. The search stops at the
// closing fence so a body line is never mistaken for the mirror.
func findFrontmatterStatusLineIndex(lines []string) int {
	if len(lines) == 0 {
		return -1
	}
	if body, _ := splitTerminator(lines[0]); body != "---" {
		return -1
	}
	for i := 1; i < len(lines); i++ {
		body, _ := splitTerminator(lines[i])
		if body == "---" {
			return -1
		}
		if fmStatusLineRe.MatchString(body) {
			return i
		}
	}
	return -1
}

// setFrontmatterStatusLine rewrites the value of the frontmatter `status:` line
// at lines[idx] to value, preserving its indentation and trailing whitespace.
func setFrontmatterStatusLine(lines []string, idx int, value string) {
	body, terminator := splitTerminator(lines[idx])
	m := fmStatusLineRe.FindStringSubmatch(body)
	lines[idx] = fmt.Sprintf("%sstatus: %s%s", m[1], value, m[3]) + terminator
}

// bodyStatusValue extracts the status value from a body `**Status:**` line
// (terminator included), or "" when the line is not a recognizable status line.
func bodyStatusValue(statusLine string) string {
	body, _ := splitTerminator(statusLine)
	m := statusLineRe.FindStringSubmatch(body)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[2])
}

// readStatus opens the file and returns the value text of the first
// `**Status:**` line. Returns ErrStatusLineNotFound if no such line is
// present.
func readStatus(artifactPath string) (Status, error) {
	f, err := os.Open(artifactPath)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if m := statusLineRe.FindStringSubmatch(line); m != nil {
			return Status(strings.TrimSpace(m[2])), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", ErrStatusLineNotFound
}

// findStatusLineIndex returns the index in lines of the first line whose
// body (terminator stripped) matches statusLineRe, or -1 if no such line
// exists.
func findStatusLineIndex(lines []string) int {
	for i, ln := range lines {
		body, _ := splitTerminator(ln)
		if statusLineRe.MatchString(body) {
			return i
		}
	}
	return -1
}

// splitKeepTerminators splits content on '\n' boundaries, retaining the
// terminator on each line so that joinLines round-trips byte-for-byte.
//
// Trailing data without a final newline is captured as a final element with
// no terminator. An empty input returns an empty slice (joinLines will then
// produce empty output).
func splitKeepTerminators(content []byte) []string {
	if len(content) == 0 {
		return nil
	}
	var lines []string
	start := 0
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			lines = append(lines, string(content[start:i+1]))
			start = i + 1
		}
	}
	if start < len(content) {
		lines = append(lines, string(content[start:]))
	}
	return lines
}

// splitTerminator splits a line into (body, terminator). The terminator is
// "" for the (possibly only) line with no trailing newline; otherwise it is
// "\n" or "\r\n".
func splitTerminator(line string) (string, string) {
	if strings.HasSuffix(line, "\r\n") {
		return line[:len(line)-2], "\r\n"
	}
	if strings.HasSuffix(line, "\n") {
		return line[:len(line)-1], "\n"
	}
	return line, ""
}

// joinLines concatenates lines (each of which retains its own terminator)
// into a single byte slice.
func joinLines(lines []string) []byte {
	var buf bytes.Buffer
	for _, ln := range lines {
		buf.WriteString(ln)
	}
	return buf.Bytes()
}

// writeFileAtomicExpected stages content beside dst, performs a deterministic
// final identity check against expected, then renames and syncs the containing
// directory. The check detects non-cooperating edits observed before rename;
// it is deliberately not called compare-and-swap because POSIX rename cannot
// make the preceding read and replacement one indivisible filesystem action.
func writeFileAtomicExpected(dst string, expected, content []byte) error {
	stat, err := os.Stat(dst)
	if err != nil {
		return err
	}
	tmp, err := osCreateTemp(dirOf(dst), ".lifecycle-rewrite-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = os.Remove(tmpName)
	}
	if _, err := ioCopy(tmp, bytes.NewReader(content)); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := fileSync(tmp); err != nil {
		_ = fileClose(tmp)
		cleanup()
		return err
	}
	if err := fileClose(tmp); err != nil {
		cleanup()
		return err
	}
	if err := osChmod(tmpName, stat.Mode().Perm()); err != nil {
		cleanup()
		return err
	}
	current, err := osReadBeforeRename(dst)
	if err != nil {
		cleanup()
		return err
	}
	if !bytes.Equal(current, expected) {
		cleanup()
		return ErrConcurrentMutation
	}
	if err := osRename(tmpName, dst); err != nil {
		cleanup()
		return err
	}
	dir, err := osOpenDir(dirOf(dst))
	if err != nil {
		return CommittedError(dst, "opening artifact directory for durable sync", err)
	}
	if err := fileSync(dir); err != nil {
		_ = fileClose(dir)
		return CommittedError(dst, "syncing artifact directory", err)
	}
	if err := fileClose(dir); err != nil {
		return CommittedError(dst, "closing artifact directory", err)
	}
	return nil
}

// dirOf returns the directory portion of path. Uses a tiny local
// implementation rather than importing path/filepath to keep this file
// dependency-light and consistent in style with the rest of the package.
func dirOf(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			if i == 0 {
				return p[:1]
			}
			return p[:i]
		}
	}
	return "."
}
