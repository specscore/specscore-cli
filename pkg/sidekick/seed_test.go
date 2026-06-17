package sidekick

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// writeFile is a tiny test helper that mkdir-ps the parent and writes content.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestResolveSeedPath_existing(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "spec", "ideas", "seeds", "foo.md")
	writeFile(t, want, "---\ncaptured_by: user\nstatus: queued\n---\n# Foo\n")

	got, err := ResolveSeedPath(root, "foo")
	if err != nil {
		t.Fatalf("ResolveSeedPath: unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("ResolveSeedPath = %q, want %q", got, want)
	}
}

func TestResolveSeedPath_missing_NotFound(t *testing.T) {
	root := t.TempDir()

	_, err := ResolveSeedPath(root, "nope")
	if err == nil {
		t.Fatal("ResolveSeedPath: expected NotFound error, got nil")
	}
	var ec *exitcode.Error
	if !asExitErr(err, &ec) {
		t.Fatalf("ResolveSeedPath: error is not *exitcode.Error: %T", err)
	}
	if ec.ExitCode() != exitcode.NotFound {
		t.Fatalf("ResolveSeedPath: exit code = %d, want %d (NotFound)", ec.ExitCode(), exitcode.NotFound)
	}
	expectedPath := filepath.Join("spec", "ideas", "seeds", "nope.md")
	if !strings.Contains(err.Error(), expectedPath) {
		t.Fatalf("ResolveSeedPath: error %q does not name expected path %q", err.Error(), expectedPath)
	}
}

// A file present ONLY under spec/ideas/archived/ MUST NOT be matched: the
// canonical lookup is the seeds/ path only.
func TestResolveSeedPath_archivedNotMatched(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "spec", "ideas", "archived", "foo.md"),
		"---\ncaptured_by: user\nstatus: Implemented\ntype: sidekick-seed\n---\n# Foo\n")

	_, err := ResolveSeedPath(root, "foo")
	if err == nil {
		t.Fatal("ResolveSeedPath: archived-only seed must not be matched, got nil error")
	}
	var ec *exitcode.Error
	if !asExitErr(err, &ec) || ec.ExitCode() != exitcode.NotFound {
		t.Fatalf("ResolveSeedPath: expected NotFound for archived-only seed, got %v", err)
	}
}

func TestRewriteFrontmatterStatus_preservesEveryOtherByte(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "spec", "ideas", "seeds", "foo.md")
	original := "---\ncaptured_by: alice\nstatus: queued\n---\n# Foo the idea\n\nSome body text.\n"
	writeFile(t, path, original)

	orig, err := RewriteFrontmatterStatus(path, "Implemented")
	if err != nil {
		t.Fatalf("RewriteFrontmatterStatus: unexpected error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	want := "---\ncaptured_by: alice\nstatus: Implemented\n---\n# Foo the idea\n\nSome body text.\n"
	if string(got) != want {
		t.Fatalf("RewriteFrontmatterStatus produced:\n%q\nwant:\n%q", string(got), want)
	}

	// The returned original line must round-trip via the unchanged-byte
	// guarantee: it should carry the prior value.
	if !strings.Contains(orig, "queued") {
		t.Fatalf("returned original line %q does not carry prior value", orig)
	}
}

func TestRewriteFrontmatterStatus_noFrontmatterStatus(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "spec", "ideas", "seeds", "foo.md")
	writeFile(t, path, "---\ncaptured_by: alice\n---\n# Foo\n")

	_, err := RewriteFrontmatterStatus(path, "Implemented")
	if err == nil {
		t.Fatal("RewriteFrontmatterStatus: expected error for missing status: key, got nil")
	}
}

// asExitErr is a local errors.As wrapper to keep the test imports tidy.
func asExitErr(err error, target **exitcode.Error) bool {
	for err != nil {
		if e, ok := err.(*exitcode.Error); ok {
			*target = e
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
