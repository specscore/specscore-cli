package lesson

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
)

// CorrectionReceiptPrefix starts the line an audited correction to Recorded
// appends to its `## Resolution` note. The line fingerprints the
// `## Enforcement` receipt (Control, Verification, Evidence) the correction
// judged not to bind, so a later `--to=enforced` can refuse to reuse it
// (cli/lesson/change-status#req:re-enforcement-requires-fresh-receipt).
const CorrectionReceiptPrefix = "Enforcement receipt at correction: sha256:"

var correctionReceiptRe = regexp.MustCompile(`^` + regexp.QuoteMeta(CorrectionReceiptPrefix) + `([0-9a-f]{64})$`)

// EnforcementReceipt returns the hex SHA-256 of l's normalised Enforcement
// receipt: the parsed Control, Verification, and Evidence values with every
// whitespace run collapsed to one space, so re-wrapping a field is not a
// change while any edit to its words is.
func EnforcementReceipt(l *Lesson) string {
	normalise := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	canonical := "Control: " + normalise(l.Control) + "\n" +
		"Verification: " + normalise(l.Verification) + "\n" +
		"Evidence: " + normalise(l.Evidence) + "\n"
	return fmt.Sprintf("%x", sha256.Sum256([]byte(canonical)))
}

// WithCorrectionReceipt appends the correction-receipt line for digest to a
// demotion note as its own trailing paragraph.
func WithCorrectionReceipt(note, digest string) string {
	return strings.TrimRight(note, "\n\r \t") + "\n\n" + CorrectionReceiptPrefix + digest
}

// parseCorrectionReceipt returns the digest carried by a correction-receipt
// line, or "" when line is not one.
func parseCorrectionReceipt(line string) string {
	if m := correctionReceiptRe.FindStringSubmatch(line); m != nil {
		return m[1]
	}
	return ""
}
