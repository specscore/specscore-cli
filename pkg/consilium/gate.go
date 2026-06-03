package consilium

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// gateConfigFileName is the per-project SpecScore config filename, mirrored
// from pkg/event/config.go so error messages here use the literal
// "specscore.yaml" string without taking a cross-package dependency.
const gateConfigFileName = "specscore.yaml"

// confidenceGateEnum / ceilingGateEnum are the closed sets of accepted values
// for the enum knobs, kept in stable order so error messages list them
// deterministically. The confidence-style knobs accept the full low|medium|high
// scale; the ceiling-style knobs accept only low|medium.
var (
	confidenceGateEnum = []string{"low", "medium", "high"}
	ceilingGateEnum    = []string{"low", "medium"}
)

// GateKnobs is the resolved set of the six consilium gate knobs the arbiter
// applies (REQ:gate-knob-config-schema). Every field is populated: a value
// supplied by config, otherwise the strict baseline.
type GateKnobs struct {
	AdversaryVetoConfidence string // high | medium | low
	CostCeiling             string // low | medium
	ComplexityCeiling       string // low | medium
	MinMedianConfidence     string // low | medium | high
	RequireAllBuilders      bool
	RequireAllCustomers     bool
}

// StrictBaseline returns the strict-baseline gate knobs applied knob-by-knob
// when a knob is absent from config.
func StrictBaseline() GateKnobs {
	return GateKnobs{
		AdversaryVetoConfidence: "high",
		CostCeiling:             "medium",
		ComplexityCeiling:       "medium",
		MinMedianConfidence:     "medium",
		RequireAllBuilders:      true,
		RequireAllCustomers:     true,
	}
}

// LoadGate reads <projectRoot>/specscore.yaml and resolves the gate knobs:
// the strict baseline merged knob-by-knob with any `consilium.gate` mapping.
// When the file does not exist, or has no `consilium.gate` mapping, the strict
// baseline is returned. An unknown knob key or an out-of-enum value fails with
// an error naming the key path (e.g. `consilium.gate.cost_ceiling`), the
// offending value, and the accepted enum.
func LoadGate(projectRoot string) (GateKnobs, error) {
	knobs := StrictBaseline()

	path := filepath.Join(projectRoot, gateConfigFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return knobs, nil
		}
		return GateKnobs{}, fmt.Errorf("read %s: %w", gateConfigFileName, err)
	}

	node, present, err := extractGateNode(data)
	if err != nil {
		return GateKnobs{}, fmt.Errorf("%s: parse: %w", gateConfigFileName, err)
	}
	if !present {
		return knobs, nil
	}

	if err := mergeGateNode(&knobs, node, "consilium.gate"); err != nil {
		return GateKnobs{}, fmt.Errorf("%s: %w", gateConfigFileName, err)
	}
	return knobs, nil
}

// ResolveGate resolves the effective gate knobs the arbiter applies: the
// specscore.yaml-resolved baseline (LoadGate) merged knob-by-knob with an
// optional `--gate` override file. The override file is a YAML document with a
// top-level `gate:` mapping of knobs (the shape `consilium config --print-gate`
// emits and accepts verbatim). An empty overridePath skips the override step.
// Override knobs are validated with the same enum rules; violations name the
// override key path (e.g. `gate.cost_ceiling`).
func ResolveGate(projectRoot, overridePath string) (GateKnobs, error) {
	knobs, err := LoadGate(projectRoot)
	if err != nil {
		return GateKnobs{}, err
	}
	if overridePath == "" {
		return knobs, nil
	}

	data, err := os.ReadFile(overridePath)
	if err != nil {
		return GateKnobs{}, fmt.Errorf("read gate override %s: %w", overridePath, err)
	}

	node, present, err := extractTopLevelGateNode(data)
	if err != nil {
		return GateKnobs{}, fmt.Errorf("%s: parse: %w", overridePath, err)
	}
	if !present {
		return knobs, nil
	}
	if err := mergeGateNode(&knobs, node, "gate"); err != nil {
		return GateKnobs{}, fmt.Errorf("%s: %w", overridePath, err)
	}
	return knobs, nil
}

