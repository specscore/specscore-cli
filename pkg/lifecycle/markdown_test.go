package lifecycle

import (
	"reflect"
	"strings"
	"testing"
)

// Lifecycle writers and the Plan parser share this scanner. Keep its grammar
// covered here so a writer cannot regress into treating Markdown examples as
// artifact structure while Plan parsing remains safe.
func TestStructuralMarkdownMask(t *testing.T) {
	tests := []struct {
		name            string
		body            string
		retainedComment string
		want            []bool
	}{
		{
			name: "empty document",
			want: []bool{},
		},
		{
			name: "frontmatter including comments is not structure",
			body: "---\n<!-- frontmatter comment -->\n...\n# Plan: Real",
			want: []bool{false, false, false, true},
		},
		{
			name: "BOM prefixed frontmatter including comments is not structure",
			body: "\ufeff---\n# Plan: frontmatter fake\n<!-- comment -->\n...\n# Plan: Real",
			want: []bool{false, false, false, false, true},
		},
		{
			name: "multiline comments and a second unmatched opener stay hidden",
			body: "<!-- one --> <!-- two\ncomment body\n-->\nreal",
			want: []bool{false, false, false, true},
		},
		{
			name: "backtick fence survives blank and all space lines until trailing space close",
			body: "```go\n\n   \n# fake\n```   \nreal",
			want: []bool{false, false, false, false, false, true},
		},
		{
			name: "shorter fence never closes a longer opener",
			body: "````go\n```\n# fake\n````\nreal",
			want: []bool{false, false, false, false, true},
		},
		{
			name: "tilde fence and all code indentation are hidden",
			body: "~~~\n# fake\n~~~\n    fake\n\tfake\n  \tfake\n   prose",
			want: []bool{false, false, false, false, false, false, true},
		},
		{
			name:            "only the caller retained HTML comment is structural",
			body:            "<!-- retained -->\n    <!-- retained -->\n<!-- another comment -->",
			retainedComment: "<!-- retained -->",
			want:            []bool{true, false, false},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var lines []string
			if tc.body != "" {
				lines = strings.Split(tc.body, "\n")
			}
			if got := StructuralMarkdownMask(lines, tc.retainedComment); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("mask = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStructuralMarkdownHelpers(t *testing.T) {
	for _, tc := range []struct {
		line string
		want bool
	}{
		{"", false}, {"   ", false}, {"   text", false}, {"    text", true}, {"\ttext", true}, {"  \ttext", true},
	} {
		if got := IsIndentedCode(tc.line); got != tc.want {
			t.Errorf("IsIndentedCode(%q) = %t, want %t", tc.line, got, tc.want)
		}
	}
	for _, tc := range []struct {
		line string
		want bool
	}{
		{"---", true}, {" ... ", true}, {"----", false}, {"", false},
	} {
		if got := IsFrontmatterFence(tc.line); got != tc.want {
			t.Errorf("IsFrontmatterFence(%q) = %t, want %t", tc.line, got, tc.want)
		}
	}
	if !IsLeadingFrontmatterFence("\ufeff---") {
		t.Fatal("BOM prefixed opening frontmatter fence must be recognized")
	}
	for _, tc := range []struct {
		fragment string
		want     bool
	}{
		{"plain", false}, {"<!-- open", true}, {"<!-- closed -->", false}, {"<!-- one --> <!-- two", true},
	} {
		if got := HTMLCommentContinues(tc.fragment); got != tc.want {
			t.Errorf("HTMLCommentContinues(%q) = %t, want %t", tc.fragment, got, tc.want)
		}
	}
	for _, tc := range []struct {
		line   string
		marker byte
		length int
		want   bool
	}{
		{"", '`', 3, false}, {"   ", '`', 3, false}, {"```   ", '`', 3, true}, {"~~~", '`', 3, false}, {"``", '`', 3, false},
	} {
		if got := FenceCloses(tc.line, tc.marker, tc.length); got != tc.want {
			t.Errorf("FenceCloses(%q,%q,%d) = %t, want %t", tc.line, tc.marker, tc.length, got, tc.want)
		}
	}
	for _, tc := range []struct {
		line   string
		marker byte
		length int
		ok     bool
	}{
		{"", 0, 0, false}, {"   ", 0, 0, false}, {"    ```", 0, 0, false}, {"prose", 0, 0, false}, {"``", '`', 2, false}, {"```go", '`', 3, true}, {"~~~", '~', 3, true},
	} {
		marker, length, ok := fenceOpens(tc.line)
		if marker != tc.marker || length != tc.length || ok != tc.ok {
			t.Errorf("fenceOpens(%q) = (%q,%d,%t), want (%q,%d,%t)", tc.line, marker, length, ok, tc.marker, tc.length, tc.ok)
		}
	}
}
