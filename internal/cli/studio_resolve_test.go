package cli

// Feature: cli/studio/answers (REQ: resolve-verb,
// REQ: resolve-ambiguous-and-unknown)
// Verifies: cli/studio/answers#ac:resolve-alias-to-canonical,
//           cli/studio/answers#ac:resolve-ambiguous-lists-candidates,
//           cli/studio/answers#ac:resolve-unknown-guides

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/internal/studio/fact"
)

// --- AC: resolve-alias-to-canonical ---

// A brand name resolves to its canonical entity id via the aliased-as fact
// (the SizeChart → sizeus benchmark anchor from REQ: resolve-verb).
func TestStudioResolve_AliasToCanonical(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("sizeus", "aliased-as", "SizeChart", "ecosystem.yaml"),
	})

	out, _, err := runStudioCmd(t, "resolve", "--workspace", wsPath, "SizeChart")
	if err != nil {
		t.Fatalf("studio resolve: %v", err)
	}
	if strings.TrimSpace(out) != "sizeus" {
		t.Errorf("resolve SizeChart = %q, want sizeus", strings.TrimSpace(out))
	}
}

// Case-insensitive — different case of the same name resolves identically.
func TestStudioResolve_AliasToCanonicalCaseInsensitive(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("sizeus", "aliased-as", "SizeChart", "ecosystem.yaml"),
	})

	out, _, err := runStudioCmd(t, "resolve", "--workspace", wsPath, "sizechart")
	if err != nil {
		t.Fatalf("studio resolve: %v", err)
	}
	if strings.TrimSpace(out) != "sizeus" {
		t.Errorf("resolve sizechart = %q, want sizeus", strings.TrimSpace(out))
	}
}

// Resolves via entity id when the name matches a fact subject.
func TestStudioResolve_EntityIDSubject(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("acme-repo", "member-of", "platform", "registry.json"),
	})

	out, _, err := runStudioCmd(t, "resolve", "--workspace", wsPath, "acme-repo")
	if err != nil {
		t.Fatalf("studio resolve: %v", err)
	}
	if strings.TrimSpace(out) != "acme-repo" {
		t.Errorf("resolve acme-repo = %q, want acme-repo", strings.TrimSpace(out))
	}
}

