package sourceref

import "testing"

func TestParseDirective(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		relation Relation
		fragment string
	}{
		{"implements short", "// specscore:implements feature/checkout#REQ:totals", RelationImplements, "REQ:totals"},
		{"verifies canonical URL", "// specscore:verifies https://specscore.org/github.com/acme/orders/spec/features/checkout#ac:discounted-total", RelationVerifies, "ac:discounted-total"},
		{"references omitted", "// specscore:feature/checkout", RelationReferences, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := ParseDirective(tt.line)
			if err != nil {
				t.Fatal(err)
			}
			if d.Relation != tt.relation || d.Target.Fragment != tt.fragment {
				t.Fatalf("directive = %+v, want relation %q fragment %q", d, tt.relation, tt.fragment)
			}
			if err := ValidateDirective(d); err != nil {
				t.Fatalf("ValidateDirective() = %v", err)
			}
		})
	}
}

func TestDirectiveCanonicalAndValidation(t *testing.T) {
	d, err := ParseDirective("// specscore:verifies feature/checkout#AC:discounted-total")
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Canonical(); got != "specscore:verifies specscore:spec/features/checkout#ac:discounted-total" {
		t.Fatalf("Canonical() = %q", got)
	}
	if got := d.Target.CanonicalTyped(); got != "specscore:spec/features/checkout#ac:discounted-total" {
		t.Fatalf("CanonicalTyped() = %q", got)
	}
	if got := d.Target.Canonical(); got != "specscore:spec/features/checkout#ac:discounted-total" {
		t.Fatalf("typed target Canonical() = %q", got)
	}
	if err := ValidateRelationTarget(RelationImplements, d.Target); err == nil {
		t.Fatal("implements AC should be rejected")
	}
	if err := ValidateRelationTarget(RelationVerifies, &Reference{Type: "plan", Fragment: "ac:x"}); err == nil {
		t.Fatal("verifies non-feature should be rejected")
	}
	if err := ValidateRelationTarget(Relation("unknown"), d.Target); err == nil {
		t.Fatal("unknown relation should be rejected")
	}
}

func TestScanDirectiveRequiresCommentPrefix(t *testing.T) {
	if got := ScanDirective(`var s = "specscore:implements feature/x#req:y"`); got != nil {
		t.Fatalf("string literal unexpectedly scanned: %+v", got)
	}
	if got := ScanDirective("// specscore:implements feature/x#req:y"); got == nil {
		t.Fatal("comment directive not scanned")
	}
}

func TestParseDirectiveRejectsUnknownRelation(t *testing.T) {
	if _, err := ParseDirective("// specscore:depends feature/x#req:y"); err == nil {
		t.Fatal("unknown relation should be rejected")
	}
}
