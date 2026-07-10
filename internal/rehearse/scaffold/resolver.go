// Package scaffold resolves AC references, extracts Given/When/Then text,
// and generates scaffold scenario files for the rehearse new command.
package scaffold

import (
	"fmt"
	"strings"
)

// ResolveResult holds the parsed components and raw body of a resolved AC.
type ResolveResult struct {
	FeatureSlug string
	ACSlug      string
	RawBody     []string // AC body lines after the ### AC: heading
}

// Resolve maps an acRef of the form "<feature-slug>#ac:<ac-slug>" to the
// raw body lines of that AC in spec/features/<feature-slug>/README.md.
//
// readFile is a seam for filesystem access so unit tests never touch disk.
func Resolve(acRef string, readFile func(path string) (string, error)) (*ResolveResult, error) {
	featureSlug, acSlug, err := parseACRef(acRef)
	if err != nil {
		return nil, err
	}

	path := "spec/features/" + featureSlug + "/README.md"
	content, err := readFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}

	body, available, found := extractACBody(content, acSlug)
	if !found {
		return nil, fmt.Errorf("AC %q not found in %s; available ACs: %s",
			acSlug, path, strings.Join(available, ", "))
	}

	return &ResolveResult{
		FeatureSlug: featureSlug,
		ACSlug:      acSlug,
		RawBody:     body,
	}, nil
}

// parseACRef splits "feature-slug#ac:ac-slug" into its two parts.
func parseACRef(acRef string) (featureSlug, acSlug string, err error) {
	const sep = "#ac:"
	idx := strings.Index(acRef, sep)
	if idx < 0 {
		return "", "", fmt.Errorf("AC reference must be in the form <feature-slug>#ac:<ac-slug>; got %q", acRef)
	}
	featureSlug = acRef[:idx]
	acSlug = acRef[idx+len(sep):]
	if featureSlug == "" || acSlug == "" {
		return "", "", fmt.Errorf("AC reference must be in the form <feature-slug>#ac:<ac-slug>; got %q", acRef)
	}
	return featureSlug, acSlug, nil
}

// extractACBody scans content for "### AC: <acSlug>" and returns the lines
// that follow up to the next "### AC:" or "##" heading, plus a list of all
// AC slugs found in the file.
func extractACBody(content, acSlug string) (body []string, available []string, found bool) {
	lines := strings.Split(content, "\n")
	const acPrefix = "### AC: "
	collecting := false

	for _, line := range lines {
		if strings.HasPrefix(line, acPrefix) {
			slug := strings.TrimSpace(line[len(acPrefix):])
			if collecting {
				// Reached the next AC heading — stop.
				break
			}
			available = append(available, slug)
			if slug == acSlug {
				collecting = true
				found = true
			}
			continue
		}
		if collecting {
			if strings.HasPrefix(line, "## ") {
				break
			}
			body = append(body, line)
		}
	}

	// If we started collecting but haven't hit a terminator yet, body is
	// already complete. Collect remaining AC slugs for the available list.
	if !found {
		return nil, available, false
	}

	// Re-scan for remaining available ACs after the found one.
	reachedFound := false
	for _, line := range lines {
		if strings.HasPrefix(line, acPrefix) {
			slug := strings.TrimSpace(line[len(acPrefix):])
			if slug == acSlug {
				reachedFound = true
				continue
			}
			if reachedFound {
				available = append(available, slug)
			}
		}
	}

	return body, available, true
}
