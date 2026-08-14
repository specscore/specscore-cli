//go:build darwin || linux

package cli

import (
	"errors"
	"io"
	"testing"
)

func resetStageUnixSeams(t *testing.T) {
	t.Helper()
	open, closeFD, fchdir, fchmod := stageUnixOpen, stageUnixClose, stageUnixFchdir, stageUnixFchmod
	mkdirat, openat, dup, write := stageUnixMkdirat, stageUnixOpenat, stageUnixDup, stageUnixWrite
	t.Cleanup(func() {
		stageUnixOpen, stageUnixClose, stageUnixFchdir, stageUnixFchmod = open, closeFD, fchdir, fchmod
		stageUnixMkdirat, stageUnixOpenat, stageUnixDup, stageUnixWrite = mkdirat, openat, dup, write
	})
}

func TestStagedSpecTree_DescriptorFailureBranches(t *testing.T) {
	newStage := func(t *testing.T) *stagedSpecTree {
		t.Helper()
		stage, err := openStagedSpecTreeNoFollow(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = closeStagedSpecTree(stage) })
		return stage
	}
	withStage := func(t *testing.T, run func(*stagedSpecTree)) {
		t.Helper()
		stage := newStage(t)
		resetStageUnixSeams(t)
		run(stage)
	}
	fileSnapshot := func() specTreeSnapshot { return rootSnapshot(map[string]string{"file.md": "body"}) }
	dirSnapshot := func() specTreeSnapshot { return rootSnapshot(nil, "nested") }

	t.Run("open and lint descriptor failures", func(t *testing.T) {
		resetStageUnixSeams(t)
		stageUnixOpen = func(string, int, uint32) (int, error) { return -1, errors.New("open failed") }
		if _, err := openStagedSpecTreeNoFollow(t.TempDir()); err == nil || !contains(err, "open failed") {
			t.Fatalf("stage open error = %v", err)
		}
		if err := runLintInStagedSpecTree(nil, func(string) error { return nil }); err == nil || !contains(err, "closed") {
			t.Fatalf("closed lint error = %v", err)
		}
		if err := closeStagedSpecTree(nil); err != nil {
			t.Fatalf("close nil stage: %v", err)
		}
	})
	t.Run("lint current-directory and handoff failures", func(t *testing.T) {
		withStage(t, func(stage *stagedSpecTree) {
			originalOpen := stageUnixOpen
			stageUnixOpen = func(string, int, uint32) (int, error) { return -1, errors.New("cwd open failed") }
			if err := runLintInStagedSpecTree(stage, func(string) error { return nil }); err == nil || !contains(err, "cwd open failed") {
				t.Fatalf("cwd open error = %v", err)
			}
			stageUnixOpen = originalOpen
		})
		withStage(t, func(stage *stagedSpecTree) {
			originalFchdir := stageUnixFchdir
			stageUnixFchdir = func(int) error { return errors.New("fchdir failed") }
			if err := runLintInStagedSpecTree(stage, func(string) error { return nil }); err == nil || !contains(err, "fchdir failed") {
				t.Fatalf("handoff error = %v", err)
			}
			stageUnixFchdir = originalFchdir
		})
		withStage(t, func(stage *stagedSpecTree) {
			originalFchdir := stageUnixFchdir
			calls := 0
			stageUnixFchdir = func(fd int) error {
				calls++
				if calls == 2 {
					return errors.New("restore after success failed")
				}
				return originalFchdir(fd)
			}
			if err := runLintInStagedSpecTree(stage, func(string) error { return nil }); err == nil || !contains(err, "restore after success failed") {
				t.Fatalf("restore after success error = %v", err)
			}
			stageUnixFchdir = originalFchdir
		})
		withStage(t, func(stage *stagedSpecTree) {
			originalFchdir := stageUnixFchdir
			calls := 0
			stageUnixFchdir = func(fd int) error {
				calls++
				if calls == 2 {
					return errors.New("restore failed")
				}
				return originalFchdir(fd)
			}
			if err := runLintInStagedSpecTree(stage, func(string) error { return errors.New("lint failed") }); err == nil || !contains(err, "restore failed") || !contains(err, "lint failed") {
				t.Fatalf("restore after lint error = %v", err)
			}
			stageUnixFchdir = originalFchdir
		})
	})
	t.Run("materializer root and directory failures", func(t *testing.T) {
		withStage(t, func(stage *stagedSpecTree) {
			if err := materializeStagedSpecTreeNoFollow(stage, specTreeSnapshot{}); err == nil || !contains(err, "missing") {
				t.Fatalf("invalid snapshot error = %v", err)
			}
		})
		withStage(t, func(stage *stagedSpecTree) {
			originalFchmod := stageUnixFchmod
			stageUnixFchmod = func(int, uint32) error { return errors.New("root chmod failed") }
			if err := materializeStagedSpecTreeNoFollow(stage, rootSnapshot(nil)); err == nil || !contains(err, "root chmod failed") {
				t.Fatalf("root chmod error = %v", err)
			}
			stageUnixFchmod = originalFchmod
		})
		withStage(t, func(stage *stagedSpecTree) {
			originalDup := stageUnixDup
			stageUnixDup = func(int) (int, error) { return -1, errors.New("dup failed") }
			if err := materializeStagedSpecTreeNoFollow(stage, dirSnapshot()); err == nil || !contains(err, "dup failed") {
				t.Fatalf("directory parent error = %v", err)
			}
			stageUnixDup = originalDup
		})
		withStage(t, func(stage *stagedSpecTree) {
			originalMkdirat := stageUnixMkdirat
			stageUnixMkdirat = func(int, string, uint32) error { return errors.New("mkdirat failed") }
			if err := materializeStagedSpecTreeNoFollow(stage, dirSnapshot()); err == nil || !contains(err, "mkdirat failed") {
				t.Fatalf("mkdir error = %v", err)
			}
			stageUnixMkdirat = originalMkdirat
		})
		withStage(t, func(stage *stagedSpecTree) {
			originalOpenat := stageUnixOpenat
			stageUnixOpenat = func(int, string, int, uint32) (int, error) { return -1, errors.New("openat failed") }
			if err := materializeStagedSpecTreeNoFollow(stage, dirSnapshot()); err == nil || !contains(err, "openat failed") {
				t.Fatalf("directory open error = %v", err)
			}
			stageUnixOpenat = originalOpenat
		})
		withStage(t, func(stage *stagedSpecTree) {
			originalFchmod := stageUnixFchmod
			calls := 0
			stageUnixFchmod = func(fd int, mode uint32) error {
				calls++
				if calls == 2 {
					return errors.New("directory chmod failed")
				}
				return originalFchmod(fd, mode)
			}
			if err := materializeStagedSpecTreeNoFollow(stage, dirSnapshot()); err == nil || !contains(err, "directory chmod failed") {
				t.Fatalf("directory chmod error = %v", err)
			}
			stageUnixFchmod = originalFchmod
		})
		withStage(t, func(stage *stagedSpecTree) {
			originalClose := stageUnixClose
			calls := 0
			stageUnixClose = func(fd int) error {
				calls++
				if calls == 2 {
					return errors.New("directory close failed")
				}
				return originalClose(fd)
			}
			if err := materializeStagedSpecTreeNoFollow(stage, dirSnapshot()); err == nil || !contains(err, "directory close failed") {
				t.Fatalf("directory close error = %v", err)
			}
			stageUnixClose = originalClose
		})
	})
	t.Run("materializer file and metadata failures", func(t *testing.T) {
		withStage(t, func(stage *stagedSpecTree) {
			originalDup := stageUnixDup
			stageUnixDup = func(int) (int, error) { return -1, errors.New("file parent failed") }
			if err := materializeStagedSpecTreeNoFollow(stage, fileSnapshot()); err == nil || !contains(err, "file parent failed") {
				t.Fatalf("file parent error = %v", err)
			}
			stageUnixDup = originalDup
		})
		withStage(t, func(stage *stagedSpecTree) {
			originalOpenat := stageUnixOpenat
			stageUnixOpenat = func(int, string, int, uint32) (int, error) { return -1, errors.New("file open failed") }
			if err := materializeStagedSpecTreeNoFollow(stage, fileSnapshot()); err == nil || !contains(err, "file open failed") {
				t.Fatalf("file open error = %v", err)
			}
			stageUnixOpenat = originalOpenat
		})
		withStage(t, func(stage *stagedSpecTree) {
			originalWrite := stageUnixWrite
			stageUnixWrite = func(int, []byte) (int, error) { return 0, errors.New("write failed") }
			if err := materializeStagedSpecTreeNoFollow(stage, fileSnapshot()); err == nil || !contains(err, "write failed") {
				t.Fatalf("file write error = %v", err)
			}
			stageUnixWrite = originalWrite
		})
		withStage(t, func(stage *stagedSpecTree) {
			originalFchmod := stageUnixFchmod
			calls := 0
			stageUnixFchmod = func(fd int, mode uint32) error {
				calls++
				if calls == 2 {
					return errors.New("file chmod failed")
				}
				return originalFchmod(fd, mode)
			}
			if err := materializeStagedSpecTreeNoFollow(stage, fileSnapshot()); err == nil || !contains(err, "file chmod failed") {
				t.Fatalf("file chmod error = %v", err)
			}
			stageUnixFchmod = originalFchmod
		})
		withStage(t, func(stage *stagedSpecTree) {
			originalClose := stageUnixClose
			calls := 0
			stageUnixClose = func(fd int) error {
				calls++
				if calls == 2 {
					return errors.New("file close failed")
				}
				return originalClose(fd)
			}
			if err := materializeStagedSpecTreeNoFollow(stage, fileSnapshot()); err == nil || !contains(err, "file close failed") {
				t.Fatalf("file close error = %v", err)
			}
			stageUnixClose = originalClose
		})
		withStage(t, func(stage *stagedSpecTree) {
			originalWrite := stageUnixWrite
			stageUnixWrite = func(int, []byte) (int, error) { return 0, nil }
			if err := materializeStagedSpecTreeNoFollow(stage, fileSnapshot()); err == nil || !errors.Is(err, io.ErrShortWrite) {
				t.Fatalf("short write error = %v", err)
			}
			stageUnixWrite = originalWrite
		})
		withStage(t, func(stage *stagedSpecTree) {
			resetSnapshotNoFollowSeams(t)
			snapshotFsetxattr = func(int, string, []byte, int) error { return errors.New("setxattr failed") }
			snapshot := fileSnapshot()
			file := snapshot.files["file.md"]
			file.metadata.extendedAttributes = map[string][]byte{"user.test": []byte("x")}
			snapshot.files["file.md"] = file
			if err := materializeStagedSpecTreeNoFollow(stage, snapshot); err == nil || !contains(err, "setxattr failed") {
				t.Fatalf("metadata error = %v", err)
			}
		})
	})
	t.Run("final directory metadata and path helper failures", func(t *testing.T) {
		withStage(t, func(stage *stagedSpecTree) {
			resetSnapshotNoFollowSeams(t)
			snapshotFsetxattr = func(int, string, []byte, int) error { return errors.New("directory metadata failed") }
			snapshot := rootSnapshot(nil)
			directory := snapshot.directories["."]
			directory.metadata.extendedAttributes = map[string][]byte{"user.test": []byte("x")}
			snapshot.directories["."] = directory
			if err := materializeStagedSpecTreeNoFollow(stage, snapshot); err == nil || !contains(err, "directory metadata failed") {
				t.Fatalf("directory metadata error = %v", err)
			}
		})
		withStage(t, func(stage *stagedSpecTree) {
			originalDup := stageUnixDup
			stageUnixDup = func(int) (int, error) { return -1, errors.New("final dup failed") }
			if err := materializeStagedSpecTreeNoFollow(stage, rootSnapshot(nil)); err == nil || !contains(err, "final dup failed") {
				t.Fatalf("final directory open error = %v", err)
			}
			stageUnixDup = originalDup
		})
		withStage(t, func(stage *stagedSpecTree) {
			originalFchmod := stageUnixFchmod
			calls := 0
			stageUnixFchmod = func(fd int, mode uint32) error {
				calls++
				if calls == 2 {
					return errors.New("final chmod failed")
				}
				return originalFchmod(fd, mode)
			}
			if err := materializeStagedSpecTreeNoFollow(stage, rootSnapshot(nil)); err == nil || !contains(err, "final chmod failed") {
				t.Fatalf("final chmod error = %v", err)
			}
			stageUnixFchmod = originalFchmod
		})
		withStage(t, func(stage *stagedSpecTree) {
			originalClose := stageUnixClose
			stageUnixClose = func(int) error { return errors.New("final close failed") }
			if err := materializeStagedSpecTreeNoFollow(stage, rootSnapshot(nil)); err == nil || !contains(err, "final close failed") {
				t.Fatalf("final close error = %v", err)
			}
			stageUnixClose = originalClose
		})
		withStage(t, func(stage *stagedSpecTree) {
			if _, err := openStageDirectoryFD(stage, "../escape"); err == nil || !contains(err, "invalid") {
				t.Fatalf("directory path validation error = %v", err)
			}
			originalDup := stageUnixDup
			stageUnixDup = func(int) (int, error) { return -1, errors.New("open dir dup failed") }
			if _, err := openStageDirectoryFD(stage, "nested"); err == nil || !contains(err, "open dir dup failed") {
				t.Fatalf("open directory dup error = %v", err)
			}
			stageUnixDup = originalDup
			originalOpenat := stageUnixOpenat
			stageUnixOpenat = func(int, string, int, uint32) (int, error) { return -1, errors.New("open dir component failed") }
			if _, err := openStageDirectoryFD(stage, "nested"); err == nil || !contains(err, "open dir component failed") {
				t.Fatalf("open directory component error = %v", err)
			}
			stageUnixOpenat = originalOpenat
			if _, _, err := stageParentDirectoryFD(stage, "../escape"); err == nil || !contains(err, "invalid") {
				t.Fatalf("parent path validation error = %v", err)
			}
			originalOpenat = stageUnixOpenat
			stageUnixOpenat = func(int, string, int, uint32) (int, error) { return -1, errors.New("parent component failed") }
			if _, _, err := stageParentDirectoryFD(stage, "nested/file.md"); err == nil || !contains(err, "parent component failed") {
				t.Fatalf("parent component error = %v", err)
			}
			stageUnixOpenat = originalOpenat
		})
	})
	t.Run("staging helper cleanup and materialization failures", func(t *testing.T) {
		root := t.TempDir()
		resetSpecTreeSnapshotSeams(t)
		resetStageUnixSeams(t)
		originalOpen := stageUnixOpen
		stageUnixOpen = func(string, int, uint32) (int, error) { return -1, errors.New("stage descriptor open failed") }
		if _, err := openStagedSpecTreeSnapshot(root, rootSnapshot(nil)); err == nil || !contains(err, "stage descriptor open failed") {
			t.Fatalf("open staged tree error = %v", err)
		}
		stageUnixOpen = originalOpen

		resetSnapshotNoFollowSeams(t)
		snapshotFsetxattr = func(int, string, []byte, int) error { return errors.New("materialize metadata failed") }
		snapshot := rootSnapshot(nil)
		directory := snapshot.directories["."]
		directory.metadata.extendedAttributes = map[string][]byte{"user.test": []byte("x")}
		snapshot.directories["."] = directory
		if _, err := openStagedSpecTreeSnapshot(root, snapshot); err == nil || !contains(err, "materialize metadata failed") {
			t.Fatalf("materialize staged tree error = %v", err)
		}

		originalCloseStaged := transactionCloseStagedTree
		transactionCloseStagedTree = func(stage *stagedSpecTree) error {
			_ = closeStagedSpecTree(stage)
			return errors.New("stage close failed")
		}
		if _, err := stageSpecTreeSnapshot(root, rootSnapshot(nil)); err == nil || !contains(err, "stage close failed") {
			t.Fatalf("stage helper close error = %v", err)
		}
		transactionCloseStagedTree = originalCloseStaged
		if err := materializeSpecSnapshot(root, specTreeSnapshot{}); err == nil || !contains(err, "missing") {
			t.Fatalf("materialize helper error = %v", err)
		}
	})
}
