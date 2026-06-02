package lint

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

var decisionImmutabilityRuleIDs = []string{
	"D-immutability-once-accepted",
	"D-observed-consequences-append-only",
}

type decisionImmutabilityChecker struct{}

func newDecisionImmutabilityChecker() *decisionImmutabilityChecker {
	return &decisionImmutabilityChecker{}
}

func (c *decisionImmutabilityChecker) name() string     { return "D-immutability-once-accepted" }
func (c *decisionImmutabilityChecker) severity() string { return "error" }

func (c *decisionImmutabilityChecker) check(specRoot string) ([]Violation, error) {
	return checkDecisionImmutability(specRoot)
}

// gitShowFn is injectable for testing; returns the content of a file at HEAD.
var gitShowFn = gitShowFile

func gitShowFile(repoRoot, relPath string) (string, error) {
	cmd := exec.Command("git", "show", "HEAD:"+relPath)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func checkDecisionImmutability(specRoot string) ([]Violation, error) {
	var vs []Violation

	decisions, err := discoverDecisionFiles(specRoot)
	if err != nil {
		return nil, err
	}

	// Find the git repo root
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = specRoot
	topOut, err := cmd.Output()
	if err != nil {
		// Not a git repo — skip immutability checks
		return nil, nil
	}
	repoRoot := strings.TrimSpace(string(topOut))

	for _, d := range decisions {
		status := ""
		if f, ok := d.fieldByName["Status"]; ok {
			status = f.Value
		}
		// Only check Accepted decisions
		if status != "Accepted" {
			continue
		}

		// Get the committed version from HEAD. repoRoot comes from
		// `git rev-parse --show-toplevel`, which resolves symlinks, so the
		// decision path must be canonicalized too — otherwise on a symlinked
		// working tree (e.g. macOS /tmp → /private/tmp) the relative path is
		// computed against mismatched prefixes, git show fails, and the rule
		// silently no-ops. EvalSymlinks errors are tolerated: an unresolvable
		// path falls through to the err check below.
		canonPath, _ := filepath.EvalSymlinks(d.path)
		relToRepo, err := filepathRelDecisionImmutability(repoRoot, canonPath)
		if err != nil {
			continue
		}
		committedContent, err := gitShowFn(repoRoot, relToRepo)
		if err != nil {
			// File not in git yet (new file) — skip immutability check.
			// This also grandfathers D-0001: if the immutability rule ships
			// in the same commit that modifies D-0001, there's no prior
			// committed version to compare against.
			continue
		}

		// Parse the committed version
		committedDecision, err := parseDecisionFromContentFn(committedContent, d.relPath, d.archived)
		if err != nil {
			continue
		}

		// Only enforce if the committed version was also Accepted
		committedStatus := ""
		if f, ok := committedDecision.fieldByName["Status"]; ok {
			committedStatus = f.Value
		}
		if committedStatus != "Accepted" {
			continue
		}

		// Compare each frozen section
		vs = append(vs, checkFrozenSections(d, committedDecision)...)

		// Check Observed Consequences append-only
		vs = append(vs, checkObservedConsequencesAppendOnly(d, committedDecision)...)
	}

	return vs, nil
}

// parseDecisionFromContentFn is injectable for testing the parse-error branch
// in checkDecisionImmutability (parseDecisionFromContent itself never errors).
var parseDecisionFromContentFn = parseDecisionFromContent

func parseDecisionFromContent(content, relPath string, archived bool) (*parsedDecision, error) {
	lines := strings.Split(content, "\n")

	d := &parsedDecision{
		relPath:       relPath,
		archived:      archived,
		lines:         lines,
		fieldByName:   make(map[string]decisionField),
		sectionByName: make(map[string]decisionSection),
	}

	// Parse title
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if m := decisionTitleRe.FindStringSubmatch(trimmed); m != nil {
			d.title = strings.TrimSpace(m[1])
			d.titleLine = i + 1
			d.titleOK = true
			d.hasTitleTag = true
		} else if strings.HasPrefix(trimmed, "# ") {
			d.titleLine = i + 1
		}
		break
	}

	// Parse header fields
	for i := d.titleLine; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "## ") {
			break
		}
		if m := decisionFieldRe.FindStringSubmatch(trimmed); m != nil {
			f := decisionField{
				Name:  m[1],
				Value: strings.TrimSpace(m[2]),
				Line:  i + 1,
			}
			d.fields = append(d.fields, f)
			d.fieldByName[f.Name] = f
		}
	}

	// Parse sections
	var currentSection *decisionSection
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") && !strings.HasPrefix(trimmed, "### ") {
			if currentSection != nil {
				s := *currentSection
				d.sections = append(d.sections, s)
				d.sectionByName[s.Title] = s
			}
			title := strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			currentSection = &decisionSection{
				Title:     title,
				StartLine: i + 1,
			}
		} else if strings.HasPrefix(trimmed, "### ") && currentSection != nil {
			h3 := strings.TrimSpace(strings.TrimPrefix(trimmed, "### "))
			currentSection.SubH3s = append(currentSection.SubH3s, h3)
		}
		if currentSection != nil && !strings.HasPrefix(trimmed, "## ") {
			currentSection.Body += line + "\n"
		}
	}
	if currentSection != nil {
		s := *currentSection
		d.sections = append(d.sections, s)
		d.sectionByName[s.Title] = s
	}

	return d, nil
}

