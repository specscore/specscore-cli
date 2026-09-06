package sourceref

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/specscore/specscore-cli/pkg/projectdef"
)

// LocalResolver resolves source references only from the current project and
// explicitly configured local projects. It never fetches from a network.
type LocalResolver struct {
	specRoot     string
	repoRoot     string
	selfIdentity string
	repos        map[string][]string
}

// NewLocalResolver builds an offline resolver for a project's spec directory.
func NewLocalResolver(specRoot string) *LocalResolver {
	repoRoot := repoRootForSpecRoot(specRoot)
	r := &LocalResolver{specRoot: specRoot, repoRoot: repoRoot, repos: map[string][]string{}}
	cfg, err := projectdef.ReadSpecConfig(repoRoot)
	if err != nil {
		return r
	}
	if identity, ok := configuredProjectIdentity(repoRoot); ok {
		r.selfIdentity = identity
	}
	for _, entry := range cfg.Projects {
		dir := entry
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(repoRoot, entry)
		}
		if identity, ok := configuredProjectIdentity(dir); ok {
			r.repos[identity] = append(r.repos[identity], dir)
		}
	}
	return r
}

// ValidateRequirementCitation proves a feature #REQ:<id> citation resolves to
// one real Markdown H4 outside fenced code. A feature without a fragment is a
// resource citation and is checked for existence only. Other non-empty
// fragments are rejected rather than being silently treated as live.
func (r *LocalResolver) ValidateRequirementCitation(ref *Reference) ([]byte, error) {
	if ref != nil && ref.Fragment != "" && !strings.HasPrefix(ref.Fragment, "REQ:") {
		return nil, fmt.Errorf("unsupported fragment %q; only #REQ:<id> is an addressable requirement anchor", ref.Fragment)
	}
	return r.validateFeatureCitation(ref, requirementAnchorExists)
}

// ValidateFeatureCitation proves a Feature, REQ, or AC citation resolves from
// the current project or an explicitly configured local mirror. Typed source
// directives use lowercase #req: and #ac: fragments; legacy uppercase #REQ:
// and #AC: spellings remain accepted.
// specscore:implements https://specscore.org/github.com/specscore/specscore-cli/spec/features/cli/code/deps#req:offline-typed-source-link-check
func (r *LocalResolver) ValidateFeatureCitation(ref *Reference) ([]byte, error) {
	return r.validateFeatureCitation(ref, featureAnchorExists)
}

func (r *LocalResolver) validateFeatureCitation(ref *Reference, validateAnchor func(string, string) error) ([]byte, error) {
	if ref == nil || ref.Type != "feature" {
		return nil, fmt.Errorf("target is not a Feature")
	}
	root, repoRoot, cross, err := r.targetRoots(ref)
	if err != nil {
		return nil, err
	}
	path := ref.ResolvedPath
	if !cross {
		path = strings.TrimPrefix(path, "spec/")
	}
	readme := filepath.Join(root, filepath.FromSlash(path), "README.md")
	rel, err := filepath.Rel(root, readme)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("target path escapes the resolved repository")
	}
	var data []byte
	if ref.Ref != "" {
		revision, err := verifyCheckedOutRef(repoRoot, ref.Ref)
		if err != nil {
			return nil, err
		}
		data, err = gitShowFile(repoRoot, revision, filepath.ToSlash(filepath.Join(ref.ResolvedPath, "README.md")))
		if err != nil {
			return nil, fmt.Errorf("target Feature does not resolve: %w", err)
		}
	} else {
		if cross {
			if cleanErr := verifyTrackedClean(repoRoot, filepath.ToSlash(filepath.Join(ref.ResolvedPath, "README.md"))); cleanErr != nil {
				return nil, cleanErr
			}
		}
		if cross {
			data, err = gitShowFile(repoRoot, "HEAD", filepath.ToSlash(filepath.Join(ref.ResolvedPath, "README.md")))
		} else {
			data, err = os.ReadFile(readme)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("target Feature does not resolve: %w", err)
	}
	if err := validateAnchor(string(data), ref.Fragment); err != nil {
		return nil, err
	}
	return data, nil
}

func (r *LocalResolver) targetRoots(ref *Reference) (root, repoRoot string, cross bool, err error) {
	root, repoRoot = r.specRoot, r.repoRoot
	if ref.CrossRepoSuffix == "" {
		return root, repoRoot, false, nil
	}
	identity := strings.TrimPrefix(ref.CrossRepoSuffix, "@")
	if identity == r.selfIdentity {
		return root, repoRoot, false, nil
	}
	matches := r.repos[identity]
	switch len(matches) {
	case 0:
		return "", "", true, fmt.Errorf("repository %q is unavailable; add an identity-matching local path to specscore.yaml projects: (offline resolution never fetches)", identity)
	case 1:
		return matches[0], matches[0], true, nil
	default:
		return "", "", true, fmt.Errorf("repository %q is ambiguous across configured local paths: %s", identity, strings.Join(matches, ", "))
	}
}

