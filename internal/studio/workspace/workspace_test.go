package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeWorkspace writes a studio.yaml with the given content into dir and
// returns its path.
func writeWorkspace(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "studio.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// mkRepo creates a subdirectory of dir and returns its absolute path.
func mkRepo(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// exitCode extracts the exit code carried by err, failing the test when err
// is nil or carries none.
func exitCode(t *testing.T, err error) int {
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

// --- Load ---

func TestLoad_Valid(t *testing.T) {
	dir := t.TempDir()
	path := writeWorkspace(t, dir, "name: demo\nrepos:\n  - repo-a\n  - /abs/repo-b\n")

	ws, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ws.Name != "demo" {
		t.Errorf("Name = %q, want %q", ws.Name, "demo")
	}
	if len(ws.Repos) != 2 || ws.Repos[0].Path != "repo-a" || ws.Repos[1].Path != "/abs/repo-b" {
		t.Errorf("Repos = %v, want [repo-a /abs/repo-b]", ws.Repos)
	}
	if !filepath.IsAbs(ws.Path) || filepath.Base(ws.Path) != "studio.yaml" {
		t.Errorf("Path = %q, want absolute path to studio.yaml", ws.Path)
	}
	if ws.Dir != filepath.Dir(ws.Path) {
		t.Errorf("Dir = %q, want %q", ws.Dir, filepath.Dir(ws.Path))
	}
}

func TestLoad_RelativePathIsAbsolutized(t *testing.T) {
	dir := t.TempDir()
	writeWorkspace(t, dir, "name: demo\nrepos: [r]\n")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	ws, err := Load("./studio.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !filepath.IsAbs(ws.Path) {
		t.Errorf("Path = %q, want absolute", ws.Path)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "studio.yaml")

	_, err := Load(path)
	if code := exitCode(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q does not name the expected workspace path %q", err.Error(), path)
	}
	if strings.Contains(err.Error(), "\n") {
		t.Errorf("error is not one line: %q", err.Error())
	}
}

func TestLoad_UnparsableYAML(t *testing.T) {
	dir := t.TempDir()
	path := writeWorkspace(t, dir, "name: [unclosed\n")

	_, err := Load(path)
	if code := exitCode(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q does not name the workspace path", err.Error())
	}
}

func TestLoad_MissingName(t *testing.T) {
	dir := t.TempDir()
	path := writeWorkspace(t, dir, "repos: [a]\n")

	_, err := Load(path)
	if code := exitCode(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("error %q does not mention the missing name field", err.Error())
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q does not name the workspace path", err.Error())
	}
}

func TestLoad_EmptyRepos(t *testing.T) {
	dir := t.TempDir()
	writeWorkspace(t, dir, "name: demo\n")

	_, err := Load(filepath.Join(dir, "studio.yaml"))
	if code := exitCode(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(err.Error(), "repos") {
		t.Errorf("error %q does not mention the repos field", err.Error())
	}
}

func TestLoad_RepoEntryMapForm(t *testing.T) {
	dir := t.TempDir()
	path := writeWorkspace(t, dir, `name: demo
repos:
  - repo-a
  - path: repo-ops
    registries:
      - ops/domains.json
      - ops/ecosystem.yaml
`)

	ws, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(ws.Repos) != 2 {
		t.Fatalf("len(Repos) = %d, want 2", len(ws.Repos))
	}
	if ws.Repos[0].Path != "repo-a" || len(ws.Repos[0].Registries) != 0 {
		t.Errorf("Repos[0] = %+v, want bare path repo-a", ws.Repos[0])
	}
	if ws.Repos[1].Path != "repo-ops" {
		t.Errorf("Repos[1].Path = %q, want repo-ops", ws.Repos[1].Path)
	}
	if regs := ws.Repos[1].Registries; len(regs) != 2 || regs[0] != "ops/domains.json" || regs[1] != "ops/ecosystem.yaml" {
		t.Errorf("Repos[1].Registries = %v, want [ops/domains.json ops/ecosystem.yaml]", regs)
	}
}

func TestLoad_RepoEntryMapFormMissingPath(t *testing.T) {
	dir := t.TempDir()
	path := writeWorkspace(t, dir, "name: demo\nrepos:\n  - registries: [x.json]\n")

	_, err := Load(path)
	if code := exitCode(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(err.Error(), "path") {
		t.Errorf("error %q does not mention the missing path field", err.Error())
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q does not name the workspace path", err.Error())
	}
}

func TestLoad_RepoEntryBadShape(t *testing.T) {
	dir := t.TempDir()
	path := writeWorkspace(t, dir, "name: demo\nrepos:\n  - path: [not, a, scalar]\n")

	_, err := Load(path)
	if code := exitCode(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q does not name the workspace path", err.Error())
	}
}

func TestLoad_AbsError(t *testing.T) {
	old := filepathAbsFn
	filepathAbsFn = func(string) (string, error) { return "", errors.New("abs boom") }
	t.Cleanup(func() { filepathAbsFn = old })

	_, err := Load("studio.yaml")
	if code := exitCode(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestLoad_ReadError(t *testing.T) {
	old := osReadFileFn
	osReadFileFn = func(string) ([]byte, error) { return nil, errors.New("read boom") }
	t.Cleanup(func() { osReadFileFn = old })

	_, err := Load("studio.yaml")
	if code := exitCode(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(err.Error(), "read boom") {
		t.Errorf("error %q does not carry the underlying cause", err.Error())
	}
}

// --- ResolveRepos ---

func TestResolveRepos_RelativeAndAbsolute(t *testing.T) {
	dir := t.TempDir()
	relRepo := mkRepo(t, dir, "repo-rel")
	absRepo := mkRepo(t, t.TempDir(), "repo-abs")
	path := writeWorkspace(t, dir, "name: demo\nrepos:\n  - repo-rel\n  - "+absRepo+"\n")

	ws, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	repos, err := ws.ResolveRepos()
	if err != nil {
		t.Fatalf("ResolveRepos: %v", err)
	}
	if len(repos) != 2 || repos[0] != relRepo || repos[1] != absRepo {
		t.Errorf("repos = %v, want [%s %s]", repos, relRepo, absRepo)
	}
}

func TestResolveRepos_Glob(t *testing.T) {
	dir := t.TempDir()
	a := mkRepo(t, dir, "repo-a")
	b := mkRepo(t, dir, "repo-b")
	// A plain file matching the glob must be skipped.
	if err := os.WriteFile(filepath.Join(dir, "repo-file"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := writeWorkspace(t, dir, "name: demo\nrepos: ['repo-*']\n")

	ws, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	repos, err := ws.ResolveRepos()
	if err != nil {
		t.Fatalf("ResolveRepos: %v", err)
	}
	if len(repos) != 2 || repos[0] != a || repos[1] != b {
		t.Errorf("repos = %v, want [%s %s]", repos, a, b)
	}
}

func TestResolveRepos_SkipsMissingAndDedupes(t *testing.T) {
	dir := t.TempDir()
	a := mkRepo(t, dir, "repo-a")
	path := writeWorkspace(t, dir, "name: demo\nrepos:\n  - repo-a\n  - repo-a\n  - 'repo-*'\n  - no-such-dir\n")

	ws, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	repos, err := ws.ResolveRepos()
	if err != nil {
		t.Fatalf("ResolveRepos: %v", err)
	}
	if len(repos) != 1 || repos[0] != a {
		t.Errorf("repos = %v, want exactly [%s]", repos, a)
	}
}

func TestResolveRepos_ZeroResolving(t *testing.T) {
	dir := t.TempDir()
	path := writeWorkspace(t, dir, "name: demo\nrepos: [no-such-dir, 'nope-*']\n")

	ws, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	_, err = ws.ResolveRepos()
	if code := exitCode(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q does not name the workspace path", err.Error())
	}
	if strings.Contains(err.Error(), "\n") {
		t.Errorf("error is not one line: %q", err.Error())
	}
}

func TestResolveRepos_BadGlob(t *testing.T) {
	dir := t.TempDir()
	path := writeWorkspace(t, dir, "name: demo\nrepos: ['[']\n")

	ws, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	_, err = ws.ResolveRepos()
	if code := exitCode(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(err.Error(), "[") {
		t.Errorf("error %q does not name the bad pattern", err.Error())
	}
}

// --- ResolveRegistries ---

func TestResolveRegistries_MapsResolvedReposToConfiguredFiles(t *testing.T) {
	dir := t.TempDir()
	ops := mkRepo(t, dir, "repo-ops")
	mkRepo(t, dir, "repo-plain")
	path := writeWorkspace(t, dir, `name: demo
repos:
  - repo-plain
  - path: repo-ops
    registries: [ops/domains.json]
  - path: repo-ops
    registries: [ops/domains.json, extra/ecosystem.yaml]
`)

	ws, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	regs := ws.ResolveRegistries()
	if len(regs) != 1 {
		t.Fatalf("ResolveRegistries = %v, want exactly one repo entry", regs)
	}
	got := regs[ops]
	// Duplicate entries merge; per-repo file lists dedupe, order preserved.
	if len(got) != 2 || got[0] != "ops/domains.json" || got[1] != "extra/ecosystem.yaml" {
		t.Errorf("registries[%s] = %v, want [ops/domains.json extra/ecosystem.yaml]", ops, got)
	}
}

func TestResolveRegistries_GlobEntryAppliesToAllMatches(t *testing.T) {
	dir := t.TempDir()
	a := mkRepo(t, dir, "repo-a")
	b := mkRepo(t, dir, "repo-b")
	// A plain file matching the glob must be skipped, like ResolveRepos does.
	if err := os.WriteFile(filepath.Join(dir, "repo-file"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := writeWorkspace(t, dir, "name: demo\nrepos:\n  - path: 'repo-*'\n    registries: [reg.json]\n")

	ws, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	regs := ws.ResolveRegistries()
	if len(regs) != 2 {
		t.Fatalf("ResolveRegistries = %v, want entries for both matches", regs)
	}
	for _, repo := range []string{a, b} {
		if got := regs[repo]; len(got) != 1 || got[0] != "reg.json" {
			t.Errorf("registries[%s] = %v, want [reg.json]", repo, got)
		}
	}
}

func TestResolveRegistries_SkipsBadGlobAndMissingDirs(t *testing.T) {
	dir := t.TempDir()
	path := writeWorkspace(t, dir, `name: demo
repos:
  - path: '['
    registries: [reg.json]
  - path: no-such-dir
    registries: [reg.json]
`)

	ws, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Bad globs are ResolveRepos's error to report; registries resolution
	// skips them silently, as it does entries naming no existing directory.
	if regs := ws.ResolveRegistries(); len(regs) != 0 {
		t.Errorf("ResolveRegistries = %v, want empty", regs)
	}
}

// --- DefaultDBPath ---

func TestDefaultDBPath(t *testing.T) {
	dir := t.TempDir()
	path := writeWorkspace(t, dir, "name: demo\nrepos: [a]\n")

	ws, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := filepath.Join(ws.Dir, ".specscore-studio", "facts.db")
	if got := ws.DefaultDBPath(); got != want {
		t.Errorf("DefaultDBPath = %q, want %q", got, want)
	}
}
