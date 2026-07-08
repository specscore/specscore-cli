package graph

import (
	"os"
	"path/filepath"

	"github.com/specscore/specscore-cli/pkg/projectdef"
)

// resolution is the outcome of resolving a modelspec:// or HCL qualified
// reference (decision 0007 three-way diagnostics plus ambiguity and success).
type resolution int

const (
	resResolved        resolution = iota // module + concept both found
	resUnknownModule                     // module not found in any resolution step
	resUnknownConcept                    // module resolved, concept absent from it
	resRepoUnavailable                   // @-suffixed repo not locally available
	resAmbiguousModule                   // module found in multiple configured projects
)

// Resolver resolves module short names to ModelSpec modules across the three
// decision-0007 steps: local graph root, configured `projects:` local paths,
// then explicit @{host}/{org}/{repo} suffixes for locally-available repos.
type Resolver struct {
	local   map[string]*ModelModule            // local module id -> model module
	project map[string][]*ModelModule          // configured-project module id -> models
	repos   map[string]map[string]*ModelModule // "host/org/repo" -> (module id -> model)
}

// BuildResolver constructs the resolver for graph g rooted at repoRoot, reading
// repoRoot/specscore.yaml for configured `projects:` local paths. A missing or
// unreadable config yields a local-only resolver — never an error, so lint
// stays deterministic and offline.
func BuildResolver(repoRoot string, g *Graph) *Resolver {
	r := &Resolver{
		local:   map[string]*ModelModule{},
		project: map[string][]*ModelModule{},
		repos:   map[string]map[string]*ModelModule{},
	}
	for _, m := range g.Modules {
		if m.Model != nil {
			r.local[m.ID] = m.Model
		}
	}
	cfg, err := projectdef.ReadSpecConfig(repoRoot)
	if err != nil {
		return r
	}
	for _, entry := range cfg.Projects {
		dir := entry
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(repoRoot, entry)
		}
		info, statErr := os.Stat(dir)
		if statErr != nil || !info.IsDir() {
			continue // not a local path — skip (URLs and absent dirs)
		}
		pg, ok, loadErr := Load(dir, "")
		if loadErr != nil || !ok {
			continue
		}
		byID := map[string]*ModelModule{}
		for _, m := range pg.Modules {
			if m.Model == nil {
				continue
			}
			r.project[m.ID] = append(r.project[m.ID], m.Model)
			byID[m.ID] = m.Model
		}
		if key := projectIdentity(dir); key != "" {
			r.repos[key] = byID
		}
	}
	return r
}

// projectIdentity returns the "host/org/repo" identity declared in dir's
// specscore.yaml project block, or "" when unavailable.
func projectIdentity(dir string) string {
	cfg, err := projectdef.ReadSpecConfig(dir)
	if err != nil || cfg.Project == nil {
		return ""
	}
	p := cfg.Project
	if p.Host == "" || p.Org == "" || p.Repo == "" {
		return ""
	}
	return p.Host + "/" + p.Org + "/" + p.Repo
}

// resolveConcept resolves a module/name reference (with an optional cross-repo
// suffix) to a concept, returning the three-way diagnostic outcome.
func (r *Resolver) resolveConcept(module, name, suffix string) resolution {
	if suffix != "" {
		repo, ok := r.repos[suffix]
		if !ok {
			return resRepoUnavailable
		}
		return conceptOutcome(repo[module], name, ok && repo[module] != nil)
	}
	if mm, ok := r.local[module]; ok {
		return conceptOutcome(mm, name, true)
	}
	mms := r.project[module]
	switch {
	case len(mms) == 1:
		return conceptOutcome(mms[0], name, true)
	case len(mms) > 1:
		return resAmbiguousModule
	default:
		return resUnknownModule
	}
}

// conceptOutcome maps a resolved-or-not module plus a concept lookup to a
// resolution outcome.
func conceptOutcome(mm *ModelModule, name string, moduleFound bool) resolution {
	if !moduleFound || mm == nil {
		return resUnknownModule
	}
	if mm.HasConcept(name) {
		return resResolved
	}
	return resUnknownConcept
}
