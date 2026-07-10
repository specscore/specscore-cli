package runner

import (
	"fmt"
	"regexp"

	"github.com/specscore/specscore-cli/internal/rehearse/blocks"
)

// Bag is one scenario run's context bag (REQ: context-bag): an ordered map of
// string variables. Steps feed it via their StepResult captures; consumption
// is per block class — bash/sql/dtql get {{name}} textual interpolation
// before dispatch, hurl-derived blocks receive the ordered Snapshot through
// StepCtx.Vars and hand it to Hurl as --variable flags.
type Bag struct {
	names  []string
	values map[string]string
}

// NewBag returns an empty context bag.
func NewBag() *Bag {
	return &Bag{values: map[string]string{}}
}

// Merge folds a step's captures into the bag: new names append in capture
// order, existing names update in place (first-insertion order is kept).
func (b *Bag) Merge(captures []blocks.Capture) {
	for _, c := range captures {
		if _, exists := b.values[c.Name]; !exists {
			b.names = append(b.names, c.Name)
		}
		b.values[c.Name] = c.Value
	}
}

// Snapshot returns the bag as ordered name/value pairs — the shape
// hurl-derived executors turn into `--variable name=value` flags.
func (b *Bag) Snapshot() []blocks.Capture {
	out := make([]blocks.Capture, 0, len(b.names))
	for _, name := range b.names {
		out = append(out, blocks.Capture{Name: name, Value: b.values[name]})
	}
	return out
}

// Map returns a copy of the bag's final state for the JSON report
// (REQ: run-report). Never nil.
func (b *Bag) Map() map[string]string {
	out := make(map[string]string, len(b.values))
	for name, value := range b.values {
		out[name] = value
	}
	return out
}

// placeholder matches one {{name}} variable reference. The name shape mirrors
// Hurl's variable names so the two consumption classes agree on what a
// reference looks like; other brace runs pass through untouched.
var placeholder = regexp.MustCompile(`\{\{([A-Za-z_][A-Za-z0-9_-]*)\}\}`)

// Interpolate textually replaces every {{name}} placeholder with the bag's
// value for name. An unknown variable is an error naming it — the step fails
// instead of running with a literal placeholder (REQ: context-bag).
func (b *Bag) Interpolate(text string) (string, error) {
	var unknown string
	out := placeholder.ReplaceAllStringFunc(text, func(match string) string {
		name := match[2 : len(match)-2]
		value, ok := b.values[name]
		if !ok {
			if unknown == "" {
				unknown = name
			}
			return match
		}
		return value
	})
	if unknown != "" {
		return "", fmt.Errorf("unknown variable {{%s}}", unknown)
	}
	return out, nil
}
