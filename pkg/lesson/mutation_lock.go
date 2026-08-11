package lesson

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

type lessonMutationLocker interface {
	Lock() error
	Unlock() error
}

type lessonMutationLockDeps struct {
	mkdirAll func(string, os.FileMode) error
	newLock  func(string) lessonMutationLocker
}

func defaultLessonMutationLockDeps() lessonMutationLockDeps {
	return lessonMutationLockDeps{
		mkdirAll: os.MkdirAll,
		newLock:  func(path string) lessonMutationLocker { return flock.New(path) },
	}
}

// withLessonMutationLock serializes cooperating writers for one Lesson from
// validation through publication and post-mutation reconciliation. Commands
// that also update the shared index always acquire this per-artifact lock
// first; the index writer lock is therefore the second and final lock.
func withLessonMutationLock(projectRoot, slug string, deps lessonMutationLockDeps, mutate func() error) error {
	lockDir := filepath.Join(projectRoot, ".specscore", "locks")
	if err := deps.mkdirAll(lockDir, 0o700); err != nil {
		return fmt.Errorf("creating Lesson mutation lock directory: %w", err)
	}
	lock := deps.newLock(filepath.Join(lockDir, "lesson-"+slug+".lock"))
	if err := lock.Lock(); err != nil {
		return fmt.Errorf("acquiring Lesson mutation lock: %w", err)
	}
	locked := true
	defer func() {
		if locked {
			_ = lock.Unlock()
		}
	}()
	if err := mutate(); err != nil {
		return err
	}
	if err := lock.Unlock(); err != nil {
		return mutationFailure(MutationUncertain, fmt.Errorf("releasing Lesson mutation lock: %w", err))
	}
	locked = false
	return nil
}

func projectRootForLessonPath(path string) string {
	lessonsDir := filepath.Dir(path)
	if filepath.Base(path) == "README.md" {
		lessonsDir = filepath.Dir(filepath.Dir(path))
	}
	if filepath.Base(lessonsDir) == "lessons" && filepath.Base(filepath.Dir(lessonsDir)) == "spec" {
		return filepath.Dir(filepath.Dir(lessonsDir))
	}
	return filepath.Dir(path)
}
