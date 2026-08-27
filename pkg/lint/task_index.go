package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/pkg/task"
)

// taskIndexChecker dispatches the task-index-row-sync rule: the flat
// project-root tasks/README.md board's Status cell for each task MUST mirror
// that task's own tasks/<slug>/README.md **Status:** line. It mirrors the
// established feature-index-row-sync / DI-row-content-sync pattern for a
// board that lives outside spec/, so it needs projectRoot rather than
// deriving everything from specRoot.
type taskIndexChecker struct {
	projectRoot string
	pending     []Reconciliation
}

func newTaskIndexChecker(projectRoot string) *taskIndexChecker {
	return &taskIndexChecker{projectRoot: projectRoot}
}

func (c *taskIndexChecker) name() string     { return "task-index-row-sync" }
func (c *taskIndexChecker) severity() string { return "error" }

func (c *taskIndexChecker) check(specRoot string) ([]Violation, error) {
	vs, _, _ := taskIndexRules(c.projectRoot, specRoot, false)
	return vs, nil
}

// fix rewrites the tasks/README.md board's Status cell from each task's own
// **Status:** line — the file is authoritative, the board row is derived (see
// https://github.com/specscore/specscore/blob/main/spec/features/index/README.md#req-file-authoritative-over-index / spec/features/index/README.md).
// `task change-status` intentionally never touches the board in the same
// write (mirroring how feature/idea/decision change-status leave their
// indexes to a later `spec lint --fix`); this rule is what reconciles it.
// Every reconciled row is recorded as a Reconciliation (see reconcile.go) so
// the correction is loud, not silent: the board could have been the side that
// was actually right, and silently overwriting it would destroy that signal.
func (c *taskIndexChecker) fix(specRoot string) error {
	_, _, rc := taskIndexRules(c.projectRoot, specRoot, true)
	c.pending = append(c.pending, rc...)
	return nil
}

func (c *taskIndexChecker) takeReconciliations() []Reconciliation {
	rc := c.pending
	c.pending = nil
	return rc
}

// taskIndexRules enforces task-index-row-sync. A task file with no
// **Status:** line yet (not migrated) is skipped — that is `specscore
// migrate`'s job, never this rule's (pkg/lint.migrateTaskBoardStatus). A
// file status the legacy board vocabulary cannot represent (task.StatusEmojis
// has no "complete" — a completion writes the lifecycle vocabulary's
// "complete", while the board's parser only knows "completed"; see the
// separately tracked unify-task-status-vocabulary gap in specscore/specscore)
// is also skipped rather than written as a cell the board's own parser would
// then reject — the board is left stale for that one row until the
// vocabulary work lands, which is honest and crash-safe, unlike guessing at a
// mapping between the two vocabularies.
func taskIndexRules(projectRoot, specRoot string, fix bool) ([]Violation, bool, []Reconciliation) {
	if projectRoot == "" {
		projectRoot = lintProjectRoot("", specRoot)
	}
	tasksDir := filepath.Join(projectRoot, "tasks")
	if info, err := os.Stat(tasksDir); err != nil || !info.IsDir() {
		return nil, false, nil
	}
	boardPath := filepath.Join(tasksDir, "README.md")
	boardData, err := os.ReadFile(boardPath)
	if err != nil {
		return nil, false, nil
	}
	bv, err := task.ParseBoard(boardData)
	if err != nil {
		return nil, false, nil // a different rule (board-format) owns a malformed board
	}

	type drift struct {
		slug, from, to string
	}
	var drifts []drift
	for _, row := range bv.Rows {
		content, err := os.ReadFile(filepath.Join(tasksDir, row.Task, "README.md"))
		if err != nil {
			continue // orphan row; a different concern
		}
		fileStatus := extractBodyStatus(content)
		if fileStatus == "" {
			continue // not yet migrated
		}
		if fileStatus == string(row.Status) {
			continue
		}
		if _, recognized := task.StatusEmojis[task.TaskStatus(fileStatus)]; !recognized {
			continue // legacy board vocabulary cannot represent this value yet
		}
		drifts = append(drifts, drift{slug: row.Task, from: string(row.Status), to: fileStatus})
	}
	if len(drifts) == 0 {
		return nil, false, nil
	}
	sort.Slice(drifts, func(i, j int) bool { return drifts[i].slug < drifts[j].slug })

	// The board lives outside spec/, so its path relative to specRoot climbs
	// out via "..": still a stable, readable identifier for a report.
	rel, relErr := filepath.Rel(specRoot, boardPath)
	if relErr != nil {
		rel = boardPath
	}

	if fix {
		updates := make(map[string]string, len(drifts))
		for _, d := range drifts {
			updates[d.slug] = d.to
		}
		if err := rewriteTaskBoardRows(boardPath, updates); err != nil {
			slugs := make([]string, 0, len(drifts))
			for _, d := range drifts {
				slugs = append(slugs, d.slug)
			}
			return []Violation{{
				File: rel, Line: 0, Severity: "error", Rule: "task-index-row-sync",
				Message: fmt.Sprintf("task board rows drifted from task READMEs: %s (fix failed: %v)", strings.Join(slugs, ", "), err),
			}}, false, nil
		}
		reconciled := make([]Reconciliation, 0, len(drifts))
		for _, d := range drifts {
			reconciled = append(reconciled, Reconciliation{
				Rule:     "task-index-row-sync",
				Artifact: d.slug,
				Changes:  []FieldChange{{Field: "status", IndexValue: d.from, FileValue: d.to}},
			})
		}
		return nil, true, reconciled
	}

	vs := make([]Violation, 0, len(drifts))
	for _, d := range drifts {
		vs = append(vs, Violation{
			File: rel, Line: 0, Severity: "error", Rule: "task-index-row-sync",
			Message: fmt.Sprintf("task board row for %q is stale: index has %q, task file has %q (run `specscore spec lint --fix`)", d.slug, d.from, d.to),
		})
	}
	return vs, false, nil
}

// rewriteTaskBoardRows rewrites ONLY the Status cell of each data row whose
// slug appears in updates, preserving every other byte of the board file
// (including any content the fixed-shape task.RenderBoard would not
// reproduce, and every other cell of an updated row). Mirrors
// rewriteFeatureIndexRows's surgical-cell-rewrite approach rather than a
// wholesale task.RenderBoard regeneration.
func rewriteTaskBoardRows(path string, updates map[string]string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	inTable := false
	changed := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inTable {
			if strings.HasPrefix(trimmed, "|") && strings.Contains(trimmed, "---") {
				inTable = true
			}
			continue
		}
		if trimmed == "" || !strings.HasPrefix(trimmed, "|") {
			break // table ended
		}
		cells, ok := splitMarkdownTableCells(trimmed)
		if !ok || len(cells) != 7 {
			continue
		}
		slug := task.ExtractSlug(cells[0])
		newStatus, ok := updates[slug]
		if !ok {
			continue
		}
		cells[1] = fmt.Sprintf(" %s `%s` ", task.StatusEmoji(task.TaskStatus(newStatus)), newStatus)
		lines[i] = "|" + strings.Join(cells, "|") + "|"
		changed = true
	}
	if !changed {
		return nil
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}
