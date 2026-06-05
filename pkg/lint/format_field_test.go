package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ideaWithFormat builds an Idea-file body carrying the given leading
// frontmatter block. The Idea walker (walkIdeaFiles) is the docTypeTarget used
// to exercise formatFieldChecker because its canonical URL is
// https://specscore.md/idea-specification and it visits ideas/*.md directly.
func ideaWithFrontmatter(frontmatter, body string) string {
	if frontmatter == "" {
		return body
	}
	return "---\n" + frontmatter + "---\n\n" + body
}

func TestFormatFieldChecker_Severity(t *testing.T) {
	c := newFormatFieldChecker()
	if got := c.severity(); got != "error" {
		t.Errorf("severity() = %q, want %q (enforced)", got, "error")
	}
	if got := c.name(); got != "format-field" {
		t.Errorf("name() = %q, want %q", got, "format-field")
	}
}

func TestFormatFieldChecker_Check(t *testing.T) {
	tests := []struct {
		name        string
		frontmatter string
		wantViol    bool
		wantSubstr  string
	}{
		{
			name:        "correct format passes",
			frontmatter: "format: https://specscore.md/idea-specification\n",
			wantViol:    false,
		},
		{
			name:        "trailing slash tolerated",
			frontmatter: "format: https://specscore.md/idea-specification/\n",
			wantViol:    false,
		},
		{
			name:        "quoted value passes",
			frontmatter: "format: \"https://specscore.md/idea-specification\"\n",
			wantViol:    false,
		},
		{
			name:        "missing frontmatter is flagged",
			frontmatter: "",
			wantViol:    true,
			wantSubstr:  "missing required frontmatter field",
		},
		{
			name:        "frontmatter present but no format key is flagged",
			frontmatter: "status: Draft\n",
			wantViol:    true,
			wantSubstr:  "missing required frontmatter field",
		},
		{
			name:        "wrong URL is flagged as mismatch",
			frontmatter: "format: https://specscore.md/feature-specification\n",
			wantViol:    true,
			wantSubstr:  "does not match the canonical URL",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			specRoot := writeSpec(t, map[string]string{
				"ideas/sample.md": ideaWithFrontmatter(tc.frontmatter, "# Idea: Sample\n"),
			})
			c := newFormatFieldChecker()
			violations, err := c.check(specRoot)
			if err != nil {
				t.Fatal(err)
			}

			var formatViol *Violation
			for i := range violations {
				if violations[i].Rule == "format-field" && violations[i].File == "ideas/sample.md" {
					formatViol = &violations[i]
				}
			}

			if tc.wantViol {
				if formatViol == nil {
					t.Fatalf("expected a format-field violation on ideas/sample.md, got %+v", violations)
				}
				if formatViol.Severity != "error" {
					t.Errorf("severity = %q, want error", formatViol.Severity)
				}
				if !strings.Contains(formatViol.Message, tc.wantSubstr) {
					t.Errorf("message %q does not contain %q", formatViol.Message, tc.wantSubstr)
				}
			} else if formatViol != nil {
				t.Errorf("expected no format-field violation, got %+v", *formatViol)
			}
		})
	}
}

// TestFormatFieldChecker_WalkError covers the `if err != nil { return nil, err }`
// branch by making a subdirectory under ideas/ unreadable so the Idea walker's
// filepath.Walk returns an error.
func TestFormatFieldChecker_WalkError(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/ok.md": ideaWithFrontmatter("format: https://specscore.md/idea-specification\n", "# Idea: OK\n"),
	})
	badDir := filepath.Join(specRoot, "ideas", "bad-subdir")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(badDir, 0o000); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(badDir, 0o755) }()

	c := newFormatFieldChecker()
	_, err := c.check(specRoot)
	if err == nil {
		t.Error("expected error for unreadable subdir under ideas")
	}
}

func TestParseLeadingFrontmatter(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantPresent bool
		wantFields  map[string]string
	}{
		{
			name:        "no opening fence",
			content:     "# Title\n\nNo frontmatter here.\n",
			wantPresent: false,
		},
		{
			name:        "opening fence but no closing fence",
			content:     "---\nformat: x\nnever closed\n",
			wantPresent: false,
		},
		{
			name:        "scalar fields parsed, comment/blank/indented/no-colon skipped",
			content:     "---\n# a comment\n\nformat: https://specscore.md/idea-specification\n  nested: ignored\nnocolon\nstatus: Draft\n---\n\n# Body\n",
			wantPresent: true,
			wantFields: map[string]string{
				"format": "https://specscore.md/idea-specification",
				"status": "Draft",
			},
		},
		{
			name:        "leading-colon line is skipped",
			content:     "---\n: novalue\nstatus: Draft\n---\n",
			wantPresent: true,
			wantFields:  map[string]string{"status": "Draft"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fields, present := parseLeadingFrontmatter([]byte(tc.content))
			if present != tc.wantPresent {
				t.Fatalf("present = %v, want %v", present, tc.wantPresent)
			}
			if !present {
				return
			}
			for k, want := range tc.wantFields {
				if got := fields[k]; got != want {
					t.Errorf("fields[%q] = %q, want %q", k, got, want)
				}
			}
			if len(fields) != len(tc.wantFields) {
				t.Errorf("fields = %v, want %v", fields, tc.wantFields)
			}
		})
	}
}

func TestStripQuotes(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{`"quoted"`, "quoted"},
		{`'quoted'`, "quoted"},
		{`bare`, "bare"},
		{`x`, "x"},                     // shorter than 2 → unchanged
		{``, ""},                       // empty → unchanged
		{`"unbalanced`, `"unbalanced`}, // only leading quote → unchanged
		{`unbalanced'`, `unbalanced'`}, // only trailing quote → unchanged
		{`'mixed"`, `'mixed"`},         // mismatched quotes → unchanged
	}
	for _, tc := range tests {
		if got := stripQuotes(tc.in); got != tc.want {
			t.Errorf("stripQuotes(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
