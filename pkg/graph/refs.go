package graph

import (
	"fmt"
	"regexp"
	"strings"
)

// kebabRe matches a bare lowercase kebab-case id (decision 0005): lowercase
// alphanumerics in dash-separated segments, no leading/trailing/double dashes.
var kebabRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// IsKebab reports whether s is a bare lowercase kebab-case identifier.
func IsKebab(s string) bool {
	return kebabRe.MatchString(s)
}

// ModelspecScheme is the reference scheme for graph-to-ModelSpec references.
const ModelspecScheme = "modelspec://"

// QualifiedRef is a parsed graph-to-graph reference of the form
// <module>.<local-id> (decision 0005).
type QualifiedRef struct {
	Module string
	Local  string
}

// ParseQualifiedRef splits a <module>.<local-id> reference. ok is false when
// the value is not exactly module + one dot + local, both non-empty, or carries
// a modelspec:// scheme.
func ParseQualifiedRef(ref string) (QualifiedRef, bool) {
	if ref == "" || strings.Contains(ref, "://") {
		return QualifiedRef{}, false
	}
	dot := strings.IndexByte(ref, '.')
	if dot <= 0 || dot == len(ref)-1 {
		return QualifiedRef{}, false
	}
	module := ref[:dot]
	local := ref[dot+1:]
	if strings.Contains(local, ".") {
		return QualifiedRef{}, false
	}
	return QualifiedRef{Module: module, Local: local}, true
}

// ModelspecKindTokens maps the plural kind segment of a modelspec:// reference
// (decision 0011) to the singular ModelSpec concept kind it addresses. The kind
// segment is OPTIONAL: the two-segment form modelspec:///<module>.<Name>
// resolves against the flat entity/component/enum trio; the three-segment form
// modelspec:///<module>.<kind>.<Name> names one of these five kinds explicitly.
var ModelspecKindTokens = map[string]string{
	"entities":    "entity",
	"components":  "component",
	"enums":       "enum",
	"collections": "collection",
	"recordsets":  "recordset",
}

// ReservedConceptNames is the set of tokens (decision 0011 / ModelSpec decision
// 0015) forbidden as ModelSpec concept names in any scope: they are the kind
// segments of consumer reference syntax, so reserving them keeps kind-explicit
// references unambiguous without lookahead.
var ReservedConceptNames = map[string]bool{
	"entities":    true,
	"components":  true,
	"enums":       true,
	"collections": true,
	"recordsets":  true,
}

// ModelspecRef is a parsed modelspec:// reference (decisions 0010/0011). The
// local form (modelspec:///<...>) carries an empty Repo; the cross-repo form
// (modelspec://{host}/{org}/{repo}/<...>) fills Repo with "host/org/repo". Kind
// is the singular concept kind for the three-segment form, or "" for the
// two-segment (trio) form. Ref carries an advisory ?ref= git pin, if any.
type ModelspecRef struct {
	Module string
	Kind   string // singular: entity|component|enum|collection|recordset; "" = two-segment (trio)
	Name   string
	Repo   string // "{host}/{org}/{repo}" for cross-repo, or "" for local
	Ref    string // advisory ?ref= git pin (branch/tag/commit), or ""
}

// modelspecErrKind classifies a modelspec reference malformation so the linter
// can route each to the right rule and message.
type modelspecErrKind int

const (
	msErrMalformed   modelspecErrKind = iota // generic grammar violation
	msErrNotScheme                           // value does not carry the modelspec:// scheme
	msErrLegacyForm                          // legacy modelspec://x.Y (authority present, empty path)
	msErrFragment                            // a fragment (#...) is reserved in v0.2
	msErrUnknownKind                         // kind segment is not one of the five tokens
)

// ModelspecParseError is the typed error returned by ParseModelspecRef. Rewrite
// is set only for the legacy-form class and carries the exact corrected
// reference the --fix rewriter applies.
type ModelspecParseError struct {
	kind    modelspecErrKind
	message string
	Rewrite string
}

func (e *ModelspecParseError) Error() string { return e.message }

// isLegacyForm reports whether err is a legacy-form modelspec parse error.
func isLegacyForm(err error) (*ModelspecParseError, bool) {
	pe, ok := err.(*ModelspecParseError)
	if ok && pe.kind == msErrLegacyForm {
		return pe, true
	}
	return nil, false
}

