package graph

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/pkg/lint"
)

// LintOptions configures a graph lint pass.
type LintOptions struct {
	RepoRoot string   // absolute repo root (contains specscore.yaml)
	Root     string   // graph-root override relative to RepoRoot; "" = spec/graph
	Rules    []string // enabled rules; nil = all
	Ignore   []string // disabled rules
	Severity string   // minimum severity: error|warning|info; "" = no filter
}

// LintResult is the outcome of a graph lint pass. NoGraphRoot is true when the
// repository has no graph root — a notice case, not a violation.
type LintResult struct {
	Violations  []lint.Violation
	NoGraphRoot bool
}

// graphRuleSeverity maps each graph rule id to its catalog severity.
var graphRuleSeverity = func() map[string]string {
	m := map[string]string{}
	for _, r := range lint.GraphRules() {
		m[r.ID] = r.Severity
	}
	return m
}()

// GraphRuleNames returns the sorted set of valid graph lint rule names.
func GraphRuleNames() []string {
	rules := lint.GraphRules()
	out := make([]string, 0, len(rules))
	for _, r := range rules {
		out = append(out, r.ID)
	}
	sort.Strings(out)
	return out
}

// ValidateRuleNames returns an error naming the first unknown graph rule.
func ValidateRuleNames(names []string) error {
	known := map[string]bool{}
	for _, n := range GraphRuleNames() {
		known[n] = true
	}
	for _, n := range names {
		if !known[n] {
			return fmt.Errorf("unknown graph rule %q", n)
		}
	}
	return nil
}

// Lint discovers the graph root and runs every enabled graph rule.
func Lint(opts LintOptions) (LintResult, error) {
	g, ok, err := Load(opts.RepoRoot, opts.Root)
	if err != nil {
		return LintResult{}, err
	}
	if !ok {
		return LintResult{NoGraphRoot: true}, nil
	}
	l := &linter{g: g, opts: opts, res: BuildResolver(opts.RepoRoot, g)}
	l.buildIndex()
	l.buildClosures()
	l.run()

	vs := l.vs
	if opts.Severity != "" {
		vs = lint.FilterBySeverity(vs, opts.Severity)
	}
	sort.SliceStable(vs, func(i, j int) bool {
		if vs[i].File != vs[j].File {
			return vs[i].File < vs[j].File
		}
		if vs[i].Line != vs[j].Line {
			return vs[i].Line < vs[j].Line
		}
		if vs[i].Rule != vs[j].Rule {
			return vs[i].Rule < vs[j].Rule
		}
		return vs[i].Message < vs[j].Message
	})
	return LintResult{Violations: vs}, nil
}

// linter carries the per-pass state.
type linter struct {
	g        *Graph
	opts     LintOptions
	res      *Resolver
	index    map[string][]*Artifact     // qualified id -> artifacts
	closures map[string]map[string]bool // module id -> transitive dependsOn set
	vs       []lint.Violation
}

func (l *linter) buildIndex() {
	l.index = map[string][]*Artifact{}
	for _, m := range l.g.Modules {
		for _, a := range collectArtifacts(m) {
			if a.ID != "" {
				qid := a.QualifiedID()
				l.index[qid] = append(l.index[qid], a)
			}
		}
	}
}

// buildClosures computes each module's transitive dependsOn set (excluding
// itself). Missing modules named in dependsOn are still followed by name.
func (l *linter) buildClosures() {
	dep := map[string][]string{}
	for _, m := range l.g.Modules {
		dep[m.ID] = m.DependsOn
	}
	l.closures = map[string]map[string]bool{}
	for _, m := range l.g.Modules {
		seen := map[string]bool{}
		var stack []string
		stack = append(stack, m.DependsOn...)
		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if seen[cur] {
				continue
			}
			seen[cur] = true
			stack = append(stack, dep[cur]...)
		}
		l.closures[m.ID] = seen
	}
}

// collectArtifacts returns a module's README (if any) plus its collection
// artifacts.
func collectArtifacts(m *Module) []*Artifact {
	var out []*Artifact
	if m.Readme != nil {
		out = append(out, m.Readme)
	}
	out = append(out, m.Artifacts...)
	return out
}

func (l *linter) enabled(rule string) bool {
	if len(l.opts.Rules) > 0 {
		return slices.Contains(l.opts.Rules, rule)
	}
	if len(l.opts.Ignore) > 0 {
		return !slices.Contains(l.opts.Ignore, rule)
	}
	return true
}

func (l *linter) rel(abs string) string {
	r, err := filepath.Rel(l.g.RepoRoot, abs)
	if err != nil {
		return abs
	}
	return filepath.ToSlash(r)
}

