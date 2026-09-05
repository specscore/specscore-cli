package sourceref

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
)

// Reference represents a parsed source reference found in source code.
type Reference struct {
	ResolvedPath    string `json:"resolved_path"`
	CrossRepoSuffix string `json:"cross_repo_suffix,omitempty"`
	Type            string `json:"type,omitempty"`
	// Fragment is the decoded address fragment, without its leading '#'. It is
	// deliberately opaque to this package: consumers such as spec lint decide
	// which resource-specific fragments they can resolve.
	Fragment string `json:"fragment,omitempty"`
	// Ref preserves a ?ref=<git-ref> pin (branch, tag, or commit) through
	// parsing and canonicalization. Parsers do not fetch it; an explicit local
	// resolver may require an exact checked-out revision before validating it.
	Ref string `json:"ref,omitempty"`

	// typed marks references parsed as directive targets. Legacy untyped source
	// references retain their historical Canonical spelling; typed targets use
	// the lowercase req/ac fragment convention.
	typed bool
}

// Canonical returns a parseable, authority-form source reference. It preserves
// both the optional revision pin and the resource fragment so consumers can
// round-trip a parsed citation without dropping its addressing semantics.
func (r Reference) Canonical() string {
	base := "specscore:" + r.ResolvedPath
	if r.CrossRepoSuffix != "" {
		base = "specscore://" + strings.TrimPrefix(r.CrossRepoSuffix, "@") + "/" + r.ResolvedPath
	}
	if r.Ref != "" {
		base += "?ref=" + r.Ref
	}
	fragment := r.Fragment
	if r.typed {
		fragment = normalizeTypedFragment(fragment)
	}
	if fragment != "" {
		base += "#" + encodeFragment(fragment)
	}
	return base
}

// LegacySuffixError is returned by ParseReference when a reference uses the
// removed `specscore:{reference}@{host}/{org}/{repo}` suffix form (decision
// 0010). Rewrite carries the exact authority-form replacement that `--fix`
// applies.
type LegacySuffixError struct {
	Rewrite string
}

func (e *LegacySuffixError) Error() string {
	return "legacy cross-repo suffix form is not allowed; rewrite to " + e.Rewrite
}

// SourceRef represents a source file reference (file + line number).
type SourceRef struct {
	FilePath    string
	LineNumber  int
	LineContent string
}

var (
	mu       sync.Mutex
	prefixes = []string{"specscore"}
	// specscore.org is the canonical expansion domain
	// (source-references#req:url-structure, decision 0010).
	domains = []string{"specscore.org"}

	// DetectionRegex is rebuilt when prefixes change.
	DetectionRegex *regexp.Regexp
)

func init() {
	DetectionRegex = buildDetectionRegex()
}

// RegisterPrefix adds a short-notation prefix (e.g. "mytool") so that
// "mytool:feature/foo" is recognized as a source reference.
// Also registers "mytool.io" as an expanded URL domain.
func RegisterPrefix(prefix string) {
	mu.Lock()
	defer mu.Unlock()
	for _, p := range prefixes {
		if p == prefix {
			return
		}
	}
	prefixes = append(prefixes, prefix)
	domains = append(domains, prefix+".io")
	DetectionRegex = buildDetectionRegex()
}

func buildDetectionRegex() *regexp.Regexp {
	var shortParts []string
	var urlParts []string
	for _, p := range prefixes {
		shortParts = append(shortParts, regexp.QuoteMeta(p+":"))
	}
	for _, d := range domains {
		urlParts = append(urlParts, regexp.QuoteMeta("https://"+d+"/"))
	}
	all := append(shortParts, urlParts...)
	pattern := `^\s*(//|#|--|/\*|\*|%|;)\s*(` + strings.Join(all, "|") + `)`
	return regexp.MustCompile(pattern)
}

// DetectReference checks if a line contains a source reference.
func DetectReference(line string) bool {
	return DetectionRegex.MatchString(line)
}