// --format json outputs an object with id, kind, and citation fields.
func TestStudioResolve_JSONFormat(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("sizeus", "aliased-as", "SizeChart", "ecosystem.yaml"),
	})

	out, _, err := runStudioCmd(t, "resolve", "--workspace", wsPath,
		"--format", "json", "SizeChart")
	if err != nil {
		t.Fatalf("studio resolve --format json: %v", err)
	}
	var resp struct {
		ID       string `json:"id"`
		Kind     string `json:"kind"`
		Citation struct {
			Subject   string `json:"subject"`
			Predicate string `json:"predicate"`
			Object    string `json:"object"`
		} `json:"citation"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if resp.ID != "sizeus" {
		t.Errorf("id = %q, want sizeus", resp.ID)
	}
	if resp.Kind == "" {
		t.Error("kind is empty")
	}
	if resp.Citation.Predicate != "aliased-as" {
		t.Errorf("citation.predicate = %q, want aliased-as", resp.Citation.Predicate)
	}
}

// --- AC: resolve-ambiguous-lists-candidates ---

// An ambiguous name lists all candidates and exits 5.
func TestStudioResolve_AmbiguousListsCandidates(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("product-a", "aliased-as", "shared", "a.yaml"),
		declaredFact("product-b", "aliased-as", "shared", "b.yaml"),
	})

	_, _, err := runStudioCmd(t, "resolve", "--workspace", wsPath, "shared")
	if code := studioExit(t, err); code != 5 {
		t.Errorf("exit code = %d, want 5 (AmbiguousSlug)", code)
	}
	if !strings.Contains(err.Error(), "product-a") || !strings.Contains(err.Error(), "product-b") {
		t.Errorf("error %q does not list both candidates", err.Error())
	}
}

// Ambiguous with --format json outputs a JSON array of candidates.
func TestStudioResolve_AmbiguousJSONFormat(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("product-a", "aliased-as", "shared", "a.yaml"),
		declaredFact("product-b", "aliased-as", "shared", "b.yaml"),
	})

	out, _, err := runStudioCmd(t, "resolve", "--workspace", wsPath,
		"--format", "json", "shared")
	if code := studioExit(t, err); code != 5 {
		t.Errorf("exit code = %d, want 5 (AmbiguousSlug)", code)
	}
	var resp struct {
		Candidates []struct {
			ID       string `json:"id"`
			Citation struct {
				Predicate string `json:"predicate"`
			} `json:"citation"`
		} `json:"candidates"`
	}
	if err2 := json.Unmarshal([]byte(out), &resp); err2 != nil {
		t.Fatalf("output is not JSON: %v\n%s", err2, out)
	}
	if len(resp.Candidates) != 2 {
		t.Fatalf("candidates len = %d, want 2", len(resp.Candidates))
	}
	for _, c := range resp.Candidates {
		if c.ID == "" {
			t.Error("candidate id is empty")
		}
		if c.Citation.Predicate == "" {
			t.Errorf("candidate %q citation predicate is empty", c.ID)
		}
	}
}

// JSON encoder failures are returned before the command constructs its normal
// ambiguous-name exit error.
func TestStudioResolve_AmbiguousJSONWriteError(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("product-a", "aliased-as", "shared", "a.yaml"),
		declaredFact("product-b", "aliased-as", "shared", "b.yaml"),
	})

	cmd := studioResolveCommand()
	cmd.SetOut(covErrWriter{})
	if err := cmd.Flags().Set("workspace", wsPath); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("format", "json"); err != nil {
		t.Fatal(err)
	}
	err := runStudioResolve(cmd, []string{"shared"})
	if err == nil || !strings.Contains(err.Error(), "cov write error") {
		t.Fatalf("runStudioResolve error = %v, want encoder write error", err)
	}
}

// --- AC: resolve-unknown-guides ---

// An unknown name exits 3 with guidance naming what was searched.
func TestStudioResolve_UnknownExits3WithGuidance(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("sizeus", "member-of", "payments", "registry.json"),
	})

	_, _, err := runStudioCmd(t, "resolve", "--workspace", wsPath, "nonexistent")
	if code := studioExit(t, err); code != 3 {
		t.Errorf("exit code = %d, want 3 (NotFound)", code)
	}
	// Error names what was searched: aliases and entity ids.
	if !strings.Contains(err.Error(), "alias") && !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error %q does not name what was searched", err.Error())
	}
	// Error suggests studio facts exploration.
	if !strings.Contains(err.Error(), "studio facts") {
		t.Errorf("error %q does not suggest studio facts exploration", err.Error())
	}
}

// --- error handling ---

// An empty name is a usage error (exit 2).
func TestStudioResolve_EmptyNameExits2(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("sizeus", "member-of", "payments", "registry.json"),
	})

	_, _, err := runStudioCmd(t, "resolve", "--workspace", wsPath, "")
	if code := studioExit(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2 (InvalidArgs) for empty name", code)
	}
}

// Whitespace-only name is a usage error (exit 2).
func TestStudioResolve_WhitespaceNameExits2(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("sizeus", "member-of", "payments", "registry.json"),
	})

	_, _, err := runStudioCmd(t, "resolve", "--workspace", wsPath, "   ")
	if code := studioExit(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2 (InvalidArgs) for whitespace-only name", code)
	}
}

// Missing name argument is a usage error (exit 2).
func TestStudioResolve_MissingArgExits2(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("sizeus", "member-of", "payments", "registry.json"),
	})

	_, _, err := runStudioCmd(t, "resolve", "--workspace", wsPath)
	if code := studioExit(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2 (InvalidArgs) for missing name", code)
	}
}

// Too many name arguments is a usage error (exit 2).
func TestStudioResolve_TooManyArgsExits2(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("sizeus", "member-of", "payments", "registry.json"),
	})

	_, _, err := runStudioCmd(t, "resolve", "--workspace", wsPath, "a", "b")
	if code := studioExit(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2 (InvalidArgs) for too many args", code)
	}
}

// Running resolve before studio index exits 2 with the store path and the
// studio-index suggestion.
func TestStudioResolve_MissingStoreExits2(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a") // no store built
	wantDB := filepath.Join(filepath.Dir(wsPath), ".specscore-studio", "facts.db")

	_, _, err := runStudioCmd(t, "resolve", "--workspace", wsPath, "SizeChart")
	if code := studioExit(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(err.Error(), wantDB) || !strings.Contains(err.Error(), "studio index") {
		t.Errorf("error %q must name the store path and suggest studio index", err.Error())
	}
}

// A bad --format exits 2 and names the offending value.
func TestStudioResolve_BadFormatExits2(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("sizeus", "aliased-as", "SizeChart", "ecosystem.yaml"),
	})

	_, _, err := runStudioCmd(t, "resolve", "--workspace", wsPath,
		"--format", "yaml", "SizeChart")
	if code := studioExit(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2 for bad --format", code)
	}
	if !strings.Contains(err.Error(), "yaml") {
		t.Errorf("error %q does not name the bad format", err.Error())
	}
}

// A bad --db path (abs resolution failure) exits 2.
func TestStudioResolve_DBFlagAbsError(t *testing.T) {
	old := filepathAbsFn
	filepathAbsFn = func(string) (string, error) { return "", errors.New("abs boom") }
	t.Cleanup(func() { filepathAbsFn = old })

	_, _, err := runStudioCmd(t, "resolve", "--db", "rel.db", "SizeChart")
	if code := studioExit(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

// --db bypasses the workspace: resolve reads the store directly.
func TestStudioResolve_DBFlagBypassesWorkspace(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	dbPath := seedProbeStore(t, wsPath, []fact.Fact{
		declaredFact("sizeus", "aliased-as", "SizeChart", "ecosystem.yaml"),
	})

	out, _, err := runStudioCmd(t, "resolve", "--db", dbPath, "SizeChart")
	if err != nil {
		t.Fatalf("studio resolve --db: %v", err)
	}
	if strings.TrimSpace(out) != "sizeus" {
		t.Errorf("resolve via --db = %q, want sizeus", strings.TrimSpace(out))
	}
}
