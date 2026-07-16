package sourceref

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// Reference represents a parsed source reference found in source code.
type Reference struct {
	ResolvedPath    string
	CrossRepoSuffix string
	Type            string
	// Ref carries an advisory ?ref=<git-ref> pin (branch, tag, or commit) when
	// present on the reference; it is preserved through canonical expansion and
	// does not change resolution (decision 0010).
	Ref string
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
	path, gitRef := splitGitRefQuery(path)
	parts := strings.Split(path, "/")
	if len(parts) < 4 {
		return nil, fmt.Errorf("invalid expanded URL format: too few path segments")
	}
	host := parts[0]
	org := parts[1]
	repo := parts[2]
	resolvedPath := strings.Join(parts[3:], "/")
	crossRepoSuffix := fmt.Sprintf("@%s/%s/%s", host, org, repo)
	refType := inferType(resolvedPath)
	return &Reference{
		ResolvedPath:    resolvedPath,
		CrossRepoSuffix: crossRepoSuffix,
		Type:            refType,
		Ref:             gitRef,
	}, nil
}

// splitGitRefQuery splits an advisory `?ref=<git-ref>` pin off a reference body.
// Only the exact `?ref=` marker is recognized (decision 0010); anything else is
// left untouched in the body.
func splitGitRefQuery(s string) (body, gitRef string) {
	const marker = "?ref="
	if i := strings.Index(s, marker); i >= 0 {
		return s[:i], s[i+len(marker):]
	}
	return s, ""
}

func parseShortNotation(notation, prefix string) (*Reference, error) {
	body := strings.TrimPrefix(notation, prefix)
	body, gitRef := splitGitRefQuery(body)

	// Cross-repo authority form: specscore://{host}/{org}/{repo}/{reference}.
	if strings.HasPrefix(body, "//") {
		return parseAuthorityForm(strings.TrimPrefix(body, "//"), gitRef)
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
	}, nil
}

// parseAuthorityForm parses the "{host}/{org}/{repo}/{reference}" path of a
// cross-repo authority reference. The first three segments are host/org/repo;
// the remainder is the reference (which may itself be type-prefixed).
func parseAuthorityForm(path, gitRef string) (*Reference, error) {
	parts := strings.Split(path, "/")
	if len(parts) < 4 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return nil, fmt.Errorf("invalid cross-repo authority form: expected specscore://{host}/{org}/{repo}/{reference}")
	}
	host, org, repo := parts[0], parts[1], parts[2]
	resolvedPath, err := resolveReference(strings.Join(parts[3:], "/"))
	if err != nil {
		return nil, err
	}
	return &Reference{
		ResolvedPath:    resolvedPath,
		CrossRepoSuffix: fmt.Sprintf("@%s/%s/%s", host, org, repo),
		Type:            inferType(resolvedPath),
		Ref:             gitRef,
	}, nil
}

func resolveReference(ref string) (string, error) {
	if ref == "" {
		return "", fmt.Errorf("empty reference")
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
