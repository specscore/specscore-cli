package ideapromote

import (
	"path/filepath"
	"strings"
)

// CrossRepoPromote performs the cross-repo path: create the new Idea at
// spec/ideas/<slug>.md by copy+transform, then git-mv the seed to
// spec/ideas/archived/<slug>.md with its frontmatter `status` set to
// `promoted` and a `promoted_to: <slug>` key added. It NEVER sets
// `deprecated`. Returns the absolute Idea path and the absolute archived
// seed path.
//
// The new Idea is written first (so a write failure leaves the seed
// untouched), then the seed is moved+marked.
func CrossRepoPromote(specRoot, slug string, transformed []byte, seedBody string) (ideaAbs, archivedAbs string, err error) {
	paths := PathsFor(slug)

	// Write the new Idea.
	ideaAbs = filepath.Join(specRoot, paths.IdeaRel)
	if err := writeFileAbs(ideaAbs, transformed); err != nil {
		return "", "", err
	}
	if err := gitStage(specRoot, paths.IdeaRel); err != nil {
		return "", "", err
	}

	// git-mv the seed to the archived path (records the rename for
	// history), then overwrite it with its frontmatter-marked body.
	if err := gitMV(specRoot, paths.SeedRel, paths.ArchivedSeedRel); err != nil {
		return "", "", err
	}
	archivedAbs = filepath.Join(specRoot, paths.ArchivedSeedRel)
	marked := markArchivedSeed(seedBody, slug)
	if err := writeFileAbs(archivedAbs, []byte(marked)); err != nil {
		return "", "", err
	}
	if err := gitStage(specRoot, paths.ArchivedSeedRel); err != nil {
		return "", "", err
	}
	return ideaAbs, archivedAbs, nil
}

// markArchivedSeed rewrites the seed's YAML frontmatter so `status` is
// `promoted` and a `promoted_to: <slug>` key is present. `deprecated` is
// never set. When the seed has no frontmatter fence, one is synthesized.
func markArchivedSeed(seedBody, slug string) string {
	lines := strings.Split(seedBody, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		// No frontmatter: synthesize a minimal one.
		fm := "---\nstatus: promoted\npromoted_to: " + slug + "\n---\n"
		return fm + seedBody
	}

	// Find the closing fence.
	closeIdx := -1
	for j := 1; j < len(lines); j++ {
		if strings.TrimSpace(lines[j]) == "---" {
			closeIdx = j
			break
		}
	}
	if closeIdx < 0 {
		// Malformed (no closing fence): prepend keys after the opener.
		out := append([]string{lines[0], "status: promoted", "promoted_to: " + slug}, lines[1:]...)
		return strings.Join(out, "\n")
	}

	statusSet := false
	promotedToSet := false
	for j := 1; j < closeIdx; j++ {
		key := frontmatterKey(lines[j])
		switch key {
		case "status":
			lines[j] = "status: promoted"
			statusSet = true
		case "promoted_to":
			lines[j] = "promoted_to: " + slug
			promotedToSet = true
		}
	}

	var inserts []string
	if !statusSet {
		inserts = append(inserts, "status: promoted")
	}
	if !promotedToSet {
		inserts = append(inserts, "promoted_to: "+slug)
	}
	if len(inserts) > 0 {
		out := append([]string{}, lines[:closeIdx]...)
		out = append(out, inserts...)
		out = append(out, lines[closeIdx:]...)
		lines = out
	}
	return strings.Join(lines, "\n")
}

// frontmatterKey returns the YAML key of a `key: value` line, or "".
func frontmatterKey(line string) string {
	if k := strings.IndexByte(line, ':'); k >= 0 {
		return strings.TrimSpace(line[:k])
	}
	return ""
}
