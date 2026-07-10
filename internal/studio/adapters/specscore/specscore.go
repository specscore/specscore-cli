// Package specscore is the SpecScore spec-tree ingestion adapter: it parses
// `spec/` trees of repos managed by SpecScore (marked by a `specscore.yaml`
// at the repo root) into idea/feature entities and `declared` facts.
//
// Feature: cli/studio/index (REQ: adapter-specscore)
//
// Emitted predicates:
//   - has-status    — a feature's or idea's `**Status:**` line
//   - has-ac        — one fact per `### AC: <slug>` heading in a feature
//   - contains      — feature parent/child directory containment
//   - sourced-from  — feature → idea per the `**Source Ideas:**` line
//   - promotes-to   — idea → feature per the `**Promotes To:**` line
//
// Subjects and objects that are spec entities are emitted repo-relative
// (`#<path-slug>`): features as `#<feature-id>` and ideas as `#ideas/<slug>`.
// The pipeline prefixes them with the workspace-scoped repo slug (fact-shape
// spec IDs: `<repo>#<path-slug>`).
package specscore

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/internal/studio/fact"
	"github.com/specscore/specscore-cli/pkg/feature"
	"github.com/specscore/specscore-cli/pkg/idea"
)

const (
	adapterID = "specscore"
	// adapterVersion bumps when the emitted fact vocabulary or shapes change.
	adapterVersion = "0.1.0"
)

// Test seams — package-level vars wrapping external functions.
// Production code calls these vars; tests replace them via t.Cleanup.
var (
	featureDiscoverFn    = feature.Discover
	parseFeatureStatusFn = feature.ParseFeatureStatus
	featureSourceIdeasFn = idea.FeatureSourceIdeas
	ideaDiscoverFn       = idea.Discover
	ideaParseFn          = idea.Parse
	osReadFileFn         = os.ReadFile
	filepathRelFn        = filepath.Rel
)

// Adapter is the SpecScore spec-tree ingestion adapter. The zero value is
// ready to use.
type Adapter struct{}

// New returns the adapter.
func New() Adapter { return Adapter{} }

// ID returns the stable adapter id stamped into every emitted fact.
func (Adapter) ID() string { return adapterID }

// Version returns the adapter version stamped into every emitted fact.
func (Adapter) Version() string { return adapterVersion }

// Ingest parses the spec tree of the repo at repoPath. A repo without a
// specscore.yaml is not SpecScore-managed and yields no facts and no
// warnings. Per-file parse failures become warnings and skip only that file
// (REQ: partial-tolerance).
func (Adapter) Ingest(repoPath string) ([]fact.Fact, []fact.Warning) {
	if _, err := os.Stat(filepath.Join(repoPath, "specscore.yaml")); err != nil {
		return nil, nil // not a SpecScore-managed repo
	}
	var facts []fact.Fact
	var warnings []fact.Warning
	warnf := func(format string, args ...any) {
		warnings = append(warnings, fact.Warning{Message: fmt.Sprintf(format, args...)})
	}
	specRoot := filepath.Join(repoPath, "spec")
	facts = append(facts, ingestFeatures(specRoot, warnf)...)
	facts = append(facts, ingestIdeas(repoPath, specRoot, warnf)...)
	return facts, warnings
}

// declared builds a declared-class fact; adapter/observed-at/ecosystem are
// stamped centrally by the pipeline.
func declared(subject, predicate, object, pointer string) fact.Fact {
	return fact.Fact{
		Subject:   subject,
		Predicate: predicate,
		Object:    object,
		Evidence:  fact.Evidence{Class: fact.Declared, Pointer: pointer},
	}
}

