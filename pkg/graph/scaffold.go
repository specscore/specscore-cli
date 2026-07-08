package graph

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// ScaffoldOptions configures `graph new`.
type ScaffoldOptions struct {
	Kind     string
	Name     string
	ID       string // optional; derived from Name when empty
	Module   string // required for non-module kinds
	RepoRoot string // absolute repo root
	Root     string // repo-level graph-root override (relative); "" = spec/graph
	Status   string // default "draft"
	Summary  string
	From     string // relationship
	To       string // relationship
	Subject  string // command / event
	Bare     bool   // module: skip collection scaffolding
}

// ScaffoldResult reports the created files (repo-relative, sorted) and the
// primary artifact path.
type ScaffoldResult struct {
	Created []string
	Target  string
}

var kindTitle = map[string]string{
	KindModule:       "Module",
	KindEntity:       "Entity",
	KindRelationship: "Relationship",
	KindCommand:      "Command",
	KindEvent:        "Event",
}

// DeriveID derives a bare kebab-case id from a display name (decision 0005):
// `CreateBooking` and `teamMember` both derive dash-separated ids
// (`create-booking`, `team-member`); non-alphanumerics become dashes.
func DeriveID(name string) string {
	var b strings.Builder
	prevDash := true // suppress a leading dash
	prevLowerOrDigit := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
			prevLowerOrDigit = true
		case r >= 'A' && r <= 'Z':
			if prevLowerOrDigit && !prevDash {
				b.WriteByte('-')
			}
			b.WriteRune(r - 'A' + 'a')
			prevDash = false
			prevLowerOrDigit = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
			prevLowerOrDigit = false
		}
	}
	return strings.Trim(b.String(), "-")
}

// Scaffold writes a new GraphSpec artifact and returns the created files. It
// performs the collision check before any write (no partial writes) and returns
// exitcode-carrying errors for the documented failure modes.
func Scaffold(opts ScaffoldOptions) (ScaffoldResult, error) {
	status := opts.Status
	if status == "" {
		status = "draft"
	}
	id := opts.ID
	if id == "" {
		id = DeriveID(opts.Name)
	}
	if id == "" {
		return ScaffoldResult{}, exitcode.InvalidArgsError("cannot derive an id from --name; supply --id")
	}
	if strings.Contains(id, ".") {
		return ScaffoldResult{}, exitcode.InvalidArgsErrorf(
			"--id %q must be a bare local id; qualified IDs are computed from placement, not stored", id)
	}
	if !IsKebab(id) {
		return ScaffoldResult{}, exitcode.InvalidArgsErrorf("--id %q must be bare lowercase kebab-case", id)
	}

	if opts.Kind == KindModule {
		return scaffoldModule(opts, id, status)
	}
	return scaffoldArtifact(opts, id, status)
}

// scaffoldModule creates a module README plus (unless --bare) the five
// collection directories with README indexes.
func scaffoldModule(opts ScaffoldOptions, id, status string) (ScaffoldResult, error) {
	rootRel := opts.Root
	if rootRel == "" {
		rootRel = DefaultGraphRoot
	}
	moduleDir := filepath.Join(opts.RepoRoot, filepath.FromSlash(rootRel), "modules", id)
	readme := filepath.Join(moduleDir, "README.md")
	if _, err := os.Stat(readme); err == nil {
		return ScaffoldResult{}, exitcode.ConflictErrorf("module already exists: %s", relTo(opts.RepoRoot, readme))
	}

	files := map[string]string{
		readme: moduleReadmeContent(id, opts.Name, status, opts.Summary),
	}
	if !opts.Bare {
		for _, coll := range CollectionDirs {
			files[filepath.Join(moduleDir, coll, "README.md")] = collectionReadmeContent(coll, id)
		}
	}
	created, err := writeAll(opts.RepoRoot, files)
	if err != nil {
		return ScaffoldResult{}, err
	}
	return ScaffoldResult{Created: created, Target: relTo(opts.RepoRoot, readme)}, nil
}

// scaffoldArtifact creates one collection artifact under an existing module.
func scaffoldArtifact(opts ScaffoldOptions, id, status string) (ScaffoldResult, error) {
	if opts.Module == "" {
		return ScaffoldResult{}, exitcode.InvalidArgsErrorf("--module is required for `graph new %s`", opts.Kind)
	}
	if opts.Kind == KindRelationship && (opts.From == "" || opts.To == "") {
		return ScaffoldResult{}, exitcode.InvalidArgsError("`graph new relationship` requires --from and --to")
	}

	g, ok, err := Load(opts.RepoRoot, opts.Root)
	if err != nil {
		return ScaffoldResult{}, exitcode.UnexpectedErrorf("loading graph: %v", err)
	}
	if !ok {
		return ScaffoldResult{}, exitcode.NotFoundErrorf("no graph root found; run `graph new module` first")
	}
	mod := g.ModuleByID(opts.Module)
	if mod == nil {
		return ScaffoldResult{}, exitcode.NotFoundErrorf("module %q not found; run `graph new module --name %s` first", opts.Module, opts.Module)
	}

	if opts.Kind == KindRelationship {
		idx := indexArtifacts(g)
		if !idx[opts.From] {
			return ScaffoldResult{}, exitcode.NotFoundErrorf("--from reference %q does not resolve to an existing artifact", opts.From)
		}
		if !idx[opts.To] {
			return ScaffoldResult{}, exitcode.NotFoundErrorf("--to reference %q does not resolve to an existing artifact", opts.To)
		}
	}

	coll := collectionByKind[opts.Kind]
	target := filepath.Join(mod.Root, coll, id+".md")
	if _, err := os.Stat(target); err == nil {
		return ScaffoldResult{}, exitcode.ConflictErrorf("artifact already exists: %s", relTo(opts.RepoRoot, target))
	}
	created, err := writeAll(opts.RepoRoot, map[string]string{
		target: artifactContent(opts, id, status),
	})
	if err != nil {
		return ScaffoldResult{}, err
	}
	return ScaffoldResult{Created: created, Target: relTo(opts.RepoRoot, target)}, nil
}

