package graph

import (
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

// ModelspecRef is a parsed modelspec:// reference of the form
// modelspec://<module>.<Name>[@{host}/{org}/{repo}] (decision 0007).
type ModelspecRef struct {
	Module string
	Name   string
	Suffix string // "{host}/{org}/{repo}" without the leading '@', or ""
}

// ParseModelspecRef parses a modelspec:// reference. ok is false when the value
// lacks the scheme or is not a well-formed <module>.<Name> body.
func ParseModelspecRef(ref string) (ModelspecRef, bool) {
	if !strings.HasPrefix(ref, ModelspecScheme) {
		return ModelspecRef{}, false
	}
	body := strings.TrimPrefix(ref, ModelspecScheme)
	var suffix string
	if at := strings.IndexByte(body, '@'); at >= 0 {
		suffix = body[at+1:]
		body = body[:at]
	}
	dot := strings.IndexByte(body, '.')
	if dot <= 0 || dot == len(body)-1 {
		return ModelspecRef{}, false
	}
	name := body[dot+1:]
	if strings.Contains(name, ".") {
		return ModelspecRef{}, false
	}
	return ModelspecRef{Module: body[:dot], Name: name, Suffix: suffix}, true
}
