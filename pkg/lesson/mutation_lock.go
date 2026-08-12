package lesson

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

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
		return mutationFailure(MutationPrePublication, fmt.Errorf("creating Lesson mutation lock directory: %w", err))
	}
	lock := deps.newLock(filepath.Join(lockDir, "lesson-"+slug+".lock"))
	if err := lock.Lock(); err != nil {
		return mutationFailure(MutationPrePublication, fmt.Errorf("acquiring Lesson mutation lock: %w", err))
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

// WithMutationLock serializes one caller-owned Lesson transaction with every
// lifecycle, recurrence, relation, and force-update writer. Callers that also
// mutate the shared index must do so inside mutate: the total order is
// per-Lesson locks (lexical slug order), the optional relation-project lock,
// then the shared Lesson-index lock.
func WithMutationLock(projectRoot, slug string, mutate func() error) error {
	if err := ValidateSlug(slug); err != nil {
		return mutationFailure(MutationPrePublication, fmt.Errorf("invalid Lesson mutation lock slug: %w", err))
	}
	return withLessonMutationLock(projectRoot, slug, defaultLessonMutationLockDeps(), mutate)
}

// WithMutationLocks serializes a caller-owned transaction that spans more than
// one Lesson. Locks are acquired in lexical slug order and duplicate slugs are
// collapsed, preserving the package-wide lock order before the shared index
// lock is acquired by mutate.
func WithMutationLocks(projectRoot string, slugs []string, mutate func() error) error {
	for _, slug := range slugs {
		if err := ValidateSlug(slug); err != nil {
			return mutationFailure(MutationPrePublication, fmt.Errorf("invalid Lesson mutation lock slug: %w", err))
		}
	}
	return withLessonMutationLocks(projectRoot, slugs, defaultLessonMutationLockDeps(), mutate)
}

func withLessonMutationLocks(projectRoot string, slugs []string, deps lessonMutationLockDeps, mutate func() error) error {
	ordered := append([]string(nil), slugs...)
	sort.Strings(ordered)
	unique := ordered[:0]
	for _, slug := range ordered {
		if len(unique) == 0 || unique[len(unique)-1] != slug {
			unique = append(unique, slug)
		}
	}
	var lockNext func(int) error
	lockNext = func(i int) error {
		if i == len(unique) {
			return mutate()
		}
		return withLessonMutationLock(projectRoot, unique[i], deps, func() error { return lockNext(i + 1) })
	}
	return lockNext(0)
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
