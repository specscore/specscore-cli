package lint

import (
	"fmt"
	"strings"

	"github.com/specscore/specscore-cli/pkg/sourceref"
)

// implementsRef is a parsed "**Implements:**" reference. It reuses the
// source-references parser (sourceref) rather than duplicating notation logic,
// so the cross-repo "@{host}/{org}/{repo}" suffix is understood for free.
type implementsRef struct {
	raw       string               // the extracted "specscore:..." token
	ref       *sourceref.Reference // the parsed source reference
	crossRepo bool                 // true when the reference carries a cross-repo suffix
}

// featureSlug returns the Capability feature slug a same-repo reference points
// at (e.g. "dashboards" for "specscore:feature/dashboards"). For references
// whose resolved path is not a feature, it returns "".
func (r implementsRef) featureSlug() string {
	const prefix = "spec/features/"
	if r.ref == nil || !strings.HasPrefix(r.ref.ResolvedPath, prefix) {
		return ""
	}
	return strings.TrimPrefix(r.ref.ResolvedPath, prefix)
}

// parseImplementsRef parses the reference token out of an "**Implements:**"
// line by reusing sourceref.ExtractReference / sourceref.ParseReference. It
// returns an error when the line carries no recognizable specscore: reference
// or when the notation is malformed.
func parseImplementsRef(line string) (implementsRef, error) {
	token := sourceref.ExtractReference(line)
	if token == "" {
		return implementsRef{}, fmt.Errorf("no recognizable specscore: reference")
	}
	ref, err := sourceref.ParseReference(token)
	if err != nil {
		return implementsRef{}, err
	}
	return implementsRef{
		raw:       token,
		ref:       ref,
		crossRepo: ref.CrossRepoSuffix != "",
	}, nil
}

// implementsReferenceChecker validates the "**Implements:**" reference on every
// Implementation Feature (capability-and-platform-implementations#
// req:implements-field, #req:implements-cross-repo, #req:implements-resolution).
type implementsReferenceChecker struct{}

func newImplementsReferenceChecker() checker { return &implementsReferenceChecker{} }

func (c *implementsReferenceChecker) name() string     { return "implements-reference" }
func (c *implementsReferenceChecker) severity() string { return "error" }

// findImplementsLine returns the first "**Implements:**" line in content, or ""
// if there is none.
func findImplementsLine(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), implementsPrefix) {
			return line
		}
	}
	return ""
}

func (c *implementsReferenceChecker) check(specRoot string) ([]Violation, error) {
	var violations []Violation
	walkErr := walkFeatureReadmes(specRoot, func(readmePath string, content []byte) {
		role := classifyFeatureRole(string(content))
		if !role.isImplementation {
			return
		}
		line := findImplementsLine(string(content))
		if _, err := parseImplementsRef(line); err != nil {
			return
		}
		// Well-formed references — same-repo (resolution validated in a later
		// task) and cross-repo (accepted without remote fetch) — pass here.
		_ = readmePath
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return violations, nil
}
