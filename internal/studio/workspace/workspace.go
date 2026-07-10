// Package workspace loads and resolves SpecScore Studio workspace files
// (studio.yaml): an ecosystem name plus a list of repo directory paths or
// glob patterns, absolute or workspace-relative.
//
// Feature: cli/studio/index (REQ: workspace-config, REQ: workspace-errors)
package workspace

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// Test seams — package-level vars wrapping external functions.
// Production code calls these vars; tests replace them via t.Cleanup.
var (
	filepathAbsFn  = filepath.Abs
	filepathGlobFn = filepath.Glob
	osReadFileFn   = os.ReadFile
)

// Workspace is a parsed studio.yaml workspace file.
type Workspace struct {
	// Name is the ecosystem name (required).
	Name string `yaml:"name"`
	// Repos are the raw repo entries: absolute or workspace-relative local
	// directory paths; glob patterns allowed (required, non-empty).
	Repos []string `yaml:"repos"`

	// Path is the absolute path of the workspace file.
	Path string `yaml:"-"`
	// Dir is the absolute directory containing the workspace file; relative
	// repo entries and workspace-relative defaults resolve against it.
	Dir string `yaml:"-"`
}

// Load reads and parses the workspace file at path (absolute or relative to
// the working directory). A missing or unparsable file, a missing ecosystem
// name, or an empty repos list returns an exit-2 error with a one-line
// actionable message naming the expected workspace path.
func Load(path string) (*Workspace, error) {
	abs, err := filepathAbsFn(path)
	if err != nil {
		return nil, exitcode.InvalidArgsErrorf("resolving workspace path %s: %v", path, err)
	}
	data, err := osReadFileFn(abs)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, exitcode.InvalidArgsErrorf("workspace file not found: %s — create it with an ecosystem `name` and a `repos` list, or pass --workspace <path>", abs)
	}
	if err != nil {
		return nil, exitcode.InvalidArgsErrorf("reading workspace file %s: %v", abs, err)
	}
	var ws Workspace
	if err := yaml.Unmarshal(data, &ws); err != nil {
		return nil, exitcode.InvalidArgsErrorf("workspace file %s is not valid YAML: %v", abs, yamlErrOneLine(err))
	}
	if ws.Name == "" {
		return nil, exitcode.InvalidArgsErrorf("workspace file %s: missing required field `name` (ecosystem name)", abs)
	}
	if len(ws.Repos) == 0 {
		return nil, exitcode.InvalidArgsErrorf("workspace file %s: `repos` must list at least one repo path or glob", abs)
	}
	ws.Path = abs
	ws.Dir = filepath.Dir(abs)
	return &ws, nil
}

// ResolveRepos expands the workspace's repo entries into a deduplicated,
// entry-ordered list of existing absolute directory paths. Relative entries
// resolve against the workspace directory; entries containing glob
// metacharacters expand via filepath.Glob (matches sorted). Entries that do
// not name an existing directory are skipped silently — per-repo tolerance
// is handled downstream. Zero resolved directories returns an exit-2 error
// naming the workspace path.
func (ws *Workspace) ResolveRepos() ([]string, error) {
	seen := make(map[string]bool)
	var out []string
	for _, entry := range ws.Repos {
		p := entry
		if !filepath.IsAbs(p) {
			p = filepath.Join(ws.Dir, p)
		}
		matches := []string{p}
		if strings.ContainsAny(p, "*?[") {
			m, err := filepathGlobFn(p)
			if err != nil {
				return nil, exitcode.InvalidArgsErrorf("workspace file %s: bad glob pattern %q: %v", ws.Path, entry, err)
			}
			matches = m
		}
		for _, m := range matches {
			if info, err := os.Stat(m); err != nil || !info.IsDir() {
				continue
			}
			if !seen[m] {
				seen[m] = true
				out = append(out, m)
			}
		}
	}
	if len(out) == 0 {
		return nil, exitcode.InvalidArgsErrorf("workspace file %s: `repos` resolve to zero existing directories — check the paths and globs against %s", ws.Path, ws.Dir)
	}
	return out, nil
}

// DefaultDBPath returns the default fact-store path for the workspace:
// <workspace-dir>/.specscore-studio/facts.db.
func (ws *Workspace) DefaultDBPath() string {
	return filepath.Join(ws.Dir, ".specscore-studio", "facts.db")
}

// yamlErrOneLine collapses a (possibly multi-line) YAML error into one line
// so workspace errors stay one-line actionable.
func yamlErrOneLine(err error) string {
	return strings.Join(strings.Fields(err.Error()), " ")
}
