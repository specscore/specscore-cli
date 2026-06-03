package consilium

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// argumentMaxLen is the upper bound on a vote's argument, per
// REQ:vote-schema-types-and-validation. Length is counted in Unicode code
// points (runes), not bytes, so multi-byte characters each count as one.
const argumentMaxLen = 280

// verdictEnum / confidenceEnum / costEnum / complexityEnum are the closed sets
// of accepted values for each enum field, kept in stable order so error
// messages list them deterministically. The cost and complexity emoji scale
// (🟢 🟡 🔴) is shared across both fields.
var (
	verdictEnum    = []string{"should-implement", "should-not-implement", "no-opinion", "abstain"}
	confidenceEnum = []string{"low", "medium", "high"}
	costEnum       = []string{"🟢", "🟡", "🔴"}
	complexityEnum = costEnum
)

// Vote is one expert-panel vote. It carries exactly the five fields defined by
// REQ:vote-schema-types-and-validation. Role is not part of the vote schema
// itself; when a votes-file entry carries a `role:` key it is retained so
// validation errors can name the offending vote by role as well as by index.
type Vote struct {
	Verdict    string `yaml:"verdict"`
	Confidence string `yaml:"confidence"`
	Cost       string `yaml:"cost"`
	Complexity string `yaml:"complexity"`
	Argument   string `yaml:"argument"`
	// Role is optional metadata used only to make validation errors more
	// legible; it is not one of the five schema fields.
	Role string `yaml:"role,omitempty"`
}

// ValidationError is the deterministic error returned by vote validation. Its
// message names the offending vote (by index, and role when present) and the
// rule violated, so the malformed-bundle ACs can string-match on both.
type ValidationError struct {
	// Index is the zero-based position of the offending vote in the bundle.
	Index int
	// Role is the offending vote's role slug when the entry carried one; "".
	Role string
	// Field is the offending field name (e.g. "verdict", "argument").
	Field string
	// Rule is a human-readable description of the rule that was violated.
	Rule string
}

// Error formats the validation error as
// `vote[<i>]<(role=<r>)>: <field>: <rule>`. The shape is stable so tests can
// string-match on it.
func (e *ValidationError) Error() string {
	loc := fmt.Sprintf("vote[%d]", e.Index)
	if e.Role != "" {
		loc += fmt.Sprintf(" (role=%s)", e.Role)
	}
	return fmt.Sprintf("%s: %s: %s", loc, e.Field, e.Rule)
}

// ParseVotes parses a YAML votes bundle (a list of votes) and validates every
// entry. On success it returns one Vote per entry with fields populated
// verbatim. Tolerance for malformed votes is zero: a YAML-parse failure, a
// missing required field, an out-of-enum verdict/confidence/cost/complexity,
// or an argument longer than 280 characters fails the entire bundle with a
// *ValidationError (or a parse error) naming the offending vote and rule. A
// malformed bundle never yields a partial result.
func ParseVotes(data []byte) ([]Vote, error) {
	var votes []Vote
	if err := yaml.Unmarshal(data, &votes); err != nil {
		return nil, fmt.Errorf("parse votes: %w", err)
	}
	for i := range votes {
		if err := validateVote(i, votes[i]); err != nil {
			return nil, err
		}
	}
	return votes, nil
}

// validateVote enforces REQ:vote-schema-types-and-validation for a single
// vote. It returns the first violation as a *ValidationError; a vote that
// passes every check returns nil.
func validateVote(idx int, v Vote) error {
	if !contains(verdictEnum, v.Verdict) {
		return enumErr(idx, v.Role, "verdict", v.Verdict, verdictEnum)
	}
	if !contains(confidenceEnum, v.Confidence) {
		return enumErr(idx, v.Role, "confidence", v.Confidence, confidenceEnum)
	}
	if !contains(costEnum, v.Cost) {
		return enumErr(idx, v.Role, "cost", v.Cost, costEnum)
	}
	if !contains(complexityEnum, v.Complexity) {
		return enumErr(idx, v.Role, "complexity", v.Complexity, complexityEnum)
	}
	if v.Argument == "" {
		return &ValidationError{
			Index: idx,
			Role:  v.Role,
			Field: "argument",
			Rule:  "must be a non-empty string",
		}
	}
	if n := utf8.RuneCountInString(v.Argument); n > argumentMaxLen {
		return &ValidationError{
			Index: idx,
			Role:  v.Role,
			Field: "argument",
			Rule:  fmt.Sprintf("must be ≤ %d characters, got %d", argumentMaxLen, n),
		}
	}
	return nil
}

// enumErr builds a ValidationError for an out-of-enum field. An empty value is
// reported as a missing-field violation so callers see "must be one of ..."
// either way; the rule string lists the accepted enum verbatim.
func enumErr(idx int, role, field, value string, enum []string) error {
	rule := "must be one of " + strings.Join(enum, ", ")
	if value == "" {
		rule = "missing required field; " + rule
	} else {
		rule = fmt.Sprintf("out-of-enum value %q; %s", value, rule)
	}
	return &ValidationError{
		Index: idx,
		Role:  role,
		Field: field,
		Rule:  rule,
	}
}

// contains reports whether s appears in haystack.
func contains(haystack []string, s string) bool {
	for _, h := range haystack {
		if h == s {
			return true
		}
	}
	return false
}
