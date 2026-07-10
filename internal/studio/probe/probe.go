// Package probe runs live checks against the ecosystem and produces
// `verified-behavior` facts that merge into the same fact store `studio index`
// builds. Unlike index, probe performs live network I/O: it reads its targets
// from the facts already in the store and turns declared expectations into
// behavioral truth by actually requesting the resource.
//
// Feature: cli/studio/probe (REQ: probe-verb, REQ: domain-liveness,
// REQ: network-failure-is-data)
//
// This phase's Task 2 lands the domain-liveness kind (adapter id `probe-domain`):
// it requests https-first with an http fallback on transport failure, records
// the URL that answered as the evidence pointer, and maps a both-schemes
// transport failure to the reserved non-numeric object `down`. Probers reach
// the network only through the package-level HTTPGetFn seam, so tests never hit
// the network.
package probe

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/specscore/specscore-cli/internal/studio/fact"
)

// HTTPGetFn is the test seam for the domain kind's HTTP requests: production
// code issues every domain GET through this var so tests replace it via
// t.Cleanup and never touch the network. It mirrors http.Get's signature: a
// non-nil error is a transport-level failure (no HTTP response), which is what
// the https→http fallback and the `down` mapping key on (REQ: domain-liveness,
// REQ: network-failure-is-data).
var HTTPGetFn = func(url string) (*http.Response, error) {
	client := &http.Client{Timeout: httpTimeout}
	return client.Get(url)
}

// httpTimeout bounds each domain request so an unresponsive host maps to a
// transport failure (→ `down`) rather than hanging the run. The Feature leaves
// the exact value a plan detail (Open Questions); a few seconds is the sensible
// bounded default.
const httpTimeout = 10 * time.Second

const (
	// DomainAdapterID is the adapter id stamped on every domain-kind fact. It is
	// part of the store merge key, so it must differ from the ci kind's id and
	// from every index adapter's id (REQ: probe-verb, REQ: probe-merge).
	DomainAdapterID = "probe-domain"

	// ServesStatusPredicate is the predicate the registries adapter declares and
	// the domain probe verifies — the join point between the declared and the
	// verified-behavior fact for one domain (REQ: domain-liveness).
	ServesStatusPredicate = "serves-status"

	// DownObject is the reserved non-numeric object marking a domain that
	// answered over neither scheme — distinct from any numeric HTTP status
	// (including 5xx, which is a response) (REQ: network-failure-is-data).
	DownObject = "down"

	// KindDomain names the domain-liveness probe kind in the run summary.
	KindDomain = "domain"
)

// Result is the outcome of a probe run: the facts produced (to be merged into
// the store) plus the human-facing warnings collected along the way. The verb
// merges Facts and renders Kinds/Warnings into its run summary.
type Result struct {
	// Kinds are the probe kinds that ran, in run order (e.g. ["domain"]).
	Kinds []string
	// Facts are the verified-behavior facts produced this run, ready to merge.
	Facts []fact.Fact
	// Warnings are per-target or per-kind notices for the run summary.
	Warnings []string
}

// RunDomain runs the domain-liveness kind over the domain targets carried by
// existing: every distinct subject of a `serves-status` fact is a domain to
// check (the domains the registries adapter declared). For each it issues an
// https-first / http-fallback GET through HTTPGetFn and emits one
// `(<domain>, serves-status, <status|down>)` verified-behavior fact under
// adapter id probe-domain (version = cliVersion), with the evidence pointer set
// to the URL that answered (or the https URL attempted when neither scheme
// answered) and observed_at/verified_at both stamped at now (REQ: domain-liveness,
// REQ: network-failure-is-data). The domain's ecosystem is carried over from
// its declared fact so the verified fact joins the same ecosystem.
func RunDomain(existing []fact.Fact, cliVersion string, now time.Time) Result {
	stamp := now.UTC().Format(time.RFC3339)
	res := Result{Kinds: []string{KindDomain}}
	for _, t := range domainTargets(existing) {
		res.Facts = append(res.Facts, probeDomain(t, cliVersion, stamp))
	}
	return res
}

// domainTarget is one domain to probe: its id (the fact subject) and the
// ecosystem to carry onto the verified fact.
type domainTarget struct {
	domain    string
	ecosystem string
}

// domainTargets extracts the distinct domain subjects carrying a serves-status
// fact from the store's facts, in deterministic (sorted) order. The first
// serves-status fact seen for a domain fixes its ecosystem; a domain that
// already has a verified-behavior serves-status fact is still a target (a
// re-probe refreshes it — the merge handles refresh-vs-new-observation).
func domainTargets(facts []fact.Fact) []domainTarget {
	seen := map[string]domainTarget{}
	for _, f := range facts {
		if f.Predicate != ServesStatusPredicate {
			continue
		}
		if _, ok := seen[f.Subject]; ok {
			continue
		}
		seen[f.Subject] = domainTarget{domain: f.Subject, ecosystem: f.Ecosystem}
	}
	domains := make([]string, 0, len(seen))
	for d := range seen {
		domains = append(domains, d)
	}
	sort.Strings(domains)
	out := make([]domainTarget, 0, len(domains))
	for _, d := range domains {
		out = append(out, seen[d])
	}
	return out
}

// probeDomain checks one domain https-first, falling back to http on a
// transport failure, and builds its verified-behavior fact. When neither
// scheme yields an HTTP response the object is `down` and the pointer records
// the https URL attempted with the failure reason as its detail
// (REQ: network-failure-is-data).
func probeDomain(t domainTarget, cliVersion, stamp string) fact.Fact {
	httpsURL := "https://" + t.domain + "/"
	resp, err := HTTPGetFn(httpsURL)
	if err == nil {
		return domainFact(t, statusObject(resp), httpsURL, cliVersion, stamp)
	}
	httpURL := "http://" + t.domain + "/"
	if resp, err := HTTPGetFn(httpURL); err == nil {
		return domainFact(t, statusObject(resp), httpURL, cliVersion, stamp)
	}
	// Neither scheme answered: the https attempt's error is the honest reason.
	pointer := fmt.Sprintf("%s: %s", httpsURL, oneLine(err.Error()))
	return domainFact(t, DownObject, pointer, cliVersion, stamp)
}

// oneLine collapses a (possibly multi-line) transport error into a single line
// so the evidence pointer stays a one-liner.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// statusObject renders a response's status code as the fact object and closes
// the body. A 5xx is a response, so it is a numeric object, not `down`.
func statusObject(resp *http.Response) string {
	if resp.Body != nil {
		_ = resp.Body.Close()
	}
	return strconv.Itoa(resp.StatusCode)
}

// domainFact assembles one verified-behavior serves-status fact for a domain.
func domainFact(t domainTarget, object, pointer, cliVersion, stamp string) fact.Fact {
	return fact.Fact{
		Subject:   t.domain,
		Predicate: ServesStatusPredicate,
		Object:    object,
		Evidence:  fact.Evidence{Class: fact.VerifiedBehavior, Pointer: pointer},
		Adapter:   fact.Adapter{ID: DomainAdapterID, Version: cliVersion},
		// A fresh observation stamps observed_at and verified_at equal; the
		// store's Merge refreshes verified_at (preserving observed_at) when a
		// re-probe re-verifies an unchanged fact (REQ: verified-at-field).
		ObservedAt: stamp,
		VerifiedAt: stamp,
		Ecosystem:  t.ecosystem,
	}
}
