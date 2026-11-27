package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// runInitCmd runs initCommand with the given args and stdin. Returns
// stdout, stderr, and the exit error (if any).
func runInitCmd(t *testing.T, stdin io.Reader, args ...string) (string, string, error) {
	t.Helper()
	cmd := initCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	if stdin != nil {
		cmd.SetIn(stdin)
	}
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

// hasGit reports whether the test machine has git on PATH. Tests that
// rely on git remote inference skip when git is absent.
func hasGit() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// initLocalGitRepo runs `git init` in dir, sets a user identity (so any
// commit succeeds without machine-level config), and optionally adds an
// origin remote.
func initLocalGitRepo(t *testing.T, dir, originURL string) {
	t.Helper()
	if !hasGit() {
		t.Skip("git not on PATH; skipping git-dependent test")
	}
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
	} {
		c := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if originURL != "" {
		c := exec.Command("git", "-C", dir, "remote", "add", "origin", originURL)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git remote add: %v\n%s", err, out)
		}
	}
}

// exitCodeOf extracts the numeric exit code from an error returned by
// initCommand. Returns 0 if err is nil, 10 if the error is not an
// exitcode.Error (matches the runtime's catch-all behavior).
func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var ec *exitcode.Error
	if errors.As(err, &ec) {
		return ec.ExitCode()
	}
	return exitcode.Unexpected
}

// specscore:verifies feature/cli/init#ac:greenfield-no-remote — empty dir, no git remote, no flags.
func TestInit_GreenfieldNoRemote(t *testing.T) {
	root := t.TempDir()
	withCwd(t, root)

	out, _, err := runInitCmd(t, nil, "--project", root)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !strings.Contains(out, "Initialized SpecScore project at") {
		t.Errorf("missing success message: %q", out)
	}

	// specscore.yaml must exist with line 1 being the schema-pointer comment.
	cfgBody, err := os.ReadFile(filepath.Join(root, "specscore.yaml"))
	if err != nil {
		t.Fatalf("specscore.yaml missing: %v", err)
	}
	cfgLines := strings.Split(string(cfgBody), "\n")
	wantLine1 := "# SpecScore Repo Config Schema: https://specscore.md/repo-config"
	if cfgLines[0] != wantLine1 {
		t.Errorf("specscore.yaml line 1 = %q, want %q", cfgLines[0], wantLine1)
	}
	// project.title should be the dir basename (root's basename is a TempDir random name).
	if !strings.Contains(string(cfgBody), "title:") {
		t.Errorf("expected project.title in: %s", cfgBody)
	}

	// Three index files exist.
	for _, p := range []string{
		"spec/README.md",
		"spec/ideas/README.md",
		"spec/features/README.md",
	} {
		if _, err := os.Stat(filepath.Join(root, p)); err != nil {
			t.Errorf("missing %s: %v", p, err)
		}
	}

	// Optional subtrees NOT created (out-of-MVP-scope).
	for _, p := range []string{"spec/research", "spec/decisions"} {
		if _, err := os.Stat(filepath.Join(root, p)); !os.IsNotExist(err) {
			t.Errorf("%s should NOT exist (out of MVP scope), got err=%v", p, err)
		}
	}

	// spec/ideas/README.md and spec/features/README.md must contain the canonical headings.
	ideasIdx, _ := os.ReadFile(filepath.Join(root, "spec/ideas/README.md"))
	for _, want := range []string{"# Ideas", "## Index", "## Open Questions", "ideas-index-specification"} {
		if !strings.Contains(string(ideasIdx), want) {
			t.Errorf("spec/ideas/README.md missing %q:\n%s", want, ideasIdx)
		}
	}
	featuresIdx, _ := os.ReadFile(filepath.Join(root, "spec/features/README.md"))
	for _, want := range []string{"# Features", "## Index", "## Open Questions", "features-index-specification"} {
		if !strings.Contains(string(featuresIdx), want) {
			t.Errorf("spec/features/README.md missing %q:\n%s", want, featuresIdx)
		}
	}
}

