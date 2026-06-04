package ideapromote

import (
	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/projectdef"
)

// VerdictMode selects how the seed's ## Consilium Verdict is carried into
// the created Idea.
type VerdictMode string

const (
	// VerdictPointer writes a single-line provenance pointer to the verdict.
	VerdictPointer VerdictMode = "pointer"
	// VerdictFull copies the entire ## Consilium Verdict section.
	VerdictFull VerdictMode = "full"
	// VerdictDrop omits the verdict entirely.
	VerdictDrop VerdictMode = "drop"
)

// DefaultVerdictMode is the carry-forward mode used when neither the
// --verdict flag nor specscore.yaml promote.verdict_carry_forward is set.
const DefaultVerdictMode = VerdictPointer

// ValidateVerdict parses a --verdict flag value into a VerdictMode. An
// empty string is allowed (it signals "unset" — the caller falls back to
// config or the default). Any other unrecognized value returns an
// *exitcode.Error with code 2 (InvalidArgs).
func ValidateVerdict(raw string) (VerdictMode, error) {
	switch raw {
	case "":
		return "", nil
	case string(VerdictPointer), string(VerdictFull), string(VerdictDrop):
		return VerdictMode(raw), nil
	default:
		return "", exitcode.InvalidArgsErrorf(
			"invalid --verdict value %q; valid values: pointer, full, drop", raw)
	}
}

// ResolveVerdictMode picks the effective carry-forward mode: the --verdict
// flag wins when set (non-empty), otherwise the project default from
// specscore.yaml promote.verdict_carry_forward, otherwise the package
// default (pointer). An unrecognized config value is ignored in favor of
// the default — flag validation (exit 2) already guards the user-facing
// path; a malformed config should not crash the verb.
func ResolveVerdictMode(specRoot string, flag VerdictMode) VerdictMode {
	if flag != "" {
		return flag
	}
	if m, ok := readConfigVerdictMode(specRoot); ok {
		return m
	}
	return DefaultVerdictMode
}

// readConfigVerdictMode reads promote.verdict_carry_forward from
// specscore.yaml. The promote block is carried in SpecConfig.Extras
// (inline) as a map. Returns (mode, true) only for a recognized value.
func readConfigVerdictMode(specRoot string) (VerdictMode, bool) {
	cfg, err := projectdef.ReadSpecConfig(specRoot)
	if err != nil {
		return "", false
	}
	promoteRaw, ok := cfg.Extras["promote"]
	if !ok {
		return "", false
	}
	promoteMap, ok := promoteRaw.(map[string]any)
	if !ok {
		return "", false
	}
	v, ok := promoteMap["verdict_carry_forward"].(string)
	if !ok {
		return "", false
	}
	switch VerdictMode(v) {
	case VerdictPointer, VerdictFull, VerdictDrop:
		return VerdictMode(v), true
	default:
		return "", false
	}
}
