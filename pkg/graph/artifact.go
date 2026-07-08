package graph

import (
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// InputItem is one entry of a CommandSpec `inputs:` list. Each item carries a
// name plus exactly one of ref: (qualified graph ref) or model: (modelspec://
// ref). Presence flags and ExtraKeys let the inputs-shape rule report items
// with both, neither, or embedded structure.
type InputItem struct {
	Name      string
	Ref       string
	Model     string
	HasName   bool
	HasRef    bool
	HasModel  bool
	ExtraKeys []string
	Line      int
}

// MetaEntry is one key of a RelationshipSpec `metadata:` flat map. Scalar
// records whether the value is a scalar (string/number/bool); when false the
// value was a nested map or list, which the metadata-shape rule rejects.
type MetaEntry struct {
	Key    string
	Value  string // scalar text when Scalar is true
	Scalar bool
	Line   int
}

// ParticipantItem is one entry of an EventSpec `participants:` list — either a
// scalar qualified reference or a role-labeled {ref, role} map (decision 0012).
type ParticipantItem struct {
	Ref  string
	Role string
	Line int
}

// RoleIssue is a shape violation found while decoding a role-labeled endpoint
// or participant (decision 0012); the graph-role-labels rule reports them.
type RoleIssue struct {
	Line    int
	Message string
}

// Artifact is a parsed GraphSpec artifact (module README or a collection
// artifact). Parse is resilient: a non-nil Artifact is returned for every
// readable file. FMError is set when the file has no leading frontmatter block
// or the block is malformed YAML; callers treat that as a hard violation.
type Artifact struct {
	Path          string // absolute path
	Stem          string // filename stem (README for modules, <id> otherwise)
	Module        string // owning module id, derived from placement
	CollectionDir string // "entities"/"relationships"/... or "" for a module README

	// Raw frontmatter presence — a key being present (even if empty) matters
	// for several rules (owner, fields/properties, sources, lifecycle).
	HasFrontmatter bool
	FMError        string
	keyLines       map[string]int
	presentKeys    map[string]bool

	// Decoded scalar fields.
	Kind    string
	ID      string
	Name    string
	Status  string
	Summary string

	// module
	DependsOn []string

	// entity
	Model                  string
	LifecycleStates        []string
	LifecycleStatesPresent bool

	// relationship. From/To always carry the endpoint reference; FromRole/
	// ToRole carry the optional decision-0012 role labels of the map form.
	From        string
	FromRole    string
	To          string
	ToRole      string
	Cardinality string
	Metadata    []MetaEntry

	// command / event. Participants carries the references (scalar view);
	// ParticipantItems carries the role-labeled per-item view (decision 0012).
	Subject          string
	Actors           []string
	Participants     []string
	ParticipantItems []ParticipantItem
	Inputs           []InputItem
	inputsMalformed  bool
	PossibleEvents   []string
	Sources          []string

	// RoleIssues collects endpoint/participant shape violations (decision
	// 0012) for the graph-role-labels rule.
	RoleIssues []RoleIssue
}

// KeyLine returns the 1-based file line of the named top-level frontmatter key,
// or 0 when the key is absent or its line is unknown.
func (a *Artifact) KeyLine(key string) int {
	if a.keyLines == nil {
		return 0
	}
	return a.keyLines[key]
}

// HasKey reports whether the frontmatter declared the given top-level key.
func (a *Artifact) HasKey(key string) bool {
	return a.presentKeys[key]
}

// QualifiedID returns the computed <module>.<id> form (decision 0005). When the
// artifact has no owning module the bare id is returned.
func (a *Artifact) QualifiedID() string {
	if a.Module == "" || a.ID == "" {
		return a.ID
	}
	return a.Module + "." + a.ID
}

// ParseArtifact reads a GraphSpec artifact file and decodes its frontmatter.
// module and collectionDir are supplied by discovery (placement-derived).
func ParseArtifact(path, module, collectionDir string) (*Artifact, error) {
	data, err := readFileFn(path)
	if err != nil {
		return nil, err
	}
	a := &Artifact{
		Path:          path,
		Stem:          strings.TrimSuffix(filepath.Base(path), ".md"),
		Module:        module,
		CollectionDir: collectionDir,
		keyLines:      map[string]int{},
		presentKeys:   map[string]bool{},
	}
	a.decode(string(data))
	return a, nil
}

// decode extracts the leading YAML frontmatter block and fills the artifact.
func (a *Artifact) decode(content string) {
	lines := strings.Split(content, "\n")
	// The frontmatter block must be the first non-empty content of the file.
	first := -1
	for i, l := range lines {
		if strings.TrimSpace(l) != "" {
			first = i
			break
		}
	}
	if first < 0 || strings.TrimSpace(lines[first]) != "---" {
		a.FMError = "artifact must begin with a YAML frontmatter block delimited by `---`"
		return
	}
	end := -1
	for j := first + 1; j < len(lines); j++ {
		if strings.TrimSpace(lines[j]) == "---" {
			end = j
			break
		}
	}
	if end < 0 {
		a.FMError = "unterminated frontmatter block (missing closing `---`)"
		return
	}
	raw := strings.Join(lines[first+1:end], "\n")
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &doc); err != nil {
		a.FMError = "frontmatter is not valid YAML: " + err.Error()
		return
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		a.FMError = "frontmatter block is empty"
		return
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		a.FMError = "frontmatter must be a YAML mapping"
		return
	}
	a.HasFrontmatter = true
	// lineOffset maps a node's frontmatter-relative line to an absolute file
	// line: the content starts at file line (first+2) (1-based).
	lineOffset := first + 1
	a.fill(root, lineOffset)
}