// extractGateNode walks the top-level mapping and returns the raw yaml.Node for
// `consilium.gate` when present. present=false signals the key was omitted.
func extractGateNode(data []byte) (node *yaml.Node, present bool, err error) {
	root, err := rootMapping(data)
	if err != nil || root == nil {
		return nil, false, err
	}
	consilium := mappingValue(root, "consilium")
	if consilium == nil || consilium.Kind != yaml.MappingNode {
		return nil, false, nil
	}
	gate := mappingValue(consilium, "gate")
	if gate == nil {
		return nil, false, nil
	}
	return gate, true, nil
}

// extractTopLevelGateNode returns the raw yaml.Node for a top-level `gate:`
// mapping (the `--gate` override-file shape). present=false when omitted.
func extractTopLevelGateNode(data []byte) (node *yaml.Node, present bool, err error) {
	root, err := rootMapping(data)
	if err != nil || root == nil {
		return nil, false, err
	}
	gate := mappingValue(root, "gate")
	if gate == nil {
		return nil, false, nil
	}
	return gate, true, nil
}

// rootMapping unmarshals data and returns the document's root mapping node, or
// nil when the document is empty or not a mapping.
func rootMapping(data []byte) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, nil
	}
	return root, nil
}

// mappingValue returns the value node for key in a mapping node, or nil when
// the key is absent.
func mappingValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// mergeGateNode merges the knob entries of a `gate:` mapping node into knobs,
// validating each against its enum. keyPrefix is the dotted path of the gate
// mapping (e.g. `consilium.gate` or `gate`) so errors name the full key path.
// A null/empty mapping is a no-op. An unknown knob key or an out-of-enum value
// fails.
func mergeGateNode(knobs *GateKnobs, gate *yaml.Node, keyPrefix string) error {
	if gate.Kind == yaml.ScalarNode && gate.Tag == "!!null" {
		return nil
	}
	if gate.Kind != yaml.MappingNode {
		return fmt.Errorf("%s: must be a mapping", keyPrefix)
	}
	for i := 0; i+1 < len(gate.Content); i += 2 {
		key := gate.Content[i].Value
		val := gate.Content[i+1]
		path := keyPrefix + "." + key
		if err := applyGateKnob(knobs, key, val, path); err != nil {
			return err
		}
	}
	return nil
}

// applyGateKnob validates and applies a single knob value onto knobs. Unknown
// keys and out-of-enum/bool values produce an error naming path and accepted
// values.
func applyGateKnob(knobs *GateKnobs, key string, val *yaml.Node, path string) error {
	switch key {
	case "adversary_veto_confidence":
		return setEnumKnob(&knobs.AdversaryVetoConfidence, val, confidenceGateEnum, path)
	case "cost_ceiling":
		return setEnumKnob(&knobs.CostCeiling, val, ceilingGateEnum, path)
	case "complexity_ceiling":
		return setEnumKnob(&knobs.ComplexityCeiling, val, ceilingGateEnum, path)
	case "min_median_confidence":
		return setEnumKnob(&knobs.MinMedianConfidence, val, confidenceGateEnum, path)
	case "require_all_builders":
		return setBoolKnob(&knobs.RequireAllBuilders, val, path)
	case "require_all_customers":
		return setBoolKnob(&knobs.RequireAllCustomers, val, path)
	default:
		return fmt.Errorf("%s: unknown gate knob", path)
	}
}

// setEnumKnob validates a scalar value against enum and assigns it to dst.
func setEnumKnob(dst *string, val *yaml.Node, enum []string, path string) error {
	if val.Kind != yaml.ScalarNode {
		return fmt.Errorf("%s: must be one of %s", path, joinEnum(enum))
	}
	for _, e := range enum {
		if val.Value == e {
			*dst = val.Value
			return nil
		}
	}
	return fmt.Errorf("%s: invalid value %q; must be one of %s", path, val.Value, joinEnum(enum))
}

// setBoolKnob validates a scalar value as a YAML bool and assigns it to dst.
func setBoolKnob(dst *bool, val *yaml.Node, path string) error {
	var b bool
	if val.Kind != yaml.ScalarNode || val.Decode(&b) != nil {
		return fmt.Errorf("%s: must be one of true, false", path)
	}
	*dst = b
	return nil
}

// joinEnum renders an enum slice as a comma-separated string in declared order.
func joinEnum(enum []string) string {
	s := ""
	for i, e := range enum {
		if i > 0 {
			s += ", "
		}
		s += e
	}
	return s
}
