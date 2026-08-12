package lesson

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type stagedFileTestFS struct {
	lessonFS
	openFileFn func(string, int, os.FileMode) (lessonFile, error)
}

func (fs stagedFileTestFS) OpenFile(path string, flags int, mode os.FileMode) (lessonFile, error) {
	if fs.openFileFn != nil {
		return fs.openFileFn(path, flags, mode)
	}
	return fs.lessonFS.OpenFile(path, flags, mode)
}

type stagedFileTestFile struct {
	lessonFile
	writeErr error
	syncErr  error
	closeErr error
	closed   *int
}

func (f stagedFileTestFile) Write(data []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return f.lessonFile.Write(data)
}

func (f stagedFileTestFile) Sync() error {
	if f.syncErr != nil {
		return f.syncErr
	}
	return f.lessonFile.Sync()
}

func (f stagedFileTestFile) Close() error {
	if f.closed != nil {
		*f.closed++
	}
	if f.closeErr != nil {
		return f.closeErr
	}
	return f.lessonFile.Close()
}

func TestWriteDurableStageFileWithFSBoundaryFailuresAreIsolated(t *testing.T) {
	tests := []struct {
		name  string
		fault func(*testing.T, *stagedFileTestFS, *int)
	}{
		{"open", func(_ *testing.T, fs *stagedFileTestFS, _ *int) {
			fs.openFileFn = func(string, int, os.FileMode) (lessonFile, error) { return nil, errors.New("open") }
		}},
		{"write closes", func(t *testing.T, fs *stagedFileTestFS, closed *int) {
			base := osLessonFS{}
			fs.openFileFn = func(path string, flags int, mode os.FileMode) (lessonFile, error) {
				file, err := base.OpenFile(path, flags, mode)
				return stagedFileTestFile{lessonFile: file, writeErr: errors.New("write"), closed: closed}, err
			}
		}},
		{"sync closes", func(t *testing.T, fs *stagedFileTestFS, closed *int) {
			base := osLessonFS{}
			fs.openFileFn = func(path string, flags int, mode os.FileMode) (lessonFile, error) {
				file, err := base.OpenFile(path, flags, mode)
				return stagedFileTestFile{lessonFile: file, syncErr: errors.New("sync"), closed: closed}, err
			}
		}},
		{"close", func(t *testing.T, fs *stagedFileTestFS, closed *int) {
			base := osLessonFS{}
			fs.openFileFn = func(path string, flags int, mode os.FileMode) (lessonFile, error) {
				file, err := base.OpenFile(path, flags, mode)
				return stagedFileTestFile{lessonFile: file, closeErr: errors.New("close"), closed: closed}, err
			}
		}},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			path := filepath.Join(root, "stage")
			closed := 0
			fs := stagedFileTestFS{lessonFS: osLessonFS{}}
			tt.fault(t, &fs, &closed)
			if err := writeDurableStageFileWithFS(path, []byte("safe"), fs); err == nil {
				t.Fatal("injected stage boundary failure succeeded")
			}
			if (tt.name == "write closes" || tt.name == "sync closes" || tt.name == "close") && closed != 1 {
				t.Fatalf("close calls=%d", closed)
			}
		})
	}
}

func TestWriteDurableStageFileWithFSWritesOnce(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "stage")
	if err := writeDurableStageFileWithFS(path, []byte("safe"), stagedFileTestFS{lessonFS: osLessonFS{}}); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "safe" {
		t.Fatalf("stage=%q err=%v", got, err)
	}
}
