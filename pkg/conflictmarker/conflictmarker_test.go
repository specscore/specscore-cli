package conflictmarker

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestScan(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []Marker
	}{
		{"two-way", "a\n<<<<<<< HEAD\nx\n=======\ny\n>>>>>>> topic\nb\n",
			[]Marker{{2, "<<<<<<<"}, {4, "======="}, {6, ">>>>>>>"}}},
		{"diff3", "<<<<<<< ours\nx\n||||||| base\nz\n=======\ny\n>>>>>>> theirs\n",
			[]Marker{{1, "<<<<<<<"}, {3, "|||||||"}, {5, "======="}, {7, ">>>>>>>"}}},
		{"crlf", "<<<<<<< HEAD\r\nx\r\n=======\r\ny\r\n>>>>>>> t\r\n",
			[]Marker{{1, "<<<<<<<"}, {3, "======="}, {5, ">>>>>>>"}}},
		{"setext-heading-alone", "Title\n=======\n\nbody\n", nil},
		{"setext-after-block-closed", "<<<<<<< a\n=======\n>>>>>>> b\nTitle\n=======\n",
			[]Marker{{1, "<<<<<<<"}, {2, "======="}, {3, ">>>>>>>"}}},
		{"orphan-theirs-marker", "x\n>>>>>>> topic\n=======\n", []Marker{{2, ">>>>>>>"}}},
		// An inner block's closing marker must not hide the outer separator.
		{"nested", "<<<<<<< A\n<<<<<<< B\n=======\n>>>>>>> C\n=======\n>>>>>>> D\n",
			[]Marker{{1, "<<<<<<<"}, {2, "<<<<<<<"}, {3, "======="}, {4, ">>>>>>>"}, {5, "======="}, {6, ">>>>>>>"}}},
		{"unlabelled-or-longer-runs", "<<<<<<<\n<<<<<<<< x\n>>>>>>>>> y\n|||||||\n", nil},
		// The documented way to quote a marker: indent it off column 0.
		{"indented-not-a-marker", " <<<<<<< HEAD\n    =======\n\t>>>>>>> t\n", nil},
		// A real conflict inside a fenced code block is still a conflict.
		{"inside-fence-flagged", "```text\n<<<<<<< HEAD\n=======\n>>>>>>> t\n```\n",
			[]Marker{{2, "<<<<<<<"}, {3, "======="}, {4, ">>>>>>>"}}},
		// A conflict straddling a fence boundary is flagged in full.
		{"straddles-fence", "```go\n<<<<<<< HEAD\nx()\n```\nprose\n=======\n```\n>>>>>>> topic\n```\n",
			[]Marker{{2, "<<<<<<<"}, {6, "======="}, {8, ">>>>>>>"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Scan([]byte(tc.in)); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Scan = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCheckAndError(t *testing.T) {
	if err := Check("a.md", []byte("```\n    <<<<<<< x\n```\n")); err != nil {
		t.Fatalf("indented example: %v", err)
	}
	err := Check("index.txt", []byte("ok\n<<<<<<< HEAD\n"))
	var ce *Error
	if !errors.As(err, &ce) || !errors.Is(err, ErrConflicted) || ce.Path != "index.txt" || ce.Markers[0].Line != 2 {
		t.Fatalf("err = %#v, want *Error at index.txt:2", err)
	}
	if !strings.Contains(err.Error(), "index.txt:2") || !strings.Contains(err.Error(), "<<<<<<<") {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestCheckFile(t *testing.T) {
	dir := t.TempDir()
	if err := CheckFile(filepath.Join(dir, "missing.md")); err != nil {
		t.Fatalf("missing file: %v", err)
	}
	p := filepath.Join(dir, "README.md")
	if err := os.WriteFile(p, []byte(">>>>>>> t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckFile(p); !errors.Is(err, ErrConflicted) {
		t.Fatalf("err = %v, want ErrConflicted", err)
	}
	// A directory is unreadable as a file: the read error is returned as-is.
	if err := CheckFile(dir); err == nil || errors.Is(err, ErrConflicted) {
		t.Fatalf("err = %v, want the read error", err)
	}
}
