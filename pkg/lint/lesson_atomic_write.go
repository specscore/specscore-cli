package lint

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/lesson"
)

// writeLintFile coordinates every generic lint/migration rewrite that can
// reach a visible Lesson artifact or the shared Lesson index. Other document
// kinds retain their established write behavior. The total Lesson lock order
// is per-artifact first and shared index last; a single write never holds both.
func writeLintFile(specRoot, path string, expected, data []byte, mode os.FileMode) error {
	path = filepath.Clean(path)
	lessonsDir := filepath.Join(filepath.Clean(specRoot), "lessons")
	rewriteExpected := func(write func() error) error {
		current, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(current, expected) {
			return fmt.Errorf("refusing stale Lesson rewrite: %s changed after it was read", path)
		}
		return write()
	}
	if path == filepath.Join(lessonsDir, "README.md") {
		return withLessonIndexLock(specRoot, defaultLessonIndexLockDeps(), func() error {
			return rewriteExpected(func() error { return writeLessonIndexAtomic(path, data) })
		})
	}
	rel, err := filepath.Rel(lessonsDir, path)
	if err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		parts := strings.Split(filepath.ToSlash(rel), "/")
		slug := ""
		if len(parts) == 2 && parts[1] == "README.md" {
			slug = parts[0]
		} else if len(parts) == 1 && strings.HasSuffix(parts[0], ".md") && parts[0] != "README.md" {
			slug = strings.TrimSuffix(parts[0], ".md")
		}
		if slug != "" && lesson.ValidateSlug(slug) == nil {
			return lesson.WithMutationLock(filepath.Dir(specRoot), slug, func() error {
				return rewriteExpected(func() error { return lesson.RewriteFileAtomic(path, data) })
			})
		}
	}
	return os.WriteFile(path, data, mode)
}
