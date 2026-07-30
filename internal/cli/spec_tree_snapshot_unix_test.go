//go:build darwin || linux

package cli

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

type lifecycleTestFileInfo struct {
	mode os.FileMode
	sys  any
}

func (info lifecycleTestFileInfo) Name() string       { return "entry" }
func (info lifecycleTestFileInfo) Size() int64        { return 0 }
func (info lifecycleTestFileInfo) Mode() os.FileMode  { return info.mode }
func (info lifecycleTestFileInfo) ModTime() time.Time { return time.Unix(1_700_000_000, 123_456_789) }
func (info lifecycleTestFileInfo) IsDir() bool        { return info.mode.IsDir() }
func (info lifecycleTestFileInfo) Sys() any           { return info.sys }

func TestSnapshotMetadata_FailClosedBranches(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "entry")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })

	t.Run("unsupported metadata", func(t *testing.T) {
		resetSnapshotNoFollowSeams(t)
		if _, err := captureSnapshotEntryMetadata(file, lifecycleTestFileInfo{mode: os.ModeSetuid | 0o644}); err == nil || !contains(err, "privileged") {
			t.Fatalf("privileged metadata error = %v", err)
		}
		if _, err := captureSnapshotEntryMetadata(file, lifecycleTestFileInfo{mode: 0o644}); err == nil || !contains(err, "unsupported stat") {
			t.Fatalf("stat metadata error = %v", err)
		}
		foreign := &syscall.Stat_t{Uid: uint32(os.Getuid() + 1), Gid: uint32(os.Getgid())}
		if _, err := captureSnapshotEntryMetadata(file, lifecycleTestFileInfo{mode: 0o644, sys: foreign}); err == nil || !contains(err, "cannot be preserved") {
			t.Fatalf("owner metadata error = %v", err)
		}
	})
	t.Run("metadata capture propagates xattr failure", func(t *testing.T) {
		resetSnapshotNoFollowSeams(t)
		info, err := file.Stat()
		if err != nil {
			t.Fatal(err)
		}
		snapshotFlistxattr = func(int, []byte) (int, error) { return 0, errors.New("capture list failed") }
		if _, err := captureSnapshotEntryMetadata(file, info); err == nil || !contains(err, "capture list failed") {
			t.Fatalf("metadata capture error = %v", err)
		}
	})
	t.Run("metadata capture propagates timestamp failure", func(t *testing.T) {
		resetSnapshotNoFollowSeams(t)
		info, err := file.Stat()
		if err != nil {
			t.Fatal(err)
		}
		snapshotMetadataEntryTimes = func(os.FileInfo) (time.Time, time.Time, error) {
			return time.Time{}, time.Time{}, errors.New("capture timestamp failed")
		}
		if _, err := captureSnapshotEntryMetadata(file, info); err == nil || !contains(err, "capture timestamp failed") {
			t.Fatalf("metadata timestamp error = %v", err)
		}
	})
	t.Run("xattr read failures", func(t *testing.T) {
		resetSnapshotNoFollowSeams(t)
		snapshotFlistxattr = func(int, []byte) (int, error) { return 0, errors.New("list failed") }
		if _, err := readSnapshotExtendedAttributes(int(file.Fd())); err == nil || !contains(err, "list failed") {
			t.Fatalf("list error = %v", err)
		}
		calls := 0
		snapshotFlistxattr = func(int, []byte) (int, error) {
			calls++
			if calls == 1 {
				return 2, nil
			}
			return 0, errors.New("names failed")
		}
		if _, err := readSnapshotExtendedAttributes(int(file.Fd())); err == nil || !contains(err, "names failed") {
			t.Fatalf("name list error = %v", err)
		}
		calls = 0
		snapshotFlistxattr = func(_ int, dest []byte) (int, error) {
			calls++
			if calls == 1 {
				return 2, nil
			}
			copy(dest, "x\x00")
			return 1, nil
		}
		if _, err := readSnapshotExtendedAttributes(int(file.Fd())); err == nil || !contains(err, "changed while reading") {
			t.Fatalf("name race error = %v", err)
		}
	})
	t.Run("empty xattr list", func(t *testing.T) {
		resetSnapshotNoFollowSeams(t)
		snapshotFlistxattr = func(int, []byte) (int, error) { return 0, nil }
		attributes, err := readSnapshotExtendedAttributes(int(file.Fd()))
		if err != nil || attributes != nil {
			t.Fatalf("empty xattrs = %#v, %v", attributes, err)
		}
	})
	t.Run("xattr value failures", func(t *testing.T) {
		resetSnapshotNoFollowSeams(t)
		snapshotFlistxattr = func(_ int, dest []byte) (int, error) {
			if dest == nil {
				return 2, nil
			}
			copy(dest, "x\x00")
			return 2, nil
		}
		snapshotFgetxattr = func(int, string, []byte) (int, error) { return 0, errors.New("value failed") }
		if _, err := readSnapshotExtendedAttributes(int(file.Fd())); err == nil || !contains(err, "value failed") {
			t.Fatalf("value size error = %v", err)
		}
		calls := 0
		snapshotFgetxattr = func(_ int, _ string, dest []byte) (int, error) {
			calls++
			if dest == nil {
				return 1, nil
			}
			return 0, errors.New("value bytes failed")
		}
		if _, err := readSnapshotExtendedAttributes(int(file.Fd())); err == nil || !contains(err, "value bytes failed") {
			t.Fatalf("value read error = %v", err)
		}
		snapshotFgetxattr = func(_ int, _ string, dest []byte) (int, error) {
			if dest == nil {
				return 2, nil
			}
			return 1, nil
		}
		if _, err := readSnapshotExtendedAttributes(int(file.Fd())); err == nil || !contains(err, "changed while reading") {
			t.Fatalf("value race error = %v", err)
		}
	})
	t.Run("ACL and capability xattrs are rejected before reads", func(t *testing.T) {
		resetSnapshotNoFollowSeams(t)
		snapshotFlistxattr = func(_ int, dest []byte) (int, error) {
			if dest == nil {
				return len("security.capability\x00"), nil
			}
			copy(dest, "security.capability\x00")
			return len("security.capability\x00"), nil
		}
		if _, err := readSnapshotExtendedAttributes(int(file.Fd())); err == nil || !contains(err, "cannot preserve ACL") {
			t.Fatalf("security xattr error = %v", err)
		}
	})
	t.Run("set xattr failure", func(t *testing.T) {
		resetSnapshotNoFollowSeams(t)
		snapshotFsetxattr = func(int, string, []byte, int) error { return errors.New("set failed") }
		if err := applyStagedEntryMetadata(int(file.Fd()), specTreeEntryMetadata{extendedAttributes: map[string][]byte{"user.test": []byte("x")}}); err == nil || !contains(err, "set failed") {
			t.Fatalf("set metadata error = %v", err)
		}
	})
}

