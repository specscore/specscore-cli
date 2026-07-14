package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/specscore/specscore-cli/internal/studio/adapters"
	"github.com/specscore/specscore-cli/internal/studio/fact"
	"github.com/specscore/specscore-cli/internal/studio/ingr"
	"github.com/specscore/specscore-cli/internal/studio/repoid"
	"github.com/specscore/specscore-cli/internal/studio/store"
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
	t.Chdir(t.TempDir()) // the rebuilt store lands in the cwd

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
	if _, err := os.Stat(filepath.Join(cwd, "rel.db")); err != nil {
		t.Errorf("rebuilt store missing at --db path: %v", err)
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

// newSpecScoreRepo writes a minimal SpecScore-managed repo (specscore.yaml +
// spec/features/x/README.md with **Status:** Approved) into dir/name.
func newSpecScoreRepo(t *testing.T, dir, name string) {
	t.Helper()
	featureDir := filepath.Join(dir, name, "spec", "features", "x")
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := "project:\n  title: Fixture Repo\n"
	if err := os.WriteFile(filepath.Join(dir, name, "specscore.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	readme := "# Feature: X\n\n**Status:** Approved\n"
	if err := os.WriteFile(filepath.Join(featureDir, "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatal(err)
	}
}

// AC: spec-status-fact — indexing a repo whose spec/features/x/README.md
// declares **Status:** Approved yields a has-status fact with the full fact
// shape, observable via `studio facts --predicate has-status --format json`.
func TestStudioIndex_SpecScoreStatusFactEndToEnd(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	wsDir := filepath.Dir(wsPath)
	newSpecScoreRepo(t, wsDir, "repo-a")

	out, _, err := runStudioCmd(t, "index", "--workspace", wsPath)
	if err != nil {
		t.Fatalf("studio index: %v", err)
	}
	if !strings.Contains(out, "Facts by adapter:") || !strings.Contains(out, "specscore: 1") {
		t.Errorf("summary missing specscore fact count; got:\n%s", out)
	}

	jsonOut, _, err := runStudioCmd(t, "facts", "--workspace", wsPath,
		"--predicate", "has-status", "--format", "json")
	if err != nil {
		t.Fatalf("studio facts: %v", err)
	}
	var facts []fact.Fact
	if err := json.Unmarshal([]byte(jsonOut), &facts); err != nil {
		t.Fatalf("facts output is not JSON: %v\n%s", err, jsonOut)
	}
	if len(facts) != 1 {
		t.Fatalf("got %d has-status facts, want 1:\n%s", len(facts), jsonOut)
	}
	f := facts[0]
	if !strings.HasSuffix(f.Subject, "#x") {
		t.Errorf("Subject = %q, want suffix #x", f.Subject)
	}
	if f.Object != "Approved" {
		t.Errorf("Object = %q, want Approved", f.Object)
	}
	if f.Class != fact.Declared {
		t.Errorf("Class = %q, want declared", f.Class)
	}
	if f.Pointer != "spec/features/x/README.md" {
		t.Errorf("Pointer = %q, want spec/features/x/README.md", f.Pointer)
	}
	if f.Adapter.ID != "specscore" || f.Adapter.Version == "" {
		t.Errorf("Adapter = %+v, want id specscore with a non-empty version", f.Adapter)
	}
	if f.ObservedAt == "" {
		t.Error("ObservedAt is empty")
	}
	if f.Ecosystem != "demo" {
		t.Errorf("Ecosystem = %q, want demo", f.Ecosystem)
	}
}

// REQ: partial-tolerance (minimal collect + print) — the run summary lists
// every collected warning.
func TestStudioIndex_SummaryListsWarnings(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")

	old := adaptersRunFn
	adaptersRunFn = func(_ []adapters.Adapter, _ []string, _ string) adapters.Result {
		return adapters.Result{
			Warnings:       []adapters.Warning{{Repo: "repo-a", Adapter: "specscore", Message: "file boom"}},
			FactsByAdapter: map[string]int{"specscore": 0},
		}
	}
	t.Cleanup(func() { adaptersRunFn = old })

	out, _, err := runStudioCmd(t, "index", "--workspace", wsPath)
	if err != nil {
		t.Fatalf("studio index: %v", err)
	}
	for _, want := range []string{"Warnings: 1", "repo-a [specscore]: file boom"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q; got:\n%s", want, out)
		}
	}
}

// newBrokenStudioWorkspace writes a studio.yaml listing one healthy
// SpecScore fixture repo plus one path that does not exist, returning the
// workspace path and the missing repo path.
func newBrokenStudioWorkspace(t *testing.T) (wsPath, missingPath string) {
	t.Helper()
	wsPath = newStudioWorkspace(t, "repo-a")
	wsDir := filepath.Dir(wsPath)
	newSpecScoreRepo(t, wsDir, "repo-a")
	missingPath = filepath.Join(wsDir, "no-such-repo")
	content := "name: demo\nrepos:\n  - repo-a\n  - no-such-repo\n"
	if err := os.WriteFile(wsPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return wsPath, missingPath
}

// AC: partial-tolerance-warns — one broken repo does not abort the run: the
// command exits 0, the summary lists a warning for the missing path, and
// facts from the healthy repo are queryable.
func TestStudioIndex_MissingRepoPathWarnsAndExitsZero(t *testing.T) {
	wsPath, missingPath := newBrokenStudioWorkspace(t)

	out, _, err := runStudioCmd(t, "index", "--workspace", wsPath)
	if err != nil {
		t.Fatalf("studio index with one missing repo: %v", err)
	}
	if !strings.Contains(out, "Warnings: 1") {
		t.Errorf("summary missing \"Warnings: 1\"; got:\n%s", out)
	}
	if !strings.Contains(out, missingPath) {
		t.Errorf("summary does not name the missing path %q; got:\n%s", missingPath, out)
	}
	// Per-repo summary lines: healthy repo with facts, missing repo with the
	// warning (REQ: partial-tolerance).
	wsDir := filepath.Dir(wsPath)
	for _, want := range []string{
		filepath.Join(wsDir, "repo-a") + ": 1 facts, 0 warnings",
		missingPath + ": 0 facts, 1 warnings",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing per-repo line %q; got:\n%s", want, out)
		}
	}

	// Facts from the healthy repo are queryable.
	countOut, _, err := runStudioCmd(t, "facts", "--workspace", wsPath,
		"--predicate", "has-status", "--count")
	if err != nil {
		t.Fatalf("studio facts: %v", err)
	}
	if countOut != "1\n" {
		t.Errorf("healthy-repo fact count = %q, want \"1\\n\"", countOut)
	}
}

// AC: strict-mode-fails — --strict escalates warnings: the command exits 3
// and the warning is still printed.
func TestStudioIndex_StrictEscalatesWarningsToExit3(t *testing.T) {
	wsPath, missingPath := newBrokenStudioWorkspace(t)

	out, _, err := runStudioCmd(t, "index", "--workspace", wsPath, "--strict")
	if code := studioExit(t, err); code != exitcode.NotFound {
		t.Errorf("exit code = %d, want 3", code)
	}
	if !strings.Contains(out, missingPath) {
		t.Errorf("warning for %q not printed under --strict; got:\n%s", missingPath, out)
	}
	// The run completes before escalating: the full summary is printed and
	// the store is rebuilt.
	if !strings.Contains(out, "Fact store: ") {
		t.Errorf("summary not completed under --strict; got:\n%s", out)
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(wsPath), ".specscore-studio", "facts.db")); statErr != nil {
		t.Errorf("fact store not rebuilt under --strict: %v", statErr)
	}
	if !strings.Contains(err.Error(), "warning") {
		t.Errorf("error %q does not mention warnings", err.Error())
	}
}

func TestStudioIndex_StrictWithoutWarningsExitsZero(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")

	_, _, err := runStudioCmd(t, "index", "--workspace", wsPath, "--strict")
	if err != nil {
		t.Fatalf("studio index --strict with no warnings: %v", err)
	}
}

// REQ: rebuild-only — a store-write failure surfaces as an error (exit 1)
// instead of a summary.
func TestStudioIndex_RebuildFailurePropagates(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	// The db parent "directory" is a regular file → MkdirAll fails.
	_, _, err := runStudioCmd(t, "index", "--workspace", wsPath,
		"--db", filepath.Join(blocker, "facts.db"))
	if err == nil {
		t.Fatal("expected a store-rebuild error")
	}
	if !strings.Contains(err.Error(), "fact-store directory") {
		t.Errorf("error %q does not mention the fact-store directory", err.Error())
	}
}

// --- studio index: INGR export (REQ: ingr-export) ---

// ingrRecordCount reads the recordset at path and returns its `# N records`
// trailer count.
func ingrRecordCount(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading recordset %s: %v", path, err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	var n int
	if _, err := fmt.Sscanf(lines[len(lines)-1], "# %d records", &n); err != nil {
		t.Fatalf("recordset %s has no record-count trailer: %q", path, lines[len(lines)-1])
	}
	return n
}

// AC: ingr-export-counts (default path) — every index run writes one
// recordset per repo slug under <workspace-dir>/.specscore-studio/ingr/ and
// the per-repo record count equals the repo's fact count in the summary.
func TestStudioIndex_IngrExportDefaultDirAndCounts(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a", "repo-b")
	wsDir := filepath.Dir(wsPath)
	newSpecScoreRepo(t, wsDir, "repo-a") // one has-status fact; repo-b stays empty

	out, _, err := runStudioCmd(t, "index", "--workspace", wsPath)
	if err != nil {
		t.Fatalf("studio index: %v", err)
	}
	ingrDir := filepath.Join(wsDir, ".specscore-studio", "ingr")
	if !strings.Contains(out, "INGR export: "+ingrDir) {
		t.Errorf("summary does not name the INGR export dir %q; got:\n%s", ingrDir, out)
	}
	for name, want := range map[string]int{"repo-a": 1, "repo-b": 0} {
		slug := repoid.LocalID(filepath.Join(wsDir, name))
		got := ingrRecordCount(t, filepath.Join(ingrDir, slug, "facts.ingr"))
		if got != want {
			t.Errorf("%s record count = %d, want %d (the summary fact count)", slug, got, want)
		}
		if !strings.Contains(out, fmt.Sprintf("%s: %d facts", filepath.Join(wsDir, name), want)) {
			t.Errorf("summary lacks %q fact count %d; got:\n%s", name, want, out)
		}
	}
}

func TestStudioIndex_IngrDirFlagOverridesDefault(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	ingrDir := filepath.Join(t.TempDir(), "custom-ingr")

	out, _, err := runStudioCmd(t, "index", "--workspace", wsPath, "--ingr-dir", ingrDir)
	if err != nil {
		t.Fatalf("studio index: %v", err)
	}
	if !strings.Contains(out, "INGR export: "+ingrDir) {
		t.Errorf("summary does not use --ingr-dir path %q; got:\n%s", ingrDir, out)
	}
	repoID := repoid.LocalID(filepath.Join(filepath.Dir(wsPath), "repo-a"))
	if _, err := os.Stat(filepath.Join(ingrDir, repoID, "facts.ingr")); err != nil {
		t.Errorf("recordset missing under --ingr-dir: %v", err)
	}
}

func TestStudioIndex_RelativeIngrDirFlagIsAbsolutized(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	t.Chdir(t.TempDir()) // the export root lands in the cwd

	out, _, err := runStudioCmd(t, "index", "--workspace", wsPath, "--ingr-dir", "rel-ingr")
	if err != nil {
		t.Fatalf("studio index: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(cwd, "rel-ingr")
	if !strings.Contains(out, "INGR export: "+abs) {
		t.Errorf("summary does not absolutize --ingr-dir to %q; got:\n%s", abs, out)
	}
}

func TestStudioIndex_IngrDirFlagAbsError(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")

	old := filepathAbsFn
	filepathAbsFn = func(p string) (string, error) {
		if p == "rel-ingr" {
			return "", errors.New("abs boom")
		}
		return filepath.Abs(p)
	}
	t.Cleanup(func() { filepathAbsFn = old })

	_, _, err := runStudioCmd(t, "index", "--workspace", wsPath, "--ingr-dir", "rel-ingr")
	if code := studioExit(t, err); code != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d", code, exitcode.InvalidArgs)
	}
}

func TestStudioIndex_NoIngrSkipsExport(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	wsDir := filepath.Dir(wsPath)

	out, _, err := runStudioCmd(t, "index", "--workspace", wsPath, "--no-ingr")
	if err != nil {
		t.Fatalf("studio index: %v", err)
	}
	if !strings.Contains(out, "INGR export: disabled (--no-ingr)") {
		t.Errorf("summary does not report the disabled export; got:\n%s", out)
	}
	ingrDir := filepath.Join(wsDir, ".specscore-studio", "ingr")
	if _, err := os.Stat(ingrDir); !os.IsNotExist(err) {
		t.Errorf("INGR export dir written despite --no-ingr: %v", err)
	}
}

// An export failure is a run-level warning, not a fatal error — the store
// stays the query surface (REQ: ingr-export).
func TestStudioIndex_IngrExportFailureIsWarning(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")

	old := ingrExportFn
	ingrExportFn = func(string, []ingr.Repo) error { return errors.New("export boom") }
	t.Cleanup(func() { ingrExportFn = old })

	out, _, err := runStudioCmd(t, "index", "--workspace", wsPath)
	if err != nil {
		t.Fatalf("studio index: %v", err)
	}
	if !strings.Contains(out, "Warnings: 1") {
		t.Errorf("summary does not count the export warning; got:\n%s", out)
	}
	if !strings.Contains(out, "INGR export failed: export boom — facts remain queryable in the store") {
		t.Errorf("summary does not list the export warning; got:\n%s", out)
	}
}

// --- studio facts ---

// seedStudioStore rebuilds the workspace's default fact store with two
// sample facts and returns its path.
func seedStudioStore(t *testing.T, wsPath string) string {
	t.Helper()
	dbPath := filepath.Join(filepath.Dir(wsPath), ".specscore-studio", "facts.db")
	facts := []fact.Fact{
		{
			Subject:    "pkg:a",
			Predicate:  "imports",
			Object:     "pkg:b",
			Evidence:   fact.Evidence{Class: fact.Derived, Pointer: "codegraph/pkg.json"},
			Adapter:    fact.Adapter{ID: "codegraph", Version: "1"},
			ObservedAt: "2026-07-10T00:00:00Z",
			Ecosystem:  "demo",
		},
		{
			Subject:    "repo#x",
			Predicate:  "has-status",
			Object:     "Approved",
			Evidence:   fact.Evidence{Class: fact.Declared, Pointer: "spec/features/x/README.md:9"},
			Adapter:    fact.Adapter{ID: "specscore", Version: "1"},
			ObservedAt: "2026-07-10T00:00:00Z",
			Ecosystem:  "demo",
		},
	}
	if err := store.Rebuild(dbPath, facts); err != nil {
		t.Fatal(err)
	}
	return dbPath
}

// AC: missing-store-error — facts in a workspace where `studio index` has
// never run exits 2 with a message naming the expected store path and
// suggesting `studio index`.
func TestStudioFacts_MissingStoreExits2(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a") // no .specscore-studio/facts.db
	wantDB := filepath.Join(filepath.Dir(wsPath), ".specscore-studio", "facts.db")

	_, _, err := runStudioCmd(t, "facts", "--workspace", wsPath, "--predicate", "imports")
	if code := studioExit(t, err); code != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d", code, exitcode.InvalidArgs)
	}
	if !strings.Contains(err.Error(), wantDB) {
		t.Errorf("error %q does not name the expected store path %q", err.Error(), wantDB)
	}
	if !strings.Contains(err.Error(), "studio index") {
		t.Errorf("error %q does not suggest `studio index`", err.Error())
	}
}

func TestStudioFacts_MissingWorkspaceExits2(t *testing.T) {
	wsPath := filepath.Join(t.TempDir(), "studio.yaml") // no workspace file

	_, _, err := runStudioCmd(t, "facts", "--workspace", wsPath)
	if code := studioExit(t, err); code != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d", code, exitcode.InvalidArgs)
	}
	if !strings.Contains(err.Error(), wsPath) {
		t.Errorf("error %q does not name the workspace path", err.Error())
	}
}

func TestStudioFacts_TableOutput(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedStudioStore(t, wsPath)

	out, _, err := runStudioCmd(t, "facts", "--workspace", wsPath, "--predicate", "imports")
	if err != nil {
		t.Fatalf("studio facts: %v", err)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("table has %d lines, want header + 1 row:\n%s", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "SUBJECT") || !strings.Contains(lines[0], "ADAPTER") {
		t.Errorf("missing table header: %q", lines[0])
	}
	for _, want := range []string{"pkg:a", "imports", "pkg:b", "derived", "codegraph"} {
		if !strings.Contains(lines[1], want) {
			t.Errorf("row %q missing %q", lines[1], want)
		}
	}
}

func TestStudioFacts_JSONOutputFullShape(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedStudioStore(t, wsPath)

	out, _, err := runStudioCmd(t, "facts", "--workspace", wsPath,
		"--predicate", "has-status", "--format", "json")
	if err != nil {
		t.Fatalf("studio facts: %v", err)
	}
	var facts []fact.Fact
	if err := json.Unmarshal([]byte(out), &facts); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if len(facts) != 1 {
		t.Fatalf("got %d facts, want 1", len(facts))
	}
	f := facts[0]
	if f.Subject != "repo#x" || f.Object != "Approved" || f.Class != fact.Declared ||
		f.Pointer != "spec/features/x/README.md:9" || f.Adapter.ID != "specscore" ||
		f.Adapter.Version != "1" || f.ObservedAt == "" || f.Ecosystem != "demo" {
		t.Errorf("JSON fact missing full fact shape: %+v", f)
	}
	// The raw JSON uses the fact-shape field names.
	for _, key := range []string{`"evidence_class"`, `"evidence_pointer"`, `"adapter"`, `"observed_at"`, `"ecosystem"`} {
		if !strings.Contains(out, key) {
			t.Errorf("JSON output missing %s:\n%s", key, out)
		}
	}
}

func TestStudioFacts_Count(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedStudioStore(t, wsPath)

	out, _, err := runStudioCmd(t, "facts", "--workspace", wsPath, "--count")
	if err != nil {
		t.Fatalf("studio facts: %v", err)
	}
	if out != "2\n" {
		t.Errorf("count output = %q, want \"2\\n\"", out)
	}
}

func TestStudioFacts_CountZeroMatchesExitsZero(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedStudioStore(t, wsPath)

	out, _, err := runStudioCmd(t, "facts", "--workspace", wsPath,
		"--subject", "gone*", "--count")
	if err != nil {
		t.Fatalf("studio facts with zero matches: %v", err)
	}
	if out != "0\n" {
		t.Errorf("count output = %q, want \"0\\n\"", out)
	}
}

func TestStudioFacts_SubjectPrefixFilter(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedStudioStore(t, wsPath)

	out, _, err := runStudioCmd(t, "facts", "--workspace", wsPath,
		"--subject", "pkg:*", "--count")
	if err != nil {
		t.Fatalf("studio facts: %v", err)
	}
	if out != "1\n" {
		t.Errorf("prefix-filtered count = %q, want \"1\\n\"", out)
	}
}

func TestStudioFacts_DBFlagBypassesWorkspace(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	dbPath := seedStudioStore(t, wsPath)

	// No workspace file at the default ./studio.yaml is needed with --db.
	out, _, err := runStudioCmd(t, "facts", "--db", dbPath, "--count")
	if err != nil {
		t.Fatalf("studio facts --db: %v", err)
	}
	if out != "2\n" {
		t.Errorf("count output = %q, want \"2\\n\"", out)
	}
}

func TestStudioFacts_DBFlagAbsError(t *testing.T) {
	old := filepathAbsFn
	filepathAbsFn = func(string) (string, error) { return "", errors.New("abs boom") }
	t.Cleanup(func() { filepathAbsFn = old })

	_, _, err := runStudioCmd(t, "facts", "--db", "rel.db")
	if code := studioExit(t, err); code != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d", code, exitcode.InvalidArgs)
	}
}

func TestStudioFacts_BadFormatExits2(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedStudioStore(t, wsPath)

	_, _, err := runStudioCmd(t, "facts", "--workspace", wsPath, "--format", "yaml")
	if code := studioExit(t, err); code != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d", code, exitcode.InvalidArgs)
	}
	if !strings.Contains(err.Error(), "yaml") {
		t.Errorf("error %q does not name the bad format", err.Error())
	}
}

