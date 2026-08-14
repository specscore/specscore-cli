package dryrun

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// resetFileOps restores the production file operations after a test exercises
// an otherwise hard-to-trigger operating-system failure. These tests must not
// use t.Parallel while a seam is overridden.
func resetFileOps(t *testing.T) {
	t.Helper()
	mkdirTemp, removeAll := dryrunMkdirTemp, dryrunRemoveAll
	stat, readFile, writeFile := dryrunStat, dryrunReadFile, dryrunWriteFile
	mkdirAll, openFile := dryrunMkdirAll, dryrunOpenFile
	open, copyFileOp := dryrunOpen, dryrunCopy
	walkDir, rel := dryrunWalkDir, dryrunRel
	t.Cleanup(func() {
		dryrunMkdirTemp, dryrunRemoveAll = mkdirTemp, removeAll
		dryrunStat, dryrunReadFile, dryrunWriteFile = stat, readFile, writeFile
		dryrunMkdirAll, dryrunOpenFile = mkdirAll, openFile
		dryrunOpen, dryrunCopy = open, copyFileOp
		dryrunWalkDir, dryrunRel = walkDir, rel
	})
}

type infoErrorEntry struct{ directory bool }

func (entry infoErrorEntry) Name() string         { return "entry" }
func (entry infoErrorEntry) IsDir() bool          { return entry.directory }
func (entry infoErrorEntry) Type() fs.FileMode    { return 0 }
func (infoErrorEntry) Info() (fs.FileInfo, error) { return nil, errors.New("entry info") }

// writeTree materializes a small spec/ tree under root for test fixtures.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(p), err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
}

// snapshotTree returns every regular file under root plus its content, so a
// test can assert byte-identity before/after an operation.
func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return out
}

func TestSandbox_LeavesRealRootUntouched(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"spec/ideas/foo.md":    "**Status:** Draft\n",
		"spec/ideas/README.md": "index\n",
	})
	before := snapshotTree(t, root)

	_, _, err := Sandbox(root, func(sandboxRoot string) (struct{}, error) {
		p := filepath.Join(sandboxRoot, "spec", "ideas", "foo.md")
		if err := os.WriteFile(p, []byte("**Status:** Approved\n"), 0o644); err != nil {
			return struct{}{}, err
		}
		// Also add and remove a file inside the sandbox, to exercise every
		// Change kind.
		newP := filepath.Join(sandboxRoot, "spec", "ideas", "bar.md")
		if err := os.WriteFile(newP, []byte("new\n"), 0o644); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, os.Remove(filepath.Join(sandboxRoot, "spec", "ideas", "README.md"))
	})
	if err != nil {
		t.Fatalf("Sandbox returned error: %v", err)
	}

	after := snapshotTree(t, root)
	if len(before) != len(after) {
		t.Fatalf("real root file count changed: before=%d after=%d", len(before), len(after))
	}
	for rel, content := range before {
		if after[rel] != content {
			t.Fatalf("real root file %s was mutated: before=%q after=%q", rel, content, after[rel])
		}
	}
}

func TestSandbox_ReportsExactChanges(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"spec/ideas/foo.md":    "**Status:** Draft\n",
		"spec/ideas/README.md": "index\n",
		"spec/ideas/keep.md":   "unchanged\n",
	})

	_, changes, err := Sandbox(root, func(sandboxRoot string) (struct{}, error) {
		if err := os.WriteFile(filepath.Join(sandboxRoot, "spec", "ideas", "foo.md"), []byte("**Status:** Approved\n"), 0o644); err != nil {
			return struct{}{}, err
		}
		if err := os.WriteFile(filepath.Join(sandboxRoot, "spec", "ideas", "bar.md"), []byte("new\n"), 0o644); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, os.Remove(filepath.Join(sandboxRoot, "spec", "ideas", "README.md"))
	})
	if err != nil {
		t.Fatalf("Sandbox returned error: %v", err)
	}

	got := make([]string, len(changes))
	for i, c := range changes {
		got[i] = c.String()
	}
	sort.Strings(got)
	want := []string{
		"A spec/ideas/bar.md",
		"D spec/ideas/README.md",
		"M spec/ideas/foo.md",
	}
	if len(got) != len(want) {
		t.Fatalf("changes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("changes = %v, want %v", got, want)
		}
	}
	// keep.md must NOT appear — it was untouched.
	for _, c := range changes {
		if c.Path == "spec/ideas/keep.md" {
			t.Fatalf("unchanged file reported as a change: %+v", c)
		}
	}
}