func TestStagedSnapshotIdentity_FailureBranches(t *testing.T) {
	stage, err := openStagedSpecTreeNoFollow(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeStagedSpecTree(stage) })
	if _, err := snapshotStagedSpecTreeNoFollow(nil); err == nil || !contains(err, "closed") {
		t.Fatalf("closed snapshot error = %v", err)
	}
	t.Run("path open failures", func(t *testing.T) {
		resetSnapshotNoFollowSeams(t)
		snapshotOpenRoot = func(string, int, uint32) (int, error) { return -1, errors.New("path open failed") }
		if _, err := stagedSpecTreeMatchesPath(stage); err == nil || !contains(err, "path open failed") {
			t.Fatalf("stage identity open error = %v", err)
		}
		if _, err := stagedSpecTreePublishedAt(stage, stage.path); err == nil || !contains(err, "path open failed") {
			t.Fatalf("published identity open error = %v", err)
		}
	})
	t.Run("held and candidate stat failures", func(t *testing.T) {
		resetSnapshotNoFollowSeams(t)
		original := snapshotFileStat
		snapshotFileStat = func(*os.File) (os.FileInfo, error) { return nil, errors.New("held stat failed") }
		if _, err := stagedSpecTreeMatchesPath(stage); err == nil || !contains(err, "held stat failed") {
			t.Fatalf("held stage stat error = %v", err)
		}
		snapshotFileStat = original
		calls := 0
		snapshotFileStat = func(file *os.File) (os.FileInfo, error) {
			calls++
			if calls == 2 {
				return nil, errors.New("candidate stat failed")
			}
			return original(file)
		}
		if _, err := stagedSpecTreePublishedAt(stage, stage.path); err == nil || !contains(err, "candidate stat failed") {
			t.Fatalf("published candidate stat error = %v", err)
		}
		snapshotFileStat = func(file *os.File) (os.FileInfo, error) { return file.Stat() }
		resetSnapshotNoFollowSeams(t)
		calls = 0
		original = snapshotFileStat
		snapshotFileStat = func(file *os.File) (os.FileInfo, error) {
			calls++
			if calls == 2 {
				return nil, errors.New("stage candidate stat failed")
			}
			return original(file)
		}
		if _, err := stagedSpecTreeMatchesPath(stage); err == nil || !contains(err, "stage candidate stat failed") {
			t.Fatalf("stage candidate stat error = %v", err)
		}
		resetSnapshotNoFollowSeams(t)
		snapshotFileStat = func(*os.File) (os.FileInfo, error) { return nil, errors.New("published held stat failed") }
		if _, err := stagedSpecTreePublishedAt(stage, stage.path); err == nil || !contains(err, "published held stat failed") {
			t.Fatalf("published held stat error = %v", err)
		}
	})
}

