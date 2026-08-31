package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

const capabilitySchemaURL = "https://specscore.md/new/cli-capability-delivery.schema.json"

const (
	capabilitySchemaRevision = "b16caad0d0693c407bfb4230b492bce5f2aa8458"
	capabilitySchemaSHA256   = "d573758c9bf41c197fce3f69af7082bf02b5926b3160bd05958c1516d95232a2"
)

type capabilityManifest struct {
	SchemaVersionURL string          `json:"$schema"`
	SchemaVersion    int             `json:"schema_version"`
	Binary           string          `json:"binary"`
	Capabilities     []cliCapability `json:"capabilities"`
}

type cliCapability struct {
	ID          string             `json:"id"`
	FeatureRefs []string           `json:"feature_refs"`
	Since       *string            `json:"since"`
	Deprecated  bool               `json:"deprecated,omitempty"`
	Replacement *string            `json:"replacement,omitempty"`
	Surfaces    capabilitySurfaces `json:"surfaces"`
	Notes       *string            `json:"notes"`
}

type capabilitySurfaces struct {
	Runtime capabilityRuntime `json:"runtime"`
	Help    capabilityHelp    `json:"help"`
	AISkill capabilityAISkill `json:"ai_skill"`
	Tests   capabilityTests   `json:"tests"`
}

type capabilityRuntime struct {
	Status     string              `json:"status"`
	Commands   []capabilityCommand `json:"commands"`
	Limitation *string             `json:"limitation"`
}

type capabilityCommand struct {
	Path  string   `json:"path"`
	Flags []string `json:"flags"`
	Modes []string `json:"modes"`
}

type capabilityHelp struct {
	Status     string             `json:"status"`
	Anchors    []capabilityAnchor `json:"anchors"`
	Limitation *string            `json:"limitation"`
}

type capabilityAnchor struct {
	Command  string   `json:"command"`
	Contains []string `json:"contains"`
}

type capabilityAISkill struct {
	Status     string                    `json:"status"`
	Skills     []capabilitySkillEvidence `json:"skills"`
	Limitation *string                   `json:"limitation"`
}

type capabilitySkillEvidence struct {
	Path     string   `json:"path"`
	Marker   string   `json:"marker"`
	Examples []string `json:"examples"`
}

type capabilityTests struct {
	Status     string                   `json:"status"`
	References []capabilityTestEvidence `json:"references"`
	Limitation *string                  `json:"limitation"`
}

type capabilityTestEvidence struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type capabilitySchemaProvenance struct {
	Repository string `json:"repository"`
	Revision   string `json:"revision"`
	Path       string `json:"path"`
	SchemaID   string `json:"schema_id"`
	SHA256     string `json:"sha256"`
}

func decodeSingleJSON(t *testing.T, name string, b []byte, value any, disallowUnknown bool) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if disallowUnknown {
		dec.DisallowUnknownFields()
	}
	if err := dec.Decode(value); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		t.Fatalf("%s has trailing JSON: %v", name, err)
	}
}

func TestCapabilityManifestValidatesAgainstPinnedCoreSchema(t *testing.T) {
	root := filepath.Join("..", "..")
	schemaPath := filepath.Join(root, "schemas", "cli-capability-delivery.schema.json")
	provenancePath := filepath.Join(root, "schemas", "cli-capability-delivery.schema.provenance.json")
	manifestPath := filepath.Join(root, "spec", "capabilities", "specscore.json")

	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(schemaBytes))
	if digest != capabilitySchemaSHA256 {
		t.Fatalf("pinned capability schema digest drifted: got %s want %s", digest, capabilitySchemaSHA256)
	}
	provenanceBytes, err := os.ReadFile(provenancePath)
	if err != nil {
		t.Fatal(err)
	}
	var provenance capabilitySchemaProvenance
	decodeSingleJSON(t, provenancePath, provenanceBytes, &provenance, true)
	wantProvenance := capabilitySchemaProvenance{
		Repository: "github.com/specscore/specscore",
		Revision:   capabilitySchemaRevision,
		Path:       "new/cli-capability-delivery.schema.json",
		SchemaID:   capabilitySchemaURL,
		SHA256:     capabilitySchemaSHA256,
	}
	if provenance != wantProvenance {
		t.Fatalf("capability schema provenance drifted: got %#v want %#v", provenance, wantProvenance)
	}

	var schemaDocument any
	decodeSingleJSON(t, schemaPath, schemaBytes, &schemaDocument, false)
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource(capabilitySchemaURL, schemaDocument); err != nil {
		t.Fatalf("load pinned capability schema: %v", err)
	}
	compiled, err := compiler.Compile(capabilitySchemaURL)
	if err != nil {
		t.Fatalf("compile pinned capability schema: %v", err)
	}
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifestDocument any
	decodeSingleJSON(t, manifestPath, manifestBytes, &manifestDocument, false)
	if err := compiled.Validate(manifestDocument); err != nil {
		t.Fatalf("capability manifest violates the pinned core schema: %v", err)
	}

	invalid := manifestDocument.(map[string]any)
	invalid["unpublished_extension"] = true
	if err := compiled.Validate(invalid); err == nil {
		t.Fatal("pinned schema accepted an undeclared top-level property")
	}
}

