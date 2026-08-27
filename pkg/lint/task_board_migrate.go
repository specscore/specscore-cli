package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/pkg/task"
)

// migrateTaskBoardStatus backfills a missing body **Status:** line into every
// tasks/<slug>/README.md on the project's flat task board (<projectRoot>/tasks/),
// taking the value from that board's existing tasks/README.md index row.
//
// This is a ONE-TIME BOOTSTRAP, not a precedent for "index wins": once a task
// file carries its own **Status:** line, that line — never the index — is
// authoritative for every subsequent read and rewrite (see reconcile.go and
// https://github.com/specscore/specscore/blob/main/spec/features/index/README.md#req-file-authoritative-over-index). The
// bootstrap exists only because, before this migration runs, the index is the
// ONLY place a value exists at all; there is nothing else to seed the file
// from. From the next transition onward, `task change-status` writes the file
// only, and `specscore spec lint --fix` (task-index-row-sync) reconciles the
// index FROM the file, never the reverse.
//
// A task whose README already has a **Status:** line is left byte-unchanged
// (idempotent — re-running is a no-op). A task with no board row to source a
// value from is left unchanged too: migrate never invents a status.
//
// This board lives outside spec/ (a sibling of it), which is why it is not
// covered by the docTypeTargets/adherence-footer walk machinery: that walk
// covers spec/plans/**/tasks/*/README.md (nested plan sub-tasks) — a
// different location from the flat project-root board this function targets.
// Returned paths are relative to projectRoot (not specRoot, since the board
// lives outside spec/), slash-separated, sorted.
func migrateTaskBoardStatus(projectRoot string) ([]string, error) {
	tasksDir := filepath.Join(projectRoot, "tasks")
	if info, err := os.Stat(tasksDir); err != nil || !info.IsDir() {
		return nil, nil
	}

	boardPath := filepath.Join(tasksDir, "README.md")
	boardData, err := os.ReadFile(boardPath)
	if err != nil {
		// No board file: nothing to source a value from.
		return nil, nil
	}
	bv, err := task.ParseBoard(boardData)
	if err != nil {
		return nil, fmt.Errorf("parsing task board %s: %w", boardPath, err)
	}
	statusBySlug := make(map[string]string, len(bv.Rows))
	for _, r := range bv.Rows {
		statusBySlug[r.Task] = string(r.Status)
	}

	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		return nil, fmt.Errorf("reading tasks directory %s: %w", tasksDir, err)
	}

	var changed []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		slug := e.Name()
		readmePath := filepath.Join(tasksDir, slug, "README.md")
		content, readErr := os.ReadFile(readmePath)
		if readErr != nil {
			continue // no README under this directory entry; not a task
		}
		if extractBodyStatus(content) != "" {
			continue // already has a Status line; migrate never overwrites it
		}
		status, ok := statusBySlug[slug]
		if !ok || status == "" {
			continue // no board row to source a value from; never invented
		}
		updated, ok := insertTaskBodyStatus(content, status)
		if !ok {
			continue // not a recognizable "# Title" file; leave it for a human
		}
		if err := os.WriteFile(readmePath, updated, 0o644); err != nil {
			return nil, fmt.Errorf("writing %s: %w", readmePath, err)
		}
		rel, _ := filepath.Rel(projectRoot, readmePath)
		changed = append(changed, filepath.ToSlash(rel))
	}
	sort.Strings(changed)
	return changed, nil
}

// insertTaskBodyStatus inserts a "**Status:** <status>" line immediately
// after the task README's leading "# Title" line, followed by a blank line —
// the exact shape internal/cli.runTaskNew now scaffolds via
// task.RenderTaskFile. Returns ok=false when the content does not start with
// a title line, so the caller can skip a file it cannot safely rewrite rather
// than guessing at its structure.
func insertTaskBodyStatus(content []byte, status string) ([]byte, bool) {
	s := string(content)
	if !strings.HasPrefix(s, "# ") {
		return nil, false
	}
	nl := strings.IndexByte(s, '\n')
	if nl < 0 {
		return nil, false
	}
	title := s[:nl]
	rest := strings.TrimPrefix(s[nl+1:], "\n")
	return []byte(title + "\n\n**Status:** " + status + "\n\n" + rest), true
}