// emit records a violation at the rule's catalog severity.
func (l *linter) emit(rule, abs string, line int, msg string) {
	l.emitSev(rule, abs, line, graphRuleSeverity[rule], msg)
}

// emitSev records a violation with an explicit severity (for mixed-severity
// rules such as graph-event-sources).
func (l *linter) emitSev(rule, abs string, line int, sev, msg string) {
	if !l.enabled(rule) {
		return
	}
	l.vs = append(l.vs, lint.Violation{
		File:     l.rel(abs),
		Line:     line,
		Severity: sev,
		Rule:     rule,
		Message:  msg,
	})
}

func (l *linter) run() {
	for _, m := range l.g.Modules {
		for _, a := range collectArtifacts(m) {
			l.checkArtifact(m, a)
		}
		if m.Model != nil {
			l.checkModel(m)
		}
	}
	l.checkDuplicateIDs()
	l.checkDuplicateModuleIDs()
}

// checkDuplicateModuleIDs reports a GraphSpec module id declared by more than
// one graph root in the union (decision 0009), naming both module paths.
func (l *linter) checkDuplicateModuleIDs() {
	groups := map[string][]*Module{}
	for _, m := range l.g.Modules {
		groups[m.ID] = append(groups[m.ID], m)
	}
	for id, mods := range groups {
		if len(mods) < 2 {
			continue
		}
		paths := make([]string, 0, len(mods))
		for _, m := range mods {
			paths = append(paths, l.rel(m.Root))
		}
		sort.Strings(paths)
		for _, m := range mods {
			line := 0
			path := filepath.Join(m.Root, "README.md")
			if m.Readme != nil {
				line = m.Readme.KeyLine("id")
			}
			l.emit("graph-duplicate-module-id", path, line,
				fmt.Sprintf("module id %q is declared by multiple graph roots: %s", id, strings.Join(paths, ", ")))
		}
	}
}

func (l *linter) checkArtifact(m *Module, a *Artifact) {
	if !a.HasFrontmatter {
		l.emit("graph-kind-valid", a.Path, 1, a.FMError)
		return
	}
	l.checkKind(a)
	l.checkIDRules(m, a)
	l.checkOwner(a)
	l.checkInlineStructure(a)

	expected := KindModule
	if a.CollectionDir != "" {
		expected = kindByCollection[a.CollectionDir]
	}
	switch expected {
	case KindEntity:
		l.checkEntity(m, a)
	case KindRelationship:
		l.checkRelationship(m, a)
	case KindCommand:
		l.checkCommand(m, a)
	case KindEvent:
		l.checkEvent(m, a)
	}
}

func (l *linter) checkKind(a *Artifact) {
	expected := KindModule
	if a.CollectionDir != "" {
		expected = kindByCollection[a.CollectionDir]
	}
	line := a.KeyLine("kind")
	if !isKind(a.Kind) {
		l.emit("graph-kind-valid", a.Path, line,
			fmt.Sprintf("kind %q is not one of module|entity|relationship|command|event", a.Kind))
		return
	}
	if a.Kind != expected {
		l.emit("graph-kind-valid", a.Path, line,
			fmt.Sprintf("kind %q does not match its collection directory (expected %q)", a.Kind, expected))
	}
}

func (l *linter) checkIDRules(m *Module, a *Artifact) {
	line := a.KeyLine("id")
	if a.CollectionDir == "" {
		if a.ID != m.ID {
			l.emit("graph-id-equals-filename-stem", a.Path, line,
				fmt.Sprintf("module id %q must equal its directory name %q", a.ID, m.ID))
		}
	} else if a.ID != a.Stem {
		l.emit("graph-id-equals-filename-stem", a.Path, line,
			fmt.Sprintf("id %q must equal filename stem %q", a.ID, a.Stem))
	}
	if a.ID != "" && !IsKebab(a.ID) {
		l.emit("graph-id-kebab-case", a.Path, line,
			fmt.Sprintf("id %q must be bare lowercase kebab-case", a.ID))
	}
	if strings.Contains(a.ID, ".") {
		l.emit("graph-no-module-prefix-in-id", a.Path, line,
			fmt.Sprintf("id %q must not contain a module prefix (dot); the qualified form is computed", a.ID))
	}
}

func (l *linter) checkOwner(a *Artifact) {
	if a.HasKey("owner") {
		l.emit("graph-no-owner-field", a.Path, a.KeyLine("owner"),
			"owner: is not allowed; ownership is derived from placement under the module root")
	}
}

