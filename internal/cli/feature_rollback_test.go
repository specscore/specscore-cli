package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func rbWrite(t *testing.T, root, rel, body string, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	return p
}

func rbSkipRoot(t *testing.T) {
	t.Helper()
	if os.Getuid() == 0 {
		t.Skip("running as root, permission tests skipped")
	}
}

func TestRestoreSpecFiles_RestoresChangedRemovesNewRecreatesMissing(t *testing.T) {
	root := t.TempDir()
	changed := rbWrite(t, root, "features/README.md", "index v1\n", 0o644)
	same := rbWrite(t, root, "features/a/README.md", "a\n", 0o644)
	gone := rbWrite(t, root, "features/b/README.md", "b\n", 0o600)
	before, err := readSpecFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(changed, []byte("index v2\n"), 0o644)
	_ = os.RemoveAll(filepath.Dir(gone))
	extra := rbWrite(t, root, "features/c/README.md", "new\n", 0o644)
	if err := restoreSpecFiles(root, before); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]string{changed: "index v1\n", same: "a\n", gone: "b\n"} {
		if b, _ := os.ReadFile(p); string(b) != want {
			t.Fatalf("%s = %q, want %q", p, b, want)
		}
	}
	if info, _ := os.Stat(gone); info.Mode().Perm() != 0o600 {
		t.Fatalf("recreated file mode = %v, want 0600", info.Mode().Perm())
	}
	if _, err := os.Stat(extra); !os.IsNotExist(err) {
		t.Fatal("a file created after the snapshot must be removed")
	}
}

func TestReadSpecFiles_UnreadableFileFails(t *testing.T) {
	rbSkipRoot(t)
	root := t.TempDir()
	p := rbWrite(t, root, "x.md", "x", 0o000)
	defer func() { _ = os.Chmod(p, 0o644) }()
	if _, err := readSpecFiles(root); err == nil {
		t.Fatal("an unreadable file must fail the snapshot")
	}
}

func TestRestoreSpecFiles_Errors(t *testing.T) {
	rbSkipRoot(t)
	cases := map[string]func(t *testing.T, root string) (map[string]specFileState, func()){
		"walk": func(t *testing.T, root string) (map[string]specFileState, func()) {
			locked := filepath.Join(root, "locked")
			_ = os.MkdirAll(locked, 0o755)
			_ = os.Chmod(locked, 0o000)
			return map[string]specFileState{}, func() { _ = os.Chmod(locked, 0o755) }
		},
		"remove": func(t *testing.T, root string) (map[string]specFileState, func()) {
			rbWrite(t, root, "ro/new.md", "x", 0o644)
			dir := filepath.Join(root, "ro")
			_ = os.Chmod(dir, 0o555)
			return map[string]specFileState{}, func() { _ = os.Chmod(dir, 0o755) }
		},
		"read": func(t *testing.T, root string) (map[string]specFileState, func()) {
			p := rbWrite(t, root, "f.md", "x", 0o000)
			return map[string]specFileState{"f.md": {content: []byte("y"), mode: 0o644}}, func() { _ = os.Chmod(p, 0o644) }
		},
		"rewrite": func(t *testing.T, root string) (map[string]specFileState, func()) {
			p := rbWrite(t, root, "f.md", "x", 0o444)
			return map[string]specFileState{"f.md": {content: []byte("y"), mode: 0o644}}, func() { _ = os.Chmod(p, 0o644) }
		},
		"mkdir": func(t *testing.T, root string) (map[string]specFileState, func()) {
			rbWrite(t, root, "blocker", "file in the way", 0o644)
			return map[string]specFileState{
				"blocker":      {content: []byte("file in the way"), mode: 0o644},
				"blocker/x.md": {content: []byte("x"), mode: 0o644},
			}, func() {}
		},
		"recreate": func(t *testing.T, root string) (map[string]specFileState, func()) {
			dir := filepath.Join(root, "ro")
			_ = os.MkdirAll(dir, 0o555)
			return map[string]specFileState{"ro/x.md": {content: []byte("x"), mode: 0o644}}, func() { _ = os.Chmod(dir, 0o755) }
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			before, cleanup := setup(t, root)
			defer cleanup()
			if err := restoreSpecFiles(root, before); err == nil {
				t.Fatal("expected restore to fail")
			}
		})
	}
}

func TestFeatureChangeStatus_UnreadableSpecFileRefusesBeforeChange(t *testing.T) {
	rbSkipRoot(t)
	root := slInitRepo(t, false)
	p := rbWrite(t, root, "spec/notes/locked.md", "x", 0o000)
	defer func() { _ = os.Chmod(p, 0o644) }()
	readme := filepath.Join(root, "spec", "features", "search", "README.md")
	orig, _ := os.ReadFile(readme)
	_, _, err := runFeature(t, "change-status", "search", "--to", "In Review")
	if err == nil || !strings.Contains(err.Error(), "reading spec tree before change-status") {
		t.Fatalf("expected the snapshot failure, got %v", err)
	}
	if b, _ := os.ReadFile(readme); string(b) != string(orig) {
		t.Fatal("nothing may change when the snapshot fails")
	}
}