func repoRootForSpecRoot(specRoot string) string {
	if _, err := os.Stat(filepath.Join(specRoot, projectdef.SpecConfigFile)); err == nil {
		return specRoot
	}
	parent := filepath.Dir(specRoot)
	if _, err := os.Stat(filepath.Join(parent, projectdef.SpecConfigFile)); err == nil {
		return parent
	}
	return specRoot
}

func configuredProjectIdentity(dir string) (string, bool) {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return "", false
	}
	cfg, err := projectdef.ReadSpecConfig(dir)
	if err != nil || cfg.Project == nil {
		return "", false
	}
	p := cfg.Project
	if p.Host == "" || p.Org == "" || p.Repo == "" {
		return "", false
	}
	return p.Host + "/" + p.Org + "/" + p.Repo, true
}

var requirementAnchorID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func requirementAnchorExists(content, fragment string) error {
	if fragment == "" {
		return nil
	}
	if !strings.HasPrefix(fragment, "REQ:") {
		return fmt.Errorf("unsupported fragment %q; only #REQ:<id> is an addressable requirement anchor", fragment)
	}
	id := strings.TrimPrefix(fragment, "REQ:")
	if !requirementAnchorID.MatchString(id) {
		return fmt.Errorf("malformed #REQ anchor %q", fragment)
	}
	return exactAnchorHeadingExists(content, "#### REQ: "+id, id, "requirement")
}

func featureAnchorExists(content, fragment string) error {
	if fragment == "" {
		return nil
	}
	colon := strings.IndexByte(fragment, ':')
	if colon < 0 {
		return fmt.Errorf("unsupported fragment %q; only #REQ:<id> and #AC:<id> are addressable Feature anchors", fragment)
	}
	id := fragment[colon+1:]
	switch strings.ToLower(fragment[:colon]) {
	case "req":
		return exactAnchorHeadingExists(content, "#### REQ: "+id, id, "requirement")
	case "ac":
		return exactAnchorHeadingExists(content, "### AC: "+id, id, "acceptance criterion")
	default:
		return fmt.Errorf("unsupported fragment %q; only #REQ:<id> and #AC:<id> are addressable Feature anchors", fragment)
	}
}

func exactAnchorHeadingExists(content, want, id, kind string) error {
	if !requirementAnchorID.MatchString(id) {
		return fmt.Errorf("malformed %s anchor %q", kind, want)
	}
	count, inComment := 0, false
	var fence byte
	fenceLength := 0
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if fence != 0 {
			if marker, length := fenceMarker(trimmed); marker == fence && length >= fenceLength && strings.TrimSpace(trimmed[length:]) == "" {
				fence, fenceLength = 0, 0
			}
			continue
		}
		if inComment {
			if strings.Contains(trimmed, "-->") {
				inComment = false
			}
			continue
		}
		if strings.Contains(trimmed, "<!--") {
			if !strings.Contains(trimmed[strings.Index(trimmed, "<!--")+4:], "-->") {
				inComment = true
			}
			continue
		}
		if marker, length := fenceMarker(trimmed); marker != 0 {
			fence, fenceLength = marker, length
			continue
		}
		if strings.TrimSuffix(line, "\r") == want {
			count++
		}
	}
	switch count {
	case 0:
		return fmt.Errorf("missing exact heading %q", want)
	case 1:
		return nil
	default:
		return fmt.Errorf("ambiguous %s anchor %q: found %d exact headings", kind, want, count)
	}
}

func fenceMarker(line string) (byte, int) {
	if len(line) < 3 || (line[0] != '`' && line[0] != '~') {
		return 0, 0
	}
	marker, length := line[0], 0
	for length < len(line) && line[length] == marker {
		length++
	}
	if length < 3 {
		return 0, 0
	}
	return marker, length
}

func verifyCheckedOutRef(repoRoot, ref string) (string, error) {
	head, err := gitRevision(repoRoot, "HEAD")
	if err != nil {
		return "", fmt.Errorf("cannot verify ?ref=%q against a local checkout: %w", ref, err)
	}
	want, err := gitRevision(repoRoot, ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("?ref=%q is not available in the configured local checkout (offline resolution never fetches)", ref)
	}
	if head != want {
		return "", fmt.Errorf("?ref=%q resolves to %s but the configured local checkout is at %s", ref, want, head)
	}
	return want, nil
}
func gitRevision(repoRoot, revision string) (string, error) {
	out, err := exec.Command("git", "-C", repoRoot, "rev-parse", "--verify", "--end-of-options", revision).Output()
	return strings.TrimSpace(string(out)), err
}
func gitShowFile(repoRoot, revision, path string) ([]byte, error) {
	return exec.Command("git", "-C", repoRoot, "show", revision+":"+path).Output()
}

func verifyTrackedClean(repoRoot, path string) error {
	if err := exec.Command("git", "-C", repoRoot, "ls-files", "--error-unmatch", "--", path).Run(); err != nil {
		return fmt.Errorf("cross-repo target %q is not tracked in the configured local checkout", path)
	}
	if err := exec.Command("git", "-C", repoRoot, "diff", "--quiet", "HEAD", "--", path).Run(); err != nil {
		return fmt.Errorf("cross-repo target %q has uncommitted bytes; commit it or cite a verified ?ref=", path)
	}
	return nil
}
