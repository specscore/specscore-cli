package lesson

import "os"

// lessonFile and lessonFS are private, per-call durability boundaries. They
// keep public artifact APIs stable while allowing deterministic tests of every
// write and directory-sync outcome without process-global test hooks.
type lessonFile interface {
	Name() string
	Chmod(os.FileMode) error
	Write([]byte) (int, error)
	Sync() error
	Close() error
}

type lessonFS interface {
	ReadFile(string) ([]byte, error)
	ReadDir(string) ([]os.DirEntry, error)
	Stat(string) (os.FileInfo, error)
	Mkdir(string, os.FileMode) error
	Remove(string) error
	CreateTemp(string, string) (lessonFile, error)
	Open(string) (lessonFile, error)
	OpenFile(string, int, os.FileMode) (lessonFile, error)
	Link(string, string) error
}

type osLessonFS struct{}

func (osLessonFS) ReadFile(path string) ([]byte, error)       { return os.ReadFile(path) }
func (osLessonFS) ReadDir(path string) ([]os.DirEntry, error) { return os.ReadDir(path) }
func (osLessonFS) Stat(path string) (os.FileInfo, error)      { return os.Stat(path) }
func (osLessonFS) Mkdir(path string, mode os.FileMode) error  { return os.Mkdir(path, mode) }
func (osLessonFS) Remove(path string) error                   { return os.Remove(path) }
func (osLessonFS) CreateTemp(dir, pattern string) (lessonFile, error) {
	return os.CreateTemp(dir, pattern)
}
func (osLessonFS) Open(path string) (lessonFile, error) { return os.Open(path) }
func (osLessonFS) OpenFile(path string, flag int, mode os.FileMode) (lessonFile, error) {
	return os.OpenFile(path, flag, mode)
}
func (osLessonFS) Link(oldname, newname string) error { return os.Link(oldname, newname) }

func syncDirectoryWithFS(path string, fs lessonFS) error {
	f, err := fs.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return f.Sync()
}
