package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/internal/studio/fact"
	"github.com/specscore/specscore-cli/internal/studio/store"
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
// the same single-valued predicate are a naming-conflict; absent when they
// share the object. `fronts` is in the single-valued set the detector fires on
// (REQ: same-predicate-disagreement).
func TestStudioContradictions_NamingConflictDeclaredDisagreement(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("d.example", "fronts", "ext-foo", "registry-a.yaml"),
		declaredFact("d.example", "fronts", "foo-contract", "registry-b.yaml"),
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

// --- Task 2 ACs ---

// AC: agreement-not-flagged — two declared facts with the same subject,
// predicate, and object from different evidence pointers are not a
// contradiction; the command exits 0 and returns an empty JSON array.
func TestStudioContradictions_AgreementNotFlagged(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("subj", "provides", "foo", "registry-a.json"),
		declaredFact("subj", "provides", "foo", "registry-b.json"),
	})

	out, _, err := runStudioCmd(t, "contradictions", "--workspace", wsPath, "--format", "json")
	if err != nil {
		t.Fatalf("studio contradictions: %v", err)
	}
	items := decodeContradictions(t, out)
	if len(items) != 0 {
		t.Errorf("agreement-not-flagged: got %d items, want 0:\n%s", len(items), out)
	}
}

// AC: behavioral-supersession-not-flagged — two verified-behavior facts with
// the same subject and predicate but different objects (a changed probe
// result) are supersession, not a contradiction; neither detector fires.
func TestStudioContradictions_BehavioralSupersessionNotFlagged(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		verifiedFact("d.example", "serves-status", "200", "https://d.example/"),
		verifiedFact("d.example", "serves-status", "down", "https://d.example/"),
	})

	out, _, err := runStudioCmd(t, "contradictions", "--workspace", wsPath, "--format", "json")
	if err != nil {
		t.Fatalf("studio contradictions: %v", err)
	}
	items := decodeContradictions(t, out)
	if len(items) != 0 {
		t.Errorf("behavioral-supersession-not-flagged: got %d items, want 0:\n%s", len(items), out)
	}
}

// AC: contradicts-fact-written — running studio contradictions writes a
// contradicts fact into the store (queryable via studio facts --predicate
// contradicts), and re-running does not create a duplicate.
func TestStudioContradictions_ContradictsFactWritten(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("dead.example", "serves-status", "200", "domains.json"),
		verifiedFact("dead.example", "serves-status", "down", "https://dead.example/"),
	})

	// First run: should detect the contradiction and write a contradicts fact.
	if _, _, err := runStudioCmd(t, "contradictions", "--workspace", wsPath); err != nil {
		t.Fatalf("first studio contradictions: %v", err)
	}

	// Query the store for contradicts facts.
	out, _, err := runStudioCmd(t, "facts", "--workspace", wsPath,
		"--predicate", "contradicts", "--format", "json")
	if err != nil {
		t.Fatalf("studio facts: %v", err)
	}
	var facts []fact.Fact
	if err := json.Unmarshal([]byte(out), &facts); err != nil {
		t.Fatalf("facts output is not JSON: %v\n%s", err, out)
	}
	if len(facts) != 1 {
		t.Fatalf("got %d contradicts facts after first run, want 1", len(facts))
	}
	f := facts[0]

	// Verify exact fact shape per REQ: contradiction-facts.
	if f.Class != fact.Derived {
		t.Errorf("Class = %q, want derived", f.Class)
	}
	if f.Adapter.ID != "contradictions" {
		t.Errorf("Adapter.ID = %q, want contradictions", f.Adapter.ID)
	}
	if f.Pointer != "status-drift" {
		t.Errorf("Pointer = %q, want status-drift", f.Pointer)
	}
	if f.Predicate != "contradicts" {
		t.Errorf("Predicate = %q, want contradicts", f.Predicate)
	}
	// Subject and object must be non-empty pointer-free triple refs.
	if f.Subject == "" || f.Object == "" {
		t.Errorf("Subject/Object is empty: Subject=%q Object=%q", f.Subject, f.Object)
	}
	// Both refs must contain "|" (the triple-key separator).
	if !strings.Contains(f.Subject, "|") || !strings.Contains(f.Object, "|") {
		t.Errorf("Subject/Object not a fact-ref triple: Subject=%q Object=%q", f.Subject, f.Object)
	}

	// Second run: re-running must not create a duplicate (idempotent merge).
	if _, _, err := runStudioCmd(t, "contradictions", "--workspace", wsPath); err != nil {
		t.Fatalf("second studio contradictions: %v", err)
	}
	out2, _, err := runStudioCmd(t, "facts", "--workspace", wsPath,
		"--predicate", "contradicts", "--count")
	if err != nil {
		t.Fatalf("studio facts count: %v", err)
	}
	if strings.TrimSpace(out2) != "1" {
		t.Errorf("count after second run = %q, want 1 (no duplicates)", strings.TrimSpace(out2))
	}
}

