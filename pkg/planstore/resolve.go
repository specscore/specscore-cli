// Package planstore resolves the one authoritative Plan namespace for a
// SpecScore project. Routing is portable repository identity; local checkout
// paths are a separate, machine-owned concern.
package planstore

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/specscore/specscore-cli/pkg/gitremote"
	"gopkg.in/yaml.v3"
)

const (
	UserConfigFile  = ".specscore.yaml"
	OrgConfigFile   = ".specscore.yaml"
	RepoConfigFile  = "specscore.yaml"
	LocalConfigFile = "specscore.local.yaml"
)

type AccessMode int

const (
	ReadOnly AccessMode = iota
	Write
)

// Resolution names both halves of a Plan operation. SourceRoot continues to
// own Features and ACs; PlansDir is the only editable Plan namespace.
type Resolution struct {
	SourceRoot         string
	SourceRepo         string
	PlansRepo          string
	PlansCheckout      string
	PlansDir           string
	External           bool
	RouteConfigPath    string
	CheckoutConfigPath string
}

// OrgConfigPath returns the organization layer associated with a source
// checkout, anchored beside its canonical clone for linked worktrees.
func OrgConfigPath(sourceRoot string) (string, error) {
	_, canonical, err := repositoryRoots(sourceRoot)
	if err != nil {
		// filepath.Abs only touches the filesystem (via os.Getwd) when
		// sourceRoot is relative, and only fails if that fails — which was
		// not reproducible even by deliberately deleting the working
		// directory mid-test on this platform. Not a deterministic,
		// non-racy branch to force.
		root, absErr := filepath.Abs(sourceRoot)
		if absErr != nil {
			return "", absErr
		}
		return filepath.Join(filepath.Dir(root), OrgConfigFile), nil
	}
	return filepath.Join(filepath.Dir(canonical), OrgConfigFile), nil
}

type layer struct {
	path string
	data map[string]any
}