// ExtractReference extracts the reference string from a line.
func ExtractReference(line string) string {
	for _, p := range prefixes {
		prefix := p + ":"
		if idx := strings.Index(line, prefix); idx != -1 {
			// Skip optional whitespace between the prefix colon and the
			// reference body, so the readable "specscore: feature/x" form is
			// accepted alongside the tight "specscore:feature/x" form. The
			// prefix is re-attached so the returned token stays canonical
			// (no interior space) for ParseReference.
			body := strings.TrimLeft(line[idx+len(prefix):], " \t")
			if endIdx := strings.IndexAny(body, " \t\n\r"); endIdx != -1 {
				body = body[:endIdx]
			}
			return prefix + body
		}
	}
	for _, d := range domains {
		urlPrefix := "https://" + d + "/"
		if idx := strings.Index(line, urlPrefix); idx != -1 {
			extracted := line[idx:]
			if endIdx := strings.IndexAny(extracted, " \t\n\r"); endIdx != -1 {
				extracted = extracted[:endIdx]
			}
			return extracted
		}
	}
	return ""
}

// ParseReference parses an extracted reference string and returns a Reference.
func ParseReference(extracted string) (*Reference, error) {
	if extracted == "" {
		return nil, fmt.Errorf("empty reference")
	}
	for _, d := range domains {
		urlPrefix := "https://" + d + "/"
		if strings.HasPrefix(extracted, urlPrefix) {
			return parseExpandedURL(extracted, urlPrefix)
		}
	}
	for _, p := range prefixes {
		prefix := p + ":"
		if strings.HasPrefix(extracted, prefix) {
			return parseShortNotation(extracted, prefix)
		}
	}
	return nil, fmt.Errorf("unrecognized reference format: %s", extracted)
}

func parseExpandedURL(url, urlPrefix string) (*Reference, error) {
	path := strings.TrimPrefix(url, urlPrefix)
	path, gitRef, fragment, err := splitReferenceParts(path)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(path, "/")
	if len(parts) < 4 {
		return nil, fmt.Errorf("invalid expanded URL format: too few path segments")
	}
	host := parts[0]
	org := parts[1]
	repo := parts[2]
	for _, segment := range []string{host, org, repo} {
		if !isSafePathSegment(segment) {
			return nil, fmt.Errorf("invalid expanded URL format: unsafe authority segment")
		}
	}
	resolvedPath := strings.Join(parts[3:], "/")
	crossRepoSuffix := fmt.Sprintf("@%s/%s/%s", host, org, repo)
	refType := inferType(resolvedPath)
	return &Reference{
		ResolvedPath:    resolvedPath,
		CrossRepoSuffix: crossRepoSuffix,
		Type:            refType,
		Ref:             gitRef,
		Fragment:        fragment,
	}, nil
}

// splitGitRefQuery splits an optional `?ref=<git-ref>` pin off a reference
// body. The query contract is deliberately narrow: an explicit pin must be
// non-empty, and no other or additional query parameter is supported.
func splitGitRefQuery(s string) (body, gitRef string, err error) {
	i := strings.IndexByte(s, '?')
	if i < 0 {
		return s, "", nil
	}
	query := s[i+1:]
	if !strings.HasPrefix(query, "ref=") {
		return "", "", fmt.Errorf("unsupported reference query %q; expected ?ref=<git-ref>", query)
	}
	gitRef = strings.TrimPrefix(query, "ref=")
	if gitRef == "" {
		return "", "", fmt.Errorf("empty ?ref= pin is not allowed")
	}
	if strings.ContainsAny(gitRef, "&?") {
		return "", "", fmt.Errorf("unsupported reference query %q; only one ?ref=<git-ref> parameter is allowed", query)
	}
	return s[:i], gitRef, nil
}

func parseShortNotation(notation, prefix string) (*Reference, error) {
	body := strings.TrimPrefix(notation, prefix)
	body, gitRef, fragment, err := splitReferenceParts(body)
	if err != nil {
		return nil, err
	}

	// Cross-repo authority form: specscore://{host}/{org}/{repo}/{reference}.
	if strings.HasPrefix(body, "//") {
		return parseAuthorityForm(strings.TrimPrefix(body, "//"), gitRef, fragment)
	}

	// Legacy suffix form: specscore:{reference}@{host}/{org}/{repo} — removed by
	// decision 0010, reported as an error carrying the authority-form rewrite.
	if idx := strings.LastIndex(body, "@"); idx != -1 {
		reference := body[:idx]
		repoPath := body[idx+1:]
		rewrite := prefix + "//" + repoPath + "/" + reference
		if gitRef != "" {
			rewrite += "?ref=" + gitRef
		}
		if fragment != "" {
			rewrite += "#" + encodeFragment(fragment)
		}
		return nil, &LegacySuffixError{Rewrite: rewrite}
	}

	resolvedPath, err := resolveReference(body)
	if err != nil {
		return nil, err
	}
	return &Reference{
		ResolvedPath: resolvedPath,
		Type:         inferType(resolvedPath),
		Ref:          gitRef,
		Fragment:     fragment,
	}, nil
}

