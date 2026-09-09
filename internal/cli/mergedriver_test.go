package cli

// Tests for `merge-driver install` (the .gitattributes/git-config installer)
// and `merge-driver index` (the generated-index regeneration driver). See
// docs/merge-drivers.md and event_merge_driver_test.go (the events driver
// and the end-to-end `git merge` integration test).

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lint"
)

func runMergeDriver(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := mergeDriverCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

// stubGitSeams replaces the git-shelling seams with in-memory fakes so
// `merge-driver install` tests don't need a real git binary. It returns the
// captured .gitattributes path and config writes for assertions.
func stubGitSeams(t *testing.T, topLevel string) (configWrites *[][2]string) {
	t.Helper()
	writes := &[][2]string{}
	prevTop, prevCfg := gitTopLevelFn, gitConfigSetFn
	gitTopLevelFn = func(string) (string, error) { return topLevel, nil }
	gitConfigSetFn = func(_ string, key, value string) error {
		*writes = append(*writes, [2]string{key, value})
		return nil
	}
	t.Cleanup(func() { gitTopLevelFn, gitConfigSetFn = prevTop, prevCfg })
	return writes
}

func TestMergeDriverInstall_WritesAttributesAndConfig(t *testing.T) {
	root := t.TempDir()
	writeSpecscoreYAML(t, root, "")
	writes := stubGitSeams(t, root)

	out, _, err := runMergeDriver(t, "install", "--project", root)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if !strings.Contains(out, "merge."+eventsDriverName+".driver installed") ||
		!strings.Contains(out, "merge."+indexDriverName+".driver installed") {
		t.Fatalf("install output = %q, missing expected confirmation lines", out)
	}

	attrs, err := os.ReadFile(filepath.Join(root, ".gitattributes"))
	if err != nil {
		t.Fatalf("reading .gitattributes: %v", err)
	}
	attrText := string(attrs)
	if !strings.Contains(attrText, ".specscore/events.jsonl merge="+eventsDriverName) {
		t.Fatalf(".gitattributes = %q, missing the events-ledger line", attrText)
	}
	for _, want := range []string{
		"spec/features/README.md merge=" + indexDriverName,
		"spec/ideas/README.md merge=" + indexDriverName,
		"spec/ideas/archived/README.md merge=" + indexDriverName,
		"spec/plans/README.md merge=" + indexDriverName,
		"spec/tasks/README.md merge=" + indexDriverName,
		"spec/lessons/README.md merge=" + indexDriverName,
		"spec/decisions/README.md merge=" + indexDriverName,
		"spec/decisions/archived/README.md merge=" + indexDriverName,
	} {
		if !strings.Contains(attrText, want) {
			t.Errorf(".gitattributes missing line %q; got:\n%s", want, attrText)
		}
	}

	wantConfig := map[string]string{
		"merge." + eventsDriverName + ".driver": "specscore event merge-driver %O %A %B",
		"merge." + indexDriverName + ".driver":  "specscore merge-driver index %O %A %B %P",
	}
	got := map[string]string{}
	for _, kv := range *writes {
		got[kv[0]] = kv[1]
	}
	for key, want := range wantConfig {
		if got[key] != want {
			t.Errorf("git config %s = %q, want %q", key, got[key], want)
		}
	}
}

func TestMergeDriverInstall_IsIdempotent(t *testing.T) {
	root := t.TempDir()
	writeSpecscoreYAML(t, root, "")
	stubGitSeams(t, root)

	if _, _, err := runMergeDriver(t, "install", "--project", root); err != nil {
		t.Fatalf("first install: %v", err)
	}
	first, err := os.ReadFile(filepath.Join(root, ".gitattributes"))
	if err != nil {
		t.Fatal(err)
	}

	out, _, err := runMergeDriver(t, "install", "--project", root)
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	if !strings.Contains(out, "added=0") {
		t.Fatalf("second install output = %q, want added=0 (no duplicate lines)", out)
	}
	second, err := os.ReadFile(filepath.Join(root, ".gitattributes"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf(".gitattributes changed on a repeat install:\n first=%q\n second=%q", first, second)
	}
}

func TestMergeDriverInstall_PreservesExistingGitattributesContent(t *testing.T) {
	root := t.TempDir()
	writeSpecscoreYAML(t, root, "")
	stubGitSeams(t, root)
	preexisting := "*.png binary\n"
	if err := os.WriteFile(filepath.Join(root, ".gitattributes"), []byte(preexisting), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := runMergeDriver(t, "install", "--project", root); err != nil {
		t.Fatalf("install: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, ".gitattributes"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(got), preexisting) {
		t.Fatalf(".gitattributes = %q, want it to still start with the pre-existing line", got)
	}
}

func TestMergeDriverInstall_CustomLedgerPathOutsideGitRootFailsClosed(t *testing.T) {
	root := t.TempDir()
	writeSpecscoreYAML(t, root, "events:\n  subscribers:\n    - type: jsonl\n      path: /elsewhere/events.jsonl\n")
	// gitRoot deliberately does not contain the configured absolute ledger
	// path, simulating a misconfiguration.
	gitRoot := t.TempDir()
	stubGitSeams(t, gitRoot)

	_, _, err := runMergeDriver(t, "install", "--project", root)
	if err == nil {
		t.Fatal("expected an error when the configured ledger is outside the git repository; got nil")
	}
	if exitCodeOf(err) != exitcode.InvalidState {
		t.Fatalf("exit code = %d, want exitcode.InvalidState (%d): %v", exitCodeOf(err), exitcode.InvalidState, err)
	}
}

func TestMergeDriverInstallCommand_Help(t *testing.T) {
	out, _, err := runMergeDriver(t, "install", "--help")
	if err != nil || !strings.Contains(out, "Idempotent") || !strings.Contains(out, ".gitattributes") {
		t.Fatalf("install --help = %q, err=%v", out, err)
	}
}

// --- merge-driver index ---

func stubLintFix(t *testing.T, fn func(lint.Options) (lint.Result, error)) {
	t.Helper()
	prev := mergeDriverLintFixFn
	mergeDriverLintFixFn = fn
	t.Cleanup(func() { mergeDriverLintFixFn = prev })
}

func TestMergeDriverIndex_RegeneratesAndCopiesOverOurs(t *testing.T) {
	root := t.TempDir()
	// Resolve symlinks up front: on macOS, t.TempDir() returns an
	// unresolved /var/... path, but os.Getwd() after os.Chdir() returns the
	// physical /private/var/... path — comparing the two directly is a
	// spurious cross-platform test failure, not a production bug.
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	writeSpecscoreYAML(t, root, "")
	withCwd(t, root)

	regenerated := filepath.Join(root, "spec", "features", "README.md")
	if err := os.MkdirAll(filepath.Dir(regenerated), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(regenerated, []byte("# Features\n\nfresh index content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var capturedOpts lint.Options
	stubLintFix(t, func(opts lint.Options) (lint.Result, error) {
		capturedOpts = opts
		return lint.Result{Fixed: []string{"features/README.md"}}, nil
	})

	ours := filepath.Join(t.TempDir(), "git-temp-a")
	if err := os.WriteFile(ours, []byte("stale conflicted content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(t.TempDir(), "git-temp-o")
	theirs := filepath.Join(t.TempDir(), "git-temp-b")
	_ = os.WriteFile(base, nil, 0o644)
	_ = os.WriteFile(theirs, nil, 0o644)

	out, _, err := runMergeDriver(t, "index", base, ours, theirs, "spec/features/README.md")
	if err != nil {
		t.Fatalf("merge-driver index: %v", err)
	}
	if !strings.Contains(out, "regenerated=spec/features/README.md") {
		t.Fatalf("output = %q, missing regenerated= line", out)
	}
	if capturedOpts.SpecRoot != filepath.Join(root, "spec") || !capturedOpts.Fix {
		t.Fatalf("lint options = %+v, want SpecRoot=%s Fix=true", capturedOpts, filepath.Join(root, "spec"))
	}
	got, err := os.ReadFile(ours)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# Features\n\nfresh index content\n" {
		t.Fatalf("<ours> = %q, want the freshly regenerated content", got)
	}
}

func TestMergeDriverIndex_RegenerationFailureLeavesOursUntouchedAndFailsClosed(t *testing.T) {
	root := t.TempDir()
	writeSpecscoreYAML(t, root, "")
	withCwd(t, root)

	stubLintFix(t, func(lint.Options) (lint.Result, error) {
		return lint.Result{}, errors.New("boom: malformed source artifact")
	})

	ours := filepath.Join(t.TempDir(), "git-temp-a")
	oursBefore := []byte("ours content before the attempted merge\n")
	if err := os.WriteFile(ours, oursBefore, 0o644); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(t.TempDir(), "git-temp-o")
	theirs := filepath.Join(t.TempDir(), "git-temp-b")
	_ = os.WriteFile(base, nil, 0o644)
	_ = os.WriteFile(theirs, nil, 0o644)

	_, errOut, err := runMergeDriver(t, "index", base, ours, theirs, "spec/lessons/README.md")
	if err == nil {
		t.Fatal("expected an error when regeneration fails; got nil")
	}
	if exitCodeOf(err) != exitcode.Conflict {
		t.Fatalf("exit code = %d, want exitcode.Conflict (%d): %v", exitCodeOf(err), exitcode.Conflict, err)
	}
	if !strings.Contains(errOut, "boom") {
		t.Fatalf("stderr = %q, want it to surface the regeneration error", errOut)
	}
	got, err := os.ReadFile(ours)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(oursBefore) {
		t.Fatalf("<ours> was rewritten despite a failed regeneration:\n before=%q\n after=%q", oursBefore, got)
	}
}

func TestMergeDriverIndexCommand_WrongArgCount(t *testing.T) {
	_, _, err := runMergeDriver(t, "index", "only", "two")
	if err == nil {
		t.Fatal("expected an error for wrong argument count; got nil")
	}
}

func TestMergeDriverIndexCommand_Help(t *testing.T) {
	out, _, err := runMergeDriver(t, "index", "--help")
	if err != nil || !strings.Contains(out, "regenerates it in place") {
		t.Fatalf("index --help = %q, err=%v", out, err)
	}
}

func TestMergeDriverCommand_Help(t *testing.T) {
	out, _, err := runMergeDriver(t, "--help")
	if err != nil || !strings.Contains(out, "install") || !strings.Contains(out, "index") {
		t.Fatalf("help = %q, err=%v", out, err)
	}
}

// --- merge-driver install: error-path coverage ---

func TestMergeDriverInstall_ProjectResolutionErrorIsReturned(t *testing.T) {
	// A plain temp dir has no specscore.yaml anywhere in its ancestry, so
	// resolveEventProjectRoot's findRepoConfigRoot fails closed.
	noProject := t.TempDir()
	_, _, err := runMergeDriver(t, "install", "--project", noProject)
	if err == nil || exitCodeOf(err) != exitcode.NotFound {
		t.Fatalf("install with no specscore.yaml = %v, want NotFound", err)
	}
}

func TestMergeDriverInstall_GitTopLevelErrorIsReturned(t *testing.T) {
	root := t.TempDir()
	writeSpecscoreYAML(t, root, "")
	prev := gitTopLevelFn
	gitTopLevelFn = func(string) (string, error) { return "", errors.New("not a git repo") }
	t.Cleanup(func() { gitTopLevelFn = prev })

	_, _, err := runMergeDriver(t, "install", "--project", root)
	if err == nil || exitCodeOf(err) != exitcode.InvalidState {
		t.Fatalf("install with no git repo = %v, want InvalidState", err)
	}
}

func TestMergeDriverInstall_LedgerConfigErrorIsReturned(t *testing.T) {
	root := t.TempDir()
	// Two configured jsonl sinks make event.ConfiguredLedgerPath ambiguous.
	writeSpecscoreYAML(t, root, "events:\n  subscribers:\n    - type: jsonl\n      path: a.jsonl\n    - type: jsonl\n      path: b.jsonl\n")
	stubGitSeams(t, root)

	_, _, err := runMergeDriver(t, "install", "--project", root)
	if err == nil || exitCodeOf(err) != exitcode.InvalidArgs {
		t.Fatalf("install with ambiguous ledger config = %v, want InvalidArgs", err)
	}
}

func TestMergeDriverInstall_GitattributesWriteErrorIsReturned(t *testing.T) {
	root := t.TempDir()
	writeSpecscoreYAML(t, root, "")
	stubGitSeams(t, root)
	prev := ensureGitattributesFn
	ensureGitattributesFn = func(string, []string) (int, error) { return 0, errors.New("disk full") }
	t.Cleanup(func() { ensureGitattributesFn = prev })

	_, _, err := runMergeDriver(t, "install", "--project", root)
	if err == nil || exitCodeOf(err) != exitcode.Unexpected {
		t.Fatalf("install with .gitattributes write failure = %v, want Unexpected", err)
	}
}

func TestMergeDriverInstall_GitConfigSetErrorIsReturned(t *testing.T) {
	root := t.TempDir()
	writeSpecscoreYAML(t, root, "")
	prevTop := gitTopLevelFn
	gitTopLevelFn = func(string) (string, error) { return root, nil }
	prevCfg := gitConfigSetFn
	gitConfigSetFn = func(string, string, string) error { return errors.New("git config denied") }
	t.Cleanup(func() { gitTopLevelFn, gitConfigSetFn = prevTop, prevCfg })

	_, _, err := runMergeDriver(t, "install", "--project", root)
	if err == nil || exitCodeOf(err) != exitcode.Unexpected {
		t.Fatalf("install with git config failure = %v, want Unexpected", err)
	}
}

// --- ensureGitattributes: direct unit tests for the paths install()'s
// happy-path tests above don't exercise ---

func TestEnsureGitattributes_ReadErrorOtherThanNotExist(t *testing.T) {
	// os.ReadFile on a directory fails with an error that is NOT
	// os.ErrNotExist, exercising the distinct "real read failure" branch.
	dir := t.TempDir()
	_, err := ensureGitattributes(dir, []string{"a merge=b"})
	if err == nil {
		t.Fatal("expected an error reading a directory as a file; got nil")
	}
}

func TestEnsureGitattributes_AppendsNewlineWhenMissingTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")
	if err := os.WriteFile(path, []byte("*.png binary"), 0o644); err != nil { // no trailing "\n"
		t.Fatal(err)
	}
	added, err := ensureGitattributes(path, []string{"a merge=b"})
	if err != nil {
		t.Fatalf("ensureGitattributes: %v", err)
	}
	if added != 1 {
		t.Fatalf("added = %d, want 1", added)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "*.png binary\na merge=b\n" {
		t.Fatalf(".gitattributes = %q, want a newline inserted before the new line", got)
	}
}

func TestEnsureGitattributes_WriteErrorIsReturned(t *testing.T) {
	// A path inside a nonexistent parent directory makes os.WriteFile fail;
	// ensureGitattributes does not create parent directories.
	path := filepath.Join(t.TempDir(), "nonexistent-subdir", ".gitattributes")
	_, err := ensureGitattributes(path, []string{"a merge=b"})
	if err == nil {
		t.Fatal("expected a write error for a missing parent directory; got nil")
	}
}

// --- merge-driver index: error-path coverage ---

func TestMergeDriverIndex_GetwdErrorIsReturned(t *testing.T) {
	prev := osGetwdFn
	osGetwdFn = func() (string, error) { return "", errors.New("getwd boom") }
	t.Cleanup(func() { osGetwdFn = prev })

	_, _, err := runMergeDriver(t, "index", "o", "a", "b", "spec/features/README.md")
	if err == nil || exitCodeOf(err) != exitcode.Unexpected {
		t.Fatalf("index with getwd failure = %v, want Unexpected", err)
	}
}

func TestMergeDriverIndex_ProjectRootNotFoundIsReturned(t *testing.T) {
	// A CWD with no specscore.yaml anywhere in its ancestry.
	withCwd(t, t.TempDir())
	_, _, err := runMergeDriver(t, "index", "o", "a", "b", "spec/features/README.md")
	if err == nil || exitCodeOf(err) != exitcode.NotFound {
		t.Fatalf("index with no specscore.yaml = %v, want NotFound", err)
	}
}

func TestMergeDriverIndex_ReadRegeneratedFileErrorIsReturned(t *testing.T) {
	root := t.TempDir()
	writeSpecscoreYAML(t, root, "")
	withCwd(t, root)
	stubLintFix(t, func(lint.Options) (lint.Result, error) { return lint.Result{}, nil })
	// No spec/features/README.md written on disk, so the post-regeneration
	// read-back fails even though the (stubbed) lint pass "succeeded".

	ours := filepath.Join(t.TempDir(), "git-temp-a")
	if err := os.WriteFile(ours, []byte("unchanged"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errOut, err := runMergeDriver(t, "index", "o", ours, "b", "spec/features/README.md")
	if err == nil || exitCodeOf(err) != exitcode.Conflict {
		t.Fatalf("index with missing regenerated file = %v, want Conflict", err)
	}
	if !strings.Contains(errOut, "spec/features/README.md") {
		t.Fatalf("stderr = %q, want it to name the missing file", errOut)
	}
}

func TestMergeDriverIndex_WriteOursErrorIsReturned(t *testing.T) {
	root := t.TempDir()
	writeSpecscoreYAML(t, root, "")
	withCwd(t, root)
	regenerated := filepath.Join(root, "spec", "features", "README.md")
	if err := os.MkdirAll(filepath.Dir(regenerated), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(regenerated, []byte("fresh"), 0o644); err != nil {
		t.Fatal(err)
	}
	stubLintFix(t, func(lint.Options) (lint.Result, error) { return lint.Result{}, nil })
	prevWrite := osWriteFileFn
	osWriteFileFn = func(string, []byte, os.FileMode) error { return errors.New("disk full") }
	t.Cleanup(func() { osWriteFileFn = prevWrite })

	_, _, err := runMergeDriver(t, "index", "o", "a", "b", "spec/features/README.md")
	if err == nil || exitCodeOf(err) != exitcode.Unexpected {
		t.Fatalf("index with write failure = %v, want Unexpected", err)
	}
}