func loadCapabilityManifest(t *testing.T) (string, capabilityManifest) {
	t.Helper()
	root := filepath.Join("..", "..")
	path := filepath.Join(root, "spec", "capabilities", "specscore.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest capabilityManifest
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&manifest); err != nil {
		t.Fatalf("manifest does not conform to the published closed shape: %v", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		t.Fatalf("manifest has trailing JSON: %v", err)
	}
	return root, manifest
}

func TestCapabilityManifestConformsAndEvidenceIsReal(t *testing.T) {
	root, manifest := loadCapabilityManifest(t)
	if manifest.SchemaVersionURL != capabilitySchemaURL || manifest.SchemaVersion != 1 || manifest.Binary != "specscore" {
		t.Fatalf("invalid manifest identity: %#v", manifest)
	}
	validStatus := map[string]bool{"Full": true, "Partial": true, "Planned": true, "Absent": true}
	previous := ""
	seen := map[string]bool{}
	declaredSkillExamples := map[string]map[string]bool{}
	for _, capability := range manifest.Capabilities {
		if !strings.HasPrefix(capability.ID, manifest.Binary+".") || seen[capability.ID] || capability.ID <= previous {
			t.Fatalf("capability IDs must be specscore.-prefixed, unique, and sorted: %q after %q", capability.ID, previous)
		}
		seen[capability.ID], previous = true, capability.ID
		for _, featureRef := range capability.FeatureRefs {
			assertRepoEvidencePath(t, root, capability.ID, featureRef)
		}
		for name, surface := range map[string]struct {
			status string
			count  int
			limit  *string
		}{
			"runtime":  {capability.Surfaces.Runtime.Status, len(capability.Surfaces.Runtime.Commands), capability.Surfaces.Runtime.Limitation},
			"help":     {capability.Surfaces.Help.Status, len(capability.Surfaces.Help.Anchors), capability.Surfaces.Help.Limitation},
			"ai_skill": {capability.Surfaces.AISkill.Status, len(capability.Surfaces.AISkill.Skills), capability.Surfaces.AISkill.Limitation},
			"tests":    {capability.Surfaces.Tests.Status, len(capability.Surfaces.Tests.References), capability.Surfaces.Tests.Limitation},
		} {
			if !validStatus[surface.status] {
				t.Errorf("%s %s has invalid status %q", capability.ID, name, surface.status)
			}
			usable := surface.status == "Full" || surface.status == "Partial"
			if usable && surface.count == 0 {
				t.Errorf("%s %s claims %s without evidence", capability.ID, name, surface.status)
			}
			if !usable && surface.count != 0 {
				t.Errorf("%s %s is unusable but advertises evidence", capability.ID, name)
			}
			if surface.status == "Full" && surface.limit != nil {
				t.Errorf("%s %s Full must have null limitation", capability.ID, name)
			}
			if surface.status != "Full" && (surface.limit == nil || strings.TrimSpace(*surface.limit) == "") {
				t.Errorf("%s %s non-Full must explain its limitation", capability.ID, name)
			}
		}
		if capability.Surfaces.Runtime.Status == "Planned" || capability.Surfaces.Runtime.Status == "Absent" {
			for name, status := range map[string]string{"help": capability.Surfaces.Help.Status, "ai_skill": capability.Surfaces.AISkill.Status} {
				if status == "Full" || status == "Partial" {
					t.Errorf("%s %s cannot advertise usable evidence when runtime is unavailable", capability.ID, name)
				}
			}
		}
		for _, reference := range capability.Surfaces.Tests.References {
			assertRepoEvidencePath(t, root, capability.ID, reference.Path)
			b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(reference.Path)))
			if err != nil {
				t.Fatal(err)
			}
			pattern := regexp.MustCompile(`(?m)^func\s+` + regexp.QuoteMeta(reference.Name) + `\s*\(`)
			if !pattern.Match(b) {
				t.Errorf("%s cites missing executable test symbol %s in %s", capability.ID, reference.Name, reference.Path)
			}
		}
		for _, skill := range capability.Surfaces.AISkill.Skills {
			assertRepoEvidencePath(t, root, capability.ID, skill.Path)
			b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(skill.Path)))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(b, []byte(skill.Marker)) {
				t.Errorf("%s cites stale skill marker %q", capability.ID, skill.Marker)
			}
			validateSkillPackage(t, root, skill.Path, b)
			if declaredSkillExamples[skill.Path] == nil {
				declaredSkillExamples[skill.Path] = map[string]bool{}
			}
			for _, example := range skill.Examples {
				declaredSkillExamples[skill.Path][example] = true
				if !bytes.Contains(b, []byte(example)) {
					t.Errorf("%s cites stale skill example %q", capability.ID, example)
					continue
				}
				if err := parseCapabilityExample(example); err != nil {
					t.Errorf("%s skill example does not parse: %q: %v", capability.ID, example, err)
				}
			}
		}
	}
	for skillPath, declared := range declaredSkillExamples {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(skillPath)))
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			example := strings.TrimSpace(line)
			if strings.HasPrefix(example, "specscore ") && !declared[example] {
				t.Errorf("skill example is not represented in capability evidence: %q", example)
			}
		}
	}
}

