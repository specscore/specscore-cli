package graph

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/pkg/projectdef"
)

// DefaultGraphRoot is the default repository-level graph root relative to the
// repo root (decision 0005). Per decision 0009 a repository's graph is the
// UNION of this root plus one per configured SpecScore module.
const DefaultGraphRoot = "spec/graph"

// Module is one GraphSpec module discovered under a graph root's
// modules/<id>/ directory. Identity is by placement: ID is the directory name
// (decision 0006).
type Module struct {
	ID        string
	Root      string      // absolute path to the module directory
	Readme    *Artifact   // parsed README.md frontmatter, or nil when absent
	DependsOn []string    // declared outbound dependencies (from README)
	Artifacts []*Artifact // collection artifacts owned by this module
	ModelDir  string      // absolute path to models/, or "" when absent
	Model     *ModelModule
}

// Graph is a fully discovered graph: the union of the repository's graph roots
// and their modules (decision 0009).
type Graph struct {
	Root     string   // absolute repo-level graph root (may not exist)
	Roots    []string // absolute paths of every existing graph root in the union
	RepoRoot string   // absolute repo root (contains specscore.yaml)
	Modules  []*Module
}

// AllArtifacts returns every collection artifact across all modules, plus each
// module README, in a deterministic order (module id, then path).
func (g *Graph) AllArtifacts() []*Artifact {
	var out []*Artifact
	for _, m := range g.Modules {
		if m.Readme != nil {
			out = append(out, m.Readme)
		}
		out = append(out, m.Artifacts...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Module != out[j].Module {
			return out[i].Module < out[j].Module
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// ModuleByID returns the first module with the given id, or nil.
func (g *Graph) ModuleByID(id string) *Module {
	for _, m := range g.Modules {
		if m.ID == id {
			return m
		}
	}
	return nil
}

// FindGraphRoot resolves the repository-level graph root for repoRoot. rootRel
// overrides the default spec/graph when non-empty. It returns the absolute path
// and whether the directory exists.
func FindGraphRoot(repoRoot, rootRel string) (string, bool) {
	rel := rootRel
	if rel == "" {
		rel = DefaultGraphRoot
	}
	abs := filepath.Join(repoRoot, filepath.FromSlash(rel))
	info, err := os.Stat(abs)
	return abs, err == nil && info.IsDir()
}

// GraphRootDirs enumerates the union of existing graph roots for repoRoot: the
// repository-level root (rootRel override or spec/graph) plus one per configured
// SpecScore module (<module.path>/<specs_dir>/graph), resolved through the
// existing repo-config machinery (decision 0009). Absent roots are omitted;
// duplicates (same cleaned path) are collapsed. Order is repo-level first, then
// module roots in configuration order.
func GraphRootDirs(repoRoot, rootRel string) []string {
	var roots []string
	seen := map[string]bool{}
	add := func(abs string) {
		info, err := os.Stat(abs)
		if err != nil || !info.IsDir() {
			return
		}
		c := filepath.Clean(abs)
		if !seen[c] {
			seen[c] = true
			roots = append(roots, c)
		}
	}
	repoLevel, _ := FindGraphRoot(repoRoot, rootRel)
	add(repoLevel)
	if cfg, err := projectdef.ReadSpecConfig(repoRoot); err == nil {
		specsDir := cfg.EffectiveSpecsDirName()
		for _, m := range cfg.EffectiveModules() {
			add(filepath.Join(repoRoot, filepath.FromSlash(m.EffectivePath()), specsDir, "graph"))
		}
	}
	return roots
}

// Load discovers the union of graph roots for repoRoot (with an optional
// rootRel override on the repository-level root). ok is false when NO graph root
// exists — callers treat that as the "no graph root" notice case, not an error.
func Load(repoRoot, rootRel string) (g *Graph, ok bool, err error) {
	roots := GraphRootDirs(repoRoot, rootRel)
	if len(roots) == 0 {
		return nil, false, nil
	}
	repoLevel, _ := FindGraphRoot(repoRoot, rootRel)
	g = &Graph{Root: repoLevel, Roots: roots, RepoRoot: repoRoot}
	for _, root := range roots {
		modulesDir := filepath.Join(root, "modules")
		entries, rerr := readDirFn(modulesDir)
		if rerr != nil {
			if os.IsNotExist(rerr) {
				continue // a root with no modules/ directory is empty but valid
			}
			return nil, false, rerr
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			m, merr := loadModule(filepath.Join(modulesDir, e.Name()), e.Name())
			if merr != nil {
				return nil, false, merr
			}
			g.Modules = append(g.Modules, m)
		}
	}
	sort.Slice(g.Modules, func(i, j int) bool {
		if g.Modules[i].ID != g.Modules[j].ID {
			return g.Modules[i].ID < g.Modules[j].ID
		}
		return g.Modules[i].Root < g.Modules[j].Root
	})
	return g, true, nil
}

// loadModule reads one module directory: its README, its collection artifacts,
// and its ModelSpec sources.
func loadModule(dir, id string) (*Module, error) {
	m := &Module{ID: id, Root: dir}

	readmePath := filepath.Join(dir, "README.md")
	if _, err := os.Stat(readmePath); err == nil {
		art, perr := ParseArtifact(readmePath, id, "")
		if perr != nil {
			return nil, perr
		}
		m.Readme = art
		m.DependsOn = art.DependsOn
	}

	for _, coll := range ArtifactCollections {
		collDir := filepath.Join(dir, coll)
		files, err := readDirFn(collDir)
		if err != nil {
			continue // collection dir absent — fine
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".md") || f.Name() == "README.md" {
				continue
			}
			art, perr := ParseArtifact(filepath.Join(collDir, f.Name()), id, coll)
			if perr != nil {
				return nil, perr
			}
			m.Artifacts = append(m.Artifacts, art)
		}
	}
	sort.Slice(m.Artifacts, func(i, j int) bool { return m.Artifacts[i].Path < m.Artifacts[j].Path })

	modelsDir := filepath.Join(dir, "models")
	if info, err := os.Stat(modelsDir); err == nil && info.IsDir() {
		m.ModelDir = modelsDir
		mod, perr := LoadModelModule(modelsDir, id)
		if perr != nil {
			return nil, perr
		}
		m.Model = mod
	}
	return m, nil
}
