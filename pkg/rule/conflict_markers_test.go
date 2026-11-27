package rule

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/conflictmarker"
)

// AC: writers-refuse-conflicted-index — ReadIndex skips non-table lines, so
// UpsertRow/RemoveRow -> WriteIndexRows used to regenerate the whole `## Rules`
// table and silently erase a conflict. Reading stays allowed (rule list and
// rule show keep working); every writer refuses, byte-identically.
func TestRuleIndexWriters_RefuseConflictedIndex(t *testing.T) {
	root := setupRulesTree(t)
	path := IndexPath(RulesDir(root))
	conflicted := strings.Replace(IndexContent(), IndexSeparatorRow+"\n", IndexSeparatorRow+"\n<<<<<<< HEAD\n=======\n>>>>>>> topic\n", 1)
	if conflicted == IndexContent() {
		t.Fatal("fixture did not inject the conflict")
	}
	if err := os.WriteFile(path, []byte(conflicted), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := ReadIndex(path)
	if err != nil || len(report.Conflicts) != 3 {
		t.Fatalf("ReadIndex = %+v, %v; want a readable report naming 3 markers", report, err)
	}
	if err := WriteIndexRows(path, nil, nil); !errors.Is(err, conflictmarker.ErrConflicted) {
		t.Fatalf("WriteIndexRows err = %v", err)
	}
	if err := UpsertRow(RulesDir(root), NewRow("x", false, "Stated", "s", nil, "e", "c", nil)); !errors.Is(err, conflictmarker.ErrConflicted) {
		t.Fatalf("UpsertRow err = %v", err)
	}
	// RemoveRow refuses even when the slug has no row (nothing to drop yet).
	if err := RemoveRow(RulesDir(root), "x"); !errors.Is(err, conflictmarker.ErrConflicted) {
		t.Fatalf("RemoveRow err = %v", err)
	}
	if got := readIndexFile(t, root); got != conflicted {
		t.Fatalf("index rewritten:\n%s", got)
	}
}