func TestSandbox_ErrorPathRewritesSandboxRootAndPreservesExitCode(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"spec/ideas/foo.md": "**Status:** Draft\n"})

	_, changes, err := Sandbox(root, func(sandboxRoot string) (struct{}, error) {
		return struct{}{}, exitcode.NotFoundErrorf("idea not found at %s", filepath.Join(sandboxRoot, "spec", "ideas", "missing.md"))
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if changes != nil {
		t.Fatalf("expected nil changes on error, got %v", changes)
	}
	type exitCoder interface{ ExitCode() int }
	ec, ok := err.(exitCoder)
	if !ok {
		t.Fatalf("error does not carry an exit code: %v", err)
	}
	if ec.ExitCode() != exitcode.NotFound {
		t.Fatalf("exit code = %d, want %d", ec.ExitCode(), exitcode.NotFound)
	}
	wantMsg := "idea not found at " + filepath.Join(root, "spec", "ideas", "missing.md")
	if err.Error() != wantMsg {
		t.Fatalf("error message = %q, want %q (sandbox path must be rewritten to the real root)", err.Error(), wantMsg)
	}
}

func TestSandbox_NoChangesWhenMutateIsReadOnly(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"spec/ideas/foo.md": "**Status:** Draft\n"})

	_, changes, err := Sandbox(root, func(sandboxRoot string) (string, error) {
		b, err := os.ReadFile(filepath.Join(sandboxRoot, "spec", "ideas", "foo.md"))
		return string(b), err
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("expected no changes, got %v", changes)
	}
}

func TestSandbox_MissingSpecDirIsNotAnError(t *testing.T) {
	root := t.TempDir() // no spec/ subdirectory at all

	_, changes, err := Sandbox(root, func(sandboxRoot string) (struct{}, error) {
		p := filepath.Join(sandboxRoot, "spec", "ideas", "foo.md")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, os.WriteFile(p, []byte("new\n"), 0o644)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changes) != 1 || changes[0].Kind != Added || changes[0].Path != "spec/ideas/foo.md" {
		t.Fatalf("changes = %v, want single Added spec/ideas/foo.md", changes)
	}
	// The real root must still have no spec/ directory.
	if _, err := os.Stat(filepath.Join(root, "spec")); !os.IsNotExist(err) {
		t.Fatalf("real root spec/ directory should not have been created, stat err = %v", err)
	}
}

func TestPrintReport(t *testing.T) {
	var buf strings.Builder
	PrintReport(&buf, "foo", "Draft", "Approved", []Change{
		{Kind: Modified, Path: "spec/ideas/foo.md"},
		{Kind: Modified, Path: "spec/ideas/README.md"},
	})
	want := "foo: Draft → Approved (dry-run; would touch 2 file(s))\n" +
		"  M spec/ideas/foo.md\n" +
		"  M spec/ideas/README.md\n"
	if buf.String() != want {
		t.Fatalf("PrintReport output = %q, want %q", buf.String(), want)
	}
}

func TestSandbox_StagingFailures(t *testing.T) {
	t.Run("temporary directory unavailable", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("TMPDIR", filepath.Join(root, "missing"))
		if _, _, err := Sandbox(root, func(string) (struct{}, error) { return struct{}{}, nil }); err == nil {
			t.Fatal("Sandbox succeeded with an unavailable temporary directory")
		}
	})
	t.Run("spec is a file", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "spec"), []byte("not a directory"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := Sandbox(root, func(string) (struct{}, error) { return struct{}{}, nil }); err == nil {
			t.Fatal("Sandbox accepted a file as spec/")
		}
	})
	t.Run("configuration is a directory", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, map[string]string{"spec/ideas/a.md": "a\n"})
		if err := os.Mkdir(filepath.Join(root, "specscore.yaml"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, _, err := Sandbox(root, func(string) (struct{}, error) { return struct{}{}, nil }); err == nil {
			t.Fatal("Sandbox accepted a directory as specscore.yaml")
		}
	})
	t.Run("mutation leaves an unreadable changed file", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, map[string]string{"spec/ideas/a.md": "a\n"})
		if _, _, err := Sandbox(root, func(sandbox string) (struct{}, error) {
			path := filepath.Join(sandbox, "spec", "ideas", "a.md")
			return struct{}{}, os.Chmod(path, 0)
		}); err == nil {
			t.Fatal("Sandbox accepted an unreadable changed file")
		}
	})
}

