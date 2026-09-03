package rule

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Scope kinds. A Rule binds everywhere its Scope list says it binds, and
// nowhere else — the property that lets `rule list --applies-to <path>` answer
// "which rules apply to what I am about to touch" without a human reading them
// all.
const (
	ScopeFleet   = "fleet"   // every repository, every product.
	ScopeProduct = "product" // one product by name, e.g. product:sneat.
	ScopeRepo    = "repo"    // one repository, e.g. repo:specscore/specscore-cli.
	ScopePath    = "path"    // a glob over paths, e.g. path:**/*.go.
)

// Scope is one parsed entry of a Rule's **Scope:** list.
type Scope struct {
	Kind  string // one of the Scope* constants
	Value string // "" for fleet; the product name, owner/repo, or glob otherwise
	Raw   string // the value exactly as written in the artifact
}

// String renders a Scope back to its canonical wire form.
func (s Scope) String() string {
	if s.Kind == ScopeFleet {
		return ScopeFleet
	}
	return s.Kind + ":" + s.Value
}

var (
	productRe = regexp.MustCompile(`^[a-z0-9]+([-.][a-z0-9]+)*$`)
	repoRe    = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
)

// ParseScope parses one raw scope token. It never guesses: an unprefixed token
// other than the bare `fleet` keyword is an error rather than a silently
// accepted product name, because a mis-scoped Rule is worse than a rejected
// one — it binds work it was never meant to bind.
func ParseScope(raw string) (Scope, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Scope{}, fmt.Errorf("scope must not be empty")
	}
	if trimmed == ScopeFleet {
		return Scope{Kind: ScopeFleet, Raw: trimmed}, nil
	}
	kind, value, found := strings.Cut(trimmed, ":")
	if !found {
		return Scope{}, fmt.Errorf(
			"scope %q must be %s, %s:<name>, %s:<owner/repo>, or %s:<glob>",
			trimmed, ScopeFleet, ScopeProduct, ScopeRepo, ScopePath)
	}
	kind = strings.TrimSpace(kind)
	value = strings.TrimSpace(value)
	if value == "" {
		return Scope{}, fmt.Errorf("scope %q is missing a value after %q", trimmed, kind+":")
	}
	switch kind {
	case ScopeProduct:
		if !productRe.MatchString(value) {
			return Scope{}, fmt.Errorf("scope %q: product name must be lowercase and hyphen/dot separated", trimmed)
		}
	case ScopeRepo:
		if !repoRe.MatchString(value) {
			return Scope{}, fmt.Errorf("scope %q: repo must be <owner>/<repository>", trimmed)
		}
	case ScopePath:
		if !doublestar.ValidatePattern(value) {
			return Scope{}, fmt.Errorf("scope %q: path glob is not a valid pattern", trimmed)
		}
	case ScopeFleet:
		return Scope{}, fmt.Errorf("scope %q: %s takes no value", trimmed, ScopeFleet)
	default:
		return Scope{}, fmt.Errorf(
			"scope %q has unknown kind %q (expected %s, %s, %s, or bare %s)",
			trimmed, kind, ScopeProduct, ScopeRepo, ScopePath, ScopeFleet)
	}
	return Scope{Kind: kind, Value: value, Raw: trimmed}, nil
}

// ParseScopes parses every entry of a scope list, reporting the first error.
func ParseScopes(raws []string) ([]Scope, error) {
	out := make([]Scope, 0, len(raws))
	for _, raw := range raws {
		s, err := ParseScope(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// Matches reports whether this scope covers the given path.
//
// `fleet` covers everything. `path:<glob>` is matched with doublestar against
// the slash-normalized path, and additionally against every suffix of that
// path, so a repo-relative pattern such as `path:pkg/**` still matches an
// absolute or project-prefixed path the caller happens to hold. `product:` and
// `repo:` match when their identifier appears as a whole path segment — the
// only path-shaped evidence a scope of that kind can offer. Callers that know
// the product or repository for certain should filter on `--scope` instead of
// inferring it from a path.
func (s Scope) Matches(p string) bool {
	normalized := strings.Trim(path.Clean(filepath.ToSlash(p)), "/")
	switch s.Kind {
	case ScopeFleet:
		return true
	case ScopePath:
		return matchGlobAnySuffix(s.Value, normalized)
	case ScopeProduct:
		return hasSegment(normalized, s.Value)
	case ScopeRepo:
		owner, repo, _ := strings.Cut(s.Value, "/")
		return hasSegments(normalized, owner, repo) || hasSegment(normalized, repo)
	}
	return false
}

// matchGlobAnySuffix reports whether pattern matches path or any of its
// trailing path suffixes. A Rule author writes `path:internal/cli/**` meaning
// "these files"; a caller may pass `/home/me/projects/org/repo/internal/cli/x.go`.
// Anchoring only at the front would make the common case silently miss.
func matchGlobAnySuffix(pattern, p string) bool {
	if ok, err := doublestar.Match(pattern, p); err == nil && ok {
		return true
	}
	segments := strings.Split(p, "/")
	for i := 1; i < len(segments); i++ {
		suffix := strings.Join(segments[i:], "/")
		if ok, err := doublestar.Match(pattern, suffix); err == nil && ok {
			return true
		}
	}
	return false
}

func hasSegment(p, segment string) bool {
	for _, s := range strings.Split(p, "/") {
		if s == segment {
			return true
		}
	}
	return false
}

// hasSegments reports whether a and b appear as consecutive path segments.
func hasSegments(p, a, b string) bool {
	segments := strings.Split(p, "/")
	for i := 0; i+1 < len(segments); i++ {
		if segments[i] == a && segments[i+1] == b {
			return true
		}
	}
	return false
}

// ScopesMatch reports whether any scope in the list covers p.
func ScopesMatch(scopes []Scope, p string) bool {
	for _, s := range scopes {
		if s.Matches(p) {
			return true
		}
	}
	return false
}
