package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The manifest is executable: stale command paths, stale AI-skill anchors,
// and accepted-but-unregistered flags fail here.  The docs file is the local
// source skill until the shared skill-bundle repository owns distribution.
type lessonCapability struct {
	ID          string   `json:"id"`
	Command     []string `json:"command"`
	Flags       []string `json:"flags"`
	HelpAnchor  string   `json:"help_anchor"`
	SkillAnchor string   `json:"skill_anchor"`
	Test        string   `json:"test"`
}

func TestLessonCapabilityManifestMatchesCommandsAndSkill(t *testing.T) {
	root := filepath.Join("..", "..")
	b, err := os.ReadFile(filepath.Join(root, "docs", "capabilities", "lessons.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rows []lessonCapability
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ID == "" || row.Test == "" || row.HelpAnchor == "" || row.SkillAnchor == "" {
			t.Fatalf("incomplete capability row: %#v", row)
		}
		base := lessonCommand()
		if row.Command[0] == "spec" || row.Command[0] == "event" {
			base = newRootCommand()
		}
		cmd, _, err := base.Find(row.Command)
		if err != nil {
			t.Fatalf("%s command %v: %v", row.ID, row.Command, err)
		}
		for _, flag := range row.Flags {
			if cmd.Flags().Lookup(flag) == nil {
				t.Errorf("%s declares accepted flag --%s but command does not register it", row.ID, flag)
			}
		}
		if !strings.Contains(cmd.Long, "Docs: docs/agent-lessons.md") && row.Command[0] != "spec" && row.Command[0] != "event" {
			t.Errorf("%s command help lacks its declared docs anchor", row.ID)
		}
		parts := strings.SplitN(row.SkillAnchor, "#", 2)
		if len(parts) != 2 {
			t.Errorf("%s malformed skill anchor %q", row.ID, row.SkillAnchor)
			continue
		}
		skill, err := os.ReadFile(filepath.Join(root, parts[0]))
		if err != nil {
			t.Errorf("%s skill missing: %v", row.ID, err)
			continue
		}
		if !strings.Contains(strings.ToLower(string(skill)), "## "+strings.ReplaceAll(parts[1], "-", " ")) {
			t.Errorf("%s stale skill anchor %s", row.ID, row.SkillAnchor)
		}
	}
}
