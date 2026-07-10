package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// runStudioCmd executes the studio command group with args against a fresh
// command tree, capturing stdout/stderr.
func runStudioCmd(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := studioCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func studioExit(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	var ec interface{ ExitCode() int }
	if !errors.As(err, &ec) {
		t.Fatalf("error carries no exit code: %v", err)
	}
	return ec.ExitCode()
}

// newStudioWorkspace writes a studio.yaml plus the given repo directories
// into a temp dir and returns the workspace-file path.
func newStudioWorkspace(t *testing.T, repos ...string) string {
	t.Helper()
	dir := t.TempDir()
	var b strings.Builder
	b.WriteString("name: demo\nrepos:\n")
	for _, r := range repos {
		if err := os.MkdirAll(filepath.Join(dir, r), 0o755); err != nil {
			t.Fatal(err)
		}
		b.WriteString("  - " + r + "\n")
	}
	path := filepath.Join(dir, "studio.yaml")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// --- studio (group) ---

func TestStudio_HelpExitsZero(t *testing.T) {
	out, _, err := runStudioCmd(t)
	if err != nil {
		t.Fatalf("studio with no subcommand: %v", err)
	}
	if !strings.Contains(out, "index") {
		t.Errorf("help output does not mention the index verb: %q", out)
	}
}

// --- studio index ---

func TestStudioIndex_Summary(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a", "repo-b")
	wsDir := filepath.Dir(wsPath)

	out, _, err := runStudioCmd(t, "index", "--workspace", wsPath)
	if err != nil {
		t.Fatalf("studio index: %v", err)
	}
	for _, want := range []string{
		"Ecosystem: demo",
		"Workspace: " + wsPath,
		"Repos: 2",
		filepath.Join(wsDir, "repo-a"),
		filepath.Join(wsDir, "repo-b"),
		"Fact store: " + filepath.Join(wsDir, ".specscore-studio", "facts.db"),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q; got:\n%s", want, out)
		}
	}
}

func TestStudioIndex_DBFlagOverridesDefault(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	dbPath := filepath.Join(t.TempDir(), "custom.db")

	out, _, err := runStudioCmd(t, "index", "--workspace", wsPath, "--db", dbPath)
	if err != nil {
		t.Fatalf("studio index: %v", err)
	}
	if !strings.Contains(out, "Fact store: "+dbPath) {
		t.Errorf("summary does not use --db path %q; got:\n%s", dbPath, out)
	}
}

func TestStudioIndex_RelativeDBFlagIsAbsolutized(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")

	out, _, err := runStudioCmd(t, "index", "--workspace", wsPath, "--db", "rel.db")
	if err != nil {
		t.Fatalf("studio index: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Fact store: "+filepath.Join(cwd, "rel.db")) {
		t.Errorf("summary does not absolutize relative --db; got:\n%s", out)
	}
}

func TestStudioIndex_DBFlagAbsError(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")

	old := filepathAbsFn
	filepathAbsFn = func(p string) (string, error) {
		if p == "rel.db" {
			return "", errors.New("abs boom")
		}
		return filepath.Abs(p)
	}
	t.Cleanup(func() { filepathAbsFn = old })

	_, _, err := runStudioCmd(t, "index", "--workspace", wsPath, "--db", "rel.db")
	if code := studioExit(t, err); code != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d", code, exitcode.InvalidArgs)
	}
}

// AC: workspace-missing-error — index without a workspace file exits 2 and
// prints a one-line error naming the expected workspace path.
func TestStudioIndex_MissingWorkspaceExits2(t *testing.T) {
	dir := t.TempDir() // no studio.yaml
	wsPath := filepath.Join(dir, "studio.yaml")

	_, _, err := runStudioCmd(t, "index", "--workspace", wsPath)
	if code := studioExit(t, err); code != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d", code, exitcode.InvalidArgs)
	}
	if !strings.Contains(err.Error(), wsPath) {
		t.Errorf("error %q does not name the expected workspace path %q", err.Error(), wsPath)
	}
	if strings.Contains(err.Error(), "\n") {
		t.Errorf("error is not one line: %q", err.Error())
	}
}

func TestStudioIndex_UnparsableWorkspaceExits2(t *testing.T) {
	dir := t.TempDir()
	wsPath := filepath.Join(dir, "studio.yaml")
	if err := os.WriteFile(wsPath, []byte("name: [unclosed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := runStudioCmd(t, "index", "--workspace", wsPath)
	if code := studioExit(t, err); code != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d", code, exitcode.InvalidArgs)
	}
}

func TestStudioIndex_ZeroResolvingWorkspaceExits2(t *testing.T) {
	dir := t.TempDir()
	wsPath := filepath.Join(dir, "studio.yaml")
	if err := os.WriteFile(wsPath, []byte("name: demo\nrepos: [no-such-dir]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := runStudioCmd(t, "index", "--workspace", wsPath)
	if code := studioExit(t, err); code != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d", code, exitcode.InvalidArgs)
	}
	if !strings.Contains(err.Error(), wsPath) {
		t.Errorf("error %q does not name the workspace path", err.Error())
	}
}

func TestStudioIndex_DefaultWorkspaceIsCWDStudioYAML(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir()) // macOS /var → /private/var
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	_, _, err = runStudioCmd(t, "index")
	if code := studioExit(t, err); code != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d", code, exitcode.InvalidArgs)
	}
	if !strings.Contains(err.Error(), filepath.Join(dir, "studio.yaml")) {
		t.Errorf("error %q does not name the default workspace path", err.Error())
	}
}

// --- root registration ---

func TestRootCommand_HasStudio(t *testing.T) {
	root := newRootCommand()
	for _, c := range root.Commands() {
		if c.Name() == "studio" {
			return
		}
	}
	t.Error("root command does not register the studio command group")
}
