package lint

import "path/filepath"

func effectivePlansDir(specRoot, override string) string {
	if override != "" {
		return override
	}
	return filepath.Join(specRoot, "plans")
}
