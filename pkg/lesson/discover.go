package lesson

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Discover walks the direct children of lessonsDir and returns the parsed
// single-file Lessons found there, sorted alphabetically by Slug.
//
// It selects candidates via IsSingleFileLessonPath (which excludes README.md
// and anything not directly under lessonsDir), Parses each, and keeps only
// files whose first H1 was `# Lesson: <title>` (HasLessonTitle == true).
//
// An absent lessonsDir is not an error: Discover returns an empty slice and
// nil.
func Discover(lessonsDir string) ([]*Lesson, error) {
	entries, err := os.ReadDir(lessonsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var lessons []*Lesson
	for _, e := range entries {
		path := filepath.Join(lessonsDir, e.Name())
		if e.IsDir() {
			path = filepath.Join(path, "README.md")
			if _, err := os.Stat(path); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, err
			}
		} else if !IsSingleFileLessonPath(lessonsDir, path) {
			continue
		}
		l, err := Parse(path)
		if err != nil {
			return nil, err
		}
		if !l.HasLessonTitle {
			continue
		}
		lessons = append(lessons, l)
	}

	sort.Slice(lessons, func(i, j int) bool {
		return lessons[i].Slug < lessons[j].Slug
	})
	seen := make(map[string]bool, len(lessons))
	for _, l := range lessons {
		if seen[l.Slug] {
			return nil, fmt.Errorf("lesson %q has both legacy flat and canonical directory forms", l.Slug)
		}
		seen[l.Slug] = true
	}
	return lessons, nil
}
