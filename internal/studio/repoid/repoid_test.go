package repoid

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestParseRemote(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{"https", "https://github.com/Sneat-Co/backstage.git", "github.com/sneat-co/backstage", true},
		{"scp ssh", "git@github.com:dal-go/backstage.git", "github.com/dal-go/backstage", true},
		{"ssh url", "ssh://git@gitlab.com/acme/subgroup/widget.git", "gitlab.com/acme/subgroup/widget", true},
		{"git transport", "git://code.example.com/acme/widget/", "code.example.com/acme/widget", true},
		{"local path", "../widget", "", false},
		{"file URL", "file:///tmp/widget", "", false},
		{"missing owner", "https://github.com/widget.git", "", false},
		{"unsafe traversal", "https://github.com/acme/../widget.git", "", false},
		{"query", "https://github.com/acme/widget.git?ref=x", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseRemote(tt.raw)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("ParseRemote(%q) = (%q, %v), want (%q, %v)", tt.raw, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestResolver_SameBasenameRemoteIDsAreOrderIndependent(t *testing.T) {
	root := t.TempDir()
	sneat := filepath.Join(root, "sneat-co", "backstage")
	dalgo := filepath.Join(root, "dal-go", "backstage")
	remotes := map[string]string{
		sneat: "https://github.com/sneat-co/backstage.git",
		dalgo: "git@github.com:dal-go/backstage.git",
	}
	origin := func(dir string) (string, error) { return remotes[dir], nil }

	for _, order := range [][]string{{sneat, dalgo}, {dalgo, sneat}} {
		r := NewResolverWithOriginURL(origin)
		got := map[string]string{}
		for _, repo := range order {
			id, err := r.ID(repo)
			if err != nil {
				t.Fatal(err)
			}
			got[repo] = id
		}
		if got[sneat] != "github.com/sneat-co/backstage" || got[dalgo] != "github.com/dal-go/backstage" {
			t.Errorf("IDs for order %v = %v", order, got)
		}
	}
}

func TestResolver_LocalOnlyFallbackIsPathStableAndOrderIndependent(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a", "repo")
	b := filepath.Join(root, "b", "repo")
	noOrigin := func(string) (string, error) { return "", errors.New("no origin") }

	first := NewResolverWithOriginURL(noOrigin)
	a1, _ := first.ID(a)
	b1, _ := first.ID(b)
	second := NewResolverWithOriginURL(noOrigin)
	b2, _ := second.ID(b)
	a2, _ := second.ID(a)
	if a1 != a2 || b1 != b2 || a1 == b1 {
		t.Fatalf("local IDs not stable/distinct: first=(%s,%s) second=(%s,%s)", a1, b1, a2, b2)
	}
	if a1 != LocalID(a) || b1 != LocalID(b) {
		t.Fatalf("resolver local IDs = (%s,%s), LocalID = (%s,%s)", a1, b1, LocalID(a), LocalID(b))
	}
}

func TestResolver_DuplicateRemoteIdentityIsError(t *testing.T) {
	r := NewResolverWithOriginURL(func(string) (string, error) {
		return "https://github.com/acme/widget.git", nil
	})
	if _, err := r.ID("/checkout/a"); err != nil {
		t.Fatal(err)
	}
	id, err := r.ID("/checkout/b")
	if err == nil || id != "github.com/acme/widget" {
		t.Fatalf("second ID = (%q, %v), want collision on github.com/acme/widget", id, err)
	}
}
