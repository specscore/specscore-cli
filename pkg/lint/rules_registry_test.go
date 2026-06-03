package lint

import "testing"

// Every registry entry must expose a non-empty id, description, family, and
// severity (cli/rules#ac:registry-has-metadata).
func TestRegistry_EntriesHaveMetadata(t *testing.T) {
	rules := AllRules()
	if len(rules) == 0 {
		t.Fatal("expected a non-empty registry")
	}
	for _, r := range rules {
		if r.ID == "" {
			t.Errorf("rule with empty id: %+v", r)
		}
		if r.Description == "" {
			t.Errorf("rule %q has empty description", r.ID)
		}
		if r.Family == "" {
			t.Errorf("rule %q has empty family", r.ID)
		}
		if r.Severity == "" {
			t.Errorf("rule %q has empty severity", r.ID)
		}
	}
}

// AllRules must be returned in deterministic (id-sorted) order.
func TestRegistry_AllRulesSorted(t *testing.T) {
	rules := AllRules()
	for i := 1; i < len(rules); i++ {
		if rules[i-1].ID >= rules[i].ID {
			t.Fatalf("AllRules not sorted by id: %q before %q", rules[i-1].ID, rules[i].ID)
		}
	}
}

// No entry can be registered with an empty description (the construction-time
// guard must panic).
func TestRegistry_RejectsEmptyDescription(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected registerRule to panic on empty description")
		}
	}()
	registerRule(Rule{ID: "x-empty-desc", Description: "", Family: "core", Severity: "error"})
}

func TestRegistry_RejectsEmptyID(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected registerRule to panic on empty id")
		}
	}()
	registerRule(Rule{ID: "", Description: "d", Family: "core", Severity: "error"})
}

func TestRegistry_RejectsEmptyFamilyOrSeverity(t *testing.T) {
	t.Run("family", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic on empty family")
			}
		}()
		registerRule(Rule{ID: "x-empty-family", Description: "d", Family: "", Severity: "error"})
	})
	t.Run("severity", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic on empty severity")
			}
		}()
		registerRule(Rule{ID: "x-empty-sev", Description: "d", Family: "core", Severity: ""})
	})
}

// AllRuleNames must stay in parity with the structured registry keys.
func TestRegistry_AllRuleNamesParity(t *testing.T) {
	names := AllRuleNames()
	rules := AllRules()
	if len(names) != len(rules) {
		t.Fatalf("AllRuleNames count %d != AllRules count %d", len(names), len(rules))
	}
	for _, r := range rules {
		if !names[r.ID] {
			t.Errorf("rule %q in AllRules but missing from AllRuleNames", r.ID)
		}
	}
}

// Custom-checker registration adds a structured entry and reset removes it.
func TestRegistry_CustomCheckerEntry(t *testing.T) {
	defer ResetCustomCheckers()
	RegisterChecker(&mockCustomChecker{n: "reg-custom", s: "warning"})

	found := false
	for _, r := range AllRules() {
		if r.ID == "reg-custom" {
			found = true
			if r.Family != "custom" {
				t.Errorf("custom checker family = %q, want custom", r.Family)
			}
			if r.Severity != "warning" {
				t.Errorf("custom checker severity = %q, want warning", r.Severity)
			}
			if r.Description == "" {
				t.Error("custom checker description must be non-empty")
			}
		}
	}
	if !found {
		t.Fatal("expected reg-custom in registry after RegisterChecker")
	}

	ResetCustomCheckers()
	if AllRuleNames()["reg-custom"] {
		t.Error("expected reg-custom removed from registry after ResetCustomCheckers")
	}
}
