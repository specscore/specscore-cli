package sidekick

// Seed-kind slug resolution + frontmatter status surface.
//
// Features implemented: cli/sidekick/change-status (seed-slug-resolution,
// seed-status-surface).
//
// A sidekick-seed carries its lifecycle status in the YAML frontmatter
// `status:` key and has NO body `**Status:**` line (unlike an Idea). This
// file hosts the two seed-kind primitives the change-status verb composes:
//
//   - ResolveSeedPath: <slug> → spec/ideas/seeds/<slug>.md within the project
//     root, refusing a match that exists only under spec/ideas/archived/, and
//     returning an exit-3 (NotFound) error naming the expected path when absent
//     (cli/sidekick/change-status#req:seed-slug-resolution,
//     lifecycle-transitions#req:slug-resolves-to-existing-artifact).
//   - RewriteFrontmatterStatus: line-targeted rewrite of the frontmatter
//     `status:` value, preserving every other byte
//     (cli/sidekick/change-status#req:seed-status-surface).
//
// The legal-transition matrix, relocation, and cobra wiring are owned by later
// tasks; this file carries only the resolution + status-surface primitives.

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// Testable os indirections (overridden in coverage tests to exercise
// otherwise-unreachable I/O error branches, mirroring pkg/idea's osStatFn
// convention). Production code always uses the real os functions.
var (
	statFn     = os.Stat
	mkdirAllFn = os.MkdirAll
	renameFn   = os.Rename
)

// ErrFrontmatterStatusNotFound is returned by RewriteFrontmatterStatus when
// the seed has no recognizable frontmatter `status:` line.
var ErrFrontmatterStatusNotFound = fmt.Errorf("sidekick: seed has no frontmatter status: line")

// fmStatusLineRe matches a YAML-frontmatter `status:` line. Capture groups:
// [1] indent, [2] value, [3] trailing whitespace + optional CR. Mirrors the
// shape lifecycle uses for the frontmatter mirror so CRLF files round-trip.
var fmStatusLineRe = regexp.MustCompile(`^([ \t]*)status:[ \t]*([^\r\n]*?)([ \t]*\r?)$`)

// ResolveSeedPath resolves slug to its canonical seed path
// spec/ideas/seeds/<slug>.md under specRoot (the project root containing the
// spec/ subtree, NOT the spec/ directory itself), honoring any configured
// ideas-path override via idea.ResolveSeedsDir.
//
// A seed already relocated to spec/ideas/archived/<slug>.md is NEVER a
// fallback (lifecycle-transitions#req:slug-resolves-to-existing-artifact): the
// canonical lookup is the seeds/ path only. When no file exists at that path,
// ResolveSeedPath returns an exit-3 (NotFound) error naming the expected
// spec/ideas/seeds/<slug>.md path
// (cli/sidekick/change-status#req:seed-slug-resolution).
func ResolveSeedPath(specRoot, slug string) (string, error) {
	if specRoot == "" {
		return "", exitcode.UnexpectedErrorf("ResolveSeedPath: specRoot required")
	}
	if slug == "" {
		return "", exitcode.InvalidArgsErrorf("ResolveSeedPath: slug required")
	}

	seedPath := filepath.Join(seedsDir(specRoot), slug+".md")
	if _, err := os.Stat(seedPath); err != nil {
		if os.IsNotExist(err) {
			return "", exitcode.NotFoundErrorf(
				"seed not found at %s", filepath.Join("spec", "ideas", "seeds", slug+".md"))
		}
		return "", exitcode.UnexpectedErrorf("stat %s: %v", seedPath, err)
	}
	return seedPath, nil
}

// seedsDir returns the absolute sidekick-seeds directory for specRoot. It
// inlines idea.ResolveSeedsDir's join (seeds/ under the resolved ideas dir)
// without importing pkg/idea — that package depends on pkg/sidekick for the
// seed lint rules, so pkg/sidekick must not import it back.
func seedsDir(specRoot string) string {
	return filepath.Join(specRoot, "spec", "ideas", "seeds")
}

