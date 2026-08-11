package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/lint"
)

// reconcileLockedLessons completes a Lesson artifact transaction while the
// caller still holds every affected per-Lesson lock. It derives index rows
// from the current locked artifacts, runs bounded read-only validation, and
// fences artifact files/directories plus the shared index before an event may
// commit. The index writer acquires the final lock in the global order.
func reconcileLockedLessons(root string, slugs, extraFence []string, deps lessonCLIDeps) error {
	ordered := append([]string(nil), slugs...)
	sort.Strings(ordered)
	unique := ordered[:0]
	for _, slug := range ordered {
		if len(unique) == 0 || unique[len(unique)-1] != slug {
			unique = append(unique, slug)
		}
	}
	specRoot := filepath.Join(root, "spec")
	lessonsDir := filepath.Join(specRoot, "lessons")
	indexPath := filepath.Join(lessonsDir, "README.md")
	fence := append([]string(nil), extraFence...)
	for _, slug := range unique {
		path, err := lesson.ResolveLessonFile(lessonsDir, slug)
		if err != nil {
			return fmt.Errorf("resolving Lesson %s after mutation: %w", slug, err)
		}
		current, err := deps.parse(path)
		if err != nil {
			return fmt.Errorf("parsing Lesson %s after mutation: %w", slug, err)
		}
		if err := deps.indexUpsert(specRoot, current); err != nil {
			return fmt.Errorf("upserting Lesson %s index row: %w", slug, err)
		}
		fence = append(fence, path)
		if current.Canonical {
			occurrencesDir := filepath.Join(filepath.Dir(path), "occurrences")
			if _, err := deps.fs.stat(occurrencesDir); err == nil {
				fence = append(fence, occurrencesDir)
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("checking Lesson %s occurrences directory: %w", slug, err)
			}
		}
	}
	violations, err := deps.lint(lint.Options{SpecRoot: specRoot})
	if err != nil {
		return fmt.Errorf("running read-only Lesson lint: %w", err)
	}
	for _, violation := range violations {
		if violation.Severity != "error" || !ownedLessonMutationViolation(violation, unique) {
			continue
		}
		return fmt.Errorf("lesson mutation failed lint: %s:%d [%s] %s", violation.File, violation.Line, violation.Rule, violation.Message)
	}
	fence = append(fence, indexPath)
	seen := make(map[string]bool, len(fence))
	compact := fence[:0]
	for _, path := range fence {
		path = filepath.Clean(path)
		if path == "." || seen[path] {
			continue
		}
		seen[path] = true
		compact = append(compact, path)
	}
	if err := newLessonMutationCoordinator(nil, deps.durable).Fence(compact...); err != nil {
		return fmt.Errorf("durably fencing Lesson mutation: %w", err)
	}
	return nil
}

func ownedLessonMutationViolation(violation lint.Violation, slugs []string) bool {
	for _, slug := range slugs {
		canonical := filepath.ToSlash(filepath.Join("lessons", slug, "README.md"))
		legacy := filepath.ToSlash(filepath.Join("lessons", slug+".md"))
		occurrences := filepath.ToSlash(filepath.Join("lessons", slug, "occurrences")) + "/"
		if violation.File == canonical || violation.File == legacy || strings.HasPrefix(violation.File, occurrences) {
			return true
		}
		if violation.File == "lessons/README.md" && (violation.Rule == "L-003" || violation.Rule == "L-004") && strings.Contains(violation.Message, slug) {
			return true
		}
	}
	return false
}
