package graph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type exitCoder interface{ ExitCode() int }

func exitOf(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	ec, ok := err.(exitCoder)
	if !ok {
		t.Fatalf("error has no exit code: %v", err)
	}
	return ec.ExitCode()
}

func TestScaffold_ModuleFullThenArtifacts(t *testing.T) {
	root := repoWith(t, map[string]string{})
	res, err := Scaffold(ScaffoldOptions{Kind: KindModule, Name: "Identity", RepoRoot: root})
	if err != nil {
		t.Fatalf("module scaffold: %v", err)
	}
	if res.Target != "spec/graph/modules/identity/README.md" {
		t.Fatalf("unexpected target %q", res.Target)
	}
	// Six collection READMEs + module README = 7 files.
	if len(res.Created) != 7 {
		t.Fatalf("expected 7 files, got %d: %v", len(res.Created), res.Created)
	}
	// The generated module must lint clean.
	if v := lintRepo(t, root).Violations; len(v) != 0 {
		t.Fatalf("scaffolded module not clean: %+v", v)
	}

	// entity under the new module.
	if _, err := Scaffold(ScaffoldOptions{Kind: KindEntity, Name: "User", Module: "identity", RepoRoot: root}); err != nil {
		t.Fatalf("entity scaffold: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "spec/graph/modules/identity/entities/user.md")); err != nil {
		t.Fatalf("entity file missing: %v", err)
	}
	// command with subject, event.
	if _, err := Scaffold(ScaffoldOptions{Kind: KindCommand, Name: "CreateUser", Module: "identity", Subject: "identity.user", RepoRoot: root}); err != nil {
		t.Fatalf("command scaffold: %v", err)
	}
	if _, err := Scaffold(ScaffoldOptions{Kind: KindEvent, Name: "UserCreated", Module: "identity", RepoRoot: root}); err != nil {
		t.Fatalf("event scaffold: %v", err)
	}
	// relationship needs a second entity to resolve endpoints.
	if _, err := Scaffold(ScaffoldOptions{Kind: KindEntity, Name: "Team", Module: "identity", RepoRoot: root}); err != nil {
		t.Fatalf("team scaffold: %v", err)
	}
	if _, err := Scaffold(ScaffoldOptions{Kind: KindRelationship, Name: "member", Module: "identity", From: "identity.user", To: "identity.team", RepoRoot: root}); err != nil {
		t.Fatalf("relationship scaffold: %v", err)
	}
	if v := lintRepo(t, root).Violations; len(v) != 0 {
		t.Fatalf("scaffolded graph not clean: %+v", v)
	}
}

func TestScaffold_BareModule(t *testing.T) {
	root := repoWith(t, map[string]string{})
	res, err := Scaffold(ScaffoldOptions{Kind: KindModule, Name: "Bare", Bare: true, RepoRoot: root, Status: "active"})
	if err != nil {
		t.Fatalf("bare module: %v", err)
	}
	if len(res.Created) != 1 {
		t.Fatalf("bare module should create only the README, got %v", res.Created)
	}
}

func TestScaffold_ModuleCollision(t *testing.T) {
	root := repoWith(t, map[string]string{})
	if _, err := Scaffold(ScaffoldOptions{Kind: KindModule, Name: "M", RepoRoot: root}); err != nil {
		t.Fatal(err)
	}
	_, err := Scaffold(ScaffoldOptions{Kind: KindModule, Name: "M", RepoRoot: root})
	if code := exitOf(t, err); code != 1 {
		t.Fatalf("expected exit 1 on module collision, got %d", code)
	}
}

func TestScaffold_ArtifactCollision(t *testing.T) {
	root := repoWith(t, map[string]string{})
	_, _ = Scaffold(ScaffoldOptions{Kind: KindModule, Name: "M", RepoRoot: root})
	if _, err := Scaffold(ScaffoldOptions{Kind: KindEntity, Name: "X", Module: "m", RepoRoot: root}); err != nil {
		t.Fatal(err)
	}
	_, err := Scaffold(ScaffoldOptions{Kind: KindEntity, Name: "X", Module: "m", RepoRoot: root})
	if code := exitOf(t, err); code != 1 {
		t.Fatalf("expected exit 1 on artifact collision, got %d", code)
	}
}

func TestScaffold_Errors(t *testing.T) {
	root := repoWith(t, map[string]string{})
	_, _ = Scaffold(ScaffoldOptions{Kind: KindModule, Name: "M", RepoRoot: root})

	t.Run("module-prefixed-id", func(t *testing.T) {
		_, err := Scaffold(ScaffoldOptions{Kind: KindEntity, ID: "m.x", Module: "m", RepoRoot: root})
		if code := exitOf(t, err); code != 2 {
			t.Fatalf("want 2, got %d", code)
		}
	})
	t.Run("non-kebab-id", func(t *testing.T) {
		_, err := Scaffold(ScaffoldOptions{Kind: KindEntity, ID: "BadID", Module: "m", RepoRoot: root})
		if code := exitOf(t, err); code != 2 {
			t.Fatalf("want 2, got %d", code)
		}
	})
	t.Run("underivable-id", func(t *testing.T) {
		_, err := Scaffold(ScaffoldOptions{Kind: KindEntity, Name: "!!!", Module: "m", RepoRoot: root})
		if code := exitOf(t, err); code != 2 {
			t.Fatalf("want 2, got %d", code)
		}
	})
	t.Run("missing-module", func(t *testing.T) {
		_, err := Scaffold(ScaffoldOptions{Kind: KindEntity, Name: "X", RepoRoot: root})
		if code := exitOf(t, err); code != 2 {
			t.Fatalf("want 2, got %d", code)
		}
	})
	t.Run("relationship-missing-endpoints", func(t *testing.T) {
		_, err := Scaffold(ScaffoldOptions{Kind: KindRelationship, Name: "R", Module: "m", RepoRoot: root})
		if code := exitOf(t, err); code != 2 {
			t.Fatalf("want 2, got %d", code)
		}
	})
	t.Run("module-not-found", func(t *testing.T) {
		_, err := Scaffold(ScaffoldOptions{Kind: KindEntity, Name: "X", Module: "ghost", RepoRoot: root})
		if code := exitOf(t, err); code != 3 {
			t.Fatalf("want 3, got %d", code)
		}
	})
	t.Run("relationship-unresolved-endpoint", func(t *testing.T) {
		_, err := Scaffold(ScaffoldOptions{Kind: KindRelationship, Name: "R", Module: "m", From: "m.ghost", To: "m.also", RepoRoot: root})
		if code := exitOf(t, err); code != 3 {
			t.Fatalf("want 3, got %d", code)
		}
	})
}

func TestScaffold_ArtifactNoGraphRoot(t *testing.T) {
	root := repoWith(t, map[string]string{}) // no graph root yet
	_, err := Scaffold(ScaffoldOptions{Kind: KindEntity, Name: "X", Module: "m", RepoRoot: root})
	if code := exitOf(t, err); code != 3 {
		t.Fatalf("want 3 when no graph root, got %d", code)
	}
}

func TestScaffold_RelationshipUnresolvedTo(t *testing.T) {
	root := repoWith(t, map[string]string{})
	_, _ = Scaffold(ScaffoldOptions{Kind: KindModule, Name: "M", RepoRoot: root})
	_, _ = Scaffold(ScaffoldOptions{Kind: KindEntity, Name: "A", Module: "m", RepoRoot: root})
	// from resolves, to does not — covers the second endpoint branch.
	_, err := Scaffold(ScaffoldOptions{Kind: KindRelationship, Name: "R", Module: "m", From: "m.a", To: "m.ghost", RepoRoot: root})
	if code := exitOf(t, err); code != 3 {
		t.Fatalf("want 3, got %d", code)
	}
}

func TestScaffold_WriteError(t *testing.T) {
	root := repoWith(t, map[string]string{
		// A regular file where the module directory must be created.
		"spec/graph/modules/m": "i am a file",
	})
	_, err := Scaffold(ScaffoldOptions{Kind: KindModule, Name: "M", RepoRoot: root})
	if code := exitOf(t, err); code != 10 {
		t.Fatalf("want 10 (unexpected) on write error, got %d", code)
	}
}

func TestDeriveID(t *testing.T) {
	cases := map[string]string{
		"Booking":       "booking",
		"CreateBooking": "create-booking",
		"Team Member":   "team-member",
		"teamMember":    "team-member",
		"":              "",
		"!!!":           "",
		"A1B2":          "a1-b2",
	}
	for in, want := range cases {
		if got := DeriveID(in); got != want {
			t.Errorf("DeriveID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestScaffoldHelpers(t *testing.T) {
	if capitalizeFirst("") != "" {
		t.Error("capitalizeFirst empty")
	}
	if capitalizeFirst("abc") != "Abc" {
		t.Error("capitalizeFirst")
	}
	if yamlScalar("simple") != "simple" {
		t.Error("yamlScalar bare")
	}
	if got := yamlScalar("has: colon"); got != "\"has: colon\"" {
		t.Errorf("yamlScalar quote: %q", got)
	}
	if got := yamlScalar(""); got != "\"\"" {
		t.Errorf("yamlScalar empty: %q", got)
	}
	if !strings.HasPrefix(collectionDescription("entities"), "EntitySpec") {
		t.Error("collectionDescription entities")
	}
	for _, c := range []string{"relationships", "commands", "events", "models"} {
		if collectionDescription(c) == "" {
			t.Errorf("collectionDescription %q empty", c)
		}
	}
	if collectionTitle("entities") != "Entities" {
		t.Error("collectionTitle")
	}
	if describe("", "entity", "X") == "" || describe("sum", "entity", "X") != "sum" {
		t.Error("describe")
	}
	if relTo("relative", "/absolute/path") != "/absolute/path" {
		t.Error("relTo error branch should return abs")
	}
}
