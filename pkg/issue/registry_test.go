package issue

import (
	"reflect"
	"testing"
)

// TestPathPatterns pins the two canonical path patterns for `issue`
// artifacts. Verifies AC: type-registered (the path-pattern half).
func TestPathPatterns(t *testing.T) {
	want := []string{
		"issues/*.md",
		"features/*/issues/*.md",
	}
	if !reflect.DeepEqual(PathPatterns, want) {
		t.Fatalf("PathPatterns mismatch:\n got  %q\n want %q", PathPatterns, want)
	}
}

// TestRuleFamilyPrefix pins the canonical rule-ID prefix. Verifies
// cli/spec/lint/issue-rules#ac:type-registered (related; not verified here) (the rule-family half).
func TestRuleFamilyPrefix(t *testing.T) {
	if RuleFamilyPrefix != "I-" {
		t.Fatalf("RuleFamilyPrefix = %q; want %q", RuleFamilyPrefix, "I-")
	}
}

// TestTypeValue pins the literal frontmatter type value.
func TestTypeValue(t *testing.T) {
	if TypeValue != "issue" {
		t.Fatalf("TypeValue = %q; want %q", TypeValue, "issue")
	}
}
