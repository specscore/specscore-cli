// Package graph implements discovery, parsing, validation, scaffolding, and
// navigation for GraphSpec artifact trees consumed by the `specscore graph`
// command group.
//
// GraphSpec defines the language (see the specscore spec repo, features
// graphspec/*, and decisions 0004-0007); this package only consumes it. A
// consumer graph tree lives at a single graph root per repository (default
// spec/graph/) with modules at modules/<module-id>/README.md, artifacts in
// plural collection directories (entities/, relationships/, commands/,
// events/) as unsuffixed <id>.md files, and ModelSpec sources at
// modules/<module-id>/models/*.hcl (decision 0006).
package graph

// Kind tokens — the five GraphSpec kinds (decision 0004). Value objects and
// enums are ModelSpec concepts, never GraphSpec kinds.
const (
	KindModule       = "module"
	KindEntity       = "entity"
	KindRelationship = "relationship"
	KindCommand      = "command"
	KindEvent        = "event"
)

// Kinds lists the five GraphSpec kinds in canonical order.
var Kinds = []string{KindModule, KindEntity, KindRelationship, KindCommand, KindEvent}

// collectionByKind maps a non-module kind to its plural collection directory.
var collectionByKind = map[string]string{
	KindEntity:       "entities",
	KindRelationship: "relationships",
	KindCommand:      "commands",
	KindEvent:        "events",
}

// kindByCollection is the inverse of collectionByKind — the collection
// directory that types each artifact placed under a module root.
var kindByCollection = map[string]string{
	"entities":      KindEntity,
	"relationships": KindRelationship,
	"commands":      KindCommand,
	"events":        KindEvent,
}

// CollectionDirs lists the five directories a full module scaffold carries
// (four artifact collections plus models/ for ModelSpec sources).
var CollectionDirs = []string{"entities", "relationships", "commands", "events", "models"}

// ArtifactCollections lists the four artifact-bearing collection directories,
// in canonical order (models/ excluded — it holds HCL, not graph artifacts).
var ArtifactCollections = []string{"entities", "relationships", "commands", "events"}

// isKind reports whether tok is one of the five GraphSpec kinds.
func isKind(tok string) bool {
	switch tok {
	case KindModule, KindEntity, KindRelationship, KindCommand, KindEvent:
		return true
	}
	return false
}
