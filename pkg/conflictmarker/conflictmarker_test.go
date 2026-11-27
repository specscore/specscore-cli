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
		name     string
		in       string
		markdown bool
		want     []Marker
	}{
		{"two-way", "a\n<<<<<<< HEAD\nx\n=======\ny\n>>>>>>> topic\nb\n", false,
			[]Marker{{2, "<<<<<<<"}, {4, "======="}, {6, ">>>>>>>"}}},
		{"diff3", "<<<<<<< ours\nx\n||||||| base\nz\n=======\ny\n>>>>>>> theirs\n", false,
			[]Marker{{1, "<<<<<<<"}, {3, "|||||||"}, {5, "======="}, {7, ">>>>>>>"}}},
		{"crlf", "<<<<<<< HEAD\r\nx\r\n=======\r\ny\r\n>>>>>>> t\r\n", false,
			[]Marker{{1, "<<<<<<<"}, {3, "======="}, {5, ">>>>>>>"}}},
		{"setext-heading-alone", "Title\n=======\n\nbody\n", true, nil},
		{"setext-after-block-closed", "<<<<<<< a\n=======\n>>>>>>> b\nTitle\n=======\n", true,
			[]Marker{{1, "<<<<<<<"}, {2, "======="}, {3, ">>>>>>>"}}},
		{"orphan-theirs-marker", "x\n>>>>>>> topic\n=======\n", false, []Marker{{2, ">>>>>>>"}}},
		// An inner block's closing marker must not hide the outer separator.
		{"nested", "<<<<<<< A\n<<<<<<< B\n=======\n>>>>>>> C\n=======\n>>>>>>> D\n", false,
			[]Marker{{1, "<<<<<<<"}, {2, "<<<<<<<"}, {3, "======="}, {4, ">>>>>>>"}, {5, "======="}, {6, ">>>>>>>"}}},
		{"unlabelled-or-longer-runs", "<<<<<<<\n<<<<<<<< x\n>>>>>>>>> y\n|||||||\n", false, nil},
		{"indented-not-a-marker", " <<<<<<< HEAD\n", false, nil},
		{"fenced-example-in-markdown", "```text\n<<<<<<< HEAD\n=======\n>>>>>>> t\n```\n", true, nil},
		{"fenced-example-outside-markdown", "```text\n<<<<<<< HEAD\n=======\n>>>>>>> t\n```\n", false,
			[]Marker{{2, "<<<<<<<"}, {3, "======="}, {4, ">>>>>>>"}}},
		{"real-conflict-after-fence", "~~~\nquoted\n~~~\n<<<<<<< HEAD\nx\n=======\n>>>>>>> t\n", true,
			[]Marker{{4, "<<<<<<<"}, {6, "======="}, {7, ">>>>>>>"}}},
		// A fence the conflict itself left unclosed cannot hide the markers.
		{"unbalanced-fence", "<<<<<<< HEAD\n```go\n=======\n>>>>>>> t\n", true,
			[]Marker{{1, "<<<<<<<"}, {3, "======="}, {4, ">>>>>>>"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Scan([]byte(tc.in), tc.markdown); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Scan = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFence(t *testing.T) {
	var f Fence
	if f.Feed("``") || f.Open() {
		t.Fatal("two backticks do not open a fence")
	}
	if !f.Feed("````md") || !f.Open() {
		t.Fatal("four backticks open a fence")
	}
	for _, notClosing := range []string{"```", "~~~~", "```` trailing"} {
		if f.Feed(notClosing) || !f.Open() {
			t.Fatalf("%q must not close a 4-backtick fence", notClosing)
		}
	}
	if !f.Feed("`````") || f.Open() {
		t.Fatal("a longer run of the same character closes the fence")
	}
	if f.Feed("") {
		t.Fatal("an empty line is not a fence line")
	}
}

func TestCheckAndError(t *testing.T) {
	if err := Check("a.md", []byte("```\n<<<<<<< x\n```\n")); err != nil {
		t.Fatalf("fenced example: %v", err)
	}
	err := Check("index.txt", []byte("ok\n<<<<<<< HEAD\n"))
	var ce *Error
	if !errors.As(err, &ce) || !errors.Is(err, ErrConflicted) || ce.Path != "index.txt" || ce.Markers[0].Line != 2 {
		t.Fatalf("err = %#v, want *Error at index.txt:2", err)
	}
	if !strings.Contains(err.Error(), "index.txt:2") || !strings.Contains(err.Error(), "<<<<<<<") {
		t.Fatalf("message = %q", err.Error())
	}
	if !IsMarkdown("X/README.MD") || IsMarkdown("specscore.yaml") {
		t.Fatal("IsMarkdown misclassified")
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
	prev := readFile
	t.Cleanup(func() { readFile = prev })
	readFile = func(string) ([]byte, error) { return nil, errors.New("unreadable") }
	if err := CheckFile(p); err == nil || errors.Is(err, ErrConflicted) || !strings.Contains(err.Error(), "unreadable") {
		t.Fatalf("err = %v, want the read error", err)
	}
}