func (l *linter) checkInlineStructure(a *Artifact) {
	for _, k := range []string{"fields", "properties"} {
		if a.HasKey(k) {
			l.emit("graph-no-inline-structure", a.Path, a.KeyLine(k),
				fmt.Sprintf("%s: is not allowed in a graph artifact; structure lives in the referenced ModelSpec model", k))
		}
	}
}

func (l *linter) checkEntity(m *Module, a *Artifact) {
	if a.HasKey("model") && a.Model != "" {
		l.checkModelspecRef(m, a.Path, a.Model, "model:", a.KeyLine("model"))
	}
	if a.LifecycleStatesPresent {
		l.checkLifecycle(a)
	}
}

func (l *linter) checkLifecycle(a *Artifact) {
	line := a.KeyLine("lifecycle.states")
	if len(a.LifecycleStates) == 0 {
		l.emit("graph-lifecycle-states", a.Path, line, "lifecycle.states must be a non-empty list")
		return
	}
	seen := map[string]bool{}
	for _, s := range a.LifecycleStates {
		if seen[s] {
			l.emit("graph-lifecycle-states", a.Path, line,
				fmt.Sprintf("duplicate lifecycle state %q", s))
		}
		seen[s] = true
	}
}

func (l *linter) checkRelationship(m *Module, a *Artifact) {
	l.checkGraphRef(m, a.Path, a.From, "from", a.KeyLine("from"))
	l.checkGraphRef(m, a.Path, a.To, "to", a.KeyLine("to"))
	l.checkMetadata(m, a)
	l.checkOwnerCoversEndpoints(m, a)
}

func (l *linter) checkOwnerCoversEndpoints(m *Module, a *Artifact) {
	type ep struct {
		module string
		line   int
	}
	var eps []ep
	if qr, ok := ParseQualifiedRef(a.From); ok {
		eps = append(eps, ep{qr.Module, a.KeyLine("from")})
	}
	if qr, ok := ParseQualifiedRef(a.To); ok {
		eps = append(eps, ep{qr.Module, a.KeyLine("to")})
	}
	closure := l.closures[m.ID]
	for _, e := range eps {
		if e.module == m.ID || closure[e.module] {
			continue
		}
		l.emit("graph-relationship-owner-covers-endpoints", a.Path, e.line,
			fmt.Sprintf("endpoint module %q is not covered by module %q dependsOn closure", e.module, m.ID))
	}
}

func (l *linter) checkMetadata(m *Module, a *Artifact) {
	for _, e := range a.Metadata {
		if !e.Scalar {
			msg := "metadata must be a flat map"
			if e.Key != "" {
				msg = fmt.Sprintf("metadata value for %q must be a scalar, qualified graph ref, or modelspec:// ref; nested maps/lists are not allowed", e.Key)
			}
			l.emit("graph-metadata-shape", a.Path, e.Line, msg)
			continue
		}
		switch {
		case strings.HasPrefix(e.Value, ModelspecScheme):
			l.checkModelspecRef(m, a.Path, e.Value, "metadata "+e.Key, e.Line)
		default:
			if qr, ok := ParseQualifiedRef(e.Value); ok {
				l.checkModuleDependency(m, qr.Module, a.Path, e.Line,
					fmt.Sprintf("metadata %q reference %q", e.Key, e.Value))
				if !l.refExists(e.Value) {
					l.emit("graph-reference-resolves", a.Path, e.Line,
						fmt.Sprintf("metadata %q reference %q does not resolve to an existing artifact", e.Key, e.Value))
				}
			}
		}
	}
}

func (l *linter) checkCommand(m *Module, a *Artifact) {
	l.checkGraphRef(m, a.Path, a.Subject, "subject", a.KeyLine("subject"))
	for _, act := range a.Actors {
		l.checkGraphRef(m, a.Path, act, "actors", a.KeyLine("actors"))
	}
	for _, ev := range a.PossibleEvents {
		l.checkGraphRef(m, a.Path, ev, "possibleEvents", a.KeyLine("possibleEvents"))
	}
	l.checkInputs(m, a)
}