// --- studio facts: --stale filter and VERIFIED age column ---

// seedStaleStore rebuilds the workspace store with two verified-behavior
// facts: one verified 48h ago and one verified 1h ago, relative to now.
func seedStaleStore(t *testing.T, wsPath string, now time.Time) string {
	t.Helper()
	dbPath := filepath.Join(filepath.Dir(wsPath), ".specscore-studio", "facts.db")
	mk := func(subject string, ago time.Duration) fact.Fact {
		ts := now.Add(-ago).UTC().Format(time.RFC3339)
		return fact.Fact{
			Subject:    subject,
			Predicate:  "serves-status",
			Object:     "200",
			Evidence:   fact.Evidence{Class: fact.VerifiedBehavior, Pointer: "https://" + subject + "/"},
			Adapter:    fact.Adapter{ID: "probe-domain", Version: "1"},
			ObservedAt: ts,
			VerifiedAt: ts,
			Ecosystem:  "demo",
		}
	}
	facts := []fact.Fact{mk("old.app", 48*time.Hour), mk("fresh.app", 1*time.Hour)}
	if err := store.Rebuild(dbPath, facts); err != nil {
		t.Fatal(err)
	}
	return dbPath
}

// AC: stale-filter-selects-old-facts — --stale 24h selects only the fact
// verified more than 24h ago.
func TestStudioFacts_StaleSelectsOldFacts(t *testing.T) {
	now := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	old := studioNowFn
	studioNowFn = func() time.Time { return now }
	t.Cleanup(func() { studioNowFn = old })

	wsPath := newStudioWorkspace(t, "repo-a")
	seedStaleStore(t, wsPath, now)

	out, _, err := runStudioCmd(t, "facts", "--workspace", wsPath,
		"--class", "verified-behavior", "--stale", "24h", "--count")
	if err != nil {
		t.Fatalf("studio facts --stale: %v", err)
	}
	if out != "1\n" {
		t.Errorf("stale count = %q, want \"1\\n\" (only the 48h-old fact)", out)
	}
}

