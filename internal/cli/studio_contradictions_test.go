package cli

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/internal/studio/fact"
)

// declaredFact builds a declared fact with the given triple and evidence
// pointer — a registries/specscore-adapter-shaped fact.
func declaredFact(subject, predicate, object, pointer string) fact.Fact {
	return fact.Fact{
		Subject:    subject,
		Predicate:  predicate,
		Object:     object,
		Evidence:   fact.Evidence{Class: fact.Declared, Pointer: pointer},
		Adapter:    fact.Adapter{ID: "registries", Version: "0.1.0"},
		ObservedAt: "2026-07-10T00:00:00Z",
		VerifiedAt: "2026-07-10T00:00:00Z",
		Ecosystem:  "demo",
	}
}

// verifiedFact builds a verified-behavior fact with the given triple and
// evidence pointer — a probe/rehearse-adapter-shaped fact.
func verifiedFact(subject, predicate, object, pointer string) fact.Fact {
	return fact.Fact{
		Subject:    subject,
		Predicate:  predicate,
		Object:     object,
		Evidence:   fact.Evidence{Class: fact.VerifiedBehavior, Pointer: pointer},
		Adapter:    fact.Adapter{ID: "probe-domain", Version: "0.1.0"},
		ObservedAt: "2026-07-10T01:00:00Z",
		VerifiedAt: "2026-07-10T01:00:00Z",
		Ecosystem:  "demo",
	}
}

// contradictionItemJSON mirrors the verb's JSON item shape for test decoding.
type contradictionItemJSON struct {
	Detector string `json:"detector"`
	A        struct {
		Subject    string `json:"subject"`
		Predicate  string `json:"predicate"`
		Object     string `json:"object"`
		Class      string `json:"evidence_class"`
		Pointer    string `json:"evidence_pointer"`
		Adapter    string `json:"adapter"`
		ObservedAt string `json:"observed_at"`
	} `json:"a"`
	B struct {
		Subject    string `json:"subject"`
		Predicate  string `json:"predicate"`
		Object     string `json:"object"`
		Class      string `json:"evidence_class"`
		Pointer    string `json:"evidence_pointer"`
		Adapter    string `json:"adapter"`
		ObservedAt string `json:"observed_at"`
	} `json:"b"`
}

func decodeContradictions(t *testing.T, out string) []contradictionItemJSON {
	t.Helper()
	var items []contradictionItemJSON
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatalf("output is not a JSON array of items: %v\n%s", err, out)
	}
	return items
}

// AC: status-drift-verified-fail — a Stable feature whose latest run failed is
// a status-drift contradiction; both sides carry their provenance.
func TestStudioContradictions_StatusDriftVerifiedFail(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("repo#feat/x", "has-status", "Stable", "spec/features/x/README.md"),
		verifiedFact("repo#feat/x#ac:y", "has-verification-status", "fail", ".specscore/rehearse/latest.json"),
	})

	out, _, err := runStudioCmd(t, "contradictions", "--workspace", wsPath, "--format", "json")
	if err != nil {
		t.Fatalf("studio contradictions: %v", err)
	}
	items := decodeContradictions(t, out)
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1:\n%s", len(items), out)
	}
	it := items[0]
	if it.Detector != "status-drift" {
		t.Errorf("detector = %q, want status-drift", it.Detector)
	}
	if it.A.Subject != "repo#feat/x" || it.A.Object != "Stable" || it.A.Class != "declared" {
		t.Errorf("side A = %+v, want the declared has-status Stable fact", it.A)
	}
	if it.B.Subject != "repo#feat/x#ac:y" || it.B.Object != "fail" || it.B.Class != "verified-behavior" {
		t.Errorf("side B = %+v, want the verified-behavior has-verification-status fail fact", it.B)
	}
	// Each side carries its provenance fields.
	for _, s := range []struct {
		name string
		ptr  string
		obs  string
	}{
		{"a", it.A.Pointer, it.A.ObservedAt},
		{"b", it.B.Pointer, it.B.ObservedAt},
	} {
		if s.ptr == "" || s.obs == "" {
			t.Errorf("side %s missing evidence_pointer/observed_at: %+v", s.name, it)
		}
	}
}

// AC: status-drift-dead-domain — a registry-declared live domain that probes
// dead is a status-drift contradiction.
func TestStudioContradictions_StatusDriftDeadDomain(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("dead.example", "serves-status", "200", "domains.json"),
		verifiedFact("dead.example", "serves-status", "down", "https://dead.example/"),
	})

	out, _, err := runStudioCmd(t, "contradictions", "--workspace", wsPath, "--format", "json")
	if err != nil {
		t.Fatalf("studio contradictions: %v", err)
	}
	items := decodeContradictions(t, out)
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1:\n%s", len(items), out)
	}
	it := items[0]
	if it.Detector != "status-drift" {
		t.Errorf("detector = %q, want status-drift", it.Detector)
	}
	if it.A.Object != "200" || it.A.Class != "declared" || it.A.Pointer != "domains.json" {
		t.Errorf("side A = %+v, want the declared 200 fact from domains.json", it.A)
	}
	if it.B.Object != "down" || it.B.Class != "verified-behavior" {
		t.Errorf("side B = %+v, want the verified-behavior down fact", it.B)
	}
	if it.B.ObservedAt == "" {
		t.Errorf("verified side missing observed_at: %+v", it.B)
	}
}

