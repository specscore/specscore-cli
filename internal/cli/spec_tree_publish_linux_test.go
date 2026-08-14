//go:build linux

package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestPublishSpecTreeNoReplace_LinuxAtomicExchangeBranches(t *testing.T) {
	reset := func(t *testing.T) {
		t.Helper()
		openParent, closeFD, exchange := publishLinuxOpenParent, publishLinuxCloseFD, publishLinuxExchange
		t.Cleanup(func() {
			publishLinuxOpenParent, publishLinuxCloseFD, publishLinuxExchange = openParent, closeFD, exchange
		})
	}
	t.Run("stage must be sibling", func(t *testing.T) {
		if _, err := publishSpecTreeNoReplace(filepath.Join(t.TempDir(), "spec"), t.TempDir()); err == nil || !contains(err, "must be a sibling") {
			t.Fatalf("publish error = %v", err)
		}
	})
	t.Run("parent open failure", func(t *testing.T) {
		reset(t)
		publishLinuxOpenParent = func(string, int, uint32) (int, error) { return -1, errors.New("open parent failed") }
		parent := t.TempDir()
		if _, err := publishSpecTreeNoReplace(filepath.Join(parent, "spec"), filepath.Join(parent, "stage")); err == nil || !contains(err, "open parent failed") {
			t.Fatalf("publish error = %v", err)
		}
	})
	t.Run("exchange failure preserves stage", func(t *testing.T) {
		reset(t)
		parent := t.TempDir()
		spec, stage := filepath.Join(parent, "spec"), filepath.Join(parent, "stage")
		if err := os.Mkdir(spec, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(stage, 0o755); err != nil {
			t.Fatal(err)
		}
		publishLinuxExchange = func(int, string, int, string, uint) error { return errors.New("exchange failed") }
		if _, err := publishSpecTreeNoReplace(spec, stage); err == nil || !contains(err, "exchange failed") {
			t.Fatalf("publish error = %v", err)
		}
		if info, err := os.Stat(stage); err != nil || !info.IsDir() {
			t.Fatalf("stage after exchange failure: info=%v err=%v", info, err)
		}
	})
	t.Run("actual exchange", func(t *testing.T) {
		reset(t)
		parent := t.TempDir()
		spec, stage := filepath.Join(parent, "spec"), filepath.Join(parent, "stage")
		for _, path := range []string{spec, stage} {
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(spec, "before.md"), []byte("before"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(stage, "after.md"), []byte("after"), 0o644); err != nil {
			t.Fatal(err)
		}
		oldPath, err := publishSpecTreeNoReplace(spec, stage)
		if err != nil {
			t.Fatal(err)
		}
		if oldPath != stage {
			t.Fatalf("old path = %q, want %q", oldPath, stage)
		}
		if got, err := os.ReadFile(filepath.Join(spec, "after.md")); err != nil || string(got) != "after" {
			t.Fatalf("published output = %q, %v", got, err)
		}
	})
}

func TestSetStagedEntryModificationTime_LinuxError(t *testing.T) {
	original := stageLinuxUtimesNanoAt
	stageLinuxUtimesNanoAt = func(int, string, []unix.Timespec, int) error { return errors.New("utimensat failed") }
	t.Cleanup(func() { stageLinuxUtimesNanoAt = original })
	if err := setStagedEntryModificationTime(-1, nowForLifecycleTimeTest()); err == nil || !contains(err, "utimensat failed") {
		t.Fatalf("timestamp error = %v", err)
	}
	if err := applyStagedEntryMetadata(-1, specTreeEntryMetadata{modificationTime: nowForLifecycleTimeTest()}); err == nil || !contains(err, "utimensat failed") {
		t.Fatalf("metadata timestamp error = %v", err)
	}
}
