package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runGraphCmd executes the graph command group with args against a fresh
// command tree, capturing stdout/stderr.
func runGraphCmd(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := graphCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func graphExit(t *testing.T, err error) int {
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

// newGraphRepo creates a temp repo with specscore.yaml and a scaffolded module
// + entity, returning the repo root.
func newGraphRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := "# SpecScore Repo Config Schema: https://specscore.md/repo-config\n\nproject:\n  title: T\n"
	if err := os.WriteFile(filepath.Join(dir, "specscore.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runGraphCmd(t, "new", "module", "--name", "Identity", "--project", dir); err != nil {
		t.Fatalf("scaffolding module: %v", err)
	}
	if _, _, err := runGraphCmd(t, "new", "entity", "--name", "User", "--module", "identity", "--project", dir); err != nil {
		t.Fatalf("scaffolding entity: %v", err)
	}
	return dir
}

// --- root command ---

func TestGraph_HelpExitsZero(t *testing.T) {
	out, _, err := runGraphCmd(t)
	if err != nil {
		t.Fatalf("bare graph should show help: %v", err)
	}
	for _, sub := range []string{"new", "lint", "list", "refs"} {
		if !strings.Contains(out, sub) {
			t.Errorf("help missing %q", sub)
		}
	}
}

// --- graph new ---

func TestGraphNew_MissingKind(t *testing.T) {
	_, _, err := runGraphCmd(t, "new")
	if code := graphExit(t, err); code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
}

func TestGraphNew_TooManyArgs(t *testing.T) {
	_, _, err := runGraphCmd(t, "new", "entity", "extra")
	if code := graphExit(t, err); code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
}

func TestGraphNew_RetiredKinds(t *testing.T) {
	for _, kind := range []string{"value-object", "enum"} {
		_, _, err := runGraphCmd(t, "new", kind, "--name", "X")
		if code := graphExit(t, err); code != 2 {
			t.Fatalf("%s: want 2, got %d", kind, code)
		}
		if !strings.Contains(err.Error(), "ModelSpec") {
			t.Fatalf("%s: message should point to ModelSpec: %v", kind, err)
		}
	}
}

func TestGraphNew_UnknownKind(t *testing.T) {
	_, _, err := runGraphCmd(t, "new", "gadget", "--name", "X")
	if code := graphExit(t, err); code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
}

func TestGraphNew_MissingName(t *testing.T) {
	_, _, err := runGraphCmd(t, "new", "entity")
	if code := graphExit(t, err); code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
}

func TestGraphNew_NoConfigRoot(t *testing.T) {
	dir := t.TempDir() // no specscore.yaml
	_, _, err := runGraphCmd(t, "new", "module", "--name", "M", "--project", dir)
	if code := graphExit(t, err); code != 3 {
		t.Fatalf("want 3, got %d", code)
	}
}

func TestGraphNew_ScaffoldAndCollision(t *testing.T) {
	dir := newGraphRepo(t)
	// Files exist.
	if _, err := os.Stat(filepath.Join(dir, "spec/graph/modules/identity/entities/user.md")); err != nil {
		t.Fatal(err)
	}
	// Repeat entity collides with exit 1.
	_, _, err := runGraphCmd(t, "new", "entity", "--name", "User", "--module", "identity", "--project", dir)
	if code := graphExit(t, err); code != 1 {
		t.Fatalf("want 1, got %d", code)
	}
}

func TestGraphNew_ModulePrefixedID(t *testing.T) {
	dir := newGraphRepo(t)
	_, _, err := runGraphCmd(t, "new", "entity", "--id", "identity.thing", "--module", "identity", "--project", dir)
	if code := graphExit(t, err); code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
}

func TestGraphNew_RelationshipEndpoints(t *testing.T) {
	dir := newGraphRepo(t)
	// Missing --from/--to exits 2.
	_, _, err := runGraphCmd(t, "new", "relationship", "--name", "reserves", "--module", "identity", "--project", dir)
	if code := graphExit(t, err); code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
	// Unresolvable endpoint exits 3.
	_, _, err = runGraphCmd(t, "new", "relationship", "--name", "reserves", "--module", "identity",
		"--from", "identity.user", "--to", "identity.ghost", "--project", dir)
	if code := graphExit(t, err); code != 3 {
		t.Fatalf("want 3, got %d", code)
	}
	// Resolvable endpoints succeed.
	if _, _, err := runGraphCmd(t, "new", "entity", "--name", "Team", "--module", "identity", "--project", dir); err != nil {
		t.Fatal(err)
	}
	out, _, err := runGraphCmd(t, "new", "relationship", "--name", "member", "--module", "identity",
		"--from", "identity.user", "--to", "identity.team", "--project", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "relationships/member.md") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestGraphNew_CommandWithSubjectAndBareModule(t *testing.T) {
	dir := newGraphRepo(t)
	if _, _, err := runGraphCmd(t, "new", "command", "--name", "CreateUser", "--module", "identity",
		"--subject", "identity.user", "--project", dir); err != nil {
		t.Fatal(err)
	}
	out, _, err := runGraphCmd(t, "new", "module", "--name", "Bare", "--bare", "--project", dir)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "\n") != 2 { // "Created ..." + one file line
		t.Fatalf("bare module should create one file: %s", out)
	}
	// Generated graph lints clean.
	if _, _, err := runGraphCmd(t, "lint", "--project", dir); err != nil {
		t.Fatalf("scaffolded graph should lint clean: %v", err)
	}
}

// --- graph lint ---

func TestGraphLint_NoGraphRootNotice(t *testing.T) {
	dir := t.TempDir()
	cfg := "# SpecScore Repo Config Schema: https://specscore.md/repo-config\n\nproject:\n  title: T\n"
	if err := os.WriteFile(filepath.Join(dir, "specscore.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, err := runGraphCmd(t, "lint", "--project", dir)
	if err != nil {
		t.Fatalf("no graph root must exit 0: %v", err)
	}
	if !strings.Contains(out, "No graph root") {
		t.Fatalf("expected notice, got: %s", out)
	}
	// Structured formats emit an empty list.
	out, _, err = runGraphCmd(t, "lint", "--project", dir, "--format", "json")
	if err != nil || !strings.Contains(out, "[]") {
		t.Fatalf("json no-root: %q %v", out, err)
	}
	out, _, err = runGraphCmd(t, "lint", "--project", dir, "--format", "yaml")
	if err != nil || !strings.Contains(out, "[]") {
		t.Fatalf("yaml no-root: %q %v", out, err)
	}
}

func TestGraphLint_ViolationsExitOne(t *testing.T) {
	dir := newGraphRepo(t)
	// Seed a violation: owner field.
	bad := filepath.Join(dir, "spec/graph/modules/identity/entities/bad.md")
	content := "---\nkind: entity\nid: bad\nname: Bad\nstatus: draft\nowner: alice\nsummary: s\n---\n"
	if err := os.WriteFile(bad, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, err := runGraphCmd(t, "lint", "--project", dir)
	if code := graphExit(t, err); code != 1 {
		t.Fatalf("want 1, got %d", code)
	}
	if !strings.Contains(out, "graph-no-owner-field") {
		t.Fatalf("expected rule in output: %s", out)
	}
	// JSON format carries the violation shape.
	out, _, _ = runGraphCmd(t, "lint", "--project", dir, "--format", "json")
	if !strings.Contains(out, `"rule": "graph-no-owner-field"`) {
		t.Fatalf("json output: %s", out)
	}
	// YAML format works too.
	out, _, _ = runGraphCmd(t, "lint", "--project", dir, "--format", "yaml")
	if !strings.Contains(out, "rule: graph-no-owner-field") {
		t.Fatalf("yaml output: %s", out)
	}
	// --ignore suppresses it back to exit 0.
	if _, _, err := runGraphCmd(t, "lint", "--project", dir, "--ignore", "graph-no-owner-field"); err != nil {
		t.Fatalf("ignore should clean: %v", err)
	}
	// --rules selects only an unrelated rule → clean.
	if _, _, err := runGraphCmd(t, "lint", "--project", dir, "--rules", "graph-id-kebab-case"); err != nil {
		t.Fatalf("rules filter should clean: %v", err)
	}
}

func TestGraphLint_FlagValidation(t *testing.T) {
	dir := newGraphRepo(t)
	cases := [][]string{
		{"lint", "--project", dir, "--rules", "a", "--ignore", "b"},
		{"lint", "--project", dir, "--severity", "bogus"},
		{"lint", "--project", dir, "--format", "bogus"},
		{"lint", "--project", dir, "--rules", "not-a-rule"},
		{"lint", "--project", dir, "--ignore", "not-a-rule"},
	}
	for _, args := range cases {
		_, _, err := runGraphCmd(t, args...)
		if code := graphExit(t, err); code != 2 {
			t.Fatalf("%v: want 2, got %d", args, code)
		}
	}
}

func TestGraphLint_LoadError(t *testing.T) {
	dir := t.TempDir()
	cfg := "# SpecScore Repo Config Schema: https://specscore.md/repo-config\n\nproject:\n  title: T\n"
	_ = os.WriteFile(filepath.Join(dir, "specscore.yaml"), []byte(cfg), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, "spec/graph"), 0o755)
	// modules as a file breaks discovery.
	_ = os.WriteFile(filepath.Join(dir, "spec/graph/modules"), []byte("x"), 0o644)
	_, _, err := runGraphCmd(t, "lint", "--project", dir)
	if code := graphExit(t, err); code != 10 {
		t.Fatalf("want 10, got %d", code)
	}
}

// --- graph list ---

func TestGraphList_TextSortedAndFilters(t *testing.T) {
	dir := newGraphRepo(t)
	out, _, err := runGraphCmd(t, "list", "--project", dir)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || lines[0] != "identity" || lines[1] != "identity.user" {
		t.Fatalf("unexpected list: %v", lines)
	}
	out, _, err = runGraphCmd(t, "list", "--project", dir, "--kind", "entity")
	if err != nil || strings.TrimSpace(out) != "identity.user" {
		t.Fatalf("kind filter: %q %v", out, err)
	}
	out, _, err = runGraphCmd(t, "list", "--project", dir, "--module", "identity", "--kind", "module")
	if err != nil || strings.TrimSpace(out) != "identity" {
		t.Fatalf("module filter: %q %v", out, err)
	}
}

func TestGraphList_StructuredFormats(t *testing.T) {
	dir := newGraphRepo(t)
	out, _, err := runGraphCmd(t, "list", "--project", dir, "--format", "yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"id:", "kind:", "name:", "path:"} {
		if !strings.Contains(out, key) {
			t.Fatalf("yaml missing %q: %s", key, out)
		}
	}
	out, _, err = runGraphCmd(t, "list", "--project", dir, "--format", "json")
	if err != nil || !strings.Contains(out, `"id": "identity.user"`) {
		t.Fatalf("json: %q %v", out, err)
	}
}

func TestGraphList_Validation(t *testing.T) {
	dir := newGraphRepo(t)
	_, _, err := runGraphCmd(t, "list", "--project", dir, "--kind", "bogus")
	if code := graphExit(t, err); code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
	_, _, err = runGraphCmd(t, "list", "--project", dir, "--format", "bogus")
	if code := graphExit(t, err); code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
}

func TestGraphList_NoGraphRoot(t *testing.T) {
	dir := t.TempDir()
	cfg := "# SpecScore Repo Config Schema: https://specscore.md/repo-config\n\nproject:\n  title: T\n"
	_ = os.WriteFile(filepath.Join(dir, "specscore.yaml"), []byte(cfg), 0o644)
	out, _, err := runGraphCmd(t, "list", "--project", dir)
	if err != nil || strings.TrimSpace(out) != "" {
		t.Fatalf("empty list expected: %q %v", out, err)
	}
	// json yields [] for an empty graph.
	out, _, err = runGraphCmd(t, "list", "--project", dir, "--format", "json")
	if err != nil || !strings.Contains(out, "[]") {
		t.Fatalf("json empty: %q %v", out, err)
	}
}

func TestGraphList_LoadError(t *testing.T) {
	dir := t.TempDir()
	cfg := "# SpecScore Repo Config Schema: https://specscore.md/repo-config\n\nproject:\n  title: T\n"
	_ = os.WriteFile(filepath.Join(dir, "specscore.yaml"), []byte(cfg), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, "spec/graph"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "spec/graph/modules"), []byte("x"), 0o644)
	_, _, err := runGraphCmd(t, "list", "--project", dir)
	if code := graphExit(t, err); code != 10 {
		t.Fatalf("want 10, got %d", code)
	}
}

// --- graph refs ---

func newRefsRepo(t *testing.T) string {
	t.Helper()
	dir := newGraphRepo(t)
	if _, _, err := runGraphCmd(t, "new", "command", "--name", "CreateUser", "--module", "identity",
		"--subject", "identity.user", "--project", dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestGraphRefs_TextAndStructured(t *testing.T) {
	dir := newRefsRepo(t)
	out, _, err := runGraphCmd(t, "refs", "identity.user", "--project", dir)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "identity.create-user" {
		t.Fatalf("refs text: %q", out)
	}
	out, _, err = runGraphCmd(t, "refs", "identity.user", "--project", dir, "--format", "yaml")
	if err != nil || !strings.Contains(out, "id: identity.create-user") || !strings.Contains(out, "kind: command") {
		t.Fatalf("refs yaml: %q %v", out, err)
	}
	out, _, err = runGraphCmd(t, "refs", "identity.user", "--project", dir, "--format", "json")
	if err != nil || !strings.Contains(out, `"identity.create-user"`) {
		t.Fatalf("refs json: %q %v", out, err)
	}
	// Shorthand resolution.
	out, _, err = runGraphCmd(t, "refs", "user", "--project", dir)
	if err != nil || strings.TrimSpace(out) != "identity.create-user" {
		t.Fatalf("shorthand: %q %v", out, err)
	}
	// Transitive terminates.
	if _, _, err := runGraphCmd(t, "refs", "identity.user", "--transitive", "--project", dir); err != nil {
		t.Fatal(err)
	}
}

func TestGraphRefs_Errors(t *testing.T) {
	dir := newRefsRepo(t)
	// Missing positional.
	_, _, err := runGraphCmd(t, "refs", "--project", dir)
	if code := graphExit(t, err); code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
	// Too many positionals.
	_, _, err = runGraphCmd(t, "refs", "a", "b", "--project", dir)
	if code := graphExit(t, err); code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
	// Bad format.
	_, _, err = runGraphCmd(t, "refs", "identity.user", "--project", dir, "--format", "bogus")
	if code := graphExit(t, err); code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
	// Not found.
	_, _, err = runGraphCmd(t, "refs", "nope", "--project", dir)
	if code := graphExit(t, err); code != 3 {
		t.Fatalf("want 3, got %d", code)
	}
}

func TestGraphRefs_Ambiguous(t *testing.T) {
	dir := newRefsRepo(t)
	// Second module also declaring `user` makes the bare shorthand ambiguous.
	if _, _, err := runGraphCmd(t, "new", "module", "--name", "Directory", "--project", dir); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runGraphCmd(t, "new", "entity", "--name", "User", "--module", "directory", "--project", dir); err != nil {
		t.Fatal(err)
	}
	_, _, err := runGraphCmd(t, "refs", "user", "--project", dir)
	if code := graphExit(t, err); code != 5 {
		t.Fatalf("want 5 (AmbiguousSlug), got %d", code)
	}
	if !strings.Contains(err.Error(), "directory.user") || !strings.Contains(err.Error(), "identity.user") {
		t.Fatalf("candidates missing: %v", err)
	}
}

func TestGraphRefs_NoGraphRoot(t *testing.T) {
	dir := t.TempDir()
	cfg := "# SpecScore Repo Config Schema: https://specscore.md/repo-config\n\nproject:\n  title: T\n"
	_ = os.WriteFile(filepath.Join(dir, "specscore.yaml"), []byte(cfg), 0o644)
	_, _, err := runGraphCmd(t, "refs", "x", "--project", dir)
	if code := graphExit(t, err); code != 3 {
		t.Fatalf("want 3, got %d", code)
	}
}

func TestGraphRefs_LoadError(t *testing.T) {
	dir := t.TempDir()
	cfg := "# SpecScore Repo Config Schema: https://specscore.md/repo-config\n\nproject:\n  title: T\n"
	_ = os.WriteFile(filepath.Join(dir, "specscore.yaml"), []byte(cfg), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, "spec/graph"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "spec/graph/modules"), []byte("x"), 0o644)
	_, _, err := runGraphCmd(t, "refs", "x", "--project", dir)
	if code := graphExit(t, err); code != 10 {
		t.Fatalf("want 10, got %d", code)
	}
}

// --- shared plumbing ---

func TestResolveGraphRepoRoot_Errors(t *testing.T) {
	// --project abs failure.
	origAbs := filepathAbsFn
	filepathAbsFn = func(string) (string, error) { return "", errors.New("boom") }
	_, err := resolveGraphRepoRoot("x")
	filepathAbsFn = origAbs
	if code := graphExit(t, err); code != 2 {
		t.Fatalf("abs error: want 2, got %d", code)
	}
	// getwd failure.
	origWd := osGetwdFn
	osGetwdFn = func() (string, error) { return "", errors.New("boom") }
	_, err = resolveGraphRepoRoot("")
	osGetwdFn = origWd
	if code := graphExit(t, err); code != 10 {
		t.Fatalf("getwd error: want 10, got %d", code)
	}
}

func TestResolveGraphRepoRoot_CwdSuccess(t *testing.T) {
	// Empty --project resolves the repo root from the working directory.
	dir := t.TempDir()
	cfg := "# SpecScore Repo Config Schema: https://specscore.md/repo-config\n\nproject:\n  title: T\n"
	if err := os.WriteFile(filepath.Join(dir, "specscore.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := osGetwdFn
	osGetwdFn = func() (string, error) { return dir, nil }
	t.Cleanup(func() { osGetwdFn = orig })
	root, err := resolveGraphRepoRoot("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if root != dir {
		t.Fatalf("root = %q, want %q", root, dir)
	}
}

func TestGraphLint_OutputEncoderErrors(t *testing.T) {
	origJ := newJSONEnc
	t.Cleanup(func() { newJSONEnc = origJ })
	newJSONEnc = func(w io.Writer) jsonEnc { return failingJSON{} }

	// No-root structured path (graph.go outputLintViolations error branch).
	noRoot := t.TempDir()
	cfg := "# SpecScore Repo Config Schema: https://specscore.md/repo-config\n\nproject:\n  title: T\n"
	_ = os.WriteFile(filepath.Join(noRoot, "specscore.yaml"), []byte(cfg), 0o644)
	_, _, err := runGraphCmd(t, "lint", "--project", noRoot, "--format", "json")
	if code := graphExit(t, err); code != 10 {
		t.Fatalf("no-root encoder error: want 10, got %d", code)
	}

	// Normal path with a discovered graph.
	dir := newGraphRepo(t)
	_, _, err = runGraphCmd(t, "lint", "--project", dir, "--format", "json")
	if code := graphExit(t, err); code != 10 {
		t.Fatalf("lint encoder error: want 10, got %d", code)
	}
}

func TestGraphCommands_RepoRootErrorsPropagate(t *testing.T) {
	// Each verb surfaces the no-config exit 3 from findRepoConfigRoot.
	dir := t.TempDir()
	for _, args := range [][]string{
		{"lint", "--project", dir},
		{"list", "--project", dir},
		{"refs", "x", "--project", dir},
	} {
		_, _, err := runGraphCmd(t, args...)
		if code := graphExit(t, err); code != 3 {
			t.Fatalf("%v: want 3, got %d", args, code)
		}
	}
}

func TestGraphEncoders_ErrorPaths(t *testing.T) {
	// Force encoder failures through the injectable factories.
	origY, origJ := newYAMLEnc, newJSONEnc
	defer func() { newYAMLEnc, newJSONEnc = origY, origJ }()
	newYAMLEnc = func(w io.Writer) yamlEnc { return failingYAML{} }
	newJSONEnc = func(w io.Writer) jsonEnc { return failingJSON{} }

	var buf bytes.Buffer
	if err := writeGraphYAML(&buf, nil); err == nil {
		t.Fatal("yaml list error expected")
	}
	if err := writeGraphJSON(&buf, nil); err == nil {
		t.Fatal("json list error expected")
	}
	if err := writeGraphRefsYAML(&buf, nil); err == nil {
		t.Fatal("yaml refs error expected")
	}
	if err := writeGraphRefsJSON(&buf, nil); err == nil {
		t.Fatal("json refs error expected")
	}
}

type failingYAML struct{}

func (failingYAML) Encode(any) error { return errors.New("yaml boom") }
func (failingYAML) Close() error     { return nil }

type failingJSON struct{}

func (failingJSON) Encode(any) error { return errors.New("json boom") }
