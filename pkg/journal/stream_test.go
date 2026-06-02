package journal

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestResolveStream_OriginOrgLowercased(t *testing.T) {
	orig := originURLFn
	originURLFn = func(string) (string, error) { return "git@github.com:SpecScore/specscore.git", nil }
	defer func() { originURLFn = orig }()

	if got := ResolveStream("/x/whatever"); got != "specscore" {
		t.Errorf("ResolveStream = %q, want specscore (origin org, lowercased)", got)
	}
}

func TestResolveStream_UnparseableOriginFallsToBasename(t *testing.T) {
	orig := originURLFn
	originURLFn = func(string) (string, error) { return "not-a-valid-url", nil }
	defer func() { originURLFn = orig }()

	if got := ResolveStream(filepath.FromSlash("/x/MyRepo")); got != "myrepo" {
		t.Errorf("ResolveStream = %q, want myrepo (basename fallback)", got)
	}
}

func TestResolveStream_NoOriginFallsToBasename(t *testing.T) {
	orig := originURLFn
	originURLFn = func(string) (string, error) { return "", errors.New("no remote") }
	defer func() { originURLFn = orig }()

	if got := ResolveStream(filepath.FromSlash("/x/Another-Repo")); got != "another-repo" {
		t.Errorf("ResolveStream = %q, want another-repo (basename fallback)", got)
	}
}
