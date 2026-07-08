package graph

import (
	"sort"
	"strings"
)

// ListItem is one row of `graph list`. Owner is the owning module id, derived
// from placement (a module row owns itself).
type ListItem struct {
	ID    string `json:"id" yaml:"id"`
	Kind  string `json:"kind" yaml:"kind"`
	Name  string `json:"name" yaml:"name"`
	Owner string `json:"owner" yaml:"owner"`
	Path  string `json:"path" yaml:"path"`
}

// List returns every GraphSpec artifact (modules plus collection artifacts) as
// list rows, filtered by kind and module when non-empty, sorted by id. owner and
// module are derived from placement.
func List(g *Graph, kind, module string) []ListItem {
	var items []ListItem
	for _, m := range g.Modules {
		for _, a := range collectArtifacts(m) {
			effKind := artifactKind(a)
			if kind != "" && effKind != kind {
				continue
			}
			if module != "" && a.Module != module {
				continue
			}
			items = append(items, ListItem{
				ID:    listID(a),
				Kind:  effKind,
				Name:  a.Name,
				Owner: a.Module,
				Path:  relTo(g.RepoRoot, a.Path),
			})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].ID != items[j].ID {
			return items[i].ID < items[j].ID
		}
		return items[i].Path < items[j].Path
	})
	return items
}

// artifactKind returns an artifact's effective kind by placement (a module
// README is "module"; collection artifacts by their directory).
func artifactKind(a *Artifact) string {
	if a.CollectionDir == "" {
		return KindModule
	}
	return kindByCollection[a.CollectionDir]
}

// listID returns the identifier used in listings: the bare module id for a
// module README, the qualified id otherwise.
func listID(a *Artifact) string {
	if a.CollectionDir == "" {
		return a.Module
	}
	return a.QualifiedID()
}

// RefItem is one referencing artifact in `graph refs` output.
type RefItem struct {
	ID   string `json:"id" yaml:"id"`
	Kind string `json:"kind" yaml:"kind"`
	Path string `json:"path" yaml:"path"`
}

// ResolveRef resolves a <ref> argument to a single qualified id. It matches a
// qualified id first, then a shorthand (bare id or display name). candidates is
// populated (sorted) when the shorthand is ambiguous; found is false when
// nothing matches.
func ResolveRef(g *Graph, ref string) (qid string, candidates []string, found bool) {
	byQID := map[string]*Artifact{}
	var matches []string
	for _, m := range g.Modules {
		for _, a := range m.Artifacts {
			if a.ID == "" {
				continue
			}
			q := a.QualifiedID()
			byQID[q] = a
		}
	}
	if _, ok := byQID[ref]; ok {
		return ref, nil, true
	}
	seen := map[string]bool{}
	for q, a := range byQID {
		if a.ID == ref || a.Name == ref {
			if !seen[q] {
				seen[q] = true
				matches = append(matches, q)
			}
		}
	}
	sort.Strings(matches)
	switch len(matches) {
	case 0:
		return "", nil, false
	case 1:
		return matches[0], nil, true
	default:
		return "", matches, true
	}
}

// Refs returns the artifacts that reference targetQID (inbound), including
// edges derived from ModelSpec structure (an entity whose model carries an
// entity-reference property references the entity whose model is the target
// concept — association-object visibility). With transitive, it returns the
// full set of artifacts that can reach targetQID through reference chains,
// cycle-safe.
func Refs(g *Graph, targetQID string, transitive bool) []RefItem {
	byQID := map[string]*Artifact{}
	outbound := map[string]map[string]bool{}
	for _, m := range g.Modules {
		for _, a := range m.Artifacts {
			if a.ID == "" {
				continue
			}
			q := a.QualifiedID()
			byQID[q] = a
			outbound[q] = artifactRefs(a)
		}
	}
	addModelEdges(g, outbound)
	// reverse[target] = set of referencers.
	reverse := map[string]map[string]bool{}
	for src, targets := range outbound {
		for t := range targets {
			if reverse[t] == nil {
				reverse[t] = map[string]bool{}
			}
			reverse[t][src] = true
		}
	}

	result := map[string]bool{}
	visited := map[string]bool{targetQID: true}
	stack := []string{targetQID}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for referencer := range reverse[cur] {
			if result[referencer] {
				continue
			}
			result[referencer] = true
			if transitive && !visited[referencer] {
				visited[referencer] = true
				stack = append(stack, referencer)
			}
		}
		if !transitive {
			break
		}
	}

	var items []RefItem
	for q := range result {
		a := byQID[q]
		items = append(items, RefItem{ID: q, Kind: artifactKind(a), Path: relTo(g.RepoRoot, a.Path)})
	}
	// IDs are unique here (result is keyed by qualified id), so no tie-break.
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items
}

// addModelEdges augments the outbound reference map with edges derived from
// ModelSpec structure (ModelSpec core-model licenses graph consumers to derive
// edges from entity-reference properties). For every artifact whose model
// concept declares a reference to another concept that is itself some
// artifact's model, an edge between the two artifacts is added. This is what
// keeps association-object entities (a relationship promoted to an entity, its
// endpoints now model properties) visible to `graph refs`.
func addModelEdges(g *Graph, outbound map[string]map[string]bool) {
	// conceptToQID maps a local "<module>.<Concept>" to the qualified id of
	// the graph artifact whose model: names it.
	conceptToQID := map[string]string{}
	for _, m := range g.Modules {
		for _, a := range m.Artifacts {
			if a.ID == "" || a.Model == "" {
				continue
			}
			msr, err := ParseModelspecRef(a.Model)
			if err != nil || msr.Repo != "" {
				continue
			}
			conceptToQID[msr.Module+"."+msr.Name] = a.QualifiedID()
		}
	}
	for _, m := range g.Modules {
		if m.Model == nil {
			continue
		}
		for _, r := range m.Model.Refs {
			srcQID, ok := conceptToQID[m.ID+"."+r.Owner]
			if !ok {
				continue
			}
			target := r.Target
			if !strings.Contains(target, ".") {
				target = m.ID + "." + target
			}
			dstQID, ok := conceptToQID[target]
			if !ok || dstQID == srcQID {
				continue
			}
			outbound[srcQID][dstQID] = true
		}
	}
}

// artifactRefs returns the set of qualified graph references an artifact makes
// (from/to/subject/actors/participants/inputs.ref/possibleEvents plus qualified
// graph refs used as metadata values). modelspec:// references are excluded.
func artifactRefs(a *Artifact) map[string]bool {
	out := map[string]bool{}
	add := func(ref string) {
		if _, ok := ParseQualifiedRef(ref); ok {
			out[ref] = true
		}
	}
	add(a.From)
	add(a.To)
	add(a.Subject)
	for _, r := range a.Actors {
		add(r)
	}
	for _, r := range a.Participants {
		add(r)
	}
	for _, r := range a.PossibleEvents {
		add(r)
	}
	for _, in := range a.Inputs {
		if in.HasRef {
			add(in.Ref)
		}
	}
	for _, e := range a.Metadata {
		if e.Scalar {
			add(e.Value)
		}
	}
	return out
}
