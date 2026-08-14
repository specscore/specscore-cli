package dryrun

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

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
