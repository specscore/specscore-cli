package plan

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Discover recursively walks canonical directory-form Plans and also reads
// legacy flat Plans directly under plansDir. IDs are relative slash paths.
func Discover(plansDir string) ([]*Plan, error) {
	if info, err := os.Stat(plansDir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	} else if !info.IsDir() {
		return nil, fmt.Errorf("plans path is not a directory: %s", plansDir)
	}
	var plans []*Plan
	paths := map[string]string{}
	err := filepath.WalkDir(plansDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			// TOCTOU tolerance: plansDir passed the os.Stat check above, but
			// WalkDir does its own Lstat(plansDir) as its very first step: if
			// a concurrent process removes plansDir in the (usually
			// microseconds-wide) window between those two calls, treat it the
			// same as "didn't exist at the start" rather than erroring. Only
			// a genuine concurrent mutation exercises this — not something a
			// deterministic, non-racy test can trigger on demand.
			if os.IsNotExist(walkErr) && path == plansDir {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() {
			if path != plansDir && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		id, candidate := planIDFromPath(plansDir, path)
		if !candidate {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("plan path must not be a symbolic link: %s", path)
		}
		// Defense in depth against plansDir itself being swapped out (e.g.
		// re-pointed to a different symlink target) between the entry check
		// above and here: filepath.WalkDir never descends into a symlinked
		// subdirectory (a symlink DirEntry reports IsDir()==false, so it is
		// visited but not recursed into), so every "path" reached by a
		// legitimate, single walk is a plain-directory descendant of
		// plansDir and this call cannot fail for it — only a concurrent
		// mutation of plansDir mid-walk could trigger this, which a
		// deterministic, non-racy test has no way to force on demand.
		if err := validateResolvedPlanPath(plansDir, path); err != nil {
			return err
		}
		if prior := paths[id]; prior != "" && prior != path {
			return fmt.Errorf("plan %q is ambiguous: both %s and %s exist", id, prior, path)
		}
		paths[id] = path
		p, parseErr := Parse(path)
		if parseErr != nil {
			return parseErr
		}
		if !p.HasPlanTitle {
			return nil
		}
		p.Slug = id
		plans = append(plans, p)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(plans, func(i, j int) bool {
		return plans[i].Slug < plans[j].Slug
	})
	return plans, nil
}