// AC: stale-filter-malformed-duration — a bad --stale duration exits 2 and
// names the invalid value.
func TestStudioFacts_StaleMalformedExits2(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedStudioStore(t, wsPath)

	_, _, err := runStudioCmd(t, "facts", "--workspace", wsPath, "--stale", "notaduration")
	if code := studioExit(t, err); code != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d", code, exitcode.InvalidArgs)
	}
	if !strings.Contains(err.Error(), "notaduration") {
		t.Errorf("error %q does not name the invalid duration", err.Error())
	}
}

// AC: age-column-rendered — the table has a VERIFIED column and a fact
// verified 3h ago renders "3h".
func TestStudioFacts_AgeColumnRendered(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	old := studioNowFn
	studioNowFn = func() time.Time { return now }
	t.Cleanup(func() { studioNowFn = old })

	wsPath := newStudioWorkspace(t, "repo-a")
	dbPath := filepath.Join(filepath.Dir(wsPath), ".specscore-studio", "facts.db")
	ts := now.Add(-3 * time.Hour).UTC().Format(time.RFC3339)
	f := fact.Fact{
		Subject: "example.app", Predicate: "serves-status", Object: "200",
		Evidence:   fact.Evidence{Class: fact.VerifiedBehavior, Pointer: "https://example.app/"},
		Adapter:    fact.Adapter{ID: "probe-domain", Version: "1"},
		ObservedAt: ts, VerifiedAt: ts, Ecosystem: "demo",
	}
	if err := store.Rebuild(dbPath, []fact.Fact{f}); err != nil {
		t.Fatal(err)
	}

	out, _, err := runStudioCmd(t, "facts", "--workspace", wsPath, "--class", "verified-behavior")
	if err != nil {
		t.Fatalf("studio facts: %v", err)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("table has %d lines, want header + 1 row:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[0], "VERIFIED") {
		t.Errorf("header %q missing VERIFIED column", lines[0])
	}
	if !strings.Contains(lines[1], "3h") {
		t.Errorf("row %q missing the 3h age", lines[1])
	}
}

// humanAge renders each freshness band and degrades gracefully on bad input
// (REQ: age-rendering).
func TestHumanAge(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	rfc := func(d time.Duration) string { return now.Add(-d).UTC().Format(time.RFC3339) }
	tests := []struct {
		name     string
		verified string
		want     string
	}{
		{"fresh hours", rfc(3 * time.Hour), "3h"},
		{"aging days", rfc(12 * 24 * time.Hour), "12d"},
		{"stale", rfc(40 * 24 * time.Hour), "stale"},
		{"just under a day", rfc(23 * time.Hour), "23h"},
		{"future clamps to zero", rfc(-2 * time.Hour), "0h"},
		{"empty", "", "?"},
		{"unparseable", "not-a-timestamp", "?"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := humanAge(tt.verified, now); got != tt.want {
				t.Errorf("humanAge(%q) = %q, want %q", tt.verified, got, tt.want)
			}
		})
	}
}