// parseAuthorityForm parses the "{host}/{org}/{repo}/{reference}" path of a
// cross-repo authority reference. The first three segments are host/org/repo;
// the remainder is the reference (which may itself be type-prefixed).
func parseAuthorityForm(path, gitRef, fragment string) (*Reference, error) {
	parts := strings.Split(path, "/")
	if len(parts) < 4 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return nil, fmt.Errorf("invalid cross-repo authority form: expected specscore://{host}/{org}/{repo}/{reference}")
	}
	host, org, repo := parts[0], parts[1], parts[2]
	for _, segment := range []string{host, org, repo} {
		if !isSafePathSegment(segment) {
			return nil, fmt.Errorf("invalid cross-repo authority form: unsafe authority segment")
		}
	}
	resolvedPath, err := resolveReference(strings.Join(parts[3:], "/"))
	if err != nil {
		return nil, err
	}
	return &Reference{
		ResolvedPath:    resolvedPath,
		CrossRepoSuffix: fmt.Sprintf("@%s/%s/%s", host, org, repo),
		Type:            inferType(resolvedPath),
		Ref:             gitRef,
		Fragment:        fragment,
	}, nil
}

// splitReferenceParts separates the optional URL fragment and optional
// ?ref= pin from a source reference. Fragments are parsed before the query so
// standard URL order (`...?ref=main#REQ:id`) works as expected. Parsing
// preserves the pin; a resolver decides whether it can validate that pin. The old
// splitGitRefQuery behavior remains intentionally permissive for compatibility.
func splitReferenceParts(s string) (body, gitRef, fragment string, err error) {
	if i := strings.IndexByte(s, '#'); i >= 0 {
		fragment, err = decodeFragment(s[i+1:])
		if err != nil {
			return "", "", "", err
		}
		s = s[:i]
	}
	body, gitRef, err = splitGitRefQuery(s)
	return body, gitRef, fragment, err
}

func decodeFragment(raw string) (string, error) {
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		return "", fmt.Errorf("invalid reference fragment encoding: %w", err)
	}
	if strings.ContainsRune(decoded, '\x00') {
		return "", fmt.Errorf("invalid reference fragment encoding: NUL is not allowed")
	}
	return decoded, nil
}

func encodeFragment(fragment string) string {
	encoded := url.PathEscape(fragment)
	// Colon is the required `REQ:<id>` delimiter and is legal unescaped in a
	// URI fragment. Retaining it keeps canonical citations readable.
	return strings.ReplaceAll(encoded, "%3A", ":")
}

func resolveReference(ref string) (string, error) {
	if ref == "" {
		return "", fmt.Errorf("empty reference")
	}
	for _, segment := range strings.Split(ref, "/") {
		if !isSafePathSegment(segment) {
			return "", fmt.Errorf("invalid reference path: unsafe path segment")
		}
	}
	if strings.HasPrefix(ref, "feature/") {
		return "spec/features/" + strings.TrimPrefix(ref, "feature/"), nil
	}
	if strings.HasPrefix(ref, "plan/") {
		return "spec/plans/" + strings.TrimPrefix(ref, "plan/"), nil
	}
	if strings.HasPrefix(ref, "doc/") {
		return "docs/" + strings.TrimPrefix(ref, "doc/"), nil
	}
	return ref, nil
}

func isSafePathSegment(segment string) bool {
	return segment != "" && segment != "." && segment != ".." && !strings.Contains(segment, "%")
}

func inferType(resolvedPath string) string {
	if strings.HasPrefix(resolvedPath, "spec/features/") {
		return "feature"
	}
	if strings.HasPrefix(resolvedPath, "spec/plans/") {
		return "plan"
	}
	if strings.HasPrefix(resolvedPath, "docs/") {
		return "doc"
	}
	return ""
}

// ScanLine scans a single line for references. Returns nil if none found.
func ScanLine(line string) *Reference {
	if !DetectReference(line) {
		return nil
	}
	extracted := ExtractReference(line)
	ref, err := ParseReference(extracted)
	if err != nil {
		return nil
	}
	return ref
}