// AC: naming-conflict-declared-disagreement — two declared facts disagreeing on
// the same predicate are a naming-conflict; absent when they share the object.
func TestStudioContradictions_NamingConflictDeclaredDisagreement(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("subj", "provides", "ext-foo", "registry-a.json"),
		declaredFact("subj", "provides", "foo-contract", "registry-b.json"),
	})

	out, _, err := runStudioCmd(t, "contradictions", "--workspace", wsPath, "--format", "json")
	if err != nil {
		t.Fatalf("studio contradictions: %v", err)
	}
	items := decodeContradictions(t, out)
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1:\n%s", len(items), out)
	}
	it := items[0]
	if it.Detector != "naming-conflict" {
		t.Errorf("detector = %q, want naming-conflict", it.Detector)
	}
	objs := map[string]bool{it.A.Object: true, it.B.Object: true}
	if !objs["ext-foo"] || !objs["foo-contract"] {
		t.Errorf("sides do not carry both objects: %+v", it)
	}
}

// The naming-conflict item is absent when the two declared facts share the same
// object (agreement across sources).
func TestStudioContradictions_SameObjectNoNamingConflict(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("subj", "provides", "foo", "registry-a.json"),
		declaredFact("subj", "provides", "foo", "registry-b.json"),
	})

	out, _, err := runStudioCmd(t, "contradictions", "--workspace", wsPath, "--format", "json")
	if err != nil {
		t.Fatalf("studio contradictions: %v", err)
	}
	if items := decodeContradictions(t, out); len(items) != 0 {
		t.Errorf("got %d items, want 0 (agreement not flagged):\n%s", len(items), out)
	}
}

// A clean store exits 0 and prints an empty JSON array.
func TestStudioContradictions_CleanStoreEmptyJSON(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("subj", "provides", "foo", "registry-a.json"),
	})

	out, _, err := runStudioCmd(t, "contradictions", "--workspace", wsPath, "--format", "json")
	if err != nil {
		t.Fatalf("studio contradictions: %v", err)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Errorf("clean-store JSON = %q, want []", strings.TrimSpace(out))
	}
}

// The human format lists each item's detector and both evidence sets.
func TestStudioContradictions_HumanOutput(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("dead.example", "serves-status", "200", "domains.json"),
		verifiedFact("dead.example", "serves-status", "down", "https://dead.example/"),
	})

	out, _, err := runStudioCmd(t, "contradictions", "--workspace", wsPath)
	if err != nil {
		t.Fatalf("studio contradictions: %v", err)
	}
	for _, want := range []string{
		"Contradictions: 1",
		"[status-drift]",
		"dead.example",
		"200",
		"down",
		"domains.json",
		"class=declared",
		"class=verified-behavior",
		"observed_at=",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("human output missing %q; got:\n%s", want, out)
		}
	}
}

// A clean store in human format prints a "No contradictions" line and exits 0.
func TestStudioContradictions_HumanCleanStore(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("subj", "provides", "foo", "registry-a.json"),
	})

	out, _, err := runStudioCmd(t, "contradictions", "--workspace", wsPath)
	if err != nil {
		t.Fatalf("studio contradictions: %v", err)
	}
	if !strings.Contains(out, "No contradictions found.") {
		t.Errorf("clean-store human output missing the empty notice; got:\n%s", out)
	}
}

// AC: contradictions-without-index-errors (guard exists) — running before
// studio index exits 2 with the store path and the studio-index suggestion.
func TestStudioContradictions_MissingStoreExits2(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a") // no store built
	wantDB := filepath.Join(filepath.Dir(wsPath), ".specscore-studio", "facts.db")

	_, _, err := runStudioCmd(t, "contradictions", "--workspace", wsPath)
	if code := studioExit(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(err.Error(), wantDB) || !strings.Contains(err.Error(), "studio index") {
		t.Errorf("error %q must name the store path and suggest `studio index`", err.Error())
	}
}

// A bad --format exits 2 and names the offending value.
func TestStudioContradictions_BadFormatExits2(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{declaredFact("subj", "provides", "foo", "registry-a.json")})

	_, _, err := runStudioCmd(t, "contradictions", "--workspace", wsPath, "--format", "yaml")
	if code := studioExit(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(err.Error(), "yaml") {
		t.Errorf("error %q does not name the bad format", err.Error())
	}
}

// A bad --db path (abs resolution failure) exits 2 before touching the store.
func TestStudioContradictions_DBFlagAbsError(t *testing.T) {
	old := filepathAbsFn
	filepathAbsFn = func(string) (string, error) { return "", errors.New("abs boom") }
	t.Cleanup(func() { filepathAbsFn = old })

	_, _, err := runStudioCmd(t, "contradictions", "--db", "rel.db")
	if code := studioExit(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

// --db bypasses the workspace: contradictions read the store directly.
func TestStudioContradictions_DBFlagBypassesWorkspace(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	dbPath := seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("dead.example", "serves-status", "200", "domains.json"),
		verifiedFact("dead.example", "serves-status", "down", "https://dead.example/"),
	})

	out, _, err := runStudioCmd(t, "contradictions", "--db", dbPath, "--format", "json")
	if err != nil {
		t.Fatalf("studio contradictions --db: %v", err)
	}
	if items := decodeContradictions(t, out); len(items) != 1 {
		t.Fatalf("got %d items, want 1:\n%s", len(items), out)
	}
}