var safeSegment = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?$`)

// Resolve finds the Plan namespace for sourceRoot. Plan operations fail when
// routing is absent or ambiguous; there is deliberately no implicit local
// fallback.
func Resolve(sourceRoot string, mode AccessMode) (Resolution, error) {
	root, canonicalRoot, err := repositoryRoots(sourceRoot)
	if err != nil {
		return Resolution{}, err
	}
	source, err := repositoryIdentity(root)
	if err != nil {
		return Resolution{}, fmt.Errorf("resolve source project identity: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Resolution{}, fmt.Errorf("resolve user home: %w", err)
	}
	user, err := readLayer(filepath.Join(home, UserConfigFile))
	if err != nil {
		return Resolution{}, err
	}
	// The organization layer is anchored beside the canonical clone, even
	// when sourceRoot is a linked worktree nested below that clone.
	org, err := readLayer(filepath.Join(filepath.Dir(canonicalRoot), OrgConfigFile))
	if err != nil {
		return Resolution{}, err
	}
	repo, err := readLayer(filepath.Join(root, RepoConfigFile))
	if err != nil {
		return Resolution{}, err
	}
	if _, present := repo.data["repo_checkouts"]; present {
		return Resolution{}, fmt.Errorf("%s: repo_checkouts is machine-local and must be configured in %s, the organization layer, or %s", repo.path, LocalConfigFile, UserConfigFile)
	}
	local, err := readLayer(filepath.Join(root, LocalConfigFile))
	if err != nil {
		return Resolution{}, err
	}

	destination, routePath, err := resolveRoute(source, local, repo, org, user)
	if err != nil {
		return Resolution{}, err
	}
	result := Resolution{SourceRoot: root, SourceRepo: source, PlansRepo: destination, RouteConfigPath: routePath}
	if destination == source {
		result.PlansCheckout = root
		result.PlansDir, err = secureJoin(root, "spec", "plans")
		if err != nil {
			return Resolution{}, err
		}
		return result, nil
	}
	result.External = true
	checkout, checkoutPath, err := resolveCheckout(destination, source, local, org, user)
	if err != nil {
		return Resolution{}, err
	}
	actual, err := repositoryIdentity(checkout)
	if err != nil {
		return Resolution{}, fmt.Errorf("plans checkout %s: %w", checkout, err)
	}
	if actual != destination {
		return Resolution{}, fmt.Errorf("plans checkout %s has origin %s, want %s", checkout, actual, destination)
	}
	checkoutRoot, _, err := repositoryRoots(checkout)
	if err != nil {
		return Resolution{}, fmt.Errorf("resolve plans checkout root: %w", err)
	}
	if checkoutRoot != checkout {
		return Resolution{}, fmt.Errorf("plans checkout %s must name the repository root, not nested path %s", checkout, checkoutRoot)
	}
	_ = mode // access mode is retained for future mutation-specific validation.
	parts := strings.Split(source, "/")
	plansDir, err := secureJoin(checkout, "spec", "plans", parts[0], parts[1], parts[2])
	if err != nil {
		return Resolution{}, err
	}
	result.PlansCheckout = checkout
	result.PlansDir = plansDir
	result.CheckoutConfigPath = checkoutPath
	return result, nil
}

func resolveRoute(source string, local, repo, org, user layer) (string, string, error) {
	for _, l := range []layer{local, repo} {
		if raw, ok := l.data["plans_repo"]; ok {
			value, ok := raw.(string)
			if !ok || strings.TrimSpace(value) == "" {
				return "", "", fmt.Errorf("%s: plans_repo must be a non-empty repository identity", l.path)
			}
			normalized, err := normalizeRepo(value, strings.Split(source, "/")[0])
			if err != nil {
				return "", "", fmt.Errorf("%s: plans_repo: %w", l.path, err)
			}
			return normalized, l.path, nil
		}
	}
	for _, l := range []layer{org, user} {
		destination, found, err := matchPlanRepos(l, source)
		if err != nil {
			return "", "", err
		}
		if found {
			return destination, l.path, nil
		}
	}
	return "", "", fmt.Errorf("no plans repository is configured for %s; set plans_repo in %s or map it under plan_repos in %s", source, repo.path, user.path)
}

func matchPlanRepos(l layer, source string) (string, bool, error) {
	raw, present := l.data["plan_repos"]
	if !present {
		return "", false, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return "", false, fmt.Errorf("%s: plan_repos must map destination repositories to source repository lists", l.path)
	}
	host := strings.Split(source, "/")[0]
	var match string
	for rawDestination, rawSources := range m {
		destination, err := normalizeRepo(rawDestination, host)
		if err != nil {
			return "", false, fmt.Errorf("%s: invalid plan_repos destination %q: %w", l.path, rawDestination, err)
		}
		items, ok := rawSources.([]any)
		if !ok {
			return "", false, fmt.Errorf("%s: plan_repos.%s must be a list", l.path, rawDestination)
		}
		for _, rawSource := range items {
			value, ok := rawSource.(string)
			if !ok {
				return "", false, fmt.Errorf("%s: plan_repos.%s entries must be repository identities", l.path, rawDestination)
			}
			candidate, err := normalizeRepo(value, host)
			if err != nil {
				return "", false, fmt.Errorf("%s: invalid source repository %q: %w", l.path, value, err)
			}
			if candidate != source {
				continue
			}
			if match != "" && match != destination {
				return "", false, fmt.Errorf("%s: source %s is mapped to multiple plans repositories: %s and %s", l.path, source, match, destination)
			}
			match = destination
		}
	}
	return match, match != "", nil
}

func resolveCheckout(destination, source string, layers ...layer) (string, string, error) {
	host := strings.Split(source, "/")[0]
	for _, l := range layers {
		raw, present := l.data["repo_checkouts"]
		if !present {
			continue
		}
		m, ok := raw.(map[string]any)
		if !ok {
			return "", "", fmt.Errorf("%s: repo_checkouts must map repository identities to absolute paths", l.path)
		}
		var selected string
		for rawRepo, rawPath := range m {
			repo, err := normalizeRepo(rawRepo, host)
			if err != nil {
				return "", "", fmt.Errorf("%s: invalid repo_checkouts key %q: %w", l.path, rawRepo, err)
			}
			if repo != destination {
				continue
			}
			path, ok := rawPath.(string)
			if !ok || !filepath.IsAbs(strings.TrimSpace(path)) {
				return "", "", fmt.Errorf("%s: repo_checkouts.%s must be an absolute path", l.path, rawRepo)
			}
			if selected != "" && filepath.Clean(selected) != filepath.Clean(path) {
				return "", "", fmt.Errorf("%s: duplicate normalized checkout entries for %s", l.path, destination)
			}
			selected = path
		}
		if selected != "" {
			resolved, err := filepath.EvalSymlinks(filepath.Clean(selected))
			if err != nil {
				return "", "", fmt.Errorf("resolve plans checkout %s from %s: %w", selected, l.path, err)
			}
			return resolved, l.path, nil
		}
	}
	return "", "", fmt.Errorf("plans repository %s has no local checkout; set repo_checkouts.%s to an absolute checkout path in %s or %s", destination, destination, LocalConfigFile, UserConfigFile)
}

func normalizeRepo(raw, defaultHost string) (string, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "/") || strings.HasSuffix(raw, "/") || strings.Contains(raw, "\\") {
		return "", fmt.Errorf("repository identity must not be an absolute path or contain path separators outside owner/repo")
	}
	parts := strings.Split(raw, "/")
	if len(parts) == 2 {
		parts = append([]string{defaultHost}, parts...)
	}
	if len(parts) != 3 {
		return "", fmt.Errorf("repository identity must be owner/repo or host/owner/repo")
	}
	for _, part := range parts {
		if !safeSegment.MatchString(part) || part == "." || part == ".." {
			return "", fmt.Errorf("unsafe repository identity segment %q", part)
		}
	}
	parts[0] = strings.ToLower(parts[0])
	parts[1] = strings.ToLower(parts[1])
	parts[2] = strings.ToLower(parts[2])
	return strings.Join(parts, "/"), nil
}

func repositoryIdentity(root string) (string, error) {
	url, err := gitremote.OriginURL(root)
	if err != nil {
		return "", err
	}
	remote, ok := gitremote.Parse(url)
	if ok {
		return normalizeRepo(remote.Host+"/"+remote.Owner+"/"+remote.Repo, remote.Host)
	}
	return "", fmt.Errorf("unsupported origin remote %q", url)
}

func repositoryRoots(start string) (root, canonical string, err error) {
	rootOut, err := gitOutput(start, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", fmt.Errorf("resolve source repository root: %w", err)
	}
	root, err = filepath.EvalSymlinks(filepath.Clean(rootOut))
	if err != nil {
		return "", "", fmt.Errorf("resolve source repository path: %w", err)
	}
	common, err := gitOutput(root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", "", fmt.Errorf("resolve source common git directory: %w", err)
	}
	common, err = filepath.EvalSymlinks(filepath.Clean(common))
	if err != nil {
		return "", "", fmt.Errorf("resolve source common git directory: %w", err)
	}
	if filepath.Base(common) != ".git" {
		return "", "", fmt.Errorf("source repository common git directory %s is not a non-bare checkout", common)
	}
	canonical = filepath.Dir(common)
	return root, canonical, nil
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func readLayer(path string) (layer, error) {
	l := layer{path: path, data: map[string]any{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return l, nil
	}
	if err != nil {
		return l, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &l.data); err != nil {
		return l, fmt.Errorf("parse config %s: %w", path, err)
	}
	return l, nil
}

func secureJoin(root string, parts ...string) (string, error) {
	for _, part := range parts {
		if !safeSegment.MatchString(part) || part == "." || part == ".." {
			return "", fmt.Errorf("unsafe plans path segment %q", part)
		}
	}
	// joined is Join(root, parts...); since every part was just checked to
	// contain neither "/" nor "..", Join can only ever extend root, never
	// climb out of it — a string-level "does this escape root" check here
	// would be permanently unreachable dead weight given that invariant. The
	// symlink-aware physical check below (via rootResolved/ancestorResolved)
	// is the one that can actually fire, for a real escape hidden behind a
	// symlink rather than a literal "..".
	joined := filepath.Join(append([]string{root}, parts...)...)
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve checkout %s: %w", root, err)
	}
	// Walk up from joined looking for the first ancestor that actually
	// exists. Because joined was built as Join(root, ...) and rootResolved's
	// EvalSymlinks call above already proved root itself exists, this walk
	// is guaranteed to reach (at most) root — never filesystem root or cwd —
	// before os.Lstat starts succeeding, so no "walked off the top without
	// finding anything" fallback is reachable here.
	ancestor := joined
	for {
		_, statErr := os.Lstat(ancestor)
		if statErr == nil {
			break
		}
		if !os.IsNotExist(statErr) {
			return "", fmt.Errorf("inspect plans path %s: %w", ancestor, statErr)
		}
		ancestor = filepath.Dir(ancestor)
	}
	ancestorResolved, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", fmt.Errorf("resolve plans path ancestor %s: %w", ancestor, err)
	}
	physicalRel, err := filepath.Rel(rootResolved, ancestorResolved)
	if err != nil || physicalRel == ".." || strings.HasPrefix(physicalRel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("plans path %s escapes checkout through symbolic link", joined)
	}
	return joined, nil
}
