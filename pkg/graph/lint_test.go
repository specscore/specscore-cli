package graph

import (
	"strings"
	"testing"
)

func TestLint_ValidFixtureClean(t *testing.T) {
	res := lintRepo(t, "testdata/valid")
	if res.NoGraphRoot {
		t.Fatal("expected a graph root")
	}
	if len(res.Violations) != 0 {
		t.Fatalf("expected clean, got: %+v", res.Violations)
	}
}

func TestLint_MultirootClean(t *testing.T) {
	res := lintRepo(t, "testdata/multiroot")
	if len(res.Violations) != 0 {
		t.Fatalf("expected clean multiroot, got: %+v", res.Violations)
	}
}

func TestLint_ModuleOnlyRootClean(t *testing.T) {
	res := lintRepo(t, "testdata/moduleonly")
	if len(res.Violations) != 0 {
		t.Fatalf("expected clean module-only root, got: %+v", res.Violations)
	}
}

func TestLint_ProjectsResolutionClean(t *testing.T) {
	res := lintRepo(t, "testdata/proja")
	if len(res.Violations) != 0 {
		t.Fatalf("expected clean cross-project resolution, got: %+v", res.Violations)
	}
}

func TestLint_DuplicateModuleID(t *testing.T) {
	res := lintRepo(t, "testdata/multiroot_dupmodule")
	if !hasRule(res.Violations, "graph-duplicate-module-id") {
		t.Fatalf("expected graph-duplicate-module-id, got: %+v", res.Violations)
	}
}

func TestLint_NoGraphRoot(t *testing.T) {
	root := repoWith(t, map[string]string{}) // just specscore.yaml, no graph
	res := lintRepo(t, root)
	if !res.NoGraphRoot {
		t.Fatalf("expected NoGraphRoot, got %+v", res)
	}
}

