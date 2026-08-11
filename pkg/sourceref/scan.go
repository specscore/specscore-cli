package sourceref

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ScanResult represents the references found in a set of files.
type ScanResult struct {
	// FileRefs maps file path to list of references found in that file
	FileRefs map[string][]*Reference
	// ParseErrors records syntactically detected annotations that could not be
	// parsed. Listing mode remains backward-compatible; validation mode reports
	// these deterministic file/line diagnostics instead of silently dropping them.
	ParseErrors map[string][]ParseError
}

type ParseError struct {
	Line  int
	Token string
	Err   error
}

// ScanFiles scans a list of files for source references.
// Returns a ScanResult with all references grouped by file.
// The result may be partial if some files fail; errors are accumulated
// and returned alongside whatever refs were successfully scanned.
func ScanFiles(filePaths []string) (*ScanResult, error) {
	result := &ScanResult{
		FileRefs: make(map[string][]*Reference), ParseErrors: make(map[string][]ParseError),
	}
	var errs []string
	for _, filePath := range filePaths {
		refs, parseErrors, err := scanFileDetailed(filePath)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", filePath, err))
			continue
		}
		if len(refs) > 0 {
			result.FileRefs[filePath] = refs
		}
		if len(parseErrors) > 0 {
			result.ParseErrors[filePath] = parseErrors
		}
	}
	if len(errs) > 0 {
		return result, fmt.Errorf("scan errors: %s", strings.Join(errs, "; "))
	}
	return result, nil
}

func scanFile(filePath string) ([]*Reference, error) {
	refs, _, err := scanFileDetailed(filePath)
	return refs, err
}

func scanFileDetailed(filePath string) ([]*Reference, []ParseError, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = file.Close() }()

	seen := make(map[string]bool)
	var refs []*Reference
	var parseErrors []ParseError

	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		if !DetectReference(line) {
			continue
		}
		token := ExtractReference(line)
		ref, parseErr := ParseReference(token)
		if parseErr != nil {
			parseErrors = append(parseErrors, ParseError{Line: lineNumber, Token: token, Err: parseErr})
			continue
		}
		if ref != nil {
			key := ref.Canonical()
			if !seen[key] {
				seen[key] = true
				refs = append(refs, ref)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return refs, parseErrors, err
	}
	sort.Slice(refs, func(i, j int) bool {
		return refs[i].Canonical() < refs[j].Canonical()
	})
	return refs, parseErrors, nil
}

// ExpandGlobPattern expands a glob pattern to a list of file paths.
// Returns sorted file paths.
//
// Supported patterns:
//   - "**/*" or "**" — matches all files recursively (special-cased)
//   - "**" as a path segment — matches any number of directory segments
//     (including zero), so "pkg/**/*.go" matches "pkg/a.go", "pkg/sub/b.go",
//     and "pkg/sub/deep/c.go" (cli/code/deps#req:path-glob).
//   - Standard filepath.Match patterns (e.g. "*.go", "src/*.txt"); a single
//     "*" or "?" never crosses a "/" separator.
func ExpandGlobPattern(pattern string) ([]string, error) {
	if pattern == "" {
		pattern = "**/*"
	}

	// Validate glob pattern first
	if _, err := filepath.Match(pattern, "test"); err != nil {
		return nil, err
	}

	var matches []string
	filepath.Walk(".", func(path string, info os.FileInfo, err error) error { //nolint:errcheck
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}

		normalPath := filepath.ToSlash(path)
		ok, _ := matchGlobPattern(normalPath, pattern)
		if ok {
			matches = append(matches, normalPath)
		}
		return nil
	})

	sort.Strings(matches)
	return matches, nil
}

