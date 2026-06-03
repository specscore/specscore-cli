package lint

import (
	"fmt"
	"os"
	"path/filepath"
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

func (c *implementsReferenceChecker) check(specRoot string) ([]Violation, error) {
	var violations []Violation
	walkErr := walkFeatureReadmes(specRoot, func(readmePath string, content []byte) {
		role := classifyFeatureRole(string(content))
		if !role.isImplementation {
			return
		}
		rel, _ := filepath.Rel(specRoot, readmePath)
		add := func(msg string) {
			violations = append(violations, Violation{
				File:     rel,
				Line:     0,
				Severity: "error",
				Rule:     "implements-reference",
				Message:  msg,
			})
		}

		ref, err := parseImplementsRef(role.implementsLine)
		if err != nil {
			add("Implements reference is not a well-formed specscore: reference (capability-and-platform-implementations#req:implements-resolution)")
			return
		}
		if ref.crossRepo {
			// Cross-repo liveness is out of scope; a well-formed cross-repo
			// reference is accepted without fetching the remote repository.
			return
		}
		slug := ref.featureSlug()
		if slug == "" {
			add(fmt.Sprintf("Implements reference %q does not resolve to a Capability Feature (capability-and-platform-implementations#req:implements-resolution)", ref.raw))
			return
		}
		target := filepath.Join(specRoot, "features", slug, "README.md")
		data, readErr := os.ReadFile(target)
		if readErr != nil {
			add(fmt.Sprintf("Implements reference %q does not resolve to an existing Capability Feature (capability-and-platform-implementations#req:implements-resolution)", ref.raw))
			return
		}
		if !classifyFeatureRole(string(data)).isCapability {
			add(fmt.Sprintf("Implements reference %q resolves to a Feature that is not a Capability — it has no \"## Implementation Matrix\" section (capability-and-platform-implementations#req:implements-resolution)", ref.raw))
		}
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return violations, nil
}
