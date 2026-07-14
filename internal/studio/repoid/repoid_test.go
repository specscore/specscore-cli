package repoid

import (
	"errors"
	"path/filepath"
	"strings"
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
		{"http with port", "HTTP://Code.Example.com:8443/acme/widget.git", "code.example.com:8443/acme/widget", true},
		{"scp ssh", "git@github.com:dal-go/backstage.git", "github.com/dal-go/backstage", true},
		{"scp without user", "Code.Example.com:Acme/Widget.git", "code.example.com/Acme/Widget", true},
		{"ssh url", "ssh://git@gitlab.com/acme/subgroup/widget.git", "gitlab.com/acme/subgroup/widget", true},
		{"git transport", "git://code.example.com/acme/widget/", "code.example.com/acme/widget", true},
		{"empty", "  ", "", false},
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

func TestNewResolverUsesGitOriginLookup(t *testing.T) {
	r := NewResolver()
	if r.originURL == nil || r.byPath == nil || r.byID == nil {
		t.Fatalf("NewResolver() returned an incompletely initialized resolver: %#v", r)
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

func TestResolver_CachesIDByCleanAbsolutePath(t *testing.T) {
	calls := 0
	r := NewResolverWithOriginURL(func(string) (string, error) {
		calls++
		return "https://github.com/acme/widget.git", nil
	})
	repo := filepath.Join(t.TempDir(), "repo")
	first, err := r.ID(repo)
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.ID(filepath.Join(repo, "."))
	if err != nil {
		t.Fatal(err)
	}
	if first != second || calls != 1 {
		t.Fatalf("cached IDs = (%q, %q), origin lookup calls = %d; want equal IDs and one lookup", first, second, calls)
	}
}

func TestAbsolutePathErrors(t *testing.T) {
	originalAbsolutePath := absolutePath
	absolutePath = func(string) (string, error) {
		return "", errors.New("working directory unavailable")
	}
	defer func() { absolutePath = originalAbsolutePath }()

	originCalled := false
	r := NewResolverWithOriginURL(func(string) (string, error) {
		originCalled = true
		return "", nil
	})
	if id, err := r.ID("repo"); err == nil || originCalled || id != "" || !strings.Contains(err.Error(), "resolving absolute repository path repo") {
		t.Fatalf("ID(repo) = (%q, %v), want a contextual absolute-path error", id, err)
	}
	if id := LocalID("repo"); !strings.HasPrefix(id, "local/repo-") {
		t.Fatalf("LocalID(repo) = %q, want deterministic relative-path fallback", id)
	}
}

func TestSafeBasename(t *testing.T) {
	tests := []struct {
		base string
		want string
	}{
		{" Feature ✨ ", "feature--"},
		{"", "repo"},
		{".", "repo"},
		{"..", "repo"},
	}
	for _, tt := range tests {
		if got := safeBasename(tt.base); got != tt.want {
			t.Errorf("safeBasename(%q) = %q, want %q", tt.base, got, tt.want)
		}
	}
}
