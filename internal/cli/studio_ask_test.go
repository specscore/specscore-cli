package cli

// Feature: cli/studio/answers (REQ: ask-verb-and-router, REQ: ask-citations,
// REQ: ask-unroutable)
// Verifies: cli/studio/answers#ac:ask-routes-with-citations,
//           cli/studio/answers#ac:ask-unroutable-lists-templates,
//           cli/studio/answers#ac:ask-routed-but-empty

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/internal/studio/fact"
)

// --- AC: ask-routes-with-citations ---

// A routable question returns an answer with citations naming the supporting
// fact's evidence pointer and adapter id (REQ: ask-citations).
func TestStudioAsk_RoutesWithCitations(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("acme.app", "fronts", "acme", "domains.json"),
	})

	out, _, err := runStudioCmd(t, "ask", "--workspace", wsPath,
		"--format", "json", "who fronts acme.app")
	if err != nil {
		t.Fatalf("studio ask: %v", err)
	}
	var resp struct {
		Question  string `json:"question"`
		Template  string `json:"template"`
		Parameter string `json:"parameter"`
		Answer    string `json:"answer"`
		Citations []struct {
			Predicate string `json:"predicate"`
			Pointer   string `json:"evidence_pointer"`
			Adapter   string `json:"adapter"`
		} `json:"citations"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if resp.Template != "who-fronts" {
		t.Errorf("template = %q, want who-fronts", resp.Template)
	}
	if !strings.Contains(resp.Answer, "acme.app") {
		t.Errorf("answer %q does not name the fronting domain acme.app", resp.Answer)
	}
	if len(resp.Citations) == 0 {
		t.Fatal("citations is empty")
	}
	c := resp.Citations[0]
	if c.Predicate != "fronts" || c.Pointer != "domains.json" || c.Adapter != "registries" {
		t.Errorf("citation = %+v, want fronts/domains.json/registries", c)
	}
}

// The human format prints the answer then an Evidence block (REQ: ask-citations).
func TestStudioAsk_HumanFormatEvidenceBlock(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("acme.app", "fronts", "acme", "domains.json"),
	})

	out, _, err := runStudioCmd(t, "ask", "--workspace", wsPath, "who fronts acme")
	if err != nil {
		t.Fatalf("studio ask: %v", err)
	}
	if !strings.Contains(out, "Evidence:") {
		t.Errorf("human output missing Evidence block:\n%s", out)
	}
	if !strings.Contains(out, "fronts") || !strings.Contains(out, "domains.json") {
		t.Errorf("Evidence block missing the citing fact:\n%s", out)
	}
}

// The parameter routes through resolve semantics: a brand alias resolves to its
// canonical id before the query runs (REQ: ask-verb-and-router).
func TestStudioAsk_ParameterResolvesThroughAlias(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("sizeus", "aliased-as", "SizeChart", "ecosystem.yaml"),
		declaredFact("sizeus", "member-of", "payments", "ecosystem.yaml"),
	})

	// "member of SizeChart" → resolve SizeChart → sizeus → member-of payments.
	out, _, err := runStudioCmd(t, "ask", "--workspace", wsPath,
		"--format", "json", "member of SizeChart")
	if err != nil {
		t.Fatalf("studio ask: %v", err)
	}
	var resp struct {
		Template  string `json:"template"`
		Citations []struct {
			Object string `json:"object"`
		} `json:"citations"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if resp.Template != "member-of" {
		t.Errorf("template = %q, want member-of", resp.Template)
	}
	if len(resp.Citations) == 0 || resp.Citations[0].Object != "payments" {
		t.Errorf("expected member-of payments citation, got %+v", resp.Citations)
	}
}