// ReadFrontmatterStatus returns the verbatim frontmatter `status:` value at
// seedPath (trimmed of surrounding whitespace), for the strict source-state
// check the change-status verb runs before any mutation. If the seed has no
// recognizable frontmatter `status:` line, it returns
// ErrFrontmatterStatusNotFound.
func ReadFrontmatterStatus(seedPath string) (string, error) {
	content, err := os.ReadFile(seedPath)
	if err != nil {
		return "", err
	}
	lines := splitKeepTerminators(content)
	idx := findFrontmatterStatusLineIndex(lines)
	if idx < 0 {
		return "", ErrFrontmatterStatusNotFound
	}
	body, _ := splitTerminator(lines[idx])
	m := fmStatusLineRe.FindStringSubmatch(body)
	return strings.TrimSpace(m[2]), nil
}

// RewriteFrontmatterStatus rewrites the frontmatter `status:` value at
// seedPath to newStatus in place, replacing only the value text. Every other
// byte of the file (line ordering, indentation, line endings, trailing
// whitespace, and all other frontmatter keys and body) is preserved
// (cli/sidekick/change-status#req:seed-status-surface).
//
// The returned string is the ORIGINAL `status:` line content (including its
// line terminator), suitable for a later rollback to undo the mutation.
//
// If the seed has no frontmatter `status:` line, RewriteFrontmatterStatus
// returns ErrFrontmatterStatusNotFound and the file is left untouched.
func RewriteFrontmatterStatus(seedPath string, newStatus string) (string, error) {
	original, err := os.ReadFile(seedPath)
	if err != nil {
		return "", err
	}
	lines := splitKeepTerminators(original)

	idx := findFrontmatterStatusLineIndex(lines)
	if idx < 0 {
		return "", ErrFrontmatterStatusNotFound
	}

	originalLine := lines[idx]
	body, terminator := splitTerminator(originalLine)
	m := fmStatusLineRe.FindStringSubmatch(body)
	indent, trailing := m[1], m[3]
	lines[idx] = fmt.Sprintf("%sstatus: %s%s", indent, newStatus, trailing) + terminator

	stat, err := statFn(seedPath)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(seedPath, joinLines(lines), stat.Mode().Perm()); err != nil {
		return "", err
	}
	return originalLine, nil
}

// findFrontmatterStatusLineIndex returns the index of the `status:` line inside
// a leading `---`-fenced YAML frontmatter block, or -1 when the file has no
// such block or the block carries no `status:` key. The search stops at the
// closing fence so a body line is never mistaken for the status surface.
func findFrontmatterStatusLineIndex(lines []string) int {
	if len(lines) == 0 {
		return -1
	}
	if body, _ := splitTerminator(lines[0]); body != "---" {
		return -1
	}
	for i := 1; i < len(lines); i++ {
		body, _ := splitTerminator(lines[i])
		if body == "---" {
			return -1
		}
		if fmStatusLineRe.MatchString(body) {
			return i
		}
	}
	return -1
}

// splitKeepTerminators splits content on '\n' boundaries, retaining the
// terminator on each line so joinLines round-trips byte-for-byte. Trailing
// data without a final newline is a final element with no terminator.
func splitKeepTerminators(content []byte) []string {
	if len(content) == 0 {
		return nil
	}
	var lines []string
	start := 0
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			lines = append(lines, string(content[start:i+1]))
			start = i + 1
		}
	}
	if start < len(content) {
		lines = append(lines, string(content[start:]))
	}
	return lines
}

// splitTerminator splits a line into (body, terminator). The terminator is ""
// for a final line with no trailing newline; otherwise "\n" or "\r\n".
func splitTerminator(line string) (string, string) {
	if strings.HasSuffix(line, "\r\n") {
		return line[:len(line)-2], "\r\n"
	}
	if strings.HasSuffix(line, "\n") {
		return line[:len(line)-1], "\n"
	}
	return line, ""
}

// joinLines concatenates lines (each retaining its own terminator) into bytes.
func joinLines(lines []string) []byte {
	var buf bytes.Buffer
	for _, ln := range lines {
		buf.WriteString(ln)
	}
	return buf.Bytes()
}