// indexArtifacts returns the set of qualified IDs of all artifacts in g.
func indexArtifacts(g *Graph) map[string]bool {
	out := map[string]bool{}
	for _, m := range g.Modules {
		for _, a := range m.Artifacts {
			if a.ID != "" {
				out[a.QualifiedID()] = true
			}
		}
	}
	return out
}

// writeAll creates parent directories and writes each file, returning the
// sorted repo-relative paths created.
func writeAll(repoRoot string, files map[string]string) ([]string, error) {
	var created []string
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, exitcode.UnexpectedErrorf("creating directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return nil, exitcode.UnexpectedErrorf("writing file: %v", err)
		}
		created = append(created, relTo(repoRoot, path))
	}
	sortStrings(created)
	return created, nil
}

func relTo(base, abs string) string {
	r, err := filepath.Rel(base, abs)
	if err != nil {
		return abs
	}
	return filepath.ToSlash(r)
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// --- content generators ---

func moduleReadmeContent(id, name, status, summary string) string {
	if name == "" {
		name = id
	}
	sum := summary
	if sum == "" {
		sum = name + " module."
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("kind: module\n")
	fmt.Fprintf(&b, "id: %s\n", id)
	fmt.Fprintf(&b, "name: %s\n", yamlScalar(name))
	fmt.Fprintf(&b, "status: %s\n", status)
	b.WriteString("dependsOn: []\n")
	fmt.Fprintf(&b, "summary: %s\n", yamlScalar(sum))
	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "# Module: %s\n\n", name)
	b.WriteString("## Description\n\n")
	fmt.Fprintf(&b, "%s\n\n", describe(summary, "module", name))
	b.WriteString("## Open Questions\n\n")
	b.WriteString("None at this time.\n")
	return b.String()
}

func artifactContent(opts ScaffoldOptions, id, status string) string {
	name := opts.Name
	if name == "" {
		name = id
	}
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "kind: %s\n", opts.Kind)
	fmt.Fprintf(&b, "id: %s\n", id)
	fmt.Fprintf(&b, "name: %s\n", yamlScalar(name))
	fmt.Fprintf(&b, "status: %s\n", status)
	switch opts.Kind {
	case KindEntity:
		b.WriteString("# model: modelspec://" + opts.Module + "." + capitalizeFirst(id) + "  # optional ModelSpec reference\n")
	case KindRelationship:
		fmt.Fprintf(&b, "from: %s\n", opts.From)
		fmt.Fprintf(&b, "to: %s\n", opts.To)
	case KindCommand, KindEvent:
		if opts.Subject != "" {
			fmt.Fprintf(&b, "subject: %s\n", opts.Subject)
		}
	}
	sum := opts.Summary
	if sum == "" {
		sum = name + "."
	}
	fmt.Fprintf(&b, "summary: %s\n", yamlScalar(sum))
	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "# %s: %s\n\n", kindTitle[opts.Kind], name)
	b.WriteString("## Description\n\n")
	fmt.Fprintf(&b, "%s\n\n", describe(opts.Summary, opts.Kind, name))
	b.WriteString("## Open Questions\n\n")
	b.WriteString("None at this time.\n")
	return b.String()
}

func collectionReadmeContent(coll, module string) string {
	var b strings.Builder
	title := collectionTitle(coll)
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "%s owned by the `%s` module.\n\n", collectionDescription(coll), module)
	b.WriteString("## Artifacts\n\n")
	b.WriteString("- _None yet._\n\n")
	b.WriteString("## Open Questions\n\n")
	b.WriteString("None at this time.\n")
	return b.String()
}

func collectionTitle(coll string) string {
	return strings.ToUpper(coll[:1]) + coll[1:]
}

func collectionDescription(coll string) string {
	switch coll {
	case "entities":
		return "EntitySpec artifacts"
	case "relationships":
		return "RelationshipSpec artifacts"
	case "commands":
		return "CommandSpec artifacts"
	case "events":
		return "EventSpec artifacts"
	default:
		return "ModelSpec sources"
	}
}

func describe(summary, kind, name string) string {
	if summary != "" {
		return summary
	}
	return fmt.Sprintf("TODO: describe the %s %s.", kind, name)
}

// capitalizeFirst upper-cases the first rune (for the entity model: hint's
// PascalCase concept-name placeholder).
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// yamlScalar returns s as a YAML scalar, double-quoting when it could be
// misparsed (contains a character outside a safe bare set).
func yamlScalar(s string) string {
	safe := s != "" && !strings.ContainsAny(s, ":#\"'\n\t{}[]&*!|>%@`") &&
		s[0] != ' ' && s[len(s)-1] != ' '
	if safe {
		return s
	}
	r := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n")
	return "\"" + r.Replace(s) + "\""
}