// specscore:verifies feature/cli/init#ac:greenfield-with-remote-inference — git remote populates project block.
func TestInit_GreenfieldWithRemoteInference(t *testing.T) {
	root := t.TempDir()
	initLocalGitRepo(t, root, "git@github.com:acme/example.git")

	_, _, err := runInitCmd(t, nil, "--project", root)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	body, _ := os.ReadFile(filepath.Join(root, "specscore.yaml"))
	for _, want := range []string{
		"host: github.com",
		"org: acme",
		"repo: example",
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("missing %q in specscore.yaml:\n%s", want, body)
		}
	}
}

// specscore:verifies feature/cli/init#ac:conflict-without-force — existing specscore.yaml + no --force = exit 1.
func TestInit_ConflictWithoutForce(t *testing.T) {
	root := t.TempDir()
	preexisting := []byte("# pre-existing user content\nsome: thing\n")
	if err := os.WriteFile(filepath.Join(root, "specscore.yaml"), preexisting, 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	_, _, err := runInitCmd(t, nil, "--project", root)
	if err == nil {
		t.Fatalf("expected conflict error, got nil")
	}
	if got := exitCodeOf(err); got != exitcode.Conflict {
		t.Errorf("exit code = %d, want %d (Conflict)", got, exitcode.Conflict)
	}

	// Pre-existing content untouched.
	body, _ := os.ReadFile(filepath.Join(root, "specscore.yaml"))
	if !bytes.Equal(body, preexisting) {
		t.Errorf("pre-existing config modified:\n%s", body)
	}
	// No spec/ tree created (atomicity: nothing else written when conflict).
	if _, err := os.Stat(filepath.Join(root, "spec")); !os.IsNotExist(err) {
		t.Errorf("spec/ should NOT be created on conflict, err=%v", err)
	}
}

// specscore:verifies feature/cli/init#ac:force-overwrites — existing specscore.yaml + --force = replaced.
func TestInit_ForceOverwrites(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "specscore.yaml"), []byte("# stale\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Pre-create unrelated content in spec/ to verify --force does NOT touch it.
	if err := os.MkdirAll(filepath.Join(root, "spec/ideas"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	preserved := []byte("# Already-existing Ideas index — do NOT overwrite\n")
	if err := os.WriteFile(filepath.Join(root, "spec/ideas/README.md"), preserved, 0o644); err != nil {
		t.Fatalf("seed idea index: %v", err)
	}

	_, _, err := runInitCmd(t, nil, "--project", root, "--force", "--title", "Acme")
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	body, _ := os.ReadFile(filepath.Join(root, "specscore.yaml"))
	if !strings.HasPrefix(string(body), "# SpecScore Repo Config Schema: https://specscore.md/repo-config") {
		t.Errorf("specscore.yaml not replaced with canonical:\n%s", body)
	}
	if !strings.Contains(string(body), "title: Acme") {
		t.Errorf("title not applied:\n%s", body)
	}

	// Pre-existing spec/ideas/README.md preserved.
	got, _ := os.ReadFile(filepath.Join(root, "spec/ideas/README.md"))
	if !bytes.Equal(got, preserved) {
		t.Errorf("spec/ideas/README.md was clobbered by --force; --force should only affect specscore.yaml. got:\n%s", got)
	}
}

// specscore:verifies feature/cli/init#ac:partial-state-resume — missing config but partial spec tree = completes.
func TestInit_PartialStateResume(t *testing.T) {
	root := t.TempDir()
	// Pre-existing spec/ideas/ with one Idea but no README.md and no specscore.yaml.
	if err := os.MkdirAll(filepath.Join(root, "spec/ideas"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	preIdea := []byte("# Idea: Pre-existing\n\n**Status:** Draft\n")
	if err := os.WriteFile(filepath.Join(root, "spec/ideas/preexisting.md"), preIdea, 0o644); err != nil {
		t.Fatalf("seed pre-idea: %v", err)
	}

	_, _, err := runInitCmd(t, nil, "--project", root)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	// All three indexes exist.
	for _, p := range []string{"spec/README.md", "spec/ideas/README.md", "spec/features/README.md"} {
		if _, err := os.Stat(filepath.Join(root, p)); err != nil {
			t.Errorf("missing %s after init: %v", p, err)
		}
	}

	// Pre-existing Idea preserved.
	got, _ := os.ReadFile(filepath.Join(root, "spec/ideas/preexisting.md"))
	if !bytes.Equal(got, preIdea) {
		t.Errorf("pre-existing idea was modified:\n%s", got)
	}
}

// specscore:verifies feature/cli/init#ac:partial-state-resume — existing index file is preserved on rerun.
func TestInit_RerunPreservesExistingIndexes(t *testing.T) {
	root := t.TempDir()
	// First init (greenfield).
	if _, _, err := runInitCmd(t, nil, "--project", root); err != nil {
		t.Fatalf("first init: %v", err)
	}
	// Manually edit one of the indexes — simulating user customization.
	customized := []byte("# My Custom Ideas Index\n\nUser-edited content.\n")
	if err := os.WriteFile(filepath.Join(root, "spec/ideas/README.md"), customized, 0o644); err != nil {
		t.Fatalf("customize: %v", err)
	}
	// Rerun init with --force on the config (it's the only way to rerun without conflict).
	if _, _, err := runInitCmd(t, nil, "--project", root, "--force"); err != nil {
		t.Fatalf("rerun: %v", err)
	}
	// Customized index preserved (init's --force only affects specscore.yaml).
	got, _ := os.ReadFile(filepath.Join(root, "spec/ideas/README.md"))
	if !bytes.Equal(got, customized) {
		t.Errorf("customized index clobbered:\n%s", got)
	}
}

// specscore:verifies feature/cli/init#ac:project-metadata-applied — explicit flags override inference.
func TestInit_ProjectMetadataAppliedFromFlags(t *testing.T) {
	root := t.TempDir()
	// Even when a remote is set, explicit flags MUST win.
	initLocalGitRepo(t, root, "git@github.com:will-be-overridden/wrong.git")

	_, _, err := runInitCmd(t, nil,
		"--project", root,
		"--title", "Acme Service",
		"--host", "github.com",
		"--org", "acme",
		"--repo", "service",
	)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	body, _ := os.ReadFile(filepath.Join(root, "specscore.yaml"))
	for _, want := range []string{
		"title: Acme Service",
		"host: github.com",
		"org: acme",
		"repo: service",
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("missing %q in specscore.yaml:\n%s", want, body)
		}
	}
	// Confirm flag overrode inference: the wrong-org from the remote must NOT appear.
	if strings.Contains(string(body), "will-be-overridden") {
		t.Errorf("flag did not override inference; got remote-derived org in:\n%s", body)
	}
}

// specscore:verifies feature/cli/init#ac:interactive-non-tty-rejected — -i without TTY = exit 2.
func TestInit_InteractiveNonTTY(t *testing.T) {
	root := t.TempDir()
	// Stdin is *bytes.Buffer (not a TTY by definition).
	_, _, err := runInitCmd(t, &bytes.Buffer{}, "--project", root, "-i")
	if err == nil {
		t.Fatalf("expected InvalidArgs error, got nil")
	}
	if got := exitCodeOf(err); got != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d (InvalidArgs)", got, exitcode.InvalidArgs)
	}
	// No files should have been created.
	if _, err := os.Stat(filepath.Join(root, "specscore.yaml")); !os.IsNotExist(err) {
		t.Errorf("specscore.yaml should not exist when -i fails the TTY check, err=%v", err)
	}
}

// specscore:verifies feature/cli/init#ac:interactive-mode-prompts-and-defaults — tests prompt flow with TTY stub.
func TestInit_InteractivePromptsHappyPath(t *testing.T) {
	root := t.TempDir()
	// Stub the TTY check so the test doesn't need a real terminal.
	orig := isTerminal
	isTerminal = func(_ io.Reader) bool { return true }
	t.Cleanup(func() { isTerminal = orig })

	// User responses: title="Manual Title", host=(empty=omit), org="acme", repo="service".
	stdin := strings.NewReader("Manual Title\n\nacme\nservice\n")
	out, _, err := runInitCmd(t, stdin, "--project", root, "-i", "--title", "PrefilledTitle")
	if err != nil {
		t.Fatalf("expected success, got: %v\n%s", err, out)
	}
	body, _ := os.ReadFile(filepath.Join(root, "specscore.yaml"))
	for _, want := range []string{
		"title: Manual Title",
		"org: acme",
		"repo: service",
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("missing %q in specscore.yaml:\n%s", want, body)
		}
	}
	// Empty-input host MUST be omitted (not emitted as empty string).
	if strings.Contains(string(body), "host: \"\"") || strings.Contains(string(body), "host: ''") {
		t.Errorf("empty host should be omitted, not emitted as empty string:\n%s", body)
	}
	// Prompt output should have shown the prefilled default for title.
	if !strings.Contains(out, "[PrefilledTitle]") {
		t.Errorf("prompt did not show prefilled default; got:\n%s", out)
	}
}

// specscore:verifies feature/cli/init#ac:exit-codes-discipline — invalid --project path = exit 2.
func TestInit_InvalidProjectPath(t *testing.T) {
	_, _, err := runInitCmd(t, nil, "--project", "/does/not/exist/anywhere")
	if err == nil {
		t.Fatalf("expected InvalidArgs error, got nil")
	}
	if got := exitCodeOf(err); got != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d (InvalidArgs)", got, exitcode.InvalidArgs)
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error message should name the not-exist condition: %v", err)
	}
}

// specscore:verifies feature/cli/init#ac:exit-codes-discipline — --project pointing at a file (not directory) = exit 2.
func TestInit_ProjectIsFileNotDirectory(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "regularfile")
	if err := os.WriteFile(filePath, []byte("hi"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	_, _, err := runInitCmd(t, nil, "--project", filePath)
	if err == nil {
		t.Fatalf("expected InvalidArgs error, got nil")
	}
	if got := exitCodeOf(err); got != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d (InvalidArgs)", got, exitcode.InvalidArgs)
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("error message should name the not-a-directory condition: %v", err)
	}
}

// AC: lint-clean-on-create (smoke) — generated config file's first line is exactly
// the schema-pointer; spec/ tree's index files contain the adherence-footer URLs.
//
// This does not invoke `specscore spec lint` (which would require linking the
// lint package and wiring up an in-process invocation); instead it asserts the
// structural invariants the lint rules check. The full end-to-end "specscore
// spec lint exits 0 after init" assertion is a CLI integration test outside
// this unit suite.
func TestInit_LintCleanInvariants(t *testing.T) {
	root := t.TempDir()
	if _, _, err := runInitCmd(t, nil, "--project", root); err != nil {
		t.Fatalf("init: %v", err)
	}

	// specscore.yaml line 1 invariant.
	body, _ := os.ReadFile(filepath.Join(root, "specscore.yaml"))
	lines := strings.Split(string(body), "\n")
	want := "# SpecScore Repo Config Schema: https://specscore.md/repo-config"
	if lines[0] != want {
		t.Errorf("specscore.yaml line 1 = %q, want %q", lines[0], want)
	}

	// spec/ideas/README.md adherence footer + table header invariants.
	ideas, _ := os.ReadFile(filepath.Join(root, "spec/ideas/README.md"))
	if !strings.Contains(string(ideas), "specscore.md/ideas-index-specification") {
		t.Errorf("ideas index missing adherence-footer URL:\n%s", ideas)
	}
	// The `## Index` table requires the canonical 5-column header.
	if !strings.Contains(string(ideas), "| Idea | Status | Date | Owner | Promotes To |") {
		t.Errorf("ideas index missing canonical column header:\n%s", ideas)
	}

	// spec/features/README.md adherence footer + table header invariants.
	feats, _ := os.ReadFile(filepath.Join(root, "spec/features/README.md"))
	if !strings.Contains(string(feats), "specscore.md/features-index-specification") {
		t.Errorf("features index missing adherence-footer URL:\n%s", feats)
	}
	if !strings.Contains(string(feats), "| Feature | Status | Description |") {
		t.Errorf("features index missing canonical column header:\n%s", feats)
	}
}