// --- studio index + facts: rehearse adapter end-to-end ---

// newRehearsedRepo writes a repo directory with a valid rehearse report
// (started_at = 2026-01-02T03:04:05Z, one pass scenario for x#ac:y) and
// returns the repo path. A go.mod is NOT written so the manifests adapter
// emits zero facts — keeping the fact count deterministic.
func newRehearsedRepo(t *testing.T, dir, name string) string {
	t.Helper()
	rehearseDir := filepath.Join(dir, name, ".specscore", "rehearse")
	if err := os.MkdirAll(rehearseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	report := `{
  "runner_version": "0.3.0",
  "git_sha": "abc",
  "git_dirty": false,
  "started_at": "2026-01-02T03:04:05Z",
  "scenarios": [
    {
      "file": "spec/features/x/_tests/s.md",
      "status": "pass",
      "verifies": ["x#ac:y"],
      "duration_ms": 42,
      "bag": {},
      "steps": []
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(rehearseDir, "latest.json"), []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, name)
}

// newBrokenRehearsalRepo writes a repo whose latest.json is not valid JSON
// and a go.mod (so the manifests adapter emits facts that remain queryable).
func newBrokenRehearsalRepo(t *testing.T, dir, name string) string {
	t.Helper()
	rehearseDir := filepath.Join(dir, name, ".specscore", "rehearse")
	if err := os.MkdirAll(rehearseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rehearseDir, "latest.json"), []byte("not json {{{"), 0o644); err != nil {
		t.Fatal(err)
	}
	gomod := "module example.com/fixture\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(dir, name, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, name)
}

// AC: observed-at-run-time — rehearse-adapter facts carry the report's
// started_at while facts from other adapters carry the index-run timestamp.
// Feature: cli/rehearse/evidence (REQ: observed-at-run-time).
func TestStudioIndex_RehearseObservedAtFromReport(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	wsDir := filepath.Dir(wsPath)
	newRehearsedRepo(t, wsDir, "repo-a")
	newSpecScoreRepo(t, wsDir, "repo-a")

	_, _, err := runStudioCmd(t, "index", "--workspace", wsPath)
	if err != nil {
		t.Fatalf("studio index: %v", err)
	}

	// Query verified-behavior facts — they must carry the report's started_at.
	jsonOut, _, err := runStudioCmd(t, "facts", "--workspace", wsPath,
		"--class", "verified-behavior", "--format", "json")
	if err != nil {
		t.Fatalf("studio facts --class verified-behavior: %v", err)
	}
	var vbFacts []fact.Fact
	if err := json.Unmarshal([]byte(jsonOut), &vbFacts); err != nil {
		t.Fatalf("verified-behavior facts not JSON: %v\n%s", err, jsonOut)
	}
	if len(vbFacts) == 0 {
		t.Fatalf("got 0 verified-behavior facts, want ≥1")
	}
	const reportTS = "2026-01-02T03:04:05Z"
	for _, f := range vbFacts {
		if f.ObservedAt != reportTS {
			t.Errorf("verified-behavior fact ObservedAt = %q, want %q: %+v",
				f.ObservedAt, reportTS, f)
		}
	}

	// Query declared facts (from the specscore adapter) — they must NOT carry
	// the report timestamp; they carry the index-run timestamp instead.
	jsonOut2, _, err := runStudioCmd(t, "facts", "--workspace", wsPath,
		"--class", "declared", "--format", "json")
	if err != nil {
		t.Fatalf("studio facts --class declared: %v", err)
	}
	var declFacts []fact.Fact
	if err := json.Unmarshal([]byte(jsonOut2), &declFacts); err != nil {
		t.Fatalf("declared facts not JSON: %v\n%s", err, jsonOut2)
	}
	if len(declFacts) == 0 {
		t.Fatalf("got 0 declared facts, want ≥1")
	}
	for _, f := range declFacts {
		if f.ObservedAt == reportTS {
			t.Errorf("declared fact ObservedAt = %q, should NOT be the report timestamp: %+v",
				f.ObservedAt, f)
		}
		if f.ObservedAt == "" {
			t.Errorf("declared fact ObservedAt is empty: %+v", f)
		}
	}
}

// AC: malformed-report-warns — a malformed report file causes a warning
// naming the report file and adapter "rehearse"; exit 0; manifests facts
// remain queryable.
// Feature: cli/rehearse/evidence (REQ: adapter-rehearse, REQ: partial-tolerance).
func TestStudioIndex_MalformedRehearseReportWarns(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	wsDir := filepath.Dir(wsPath)
	newBrokenRehearsalRepo(t, wsDir, "repo-a")

	out, _, err := runStudioCmd(t, "index", "--workspace", wsPath)
	if err != nil {
		t.Fatalf("studio index with malformed rehearse report exited non-zero: %v", err)
	}

	// The summary must list a warning naming latest.json and adapter "rehearse".
	if !strings.Contains(out, "Warnings: 1") {
		t.Errorf("summary missing \"Warnings: 1\"; got:\n%s", out)
	}
	if !strings.Contains(out, "rehearse") {
		t.Errorf("warning summary does not name adapter \"rehearse\"; got:\n%s", out)
	}
	if !strings.Contains(out, "latest.json") {
		t.Errorf("warning summary does not name the report file \"latest.json\"; got:\n%s", out)
	}

	// Manifests facts (from go.mod) must still be queryable.
	countOut, _, err := runStudioCmd(t, "facts", "--workspace", wsPath,
		"--adapter", "manifests", "--count")
	if err != nil {
		t.Fatalf("studio facts --adapter manifests: %v", err)
	}
	if countOut == "0\n" {
		t.Errorf("manifests adapter emitted 0 facts despite go.mod being present")
	}
}

// AC: missing-report-silent — no .specscore/rehearse/ directory → no warning
// from adapter "rehearse" and zero verified-behavior facts.
// Feature: cli/rehearse/evidence (REQ: adapter-rehearse).
func TestStudioIndex_MissingRehearseDirectorySilent(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	wsDir := filepath.Dir(wsPath)
	// Add a SpecScore spec so the store has at least one fact (the store
	// returns an error when queried while empty, so we need a non-empty store
	// to safely filter by class).
	newSpecScoreRepo(t, wsDir, "repo-a")

	out, _, err := runStudioCmd(t, "index", "--workspace", wsPath)
	if err != nil {
		t.Fatalf("studio index: %v", err)
	}

	// No warning from the rehearse adapter — the summary may list "rehearse: 0"
	// in the "Facts by adapter" section, but the Warnings lines must not
	// mention "rehearse".
	warningsIdx := strings.Index(out, "Warnings:")
	if warningsIdx >= 0 {
		warningsSection := out[warningsIdx:]
		if strings.Contains(warningsSection, "rehearse") {
			t.Errorf("warnings section mentions \"rehearse\" — expected silence; got:\n%s", warningsSection)
		}
	}

	// Zero verified-behavior facts — the store has specscore facts so it's
	// non-empty and the query returns 0 matches instead of an empty-store error.
	countOut, _, err := runStudioCmd(t, "facts", "--workspace", wsPath,
		"--class", "verified-behavior", "--count")
	if err != nil {
		t.Fatalf("studio facts --class verified-behavior: %v", err)
	}
	if countOut != "0\n" {
		t.Errorf("verified-behavior count = %q, want \"0\\n\"", countOut)
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
