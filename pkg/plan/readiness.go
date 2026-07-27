package plan

import (
	"fmt"
	"path/filepath"
	"strings"
)

// parseReadinessPlan is a test seam for the otherwise rare read failures that
// can occur between a successful path resolution and parsing its file.
var parseReadinessPlan = Parse

// PrerequisiteReadiness describes whether a Plan may begin execution with
// respect to its declared same-repository prerequisite Plans. It deliberately
// does not inspect the Plan's own tasks: callers decide what operation counts
// as beginning execution. A prerequisite is met only when its task rollup
// derives the Implemented execution band; a hand-authored **Status:** line is
// never sufficient on its own.
type PrerequisiteReadiness struct {
	Ready bool
	Unmet []UnmetPrerequisite
}

// UnmetPrerequisite records the declared and derived state of one prerequisite
// that prevents execution. Status is the prerequisite's recorded **Status:**
// value (or "missing" when its Plan cannot be resolved); DerivedStatus is the
// task-rollup result, or empty when the rollup is indeterminate.
type UnmetPrerequisite struct {
	Slug          string `yaml:"slug" json:"slug"`
	Status        string `yaml:"status" json:"status"`
	DerivedStatus string `yaml:"derived_status" json:"derived_status"`
}

// PrerequisiteReadiness evaluates p's declared prerequisites using plansDir
// (the project's spec/plans directory). The result keeps every unmet
// prerequisite in declaration order, so callers can give an actionable
// diagnostic rather than stopping at the first one. Missing or malformed
// references are not treated as ready; P-009 remains responsible for naming
// their authoring errors during lint.
//
// Directory-form prerequisites are resolved with the same flat-first lookup
// used by plan lifecycle commands, so a valid legacy directory-form Plan can
// satisfy a prerequisite while it remains supported by those commands.
func (p *Plan) PrerequisiteReadiness(plansDir string) (PrerequisiteReadiness, error) {
	result := PrerequisiteReadiness{Ready: true, Unmet: []UnmetPrerequisite{}}
	if reason := malformedPrerequisiteDeclaration(p); reason != "" {
		result.Ready = false
		result.Unmet = append(result.Unmet, UnmetPrerequisite{
			Slug:          "<malformed prerequisite declaration>",
			Status:        "invalid",
			DerivedStatus: reason,
		})
	}
	for _, slug := range p.PrerequisitePlans {
		path, err := resolvePlanFile(plansDir, slug)
		if err != nil {
			result.Ready = false
			result.Unmet = append(result.Unmet, UnmetPrerequisite{Slug: slug, Status: "missing"})
			continue
		}

		prerequisite, err := parseReadinessPlan(path)
		if err != nil {
			return PrerequisiteReadiness{}, fmt.Errorf("parsing prerequisite plan %q at %s: %w", slug, path, err)
		}
		derived, derivedOK := prerequisite.DeriveExecutionBand()
		if derivedOK && derived == "Implemented" {
			continue
		}

		status := strings.TrimSpace(prerequisite.Status)
		if status == "" {
			status = "unset"
		}
		result.Ready = false
		result.Unmet = append(result.Unmet, UnmetPrerequisite{
			Slug:          slug,
			Status:        status,
			DerivedStatus: derived,
		})
	}
	return result, nil
}

// malformedPrerequisiteDeclaration is deliberately conservative: lint P-009
// owns the authoring diagnostics, but readiness must never report true for an
// invalid declaration merely because Parse recovered a partial list.
func malformedPrerequisiteDeclaration(p *Plan) string {
	if p.PrerequisiteLine == 0 || strings.TrimSpace(p.PrerequisiteRaw) == "—" {
		return ""
	}
	raw := strings.TrimSpace(p.PrerequisiteRaw)
	if raw == "" {
		return "empty"
	}
	seen := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		slug := strings.TrimSpace(part)
		if slug == "" {
			return "empty entry"
		}
		if err := ValidateSlug(slug); err != nil {
			return "invalid slug"
		}
		if seen[slug] {
			return "duplicate slug"
		}
		seen[slug] = true
		if slug == p.Slug {
			return "self reference"
		}
	}
	return ""
}

// UnmetMessage renders the stable, actionable part of an execution-readiness
// refusal. It names every unmet slug with both the recorded Plan status and
// the task-rollup-derived status that is authoritative for this gate.
func (r PrerequisiteReadiness) UnmetMessage() string {
	parts := make([]string, len(r.Unmet))
	for i, prerequisite := range r.Unmet {
		derived := prerequisite.DerivedStatus
		if derived == "" {
			derived = "indeterminate"
		}
		parts[i] = fmt.Sprintf("%s (status %q; derived %q)", prerequisite.Slug, prerequisite.Status, derived)
	}
	return strings.Join(parts, ", ")
}

// PlanReadiness resolves slug in specRoot and evaluates its prerequisites.
// It is the command-facing convenience wrapper around PrerequisiteReadiness.
func PlanReadiness(specRoot, slug string) (PrerequisiteReadiness, error) {
	plansDir := filepath.Join(specRoot, "spec", "plans")
	path, err := resolvePlanFile(plansDir, slug)
	if err != nil {
		return PrerequisiteReadiness{}, err
	}
	p, err := parseReadinessPlan(path)
	if err != nil {
		return PrerequisiteReadiness{}, fmt.Errorf("parsing plan %q: %w", slug, err)
	}
	return p.PrerequisiteReadiness(plansDir)
}