// The is-it-live template does the fronts→serves-status two-step hop, citing
// both facts (REQ: ask-verb-and-router).
func TestStudioAsk_IsItLiveTwoStepHop(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("dead.example", "fronts", "acme", "domains.json"),
		verifiedFact("dead.example", "serves-status", "down", "https://dead.example/"),
	})

	out, _, err := runStudioCmd(t, "ask", "--workspace", wsPath,
		"--format", "json", "is acme live")
	if err != nil {
		t.Fatalf("studio ask: %v", err)
	}
	var resp struct {
		Template  string `json:"template"`
		Citations []struct {
			Predicate string `json:"predicate"`
			Class     string `json:"evidence_class"`
		} `json:"citations"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if resp.Template != "is-it-live" {
		t.Errorf("template = %q, want is-it-live", resp.Template)
	}
	var haveFronts, haveServes bool
	for _, c := range resp.Citations {
		if c.Predicate == "fronts" {
			haveFronts = true
		}
		if c.Predicate == "serves-status" && c.Class == "verified-behavior" {
			haveServes = true
		}
	}
	if !haveFronts || !haveServes {
		t.Errorf("is-it-live citations miss the two-step hop: %+v", resp.Citations)
	}
}

// --- AC: ask-unroutable-lists-templates ---

// An unroutable question exits 1, prints a "no template matched" notice, and
// lists the routable templates.
func TestStudioAsk_UnroutableExits1AndListsTemplates(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("acme.app", "fronts", "acme", "domains.json"),
	})

	out, _, err := runStudioCmd(t, "ask", "--workspace", wsPath, "why does contactus exist")
	if code := studioExit(t, err); code != 1 {
		t.Errorf("exit code = %d, want 1 (unroutable)", code)
	}
	if !strings.Contains(out, "no template matched") {
		t.Errorf("output missing the 'no template matched' notice:\n%s", out)
	}
	// Lists the routable templates (same content as --list).
	for _, id := range []string{"who-fronts", "status-of", "aliases-resolve"} {
		if !strings.Contains(out, id) {
			t.Errorf("template list missing %q:\n%s", id, out)
		}
	}
}

// --list prints the routable templates and exits 0 without a question.
func TestStudioAsk_ListExits0(t *testing.T) {
	out, _, err := runStudioCmd(t, "ask", "--list")
	if err != nil {
		t.Fatalf("studio ask --list: %v", err)
	}
	if !strings.Contains(out, "Routable templates:") {
		t.Errorf("--list output missing header:\n%s", out)
	}
	// All 13 template ids appear.
	for _, id := range []string{
		"who-fronts", "what-repos-implement", "status-of", "aliases-of",
		"member-of", "is-it-live", "ci-status-of", "what-verifies",
		"contradictions-for", "freshness-of", "what-uses", "version-pins",
		"aliases-resolve",
	} {
		if !strings.Contains(out, id) {
			t.Errorf("--list missing template %q:\n%s", id, out)
		}
	}
}

// The unroutable list content matches the --list content (REQ: ask-unroutable).
func TestStudioAsk_UnroutableListMatchesListFlag(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("acme.app", "fronts", "acme", "domains.json"),
	})

	listOut, _, err := runStudioCmd(t, "ask", "--list")
	if err != nil {
		t.Fatalf("--list: %v", err)
	}
	unroutableOut, _, _ := runStudioCmd(t, "ask", "--workspace", wsPath, "why does contactus exist")
	// The template list section of the unroutable output equals the --list output.
	if !strings.Contains(unroutableOut, strings.TrimSpace(listOut)) {
		t.Errorf("unroutable list does not match --list content.\n--list:\n%s\nunroutable:\n%s",
			listOut, unroutableOut)
	}
}

// --- AC: ask-routed-but-empty ---

// A routed question whose parameter resolves to no facts exits 3, not 0, with a
// message distinguishing routed-but-no-data from unroutable, and no citation-
// free answer.
func TestStudioAsk_RoutedButEmptyExits3(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	// A store with a fact so it is non-empty, but no fronts fact for the queried
	// domain.
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("acme.app", "fronts", "acme", "domains.json"),
	})

	out, _, err := runStudioCmd(t, "ask", "--workspace", wsPath, "who fronts unknown.example")
	if code := studioExit(t, err); code != 3 {
		t.Errorf("exit code = %d, want 3 (routed-but-empty)", code)
	}
	if !strings.Contains(err.Error(), "routed to who-fronts") {
		t.Errorf("error %q does not name the routed template", err.Error())
	}
	if !strings.Contains(err.Error(), "unknown.example") {
		t.Errorf("error %q does not name the parameter", err.Error())
	}
	// No citation-free answer prose is printed.
	if strings.Contains(out, "Evidence:") || strings.Contains(out, "fronts edge") {
		t.Errorf("routed-but-empty leaked an answer:\n%s", out)
	}
}

// --- ambiguous parameter ---

// A parameter that resolves to multiple candidates exits 5 (AmbiguousSlug) and
// lists the candidates.
func TestStudioAsk_AmbiguousParameterExits5(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("product-a", "aliased-as", "shared", "a.yaml"),
		declaredFact("product-b", "aliased-as", "shared", "b.yaml"),
	})

	_, _, err := runStudioCmd(t, "ask", "--workspace", wsPath, "who fronts shared")
	if code := studioExit(t, err); code != 5 {
		t.Errorf("exit code = %d, want 5 (AmbiguousSlug)", code)
	}
	if !strings.Contains(err.Error(), "product-a") || !strings.Contains(err.Error(), "product-b") {
		t.Errorf("error %q does not list both candidates", err.Error())
	}
}

// --- error handling ---

// An empty question is a usage error (exit 2).
func TestStudioAsk_EmptyQuestionExits2(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("acme.app", "fronts", "acme", "domains.json"),
	})

	_, _, err := runStudioCmd(t, "ask", "--workspace", wsPath, "   ")
	if code := studioExit(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2 (InvalidArgs) for empty question", code)
	}
}

// No question argument at all is a usage error (exit 2).
func TestStudioAsk_MissingQuestionExits2(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("acme.app", "fronts", "acme", "domains.json"),
	})

	_, _, err := runStudioCmd(t, "ask", "--workspace", wsPath)
	if code := studioExit(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2 (InvalidArgs) for missing question", code)
	}
}

// A multi-word question passed as separate args is joined into one question.
func TestStudioAsk_MultiWordArgsJoined(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("acme.app", "fronts", "acme", "domains.json"),
	})

	out, _, err := runStudioCmd(t, "ask", "--workspace", wsPath,
		"--format", "json", "who", "fronts", "acme")
	if err != nil {
		t.Fatalf("studio ask with split args: %v", err)
	}
	if !strings.Contains(out, "who-fronts") {
		t.Errorf("split-arg question did not route to who-fronts:\n%s", out)
	}
}

// A bad --format exits 2 and names the offending value.
func TestStudioAsk_BadFormatExits2(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("acme.app", "fronts", "acme", "domains.json"),
	})

	_, _, err := runStudioCmd(t, "ask", "--workspace", wsPath, "--format", "yaml", "who fronts acme")
	if code := studioExit(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2 for bad --format", code)
	}
	if !strings.Contains(err.Error(), "yaml") {
		t.Errorf("error %q does not name the bad format", err.Error())
	}
}

// Running ask before studio index exits 2 with the store path and the
// studio-index suggestion.
func TestStudioAsk_MissingStoreExits2(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a") // no store built
	wantDB := filepath.Join(filepath.Dir(wsPath), ".specscore-studio", "facts.db")

	_, _, err := runStudioCmd(t, "ask", "--workspace", wsPath, "who fronts acme")
	if code := studioExit(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(err.Error(), wantDB) || !strings.Contains(err.Error(), "studio index") {
		t.Errorf("error %q must name the store path and suggest studio index", err.Error())
	}
}

// A bad --db path (abs resolution failure) exits 2.
func TestStudioAsk_DBFlagAbsError(t *testing.T) {
	old := filepathAbsFn
	filepathAbsFn = func(string) (string, error) { return "", errors.New("abs boom") }
	t.Cleanup(func() { filepathAbsFn = old })

	_, _, err := runStudioCmd(t, "ask", "--db", "rel.db", "who fronts acme")
	if code := studioExit(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

// --db bypasses the workspace: ask reads the store directly.
func TestStudioAsk_DBFlagBypassesWorkspace(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	dbPath := seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("acme.app", "fronts", "acme", "domains.json"),
	})

	out, _, err := runStudioCmd(t, "ask", "--db", dbPath, "who fronts acme")
	if err != nil {
		t.Fatalf("studio ask --db: %v", err)
	}
	if !strings.Contains(out, "acme.app") {
		t.Errorf("ask via --db did not answer:\n%s", out)
	}
}

// A routed-but-empty case where the entity resolves uniquely but the template
// query finds no facts (an entity present in the store but with no fact of the
// template's predicate) still exits 3.
func TestStudioAsk_ResolvedEntityNoTemplateFactsExits3(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		// sizeus exists as an entity id (member-of subject) but has no ci-status.
		declaredFact("sizeus", "member-of", "payments", "ecosystem.yaml"),
	})

	_, _, err := runStudioCmd(t, "ask", "--workspace", wsPath, "ci status of sizeus")
	if code := studioExit(t, err); code != 3 {
		t.Errorf("exit code = %d, want 3 (routed-but-empty)", code)
	}
}

// The studio command group registers the ask subcommand.
func TestStudio_HasAskCommand(t *testing.T) {
	cmd := studioCommand()
	for _, c := range cmd.Commands() {
		if c.Name() == "ask" {
			return
		}
	}
	t.Error("studio command group does not register the ask subcommand")
}
