package idea

import (
	"path/filepath"
	"strings"

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
	// Lifecycle lint runs against a sibling `.specscore-lint-stage-*` tree so
	// it can later be published with a same-parent no-replace rename. Its
	// parent is still the real project root, which means normal configuration
	// lookup would otherwise redirect an Ideas fixer to the live module. A
	// staged tree is deliberately self-contained and therefore always uses its
	// own conventional ideas/ child.
	if strings.HasPrefix(filepath.Base(filepath.Clean(specDir)), ".specscore-lint-stage-") {
		return filepath.Join(specDir, "ideas")
	}
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
