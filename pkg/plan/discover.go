package plan

import (
	"os"
	"path/filepath"
	"sort"
)

// Discover walks the direct children of plansDir and returns the parsed
// single-file Plans found there, sorted alphabetically by Slug.
//
// It selects candidates via IsSingleFilePlanPath (which excludes README.md and
// anything not directly under plansDir), Parses each, and keeps only files whose
// first H1 was `# Plan: <title>` (HasPlanTitle == true). Directory-form plans at
// spec/plans/<slug>/README.md are out of scope and skipped.
//
// An absent plansDir is not an error: Discover returns an empty slice and nil.
func Discover(plansDir string) ([]*Plan, error) {
	entries, err := os.ReadDir(plansDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var plans []*Plan
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(plansDir, e.Name())
		if !IsSingleFilePlanPath(plansDir, path) {
			continue
		}
		p, err := Parse(path)
		if err != nil {
			return nil, err
		}
		if !p.HasPlanTitle {
			continue
		}
		plans = append(plans, p)
	}

	sort.Slice(plans, func(i, j int) bool {
		return plans[i].Slug < plans[j].Slug
	})
	return plans, nil
}
