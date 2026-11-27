package lesson

import (
	"crypto/sha256"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

// CorrectionReceiptPrefix starts the line an audited correction to Recorded
// appends to its `## Resolution` note. The line fingerprints the
// `## Enforcement` receipt (Control, Verification, Evidence) the correction
// judged not to bind, so a later `--to=enforced` can refuse to reuse it
// (cli/lesson/change-status#req:re-enforcement-requires-fresh-receipt).
const CorrectionReceiptPrefix = "Enforcement receipt at correction: sha256:"

// correctionReceiptRe matches a receipt line exactly where ChangeStatus
// writes one: unindented, nothing after the digest but trailing whitespace.
var correctionReceiptRe = regexp.MustCompile(`^` + regexp.QuoteMeta(CorrectionReceiptPrefix) + `([0-9a-f]{64})[ \t\r]*$`)

// readLessonFn is the testable indirection for the under-lock receipt read.
var readLessonFn = os.ReadFile

// ReceiptState is a Lesson's Enforcement receipt and every receipt its
// audited corrections recorded.
type ReceiptState struct {
	// Current is the hex SHA-256 of the receipt in the first `## Enforcement`
	// section, or "" when that section has no Control, Verification, or
	// Evidence value (a compatibility Lesson with prose only has no receipt).
	Current string
	// Recorded lists, in document order, every correction-receipt line in a
	// `## Resolution` section.
	Recorded []string
}

// Disproved reports whether the current receipt equals any receipt an
// earlier audited correction recorded.
func (s ReceiptState) Disproved() bool {
	if s.Current == "" {
		return false
	}
	for _, recorded := range s.Recorded {
		if recorded == s.Current {
			return true
		}
	}
	return false
}

// ReadReceiptState extracts the receipt state from Lesson content. Only
// structural lines count (frontmatter, fenced or indented code, and HTML
// comments are skipped, exactly as the Resolution writer skips them);
// Control, Verification, and Evidence are read only from the first
// `## Enforcement` section, first occurrence each, so text elsewhere never
// changes the receipt.
func ReadReceiptState(content []byte) ReceiptState {
	lines := strings.Split(string(content), "\n")
	structure := lifecycle.StructuralMarkdownMask(lines, "")
	var state ReceiptState
	fields := map[string]string{}
	section, enforcementSeen, inEnforcement := "", false, false
	for i, line := range lines {
		if !structure[i] {
			continue
		}
		body := strings.TrimRight(line, " \t\r")
		if title, ok := strings.CutPrefix(body, "## "); ok {
			section = strings.TrimSpace(title)
			inEnforcement = section == "Enforcement" && !enforcementSeen
			enforcementSeen = enforcementSeen || inEnforcement
			continue
		}
		if inEnforcement {
			if name, val, ok := matchBoldField(strings.TrimSpace(body)); ok && isReceiptField(name) {
				if _, seen := fields[name]; !seen {
					fields[name], _ = joinWrappedFieldValue(lines, i, val)
				}
			}
			continue
		}
		if section == "Resolution" {
			if m := correctionReceiptRe.FindStringSubmatch(body); m != nil {
				state.Recorded = append(state.Recorded, m[1])
			}
		}
	}
	state.Current = enforcementReceipt(fields["Control"], fields["Verification"], fields["Evidence"])
	return state
}

func isReceiptField(name string) bool {
	return name == "Control" || name == "Verification" || name == "Evidence"
}

// enforcementReceipt hashes the three values with every whitespace run
// collapsed to one space, so re-wrapping a field is not a change while any
// edit to its words is. An all-empty set has no receipt.
func enforcementReceipt(control, verification, evidence string) string {
	normalise := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	control, verification, evidence = normalise(control), normalise(verification), normalise(evidence)
	if control == "" && verification == "" && evidence == "" {
		return ""
	}
	canonical := "Control: " + control + "\nVerification: " + verification + "\nEvidence: " + evidence + "\n"
	return fmt.Sprintf("%x", sha256.Sum256([]byte(canonical)))
}

// WithCorrectionReceipt appends the correction-receipt line for digest to a
// demotion note as its own trailing paragraph.
func WithCorrectionReceipt(note, digest string) string {
	line := CorrectionReceiptPrefix + digest
	if note = strings.TrimRight(note, "\n\r \t"); note == "" {
		return line
	}
	return note + "\n\n" + line
}

// CheckReEnforcement refuses (exit 4) a transition to Enforced while the
// Lesson at path carries a receipt an audited correction already recorded.
func CheckReEnforcement(slug, path string) error {
	content, err := readLessonFn(path)
	if err != nil {
		return exitcode.UnexpectedErrorf("reading lesson %q receipt: %v", slug, err)
	}
	if state := ReadReceiptState(content); state.Disproved() {
		return exitcode.InvalidStateErrorf(
			"lesson %q cannot be re-enforced: its ## Enforcement Control, Verification, and Evidence match a receipt an audited correction to Recorded already disproved (sha256:%s); replace the disproved receipt with a fresh positive one, then retry (nothing was modified)",
			slug, state.Current)
	}
	return nil
}

// ValidateTransitionNote refuses a change-status note that could forge or
// misplace receipt state: a correction-receipt-like line, a Control,
// Verification, or Evidence field line, a `## ` heading (it would end the
// Resolution section before the receipt), or an unclosed code fence or HTML
// comment (it would swallow the receipt line appended after the note).
func ValidateTransitionNote(note string) error {
	lines := strings.Split(note, "\n")
	marker := strings.ToLower(strings.TrimSuffix(CorrectionReceiptPrefix, " sha256:"))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(strings.ToLower(trimmed), marker) {
			return fmt.Errorf("--note must not contain an %q line; only the audited correction writes receipts", strings.TrimSuffix(CorrectionReceiptPrefix, " sha256:"))
		}
		if trimmed == "##" || strings.HasPrefix(trimmed, "## ") {
			return fmt.Errorf("--note must not contain a `## ` heading; it would end the ## Resolution section")
		}
		if name, _, ok := matchBoldField(trimmed); ok && isReceiptField(name) {
			return fmt.Errorf("--note must not contain a **%s:** field line; Enforcement fields belong in ## Enforcement", name)
		}
	}
	// Frame the note between plain lines so a leading `---` is not read as
	// frontmatter, then require the line after it to be structural.
	framed := append(append([]string{"note"}, lines...), "end")
	if structure := lifecycle.StructuralMarkdownMask(framed, ""); !structure[len(framed)-1] {
		return fmt.Errorf("--note must not leave a code fence or HTML comment open")
	}
	return nil
}
