package idea

import (
	"path/filepath"

	"github.com/specscore/specscore-cli/pkg/projectdef"
)

// ResolveIdeasDir returns the absolute ideas directory for the repo's root
// module, honoring path_overrides.ideas_path
// (configurable-ideas-path#req:single-resolver). specDir is the spec/
// directory; the project root is its parent. When no config is present or
// no override is set, it returns specDir/ideas — identical to the historical
// default (configurable-ideas-path#req:ideas-path-default).
func ResolveIdeasDir(specDir string) string {
	projectRoot := filepath.Dir(specDir)
	cfg, err := projectdef.ReadSpecConfig(projectRoot)
	if err != nil {
		return filepath.Join(specDir, "ideas")
	}
	m := cfg.EffectiveModules()[0]
	rel := filepath.FromSlash(m.EffectiveIdeasPath())
	return filepath.Join(projectRoot, m.EffectivePath(), rel)
}

// ResolveSeedsDir returns the absolute sidekick-seed directory, which is
// always "seeds" under the resolved ideas directory
// (configurable-ideas-path#req:seeds-follow-ideas).
func ResolveSeedsDir(specDir string) string {
	return filepath.Join(ResolveIdeasDir(specDir), "seeds")
}
