//go:build darwin || linux

package cli

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func resetSnapshotNoFollowSeams(t *testing.T) {
	t.Helper()
	openRoot, openAt := snapshotOpenRoot, snapshotOpenAt
	stat, readNames, closeFile := snapshotFileStat, snapshotReadDirNames, snapshotClose
	t.Cleanup(func() {
		snapshotOpenRoot, snapshotOpenAt = openRoot, openAt
		snapshotFileStat, snapshotReadDirNames, snapshotClose = stat, readNames, closeFile
	})
}

func TestSnapshotSpecTreeNoFollow_FailClosedDescriptorBranches(t *testing.T) {
	t.Run("root descriptor stat failure", func(t *testing.T) {
		resetSnapshotNoFollowSeams(t)
		root := t.TempDir()
		snapshotFileStat = func(*os.File) (os.FileInfo, error) { return nil, errors.New("root stat failed") }
		if _, err := snapshotSpecTreeForTransaction(root); err == nil || !contains(err, "root stat failed") {
			t.Fatalf("snapshot error = %v", err)
		}
	})

	t.Run("directory enumeration and entry validation failures", func(t *testing.T) {
		root := t.TempDir()
		t.Run("read names", func(t *testing.T) {
			resetSnapshotNoFollowSeams(t)
			snapshotReadDirNames = func(*os.File) ([]string, error) { return nil, errors.New("readdir failed") }
			if _, err := snapshotSpecTreeForTransaction(root); err == nil || !contains(err, "readdir failed") {
				t.Fatalf("snapshot error = %v", err)
			}
		})
		t.Run("invalid name", func(t *testing.T) {
			resetSnapshotNoFollowSeams(t)
			snapshotReadDirNames = func(*os.File) ([]string, error) { return []string{".."}, nil }
			if _, err := snapshotSpecTreeForTransaction(root); err == nil || !contains(err, "invalid directory entry") {
				t.Fatalf("snapshot error = %v", err)
			}
		})
	})

	t.Run("open and child stat failures", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "file.md"), []byte("body"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Run("openat", func(t *testing.T) {
			resetSnapshotNoFollowSeams(t)
			snapshotOpenAt = func(int, string, int, uint32) (int, error) { return -1, errors.New("openat failed") }
			if _, err := snapshotSpecTreeForTransaction(root); err == nil || !contains(err, "openat failed") {
				t.Fatalf("snapshot error = %v", err)
			}
		})
		t.Run("child stat", func(t *testing.T) {
			resetSnapshotNoFollowSeams(t)
			calls := 0
			original := snapshotFileStat
			snapshotFileStat = func(file *os.File) (os.FileInfo, error) {
				calls++
				if calls == 2 {
					return nil, errors.New("child stat failed")
				}
				return original(file)
			}
			if _, err := snapshotSpecTreeForTransaction(root); err == nil || !contains(err, "child stat failed") {
				t.Fatalf("snapshot error = %v", err)
			}
		})
	})

	t.Run("nested directory error and close failure", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, "nested"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Run("nested readdir", func(t *testing.T) {
			resetSnapshotNoFollowSeams(t)
			original := snapshotReadDirNames
			snapshotReadDirNames = func(file *os.File) ([]string, error) {
				if file.Name() == "nested" {
					return nil, errors.New("nested readdir failed")
				}
				return original(file)
			}
			if _, err := snapshotSpecTreeForTransaction(root); err == nil || !contains(err, "nested readdir failed") {
				t.Fatalf("snapshot error = %v", err)
			}
		})
		t.Run("nested close", func(t *testing.T) {
			resetSnapshotNoFollowSeams(t)
			original := snapshotClose
			snapshotClose = func(file *os.File) error {
				if file.Name() == "nested" {
					return errors.New("nested close failed")
				}
				return original(file)
			}
			if _, err := snapshotSpecTreeForTransaction(root); err == nil || !contains(err, "nested close failed") {
				t.Fatalf("snapshot error = %v", err)
			}
		})
	})

	t.Run("nonregular, restat, close and concurrent file branches", func(t *testing.T) {
		root := t.TempDir()
		fifo := filepath.Join(root, "pipe")
		if err := unix.Mkfifo(fifo, 0o600); err != nil {
			t.Skipf("mkfifo unavailable: %v", err)
		}
		snapshot, err := snapshotSpecTreeForTransaction(root)
		if err != nil {
			t.Fatal(err)
		}
		if len(snapshot.files) != 0 {
			t.Fatalf("nonregular FIFO was captured: %#v", snapshot.files)
		}

		file := filepath.Join(root, "file.md")
		if err := os.WriteFile(file, []byte("body"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Run("restat", func(t *testing.T) {
			resetSnapshotNoFollowSeams(t)
			calls, original := 0, snapshotFileStat
			snapshotFileStat = func(f *os.File) (os.FileInfo, error) {
				calls++
				if calls == 3 {
					return nil, errors.New("restat failed")
				}
				return original(f)
			}
			if _, err := snapshotSpecTreeForTransaction(root); err == nil || !contains(err, "restat failed") {
				t.Fatalf("snapshot error = %v", err)
			}
		})
		t.Run("file close", func(t *testing.T) {
			resetSnapshotNoFollowSeams(t)
			original := snapshotClose
			snapshotClose = func(f *os.File) error {
				if f.Name() == "file.md" {
					return errors.New("file close failed")
				}
				return original(f)
			}
			if _, err := snapshotSpecTreeForTransaction(root); err == nil || !contains(err, "file close failed") {
				t.Fatalf("snapshot error = %v", err)
			}
		})
		t.Run("concurrent write", func(t *testing.T) {
			originalRead := transactionReadSnapshotFile
			transactionReadSnapshotFile = func(r io.Reader) ([]byte, error) {
				data, err := io.ReadAll(r)
				if err == nil {
					err = os.WriteFile(file, []byte("rewritten while read"), 0o644)
				}
				return data, err
			}
			t.Cleanup(func() { transactionReadSnapshotFile = originalRead })
			if _, err := snapshotSpecTreeForTransaction(root); err == nil || !contains(err, "concurrent modification") {
				t.Fatalf("snapshot error = %v", err)
			}
		})
	})

	t.Run("file restat failure", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "file.md"), []byte("body"), 0o644); err != nil {
			t.Fatal(err)
		}
		resetSnapshotNoFollowSeams(t)
		calls, original := 0, snapshotFileStat
		snapshotFileStat = func(file *os.File) (os.FileInfo, error) {
			calls++
			if calls == 3 { // root stat, file stat, then the after-read stat.
				return nil, errors.New("post-read stat failed")
			}
			return original(file)
		}
		if _, err := snapshotSpecTreeForTransaction(root); err == nil || !contains(err, "post-read stat failed") {
			t.Fatalf("snapshot error = %v", err)
		}
	})
}
