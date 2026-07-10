// Package ask is SpecScore Studio's deterministic question router — the engine
// behind `specscore studio ask "<question>"`. It is NOT an LLM: it lowercases
// the question, matches it against a fixed library of 13 parameterized question
// *templates* (each a keyword trigger plus a captured parameter), resolves the
// captured parameter through resolve.Resolve where the template targets an
// entity, runs the template's pure store query over a `[]fact.Fact` slice, and
// returns an answer that always carries citations (the facts it stands on).
//
// Feature: cli/studio/answers (REQ: ask-verb-and-router, REQ: ask-citations,
// REQ: ask-unroutable)
//
// The router is deterministic: templates are tried in a fixed order and each
// trigger is a plain prefix/suffix keyword match on the lowercased question, so
// the same question always routes the same way. A trigger captures the
// parameter as the text between an opening keyword and (optionally) a closing
// keyword — the "is <X> green" / "is <X> live" shapes disambiguate on their
// closing keyword, never on order-dependent guessing. A question matching no
// template is unroutable (the caller exits 1 and lists the templates); a
// question that routes but whose parameter resolves to no facts is
// routed-but-empty (the caller exits 3) — never a citation-free assertion.
//
// The package is pure of CLI wiring: Route and each template's Query take the
// full fact slice and return values, so both the `ask` verb and the benchmark
// runner drive it the same way.
package ask

import (
	"sort"
	"strconv"
	"strings"

	"github.com/specscore/specscore-cli/internal/studio/fact"
	"github.com/specscore/specscore-cli/internal/studio/resolve"
)

// Citation is one supporting fact named on an answer (REQ: ask-citations): the
// full triple plus the evidence class, pointer, and adapter id so a caller can
// render the "why" of every answer without a second store query. The field set
// is exactly the shape the Feature's ask-citations REQ pins.
type Citation struct {
	Subject   string `json:"subject"`
	Predicate string `json:"predicate"`
	Object    string `json:"object"`
	Class     string `json:"evidence_class"`
	Pointer   string `json:"evidence_pointer"`
	Adapter   string `json:"adapter"`
}

// citeFact projects a fact onto the Citation shape.
func citeFact(f fact.Fact) Citation {
	return Citation{
		Subject:   f.Subject,
		Predicate: f.Predicate,
		Object:    f.Object,
		Class:     string(f.Class),
		Pointer:   f.Pointer,
		Adapter:   f.Adapter.ID,
	}
}

// Answer is a routed, non-empty template result (REQ: ask-verb-and-router): the
// human-readable answer prose plus the citations it stands on. An Answer is only
// ever returned with a non-empty Citations slice — a zero-fact result is not an
// answer (REQ: ask-citations) and surfaces as the routed-but-empty path.
type Answer struct {
	// Answer is the human-readable prose answer.
	Answer string
	// Citations are the supporting facts, never empty for a real Answer.
	Citations []Citation
}

// trigger captures a template's parameter as the text between an opening keyword
// prefix and an optional closing keyword suffix. Prefix is required; when Suffix
// is non-empty the captured text must end with it (which is then stripped), so
// "is <X> live" and "is <X> green" share the "is " prefix but disambiguate on
// their distinct suffixes.
type trigger struct {
	Prefix string
	Suffix string
}

// capture matches the trigger against the lowercased question and returns the
// trimmed parameter between the prefix and suffix, or ok=false when the trigger
// does not apply. A trailing "?" is tolerated before the suffix match.
func (t trigger) capture(question string) (string, bool) {
	idx := strings.Index(question, t.Prefix)
	if idx < 0 {
		return "", false
	}
	tail := strings.TrimSpace(question[idx+len(t.Prefix):])
	tail = strings.TrimSpace(strings.TrimSuffix(tail, "?"))
	if t.Suffix != "" {
		if !strings.HasSuffix(tail, t.Suffix) {
			return "", false
		}
		tail = strings.TrimSpace(strings.TrimSuffix(tail, t.Suffix))
	}
	if tail == "" {
		return "", false
	}
	return tail, true
}