func (l *linter) checkInputs(m *Module, a *Artifact) {
	if a.inputsMalformed {
		l.emit("graph-inputs-shape", a.Path, a.KeyLine("inputs"),
			"inputs must be a list of {name, ref|model} items")
	}
	seen := map[string]bool{}
	for _, in := range a.Inputs {
		label := in.Name
		if label == "" {
			label = "(unnamed)"
		}
		if !in.HasName || in.Name == "" {
			l.emit("graph-inputs-shape", a.Path, in.Line, "input item is missing a `name`")
		} else {
			if !IsKebab(in.Name) {
				l.emit("graph-inputs-shape", a.Path, in.Line,
					fmt.Sprintf("input name %q must be kebab-case", in.Name))
			}
			if seen[in.Name] {
				l.emit("graph-inputs-shape", a.Path, in.Line,
					fmt.Sprintf("duplicate input name %q", in.Name))
			}
			seen[in.Name] = true
		}
		cnt := 0
		if in.HasRef {
			cnt++
		}
		if in.HasModel {
			cnt++
		}
		if cnt != 1 {
			l.emit("graph-inputs-shape", a.Path, in.Line,
				fmt.Sprintf("input %q must carry exactly one of `ref` or `model`", label))
		}
		if len(in.ExtraKeys) > 0 {
			l.emit("graph-inputs-shape", a.Path, in.Line,
				fmt.Sprintf("input %q has unexpected key(s): %s (commands carry no structure of their own)",
					label, strings.Join(in.ExtraKeys, ", ")))
		}
		if in.HasRef && in.Ref != "" {
			l.checkGraphRef(m, a.Path, in.Ref, "inputs.ref", in.Line)
		}
		if in.HasModel && in.Model != "" {
			l.checkModelspecRef(m, a.Path, in.Model, "inputs.model", in.Line)
		}
	}
}

func (l *linter) checkEvent(m *Module, a *Artifact) {
	l.checkGraphRef(m, a.Path, a.Subject, "subject", a.KeyLine("subject"))
	for _, p := range a.Participants {
		l.checkGraphRef(m, a.Path, p, "participants", a.KeyLine("participants"))
	}
	l.checkEventSources(a)
}

func (l *linter) checkEventSources(a *Artifact) {
	line := a.KeyLine("sources")
	if a.HasKey("sources") && len(a.Sources) == 0 {
		l.emitSev("graph-event-sources", a.Path, line, "warning",
			"omit `sources` instead of an explicit empty `sources: []`")
	}
	for _, s := range a.Sources {
		if _, ok := ParseQualifiedRef(s); !ok {
			continue
		}
		for _, art := range l.index[s] {
			if art.CollectionDir == "commands" {
				l.emitSev("graph-event-sources", a.Path, line, "error",
					fmt.Sprintf("event `sources` must not reference commands (%q); command triggers derive from CommandSpec possibleEvents", s))
				break
			}
		}
	}
}

// checkGraphRef validates a graph-to-graph qualified reference: it must be a
// valid <module>.<id> form, the module must be in the owner's dependsOn, and it
// must resolve to an existing artifact.
func (l *linter) checkGraphRef(owner *Module, ownerPath, ref, field string, line int) {
	if ref == "" {
		return
	}
	qr, ok := ParseQualifiedRef(ref)
	if !ok {
		l.emit("graph-reference-resolves", ownerPath, line,
			fmt.Sprintf("%s reference %q is not a valid <module>.<id> reference", field, ref))
		return
	}
	l.checkModuleDependency(owner, qr.Module, ownerPath, line,
		fmt.Sprintf("%s reference %q", field, ref))
	if !l.refExists(ref) {
		l.emit("graph-reference-resolves", ownerPath, line,
			fmt.Sprintf("%s reference %q does not resolve to an existing artifact", field, ref))
	}
}

func (l *linter) refExists(qid string) bool {
	return len(l.index[qid]) > 0
}

// checkModuleDependency enforces dependency-direction: targetModule must be the
// owner itself or listed in the owner's dependsOn.
func (l *linter) checkModuleDependency(owner *Module, targetModule, filePath string, line int, ctx string) {
	if targetModule == owner.ID {
		return
	}
	if slices.Contains(owner.DependsOn, targetModule) {
		return
	}
	l.emit("graph-dependency-direction", filePath, line,
		fmt.Sprintf("%s targets module %q which is not in module %q dependsOn", ctx, targetModule, owner.ID))
}