func TestLint_NegativeCases(t *testing.T) {
	cases := []struct {
		name  string
		rule  string
		files map[string]string
	}{
		{
			name: "id-equals-filename-stem",
			rule: "graph-id-equals-filename-stem",
			files: map[string]string{
				"spec/graph/modules/m/README.md":           fmModule("m", "[]"),
				"spec/graph/modules/m/entities/booking.md": fmArt("entity", "bookingx"),
			},
		},
		{
			name: "id-kebab-case",
			rule: "graph-id-kebab-case",
			files: map[string]string{
				"spec/graph/modules/m/README.md":           fmModule("m", "[]"),
				"spec/graph/modules/m/entities/Booking.md": fmArt("entity", "Booking"),
			},
		},
		{
			name: "no-module-prefix-in-id",
			rule: "graph-no-module-prefix-in-id",
			files: map[string]string{
				"spec/graph/modules/m/README.md":          fmModule("m", "[]"),
				"spec/graph/modules/m/entities/m.book.md": fmArt("entity", "m.book"),
			},
		},
		{
			name: "kind-valid-mismatch",
			rule: "graph-kind-valid",
			files: map[string]string{
				"spec/graph/modules/m/README.md":     fmModule("m", "[]"),
				"spec/graph/modules/m/entities/x.md": fmArt("command", "x"),
			},
		},
		{
			name: "kind-invalid-token",
			rule: "graph-kind-valid",
			files: map[string]string{
				"spec/graph/modules/m/README.md":     fmModule("m", "[]"),
				"spec/graph/modules/m/entities/x.md": fmArt("bogus", "x"),
			},
		},
		{
			name: "no-owner-field",
			rule: "graph-no-owner-field",
			files: map[string]string{
				"spec/graph/modules/m/README.md":     fmModule("m", "[]"),
				"spec/graph/modules/m/entities/x.md": fmArt("entity", "x", "owner: alice"),
			},
		},
		{
			name: "no-inline-structure",
			rule: "graph-no-inline-structure",
			files: map[string]string{
				"spec/graph/modules/m/README.md":     fmModule("m", "[]"),
				"spec/graph/modules/m/entities/x.md": fmArt("entity", "x", "fields: [a]"),
			},
		},
		{
			name: "reference-resolves",
			rule: "graph-reference-resolves",
			files: map[string]string{
				"spec/graph/modules/m/README.md":     fmModule("m", "[n]"),
				"spec/graph/modules/n/README.md":     fmModule("n", "[]"),
				"spec/graph/modules/m/commands/c.md": fmArt("command", "c", "subject: n.ghost"),
			},
		},
		{
			name: "model-ref-resolves-unknown-concept",
			rule: "graph-model-ref-resolves",
			files: map[string]string{
				"spec/graph/modules/m/README.md":     fmModule("m", "[]"),
				"spec/graph/modules/m/entities/e.md": fmArt("entity", "e", "model: modelspec:///m.Ghost"),
				"spec/graph/modules/m/models/m.hcl":  "entity \"Real\" {\n  key = [\"id\"]\n}\n",
			},
		},
		{
			name: "dependency-direction",
			rule: "graph-dependency-direction",
			files: map[string]string{
				"spec/graph/modules/m/README.md":         fmModule("m", "[]"),
				"spec/graph/modules/n/README.md":         fmModule("n", "[]"),
				"spec/graph/modules/n/entities/thing.md": fmArt("entity", "thing"),
				"spec/graph/modules/m/commands/c.md":     fmArt("command", "c", "subject: n.thing"),
			},
		},
		{
			name: "relationship-owner-covers-endpoints",
			rule: "graph-relationship-owner-covers-endpoints",
			files: map[string]string{
				"spec/graph/modules/o/README.md":          fmModule("o", "[]"),
				"spec/graph/modules/a/README.md":          fmModule("a", "[]"),
				"spec/graph/modules/b/README.md":          fmModule("b", "[]"),
				"spec/graph/modules/a/entities/x.md":      fmArt("entity", "x"),
				"spec/graph/modules/b/entities/y.md":      fmArt("entity", "y"),
				"spec/graph/modules/o/relationships/r.md": fmArt("relationship", "r", "from: a.x", "to: b.y"),
			},
		},
		{
			name: "metadata-shape",
			rule: "graph-metadata-shape",
			files: map[string]string{
				"spec/graph/modules/m/README.md":          fmModule("m", "[]"),
				"spec/graph/modules/m/entities/a.md":      fmArt("entity", "a"),
				"spec/graph/modules/m/entities/b.md":      fmArt("entity", "b"),
				"spec/graph/modules/m/relationships/r.md": fmArt("relationship", "r", "from: m.a", "to: m.b", "metadata:", "  role:", "    nested: x"),
			},
		},
		{
			name: "inputs-shape-both",
			rule: "graph-inputs-shape",
			files: map[string]string{
				"spec/graph/modules/m/README.md":     fmModule("m", "[]"),
				"spec/graph/modules/m/entities/a.md": fmArt("entity", "a"),
				"spec/graph/modules/m/commands/c.md": fmArt("command", "c", "inputs:", "  - name: p", "    ref: m.a", "    model: modelspec:///m.A"),
			},
		},
		{
			name: "event-sources-command",
			rule: "graph-event-sources",
			files: map[string]string{
				"spec/graph/modules/m/README.md":     fmModule("m", "[]"),
				"spec/graph/modules/m/commands/c.md": fmArt("command", "c"),
				"spec/graph/modules/m/events/e.md":   fmArt("event", "e", "sources:", "  - m.c"),
			},
		},
		{
			name: "lifecycle-empty",
			rule: "graph-lifecycle-states",
			files: map[string]string{
				"spec/graph/modules/m/README.md":     fmModule("m", "[]"),
				"spec/graph/modules/m/entities/e.md": fmArt("entity", "e", "lifecycle:", "  states: []"),
			},
		},
		{
			name: "lifecycle-duplicate",
			rule: "graph-lifecycle-states",
			files: map[string]string{
				"spec/graph/modules/m/README.md":     fmModule("m", "[]"),
				"spec/graph/modules/m/entities/e.md": fmArt("entity", "e", "lifecycle:", "  states: [a, a]"),
			},
		},
		{
			name: "duplicate-id",
			rule: "graph-duplicate-id",
			files: map[string]string{
				"spec/graph/modules/m/README.md":           fmModule("m", "[]"),
				"spec/graph/modules/m/entities/booking.md": fmArt("entity", "booking"),
				"spec/graph/modules/m/commands/booking.md": fmArt("command", "booking"),
			},
		},
		{
			name: "model-duplicate-concept",
			rule: "graph-model-duplicate-concept",
			files: map[string]string{
				"spec/graph/modules/m/README.md":    fmModule("m", "[]"),
				"spec/graph/modules/m/models/m.hcl": "entity \"A\" {\n  key = [\"id\"]\n}\n\nentity \"A\" {\n  key = [\"id\"]\n}\n",
			},
		},
		{
			name: "model-enum-values-empty",
			rule: "graph-model-enum-values",
			files: map[string]string{
				"spec/graph/modules/m/README.md":    fmModule("m", "[]"),
				"spec/graph/modules/m/models/m.hcl": "enum \"E\" {\n}\n",
			},
		},
		{
			name: "model-enum-values-duplicate",
			rule: "graph-model-enum-values",
			files: map[string]string{
				"spec/graph/modules/m/README.md":    fmModule("m", "[]"),
				"spec/graph/modules/m/models/m.hcl": "enum \"E\" {\n  values = [\"a\", \"a\"]\n}\n",
			},
		},
		{
			name: "model-bare-ref-unresolved",
			rule: "graph-model-ref-resolves",
			files: map[string]string{
				"spec/graph/modules/m/README.md":    fmModule("m", "[]"),
				"spec/graph/modules/m/models/m.hcl": "entity \"A\" {\n  property \"p\" {\n    entity = \"Missing\"\n  }\n}\n",
			},
		},
		{
			name: "model-qualified-unknown-module",
			rule: "graph-model-ref-resolves",
			files: map[string]string{
				"spec/graph/modules/m/README.md":    fmModule("m", "[zzz]"),
				"spec/graph/modules/m/models/m.hcl": "entity \"A\" {\n  property \"p\" {\n    entity = \"zzz.Thing\"\n  }\n}\n",
			},
		},
		{
			name: "hcl-parse-error",
			rule: "graph-model-ref-resolves",
			files: map[string]string{
				"spec/graph/modules/m/README.md":    fmModule("m", "[]"),
				"spec/graph/modules/m/models/m.hcl": "entity \"A\" {\n  this is not valid hcl\n",
			},
		},
		{
			name: "frontmatter-missing",
			rule: "graph-kind-valid",
			files: map[string]string{
				"spec/graph/modules/m/README.md":     fmModule("m", "[]"),
				"spec/graph/modules/m/entities/x.md": "no frontmatter at all\n",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := repoWith(t, tc.files)
			res := lintRepo(t, root)
			if !hasRule(res.Violations, tc.rule) {
				t.Fatalf("expected rule %q; got %+v", tc.rule, res.Violations)
			}
		})
	}
}