// matchGlobPattern matches a file path against a glob pattern.
// Supports * and ? (matching within a single path segment) and ** (matching
// across segments, including zero segments).
func matchGlobPattern(path string, pattern string) (bool, error) {
	if pattern == "**/*" || pattern == "**" {
		return true, nil
	}

	// Patterns containing "**" need a matcher that lets it span "/" separators,
	// which filepath.Match cannot do. Translate them to an anchored regexp.
	if strings.Contains(pattern, "**") {
		re := doubleStarRegexp(pattern)
		return re.MatchString(path), nil
	}

	matched, err := filepath.Match(pattern, path)
	if err != nil {
		return false, err
	}
	return matched, nil
}

// doubleStarRegexp compiles a glob pattern containing "**" into an anchored
// regexp. Translation rules:
//   - "**/"  → matches zero or more leading path segments
//   - "**"   → matches any run of characters, including "/"
//   - "*"    → matches any run of non-separator characters
//   - "?"    → matches a single non-separator character
//   - every other character is matched literally
func doubleStarRegexp(pattern string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				// "**" — optionally followed by "/" to allow zero segments.
				if i+2 < len(pattern) && pattern[i+2] == '/' {
					b.WriteString("(?:.*/)?")
					i += 2
				} else {
					b.WriteString(".*")
					i++
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
	}
	b.WriteString("$")
	// The translation emits only fixed, quoted fragments and known-valid regexp
	// tokens, so compilation cannot fail. MustCompile makes that invariant
	// explicit instead of exposing an unreachable error path to callers.
	return regexp.MustCompile(b.String())
}

// GetUniqueReferences extracts unique references from a ScanResult, optionally filtered by type.
// Returns references sorted by (resolved_path, cross_repo_suffix).
func GetUniqueReferences(result *ScanResult, typeFilter string) []*Reference {
	seen := make(map[string]*Reference)

	for _, refs := range result.FileRefs {
		for _, ref := range refs {
			if typeFilter != "" && ref.Type != typeFilter {
				continue
			}
			key := ref.Canonical()
			if _, exists := seen[key]; !exists {
				seen[key] = ref
			}
		}
	}

	var unique []*Reference
	for _, ref := range seen {
		unique = append(unique, ref)
	}

	sort.Slice(unique, func(i, j int) bool {
		keyI := unique[i].Canonical()
		keyJ := unique[j].Canonical()
		return keyI < keyJ
	})

	return unique
}

// FormatOutput formats the scan results for output.
// If singleFile is true, returns a flat list. Otherwise, groups by file with headers.
func FormatOutput(result *ScanResult, singleFile bool, typeFilter string) string {
	if len(result.FileRefs) == 0 {
		return ""
	}

	var output []string

	if singleFile {
		refs := GetUniqueReferences(result, typeFilter)
		for _, ref := range refs {
			output = append(output, ref.display())
		}
	} else {
		fileNames := make([]string, 0, len(result.FileRefs))
		for fname := range result.FileRefs {
			fileNames = append(fileNames, fname)
		}
		sort.Strings(fileNames)

		for i, fname := range fileNames {
			if i > 0 {
				output = append(output, "")
			}
			output = append(output, fname)

			refs := result.FileRefs[fname]
			filtered := refs
			if typeFilter != "" {
				filtered = nil
				for _, ref := range refs {
					if ref.Type == typeFilter {
						filtered = append(filtered, ref)
					}
				}
			}

			for _, ref := range filtered {
				output = append(output, "  "+ref.display())
			}
		}
	}

	if len(output) == 0 {
		return ""
	}

	return fmt.Sprintf("%s\n", joinStrings(output))
}

// display preserves the historic concise deps output while retaining address
// components that make two citations meaningfully distinct.
func (r Reference) display() string {
	value := r.ResolvedPath + r.CrossRepoSuffix
	if r.Ref != "" {
		value += "?ref=" + r.Ref
	}
	if r.Fragment != "" {
		value += "#" + r.Fragment
	}
	return value
}

// joinStrings joins strings with newlines.
func joinStrings(strs []string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += "\n"
		}
		result += s
	}
	return result
}
