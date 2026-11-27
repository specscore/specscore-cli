// Package conflictmarker detects unresolved git merge-conflict markers in
// SpecScore-managed text. It is deliberately small and dependency-free so a
// source-of-truth table writer (the Lessons index in pkg/lint, the Rules index
// in pkg/rule) can refuse a conflicted file before rewriting it, without
// importing pkg/lint
// (cli/spec/lint#req:merge-conflict-markers-detected,
// cli/spec/lint#req:writers-refuse-conflicted-files).
package conflictmarker

import (
	"errors"
	"fmt"
	"os"
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
// any conflict marker, and nil otherwise.
func Check(path string, data []byte) error {
	if markers := Scan(data); len(markers) > 0 {
		return &Error{Path: path, Markers: markers}
	}
	return nil
}

// CheckFile reads path and runs Check on it. A missing file is not
// conflicted (a writer about to create it has nothing to refuse).
func CheckFile(path string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return Check(path, data)
}

// Scan returns every git merge-conflict marker line in data: a line starting
// at column 0 with `<<<<<<< ` (ours), `||||||| ` (diff3/zdiff3 base) or
// `>>>>>>> ` (theirs), and a line that is exactly `=======` — the separator —
// only while inside at least one open `<<<<<<<` block, because a bare
// `=======` is also a valid Markdown setext H1 underline. A depth counter
// tracks nested blocks so an inner `>>>>>>>` does not hide the outer
// separator. CRLF line endings are tolerated.
//
// Markdown code fences are deliberately NOT special: a real conflict inside a
// fenced block (a Synopsis, a YAML example) is exactly as unresolved as one in
// prose, and fence tracking can itself be fooled by a conflict that adds or
// drops a fence line. A document that needs to quote a marker indents it, so
// the marker no longer starts at column 0.
func Scan(data []byte) []Marker {
	var out []Marker
	depth := 0
	for i, raw := range strings.Split(string(data), "\n") {
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