// fill walks the frontmatter mapping and populates typed fields, presence, and
// line numbers.
func (a *Artifact) fill(root *yaml.Node, lineOffset int) {
	for i := 0; i+1 < len(root.Content); i += 2 {
		key := root.Content[i]
		val := root.Content[i+1]
		if key.Kind != yaml.ScalarNode {
			continue
		}
		name := key.Value
		a.presentKeys[name] = true
		a.keyLines[name] = lineOffset + key.Line
		switch name {
		case "kind":
			a.Kind = scalar(val)
		case "id":
			a.ID = scalar(val)
		case "name":
			a.Name = scalar(val)
		case "status":
			a.Status = scalar(val)
		case "summary":
			a.Summary = scalar(val)
		case "model":
			a.Model = scalar(val)
		case "from":
			a.From, a.FromRole = a.fillEndpoint(val, "from", lineOffset)
		case "to":
			a.To, a.ToRole = a.fillEndpoint(val, "to", lineOffset)
		case "cardinality":
			a.Cardinality = scalar(val)
		case "subject":
			a.Subject = scalar(val)
		case "dependsOn":
			a.DependsOn = scalarList(val)
		case "actors":
			a.Actors = scalarList(val)
		case "participants":
			a.fillParticipants(val, lineOffset)
		case "possibleEvents":
			a.PossibleEvents = scalarList(val)
		case "sources":
			a.Sources = scalarList(val)
		case "lifecycle":
			a.fillLifecycle(val, lineOffset)
		case "metadata":
			a.fillMetadata(val, lineOffset)
		case "inputs":
			a.fillInputs(val, lineOffset)
		}
	}
}

// fillEndpoint decodes a relationship endpoint (decision 0012): a scalar
// qualified reference, or a {ref, role} map. Shape violations are recorded as
// RoleIssues for the graph-role-labels rule.
func (a *Artifact) fillEndpoint(val *yaml.Node, field string, lineOffset int) (ref, role string) {
	if val.Kind == yaml.ScalarNode {
		return val.Value, ""
	}
	if val.Kind != yaml.MappingNode {
		a.roleIssue(lineOffset+val.Line, field+" must be a qualified reference or a {ref, role} map")
		return "", ""
	}
	return a.decodeRefRoleMap(val, field, lineOffset)
}

// fillParticipants decodes the participants list: each item is a scalar
// qualified reference or a {ref, role} map (decision 0012). Participants (the
// scalar ref view) and ParticipantItems (the role-labeled view) stay in sync.
func (a *Artifact) fillParticipants(val *yaml.Node, lineOffset int) {
	if val.Kind != yaml.SequenceNode {
		return
	}
	for _, item := range val.Content {
		line := lineOffset + item.Line
		switch item.Kind {
		case yaml.ScalarNode:
			a.Participants = append(a.Participants, item.Value)
			a.ParticipantItems = append(a.ParticipantItems, ParticipantItem{Ref: item.Value, Line: line})
		case yaml.MappingNode:
			ref, role := a.decodeRefRoleMap(item, "participants item", lineOffset)
			if ref != "" {
				a.Participants = append(a.Participants, ref)
			}
			a.ParticipantItems = append(a.ParticipantItems, ParticipantItem{Ref: ref, Role: role, Line: line})
		default:
			a.roleIssue(line, "participants item must be a qualified reference or a {ref, role} map")
		}
	}
}

