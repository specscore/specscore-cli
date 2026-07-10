package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/internal/studio/adapters"
	"github.com/specscore/specscore-cli/internal/studio/fact"
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
