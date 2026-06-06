package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// featureWithStatus builds a minimal feature-README body carrying an optional
// leading frontmatter block and a body `**Status:**` line. The feature README
// is a status-bearing docTypeTarget, so the status-mirror checker inspects it.
func statusBearingBody(frontmatter, bodyStatusLine string) string {
	body := "# Feature: Sample\n\n"
	if bodyStatusLine != "" {
		body += bodyStatusLine + "\n"
	}
	body += "\n## Summary\n\nText.\n"
	if frontmatter == "" {
		return body
	}
	return "---\n" + frontmatter + "---\n\n" + body
}

func TestStatusMirrorChecker_Meta(t *testing.T) {
	c := newStatusMirrorChecker()
	if got := c.name(); got != "status-mirror" {
		t.Errorf("name() = %q, want status-mirror", got)
	}
	if got := c.severity(); got != "error" {
		t.Errorf("severity() = %q, want error (enforced)", got)
	}
}

func TestStatusMirrorChecker_Check(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		wantViol   bool
		wantSubstr string
	}{
		{
			name:       "missing frontmatter status is flagged",
			content:    statusBearingBody("", "**Status:** Approved"),
			wantViol:   true,
			wantSubstr: "missing required frontmatter field",
		},
		{
			name:     "matching mirror passes",
			content:  statusBearingBody("status: Approved\n", "**Status:** Approved"),
			wantViol: false,
		},
		{
			name:     "case-insensitive whitespace-trimmed match passes",
			content:  statusBearingBody("status:   approved  \n", "**Status:** Approved"),
			wantViol: false,
		},
		{
			name:       "drift is flagged",
			content:    statusBearingBody("status: Draft\n", "**Status:** Approved"),
			wantViol:   true,
			wantSubstr: "does not mirror body",
		},
		{
			name:     "no body status line is skipped",
			content:  statusBearingBody("", ""),
			wantViol: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			specRoot := writeSpec(t, map[string]string{
				"features/sample/README.md": tc.content,
			})
			c := newStatusMirrorChecker()
			violations, err := c.check(specRoot)
			if err != nil {
				t.Fatal(err)
			}

			var v *Violation
			for i := range violations {
				if violations[i].Rule == "status-mirror" {
					v = &violations[i]
				}
			}
			if tc.wantViol {
				if v == nil {
					t.Fatalf("expected a status-mirror violation, got %+v", violations)
				}
				if v.Severity != "error" {
					t.Errorf("severity = %q, want error", v.Severity)
				}
				if !strings.Contains(v.Message, tc.wantSubstr) {
					t.Errorf("message %q does not contain %q", v.Message, tc.wantSubstr)
				}
			} else if v != nil {
				t.Errorf("expected no status-mirror violation, got %+v", *v)
			}
		})
	}
}

// TestStatusMirrorChecker_StatusLessRejectsStatus covers REQ:lint-status-mirror
// case (b): a status-less type (features-index README) carrying a frontmatter
// `status:` is flagged, while the same type with no `status:` passes — keyed by
// type, not file location.
func TestStatusMirrorChecker_StatusLessRejectsStatus(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantViol bool
	}{
		{
			name:     "status-less carrying status is flagged",
			content:  "---\nstatus: Draft\n---\n\n# Features\n",
			wantViol: true,
		},
		{
			name:     "status-less with no frontmatter passes",
			content:  "# Features\n\nNo frontmatter.\n",
			wantViol: false,
		},
		{
			name:     "status-less with frontmatter but no status key passes",
			content:  "---\nformat: https://specscore.md/features-index-specification\n---\n\n# Features\n",
			wantViol: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			specRoot := writeSpec(t, map[string]string{
				"features/README.md": tc.content,
			})
			violations, err := newStatusMirrorChecker().check(specRoot)
			if err != nil {
				t.Fatal(err)
			}
			var v *Violation
			for i := range violations {
				if violations[i].Rule == "status-mirror" {
					v = &violations[i]
				}
			}
			if tc.wantViol {
				if v == nil {
					t.Fatalf("expected a status-mirror violation, got %+v", violations)
				}
				if v.Severity != "error" {
					t.Errorf("severity = %q, want error", v.Severity)
				}
				if !strings.Contains(v.Message, "must not carry") {
					t.Errorf("message %q does not mention rejection", v.Message)
				}
			} else if v != nil {
				t.Errorf("expected no status-mirror violation, got %+v", *v)
			}
		})
	}
}

