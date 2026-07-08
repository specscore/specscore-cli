package graph

import (
	"os"
	"path/filepath"
	"slices"

	"github.com/specscore/specscore-cli/pkg/projectdef"
)

// resolution is the outcome of resolving a modelspec:// or HCL qualified
// reference (decision 0007 three-way diagnostics plus ambiguity, the decision
// 0013 enum-value fragment outcomes, and success).
type resolution int

const (
	resResolved             resolution = iota // module + concept both found (and any fragment names an enum value)
	resUnknownModule                          // module not found in any resolution step
	resUnknownConcept                         // module resolved, concept absent from it
	resKindMismatch                           // concept exists under a different kind than requested
	resRepoUnavailable                        // cross-repo target not locally available
	resAmbiguousModule                        // module found in multiple configured projects
	resFragmentNotEnum                        // fragment on a concept that is not an enum (decision 0013)
	resFragmentUnknownValue                   // fragment names a value the enum does not declare (decision 0013)
)

// conceptResult carries the resolution outcome plus, for resKindMismatch and
// resFragmentNotEnum, the kind the concept name actually has, and the resolved
// concept on success.
type conceptResult struct {
	outcome    resolution
	actualKind string
	concept    *Concept // set when outcome is resResolved
}

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

// resolveConcept resolves a module/name reference to a concept, returning the
// diagnostic outcome. kind is the singular concept kind for a kind-explicit
// reference, or "" for the kind-free (trio) form. repo is "host/org/repo" for a
// cross-repo reference, or "" for a local one. fragment is a '#<value>'
// enum-value address (decision 0013), or "": a non-empty fragment resolves only
// when the concept is an enum declaring that value. The advisory ?ref= pin does
// not change resolution (decision 0010): local resolution proceeds regardless.
func (r *Resolver) resolveConcept(module, name, kind, repo, fragment string) conceptResult {
	if repo != "" {
		byID, ok := r.repos[repo]
		if !ok {
			return conceptResult{outcome: resRepoUnavailable}
		}
		return conceptOutcome(byID[module], kind, name, fragment, byID[module] != nil)
	}
	if mm, ok := r.local[module]; ok {
		return conceptOutcome(mm, kind, name, fragment, true)
	}
	mms := r.project[module]
	switch {
	case len(mms) == 1:
		return conceptOutcome(mms[0], kind, name, fragment, true)
	case len(mms) > 1:
		return conceptResult{outcome: resAmbiguousModule}
	default:
		return conceptResult{outcome: resUnknownModule}
	}
}

// conceptOutcome maps a resolved-or-not module plus a concept lookup to a
// resolution outcome.
func conceptOutcome(mm *ModelModule, kind, name, fragment string, moduleFound bool) conceptResult {
	if !moduleFound || mm == nil {
		return conceptResult{outcome: resUnknownModule}
	}
	l := mm.lookupConcept(kind, name)
	switch {
	case l.found:
		return fragmentOutcome(l.concept, fragment)
	case l.exists:
		return conceptResult{outcome: resKindMismatch, actualKind: l.actualKind}
	default:
		return conceptResult{outcome: resUnknownConcept}
	}
}

// fragmentOutcome applies decision-0013 enum-value fragment resolution to a
// resolved concept: a non-empty fragment resolves only when the concept is an
// enum and the fragment names one of its declared values.
func fragmentOutcome(c *Concept, fragment string) conceptResult {
	if fragment == "" {
		return conceptResult{outcome: resResolved, concept: c}
	}
	if c.Kind != "enum" {
		return conceptResult{outcome: resFragmentNotEnum, actualKind: c.Kind}
	}
	if !slices.Contains(c.EnumValues, fragment) {
		return conceptResult{outcome: resFragmentUnknownValue}
	}
	return conceptResult{outcome: resResolved, concept: c}
}
