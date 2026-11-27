// Package conflictmarker detects unresolved git merge-conflict markers in
// SpecScore-managed text. It is deliberately small and dependency-free so
// every index writer (pkg/lint, pkg/rule, pkg/feature, pkg/lesson, …) can
// refuse a conflicted file before rewriting it, without importing pkg/lint
// (cli/spec/lint#req:merge-conflict-markers-detected,
// cli/spec/lint#req:writers-refuse-conflicted-files).
package conflictmarker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Width is git's default conflict-marker-size: every marker git writes is
// exactly seven repeated characters.
const Width = 7

// ErrConflicted is the sentinel every *Error unwraps to.
var ErrConflicted = errors.New("unresolved git merge-conflict markers")

// Marker is one conflict-marker line: its 1-based line number and the seven
// character marker it starts with.
type Marker struct {
	Line   int
	Marker string
}

// Error reports a file a writer refused to rewrite because it still carries
// conflict markers.
type Error struct {
	Path    string
	Markers []Marker
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s:%d: %v (first marker %q); resolve the conflict before rewriting this file",
		e.Path, e.Markers[0].Line, ErrConflicted, e.Markers[0].Marker)
}

func (e *Error) Unwrap() error { return ErrConflicted }

// Check returns an *Error when data — the current content of path — carries
// any conflict marker, and nil otherwise. Markdown fence handling is decided
// by path's extension (see Scan).
func Check(path string, data []byte) error {
	if markers := Scan(data, IsMarkdown(path)); len(markers) > 0 {
		return &Error{Path: path, Markers: markers}
	}
	return nil
}

// readFile is a seam for the defensive read-error branch of CheckFile.
var readFile = os.ReadFile

// CheckFile reads path and runs Check on it. A missing file is not
// conflicted (a writer about to create it has nothing to refuse).
func CheckFile(path string) error {
	data, err := readFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return Check(path, data)
}

// IsMarkdown reports whether path is a Markdown file, whose balanced fenced
// code blocks may legitimately quote conflict markers.
func IsMarkdown(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".md")
}

// Scan returns every git merge-conflict marker line in data: a line starting
// with `<<<<<<< ` (ours), `||||||| ` (diff3/zdiff3 base) or `>>>>>>> `
// (theirs), and a line that is exactly `=======` — the separator — only while
// inside at least one open `<<<<<<<` block, because a bare `=======` is also
// a valid Markdown setext H1 underline. A depth counter tracks nested blocks
// so an inner `>>>>>>>` does not hide the outer separator. CRLF line endings
// are tolerated.
//
// When markdown is true, lines inside a balanced CommonMark fenced code block
// are ignored, so a document can quote conflict markers as an example. A file
// whose fences do not balance (a conflict can itself add or drop a fence
// line) is scanned as if it had no fences, so a marker can never be hidden
// by an unclosed fence.
func Scan(data []byte, markdown bool) []Marker {
	lines := strings.Split(string(data), "\n")
	if !markdown {
		return scanLines(lines, nil)
	}
	inFence := make([]bool, len(lines))
	var fence Fence
	for i, raw := range lines {
		wasOpen := fence.Open()
		if fence.Feed(strings.TrimSuffix(raw, "\r")) || wasOpen {
			inFence[i] = true
		}
	}
	if fence.Open() {
		return scanLines(lines, nil)
	}
	return scanLines(lines, inFence)
}

func scanLines(lines []string, skip []bool) []Marker {
	var out []Marker
	depth := 0
	for i, raw := range lines {
		if skip != nil && skip[i] {
			continue
		}
		line := strings.TrimSuffix(raw, "\r")
		switch {
		case isLabelled(line, '<'):
			out = append(out, Marker{Line: i + 1, Marker: "<<<<<<<"})
			depth++
		case isLabelled(line, '|'):
			out = append(out, Marker{Line: i + 1, Marker: "|||||||"})
		case depth > 0 && line == strings.Repeat("=", Width):
			out = append(out, Marker{Line: i + 1, Marker: "======="})
		case isLabelled(line, '>'):
			out = append(out, Marker{Line: i + 1, Marker: ">>>>>>>"})
			if depth > 0 {
				depth--
			}
		}
	}
	return out
}

// isLabelled reports whether line is a git conflict marker made of exactly
// seven ch characters followed by a space (git always appends the side's
// label, e.g. `<<<<<<< HEAD`). Requiring the space keeps a longer run (an
// eighth character, a deeper blockquote) from matching.
func isLabelled(line string, ch byte) bool {
	return strings.HasPrefix(line, strings.Repeat(string(ch), Width)+" ")
}

// Fence tracks a CommonMark fenced code block: it opens on a line of three or
// more backticks or tildes, and closes only on a line of the same character,
// at least as long, with nothing after it.
type Fence struct {
	ch byte
	n  int
}

// Open reports whether a fenced code block is currently open.
func (f *Fence) Open() bool { return f.n > 0 }

// fenceRun returns the fence character and run length that t starts with, or
// 0, 0 when t does not start with three or more backticks or tildes.
func fenceRun(t string) (byte, int) {
	if t == "" || (t[0] != '`' && t[0] != '~') {
		return 0, 0
	}
	n := len(t) - len(strings.TrimLeft(t, t[:1]))
	if n < 3 {
		return 0, 0
	}
	return t[0], n
}

// Feed advances the fence state over one line and reports whether the line
// was a fence line (opening or closing).
func (f *Fence) Feed(line string) bool {
	t := strings.TrimSpace(line)
	ch, n := fenceRun(t)
	if !f.Open() {
		if n > 0 {
			f.ch, f.n = ch, n
			return true
		}
		return false
	}
	if ch == f.ch && n >= f.n && strings.TrimSpace(t[n:]) == "" {
		f.ch, f.n = 0, 0
		return true
	}
	return false
}