func validateSkillPackage(t *testing.T, root, skillPath string, skillBytes []byte) {
	t.Helper()
	parts := strings.SplitN(string(skillBytes), "---", 3)
	if len(parts) != 3 || strings.TrimSpace(parts[0]) != "" {
		t.Fatalf("%s lacks YAML frontmatter", skillPath)
	}
	var frontmatter map[string]string
	if err := yaml.Unmarshal([]byte(parts[1]), &frontmatter); err != nil {
		t.Fatalf("%s frontmatter: %v", skillPath, err)
	}
	wantName := filepath.Base(filepath.Dir(filepath.Join(root, filepath.FromSlash(skillPath))))
	if len(frontmatter) != 2 || frontmatter["name"] != wantName || strings.TrimSpace(frontmatter["description"]) == "" || strings.TrimSpace(parts[2]) == "" {
		t.Fatalf("%s must contain only matching name and nonempty description frontmatter plus instructions", skillPath)
	}
	metadataPath := filepath.Join(filepath.Dir(filepath.Join(root, filepath.FromSlash(skillPath))), "agents", "openai.yaml")
	metadataBytes, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("%s install metadata: %v", skillPath, err)
	}
	var node yaml.Node
	if err := yaml.Unmarshal(metadataBytes, &node); err != nil {
		t.Fatalf("%s metadata: %v", skillPath, err)
	}
	if len(node.Content) != 1 || len(node.Content[0].Content) != 2 || node.Content[0].Content[0].Value != "interface" {
		t.Fatalf("%s metadata must contain only interface", skillPath)
	}
	interfaceNode := node.Content[0].Content[1]
	values := map[string]string{}
	for i := 0; i+1 < len(interfaceNode.Content); i += 2 {
		key, value := interfaceNode.Content[i], interfaceNode.Content[i+1]
		if value.Style != yaml.DoubleQuotedStyle {
			t.Errorf("%s interface.%s must be double-quoted", skillPath, key.Value)
		}
		values[key.Value] = value.Value
	}
	if len(values) != 3 || values["display_name"] == "" || utf8.RuneCountInString(values["short_description"]) < 25 || utf8.RuneCountInString(values["short_description"]) > 64 || !strings.Contains(values["default_prompt"], "$"+wantName) || !strings.HasSuffix(values["default_prompt"], ".") {
		t.Fatalf("%s has invalid deterministic interface metadata: %#v", skillPath, values)
	}
}