// checkModelspecRef validates a modelspec:// reference and its dependency
// direction (decision 0007).
func (l *linter) checkModelspecRef(owner *Module, ownerPath, ref, field string, line int) {
	msr, ok := ParseModelspecRef(ref)
	if !ok {
		l.emit("graph-model-ref-resolves", ownerPath, line,
			fmt.Sprintf("%s reference %q is not a valid modelspec:// reference", field, ref))
		return
	}
	if msr.Suffix == "" {
		l.checkModuleDependency(owner, msr.Module, ownerPath, line,
			fmt.Sprintf("%s reference %q", field, ref))
	}
	switch l.res.resolveConcept(msr.Module, msr.Name, msr.Suffix) {
	case resResolved:
	case resUnknownModule:
		l.emit("graph-model-ref-resolves", ownerPath, line,
			fmt.Sprintf("%s reference %q: unknown module %q", field, ref, msr.Module))
	case resUnknownConcept:
		l.emit("graph-model-ref-resolves", ownerPath, line,
			fmt.Sprintf("%s reference %q: unknown concept %q in module %q", field, ref, msr.Name, msr.Module))
	case resRepoUnavailable:
		l.emit("graph-model-ref-resolves", ownerPath, line,
			fmt.Sprintf("%s reference %q: unresolved (repository %q not available)", field, ref, msr.Suffix))
	case resAmbiguousModule:
		l.emit("graph-model-ref-resolves", ownerPath, line,
			fmt.Sprintf("%s reference %q: module %q is ambiguous across configured projects", field, ref, msr.Module))
	}
}

func (l *linter) checkModel(m *Module) {
	mm := m.Model
	for _, d := range mm.ParseErrors {
		l.emit("graph-model-ref-resolves", d.File, d.Line,
			fmt.Sprintf("cannot parse ModelSpec source: %s", d.Message))
	}
	seen := map[string]*Concept{}
	for _, c := range mm.Concepts {
		if prev, ok := seen[c.Name]; ok {
			l.emit("graph-model-duplicate-concept", c.File, c.Line,
				fmt.Sprintf("duplicate concept name %q (also declared at line %d)", c.Name, prev.Line))
			continue
		}
		seen[c.Name] = c
	}
	for _, c := range mm.Concepts {
		if c.Kind != "enum" {
			continue
		}
		if len(c.EnumValues) == 0 {
			l.emit("graph-model-enum-values", c.File, c.Line,
				fmt.Sprintf("enum %q must declare at least one value", c.Name))
		}
		vseen := map[string]bool{}
		for _, v := range c.EnumValues {
			if vseen[v] {
				l.emit("graph-model-enum-values", c.File, c.Line,
					fmt.Sprintf("enum %q has duplicate value %q", c.Name, v))
			}
			vseen[v] = true
		}
	}
	for _, ref := range mm.Refs {
		l.checkModelRef(m, ref)
	}
}

func (l *linter) checkModelRef(m *Module, ref *ModelRef) {
	dot := strings.IndexByte(ref.Target, '.')
	if dot < 0 {
		if m.Model == nil || !m.Model.HasConcept(ref.Target) {
			l.emit("graph-model-ref-resolves", ref.File, ref.Line,
				fmt.Sprintf("model %s reference %q does not resolve to a concept in module %q", ref.Attr, ref.Target, m.ID))
		}
		return
	}
	module := ref.Target[:dot]
	name := ref.Target[dot+1:]
	l.checkModuleDependency(m, module, ref.File, ref.Line,
		fmt.Sprintf("model %s reference %q", ref.Attr, ref.Target))
	switch l.res.resolveConcept(module, name, "") {
	case resResolved:
	case resUnknownModule:
		l.emit("graph-model-ref-resolves", ref.File, ref.Line,
			fmt.Sprintf("model %s reference %q: unknown module %q", ref.Attr, ref.Target, module))
	case resUnknownConcept:
		l.emit("graph-model-ref-resolves", ref.File, ref.Line,
			fmt.Sprintf("model %s reference %q: unknown concept %q in module %q", ref.Attr, ref.Target, name, module))
	case resAmbiguousModule:
		l.emit("graph-model-ref-resolves", ref.File, ref.Line,
			fmt.Sprintf("model %s reference %q: module %q is ambiguous across configured projects", ref.Attr, ref.Target, module))
	case resRepoUnavailable:
		// unreachable: bare/qualified HCL refs carry no @-suffix.
	}
}

func (l *linter) checkDuplicateIDs() {
	groups := map[string][]*Artifact{}
	for _, m := range l.g.Modules {
		for _, a := range collectArtifacts(m) {
			if a.ID == "" {
				continue
			}
			groups[a.QualifiedID()] = append(groups[a.QualifiedID()], a)
		}
	}
	for qid, arts := range groups {
		if len(arts) < 2 {
			continue
		}
		paths := make([]string, 0, len(arts))
		for _, a := range arts {
			paths = append(paths, l.rel(a.Path))
		}
		sort.Strings(paths)
		for _, a := range arts {
			l.emit("graph-duplicate-id", a.Path, a.KeyLine("id"),
				fmt.Sprintf("duplicate qualified id %q defined by: %s", qid, strings.Join(paths, ", ")))
		}
	}
}