func TestCopyHelpersAndPathRewriteFailures(t *testing.T) {
	root := t.TempDir()
	plain := filepath.Join(root, "plain")
	if err := os.WriteFile(plain, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyTree(plain, filepath.Join(root, "dst")); err == nil {
		t.Fatal("copyTree accepted a file")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var entry os.DirEntry
	for _, candidate := range entries {
		if candidate.Name() == "plain" {
			entry = candidate
		}
	}
	if entry == nil {
		t.Fatal("missing test entry")
	}
	if err := copyFile(filepath.Join(root, "missing"), filepath.Join(root, "out"), entry); err == nil {
		t.Fatal("copyFile accepted a missing source")
	}
	if err := copyFile(plain, filepath.Join(plain, "child"), entry); err == nil {
		t.Fatal("copyFile accepted a destination below a file")
	}
	if got := rewriteSandboxPath(errors.New("at /sandbox/path"), "/sandbox", "/real"); got.Error() != "at /real/path" {
		t.Fatalf("rewritten plain error = %q", got)
	}
	original := errors.New("unchanged")
	if got := rewriteSandboxPath(original, "/sandbox", "/real"); got != original {
		t.Fatal("path-free error should be returned unchanged")
	}
}

func TestSandboxAndHelpers_SurfaceFileOperationFailures(t *testing.T) {
	t.Run("sandbox temporary directory error", func(t *testing.T) {
		resetFileOps(t)
		dryrunMkdirTemp = func(string, string) (string, error) { return "", errors.New("no temp") }
		if _, _, err := Sandbox(t.TempDir(), func(string) (struct{}, error) { return struct{}{}, nil }); err == nil {
			t.Fatal("Sandbox succeeded")
		}
	})

	t.Run("project config failures", func(t *testing.T) {
		root := t.TempDir()
		config := filepath.Join(root, "specscore.yaml")
		if err := os.WriteFile(config, []byte("name: test\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		for name, setFailure := range map[string]func(){
			"stat":  func() { dryrunStat = func(string) (os.FileInfo, error) { return nil, errors.New("stat") } },
			"read":  func() { dryrunReadFile = func(string) ([]byte, error) { return nil, errors.New("read") } },
			"write": func() { dryrunWriteFile = func(string, []byte, os.FileMode) error { return errors.New("write") } },
		} {
			t.Run(name, func(t *testing.T) {
				resetFileOps(t)
				setFailure()
				if err := copyProjectConfig(root, t.TempDir()); err == nil {
					t.Fatal("copyProjectConfig succeeded")
				}
			})
		}
	})

	t.Run("tree staging failures", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, map[string]string{"source/file.md": "content\n"})
		src := filepath.Join(root, "source")
		entry, err := os.ReadDir(src)
		if err != nil || len(entry) != 1 {
			t.Fatalf("ReadDir = %v, %v", entry, err)
		}
		for name, setFailure := range map[string]func(){
			"stat": func() { dryrunStat = func(string) (os.FileInfo, error) { return nil, errors.New("stat") } },
			"walk": func() { dryrunWalkDir = func(string, fs.WalkDirFunc) error { return errors.New("walk") } },
			"walk callback": func() {
				dryrunWalkDir = func(path string, visit fs.WalkDirFunc) error { return visit(path, nil, errors.New("walk callback")) }
			},
			"relative path": func() {
				dryrunWalkDir = func(path string, visit fs.WalkDirFunc) error { return visit(path, entry[0], nil) }
				dryrunRel = func(string, string) (string, error) { return "", errors.New("rel") }
			},
			"directory info": func() {
				dryrunWalkDir = func(path string, visit fs.WalkDirFunc) error {
					return visit(path, infoErrorEntry{directory: true}, nil)
				}
			},
			"mkdir": func() {
				dryrunWalkDir = func(path string, visit fs.WalkDirFunc) error { return visit(path, entry[0], nil) }
				dryrunMkdirAll = func(string, os.FileMode) error { return errors.New("mkdir") }
			},
		} {
			t.Run(name, func(t *testing.T) {
				resetFileOps(t)
				setFailure()
				if err := copyTree(src, filepath.Join(root, "destination")); err == nil {
					t.Fatal("copyTree succeeded")
				}
			})
		}
	})

	t.Run("file staging failures", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, map[string]string{"source/file.md": "content\n"})
		src := filepath.Join(root, "source", "file.md")
		entries, err := os.ReadDir(filepath.Dir(src))
		if err != nil || len(entries) != 1 {
			t.Fatalf("ReadDir = %v, %v", entries, err)
		}
		entry := entries[0]
		for name, setFailure := range map[string]func(){
			"entry info":  func() {},
			"mkdir":       func() { dryrunMkdirAll = func(string, os.FileMode) error { return errors.New("mkdir") } },
			"open source": func() { dryrunOpen = func(string) (*os.File, error) { return nil, errors.New("open") } },
			"open destination": func() {
				dryrunOpenFile = func(string, int, os.FileMode) (*os.File, error) { return nil, errors.New("open destination") }
			},
			"copy": func() { dryrunCopy = func(io.Writer, io.Reader) (int64, error) { return 0, errors.New("copy") } },
		} {
			t.Run(name, func(t *testing.T) {
				resetFileOps(t)
				setFailure()
				fileEntry := entry
				if name == "entry info" {
					fileEntry = infoErrorEntry{}
				}
				if err := copyFile(src, filepath.Join(root, "destination", "file.md"), fileEntry); err == nil {
					t.Fatal("copyFile succeeded")
				}
			})
		}
	})

	t.Run("diff failures", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, map[string]string{"old/a.md": "old\n", "new/a.md": "new\n"})
		oldDir, newDir := filepath.Join(root, "old"), filepath.Join(root, "new")
		for name, setFailure := range map[string]func(){
			"listing": func() { dryrunStat = func(string) (os.FileInfo, error) { return nil, errors.New("stat") } },
			"listing new tree": func() {
				calls := 0
				dryrunWalkDir = func(path string, visit fs.WalkDirFunc) error {
					calls++
					if calls == 2 {
						return errors.New("new walk")
					}
					return filepath.WalkDir(path, visit)
				}
			},
			"reading old": func() { dryrunReadFile = func(string) ([]byte, error) { return nil, errors.New("read old") } },
			"reading new": func() {
				calls := 0
				dryrunReadFile = func(path string) ([]byte, error) {
					calls++
					if calls == 2 {
						return nil, errors.New("read new")
					}
					return os.ReadFile(path)
				}
			},
		} {
			t.Run(name, func(t *testing.T) {
				resetFileOps(t)
				setFailure()
				if _, err := diffTrees(oldDir, newDir); err == nil {
					t.Fatal("diffTrees succeeded")
				}
			})
		}
	})

	t.Run("file listing failures", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, map[string]string{"tree/file.md": "content\n"})
		dir := filepath.Join(root, "tree")
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 1 {
			t.Fatalf("ReadDir = %v, %v", entries, err)
		}
		for name, setFailure := range map[string]func(){
			"stat": func() { dryrunStat = func(string) (os.FileInfo, error) { return nil, errors.New("stat") } },
			"walk": func() { dryrunWalkDir = func(string, fs.WalkDirFunc) error { return errors.New("walk") } },
			"walk callback": func() {
				dryrunWalkDir = func(path string, visit fs.WalkDirFunc) error { return visit(path, nil, errors.New("walk callback")) }
			},
			"relative path": func() {
				dryrunWalkDir = func(path string, visit fs.WalkDirFunc) error { return visit(path, entries[0], nil) }
				dryrunRel = func(string, string) (string, error) { return "", errors.New("rel") }
			},
		} {
			t.Run(name, func(t *testing.T) {
				resetFileOps(t)
				setFailure()
				if _, err := listFiles(dir); err == nil {
					t.Fatal("listFiles succeeded")
				}
			})
		}
	})
}
