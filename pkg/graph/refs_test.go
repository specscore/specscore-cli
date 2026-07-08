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
	q, ok := ParseModelspecRef("modelspec://identity.TeamRole")
	if !ok || q.Module != "identity" || q.Name != "TeamRole" || q.Suffix != "" {
		t.Fatalf("plain: %+v %v", q, ok)
	}
	q, ok = ParseModelspecRef("modelspec://shared.Thing@example.com/acme/repo")
	if !ok || q.Module != "shared" || q.Name != "Thing" || q.Suffix != "example.com/acme/repo" {
		t.Fatalf("suffixed: %+v %v", q, ok)
	}
	for _, bad := range []string{"identity.Team", "modelspec://x", "modelspec://.Y", "modelspec://x.", "modelspec://x.Y.Z"} {
		if _, ok := ParseModelspecRef(bad); ok {
			t.Errorf("ParseModelspecRef(%q) should fail", bad)
		}
	}
}
