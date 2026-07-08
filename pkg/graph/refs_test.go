package graph

import "testing"

func TestIsKebab(t *testing.T) {
	for _, s := range []string{"a", "team-member", "a1", "create-booking"} {
		if !IsKebab(s) {
			t.Errorf("%q should be kebab", s)
		}
	}
	for _, s := range []string{"", "A", "a.b", "a--b", "-a", "a-", "aB"} {
		if IsKebab(s) {
			t.Errorf("%q should not be kebab", s)
		}
	}
}

func TestParseQualifiedRef(t *testing.T) {
	ok := []struct{ in, mod, loc string }{
		{"identity.team", "identity", "team"},
		{"a.b", "a", "b"},
	}
	for _, c := range ok {
		q, good := ParseQualifiedRef(c.in)
		if !good || q.Module != c.mod || q.Local != c.loc {
			t.Errorf("ParseQualifiedRef(%q) = %+v %v", c.in, q, good)
		}
	}
	for _, bad := range []string{"", "nodot", "a.", ".b", "a.b.c", "modelspec://x.Y"} {
		if _, good := ParseQualifiedRef(bad); good {
			t.Errorf("ParseQualifiedRef(%q) should fail", bad)
		}
	}
}

func TestParseModelspecRef(t *testing.T) {
	// Local two-segment (trio) form.
	q, err := ParseModelspecRef("modelspec:///identity.TeamRole")
	if err != nil || q.Module != "identity" || q.Kind != "" || q.Name != "TeamRole" || q.Repo != "" {
		t.Fatalf("local two-seg: %+v %v", q, err)
	}
	// Local three-segment kind-explicit form (each kind token).
	for tok, kind := range ModelspecKindTokens {
		q, err := ParseModelspecRef("modelspec:///vault." + tok + ".Name")
		if err != nil || q.Module != "vault" || q.Kind != kind || q.Name != "Name" {
			t.Fatalf("kind %s: %+v %v", tok, q, err)
		}
	}
	// Cross-repo authority form; the last path segment is the concept.
	q, err = ParseModelspecRef("modelspec://example.com/acme/repo/shared.Thing")
	if err != nil || q.Module != "shared" || q.Name != "Thing" || q.Repo != "example.com/acme/repo" {
		t.Fatalf("cross-repo: %+v %v", q, err)
	}
	// Cross-repo with a nested subgroup path and an explicit kind.
	q, err = ParseModelspecRef("modelspec://gitlab.example/acme/sub/repo/shared.enums.Color")
	if err != nil || q.Repo != "gitlab.example/acme/sub/repo" || q.Kind != "enum" || q.Name != "Color" {
		t.Fatalf("nested cross-repo: %+v %v", q, err)
	}
	// Advisory ?ref= pin, recorded but not resolution-affecting.
	q, err = ParseModelspecRef("modelspec:///identity.Team?ref=main")
	if err != nil || q.Ref != "main" || q.Name != "Team" {
		t.Fatalf("ref pin: %+v %v", q, err)
	}
	q, err = ParseModelspecRef("modelspec://example.com/acme/repo/shared.Thing?ref=v1.2.3")
	if err != nil || q.Ref != "v1.2.3" || q.Repo != "example.com/acme/repo" {
		t.Fatalf("cross-repo ref pin: %+v %v", q, err)
	}
	// Enum-value fragments (decision 0013) parse on both local forms and after
	// a ?ref= pin.
	q, err = ParseModelspecRef("modelspec:///catalog.Color#green")
	if err != nil || q.Fragment != "green" || q.Name != "Color" {
		t.Fatalf("fragment: %+v %v", q, err)
	}
	q, err = ParseModelspecRef("modelspec:///catalog.enums.Color#green")
	if err != nil || q.Fragment != "green" || q.Kind != "enum" {
		t.Fatalf("kind-explicit fragment: %+v %v", q, err)
	}
	q, err = ParseModelspecRef("modelspec:///catalog.Color?ref=main#green")
	if err != nil || q.Fragment != "green" || q.Ref != "main" {
		t.Fatalf("fragment after pin: %+v %v", q, err)
	}
}

func TestParseModelspecRef_Legacy(t *testing.T) {
	_, err := ParseModelspecRef("modelspec://identity.Team")
	pe, ok := isLegacyForm(err)
	if !ok || pe.Rewrite != "modelspec:///identity.Team" {
		t.Fatalf("legacy: %v %+v", ok, pe)
	}
	// Legacy form preserves a ?ref= pin in its rewrite.
	_, err = ParseModelspecRef("modelspec://identity.Team?ref=main")
	pe, ok = isLegacyForm(err)
	if !ok || pe.Rewrite != "modelspec:///identity.Team?ref=main" {
		t.Fatalf("legacy+ref: %v %+v", ok, pe)
	}
	// ... and an enum-value fragment (decision 0013).
	_, err = ParseModelspecRef("modelspec://identity.Team#lead")
	pe, ok = isLegacyForm(err)
	if !ok || pe.Rewrite != "modelspec:///identity.Team#lead" {
		t.Fatalf("legacy+fragment: %v %+v", ok, pe)
	}
	// A non-legacy error is not reported as legacy.
	if _, ok := isLegacyForm(ParseErrOf(t, "modelspec:///x")); ok {
		t.Fatal("malformed should not be legacy")
	}
	if _, ok := isLegacyForm(nil); ok {
		t.Fatal("nil is not legacy")
	}
}

// ParseErrOf returns the error from parsing ref, failing if there is none.
func ParseErrOf(t *testing.T, ref string) error {
	t.Helper()
	_, err := ParseModelspecRef(ref)
	if err == nil {
		t.Fatalf("ParseModelspecRef(%q) unexpectedly succeeded", ref)
	}
	return err
}

func TestParseModelspecRef_Errors(t *testing.T) {
	cases := map[string]modelspecErrKind{
		"identity.Team":                     msErrNotScheme, // no scheme
		"modelspec:///x":                    msErrMalformed, // one segment
		"modelspec:///.Y":                   msErrMalformed, // empty module
		"modelspec:///x.":                   msErrMalformed, // empty name
		"modelspec:///x.Y.Z.W":              msErrMalformed, // four segments
		"modelspec:///x/y.Z":                msErrMalformed, // slash in local path
		"modelspec:///vault.widgets.W":      msErrUnknownKind,
		"modelspec:///vault.Name#":          msErrFragment,  // empty fragment
		"modelspec:///vault.Name#a#b":       msErrFragment,  // '#' appearing twice
		"modelspec:///vault.Name?branch=x":  msErrMalformed, // unsupported query
		"modelspec:///vault.Name?ref=":      msErrMalformed, // empty pin
		"modelspec:///vault.Name?ref=a&b":   msErrMalformed, // multi-param query
		"modelspec://example.com/only.Y":    msErrMalformed, // no org/repo before concept
		"modelspec://example.com//repo/x.Y": msErrMalformed, // empty repo-path segment
	}
	for ref, want := range cases {
		_, err := ParseModelspecRef(ref)
		pe, ok := err.(*ModelspecParseError)
		if !ok || pe.kind != want {
			t.Errorf("ParseModelspecRef(%q): got %v, want kind %v", ref, err, want)
		}
	}
}