func assertRepoEvidencePath(t *testing.T, root, capabilityID, path string) {
	t.Helper()
	if path == "" || filepath.IsAbs(path) || strings.Contains(path, "\\") || filepath.ToSlash(filepath.Clean(path)) != path || path == "." || strings.HasPrefix(path, "../") {
		t.Fatalf("%s has non-portable evidence path %q", capabilityID, path)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
		t.Fatalf("%s evidence path does not resolve: %s: %v", capabilityID, path, err)
	}
}

func TestCapabilityManifestCoversPublicCommandTree(t *testing.T) {
	_, manifest := loadCapabilityManifest(t)
	root, _ := newRootCommand()
	want := publicCapabilityCommandTree(root)
	got := map[string]bool{}
	for _, capability := range manifest.Capabilities {
		for _, command := range capability.Surfaces.Runtime.Commands {
			runtimeFlags, exists := want[command.Path]
			if !exists {
				t.Errorf("%s advertises missing public command: %s", capability.ID, command.Path)
				continue
			}
			if strings.Join(runtimeFlags, "\x00") != strings.Join(command.Flags, "\x00") {
				t.Errorf("%s %s flags drifted: manifest=%v runtime=%v", capability.ID, command.Path, command.Flags, runtimeFlags)
			}
			got[command.Path] = true
		}
	}
	for path := range want {
		if !got[path] {
			t.Errorf("public command is missing from capability manifest: %s", path)
		}
	}
}

