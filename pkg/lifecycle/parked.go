// Package lifecycle — the orthogonal "parked" scheduling axis.
//
// Parked answers a different question than **Status:** does. Status is a
// MATURITY axis — how well-specified and agreed is this artifact? Parked is
// a SCHEDULING axis — when will we build it? A fully-specced, ratified
// (high-maturity) Feature can still be "not this release" (parked); an
// early Draft can be parked too. The two axes are independent, so parking
// an artifact:
//
//   - NEVER changes `**Status:**`. A parked artifact keeps whatever status
//     it had; unparking restores nothing because nothing was taken away.
//   - Is NOT a lifecycle transition. There is no legal-transition matrix
//     entry for it, and it is not gated on the artifact's current status —
//     a Draft can be parked, so can an Approved.
//
// This mirrors the existing "Archived" axis for the Idea kind (see
// pkg/idea/archive.go): both are structured header-line facts orthogonal to
// **Status:**, inserted immediately after the `**Status:**` line. Parked
// differs from Archived in one respect: it never relocates the file — a
// parked artifact stays exactly where it lives; only the header changes.
//
// Unlike an optional structured field (e.g. SetSupersededBy's successor),
// parking REQUIRES a reason and a date — a bare `**Parked:** true` with no
// explanation rots into a graveyard nobody can audit, which is the exact
// failure this axis exists to prevent (see cli/parked#req:reason-required).
package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

// parkedLineRe matches a `**Parked:** <value>` header line. Capture groups
// mirror statusLineRe: [1] indent, [2] value, [3] trailing.
var parkedLineRe = regexp.MustCompile(`^([ \t]*)\*\*Parked:\*\*[ \t]+([^\r\n]*?)([ \t]*\r?)$`)

// parkedReasonLineRe matches a `**Parked Reason:** <value>` header line.
var parkedReasonLineRe = regexp.MustCompile(`^([ \t]*)\*\*Parked Reason:\*\*[ \t]+([^\r\n]*?)([ \t]*\r?)$`)

// parkedDateLineRe matches a `**Parked Date:** <value>` header line.
var parkedDateLineRe = regexp.MustCompile(`^([ \t]*)\*\*Parked Date:\*\*[ \t]+([^\r\n]*?)([ \t]*\r?)$`)

// ErrParkReasonRequired is returned by SetParked when reason is empty or
// whitespace-only. A bare `park` with no explanation is rejected BEFORE any
// mutation — this is the load-bearing guard against the parked axis rotting
// into an unaudited graveyard.
var ErrParkReasonRequired = errors.New("lifecycle: park requires a non-empty reason")

// ErrNotParked is returned by ClearParked when the artifact carries no
// `**Parked:** true` axis to clear.
var ErrNotParked = errors.New("lifecycle: artifact is not parked")

// parkedTodayUTC returns today's date in the repo-wide ISO-8601 (YYYY-MM-DD)
// convention (mirrors pkg/idea, pkg/feature, pkg/decision, pkg/plan, and
// pkg/lesson scaffolds, all of which stamp `time.Now().UTC().Format(
// "2006-01-02")`). A package-level var so tests can inject a fixed date.
var parkedTodayUTC = func() string { return time.Now().UTC().Format("2006-01-02") }

// ParkedInfo is the parsed parked axis of an artifact.
type ParkedInfo struct {
	Parked bool
	Reason string
	Date   string // YYYY-MM-DD, or "" if absent/unparseable
}

// ReadParked scans artifactPath for the `**Parked:**` / `**Parked Reason:**`
// / `**Parked Date:**` header lines and returns their parsed values. A file
// with no `**Parked:**` line (or `**Parked:** false`/anything other than
// `true`) reports Parked: false — absence means not parked, mirroring
// Idea's `**Archived:**` convention.
func ReadParked(artifactPath string) (ParkedInfo, error) {
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		return ParkedInfo{}, err
	}
	var info ParkedInfo
	for _, ln := range splitKeepTerminators(data) {
		body, _ := splitTerminator(ln)
		if m := parkedLineRe.FindStringSubmatch(body); m != nil {
			info.Parked = strings.EqualFold(strings.TrimSpace(m[2]), "true")
			continue
		}
		if m := parkedReasonLineRe.FindStringSubmatch(body); m != nil {
			info.Reason = strings.TrimSpace(m[2])
			continue
		}
		if m := parkedDateLineRe.FindStringSubmatch(body); m != nil {
			info.Date = strings.TrimSpace(m[2])
			continue
		}
	}
	return info, nil
}

// IsParked is a convenience wrapper over ReadParked for callers that only
// need the boolean (e.g. a `--parked` list filter).
func IsParked(artifactPath string) (bool, error) {
	info, err := ReadParked(artifactPath)
	if err != nil {
		return false, err
	}
	return info.Parked, nil
}

