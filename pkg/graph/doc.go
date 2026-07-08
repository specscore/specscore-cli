// Package graph implements discovery, parsing, validation, scaffolding, and
// navigation for GraphSpec artifact trees consumed by the `specscore graph`
// command group.
//
// GraphSpec defines the language (see the specscore spec repo, features
// graphspec/*, and decisions 0004-0007); this package only consumes it. A
// consumer graph tree lives at a single graph root per repository (default
// spec/graph/) with modules at modules/<module-id>/README.md, artifacts in
// plural collection directories (entities/, relationships/, commands/,
// events/, policies/) as unsuffixed <id>.md files, and ModelSpec sources at
// modules/<module-id>/models/*.hcl (decision 0006).
package graph

// Kind tokens — the six GraphSpec kinds (decision 0004, amended by decision
// 0013 which adds policy). Value objects and enums are ModelSpec concepts,
// never GraphSpec kinds.
const (
	KindModule       = "module"
	KindEntity       = "entity"
	KindRelationship = "relationship"
	KindCommand      = "command"
	KindEvent        = "event"
	KindPolicy       = "policy"
)

// Kinds lists the six GraphSpec kinds in canonical order.
var Kinds = []string{KindModule, KindEntity, KindRelationship, KindCommand, KindEvent, KindPolicy}

// collectionByKind maps a non-module kind to its plural collection directory.
var collectionByKind = map[string]string{
	KindEntity:       "entities",
	KindRelationship: "relationships",
	KindCommand:      "commands",
	KindEvent:        "events",
	KindPolicy:       "policies",
}

// kindByCollection is the inverse of collectionByKind — the collection
// directory that types each artifact placed under a module root.
var kindByCollection = map[string]string{
	"entities":      KindEntity,
	"relationships": KindRelationship,
	"commands":      KindCommand,
	"events":        KindEvent,
	"policies":      KindPolicy,
}

// CollectionDirs lists the six directories a full module scaffold carries
// (five artifact collections plus models/ for ModelSpec sources).
var CollectionDirs = []string{"entities", "relationships", "commands", "events", "policies", "models"}

// ArtifactCollections lists the five artifact-bearing collection directories,
// in canonical order (models/ excluded — it holds HCL, not graph artifacts).
var ArtifactCollections = []string{"entities", "relationships", "commands", "events", "policies"}

// isKind reports whether tok is one of the six GraphSpec kinds.
func isKind(tok string) bool {
	switch tok {
	case KindModule, KindEntity, KindRelationship, KindCommand, KindEvent, KindPolicy:
		return true
	}
	return false
}