// Template is one entry in the router's fixed library: a stable id, the trigger
// forms shown by `--list` and the unroutable notice, its parameter-capture
// triggers, and a pure Query over the fact slice. TargetsEntity marks templates
// whose captured parameter is an entity name to canonicalise through
// resolve.Resolve before Query runs; when false (only aliases-resolve) the raw
// parameter is passed to Query, which does its own resolution.
type Template struct {
	// ID is the stable template id (e.g. "who-fronts").
	ID string
	// Forms are the human-readable trigger forms surfaced by --list and the
	// unroutable notice (e.g. `who fronts <X>`).
	Forms []string
	// triggers are the parameter-capture patterns this template matches.
	triggers []trigger
	// TargetsEntity is true when the captured parameter is an entity name the
	// router canonicalises through resolve before calling Query.
	TargetsEntity bool
	// Query runs the template's store query over facts for the (already
	// resolved, when TargetsEntity) parameter and returns an Answer plus ok. ok
	// is false when no supporting fact exists (routed-but-empty).
	Query func(facts []fact.Fact, param string) (Answer, bool)
}

// Templates returns the router's 13 templates in trigger-match order. Every
// trigger is disambiguated by its prefix+suffix keyword pair, so order only
// breaks ties between genuinely overlapping shapes; the "ci status of <X>"
// trigger precedes the bare "status of <X>" so a CI question never matches the
// lifecycle-status template.
func Templates() []Template {
	return []Template{
		whoFronts(),
		whatReposImplement(),
		ciStatusOf(),
		statusOf(),
		aliasesOf(),
		memberOf(),
		isItLive(),
		whatVerifies(),
		contradictionsFor(),
		freshnessOf(),
		whatUses(),
		versionPins(),
		aliasesResolve(),
	}
}

// Route matches question against the template library (REQ: ask-verb-and-router):
// it lowercases the question, tries each template's triggers in order, and on
// the first hit returns the template, the captured parameter (trimmed), and
// true. It returns ok=false when no template matches (the unroutable path). The
// captured parameter is the entity between the trigger's prefix and suffix — for
// entity-targeting templates the caller resolves it before running Query.
func Route(question string) (Template, string, bool) {
	lower := strings.ToLower(strings.TrimSpace(question))
	for _, tmpl := range Templates() {
		for _, trig := range tmpl.triggers {
			if param, ok := trig.capture(lower); ok {
				return tmpl, param, true
			}
		}
	}
	return Template{}, "", false
}

// Resolve canonicalises an entity parameter through resolve.Resolve for a
// template whose TargetsEntity is true (REQ: ask-verb-and-router routes the
// parameter through resolve semantics). It returns the resolve.Result so the
// caller can render an ambiguous parameter per the house AmbiguousSlug rule.
// The resolved id (Result.ID for Unique) is what Query receives as its param.
func Resolve(facts []fact.Fact, param string) resolve.Result {
	return resolve.Resolve(facts, param)
}

// --- template query helpers ---

// answerFromFacts builds an Answer whose prose is the given text and whose
// citations are the supporting facts; ok is false (routed-but-empty) when
// supporting is empty, so no citation-free answer is ever returned.
func answerFromFacts(text string, supporting []fact.Fact) (Answer, bool) {
	if len(supporting) == 0 {
		return Answer{}, false
	}
	cites := make([]Citation, 0, len(supporting))
	for _, f := range supporting {
		cites = append(cites, citeFact(f))
	}
	return Answer{Answer: text, Citations: cites}, true
}

// eq reports case-insensitive string equality — the match used to compare a
// resolved entity id against fact subjects/objects.
func eq(a, b string) bool { return strings.EqualFold(a, b) }

// hasPrefixFold reports whether s begins with prefix, case-insensitively — the
// object-prefix / subject-prefix match some templates use.
func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

// --- the 13 templates ---

// whoFronts: "who fronts X" — the fronts fact(s) mentioning resolve(X) on
// either side; the answer names the counterparty of the fronts edge. A `fronts`
// fact is `(<domain>, fronts, <worker|product>)`, so asking "who fronts acme.app"
// (X = the domain, the subject) answers with the fronted worker (the object),
// while "who fronts acme" (X = the worker, the object) answers with the fronting
// domain (the subject) — matching either side keeps both phrasings answerable
// from the one directional fact.
func whoFronts() Template {
	return Template{
		ID:            "who-fronts",
		Forms:         []string{"who fronts <X>", "what fronts <X>"},
		triggers:      []trigger{{Prefix: "who fronts "}, {Prefix: "what fronts "}},
		TargetsEntity: true,
		Query: func(facts []fact.Fact, param string) (Answer, bool) {
			var supporting []fact.Fact
			var counterparties []string
			for _, f := range facts {
				switch {
				case f.Predicate != "fronts":
					continue
				case eq(f.Subject, param):
					supporting = append(supporting, f)
					counterparties = append(counterparties, f.Object)
				case eq(f.Object, param):
					supporting = append(supporting, f)
					counterparties = append(counterparties, f.Subject)
				}
			}
			return answerFromFacts(param+" fronts edge → "+joinList(counterparties), supporting)
		},
	}
}

