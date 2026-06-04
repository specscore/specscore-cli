package ideapromote

import "github.com/specscore/specscore-cli/pkg/exitcode"

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
