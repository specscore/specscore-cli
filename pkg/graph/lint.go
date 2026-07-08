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
	Fix      bool     // when true, apply the specified graph fixers on disk
}

// LintResult is the outcome of a graph lint pass. NoGraphRoot is true when the
// repository has no graph root — a notice case, not a violation. Fixed lists
// the repo-root-relative paths of files changed by the fix pass (only when
// opts.Fix is true).
type LintResult struct {
	Violations  []lint.Violation
	NoGraphRoot bool
	Fixed       []string
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
	var fixed []string
	if opts.Fix {
		fixed, err = fixLegacyModelspecForms(g)
		if err != nil {
			return LintResult{}, err
		}
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
	return LintResult{Violations: vs, Fixed: fixed}, nil
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
	l.checkEventReachability()
}

// checkEventReachability reports (info) events that no command can produce and
// no non-command source feeds: they are declared facts nothing emits. An event
// with a `sources:` key (even a lint-warned empty one) is considered fed.
// Draft events are exempt — a freshly scaffolded event is always initially
// unreachable (same stance as the non-draft model: warning on entities).
func (l *linter) checkEventReachability() {
	possible := map[string]bool{}
	for _, m := range l.g.Modules {
		for _, a := range m.Artifacts {
			for _, ev := range a.PossibleEvents {
				possible[ev] = true
			}
		}
	}
	for _, m := range l.g.Modules {
		for _, a := range m.Artifacts {
			if a.CollectionDir != "events" || !a.HasFrontmatter || a.Status == "draft" {
				continue
			}
			if a.HasKey("sources") || possible[a.QualifiedID()] {
				continue
			}
			l.emit("graph-event-reachability", a.Path, a.KeyLine("id"),
				fmt.Sprintf("event %q is in no command's possibleEvents and declares no sources; nothing can produce it", a.QualifiedID()))
		}
	}
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
	l.checkRoleIssues(a)
	l.checkUnknownKeys(a)

	expected := KindModule
	if a.CollectionDir != "" {
		expected = kindByCollection[a.CollectionDir]
	}
	switch expected {
	case KindEntity:
		l.checkEntity(m, a)
		l.checkRules(m, a)
	case KindRelationship:
		l.checkRelationship(m, a)
		l.checkRules(m, a)
	case KindCommand:
		l.checkCommand(m, a)
		l.checkRules(m, a)
	case KindEvent:
		l.checkEvent(m, a)
	case KindPolicy:
		l.checkPolicy(m, a)
	}
}

// checkRoleIssues reports decision-0012 endpoint/participant shape violations
// collected at parse time.
func (l *linter) checkRoleIssues(a *Artifact) {
	for _, ri := range a.RoleIssues {
		l.emit("graph-role-labels", a.Path, ri.Line, ri.Message)
	}
}

// knownKeysByKind maps each GraphSpec kind to its allowed top-level frontmatter
// keys. Keys outside the set are reported by graph-unknown-key (warning): they
// are silently meaningless to tooling — e.g. lifecycle: on a relationship.
var knownKeysByKind = map[string]map[string]bool{
	KindModule:       keySet("kind", "id", "name", "status", "summary", "dependsOn"),
	KindEntity:       keySet("kind", "id", "name", "status", "summary", "model", "lifecycle", "rules"),
	KindRelationship: keySet("kind", "id", "name", "status", "summary", "from", "to", "cardinality", "metadata", "rules"),
	KindCommand:      keySet("kind", "id", "name", "status", "summary", "subject", "actors", "inputs", "possibleEvents", "rules"),
	KindEvent:        keySet("kind", "id", "name", "status", "summary", "subject", "participants", "sources"),
	KindPolicy:       keySet("kind", "id", "name", "status", "summary", "applies", "when", "requires", "invariant"),
}

func keySet(keys ...string) map[string]bool {
	m := make(map[string]bool, len(keys))
	for _, k := range keys {
		m[k] = true
	}
	return m
}

// checkUnknownKeys warns on frontmatter keys the artifact's placement-derived
// kind does not define. Keys already rejected by dedicated rules (owner,
// fields, properties) are skipped to avoid double-reporting.
func (l *linter) checkUnknownKeys(a *Artifact) {
	expected := KindModule
	if a.CollectionDir != "" {
		expected = kindByCollection[a.CollectionDir]
	}
	known := knownKeysByKind[expected]
	var unknown []string
	for k := range a.presentKeys {
		if known[k] || k == "owner" || k == "fields" || k == "properties" {
			continue
		}
		unknown = append(unknown, k)
	}
	sort.Strings(unknown)
	for _, k := range unknown {
		l.emit("graph-unknown-key", a.Path, a.KeyLine(k),
			fmt.Sprintf("key %q is not defined for kind %q and is ignored by tooling", k, expected))
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
			fmt.Sprintf("kind %q is not one of module|entity|relationship|command|event|policy", a.Kind))
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
	// A self-referential relationship without role labels leaves direction as
	// the only carrier of meaning (decision 0012).
	if a.From != "" && a.From == a.To && (a.FromRole == "" || a.ToRole == "") {
		l.emit("graph-ambiguous-endpoints", a.Path, a.KeyLine("from"),
			fmt.Sprintf("both endpoints reference %q; label them with roles ({ref: %s, role: ...}) so the sides are machine-readable (decision 0012)", a.From, a.From))
	}
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
	l.checkDuplicateParticipants(a)
}

// checkDuplicateParticipants warns when two participants carry the same
// reference without distinguishing role labels (decision 0012): either
// copy-paste noise or two roles the author could not express before roles
// existed.
func (l *linter) checkDuplicateParticipants(a *Artifact) {
	seen := map[string]int{} // ref+role -> first line
	for _, p := range a.ParticipantItems {
		if p.Ref == "" {
			continue
		}
		key := p.Ref + "\x00" + p.Role
		if first, ok := seen[key]; ok {
			l.emit("graph-ambiguous-endpoints", a.Path, p.Line,
				fmt.Sprintf("participant %q duplicates line %d without a distinguishing role label (decision 0012)", p.Ref, first))
			continue
		}
		seen[key] = p.Line
	}
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

// withArticle prefixes a concept kind with its indefinite article ("an entity",
// "an enum", "a component").
func withArticle(kind string) string {
	switch kind[0] {
	case 'a', 'e', 'i', 'o', 'u':
		return "an " + kind
	default:
		return "a " + kind
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

// checkModelspecRef validates a modelspec:// reference (decisions 0007/0010/
// 0011) and its dependency direction. The legacy authority-empty-path form is a
// distinct error carrying the exact rewrite so `graph lint --fix` can repair it.
func (l *linter) checkModelspecRef(owner *Module, ownerPath, ref, field string, line int) {
	msr, err := ParseModelspecRef(ref)
	if err != nil {
		if pe, ok := isLegacyForm(err); ok {
			l.emit("graph-model-legacy-form", ownerPath, line,
				fmt.Sprintf("%s reference %q uses the legacy modelspec form; rewrite to %q", field, ref, pe.Rewrite))
			return
		}
		l.emit("graph-model-ref-resolves", ownerPath, line,
			fmt.Sprintf("%s reference %q is not a valid modelspec:// reference: %v", field, ref, err))
		return
	}
	if msr.Repo == "" {
		l.checkModuleDependency(owner, msr.Module, ownerPath, line,
			fmt.Sprintf("%s reference %q", field, ref))
	}
	res := l.res.resolveConcept(msr.Module, msr.Name, msr.Kind, msr.Repo, msr.Fragment)
	switch res.outcome {
	case resResolved:
	case resUnknownModule:
		l.emit("graph-model-ref-resolves", ownerPath, line,
			fmt.Sprintf("%s reference %q: unknown module %q", field, ref, msr.Module))
	case resUnknownConcept:
		l.emit("graph-model-ref-resolves", ownerPath, line,
			fmt.Sprintf("%s reference %q: unknown concept %q in module %q", field, ref, msr.Name, msr.Module))
	case resKindMismatch:
		l.emit("graph-model-ref-resolves", ownerPath, line,
			fmt.Sprintf("%s reference %q: concept %q in module %q is %s, not %s", field, ref, msr.Name, msr.Module, withArticle(res.actualKind), withArticle(msr.Kind)))
	case resRepoUnavailable:
		l.emit("graph-model-ref-resolves", ownerPath, line,
			fmt.Sprintf("%s reference %q: unresolved (repository %q not available)", field, ref, msr.Repo))
	case resAmbiguousModule:
		l.emit("graph-model-ref-resolves", ownerPath, line,
			fmt.Sprintf("%s reference %q: module %q is ambiguous across configured projects", field, ref, msr.Module))
	case resFragmentNotEnum:
		l.emit("graph-model-ref-resolves", ownerPath, line,
			fmt.Sprintf("%s reference %q: concept %q is %s, not an enum — fragments address enum values (decision 0013)", field, ref, msr.Name, withArticle(res.actualKind)))
	case resFragmentUnknownValue:
		l.emit("graph-model-ref-resolves", ownerPath, line,
			fmt.Sprintf("%s reference %q: enum %q has no value %q", field, ref, msr.Name, msr.Fragment))
	}
}

func (l *linter) checkModel(m *Module) {
	mm := m.Model
	for _, d := range mm.ParseErrors {
		l.emit("graph-model-ref-resolves", d.File, d.Line,
			fmt.Sprintf("cannot parse ModelSpec source: %s", d.Message))
	}
	// Reserved-token concept names are forbidden in every scope (decision 0011 /
	// ModelSpec decision 0015): they are the kind segments of reference syntax.
	for _, c := range mm.Concepts {
		if ReservedConceptNames[c.Name] {
			l.emit("graph-model-reserved-name", c.File, c.Line,
				fmt.Sprintf("%s name %q is a reserved kind token (entities, components, enums, collections, recordsets) and cannot name a concept", c.Kind, c.Name))
		}
	}
	// Duplicate detection is per name scope: the entity/component/enum trio
	// shares one scope; collections and recordsets each have their own. A
	// module and a same-named entity never collide — modules are bare-ID
	// citizens, not concepts (decisions 0011).
	seen := map[string]*Concept{}
	for _, c := range mm.Concepts {
		key := conceptScope(c.Kind) + "\x00" + c.Name
		if prev, ok := seen[key]; ok {
			l.emit("graph-model-duplicate-concept", c.File, c.Line,
				fmt.Sprintf("duplicate concept name %q in the %s scope (also declared at line %d)", c.Name, conceptScope(c.Kind), prev.Line))
			continue
		}
		seen[key] = c
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
	// HCL qualified names are kind-free (the attribute is the kind selector,
	// decision 0011), never cross-repo, and never carry fragments, so they
	// resolve in the trio scope.
	res := l.res.resolveConcept(module, name, "", "", "")
	switch res.outcome {
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
	}
	// resKindMismatch and resRepoUnavailable are unreachable here: HCL qualified
	// names are kind-free and never cross-repo.
}

func (l *linter) checkDuplicateIDs() {
	groups := map[string][]*Artifact{}
	for _, m := range l.g.Modules {
		for _, a := range collectArtifacts(m) {
			// Module READMEs are bare-ID citizens, not qualified-ID concepts
			// (decision 0011): a module never occupies a `<m>.<m>` slot, so a
			// module and a same-named entity coexist. Module-id uniqueness is
			// enforced separately by checkDuplicateModuleIDs. Only collection
			// artifacts carry qualified IDs.
			if a.ID == "" || a.CollectionDir == "" {
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

// --- decision 0013: Tier-1 rules: blocks and the policy kind ---

// checkRules validates a Tier-1 rules: block (decision 0013) on an entity,
// relationship, or command artifact: a list of {id, text, refs?} maps with
// artifact-unique kebab-case ids, non-empty text, and resolvable refs.
func (l *linter) checkRules(m *Module, a *Artifact) {
	if a.rulesMalformed {
		l.emit("graph-rules-shape", a.Path, a.KeyLine("rules"),
			"rules must be a list of {id, text, refs} maps")
	}
	seen := map[string]bool{}
	for _, r := range a.Rules {
		label := r.ID
		if label == "" {
			label = "(unnamed)"
		}
		if !r.HasID || r.ID == "" {
			l.emit("graph-rules-shape", a.Path, r.Line, "rule item is missing an `id`")
		} else {
			if !IsKebab(r.ID) {
				l.emit("graph-rules-shape", a.Path, r.Line,
					fmt.Sprintf("rule id %q must be bare lowercase kebab-case", r.ID))
			}
			if seen[r.ID] {
				l.emit("graph-rules-shape", a.Path, r.Line,
					fmt.Sprintf("duplicate rule id %q", r.ID))
			}
			seen[r.ID] = true
		}
		if !r.HasText || r.Text == "" {
			l.emit("graph-rules-shape", a.Path, r.Line,
				fmt.Sprintf("rule %q must carry a non-empty `text`", label))
		}
		if len(r.ExtraKeys) > 0 {
			l.emit("graph-rules-shape", a.Path, r.Line,
				fmt.Sprintf("rule %q has unexpected key(s): %s (only id, text, and refs are allowed)",
					label, strings.Join(r.ExtraKeys, ", ")))
		}
		if r.refsMalformed {
			l.emit("graph-rules-shape", a.Path, r.Line,
				fmt.Sprintf("rule %q refs must be a list of qualified graph or modelspec:// references", label))
		}
		for _, ref := range r.Refs {
			if strings.HasPrefix(ref, ModelspecScheme) {
				l.checkModelspecRef(m, a.Path, ref, "rules.refs", r.Line)
				continue
			}
			l.checkGraphRef(m, a.Path, ref, "rules.refs", r.Line)
		}
	}
}

// checkPolicy validates a PolicySpec artifact's clause shape and operands
// (decision 0013): the applies target, when/requires/invariant clauses, and
// dependency direction for every referenced module.
func (l *linter) checkPolicy(m *Module, a *Artifact) {
	appliesArt := l.checkPolicyApplies(m, a)
	l.checkPolicyWhen(m, a, appliesArt)
	l.checkPolicyRequires(m, a)
	l.checkPolicyInvariant(m, a, appliesArt)
}

// checkPolicyApplies validates the required applies: block and returns the
// resolved target artifact (nil when missing, malformed, or unresolved).
func (l *linter) checkPolicyApplies(m *Module, a *Artifact) *Artifact {
	if !a.HasKey("applies") {
		l.emit("graph-policy-shape", a.Path, a.KeyLine("kind"),
			"policy must declare an `applies:` block with exactly one of command|entity|relationship (decision 0013)")
		return nil
	}
	line := a.KeyLine("applies")
	cm := a.applies
	if cm.Malformed {
		l.emit("graph-policy-shape", a.Path, line,
			"applies must be a map carrying exactly one of command|entity|relationship")
		return nil
	}
	l.checkClauseExtraKeys(a, cm, "applies", KindCommand, KindEntity, KindRelationship)
	if a.AppliesKind == "" {
		l.emit("graph-policy-shape", a.Path, line,
			"applies must carry exactly one of command|entity|relationship")
		return nil
	}
	return l.checkPolicyRef(m, a, a.AppliesRef, a.AppliesKind, "applies."+a.AppliesKind, line)
}

// checkPolicyWhen validates the optional when: clause list: each clause is
// exactly one of {input} (only for command policies, naming an input of the
// applies command) or {is-role: {relationship, role}}.
func (l *linter) checkPolicyWhen(m *Module, a *Artifact, appliesArt *Artifact) {
	if a.whenMalformed {
		l.emit("graph-policy-shape", a.Path, a.KeyLine("when"), "when must be a list of clause maps")
	}
	for _, c := range a.when {
		if c.Malformed {
			l.emit("graph-policy-shape", a.Path, c.Line,
				"when clause must be a map carrying exactly one of `input` or `is-role`")
			continue
		}
		l.checkClauseExtraKeys(a, c, "when clause", "input", "is-role")
		hasInput := slices.Contains(c.Keys, "input")
		hasIsRole := slices.Contains(c.Keys, "is-role")
		if hasInput == hasIsRole {
			l.emit("graph-policy-shape", a.Path, c.Line,
				"when clause must carry exactly one of `input` or `is-role`")
			continue
		}
		if hasInput {
			l.checkPolicyWhenInput(a, c, appliesArt)
			continue
		}
		l.checkPolicyIsRole(m, a, c)
	}
}

// checkPolicyWhenInput validates a when {input} clause: only legal when the
// policy applies to a command, and the value must name one of its inputs.
func (l *linter) checkPolicyWhenInput(a *Artifact, c *clauseMap, appliesArt *Artifact) {
	name := c.Scalars["input"]
	if name == "" {
		l.emit("graph-policy-shape", a.Path, c.Line, "when.input must be a non-empty input name")
		return
	}
	if a.AppliesKind != KindCommand {
		l.emit("graph-policy-shape", a.Path, c.Line,
			"when.input is only valid when applies names a command (decision 0013)")
		return
	}
	if appliesArt == nil {
		return // the applies reference itself already failed to resolve
	}
	for _, in := range appliesArt.Inputs {
		if in.Name == name {
			return
		}
	}
	l.emit("graph-policy-shape", a.Path, c.Line,
		fmt.Sprintf("when.input %q is not an input of command %q", name, a.AppliesRef))
}

// checkPolicyIsRole validates a when {is-role: {relationship, role}} clause.
func (l *linter) checkPolicyIsRole(m *Module, a *Artifact, c *clauseMap) {
	nested, ok := c.Nested["is-role"]
	if !ok {
		l.emit("graph-policy-shape", a.Path, c.Line, "is-role must be a {relationship, role} map")
		return
	}
	rel, role := l.checkClauseKeys(a, nested, "is-role", "relationship", "role")
	if rel != "" {
		l.checkPolicyRef(m, a, rel, KindRelationship, "is-role.relationship", nested.Line)
	}
	if role != "" && !IsKebab(role) {
		l.emit("graph-policy-shape", a.Path, nested.Line,
			fmt.Sprintf("is-role role %q must be a bare lowercase kebab-case token", role))
	}
}

// checkPolicyRequires validates the optional requires: clause list: each
// clause is exactly one of {entity, in-state} or {actor-is: {entity,
// model-role}}.
func (l *linter) checkPolicyRequires(m *Module, a *Artifact) {
	if a.requiresMalformed {
		l.emit("graph-policy-shape", a.Path, a.KeyLine("requires"), "requires must be a list of clause maps")
	}
	for _, c := range a.requires {
		if c.Malformed {
			l.emit("graph-policy-shape", a.Path, c.Line,
				"requires clause must be a map: {entity, in-state} or {actor-is: {entity, model-role}}")
			continue
		}
		l.checkClauseExtraKeys(a, c, "requires clause", "entity", "in-state", "actor-is")
		hasEntity := slices.Contains(c.Keys, "entity")
		hasInState := slices.Contains(c.Keys, "in-state")
		hasActorIs := slices.Contains(c.Keys, "actor-is")
		switch {
		case hasActorIs && !hasEntity && !hasInState:
			l.checkPolicyActorIs(m, a, c)
		case !hasActorIs && hasEntity && hasInState:
			l.checkPolicyEntityState(m, a, c.Scalars["entity"], c.Scalars["in-state"], "requires", c.Line)
		default:
			l.emit("graph-policy-shape", a.Path, c.Line,
				"requires clause must carry exactly one of {entity, in-state} or {actor-is: {entity, model-role}}")
		}
	}
}

// checkPolicyActorIs validates a requires {actor-is: {entity, model-role}}
// clause: the entity must resolve and its ModelSpec model concept must declare
// a property named model-role.
func (l *linter) checkPolicyActorIs(m *Module, a *Artifact, c *clauseMap) {
	nested, ok := c.Nested["actor-is"]
	if !ok {
		l.emit("graph-policy-shape", a.Path, c.Line, "actor-is must be a {entity, model-role} map")
		return
	}
	entity, role := l.checkClauseKeys(a, nested, "actor-is", "entity", "model-role")
	var ent *Artifact
	if entity != "" {
		ent = l.checkPolicyRef(m, a, entity, KindEntity, "actor-is.entity", nested.Line)
	}
	if role == "" || ent == nil {
		return
	}
	if ent.Model == "" {
		l.emit("graph-policy-shape", a.Path, nested.Line,
			fmt.Sprintf("actor-is model-role %q cannot be validated: entity %q declares no model", role, ent.QualifiedID()))
		return
	}
	msr, err := ParseModelspecRef(ent.Model)
	if err != nil {
		return // the entity's own model: reference already fails graph-model-ref-resolves
	}
	res := l.res.resolveConcept(msr.Module, msr.Name, msr.Kind, msr.Repo, "")
	if res.outcome != resResolved {
		return // already reported on the entity artifact
	}
	if !slices.Contains(res.concept.Properties, role) {
		l.emit("graph-policy-shape", a.Path, nested.Line,
			fmt.Sprintf("actor-is model-role %q is not a property of entity %q model concept %q", role, ent.QualifiedID(), msr.Name))
	}
}

// checkPolicyInvariant validates the optional invariant: clause list, only
// legal when the policy applies to an entity: each clause carries
// {when-referenced: {entity, in-state}} plus {then: {self-state}} where
// self-state is a lifecycle state of the applies entity.
func (l *linter) checkPolicyInvariant(m *Module, a *Artifact, appliesArt *Artifact) {
	if !a.HasKey("invariant") {
		return
	}
	line := a.KeyLine("invariant")
	if a.AppliesKind != "" && a.AppliesKind != KindEntity {
		l.emit("graph-policy-shape", a.Path, line,
			"invariant is only valid when applies names an entity (decision 0013)")
		return
	}
	if a.invariantMalformed {
		l.emit("graph-policy-shape", a.Path, line, "invariant must be a list of clause maps")
	}
	for _, c := range a.invariant {
		if c.Malformed {
			l.emit("graph-policy-shape", a.Path, c.Line,
				"invariant clause must be a map: {when-referenced: {entity, in-state}, then: {self-state}}")
			continue
		}
		l.checkClauseExtraKeys(a, c, "invariant clause", "when-referenced", "then")
		if !slices.Contains(c.Keys, "when-referenced") || !slices.Contains(c.Keys, "then") {
			l.emit("graph-policy-shape", a.Path, c.Line,
				"invariant clause must carry both `when-referenced` and `then`")
			continue
		}
		l.checkPolicyWhenReferenced(m, a, c)
		l.checkPolicyThen(a, c, appliesArt)
	}
}

// checkPolicyWhenReferenced validates an invariant when-referenced operand.
func (l *linter) checkPolicyWhenReferenced(m *Module, a *Artifact, c *clauseMap) {
	nested, ok := c.Nested["when-referenced"]
	if !ok {
		l.emit("graph-policy-shape", a.Path, c.Line, "when-referenced must be a {entity, in-state} map")
		return
	}
	l.checkClauseExtraKeys(a, nested, "when-referenced", "entity", "in-state")
	l.checkPolicyEntityState(m, a, nested.Scalars["entity"], nested.Scalars["in-state"], "when-referenced", nested.Line)
}

// checkPolicyThen validates an invariant then operand: {self-state} where the
// state belongs to the applies entity's lifecycle.
func (l *linter) checkPolicyThen(a *Artifact, c *clauseMap, appliesArt *Artifact) {
	nested, ok := c.Nested["then"]
	if !ok {
		l.emit("graph-policy-shape", a.Path, c.Line, "then must be a {self-state} map")
		return
	}
	l.checkClauseExtraKeys(a, nested, "then", "self-state")
	state := nested.Scalars["self-state"]
	if state == "" {
		l.emit("graph-policy-shape", a.Path, nested.Line, "then must carry a non-empty `self-state`")
		return
	}
	if appliesArt == nil {
		return // the applies reference itself already failed to resolve
	}
	l.checkPolicyStateMembership(a, appliesArt, state, "then.self-state", nested.Line)
}

// checkPolicyEntityState validates an {entity, in-state} operand pair: the
// entity resolves, and in-state is one of its lifecycle states.
func (l *linter) checkPolicyEntityState(m *Module, a *Artifact, entity, state, field string, line int) {
	var ent *Artifact
	if entity == "" {
		l.emit("graph-policy-shape", a.Path, line,
			fmt.Sprintf("%s must carry a non-empty qualified `entity` reference", field))
	} else {
		ent = l.checkPolicyRef(m, a, entity, KindEntity, field+".entity", line)
	}
	if state == "" {
		l.emit("graph-policy-shape", a.Path, line,
			fmt.Sprintf("%s must carry a non-empty `in-state` token", field))
		return
	}
	if ent == nil {
		return
	}
	l.checkPolicyStateMembership(a, ent, state, field+".in-state", line)
}

// checkPolicyStateMembership verifies that state is one of ent's declared
// lifecycle.states; an entity without a lifecycle is itself an error here.
func (l *linter) checkPolicyStateMembership(a *Artifact, ent *Artifact, state, field string, line int) {
	if len(ent.LifecycleStates) == 0 {
		l.emit("graph-policy-shape", a.Path, line,
			fmt.Sprintf("%s %q cannot be validated: entity %q has no lifecycle", field, state, ent.QualifiedID()))
		return
	}
	if !slices.Contains(ent.LifecycleStates, state) {
		l.emit("graph-policy-shape", a.Path, line,
			fmt.Sprintf("%s %q is not a lifecycle state of entity %q (states: %s)",
				field, state, ent.QualifiedID(), strings.Join(ent.LifecycleStates, ", ")))
	}
}

// checkPolicyRef validates a policy clause reference that must resolve to an
// artifact of a specific kind: valid <module>.<id> form, dependency direction,
// and kind-checked resolution against the qualified-id index. It returns the
// resolved artifact, or nil.
func (l *linter) checkPolicyRef(m *Module, a *Artifact, ref, wantKind, field string, line int) *Artifact {
	qr, ok := ParseQualifiedRef(ref)
	if !ok {
		l.emit("graph-policy-shape", a.Path, line,
			fmt.Sprintf("%s reference %q is not a valid <module>.<id> reference", field, ref))
		return nil
	}
	l.checkModuleDependency(m, qr.Module, a.Path, line,
		fmt.Sprintf("%s reference %q", field, ref))
	var otherKind string
	for _, art := range l.index[ref] {
		k := kindByCollection[art.CollectionDir]
		if k == wantKind {
			return art
		}
		otherKind = k
	}
	if otherKind != "" {
		l.emit("graph-policy-shape", a.Path, line,
			fmt.Sprintf("%s reference %q resolves to %s, not %s", field, ref, withArticle(otherKind), withArticle(wantKind)))
		return nil
	}
	l.emit("graph-policy-shape", a.Path, line,
		fmt.Sprintf("%s reference %q does not resolve to an existing %s", field, ref, wantKind))
	return nil
}

// checkClauseKeys validates a two-key clause operand map (is-role, actor-is):
// both keys are required non-empty scalars; other keys are rejected. It
// returns the two values ("" when missing or non-scalar).
func (l *linter) checkClauseKeys(a *Artifact, cm *clauseMap, label, k1, k2 string) (string, string) {
	l.checkClauseExtraKeys(a, cm, label, k1, k2)
	v1 := cm.Scalars[k1]
	v2 := cm.Scalars[k2]
	if v1 == "" {
		l.emit("graph-policy-shape", a.Path, cm.Line,
			fmt.Sprintf("%s must carry a non-empty `%s`", label, k1))
	}
	if v2 == "" {
		l.emit("graph-policy-shape", a.Path, cm.Line,
			fmt.Sprintf("%s must carry a non-empty `%s`", label, k2))
	}
	return v1, v2
}

// checkClauseExtraKeys rejects every key of cm outside the allowed set.
func (l *linter) checkClauseExtraKeys(a *Artifact, cm *clauseMap, label string, allowed ...string) {
	for _, k := range cm.Keys {
		if !slices.Contains(allowed, k) {
			l.emit("graph-policy-shape", a.Path, cm.Line,
				fmt.Sprintf("%s has unexpected key %q (allowed: %s)", label, k, strings.Join(allowed, ", ")))
		}
	}
}