// ingestFeatures emits has-status, has-ac, contains, and sourced-from facts
// for every feature directory under <specRoot>/features.
func ingestFeatures(specRoot string, warnf func(string, ...any)) []fact.Fact {
	featuresDir := filepath.Join(specRoot, "features")
	if info, err := os.Stat(featuresDir); err != nil || !info.IsDir() {
		return nil // repo declares specscore.yaml but has no features tree
	}
	features, err := featureDiscoverFn(featuresDir)
	if err != nil {
		warnf("discovering features under %s: %v", featuresDir, err)
		return nil
	}
	isFeature := make(map[string]bool, len(features))
	for _, f := range features {
		isFeature[f.ID] = true
	}

	var facts []fact.Fact
	for _, f := range features {
		subject := "#" + f.ID
		readmeRel := path.Join("spec", "features", f.ID, "README.md")
		readmeAbs := filepath.Join(featuresDir, filepath.FromSlash(f.ID), "README.md")

		status, err := parseFeatureStatusFn(readmeAbs)
		switch {
		case err != nil:
			warnf("reading status of feature %s: %v", f.ID, err)
		case status != "Unknown": // "Unknown" = no **Status:** line declared
			facts = append(facts, declared(subject, "has-status", status, readmeRel))
		}

		acs, err := parseACs(readmeAbs)
		if err != nil {
			warnf("reading acceptance criteria of feature %s: %v", f.ID, err)
		}
		for _, ac := range acs {
			pointer := fmt.Sprintf("%s:%d", readmeRel, ac.line)
			facts = append(facts, declared(subject, "has-ac", ac.slug, pointer))
		}

		if parent := path.Dir(f.ID); parent != "." && isFeature[parent] {
			facts = append(facts, declared("#"+parent, "contains", subject, readmeRel))
		}
	}

	sourceIdeas, err := featureSourceIdeasFn(specRoot)
	if err != nil {
		warnf("reading feature Source Ideas under %s: %v", featuresDir, err)
		return facts
	}
	featureSlugs := make([]string, 0, len(sourceIdeas))
	for slug := range sourceIdeas {
		featureSlugs = append(featureSlugs, slug)
	}
	sort.Strings(featureSlugs)
	for _, slug := range featureSlugs {
		readmeRel := path.Join("spec", "features", slug, "README.md")
		for _, ideaSlug := range sourceIdeas[slug] {
			facts = append(facts, declared("#"+slug, "sourced-from", "#ideas/"+ideaSlug, readmeRel))
		}
	}
	return facts
}

// ingestIdeas emits has-status and promotes-to facts for every idea artifact
// under <specRoot>/ideas (active, archived, and feature proposals — all keyed
// `#ideas/<slug>` by file slug).
func ingestIdeas(repoPath, specRoot string, warnf func(string, ...any)) []fact.Fact {
	ideas, err := ideaDiscoverFn(specRoot)
	if err != nil {
		warnf("discovering ideas under %s: %v", specRoot, err)
		return nil
	}
	var facts []fact.Fact
	for _, d := range ideas {
		parsed, err := ideaParseFn(d.Path)
		if err != nil {
			warnf("parsing idea %s: %v", d.Slug, err)
			continue
		}
		pointer, err := filepathRelFn(repoPath, d.Path)
		if err != nil {
			warnf("resolving idea path %s: %v", d.Path, err)
			continue
		}
		pointer = filepath.ToSlash(pointer)
		subject := "#ideas/" + d.Slug
		if status := parsed.Status(); status != "" {
			facts = append(facts, declared(subject, "has-status", status, pointer))
		}
		for _, target := range parsed.PromotesTo() {
			facts = append(facts, declared(subject, "promotes-to", "#"+target, pointer))
		}
	}
	return facts
}

// acRef is one `### AC: <slug>` heading with its 1-based line number.
type acRef struct {
	slug string
	line int
}

// acHeadingRe matches `### AC: <ac-slug>` headings; the slug captures up to
// the first whitespace or `(` (the verifies-parenthetical).
var acHeadingRe = regexp.MustCompile(`^###\s+AC:\s+(\S+?)(?:\s|\(|$)`)

// parseACs returns the AC headings under `## Acceptance Criteria` in the
// feature README at path, in file order. It mirrors pkg/lint's unexported
// feature-AC parser; kept minimal here rather than exporting from the lint
// package (which the CLI treats as a rule engine, not a parsing library).
func parseACs(readmePath string) ([]acRef, error) {
	data, err := osReadFileFn(readmePath)
	if err != nil {
		return nil, err
	}
	var out []acRef
	inACs := false
	for i, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if title, ok := strings.CutPrefix(trimmed, "## "); ok {
			inACs = strings.TrimSpace(title) == "Acceptance Criteria"
			continue
		}
		if !inACs {
			continue
		}
		if m := acHeadingRe.FindStringSubmatch(trimmed); m != nil {
			if slug := strings.TrimRight(m[1], ":"); slug != "" {
				out = append(out, acRef{slug: slug, line: i + 1})
			}
		}
	}
	return out, nil
}
