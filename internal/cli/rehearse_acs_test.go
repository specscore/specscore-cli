package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeACFixture(t *testing.T, dir, slug, body string) {
	t.Helper()
	acsDir := filepath.Join(dir, "_acs")
	if err := os.MkdirAll(acsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(acsDir, slug+".ac.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRehearseACs_Print(t *testing.T) {
	dir := t.TempDir()
	writeACFixture(t, dir, "session-hardened", "# AC: session-hardened\n\n**Status:** accepted\n\n## Statement\n\nThe cookie is HttpOnly.\n")
	// A non-AC file and a subdir are ignored.
	_ = os.WriteFile(filepath.Join(dir, "_acs", "notes.txt"), []byte("x"), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, "_acs", "sub"), 0o755)

	var buf bytes.Buffer
	if err := runRehearseACs(dir, false, &buf); err != nil {
		t.Fatalf("runRehearseACs: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "## Acceptance Criteria") ||
		!strings.Contains(out, "[session-hardened](_acs/session-hardened.ac.md)") ||
		!strings.Contains(out, "The cookie is HttpOnly.") {
		t.Errorf("summary output missing expected content:\n%s", out)
	}
}

func TestRehearseACs_BadDir(t *testing.T) {
	if err := runRehearseACs(t.TempDir(), false, &bytes.Buffer{}); err == nil {
		t.Fatal("want error when _acs/ is absent")
	}
}

func TestRehearseACs_BadACFile(t *testing.T) {
	dir := t.TempDir()
	writeACFixture(t, dir, "broken", "no AC heading here\n")
	if err := runRehearseACs(dir, false, &bytes.Buffer{}); err == nil {
		t.Fatal("want error for an AC file with no `# AC:` heading")
	}
}

func TestRehearseACs_Write(t *testing.T) {
	dir := t.TempDir()
	writeACFixture(t, dir, "a-one", "# AC: a-one\n\n## Statement\n\nA holds.\n")
	readme := "# Feature: demo\n\n## Behavior\n\nStuff.\n\n## Acceptance Criteria\n\nOLD INLINE CONTENT\n\n### AC: a-one\n\nold body\n\n## Open Questions\n\nNone.\n"
	readmePath := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readmePath, []byte(readme), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := runRehearseACs(dir, true, &buf); err != nil {
		t.Fatalf("runRehearseACs --write: %v", err)
	}
	got, _ := os.ReadFile(readmePath)
	s := string(got)
	if !strings.Contains(s, "[a-one](_acs/a-one.ac.md)") || !strings.Contains(s, "A holds.") {
		t.Errorf("README not regenerated with the table:\n%s", s)
	}
	if strings.Contains(s, "OLD INLINE CONTENT") || strings.Contains(s, "old body") {
		t.Errorf("old inline AC section not replaced:\n%s", s)
	}
	// Surrounding sections preserved.
	if !strings.Contains(s, "## Behavior") || !strings.Contains(s, "## Open Questions") {
		t.Errorf("surrounding sections lost:\n%s", s)
	}
	if !strings.Contains(buf.String(), "updated") {
		t.Errorf("no confirmation printed: %q", buf.String())
	}
}

func TestRehearseACs_WriteMissingReadme(t *testing.T) {
	dir := t.TempDir()
	writeACFixture(t, dir, "a", "# AC: a\n\n## Statement\n\nX.\n")
	if err := runRehearseACs(dir, true, &bytes.Buffer{}); err == nil {
		t.Fatal("want error when --write but README.md is absent")
	}
}

func TestRehearseACs_WriteUnwritableReadme(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	dir := t.TempDir()
	writeACFixture(t, dir, "a", "# AC: a\n\n## Statement\n\nX.\n")
	readmePath := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# demo\n"), 0o444); err != nil {
		t.Fatal(err)
	}
	if err := runRehearseACs(dir, true, &bytes.Buffer{}); err == nil {
		t.Fatal("want error when README.md is read-only")
	}
}

func TestInjectACSummary_AppendsWhenAbsent(t *testing.T) {
	got := injectACSummary("# Feature: demo\n\n## Behavior\n\nx.\n", "## Acceptance Criteria\n\n| AC |\n|----|\n")
	if !strings.Contains(got, "## Behavior") || !strings.HasSuffix(got, "|----|\n") {
		t.Errorf("append result:\n%s", got)
	}
}

func TestInjectACSummary_SectionAtEndOfFile(t *testing.T) {
	readme := "# Feature: demo\n\n## Acceptance Criteria\n\nold\n"
	got := injectACSummary(readme, "## Acceptance Criteria\n\nNEW\n")
	if strings.Contains(got, "old") || !strings.Contains(got, "NEW") {
		t.Errorf("end-of-file section not replaced:\n%s", got)
	}
}

func TestRehearseACsCommand_Execute(t *testing.T) {
	dir := t.TempDir()
	writeACFixture(t, dir, "a", "# AC: a\n\n## Statement\n\nX.\n")
	cmd := rehearseACsCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{dir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(buf.String(), "## Acceptance Criteria") {
		t.Errorf("command output missing summary: %s", buf.String())
	}
}
