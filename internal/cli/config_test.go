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

func runConfigCmd(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := configCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

// cfgTestRepo builds a repo with all three config layers plus a fake HOME.
func cfgTestRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeFileT(t, filepath.Join(home, ".specscore.yaml"), "studio:\n  theme: dark\n")
	writeFileT(t, filepath.Join(root, "specscore.yaml"), "studio:\n  theme: light\n  name: Acme\n")
	writeFileT(t, filepath.Join(root, "specscore.local.yaml"), "studio:\n  theme: solarized\n")
	if err := os.MkdirAll(filepath.Join(root, "spec"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeFileT(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestConfigShow_MergesLayers(t *testing.T) {
	root := cfgTestRepo(t)
	out, _, err := runConfigCmd(t, "show", "--project", root)
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	if !strings.Contains(out, "theme: solarized") {
		t.Errorf("output missing merged theme:\n%s", out)
	}
	if !strings.Contains(out, "name: Acme") {
		t.Errorf("output missing project name:\n%s", out)
	}
}

func TestConfigShow_Origin(t *testing.T) {
	root := cfgTestRepo(t)
	out, _, err := runConfigCmd(t, "show", "--origin", "--project", root)
	if err != nil {
		t.Fatalf("show --origin: %v", err)
	}
	if !strings.Contains(out, "studio.theme: solarized  # local") {
		t.Errorf("missing local origin line:\n%s", out)
	}
	if !strings.Contains(out, "studio.name: Acme  # project") {
		t.Errorf("missing project origin line:\n%s", out)
	}
}

func TestConfigGet_ReturnsValue(t *testing.T) {
	root := cfgTestRepo(t)
	out, _, err := runConfigCmd(t, "get", "studio.theme", "--project", root)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if strings.TrimSpace(out) != "solarized" {
		t.Errorf("get studio.theme = %q, want solarized", strings.TrimSpace(out))
	}
}

func TestConfigGet_Origin(t *testing.T) {
	root := cfgTestRepo(t)
	out, _, err := runConfigCmd(t, "get", "studio.theme", "--origin", "--project", root)
	if err != nil {
		t.Fatalf("get --origin: %v", err)
	}
	if strings.TrimSpace(out) != "solarized  # local" {
		t.Errorf("get --origin = %q, want 'solarized  # local'", strings.TrimSpace(out))
	}
}

func TestConfigGet_MapValueRendersYAML(t *testing.T) {
	root := cfgTestRepo(t)
	out, _, err := runConfigCmd(t, "get", "studio", "--project", root)
	if err != nil {
		t.Fatalf("get map: %v", err)
	}
	if !strings.Contains(out, "name: Acme") || !strings.Contains(out, "theme: solarized") {
		t.Errorf("map render missing keys:\n%s", out)
	}
}

func TestConfigGet_MissingKeyNotFound(t *testing.T) {
	root := cfgTestRepo(t)
	_, _, err := runConfigCmd(t, "get", "nope.nada", "--project", root)
	if err == nil {
		t.Fatal("expected error for missing key")
	}
	if exitCodeOf(err) != exitcode.NotFound {
		t.Errorf("exit code = %d, want NotFound(%d)", exitCodeOf(err), exitcode.NotFound)
	}
}

func TestConfigGet_NonLeafOriginOmitted(t *testing.T) {
	root := cfgTestRepo(t)
	// `studio` is a map; it has no leaf origin, so --origin prints no annotation.
	out, _, err := runConfigCmd(t, "get", "studio", "--origin", "--project", root)
	if err != nil {
		t.Fatalf("get map --origin: %v", err)
	}
	if strings.Contains(out, "#") {
		t.Errorf("non-leaf get --origin should not annotate origin:\n%s", out)
	}
}

func TestConfigGet_ResolveError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	_, _, err := runConfigCmd(t, "get", "studio.theme", "--project", dir)
	if err == nil {
		t.Fatal("expected resolve error when no specscore.yaml found")
	}
}

func TestConfigGet_DescendIntoScalarNotFound(t *testing.T) {
	root := cfgTestRepo(t)
	// studio.theme is a scalar; descending further must report not-found.
	_, _, err := runConfigCmd(t, "get", "studio.theme.deeper", "--project", root)
	if err == nil {
		t.Fatal("expected not-found descending into a scalar")
	}
	if exitCodeOf(err) != exitcode.NotFound {
		t.Errorf("exit code = %d, want NotFound(%d)", exitCodeOf(err), exitcode.NotFound)
	}
}

func TestConfigShow_NoSpecscoreYAML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	_, _, err := runConfigCmd(t, "show", "--project", dir)
	if err == nil {
		t.Fatal("expected error when no specscore.yaml found")
	}
	if exitCodeOf(err) != exitcode.NotFound {
		t.Errorf("exit code = %d, want NotFound(%d)", exitCodeOf(err), exitcode.NotFound)
	}
}

func TestConfigShow_ParseErrorSurfaces(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeFileT(t, filepath.Join(root, "specscore.yaml"), "studio: [unclosed\n")
	if err := os.MkdirAll(filepath.Join(root, "spec"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, _, err := runConfigCmd(t, "show", "--project", root)
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestConfigShow_CwdBranch(t *testing.T) {
	root := cfgTestRepo(t)
	orig := osGetwdFn
	osGetwdFn = func() (string, error) { return root, nil }
	defer func() { osGetwdFn = orig }()

	out, _, err := runConfigCmd(t, "show")
	if err != nil {
		t.Fatalf("show (cwd): %v", err)
	}
	if !strings.Contains(out, "theme: solarized") {
		t.Errorf("cwd-resolved show missing value:\n%s", out)
	}
}

func TestConfigShow_GetwdError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	orig := osGetwdFn
	osGetwdFn = func() (string, error) { return "", errors.New("boom") }
	defer func() { osGetwdFn = orig }()

	_, _, err := runConfigCmd(t, "show")
	if err == nil {
		t.Fatal("expected getwd error")
	}
}

func TestConfigShow_AbsError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	orig := filepathAbsFn
	filepathAbsFn = func(string) (string, error) { return "", errors.New("boom") }
	defer func() { filepathAbsFn = orig }()

	_, _, err := runConfigCmd(t, "show", "--project", "whatever")
	if err == nil {
		t.Fatal("expected abs error")
	}
}

func TestConfigShow_HomeDirError(t *testing.T) {
	root := t.TempDir()
	writeFileT(t, filepath.Join(root, "specscore.yaml"), "studio:\n  theme: x\n")
	if err := os.MkdirAll(filepath.Join(root, "spec"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", "") // makes os.UserHomeDir fail on unix

	_, _, err := runConfigCmd(t, "show", "--project", root)
	if err == nil {
		t.Fatal("expected home-dir error")
	}
}