func TestStatusMirrorChecker_WalkError(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/ok.md": "# Idea: OK\n\n**Status:** Draft\n",
	})
	badDir := filepath.Join(specRoot, "ideas", "bad-subdir")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(badDir, 0o000); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(badDir, 0o755) }()

	c := newStatusMirrorChecker()
	if _, err := c.check(specRoot); err == nil {
		t.Error("expected error for unreadable subdir under ideas")
	}
}

func TestStatusMirrorChecker_Fix(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		wantStatus   string // expected frontmatter status: value after fix
		wantPreserve string // substring that must survive the fix (empty = none)
	}{
		{
			name:         "missing status inserts into existing block, preserving format",
			content:      statusBearingBody("format: https://specscore.md/feature-specification\n", "**Status:** Implementing"),
			wantStatus:   "Implementing",
			wantPreserve: "format: https://specscore.md/feature-specification",
		},
		{
			name:       "drift is rewritten from body",
			content:    statusBearingBody("status: Draft\n", "**Status:** Approved"),
			wantStatus: "Approved",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			specRoot := writeSpec(t, map[string]string{
				"features/sample/README.md": tc.content,
			})
			c := newStatusMirrorChecker().(fixer)
			if err := c.fix(specRoot); err != nil {
				t.Fatal(err)
			}
			got, _ := os.ReadFile(filepath.Join(specRoot, "features", "sample", "README.md"))
			fields, present := parseLeadingFrontmatter(got)
			if !present {
				t.Fatalf("expected a frontmatter block after fix, got:\n%s", got)
			}
			if fields["status"] != tc.wantStatus {
				t.Errorf("frontmatter status = %q, want %q\n%s", fields["status"], tc.wantStatus, got)
			}
			if tc.wantPreserve != "" && !strings.Contains(string(got), tc.wantPreserve) {
				t.Errorf("fix dropped preserved content %q:\n%s", tc.wantPreserve, got)
			}
			// Re-check: the artifact must now be clean.
			violations, err := newStatusMirrorChecker().check(specRoot)
			if err != nil {
				t.Fatal(err)
			}
			for _, v := range violations {
				if v.Rule == "status-mirror" {
					t.Errorf("artifact still flagged after fix: %+v", v)
				}
			}
		})
	}
}

// TestStatusMirrorChecker_FixLeavesUntouched confirms fix never rewrites:
//   - an already-mirrored artifact (block present, status in sync),
//   - an artifact with no body status to mirror, and
//   - a status-bearing artifact with NO frontmatter block (block creation is
//     the migration's job, not this rule's — fix must not fabricate one).
//
// Each is left byte-for-byte unchanged.
func TestStatusMirrorChecker_FixLeavesUntouched(t *testing.T) {
	inSync := statusBearingBody("status: Approved\n", "**Status:** Approved")
	noBodyStatus := statusBearingBody("format: https://specscore.md/feature-specification\n", "")
	noBlock := statusBearingBody("", "**Status:** Approved")
	specRoot := writeSpec(t, map[string]string{
		"features/synced/README.md":       inSync,
		"features/nobodystatus/README.md": noBodyStatus,
		"features/noblock/README.md":      noBlock,
	})
	c := newStatusMirrorChecker().(fixer)
	if err := c.fix(specRoot); err != nil {
		t.Fatal(err)
	}
	for rel, want := range map[string]string{
		"features/synced/README.md":       inSync,
		"features/nobodystatus/README.md": noBodyStatus,
		"features/noblock/README.md":      noBlock,
	} {
		got, _ := os.ReadFile(filepath.Join(specRoot, filepath.FromSlash(rel)))
		if string(got) != want {
			t.Errorf("%s was modified by fix:\n%s", rel, got)
		}
	}
}

func TestStatusMirrorChecker_FixWalkError(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/ok.md": "# Idea: OK\n\n**Status:** Draft\n",
	})
	badDir := filepath.Join(specRoot, "ideas", "bad-subdir")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(badDir, 0o000); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(badDir, 0o755) }()

	c := newStatusMirrorChecker().(fixer)
	if err := c.fix(specRoot); err == nil {
		t.Error("expected error for unreadable subdir under ideas")
	}
}

