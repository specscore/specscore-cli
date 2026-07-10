package fact

import (
	"encoding/json"
	"testing"
)

func TestClasses(t *testing.T) {
	// Verify the three evidence-class constants have the expected string values
	// so marshalled JSON facts carry the right evidence_class strings.
	// Feature: cli/rehearse/evidence (REQ: verified-behavior-class)
	if Declared != "declared" {
		t.Errorf("Declared = %q, want %q", Declared, "declared")
	}
	if Derived != "derived" {
		t.Errorf("Derived = %q, want %q", Derived, "derived")
	}
	if VerifiedBehavior != "verified-behavior" {
		t.Errorf("VerifiedBehavior = %q, want %q", VerifiedBehavior, "verified-behavior")
	}
}

func TestFact_JSONShape(t *testing.T) {
	f := Fact{
		Subject:   "repo:github.com/acme/app",
		Predicate: "has-status",
		Object:    "Approved",
		Evidence: Evidence{
			Class:   Declared,
			Pointer: "spec/features/x/README.md:9",
		},
		Adapter:    Adapter{ID: "specscore", Version: "1"},
		ObservedAt: "2026-07-10T00:00:00Z",
		Ecosystem:  "demo",
	}
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"subject":          "repo:github.com/acme/app",
		"predicate":        "has-status",
		"object":           "Approved",
		"evidence_class":   "declared",
		"evidence_pointer": "spec/features/x/README.md:9",
		"observed_at":      "2026-07-10T00:00:00Z",
		"ecosystem":        "demo",
	}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("JSON field %q = %v, want %q", k, m[k], v)
		}
	}
	adapter, ok := m["adapter"].(map[string]any)
	if !ok {
		t.Fatalf("JSON field adapter = %v, want an object", m["adapter"])
	}
	if adapter["id"] != "specscore" || adapter["version"] != "1" {
		t.Errorf("adapter = %v, want id specscore version 1", adapter)
	}
}

func TestFact_JSONRoundTrip(t *testing.T) {
	f := Fact{
		Subject:    "a",
		Predicate:  "imports",
		Object:     "b",
		Evidence:   Evidence{Class: Derived, Pointer: "codegraph/pkg.json"},
		Adapter:    Adapter{ID: "codegraph", Version: "2"},
		ObservedAt: "2026-07-10T00:00:00Z",
		Ecosystem:  "demo",
	}
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	var got Fact
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got != f {
		t.Errorf("round trip = %+v, want %+v", got, f)
	}
}

// --- stable-ID helpers ---

func TestRepoSlugger_BasenameAndCollisions(t *testing.T) {
	s := NewRepoSlugger()
	tests := []struct{ path, want string }{
		{"/Users/alex/projects/app", "app"},
		{"/Users/alex/projects/app", "app"}, // same path → same slug
		{"/srv/mirrors/app", "app-2"},       // collision → -N counter
		{"/tmp/checkouts/app", "app-3"},
		{"/Users/alex/projects/other", "other"},
		{"/srv/mirrors/app/", "app-2"}, // trailing slash → same dir as the mirror
	}
	for _, tt := range tests {
		if got := s.Slug(tt.path); got != tt.want {
			t.Errorf("Slug(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestRepoSlugger_CounterSlugAlreadyTaken(t *testing.T) {
	s := NewRepoSlugger()
	if got := s.Slug("/a/app-2"); got != "app-2" {
		t.Fatalf("Slug(/a/app-2) = %q, want app-2", got)
	}
	if got := s.Slug("/a/app"); got != "app" {
		t.Fatalf("Slug(/a/app) = %q, want app", got)
	}
	// app-2 is taken by a literal basename, so the collision counter skips it.
	if got := s.Slug("/b/app"); got != "app-3" {
		t.Errorf("Slug(/b/app) = %q, want app-3", got)
	}
}

func TestSpecID(t *testing.T) {
	if got := SpecID("github.com/acme/app", "cli/studio/index"); got != "github.com/acme/app#cli/studio/index" {
		t.Errorf("SpecID = %q", got)
	}
}

func TestPackageID(t *testing.T) {
	if got := PackageID("example.com/m", "v1.2.3"); got != "example.com/m@v1.2.3" {
		t.Errorf("PackageID = %q", got)
	}
	if got := PackageID("example.com/m", ""); got != "example.com/m" {
		t.Errorf("PackageID without version = %q", got)
	}
}

func TestDomainID(t *testing.T) {
	if got := DomainID("Example.APP."); got != "example.app" {
		t.Errorf("DomainID = %q", got)
	}
}
