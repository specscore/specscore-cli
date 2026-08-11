package lint

import (
	"errors"
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
	resolver := sourceref.NewLocalResolver(specRoot)
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
			var lse *sourceref.LegacySuffixError
			if errors.As(err, &lse) {
				violations = append(violations, Violation{
					File:      rel,
					Line:      0,
					Severity:  "error",
					Rule:      "implements-reference",
					Message:   fmt.Sprintf("Implements reference uses the legacy cross-repo suffix form; rewrite to %s (source-references#req:cross-repo-authority)", lse.Rewrite),
					FixTarget: "implements-reference",
				})
				return
			}
			add("Implements reference is not a well-formed specscore: reference (capability-and-platform-implementations#req:implements-resolution)")
			return
		}
		slug := ref.featureSlug()
		if slug == "" {
			add(fmt.Sprintf("Implements reference %q does not resolve to a Capability Feature (capability-and-platform-implementations#req:implements-resolution)", ref.raw))
			return
		}
		data, readErr := resolver.ValidateRequirementCitation(ref.ref)
		if readErr != nil {
			add(fmt.Sprintf("Implements reference %q cannot be resolved locally: %v (capability-and-platform-implementations#req:implements-resolution)", ref.raw, readErr))
			return
		}
		if !classifyFeatureRole(string(data)).isCapability {
			add(fmt.Sprintf("Implements reference %q resolves to a Feature that is not a Capability — it has no \"## Implementation Matrix\" section (capability-and-platform-implementations#req:implements-resolution)", ref.raw))
			return
		}
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return violations, nil
}

// fix rewrites every Implementation Feature's "**Implements:**" line that uses
// the removed legacy `@{host}/{org}/{repo}` suffix to the authority form
// (source-references#req:cross-repo-authority / decision 0010), preserving all
// other bytes. Idempotent: an already-authority-form line is left untouched.
func (c *implementsReferenceChecker) fix(specRoot string) error {
	return walkFeatureReadmes(specRoot, func(readmePath string, content []byte) {
		out, changed := rewriteLegacyImplementsLine(content)
		if !changed {
			return
		}
		_ = os.WriteFile(readmePath, out, 0o644)
	})
}

// rewriteLegacyImplementsLine rewrites the "**Implements:**" line's legacy
// specscore suffix reference to its authority form. It returns the possibly
// updated content and whether a rewrite happened.
func rewriteLegacyImplementsLine(content []byte) ([]byte, bool) {
	role := classifyFeatureRole(string(content))
	if !role.isImplementation {
		return content, false
	}
	token := sourceref.ExtractReference(role.implementsLine)
	if token == "" {
		return content, false
	}
	_, err := sourceref.ParseReference(token)
	var lse *sourceref.LegacySuffixError
	if !errors.As(err, &lse) {
		return content, false
	}
	newLine := strings.Replace(role.implementsLine, token, lse.Rewrite, 1)
	out := strings.Replace(string(content), role.implementsLine, newLine, 1)
	return []byte(out), true
}