// AC: contradicts-fact-written (--no-write) — with --no-write, the verb
// detects and prints items but does not write any contradicts fact.
func TestStudioContradictions_NoWriteSkipsWriteBack(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("dead.example", "serves-status", "200", "domains.json"),
		verifiedFact("dead.example", "serves-status", "down", "https://dead.example/"),
	})

	// Run with --no-write: items should still be reported.
	out, _, err := runStudioCmd(t, "contradictions", "--workspace", wsPath,
		"--no-write", "--format", "json")
	if err != nil {
		t.Fatalf("studio contradictions --no-write: %v", err)
	}
	if items := decodeContradictions(t, out); len(items) != 1 {
		t.Fatalf("--no-write: got %d items, want 1", len(items))
	}

	// No contradicts fact should have been written.
	// We need to check that the store does NOT have contradicts facts.
	// Since store.Query would error on zero facts, seed store has 2 facts.
	out2, _, err := runStudioCmd(t, "facts", "--workspace", wsPath,
		"--predicate", "contradicts", "--count")
	if err != nil {
		t.Fatalf("studio facts count: %v", err)
	}
	if strings.TrimSpace(out2) != "0" {
		t.Errorf("contradicts count with --no-write = %q, want 0", strings.TrimSpace(out2))
	}
}