// whatReposImplement: "what repos implement X" — implemented-by facts whose
// subject is resolve(X); the answer names the implementing repos (objects).
func whatReposImplement() Template {
	return Template{
		ID:            "what-repos-implement",
		Forms:         []string{"what repos implement <X>", "which repo implements <X>"},
		triggers:      []trigger{{Prefix: "what repos implement "}, {Prefix: "which repo implements "}},
		TargetsEntity: true,
		Query: func(facts []fact.Fact, param string) (Answer, bool) {
			var supporting []fact.Fact
			var repos []string
			for _, f := range facts {
				if f.Predicate == "implemented-by" && eq(f.Subject, param) {
					supporting = append(supporting, f)
					repos = append(repos, f.Object)
				}
			}
			return answerFromFacts(param+" is implemented by "+joinList(repos), supporting)
		},
	}
}

// ciStatusOf: "ci status of X" / "is X green" — the ci-status verified-behavior
// fact whose subject is resolve(X).
func ciStatusOf() Template {
	return Template{
		ID:            "ci-status-of",
		Forms:         []string{"ci status of <X>", "is <X> green"},
		triggers:      []trigger{{Prefix: "ci status of "}, {Prefix: "is ", Suffix: " green"}},
		TargetsEntity: true,
		Query: func(facts []fact.Fact, param string) (Answer, bool) {
			var supporting []fact.Fact
			var status string
			for _, f := range facts {
				if f.Predicate == "ci-status" && eq(f.Subject, param) {
					supporting = append(supporting, f)
					status = f.Object
				}
			}
			return answerFromFacts(param+" ci-status is "+status, supporting)
		},
	}
}

// statusOf: "status of X" / "is X approved/stable" — the has-status fact whose
// subject is resolve(X).
func statusOf() Template {
	return Template{
		ID:    "status-of",
		Forms: []string{"status of <X>", "is <X> approved", "is <X> stable"},
		triggers: []trigger{
			{Prefix: "status of "},
			{Prefix: "is ", Suffix: " approved"},
			{Prefix: "is ", Suffix: " stable"},
		},
		TargetsEntity: true,
		Query: func(facts []fact.Fact, param string) (Answer, bool) {
			var supporting []fact.Fact
			var status string
			for _, f := range facts {
				if f.Predicate == "has-status" && eq(f.Subject, param) {
					supporting = append(supporting, f)
					status = f.Object
				}
			}
			return answerFromFacts(param+" has status "+status, supporting)
		},
	}
}

// aliasesOf: "aliases of X" / "what is X called" — aliased-as facts whose
// subject is resolve(X); the answer names the brand aliases (objects).
func aliasesOf() Template {
	return Template{
		ID:            "aliases-of",
		Forms:         []string{"aliases of <X>", "what is <X> called"},
		triggers:      []trigger{{Prefix: "aliases of "}, {Prefix: "what is ", Suffix: " called"}},
		TargetsEntity: true,
		Query: func(facts []fact.Fact, param string) (Answer, bool) {
			var supporting []fact.Fact
			var aliases []string
			for _, f := range facts {
				if f.Predicate == "aliased-as" && eq(f.Subject, param) {
					supporting = append(supporting, f)
					aliases = append(aliases, f.Object)
				}
			}
			return answerFromFacts(param+" is also called "+joinList(aliases), supporting)
		},
	}
}

