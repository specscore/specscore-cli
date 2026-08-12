package lint

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/specscore/specscore-cli/pkg/lesson"
)

type lessonIndexFaultFile struct {
	lessonIndexFile
	fail  string
	role  string
	short bool
}

func (f lessonIndexFaultFile) Chmod(mode os.FileMode) error {
	if f.fail == f.role+"-chmod" {
		return errors.New(f.fail)
	}
	return f.lessonIndexFile.Chmod(mode)
}
func (f lessonIndexFaultFile) Write(data []byte) (int, error) {
	if f.fail == f.role+"-write" {
		return 0, errors.New(f.fail)
	}
	if f.short {
		return len(data) - 1, nil
	}
	return f.lessonIndexFile.Write(data)
}
func (f lessonIndexFaultFile) Sync() error {
	if f.fail == f.role+"-sync" {
		return errors.New(f.fail)
	}
	return f.lessonIndexFile.Sync()
}
func (f lessonIndexFaultFile) Close() error {
	if f.fail == f.role+"-close" {
		return errors.New(f.fail)
	}
	return f.lessonIndexFile.Close()
}

func lessonIndexFaultOps(fail string) lessonIndexWriteOps {
	ops := defaultLessonIndexWriteOps()
	realStat, realCreate, realRename, realOpen := ops.stat, ops.createTemp, ops.rename, ops.open
	ops.stat = func(path string) (os.FileInfo, error) {
		if fail == "stat" {
			return nil, errors.New(fail)
		}
		return realStat(path)
	}
	ops.createTemp = func(dir, pattern string) (lessonIndexFile, error) {
		if fail == "create" {
			return nil, errors.New(fail)
		}
		f, err := realCreate(dir, pattern)
		if err != nil {
			return nil, err
		}
		return lessonIndexFaultFile{lessonIndexFile: f, fail: fail, role: "file", short: fail == "short-write"}, nil
	}
	ops.rename = func(oldPath, newPath string) error {
		if fail == "rename" {
			return errors.New(fail)
		}
		return realRename(oldPath, newPath)
	}
	ops.open = func(path string) (lessonIndexFile, error) {
		if fail == "dir-open" {
			return nil, errors.New(fail)
		}
		f, err := realOpen(path)
		if err != nil {
			return nil, err
		}
		return lessonIndexFaultFile{lessonIndexFile: f, fail: fail, role: "dir"}, nil
	}
	return ops
}

func TestLessonIndexAtomicWriterPreservesParseableOriginalAndSupportsDurableRetry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "README.md")
	original := []byte("# Lessons\n\n## Lessons\n\n| Lesson | Status | Classifications | Occurrences | Last Occurred | Enforcement |\n|---|---|---|---:|---|---|\n")
	updated := append(append([]byte(nil), original...), []byte("| [rule](rule/README.md) | Recorded | process | 0 |  | — |\n")...)
	for _, fault := range []string{"stat", "create", "file-chmod", "file-write", "short-write", "file-sync", "file-close", "rename"} {
		t.Run(fault, func(t *testing.T) {
			if err := os.WriteFile(path, original, 0o640); err != nil {
				t.Fatal(err)
			}
			err := writeLessonIndexAtomicWithOps(path, updated, lessonIndexFaultOps(fault))
			if err == nil {
				t.Fatal("fault was accepted")
			}
			if fault == "short-write" && !errors.Is(err, io.ErrShortWrite) {
				t.Fatalf("short write = %v", err)
			}
			got, readErr := os.ReadFile(path)
			if readErr != nil || string(got) != string(original) {
				t.Fatalf("pre-rename fault changed parseable original: %q, %v", got, readErr)
			}
		})
	}
	for _, fault := range []string{"dir-open", "dir-sync", "dir-close"} {
		t.Run(fault, func(t *testing.T) {
			if err := os.WriteFile(path, original, 0o640); err != nil {
				t.Fatal(err)
			}
			err := writeLessonIndexAtomicWithOps(path, updated, lessonIndexFaultOps(fault))
			if err == nil || lesson.MutationOutcomeOf(err) != lesson.MutationUncertain {
				t.Fatalf("post-rename fault = %v", err)
			}
			got, readErr := os.ReadFile(path)
			if readErr != nil || string(got) != string(updated) {
				t.Fatalf("published index not retained: %q, %v", got, readErr)
			}
			if retryErr := writeLessonIndexAtomic(path, append(updated, []byte("<!-- retry -->\n")...)); retryErr != nil {
				t.Fatalf("durable retry: %v", retryErr)
			}
		})
	}
	if err := os.WriteFile(path, original, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := writeLessonIndexAtomic(path, updated); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %v, err=%v", info.Mode().Perm(), err)
	}
}