func TestStatusMirrorChecker_FixWriteError(t *testing.T) {
	// Two read-only artifacts under one walker: the first write fails (setting
	// writeErr); the second invocation exercises the `writeErr != nil` early
	// return before fix returns the aggregated error.
	// Each artifact carries a frontmatter block missing `status:`, so fix
	// attempts a write (which fails on the read-only file).
	specRoot := writeSpec(t, map[string]string{
		"ideas/locked-a.md": "---\nformat: https://specscore.md/idea-specification\n---\n\n# Idea: Locked A\n\n**Status:** Draft\n",
		"ideas/locked-b.md": "---\nformat: https://specscore.md/idea-specification\n---\n\n# Idea: Locked B\n\n**Status:** Draft\n",
	})
	for _, n := range []string{"locked-a.md", "locked-b.md"} {
		p := filepath.Join(specRoot, "ideas", n)
		if err := os.Chmod(p, 0o444); err != nil {
			t.Skip("cannot change permissions")
		}
		defer func() { _ = os.Chmod(p, 0o644) }()
	}

	c := newStatusMirrorChecker().(fixer)
	if err := c.fix(specRoot); err == nil {
		t.Error("expected write error fixing read-only artifacts")
	}
}

func TestExtractBodyStatus(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "body status after frontmatter, frontmatter mirror ignored",
			content: "---\nstatus: Draft\n---\n\n# X\n\n**Status:** Approved\n",
			want:    "Approved",
		},
		{
			name:    "backtick-quoted value trimmed",
			content: "**Status:** `Implementing`\n",
			want:    "Implementing",
		},
		{
			name:    "no status line",
			content: "# X\n\nNo status here.\n",
			want:    "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractBodyStatus([]byte(tc.content)); got != tc.want {
				t.Errorf("extractBodyStatus = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSetFrontmatterStatus(t *testing.T) {
	tests := []struct {
		name    string
		content string
		status  string
		want    string
	}{
		{
			name:    "no opening fence returns content unchanged",
			content: "# Title\n\n**Status:** Draft\n",
			status:  "Draft",
			want:    "# Title\n\n**Status:** Draft\n",
		},
		{
			name:    "opening fence without closing returns content unchanged",
			content: "---\nformat: x\nnever closed\n",
			status:  "Draft",
			want:    "---\nformat: x\nnever closed\n",
		},
		{
			name:    "existing status line is replaced",
			content: "---\nformat: x\nstatus: Old\n---\n\n# B\n",
			status:  "New",
			want:    "---\nformat: x\nstatus: New\n---\n\n# B\n",
		},
		{
			name:    "status inserted before closing fence when absent",
			content: "---\nformat: x\n---\n\n# B\n",
			status:  "New",
			want:    "---\nformat: x\nstatus: New\n---\n\n# B\n",
		},
		{
			name:    "comment and indented lines in block are skipped when scanning",
			content: "---\n# note\n  indent: y\n---\n\n# B\n",
			status:  "New",
			want:    "---\n# note\n  indent: y\nstatus: New\n---\n\n# B\n",
		},
		{
			name:    "colonless block line is skipped when scanning",
			content: "---\nnocolon\n---\n\n# B\n",
			status:  "New",
			want:    "---\nnocolon\nstatus: New\n---\n\n# B\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := string(setFrontmatterStatus([]byte(tc.content), tc.status)); got != tc.want {
				t.Errorf("setFrontmatterStatus =\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func TestBodyAfterFrontmatter(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"no opening fence", "# Title\nbody\n", "# Title\nbody\n"},
		{"opening without closing", "---\nformat: x\nbody\n", "---\nformat: x\nbody\n"},
		{"complete block", "---\nformat: x\n---\nbody\n", "body\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := bodyAfterFrontmatter([]byte(tc.content)); got != tc.want {
				t.Errorf("bodyAfterFrontmatter = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFrontmatterStatus_NoBlock(t *testing.T) {
	if got := frontmatterStatus([]byte("# No frontmatter\n")); got != "" {
		t.Errorf("frontmatterStatus = %q, want empty", got)
	}
}
