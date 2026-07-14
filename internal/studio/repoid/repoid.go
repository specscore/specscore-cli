// Package repoid resolves stable Studio repository entity IDs.
//
// Feature: cli/studio/index (REQ: fact-shape)
package repoid

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/gitremote"
)

// OriginURLFunc resolves the origin remote URL for a local repository path.
// A missing origin, a non-repository directory, or any other git failure is
// represented by a non-nil error and makes the resolver use the local-only ID.
type OriginURLFunc func(dir string) (string, error)

var absolutePath = filepath.Abs

// Resolver mints stable repository IDs and detects two different paths that
// resolve to the same identity. It is intentionally stateful for one workspace
// run so an identity collision cannot silently merge two ingestion sources.
type Resolver struct {
	originURL OriginURLFunc
	byPath    map[string]string
	byID      map[string]string
}

// NewResolver returns a resolver backed by `git remote get-url origin`.
func NewResolver() *Resolver {
	return NewResolverWithOriginURL(gitremote.OriginURL)
}

// NewResolverWithOriginURL returns a resolver with an injectable origin lookup.
func NewResolverWithOriginURL(originURL OriginURLFunc) *Resolver {
	return &Resolver{
		originURL: originURL,
		byPath:    map[string]string{},
		byID:      map[string]string{},
	}
}

// ID returns a stable repository entity ID. A supported origin remote becomes
// its normalized forge coordinate (`host/org/name`). A repository without a
// supported origin uses a deterministic local-only ID derived from its
// absolute path. If another path already claimed the same ID, ID returns that
// ID alongside an error instead of assigning an input-order suffix.
func (r *Resolver) ID(repoPath string) (string, error) {
	clean, err := absolutePath(filepath.Clean(repoPath))
	if err != nil {
		return "", fmt.Errorf("resolving absolute repository path %s: %w", repoPath, err)
	}
	if id, ok := r.byPath[clean]; ok {
		return id, nil
	}

	id := LocalID(clean)
	if raw, err := r.originURL(clean); err == nil {
		if remoteID, ok := ParseRemote(raw); ok {
			id = remoteID
		}
	}
	if claimedBy, exists := r.byID[id]; exists && claimedBy != clean {
		return id, fmt.Errorf("repository identity collision: %s and %s both resolve to %s", claimedBy, clean, id)
	}
	r.byPath[clean] = id
	r.byID[id] = clean
	return id, nil
}

// LocalID returns an order-independent ID for a local-only repository. The
// readable basename is paired with a short hash of the normalized absolute
// path, so same-basename repositories remain distinct without `-2` aliases.
func LocalID(repoPath string) string {
	clean, err := absolutePath(filepath.Clean(repoPath))
	if err != nil {
		clean = filepath.Clean(repoPath)
	}
	normalized := filepath.ToSlash(clean)
	sum := sha256.Sum256([]byte(normalized))
	base := safeBasename(filepath.Base(clean))
	return fmt.Sprintf("local/%s-%x", base, sum[:6])
}

// ParseRemote normalizes a supported git remote URL to `host/path`. URL-style
// HTTPS, HTTP, SSH and git remotes and SCP-style SSH remotes are supported.
// Transport, credentials, a trailing slash, and a trailing `.git` do not form
// part of repository identity. Local/file remotes and unsafe paths are rejected.
func ParseRemote(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}

	var host, repoPath string
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || !supportedScheme(u.Scheme) || u.Hostname() == "" || u.RawQuery != "" || u.Fragment != "" {
			return "", false
		}
		host = strings.ToLower(u.Hostname())
		if port := u.Port(); port != "" {
			host += ":" + port
		}
		repoPath = strings.TrimPrefix(u.Path, "/")
	} else {
		// SCP-style SSH: git@github.com:org/repo.git.
		left, right, ok := strings.Cut(raw, ":")
		if !ok || strings.Contains(left, "/") || right == "" {
			return "", false
		}
		if _, h, ok := strings.Cut(left, "@"); ok {
			host = h
		} else {
			host = left
		}
		host = strings.ToLower(host)
		repoPath = right
	}

	repoPath = strings.TrimSuffix(strings.TrimSuffix(repoPath, "/"), ".git")
	parts := strings.Split(repoPath, "/")
	if host == "" || len(parts) < 2 {
		return "", false
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, `\\:`) {
			return "", false
		}
	}
	// GitHub repository coordinates are case-insensitive; normalize them so
	// equivalent remote spellings cannot mint distinct entity IDs.
	if host == "github.com" {
		for i := range parts {
			parts[i] = strings.ToLower(parts[i])
		}
	}
	return host + "/" + strings.Join(parts, "/"), true
}

func supportedScheme(scheme string) bool {
	switch strings.ToLower(scheme) {
	case "https", "http", "ssh", "git":
		return true
	default:
		return false
	}
}

func safeBasename(base string) string {
	base = strings.TrimSpace(strings.ToLower(base))
	var b strings.Builder
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 || b.String() == "." || b.String() == ".." {
		return "repo"
	}
	return b.String()
}
