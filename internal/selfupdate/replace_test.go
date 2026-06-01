package selfupdate

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReplaceExecutableSwapsNewBytes(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "specscore")
	if err := os.WriteFile(target, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	newBin := filepath.Join(dir, "new-source")
	if err := os.WriteFile(newBin, []byte("new binary"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ReplaceExecutable(target, newBin); err != nil {
		t.Fatalf("ReplaceExecutable: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new binary" {
		t.Fatalf("target bytes = %q, want %q", got, "new binary")
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("target is not executable: mode %v", info.Mode())
	}

	// No leftover staging temp file should remain in the directory.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if name == "specscore" || name == "new-source" {
			continue
		}
		t.Fatalf("unexpected leftover file in dir: %q", name)
	}
}

func TestReplaceExecutableAtomicOnStagingFailure(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "specscore")
	if err := os.WriteFile(target, []byte("original bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Source does not exist, so staging (copy) must fail before any rename.
	missing := filepath.Join(dir, "does-not-exist")

	if err := ReplaceExecutable(target, missing); err == nil {
		t.Fatal("expected error when new binary source is missing, got nil")
	}

	// Original target must remain intact and complete.
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("target missing after failed swap: %v", err)
	}
	if string(got) != "original bytes" {
		t.Fatalf("target corrupted after failed swap: got %q, want %q", got, "original bytes")
	}

	// No leftover staging temp file.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "specscore" {
			t.Fatalf("unexpected leftover file in dir: %q", e.Name())
		}
	}
}

func TestVerifyBinaryVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake binary not portable to windows")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "fake")
	script := "#!/bin/sh\necho \"specscore version 1.2.3\"\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := VerifyBinaryVersion(bin, "1.2.3"); err != nil {
		t.Fatalf("VerifyBinaryVersion match: %v", err)
	}

	err := VerifyBinaryVersion(bin, "9.9.9")
	if err == nil {
		t.Fatal("expected mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "9.9.9") {
		t.Fatalf("mismatch error should mention wanted version: %v", err)
	}
}
