package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const featureSpecURL = "https://specscore.md/feature-specification"
const planSpecURL = "https://specscore.md/plan-specification"

// footerDoc builds a feature-README body with an optional leading frontmatter
// block and an optional adherence footer carrying footerURL.
func footerDoc(frontmatter, footerURL string) string {
	body := "# Feature: Sample\n\n## Summary\n\nText.\n"
	if footerURL != "" {
		body += "\n---\n*This document follows the " + footerURL + "*\n"
	}
	if frontmatter == "" {
		return body
	}
	return "---\n" + frontmatter + "---\n\n" + body
}

func TestFooterFormatMirror_Meta(t *testing.T) {
	c := newFooterFormatMirrorChecker()
	if got := c.name(); got != "footer-format-mirror" {
		t.Errorf("name() = %q, want footer-format-mirror", got)
	}
	if got := c.severity(); got != "warning" {
		t.Errorf("severity() = %q, want warning (graced)", got)
	}
}

func TestFooterFormatMirror_Check(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantViol bool
	}{
		{
			name:     "footer matches format passes",
			content:  footerDoc("format: "+featureSpecURL+"\n", featureSpecURL),
			wantViol: false,
		},
		{
			name:     "trailing slash tolerated",
			content:  footerDoc("format: "+featureSpecURL+"\n", featureSpecURL+"/"),
			wantViol: false,
		},
		{
			name:     "footer differs from format is flagged",
			content:  footerDoc("format: "+featureSpecURL+"\n", planSpecURL),
			wantViol: true,
		},
		{
			name:     "no frontmatter is skipped",
			content:  footerDoc("", featureSpecURL),
			wantViol: false,
		},
		{
			name:     "frontmatter without format key is skipped",
			content:  footerDoc("status: Draft\n", planSpecURL),
			wantViol: false,
		},
		{
			name:     "format present but no footer is skipped",
			content:  footerDoc("format: "+featureSpecURL+"\n", ""),
			wantViol: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			specRoot := writeSpec(t, map[string]string{
				"features/sample/README.md": tc.content,
			})
			violations, err := newFooterFormatMirrorChecker().check(specRoot)
			if err != nil {
				t.Fatal(err)
			}
			var v *Violation
			for i := range violations {
				if violations[i].Rule == "footer-format-mirror" {
					v = &violations[i]
				}
			}
			if tc.wantViol {
				if v == nil {
					t.Fatalf("expected a footer-format-mirror violation, got %+v", violations)
				}
				if v.Severity != "warning" {
					t.Errorf("severity = %q, want warning", v.Severity)
				}
				if !strings.Contains(v.Message, "does not match the canonical frontmatter") {
					t.Errorf("unexpected message %q", v.Message)
				}
			} else if v != nil {
				t.Errorf("expected no violation, got %+v", *v)
			}
		})
	}
}

func TestFooterFormatMirror_WalkError(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/ok.md": "# Idea: OK\n",
	})
	badDir := filepath.Join(specRoot, "ideas", "bad-subdir")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(badDir, 0o000); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(badDir, 0o755) }()

	if _, err := newFooterFormatMirrorChecker().check(specRoot); err == nil {
		t.Error("expected error for unreadable subdir under ideas")
	}
}

func TestFooterFormatMirror_Fix(t *testing.T) {
	// Footer (plan URL) disagrees with the canonical frontmatter format
	// (feature URL); --fix rewrites the footer from format, leaving format
	// itself untouched.
	specRoot := writeSpec(t, map[string]string{
		"features/sample/README.md": footerDoc("format: "+featureSpecURL+"\n", planSpecURL),
	})
	c := newFooterFormatMirrorChecker().(fixer)
	if err := c.fix(specRoot); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(specRoot, "features", "sample", "README.md"))
	if footer := extractFooterURL(got); footer != featureSpecURL {
		t.Errorf("footer URL after fix = %q, want %q\n%s", footer, featureSpecURL, got)
	}
	fields, _ := parseLeadingFrontmatter(got)
	if fields["format"] != featureSpecURL {
		t.Errorf("frontmatter format was changed to %q (must stay canonical)\n%s", fields["format"], got)
	}
	// Re-check clean.
	violations, err := newFooterFormatMirrorChecker().check(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range violations {
		if v.Rule == "footer-format-mirror" {
			t.Errorf("still flagged after fix: %+v", v)
		}
	}
}

func TestFooterFormatMirror_FixLeavesUntouched(t *testing.T) {
	matched := footerDoc("format: "+featureSpecURL+"\n", featureSpecURL)
	noFormat := footerDoc("", planSpecURL)
	specRoot := writeSpec(t, map[string]string{
		"features/matched/README.md":  matched,
		"features/noformat/README.md": noFormat,
	})
	c := newFooterFormatMirrorChecker().(fixer)
	if err := c.fix(specRoot); err != nil {
		t.Fatal(err)
	}
	for rel, want := range map[string]string{
		"features/matched/README.md":  matched,
		"features/noformat/README.md": noFormat,
	} {
		got, _ := os.ReadFile(filepath.Join(specRoot, filepath.FromSlash(rel)))
		if string(got) != want {
			t.Errorf("%s was modified by fix:\n%s", rel, got)
		}
	}
}

func TestFooterFormatMirror_FixWalkError(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/ok.md": "# Idea: OK\n",
	})
	badDir := filepath.Join(specRoot, "ideas", "bad-subdir")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(badDir, 0o000); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(badDir, 0o755) }()

	c := newFooterFormatMirrorChecker().(fixer)
	if err := c.fix(specRoot); err == nil {
		t.Error("expected error for unreadable subdir under ideas")
	}
}

func TestFooterFormatMirror_FixWriteError(t *testing.T) {
	// Two read-only artifacts whose footer disagrees with format: the first
	// write fails (setting writeErr); the second exercises the early return.
	doc := "---\nformat: https://specscore.md/idea-specification\n---\n\n# Idea\n\n---\n*This document follows the " + planSpecURL + "*\n"
	specRoot := writeSpec(t, map[string]string{
		"ideas/locked-a.md": doc,
		"ideas/locked-b.md": doc,
	})
	for _, n := range []string{"locked-a.md", "locked-b.md"} {
		p := filepath.Join(specRoot, "ideas", n)
		if err := os.Chmod(p, 0o444); err != nil {
			t.Skip("cannot change permissions")
		}
		defer func() { _ = os.Chmod(p, 0o644) }()
	}

	c := newFooterFormatMirrorChecker().(fixer)
	if err := c.fix(specRoot); err == nil {
		t.Error("expected write error fixing read-only artifacts")
	}
}

func TestExtractFooterURL(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "last spec URL in body wins, frontmatter format excluded",
			content: "---\nformat: " + featureSpecURL + "\n---\n\n# X\n\nsee " + planSpecURL + "\n\n---\n*This document follows the " + featureSpecURL + "*\n",
			want:    featureSpecURL,
		},
		{
			name:    "no spec URL returns empty",
			content: "# X\n\nNothing here.\n",
			want:    "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractFooterURL([]byte(tc.content)); got != tc.want {
				t.Errorf("extractFooterURL = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestReplaceLastSpecURL_NoMatch(t *testing.T) {
	content := []byte("# Title\n\nNo spec URL here.\n")
	if got := replaceLastSpecURL(content, featureSpecURL); string(got) != string(content) {
		t.Errorf("replaceLastSpecURL changed content with no match: %q", got)
	}
}
