package plan

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// parseReadinessPlan is a test seam for the otherwise rare read failures that
// can occur between a successful path resolution and parsing its file.
var parseReadinessPlan = Parse

// resolveReadinessPlanFile is a test seam over the shared flat-first plan
// resolver. It lets readiness prove that only a genuine NotFound is rendered
// as a missing prerequisite; filesystem failures remain operational errors.
var resolveReadinessPlanFile = resolvePlanFile

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
	DerivedStatus string `yaml:"derived_status,omitempty" json:"derived_status,omitempty"`
	// Reason is set only for invalid prerequisite graphs or declarations. It
	// keeps derived_status reserved for a determinate task-rollup result.
	Reason string `yaml:"reason,omitempty" json:"reason,omitempty"`
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
	return newPrerequisiteReadinessEvaluator(plansDir).evaluatePlan(p, p.Slug)
}

func newPrerequisiteReadinessEvaluator(plansDir string) *prerequisiteReadinessEvaluator {
	return &prerequisiteReadinessEvaluator{
		plansDir: plansDir,
		visited:  make(map[string]prerequisiteEvaluation),
		active:   make(map[string]int),
	}
}

type prerequisiteEvaluation struct {
	derived   string
	derivedOK bool
	status    string
	unmet     []UnmetPrerequisite
}

// prerequisiteReadinessEvaluator walks the complete reachable prerequisite
// graph. visited bounds repeated DAG branches; active identifies a back edge
// before an apparently-Implemented direct prerequisite can bypass a cycle.
type prerequisiteReadinessEvaluator struct {
	plansDir string
	visited  map[string]prerequisiteEvaluation
	active   map[string]int
	stack    []string
}

func (e *prerequisiteReadinessEvaluator) evaluatePlan(p *Plan, slug string) (PrerequisiteReadiness, error) {
	if slug == "" {
		slug = "<current plan>"
	}
	e.active[slug] = len(e.stack)
	e.stack = append(e.stack, slug)
	evaluation, err := e.evaluateContents(p)
	e.stack = e.stack[:len(e.stack)-1]
	delete(e.active, slug)
	if err != nil {
		return PrerequisiteReadiness{}, err
	}
	return PrerequisiteReadiness{Ready: len(evaluation.unmet) == 0, Unmet: evaluation.unmet}, nil
}

func (e *prerequisiteReadinessEvaluator) evaluateContents(p *Plan) (prerequisiteEvaluation, error) {
	result := prerequisiteEvaluation{unmet: []UnmetPrerequisite{}}
	if reason := malformedPrerequisiteDeclaration(p); reason != "" {
		result.unmet = append(result.unmet, UnmetPrerequisite{
			Slug:   "<malformed prerequisite declaration>",
			Status: "invalid",
			Reason: reason,
		})
	}
	for _, slug := range p.PrerequisitePlans {
		prerequisite, err := e.evaluateSlug(slug)
		if err != nil {
			if isNotFound(err) {
				result.unmet = append(result.unmet, UnmetPrerequisite{Slug: slug, Status: "missing"})
				continue
			}
			return prerequisiteEvaluation{}, err
		}
		if !prerequisite.derivedOK || prerequisite.derived != "Implemented" {
			// A cycle/invalid child already carries its actionable diagnostic;
			// avoid adding a duplicate synthetic direct prerequisite beside it.
			if len(prerequisite.unmet) == 0 {
				result.unmet = append(result.unmet, UnmetPrerequisite{
					Slug: slug, Status: prerequisite.status, DerivedStatus: prerequisite.derived,
				})
			} else {
				result.unmet = append(result.unmet, prerequisite.unmet...)
			}
			continue
		}
		if len(prerequisite.unmet) > 0 {
			result.unmet = append(result.unmet, prerequisite.unmet...)
		}
	}
	result.derived, result.derivedOK = p.DeriveExecutionBand()
	result.status = strings.TrimSpace(p.Status)
	if result.status == "" {
		result.status = "unset"
	}
	return result, nil
}

func (e *prerequisiteReadinessEvaluator) evaluateSlug(slug string) (prerequisiteEvaluation, error) {
	if start, active := e.active[slug]; active {
		cycle := append(append([]string{}, e.stack[start:]...), slug)
		return prerequisiteEvaluation{unmet: []UnmetPrerequisite{{
			Slug:   slug,
			Status: "invalid",
			Reason: "prerequisite cycle: " + strings.Join(cycle, " -> "),
		}}}, nil
	}
	if cached, ok := e.visited[slug]; ok {
		return cached, nil
	}

	path, err := resolveReadinessPlanFile(e.plansDir, slug)
	if err != nil {
		return prerequisiteEvaluation{}, err
	}
	p, err := parseReadinessPlan(path)
	if err != nil {
		return prerequisiteEvaluation{}, exitcode.UnexpectedErrorf("parsing prerequisite plan %q at %s: %v", slug, path, err)
	}

	e.active[slug] = len(e.stack)
	e.stack = append(e.stack, slug)
	evaluation, err := e.evaluateContents(p)
	e.stack = e.stack[:len(e.stack)-1]
	delete(e.active, slug)
	if err != nil {
		return prerequisiteEvaluation{}, err
	}
	e.visited[slug] = evaluation
	return evaluation, nil
}

func isNotFound(err error) bool {
	var coded interface{ ExitCode() int }
	return errors.As(err, &coded) && coded.ExitCode() == exitcode.NotFound
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
		if prerequisite.Reason != "" {
			parts[i] = fmt.Sprintf("%s (status %q; %s)", prerequisite.Slug, prerequisite.Status, prerequisite.Reason)
			continue
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
		return PrerequisiteReadiness{}, exitcode.UnexpectedErrorf("parsing plan %q: %v", slug, err)
	}
	// Use the command argument as the root identity. A title can be stale or
	// malformed, but the prerequisite graph's edges name filesystem slugs.
	return newPrerequisiteReadinessEvaluator(plansDir).evaluatePlan(p, slug)
}