func TestLint_EventSourcesEmptyIsWarning(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md":   fmModule("m", "[]"),
		"spec/graph/modules/m/events/e.md": fmArt("event", "e", "sources: []"),
	})
	res := lintRepo(t, root)
	found := false
	for _, v := range res.Violations {
		if v.Rule == "graph-event-sources" {
			found = true
			if v.Severity != "warning" {
				t.Fatalf("expected warning severity, got %q", v.Severity)
			}
		}
	}
	if !found {
		t.Fatalf("expected graph-event-sources warning, got %+v", res.Violations)
	}
}

func TestLint_RulesFilter(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md":     fmModule("m", "[]"),
		"spec/graph/modules/m/entities/x.md": fmArt("entity", "x", "owner: a", "fields: [q]"),
	})
	// Only the owner rule enabled.
	res := lintRepo(t, root, func(o *LintOptions) { o.Rules = []string{"graph-no-owner-field"} })
	c := ruleCounts(res.Violations)
	if c["graph-no-owner-field"] != 1 || c["graph-no-inline-structure"] != 0 {
		t.Fatalf("--rules filter wrong: %+v", res.Violations)
	}
	// Ignore the owner rule.
	res = lintRepo(t, root, func(o *LintOptions) { o.Ignore = []string{"graph-no-owner-field"} })
	c = ruleCounts(res.Violations)
	if c["graph-no-owner-field"] != 0 || c["graph-no-inline-structure"] == 0 {
		t.Fatalf("--ignore filter wrong: %+v", res.Violations)
	}
}

func TestLint_SeverityFilter(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md":   fmModule("m", "[]"),
		"spec/graph/modules/m/events/e.md": fmArt("event", "e", "sources: []"),
	})
	// error-only filter drops the warning.
	res := lintRepo(t, root, func(o *LintOptions) { o.Severity = "error" })
	if len(res.Violations) != 0 {
		t.Fatalf("expected warning filtered out at error severity, got %+v", res.Violations)
	}
	res = lintRepo(t, root, func(o *LintOptions) { o.Severity = "warning" })
	if !hasRule(res.Violations, "graph-event-sources") {
		t.Fatalf("expected warning at warning severity")
	}
}

func TestLint_LoadError(t *testing.T) {
	// A graph root whose modules/ path is a regular file makes ReadDir fail
	// with a non-IsNotExist error.
	root := repoWith(t, map[string]string{
		"spec/graph/modules": "not a directory",
	})
	_, err := Lint(LintOptions{RepoRoot: root})
	if err == nil {
		t.Fatal("expected Lint to surface the discovery error")
	}
}

func TestValidateRuleNames(t *testing.T) {
	if err := ValidateRuleNames([]string{"graph-id-kebab-case"}); err != nil {
		t.Fatalf("valid rule rejected: %v", err)
	}
	if err := ValidateRuleNames([]string{"nope"}); err == nil {
		t.Fatal("expected unknown rule error")
	}
}

func TestGraphRuleNames_Sorted(t *testing.T) {
	names := GraphRuleNames()
	if len(names) == 0 {
		t.Fatal("expected graph rules")
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Fatalf("not sorted: %q before %q", names[i-1], names[i])
		}
	}
	// spot-check a couple of expected ids.
	joined := strings.Join(names, ",")
	for _, want := range []string{"graph-kind-valid", "graph-duplicate-module-id"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing rule %q in %s", want, joined)
		}
	}
}