func TestCapabilityManifestCanonicalJSON(t *testing.T) {
	root, manifest := loadCapabilityManifest(t)
	want, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	path := filepath.Join(root, "spec", "capabilities", "specscore.json")
	if os.Getenv("UPDATE_CAPABILITY_MANIFEST") == "1" {
		if err := os.WriteFile(path, want, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("capability manifest JSON is not canonical; run UPDATE_CAPABILITY_MANIFEST=1 go test ./internal/cli -run TestCapabilityManifestCanonicalJSON")
	}
}

func publicCapabilityCommandTree(root *cobra.Command) map[string][]string {
	out := map[string][]string{"specscore": commandFlagNames(root, true)}
	var walk func(*cobra.Command)
	walk = func(command *cobra.Command) {
		var visible []*cobra.Command
		for _, child := range command.Commands() {
			if !child.Hidden {
				visible = append(visible, child)
			}
		}
		if len(visible) == 0 && command != root {
			out[command.CommandPath()] = commandFlagNames(command, false)
			return
		}
		for _, child := range visible {
			walk(child)
		}
	}
	walk(root)
	return out
}

func commandFlagNames(command *cobra.Command, root bool) []string {
	seen := map[string]bool{}
	var flags []string
	visit := func(flag *pflag.Flag) {
		if flag.Name == "help" || seen[flag.Name] {
			return
		}
		seen[flag.Name] = true
		flags = append(flags, "--"+flag.Name)
	}
	command.NonInheritedFlags().VisitAll(visit)
	if root {
		command.PersistentFlags().VisitAll(visit)
	}
	sort.Strings(flags)
	return flags
}

func TestCapabilityManifestHelpAnchorsAndGeneratedMatrix(t *testing.T) {
	root, manifest := loadCapabilityManifest(t)
	for _, capability := range manifest.Capabilities {
		for _, anchor := range capability.Surfaces.Help.Anchors {
			parts := strings.Fields(anchor.Command)
			if len(parts) == 0 || parts[0] != "specscore" {
				t.Fatalf("%s has invalid help command %q", capability.ID, anchor.Command)
			}
			command, _ := newRootCommand()
			var stdout, stderr bytes.Buffer
			command.SetOut(&stdout)
			command.SetErr(&stderr)
			command.SetArgs(parts[1:])
			if err := command.Execute(); err != nil {
				t.Fatalf("%s help command failed: %s: %v", capability.ID, anchor.Command, err)
			}
			text := stdout.String() + stderr.String()
			for _, fragment := range anchor.Contains {
				if !strings.Contains(text, fragment) {
					t.Errorf("%s help anchor %q lacks %q", capability.ID, anchor.Command, fragment)
				}
			}
		}
	}
	want := renderCapabilityMatrix(manifest)
	path := filepath.Join(root, "docs", "capabilities", "specscore.md")
	if os.Getenv("UPDATE_CAPABILITY_MATRIX") == "1" {
		if err := os.WriteFile(path, want, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("generated capability matrix drifted; run UPDATE_CAPABILITY_MATRIX=1 go test ./internal/cli -run TestCapabilityManifestHelpAnchorsAndGeneratedMatrix")
	}
}

func parseCapabilityExample(example string) error {
	parts, err := splitShellWords(example)
	if err != nil {
		return err
	}
	if len(parts) < 2 || parts[0] != "specscore" {
		return fmt.Errorf("example must begin with specscore")
	}
	root, _ := newRootCommand()
	command, _, err := root.Find(parts[1:])
	if err != nil {
		return err
	}
	commandWords := len(strings.Fields(command.CommandPath()))
	remaining := parts[commandWords:]
	if err := command.ParseFlags(remaining); err != nil {
		return err
	}
	if command.Args != nil {
		return command.Args(command, command.Flags().Args())
	}
	return nil
}

// splitShellWords parses the deliberately small, portable shell subset used
// by skill examples. It preserves quoted JSON/string values without executing
// substitutions or consulting a platform shell.
func splitShellWords(input string) ([]string, error) {
	var words []string
	var current strings.Builder
	var quote rune
	escaped := false
	started := false
	flush := func() {
		if started {
			words = append(words, current.String())
			current.Reset()
			started = false
		}
	}
	for _, r := range input {
		if escaped {
			current.WriteRune(r)
			started = true
			escaped = false
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
				started = true
			} else if r == '\\' && quote == '"' {
				escaped = true
			} else {
				current.WriteRune(r)
				started = true
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
			started = true
		case '\\':
			escaped = true
			started = true
		case ' ', '\t', '\r', '\n':
			flush()
		default:
			current.WriteRune(r)
			started = true
		}
	}
	if escaped || quote != 0 {
		return nil, fmt.Errorf("unterminated escape or quote")
	}
	flush()
	return words, nil
}

func renderCapabilityMatrix(manifest capabilityManifest) []byte {
	var out strings.Builder
	out.WriteString("# SpecScore CLI capability delivery\n\nGenerated from `spec/capabilities/specscore.json`; do not edit by hand.\n\n")
	out.WriteString("| Capability | Runtime | Commands/Flags | Help | AI Skill | Tests | Limitations |\n")
	out.WriteString("|---|---|---|---|---|---|---|\n")
	for _, capability := range manifest.Capabilities {
		var commands []string
		for _, command := range capability.Surfaces.Runtime.Commands {
			item := "`" + command.Path + "`"
			if len(command.Flags) > 0 {
				item += " " + strings.Join(command.Flags, ", ")
			}
			commands = append(commands, item)
		}
		var limitations []string
		for name, limit := range map[string]*string{"Runtime": capability.Surfaces.Runtime.Limitation, "Help": capability.Surfaces.Help.Limitation, "AI Skill": capability.Surfaces.AISkill.Limitation, "Tests": capability.Surfaces.Tests.Limitation} {
			if limit != nil {
				limitations = append(limitations, name+": "+*limit)
			}
		}
		sort.Strings(limitations)
		fmt.Fprintf(&out, "| `%s` | %s | %s | %s | %s | %s | %s |\n", capability.ID, capability.Surfaces.Runtime.Status, strings.Join(commands, "<br>"), capability.Surfaces.Help.Status, capability.Surfaces.AISkill.Status, capability.Surfaces.Tests.Status, strings.Join(limitations, "<br>"))
	}
	return []byte(out.String())
}