// memberOf: "what vertical is X in" / "member of X" — the member-of fact whose
// subject is resolve(X); the answer names the vertical (object).
func memberOf() Template {
	return Template{
		ID:            "member-of",
		Forms:         []string{"what vertical is <X> in", "member of <X>"},
		triggers:      []trigger{{Prefix: "what vertical is ", Suffix: " in"}, {Prefix: "member of "}},
		TargetsEntity: true,
		Query: func(facts []fact.Fact, param string) (Answer, bool) {
			var supporting []fact.Fact
			var vertical string
			for _, f := range facts {
				if f.Predicate == "member-of" && eq(f.Subject, param) {
					supporting = append(supporting, f)
					vertical = f.Object
				}
			}
			return answerFromFacts(param+" is a member of "+vertical, supporting)
		},
	}
}

// isItLive: "is X live" / "does X serve" — the fronts→serves-status two-step
// hop: resolve(X) → the domain(s) that front it → their verified-behavior
// serves-status. The answer cites both the fronts and the probed serves-status
// fact so the liveness verdict is traceable end to end. The entity itself is
// also treated as a candidate domain, so a direct serves-status on the named
// domain answers without a fronts hop.
func isItLive() Template {
	return Template{
		ID:            "is-it-live",
		Forms:         []string{"is <X> live", "does <X> serve"},
		triggers:      []trigger{{Prefix: "is ", Suffix: " live"}, {Prefix: "does ", Suffix: " serve"}},
		TargetsEntity: true,
		Query: func(facts []fact.Fact, param string) (Answer, bool) {
			// Step 1: the domains that front the entity (fronts object = entity
			// → subject is the domain).
			var frontsFacts []fact.Fact
			domains := map[string]bool{strings.ToLower(param): true}
			for _, f := range facts {
				if f.Predicate == "fronts" && eq(f.Object, param) {
					frontsFacts = append(frontsFacts, f)
					domains[strings.ToLower(f.Subject)] = true
				}
			}
			// Step 2: the verified-behavior serves-status fact(s) for those
			// domains — the probe's live observation.
			supporting := append([]fact.Fact(nil), frontsFacts...)
			var verdicts []string
			for _, f := range facts {
				if f.Predicate == "serves-status" && f.Class == fact.VerifiedBehavior && domains[strings.ToLower(f.Subject)] {
					supporting = append(supporting, f)
					verdicts = append(verdicts, f.Subject+"="+f.Object)
				}
			}
			if len(verdicts) == 0 {
				// No probed liveness fact → routed-but-empty (a fronts fact alone
				// is not a liveness answer).
				return Answer{}, false
			}
			return answerFromFacts(param+" liveness: "+joinList(verdicts), supporting)
		},
	}
}

// whatVerifies: "what verifies X" / "is X tested" — the rehearse facts
// (verified-by / has-verification-status) whose subject prefix is resolve(X).
func whatVerifies() Template {
	return Template{
		ID:            "what-verifies",
		Forms:         []string{"what verifies <X>", "is <X> tested"},
		triggers:      []trigger{{Prefix: "what verifies "}, {Prefix: "is ", Suffix: " tested"}},
		TargetsEntity: true,
		Query: func(facts []fact.Fact, param string) (Answer, bool) {
			var supporting []fact.Fact
			var scenarios []string
			for _, f := range facts {
				if (f.Predicate == "verified-by" || f.Predicate == "has-verification-status") &&
					hasPrefixFold(f.Subject, param) {
					supporting = append(supporting, f)
					if f.Predicate == "verified-by" {
						scenarios = append(scenarios, f.Object)
					}
				}
			}
			return answerFromFacts(param+" is verified by "+joinList(scenarios), supporting)
		},
	}
}

// contradictionsFor: "contradictions for X" / "does X conflict" — the
// contradicts fact(s) either of whose sides references resolve(X).
func contradictionsFor() Template {
	return Template{
		ID:            "contradictions-for",
		Forms:         []string{"contradictions for <X>", "does <X> conflict"},
		triggers:      []trigger{{Prefix: "contradictions for "}, {Prefix: "does ", Suffix: " conflict"}},
		TargetsEntity: true,
		Query: func(facts []fact.Fact, param string) (Answer, bool) {
			var supporting []fact.Fact
			for _, f := range facts {
				if f.Predicate == "contradicts" &&
					(refMentions(f.Subject, param) || refMentions(f.Object, param)) {
					supporting = append(supporting, f)
				}
			}
			return answerFromFacts(
				param+" is involved in "+strconv.Itoa(len(supporting))+" contradiction(s)", supporting)
		},
	}
}