func TestStagedSpecTree_PreservesNanosecondTimestampAndXattr(t *testing.T) {
	project := t.TempDir()
	specRoot := filepath.Join(project, "spec")
	if err := os.Mkdir(specRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(specRoot, "README.md")
	if err := os.WriteFile(filePath, []byte("body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wantTime := time.Unix(1_700_000_000, 123_456_789)
	if err := os.Chtimes(filePath, wantTime, wantTime); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.Fsetxattr(int(file.Fd()), "user.specscore-lifecycle-test", []byte("metadata"), 0); err != nil {
		_ = file.Close()
		t.Fatalf("setting real xattr: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := snapshotSpecTreeForTransaction(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := openStagedSpecTreeSnapshot(specRoot, before)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = closeStagedSpecTree(stage)
		_ = os.RemoveAll(stage.path)
	})
	after, err := snapshotStagedSpecTreeNoFollow(stage)
	if err != nil {
		t.Fatal(err)
	}
	if !specTreeSnapshotsEqual(before, after) {
		t.Fatalf("staged snapshot differs\nwant: %#v\n got: %#v", before, after)
	}
	if got := after.files["README.md"].metadata.modificationTime; !got.Equal(wantTime) {
		t.Fatalf("stage modtime = %s, want %s", got, wantTime)
	}
	if got := string(after.files["README.md"].metadata.extendedAttributes["user.specscore-lifecycle-test"]); got != "metadata" {
		t.Fatalf("stage xattr = %q, want metadata", got)
	}
}

func TestSnapshotSpecTreeNoFollow_RejectsHardLinkedRegularFile(t *testing.T) {
	root := t.TempDir()
	original := filepath.Join(root, "README.md")
	if err := os.WriteFile(original, []byte("body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(original, filepath.Join(root, "README-copy.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := snapshotSpecTreeForTransaction(root); err == nil || !contains(err, "hard-linked file") {
		t.Fatalf("hard-link snapshot error = %v", err)
	}
}

func resetSnapshotNoFollowSeams(t *testing.T) {
	t.Helper()
	openRoot, openAt := snapshotOpenRoot, snapshotOpenAt
	stat, readNames, seek, closeFile := snapshotFileStat, snapshotReadDirNames, snapshotSeek, snapshotClose
	listXattr, getXattr, setXattr, entryTimes := snapshotFlistxattr, snapshotFgetxattr, snapshotFsetxattr, snapshotMetadataEntryTimes
	t.Cleanup(func() {
		snapshotOpenRoot, snapshotOpenAt = openRoot, openAt
		snapshotFileStat, snapshotReadDirNames, snapshotSeek, snapshotClose = stat, readNames, seek, closeFile
		snapshotFlistxattr, snapshotFgetxattr, snapshotFsetxattr, snapshotMetadataEntryTimes = listXattr, getXattr, setXattr, entryTimes
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
	t.Run("root descriptor rewind failure", func(t *testing.T) {
		resetSnapshotNoFollowSeams(t)
		snapshotSeek = func(*os.File, int64, int) (int64, error) { return 0, errors.New("rewind failed") }
		if _, err := snapshotSpecTreeForTransaction(t.TempDir()); err == nil || !contains(err, "rewind failed") {
			t.Fatalf("snapshot rewind error = %v", err)
		}
	})

	t.Run("root descriptor metadata failure", func(t *testing.T) {
		resetSnapshotNoFollowSeams(t)
		root := t.TempDir()
		snapshotFileStat = func(*os.File) (os.FileInfo, error) {
			return lifecycleTestFileInfo{mode: os.ModeDir | os.ModeSetuid | 0o755}, nil
		}
		if _, err := snapshotSpecTreeForTransaction(root); err == nil || !contains(err, "root metadata") {
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
		if _, err := snapshotSpecTreeForTransaction(root); err == nil || !contains(err, "non-regular file pipe") {
			t.Fatalf("nonregular FIFO error = %v", err)
		}
		if err := os.Remove(fifo); err != nil {
			t.Fatal(err)
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