// frozenSections are immutable once a Decision is Accepted.
// "Observed Consequences" is handled separately (append-only).
var frozenSections = []string{
	"Context", "Decision", "Rationale", "Declined Alternatives",
	"Consequences at Decision Time", "Affected Features",
}

func checkFrozenSections(current, committed *parsedDecision) []Violation {
	var vs []Violation

	// Check title immutability
	if current.title != committed.title {
		vs = append(vs, Violation{
			File: current.relPath, Line: current.titleLine, Severity: "error",
			Rule:    "D-immutability-once-accepted",
			Message: fmt.Sprintf("Accepted Decision title changed from %q to %q", committed.title, current.title),
		})
	}

	// Check frozen header fields (all except Status, Superseded By)
	mutableFields := map[string]bool{
		"Status":        true,
		"Superseded By": true,
	}
	for _, f := range current.fields {
		if mutableFields[f.Name] {
			continue
		}
		committedField, ok := committed.fieldByName[f.Name]
		if !ok {
			continue
		}
		if f.Value != committedField.Value {
			vs = append(vs, Violation{
				File: current.relPath, Line: f.Line, Severity: "error",
				Rule:    "D-immutability-once-accepted",
				Message: fmt.Sprintf("Accepted Decision field **%s:** changed from %q to %q", f.Name, committedField.Value, f.Value),
			})
		}
	}

	// Check frozen sections
	for _, name := range frozenSections {
		currSection, currOK := current.sectionByName[name]
		commSection, commOK := committed.sectionByName[name]
		if !currOK || !commOK {
			continue
		}
		currBody := strings.TrimSpace(currSection.Body)
		commBody := strings.TrimSpace(commSection.Body)
		if currBody != commBody {
			vs = append(vs, Violation{
				File: current.relPath, Line: currSection.StartLine, Severity: "error",
				Rule:    "D-immutability-once-accepted",
				Message: fmt.Sprintf("Accepted Decision section %q was modified (body is frozen once Accepted)", name),
			})
		}
	}

	return vs
}

func checkObservedConsequencesAppendOnly(current, committed *parsedDecision) []Violation {
	var vs []Violation

	currSection, currOK := current.sectionByName["Observed Consequences"]
	commSection, commOK := committed.sectionByName["Observed Consequences"]
	if !currOK || !commOK {
		return nil
	}

	currBody := strings.TrimSpace(currSection.Body)
	commBody := strings.TrimSpace(commSection.Body)

	// If unchanged, no violation
	if currBody == commBody {
		return nil
	}

	// Append-only: the committed body must be a prefix of the current body.
	// Split into lines and check that all committed lines are still present
	// at the beginning of the current body.
	commLines := nonEmptyLines(commBody)
	currLines := nonEmptyLines(currBody)

	// Handle the special case where committed had "None observed yet."
	// and current replaced it with real entries — that's allowed.
	if len(commLines) == 1 && commLines[0] == "None observed yet." {
		return nil
	}

	// Check that all committed lines appear as a prefix of current lines
	if len(commLines) > len(currLines) {
		vs = append(vs, Violation{
			File: current.relPath, Line: currSection.StartLine, Severity: "error",
			Rule:    "D-observed-consequences-append-only",
			Message: "Observed Consequences entries were removed (append-only after Accepted)",
		})
		return vs
	}

	for i, commLine := range commLines {
		if i >= len(currLines) || strings.TrimSpace(currLines[i]) != strings.TrimSpace(commLine) {
			vs = append(vs, Violation{
				File: current.relPath, Line: currSection.StartLine, Severity: "error",
				Rule:    "D-observed-consequences-append-only",
				Message: "existing Observed Consequences entries were modified (append-only after Accepted)",
			})
			return vs
		}
	}

	return nil
}

func nonEmptyLines(s string) []string {
	lines := strings.Split(s, "\n")
	var out []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}