// freshnessOf: "how fresh is X" / "when was X verified" — any fact about
// resolve(X), reporting the freshest (max verified_at) one and citing it.
func freshnessOf() Template {
	return Template{
		ID:            "freshness-of",
		Forms:         []string{"how fresh is <X>", "when was <X> verified"},
		triggers:      []trigger{{Prefix: "how fresh is "}, {Prefix: "when was ", Suffix: " verified"}},
		TargetsEntity: true,
		Query: func(facts []fact.Fact, param string) (Answer, bool) {
			var freshest *fact.Fact
			for i := range facts {
				f := facts[i]
				if !eq(f.Subject, param) {
					continue
				}
				if freshest == nil || f.VerifiedAt > freshest.VerifiedAt {
					fc := f
					freshest = &fc
				}
			}
			if freshest == nil {
				return Answer{}, false
			}
			return answerFromFacts(param+" was last verified at "+freshest.VerifiedAt, []fact.Fact{*freshest})
		},
	}
}

// whatUses: "what uses X" / "who consumes X" — consumes facts whose object
// prefix is resolve(X); the answer names the consuming subjects.
func whatUses() Template {
	return Template{
		ID:            "what-uses",
		Forms:         []string{"what uses <X>", "who consumes <X>"},
		triggers:      []trigger{{Prefix: "what uses "}, {Prefix: "who consumes "}},
		TargetsEntity: true,
		Query: func(facts []fact.Fact, param string) (Answer, bool) {
			var supporting []fact.Fact
			var consumers []string
			for _, f := range facts {
				if f.Predicate == "consumes" && hasPrefixFold(f.Object, param) {
					supporting = append(supporting, f)
					consumers = append(consumers, f.Subject)
				}
			}
			return answerFromFacts(param+" is used by "+joinList(consumers), supporting)
		},
	}
}

// versionPins: "what version of X" / "which version pins X" — consumes facts
// whose object prefix is resolve(X); the answer names the pinned versions (the
// full package-id object, which carries the @version suffix).
func versionPins() Template {
	return Template{
		ID:            "version-pins",
		Forms:         []string{"what version of <X>", "which version pins <X>"},
		triggers:      []trigger{{Prefix: "what version of "}, {Prefix: "which version pins "}},
		TargetsEntity: true,
		Query: func(facts []fact.Fact, param string) (Answer, bool) {
			var supporting []fact.Fact
			var pins []string
			for _, f := range facts {
				if f.Predicate == "consumes" && hasPrefixFold(f.Object, param) {
					supporting = append(supporting, f)
					pins = append(pins, f.Subject+" pins "+f.Object)
				}
			}
			return answerFromFacts(param+" version pins: "+joinList(pins), supporting)
		},
	}
}

// aliasesResolve: "what is X" / "resolve X" — the bare id lookup. Unlike the
// entity-targeting templates this one does its own resolution (TargetsEntity is
// false): it resolves the raw parameter and cites the resolving fact.
func aliasesResolve() Template {
	return Template{
		ID:            "aliases-resolve",
		Forms:         []string{"what is <X>", "resolve <X>"},
		triggers:      []trigger{{Prefix: "what is "}, {Prefix: "resolve "}},
		TargetsEntity: false,
		Query: func(facts []fact.Fact, param string) (Answer, bool) {
			r := resolve.Resolve(facts, param)
			if r.Kind != resolve.Unique {
				// Ambiguous or NotFound → no single answer here; routed-but-empty
				// (the dedicated resolve verb handles ambiguity disambiguation).
				return Answer{}, false
			}
			return answerFromFacts(param+" resolves to "+r.ID, []fact.Fact{r.Citation})
		},
	}
}

// --- small rendering helpers ---

// joinList renders a slice of strings in a stable, sorted, comma-separated form
// so the answer prose is deterministic regardless of fact order.
func joinList(xs []string) string {
	sorted := append([]string(nil), xs...)
	sort.Strings(sorted)
	return strings.Join(sorted, ", ")
}

// refMentions reports whether a contradicts fact-ref (a "<subject>|<predicate>|<object>"
// triple key) mentions the entity as its subject or object component,
// case-insensitively.
func refMentions(ref, entity string) bool {
	parts := strings.SplitN(ref, "|", 3)
	if eq(parts[0], entity) {
		return true
	}
	return len(parts) == 3 && eq(parts[2], entity)
}