// SetParked marks artifactPath as parked: it writes (or rewrites, if
// already present) a contiguous three-line block —
//
//	**Parked:** true
//	**Parked Reason:** <reason>
//	**Parked Date:** <today, UTC, YYYY-MM-DD>
//
// — immediately after the `**Status:**` line, WITHOUT touching **Status:**
// itself or any other byte of the file. Re-parking an already-parked
// artifact overwrites the reason and resets the date to today — the same
// "rewrite the value in place" semantics SetSupersededBy/SetSupersedes use
// for their structured fields — so confirming "still deliberately deferred"
// is just re-running `park` with a fresh --reason.
//
// reason is REQUIRED: an empty or whitespace-only value returns
// ErrParkReasonRequired and leaves the file untouched (no partial mutation).
// A file with no recognizable `**Status:**` line returns
// ErrStatusLineNotFound, also untouched.
//
// On a write, original holds the exact pre-invocation file bytes so the
// caller can roll back via RestoreBody, matching every other structured
// header-field writer in this package (SetSupersededBy, SetSupersedes,
// AppendResolutionNote).
func SetParked(artifactPath, reason string) (original []byte, wrote bool, err error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, false, ErrParkReasonRequired
	}

	orig, err := os.ReadFile(artifactPath)
	if err != nil {
		return nil, false, err
	}
	lines := splitKeepTerminators(orig)

	statusIdx, cleaned := removeParkedLines(lines)
	if statusIdx < 0 {
		return nil, false, ErrStatusLineNotFound
	}

	anchorBody, terminator := splitTerminator(cleaned[statusIdx])
	if terminator == "" {
		terminator = "\n"
		cleaned[statusIdx] = anchorBody + terminator
	}
	date := parkedTodayUTC()
	block := []string{
		fmt.Sprintf("**Parked:** true%s", terminator),
		fmt.Sprintf("**Parked Reason:** %s%s", reason, terminator),
		fmt.Sprintf("**Parked Date:** %s%s", date, terminator),
	}
	newLines := insertAt(cleaned, statusIdx+1, block)

	if err := writeFileAtomic(artifactPath, joinLines(newLines)); err != nil {
		return nil, false, err
	}
	return orig, true, nil
}

// ClearParked reverses SetParked: it removes the `**Parked:**`,
// `**Parked Reason:**`, and `**Parked Date:**` header lines entirely — an
// absent flag means not parked, so unparking restores nothing (nothing was
// taken away; **Status:** was never touched).
//
// If the artifact carries no `**Parked:** true` line, ClearParked returns
// ErrNotParked and leaves the file untouched — there is nothing to undo,
// which is more useful to the caller than a silent no-op (it catches a
// mistyped slug the caller believed was already parked).
//
// On a write, original holds the exact pre-invocation file bytes, for the
// same rollback contract as SetParked.
func ClearParked(artifactPath string) (original []byte, wrote bool, err error) {
	orig, err := os.ReadFile(artifactPath)
	if err != nil {
		return nil, false, err
	}
	lines := splitKeepTerminators(orig)

	parked := false
	for _, ln := range lines {
		body, _ := splitTerminator(ln)
		if m := parkedLineRe.FindStringSubmatch(body); m != nil {
			parked = strings.EqualFold(strings.TrimSpace(m[2]), "true")
			break
		}
	}
	if !parked {
		return nil, false, ErrNotParked
	}

	_, newLines := removeParkedLines(lines)
	if err := writeFileAtomic(artifactPath, joinLines(newLines)); err != nil {
		return nil, false, err
	}
	return orig, true, nil
}

// removeParkedLines returns a copy of lines with any existing
// `**Parked:**` / `**Parked Reason:**` / `**Parked Date:**` header lines
// stripped, plus the index (in the RETURNED slice) of the `**Status:**`
// line to anchor a fresh insertion after, or -1 when no `**Status:**` line
// is present. Mirrors idea.setArchivedHeader's single-pass approach, using
// this package's terminator-preserving line helpers so CRLF files round-trip
// byte-for-byte outside the mutated block.
func removeParkedLines(lines []string) (statusIdx int, out []string) {
	statusIdx = -1
	for _, ln := range lines {
		body, _ := splitTerminator(ln)
		if parkedLineRe.MatchString(body) || parkedReasonLineRe.MatchString(body) || parkedDateLineRe.MatchString(body) {
			continue
		}
		if statusIdx == -1 && statusLineRe.MatchString(body) {
			statusIdx = len(out)
		}
		out = append(out, ln)
	}
	return statusIdx, out
}
