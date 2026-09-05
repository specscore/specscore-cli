package sourceref

import "testing"

func TestDirectiveErrorAndNilBranches(t *testing.T) {
	for _, raw := range []string{
		"// specscore:implements",
		"// specscore:implements %%%",
		"// specscore:",
		"plain source text",
	} {
		if d, err := ParseDirective(raw); err == nil && d != nil {
			t.Fatalf("ParseDirective(%q) = %+v, want an error or nil", raw, d)
		}
	}
	if got := ScanDirective("// specscore:implements"); got != nil {
		t.Fatalf("malformed ScanDirective = %+v", got)
	}
	if got := (Directive{}).Canonical(); got != "" {
		t.Fatalf("nil target canonical = %q", got)
	}
	if err := ValidateRelationTarget(RelationImplements, nil); err == nil {
		t.Fatal("nil target should be rejected")
	}
	if err := ValidateDirective(nil); err == nil {
		t.Fatal("nil directive should be rejected")
	}
}

func TestDirectiveValidationConvenienceAndCanonicalFallbacks(t *testing.T) {
	if err := ValidateRelationTargetString(RelationImplements, "feature/checkout#req:totals"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRelationTargetString(Relation("unknown"), "feature/checkout#req:totals"); err == nil {
		t.Fatal("unknown relation should be rejected")
	}
	if err := ValidateRelationTargetString(RelationReferences, "%%% "); err == nil {
		t.Fatal("malformed target should be rejected")
	}
	if got := (Directive{Target: &Reference{ResolvedPath: "feature/x"}}).Canonical(); got != "specscore:references specscore:feature/x" {
		t.Fatalf("omitted relation canonical = %q", got)
	}
	if got := canonicalReference(nil); got != "" {
		t.Fatalf("nil canonical reference = %q", got)
	}
	if !hasFragmentPrefix("REQ:one", "req") || hasFragmentPrefix("req", "req") {
		t.Fatal("fragment prefix checks did not cover valid and short forms")
	}
	if got := (Reference{ResolvedPath: "feature/x", Fragment: "other:value"}).CanonicalTyped(); got != "specscore:feature/x#other:value" {
		t.Fatalf("opaque typed fragment canonical = %q", got)
	}
	if got := (Reference{ResolvedPath: "feature/x", Fragment: "REQ:one"}).CanonicalTyped(); got != "specscore:feature/x#req:one" {
		t.Fatalf("REQ typed fragment canonical = %q", got)
	}
}