// decodeRefRoleMap decodes a decision-0012 {ref, role} map. Both keys are
// required (a map without a role says nothing the scalar form doesn't); other
// keys are rejected; the role must be a bare kebab-case token.
func (a *Artifact) decodeRefRoleMap(node *yaml.Node, field string, lineOffset int) (ref, role string) {
	line := lineOffset + node.Line
	hasRef, hasRole := false, false
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i]
		v := node.Content[i+1]
		switch k.Value {
		case "ref":
			hasRef = true
			ref = scalar(v)
		case "role":
			hasRole = true
			role = scalar(v)
		default:
			a.roleIssue(lineOffset+k.Line, field+" map has unexpected key \""+k.Value+"\" (only ref and role are allowed)")
		}
	}
	if !hasRef || ref == "" {
		a.roleIssue(line, field+" map must carry a non-empty `ref`")
	}
	if !hasRole || role == "" {
		a.roleIssue(line, field+" map must carry a non-empty `role` (use the scalar form when no role label is needed)")
	} else if !IsKebab(role) {
		a.roleIssue(line, field+" role \""+role+"\" must be a bare lowercase kebab-case token")
	}
	return ref, role
}

// roleIssue records one decision-0012 shape violation.
func (a *Artifact) roleIssue(line int, msg string) {
	a.RoleIssues = append(a.RoleIssues, RoleIssue{Line: line, Message: msg})
}

// fillLifecycle records whether lifecycle.states is present and its values.
func (a *Artifact) fillLifecycle(val *yaml.Node, lineOffset int) {
	if val.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(val.Content); i += 2 {
		if val.Content[i].Value != "states" {
			continue
		}
		a.LifecycleStatesPresent = true
		a.keyLines["lifecycle.states"] = lineOffset + val.Content[i].Line
		a.LifecycleStates = scalarList(val.Content[i+1])
	}
}

// fillMetadata records each metadata entry, flagging non-scalar values.
func (a *Artifact) fillMetadata(val *yaml.Node, lineOffset int) {
	if val.Kind != yaml.MappingNode {
		// A non-mapping metadata block is recorded as one non-scalar entry so
		// the metadata-shape rule reports it.
		a.Metadata = append(a.Metadata, MetaEntry{Key: "", Scalar: false, Line: lineOffset + val.Line})
		return
	}
	for i := 0; i+1 < len(val.Content); i += 2 {
		k := val.Content[i]
		v := val.Content[i+1]
		e := MetaEntry{Key: k.Value, Line: lineOffset + k.Line}
		if v.Kind == yaml.ScalarNode {
			e.Scalar = true
			e.Value = v.Value
		}
		a.Metadata = append(a.Metadata, e)
	}
}

// fillInputs decodes the command inputs list; non-list or non-mapping items
// set inputsMalformed so the inputs-shape rule can report the container.
func (a *Artifact) fillInputs(val *yaml.Node, lineOffset int) {
	if val.Kind != yaml.SequenceNode {
		a.inputsMalformed = true
		return
	}
	for _, item := range val.Content {
		if item.Kind != yaml.MappingNode {
			a.inputsMalformed = true
			continue
		}
		in := InputItem{Line: lineOffset + item.Line}
		for i := 0; i+1 < len(item.Content); i += 2 {
			ik := item.Content[i]
			iv := item.Content[i+1]
			switch ik.Value {
			case "name":
				in.HasName = true
				in.Name = scalar(iv)
			case "ref":
				in.HasRef = true
				in.Ref = scalar(iv)
			case "model":
				in.HasModel = true
				in.Model = scalar(iv)
			default:
				in.ExtraKeys = append(in.ExtraKeys, ik.Value)
			}
		}
		a.Inputs = append(a.Inputs, in)
	}
}

// scalar returns a scalar node's value, or "" for non-scalar nodes.
func scalar(n *yaml.Node) string {
	if n.Kind == yaml.ScalarNode {
		return n.Value
	}
	return ""
}

// scalarList returns the scalar values of a sequence node. A non-sequence node
// yields nil.
func scalarList(n *yaml.Node) []string {
	if n.Kind != yaml.SequenceNode {
		return nil
	}
	out := make([]string, 0, len(n.Content))
	for _, c := range n.Content {
		if c.Kind == yaml.ScalarNode {
			out = append(out, c.Value)
		}
	}
	return out
}