// ParseModelspecRef parses a modelspec:// reference into its parts. On failure
// it returns a *ModelspecParseError whose class the linter inspects (legacy
// form carries the exact --fix rewrite; other classes carry a clear message).
func ParseModelspecRef(ref string) (ModelspecRef, error) {
	if !strings.HasPrefix(ref, ModelspecScheme) {
		return ModelspecRef{}, &ModelspecParseError{
			kind:    msErrNotScheme,
			message: fmt.Sprintf("missing %s scheme", ModelspecScheme),
		}
	}
	rest := ref[len(ModelspecScheme):]

	// A fragment is reserved for future intra-concept addressing (decision 0010).
	if strings.IndexByte(rest, '#') >= 0 {
		return ModelspecRef{}, &ModelspecParseError{
			kind:    msErrFragment,
			message: "a fragment (#...) is reserved for future intra-concept addressing and is not allowed in v0.2",
		}
	}

	// Split off an optional ?ref= git pin (advisory; does not alter resolution).
	var gitRef, query string
	if q := strings.IndexByte(rest, '?'); q >= 0 {
		query = rest[q+1:]
		rest = rest[:q]
		var qerr error
		gitRef, qerr = parseRefQuery(query)
		if qerr != nil {
			return ModelspecRef{}, qerr
		}
	}

	// Empty authority (triple slash): the local form. rest begins with '/'.
	if strings.HasPrefix(rest, "/") {
		concept := rest[1:]
		if concept == "" || strings.Contains(concept, "/") {
			return ModelspecRef{}, malformedModelspec()
		}
		return parseConceptSegment(concept, "", gitRef)
	}

	// Authority present. The path (if any) starts after the first '/'.
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		// Authority present, empty path: the legacy form. Rewrite to the
		// triple-slash local form, preserving any ?ref= pin.
		rewrite := ModelspecScheme + "/" + rest
		if query != "" {
			rewrite += "?" + query
		}
		return ModelspecRef{}, &ModelspecParseError{
			kind:    msErrLegacyForm,
			message: fmt.Sprintf("legacy modelspec form %q (authority present, empty path); rewrite to %q", ref, rewrite),
			Rewrite: rewrite,
		}
	}
	host := rest[:slash]
	pathPart := rest[slash+1:]
	segs := strings.Split(pathPart, "/")
	// The last path segment is the concept; everything before it (org/repo, or
	// a nested subgroup path) is the repository path. A well-formed cross-repo
	// reference needs at least {org}/{repo}/<concept>.
	if host == "" || len(segs) < 3 {
		return ModelspecRef{}, malformedModelspec()
	}
	concept := segs[len(segs)-1]
	repoPath := segs[:len(segs)-1]
	for _, s := range repoPath {
		if s == "" {
			return ModelspecRef{}, malformedModelspec()
		}
	}
	repo := host + "/" + strings.Join(repoPath, "/")
	return parseConceptSegment(concept, repo, gitRef)
}

// parseConceptSegment parses the final <module>[.<kind>].<Name> segment.
func parseConceptSegment(concept, repo, gitRef string) (ModelspecRef, error) {
	parts := strings.Split(concept, ".")
	for _, p := range parts {
		if p == "" {
			return ModelspecRef{}, malformedModelspec()
		}
	}
	switch len(parts) {
	case 2:
		return ModelspecRef{Module: parts[0], Name: parts[1], Repo: repo, Ref: gitRef}, nil
	case 3:
		kind, ok := ModelspecKindTokens[parts[1]]
		if !ok {
			return ModelspecRef{}, &ModelspecParseError{
				kind:    msErrUnknownKind,
				message: fmt.Sprintf("unknown kind segment %q (expected one of entities, components, enums, collections, recordsets)", parts[1]),
			}
		}
		return ModelspecRef{Module: parts[0], Kind: kind, Name: parts[2], Repo: repo, Ref: gitRef}, nil
	default:
		return ModelspecRef{}, malformedModelspec()
	}
}

// parseRefQuery extracts the git pin from a modelspec reference query string.
// Only the single `ref=<value>` parameter is defined in v0.2 (decision 0010);
// any other query shape is a malformation.
func parseRefQuery(query string) (string, error) {
	const p = "ref="
	if !strings.HasPrefix(query, p) || strings.ContainsAny(query, "&;") {
		return "", &ModelspecParseError{
			kind:    msErrMalformed,
			message: fmt.Sprintf("unsupported query %q (only ?ref=<git-ref> is defined)", query),
		}
	}
	val := query[len(p):]
	if val == "" {
		return "", &ModelspecParseError{
			kind:    msErrMalformed,
			message: "empty ?ref= git pin",
		}
	}
	return val, nil
}

func malformedModelspec() *ModelspecParseError {
	return &ModelspecParseError{
		kind:    msErrMalformed,
		message: "expected modelspec:///<module>[.<kind>].<Name> (local) or modelspec://{host}/{org}/{repo}/<module>[.<kind>].<Name> (cross-repo)",
	}
}
