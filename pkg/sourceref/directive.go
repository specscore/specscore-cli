package sourceref

import (
	"fmt"
	"regexp"
	"strings"
)

// Relation is the semantic relationship between a source location and a
// SpecScore target.
type Relation string

const (
	RelationImplements Relation = "implements"
	RelationVerifies   Relation = "verifies"
	RelationReferences Relation = "references"
)

// Directive is a typed source annotation. Source-line information is retained
// for callers that scan files themselves; symbol attachment is deliberately
// left to language-aware providers such as CodeGrapher.
type Directive struct {
	Relation Relation   `json:"relation"`
	Target   *Reference `json:"target"`
	Line     int        `json:"line,omitempty"`
}

var (
	directiveRe        = regexp.MustCompile(`\bspecscore:(implements|verifies|references)\s+([^\s]+)`)
	unknownDirectiveRe = regexp.MustCompile(`\bspecscore:([A-Za-z][A-Za-z-]*)\s+(?:(?:https://specscore\.org/)|(?:specscore:)|(?:feature|plan|doc)/)`)
)

// ParseDirective parses one typed source directive. An unqualified source
// reference is also accepted and gets references semantics. The target may be
// a short reference, a specscore authority reference, or a canonical
// https://specscore.org URL.
func ParseDirective(line string) (*Directive, error) {
	if m := directiveRe.FindStringSubmatch(line); m != nil {
		target, err := parseDirectiveTarget(m[2])
		if err != nil {
			return nil, err
		}
		return &Directive{Relation: Relation(m[1]), Target: target}, nil
	}
	if strings.Contains(line, "specscore:implements") || strings.Contains(line, "specscore:verifies") || strings.Contains(line, "specscore:references") {
		return nil, fmt.Errorf("malformed specscore directive: expected relation and target")
	}
	if m := unknownDirectiveRe.FindStringSubmatch(line); m != nil {
		return nil, fmt.Errorf("unknown source relation %q", m[1])
	}

	// Preserve the existing comment-prefix requirement for untyped references.
	if !DetectReference(line) {
		return nil, nil
	}
	token := ExtractReference(line)
	ref, err := ParseReference(token)
	if err != nil {
		return nil, err
	}
	return &Directive{Relation: RelationReferences, Target: ref}, nil
}

// ScanDirective scans one source line and returns nil when it has no
// SpecScore annotation. It is intentionally syntax-agnostic: providers may
// later attach the directive to the nearest parsed symbol.
func ScanDirective(line string) *Directive {
	if !DetectReference(line) {
		return nil
	}
	d, err := ParseDirective(line)
	if err != nil {
		return nil
	}
	return d
}

// CanonicalTyped returns the parseable canonical reference with the
// resource-specific fragment prefix normalized for typed traceability.
func (r Reference) CanonicalTyped() string {
	return canonicalReference(&r)
}

// Canonical returns the directive with its target in canonical URL form. REQ
// and AC fragment prefixes are normalized to lowercase; other fragments stay
// opaque to this package.
func (d Directive) Canonical() string {
	if d.Target == nil {
		return ""
	}
	return "specscore:" + string(normalizeRelation(d.Relation)) + " " + canonicalReference(d.Target)
}

// ValidateRelationTarget applies the relation-to-resource rules owned by
// SpecScore. It does not inspect the source symbol kind; executable-test
// attachment is a provider concern.
func ValidateRelationTarget(relation Relation, target *Reference) error {
	if target == nil {
		return fmt.Errorf("relation %q has no target", relation)
	}
	switch relation {
	case RelationReferences:
		return nil
	case RelationImplements:
		if target.Type != "feature" || !hasFragmentPrefix(target.Fragment, "req") {
			return fmt.Errorf("implements target must be a REQ")
		}
	case RelationVerifies:
		if target.Type != "feature" || (!hasFragmentPrefix(target.Fragment, "req") && !hasFragmentPrefix(target.Fragment, "ac")) {
			return fmt.Errorf("verifies target must be an AC or REQ")
		}
	default:
		return fmt.Errorf("unknown source relation %q", relation)
	}
	return nil
}

// ValidateDirective validates a parsed directive's relation and target.
func ValidateDirective(d *Directive) error {
	if d == nil {
		return fmt.Errorf("nil source directive")
	}
	return ValidateRelationTarget(d.Relation, d.Target)
}

// ValidateRelationTargetString parses and validates an authoring-form target
// in one step. It is a convenience for linter and provider adapters that keep
// directives as text until validation.
func ValidateRelationTargetString(relation Relation, raw string) error {
	target, err := parseDirectiveTarget(raw)
	if err != nil {
		return err
	}
	return ValidateRelationTarget(relation, target)
}

func parseDirectiveTarget(raw string) (*Reference, error) {
	var (
		ref *Reference
		err error
	)
	if strings.HasPrefix(raw, "https://specscore.org/") || strings.HasPrefix(raw, "specscore:") {
		ref, err = ParseReference(raw)
	} else {
		ref, err = ParseReference("specscore:" + raw)
	}
	if err != nil {
		return nil, err
	}
	ref.typed = true
	return ref, nil
}

func normalizeRelation(relation Relation) Relation {
	if relation == "" {
		return RelationReferences
	}
	return relation
}

func hasFragmentPrefix(fragment, prefix string) bool {
	if len(fragment) <= len(prefix)+1 || fragment[len(prefix)] != ':' {
		return false
	}
	return strings.EqualFold(fragment[:len(prefix)], prefix)
}

func canonicalReference(ref *Reference) string {
	if ref == nil {
		return ""
	}
	copy := *ref
	copy.typed = true
	return copy.Canonical()
}

func normalizeTypedFragment(fragment string) string {
	if hasFragmentPrefix(fragment, "req") {
		return "req:" + fragment[strings.IndexByte(fragment, ':')+1:]
	}
	if hasFragmentPrefix(fragment, "ac") {
		return "ac:" + fragment[strings.IndexByte(fragment, ':')+1:]
	}
	return fragment
}
