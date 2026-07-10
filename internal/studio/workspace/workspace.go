// Package workspace loads and resolves SpecScore Studio workspace files
// (studio.yaml): an ecosystem name plus a list of repo entries — directory
// paths or glob patterns, absolute or workspace-relative, each optionally
// carrying a per-repo `registries:` file list for the registries adapter.
//
// Feature: cli/studio/index (REQ: workspace-config, REQ: workspace-errors,
// REQ: adapter-registries)
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

// RepoEntry is one raw entry of the workspace `repos` list. It accepts two
// YAML forms — a plain string path, or a mapping `{path: ..., registries:
// [...]}` — so existing string-only workspaces keep parsing unchanged
// (REQ: adapter-registries).
type RepoEntry struct {
	// Path is the absolute or workspace-relative repo directory path; glob
	// patterns allowed (required).
	Path string `yaml:"path"`
	// Registries are optional repo-relative ops-registry file paths for the
	// registries adapter to parse in addition to its discovered defaults.
	Registries []string `yaml:"registries"`
}

// UnmarshalYAML accepts both entry forms: a scalar string decodes into Path;
// anything else decodes as the mapping form.
func (e *RepoEntry) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		return node.Decode(&e.Path)
	}
	type plain RepoEntry // drop methods to avoid recursing into this hook
	var p plain
	if err := node.Decode(&p); err != nil {
		return err
	}
	*e = RepoEntry(p)
	return nil
}

// Workspace is a parsed studio.yaml workspace file.
type Workspace struct {
	// Name is the ecosystem name (required).
	Name string `yaml:"name"`
	// Repos are the raw repo entries (required, non-empty).
	Repos []RepoEntry `yaml:"repos"`

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
	for i, entry := range ws.Repos {
		if entry.Path == "" {
			return nil, exitcode.InvalidArgsErrorf("workspace file %s: repos[%d] has no `path` — use a string entry or `{path: ..., registries: [...]}`", abs, i)
		}
	}
	ws.Path = abs
	ws.Dir = filepath.Dir(abs)
	return &ws, nil
}

// ResolveRepos expands the workspace's repo entries into a deduplicated,
// entry-ordered list of existing absolute directory paths, plus the
// deduplicated absolute paths of literal (non-glob) entries that name no
// existing directory. Relative entries resolve against the workspace
// directory; entries containing glob metacharacters expand via filepath.Glob
// (matches sorted) and match nothing silently. Missing literal paths are NOT
// an error here — the ingestion pipeline surfaces them as repo-level
// warnings (REQ: partial-tolerance). Zero resolved directories returns an
// exit-2 error naming the workspace path (REQ: workspace-errors).
func (ws *Workspace) ResolveRepos() (repos, missing []string, err error) {
	seen := make(map[string]bool)
	seenMissing := make(map[string]bool)
	for _, entry := range ws.Repos {
		matches, resolved, isGlob, err := ws.expandEntry(entry.Path)
		if err != nil {
			return nil, nil, err
		}
		if !isGlob && len(matches) == 0 {
			if !seenMissing[resolved] {
				seenMissing[resolved] = true
				missing = append(missing, resolved)
			}
			continue
		}
		for _, m := range matches {
			if !seen[m] {
				seen[m] = true
				repos = append(repos, m)
			}
		}
	}
	if len(repos) == 0 {
		return nil, nil, exitcode.InvalidArgsErrorf("workspace file %s: `repos` resolve to zero existing directories — check the paths and globs against %s", ws.Path, ws.Dir)
	}
	return repos, missing, nil
}

// ResolveRegistries maps each resolved repo directory (the paths ResolveRepos
// returns) to the per-repo `registries:` file lists configured for it.
// Duplicate repo entries merge; per-repo file lists dedupe, first occurrence
// wins the order. Bad globs and entries naming no existing directory are
// skipped silently — they are ResolveRepos's errors to report.
func (ws *Workspace) ResolveRegistries() map[string][]string {
	out := make(map[string][]string)
	for _, entry := range ws.Repos {
		if len(entry.Registries) == 0 {
			continue
		}
		matches, _, _, err := ws.expandEntry(entry.Path)
		if err != nil {
			continue
		}
		for _, m := range matches {
			out[m] = appendUnique(out[m], entry.Registries)
		}
	}
	return out
}

// expandEntry resolves one repo entry path (absolute, workspace-relative, or
// glob) to the existing directories it names, in glob-sorted order, plus the
// resolved absolute form of the entry and whether it was a glob pattern. A
// bad glob pattern returns an exit-2 error; missing paths simply match
// nothing.
func (ws *Workspace) expandEntry(entry string) (dirs []string, resolved string, isGlob bool, err error) {
	p := entry
	if !filepath.IsAbs(p) {
		p = filepath.Join(ws.Dir, p)
	}
	matches := []string{p}
	if strings.ContainsAny(p, "*?[") {
		isGlob = true
		m, err := filepathGlobFn(p)
		if err != nil {
			return nil, p, true, exitcode.InvalidArgsErrorf("workspace file %s: bad glob pattern %q: %v", ws.Path, entry, err)
		}
		matches = m
	}
	for _, m := range matches {
		if info, err := os.Stat(m); err == nil && info.IsDir() {
			dirs = append(dirs, m)
		}
	}
	return dirs, p, isGlob, nil
}

// appendUnique appends the items not already present in dst, preserving
// first-occurrence order.
func appendUnique(dst, items []string) []string {
	have := make(map[string]bool, len(dst))
	for _, d := range dst {
		have[d] = true
	}
	for _, it := range items {
		if !have[it] {
			have[it] = true
			dst = append(dst, it)
		}
	}
	return dst
}

// DefaultDBPath returns the default fact-store path for the workspace:
// <workspace-dir>/.specscore-studio/facts.db.
func (ws *Workspace) DefaultDBPath() string {
	return filepath.Join(ws.Dir, ".specscore-studio", "facts.db")
}

// DefaultIngrDir returns the default INGR export root for the workspace:
// <workspace-dir>/.specscore-studio/ingr (REQ: ingr-export).
func (ws *Workspace) DefaultIngrDir() string {
	return filepath.Join(ws.Dir, ".specscore-studio", "ingr")
}

// yamlErrOneLine collapses a (possibly multi-line) YAML error into one line
// so workspace errors stay one-line actionable.
func yamlErrOneLine(err error) string {
	return strings.Join(strings.Fields(err.Error()), " ")
}