// AC: contradictions-ignore-suppresses — an entry in the ignore file suppresses
// the matched item from both the output and the write-back. --show-ignored
// lists the suppressed identity.
func TestStudioContradictions_IgnoreSuppressesItemAndWriteBack(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	wsDir := filepath.Dir(wsPath)
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("dead.example", "serves-status", "200", "domains.json"),
		verifiedFact("dead.example", "serves-status", "down", "https://dead.example/"),
	})

	// Build the canonical identity string by running contradictions once (no
	// write) to find the refs, then suppress them.
	out, _, err := runStudioCmd(t, "contradictions", "--workspace", wsPath,
		"--no-write", "--format", "json")
	if err != nil {
		t.Fatalf("initial contradictions run: %v", err)
	}
	items := decodeContradictions(t, out)
	if len(items) != 1 {
		t.Fatalf("expected 1 item before suppression, got %d", len(items))
	}

	// Derive the canonical refs: the fact refs for side A and B, smaller first.
	refA := items[0].A.Subject + "|" + items[0].A.Predicate + "|" + items[0].A.Object
	refB := items[0].B.Subject + "|" + items[0].B.Predicate + "|" + items[0].B.Object
	if refA > refB {
		refA, refB = refB, refA
	}
	identity := refA + "  " + refB

	// Write the ignore file: the identity line carries an inline reason comment
	// (REQ: suppression-ignore-list — the reason on the matching line).
	ignoreDir := filepath.Join(wsDir, ".specscore-studio")
	if err := os.MkdirAll(ignoreDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ignoreFile := filepath.Join(ignoreDir, "contradictions-ignore.txt")
	if err := os.WriteFile(ignoreFile,
		[]byte("# known conflict\n"+identity+" # accepted until Q3\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Run with the ignore file active: item should be suppressed from output.
	out2, _, err := runStudioCmd(t, "contradictions", "--workspace", wsPath, "--format", "json")
	if err != nil {
		t.Fatalf("studio contradictions with ignore: %v", err)
	}
	if items2 := decodeContradictions(t, out2); len(items2) != 0 {
		t.Errorf("suppressed item still appears in output: %+v", items2)
	}

	// No contradicts fact should have been written.
	out3, _, err := runStudioCmd(t, "facts", "--workspace", wsPath,
		"--predicate", "contradicts", "--count")
	if err != nil {
		t.Fatalf("studio facts count: %v", err)
	}
	if strings.TrimSpace(out3) != "0" {
		t.Errorf("contradicts count after suppressed run = %q, want 0", strings.TrimSpace(out3))
	}

	// --show-ignored lists the suppressed item marked as ignored, with its
	// canonical "<side-a>  <side-b>" identity and the reason comment — JSON form.
	out4, _, err := runStudioCmd(t, "contradictions", "--workspace", wsPath,
		"--show-ignored", "--format", "json")
	if err != nil {
		t.Fatalf("studio contradictions --show-ignored: %v", err)
	}
	// With --show-ignored the suppressed item should appear with an [ignored]
	// marker in its detector field.
	if !strings.Contains(out4, "[ignored]") {
		t.Errorf("--show-ignored output missing [ignored] marker:\n%s", out4)
	}
	// The canonical identity string must literally appear in the output.
	if !strings.Contains(out4, identity) {
		t.Errorf("--show-ignored JSON missing the canonical identity %q:\n%s", identity, out4)
	}
	// The typed JSON fields carry the identity and reason.
	var shown []struct {
		Detector     string `json:"detector"`
		Ignored      bool   `json:"ignored"`
		Identity     string `json:"identity"`
		IgnoreReason string `json:"ignore_reason"`
	}
	if err := json.Unmarshal([]byte(out4), &shown); err != nil {
		t.Fatalf("--show-ignored output is not JSON: %v\n%s", err, out4)
	}
	if len(shown) != 1 {
		t.Fatalf("--show-ignored got %d items, want 1:\n%s", len(shown), out4)
	}
	if !shown[0].Ignored {
		t.Errorf("--show-ignored item not marked ignored: %+v", shown[0])
	}
	if shown[0].Identity != identity {
		t.Errorf("identity = %q, want %q", shown[0].Identity, identity)
	}
	if shown[0].IgnoreReason != "accepted until Q3" {
		t.Errorf("ignore_reason = %q, want %q", shown[0].IgnoreReason, "accepted until Q3")
	}

	// Human form: the identity and reason appear too.
	out5, _, err := runStudioCmd(t, "contradictions", "--workspace", wsPath, "--show-ignored")
	if err != nil {
		t.Fatalf("studio contradictions --show-ignored (human): %v", err)
	}
	for _, want := range []string{
		"[ignored]",
		"suppressed: " + identity,
		"reason: accepted until Q3",
	} {
		if !strings.Contains(out5, want) {
			t.Errorf("--show-ignored human output missing %q:\n%s", want, out5)
		}
	}
}

// A suppressed item whose ignore-file line has no inline reason comment lists
// its identity under --show-ignored without a reason field/line.
func TestStudioContradictions_ShowIgnoredWithoutReason(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	wsDir := filepath.Dir(wsPath)
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("dead.example", "serves-status", "200", "domains.json"),
		verifiedFact("dead.example", "serves-status", "down", "https://dead.example/"),
	})

	identity := "dead.example|serves-status|200  dead.example|serves-status|down"
	ignoreDir := filepath.Join(wsDir, ".specscore-studio")
	if err := os.MkdirAll(ignoreDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ignoreDir, "contradictions-ignore.txt"),
		[]byte(identity+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// JSON: identity present, ignore_reason omitted (empty).
	out, _, err := runStudioCmd(t, "contradictions", "--workspace", wsPath,
		"--show-ignored", "--no-write", "--format", "json")
	if err != nil {
		t.Fatalf("studio contradictions --show-ignored: %v", err)
	}
	if !strings.Contains(out, identity) {
		t.Errorf("JSON output missing identity %q:\n%s", identity, out)
	}
	if strings.Contains(out, "ignore_reason") {
		t.Errorf("JSON output must omit ignore_reason when the line has no comment:\n%s", out)
	}

	// Human: identity line present, no reason line.
	out2, _, err := runStudioCmd(t, "contradictions", "--workspace", wsPath,
		"--show-ignored", "--no-write")
	if err != nil {
		t.Fatalf("studio contradictions --show-ignored (human): %v", err)
	}
	if !strings.Contains(out2, "suppressed: "+identity) {
		t.Errorf("human output missing 'suppressed: %s':\n%s", identity, out2)
	}
	if strings.Contains(out2, "reason:") {
		t.Errorf("human output must omit the reason line when the ignore line has no comment:\n%s", out2)
	}
}

// TestStudioContradictions_IgnoreFileCustomPath checks that --ignore-file
// overrides the default ignore-file location.
func TestStudioContradictions_IgnoreFileCustomPath(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("dead.example", "serves-status", "200", "domains.json"),
		verifiedFact("dead.example", "serves-status", "down", "https://dead.example/"),
	})

	// Get the canonical identity for the one item.
	out, _, err := runStudioCmd(t, "contradictions", "--workspace", wsPath,
		"--no-write", "--format", "json")
	if err != nil {
		t.Fatalf("initial run: %v", err)
	}
	items := decodeContradictions(t, out)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	refA := items[0].A.Subject + "|" + items[0].A.Predicate + "|" + items[0].A.Object
	refB := items[0].B.Subject + "|" + items[0].B.Predicate + "|" + items[0].B.Object
	if refA > refB {
		refA, refB = refB, refA
	}
	identity := refA + "  " + refB

	// Write the ignore file at a custom path.
	customIgnore := filepath.Join(t.TempDir(), "my-ignore.txt")
	if err := os.WriteFile(customIgnore, []byte(identity+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out2, _, err := runStudioCmd(t, "contradictions", "--workspace", wsPath,
		"--ignore-file", customIgnore, "--no-write", "--format", "json")
	if err != nil {
		t.Fatalf("studio contradictions --ignore-file: %v", err)
	}
	if items2 := decodeContradictions(t, out2); len(items2) != 0 {
		t.Errorf("custom ignore file did not suppress the item: %+v", items2)
	}
}

// An unreadable ignore file (bad path specified with --ignore-file) exits 2.
func TestStudioContradictions_UnreadableIgnoreFileExits2(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("dead.example", "serves-status", "200", "domains.json"),
		verifiedFact("dead.example", "serves-status", "down", "https://dead.example/"),
	})

	_, _, err := runStudioCmd(t, "contradictions", "--workspace", wsPath,
		"--ignore-file", "/no/such/path/ignore.txt")
	if code := studioExit(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2 for unreadable --ignore-file", code)
	}
	if !strings.Contains(err.Error(), "ignore.txt") {
		t.Errorf("error %q does not name the ignore file path", err.Error())
	}
}

// A merge failure from the write-back exits non-zero; the human-readable error
// is propagated.
func TestStudioContradictions_WriteBackMergeFailurePropagates(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("dead.example", "serves-status", "200", "domains.json"),
		verifiedFact("dead.example", "serves-status", "down", "https://dead.example/"),
	})

	old := storeMergeFn
	storeMergeFn = func(string, []fact.Fact) (store.MergeResult, error) {
		return store.MergeResult{}, errors.New("merge boom")
	}
	t.Cleanup(func() { storeMergeFn = old })

	_, _, err := runStudioCmd(t, "contradictions", "--workspace", wsPath)
	if err == nil {
		t.Fatal("expected the merge error to propagate")
	}
	if !strings.Contains(err.Error(), "merge boom") {
		t.Errorf("error %q does not carry the merge failure", err.Error())
	}
}
