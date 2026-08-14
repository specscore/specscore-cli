package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/idea"
	"github.com/specscore/specscore-cli/pkg/issue"
)

func TestIssueTransitions_StructuredFormatsAndEdgeCases(t *testing.T) {
	root := setupIssueSpecRoot(t)
	withCwd(t, root)
	if _, stderr, err := runIssue(t, "new", "timeout", "--severity=high"); err != nil {
		t.Fatalf("issue new: %v\nstderr=%s", err, stderr)
	}

	for _, args := range [][]string{
		{"transitions", "--format=json"},
		{"transitions", "--format=yaml"},
		{"transitions", "timeout", "--format=json"},
		{"transitions", "timeout", "--format=yaml"},
	} {
		if out, stderr, err := runIssue(t, args...); err != nil || strings.TrimSpace(out) == "" {
			t.Fatalf("issue %v: out=%q err=%v stderr=%s", args, out, err, stderr)
		}
	}
	if _, _, err := runIssue(t, "transitions", "missing"); err == nil {
		t.Fatal("missing issue unexpectedly resolved")
	}
	if _, _, err := runIssue(t, "transitions", "--format=toml"); err == nil {
		t.Fatal("unsupported format unexpectedly succeeded")
	}
	if _, _, err := runIssue(t, "transitions", "timeout", "--project="+t.TempDir()); err == nil {
		t.Fatal("empty non-project root unexpectedly resolved")
	}

	path := filepath.Join(root, "spec", "issues", "timeout.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(body), "status: open\n", "status: investigating\n", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, stderr, err := runIssue(t, "transitions", "timeout"); err != nil || !strings.Contains(out, "previous: open") {
		t.Fatalf("non-initial status: out=%q err=%v stderr=%s", out, err, stderr)
	}
	body, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(body), "status: investigating\n", "", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, stderr, err := runIssue(t, "transitions", "timeout"); err != nil || !strings.Contains(out, "timeout: open") {
		t.Fatalf("default status: out=%q err=%v stderr=%s", out, err, stderr)
	}
}

func TestTransitionDiscoveryAndParsingFailures(t *testing.T) {
	t.Run("idea discovery", func(t *testing.T) {
		stageActiveIdea(t, "auth", "Draft", "")
		original := discoverIdeasForTransitions
		discoverIdeasForTransitions = func(string) ([]idea.Discovered, error) {
			return nil, errors.New("injected idea discovery failure")
		}
		t.Cleanup(func() { discoverIdeasForTransitions = original })
		if _, _, err := runIdea(t, "transitions", "missing"); err == nil || !strings.Contains(err.Error(), "discovering idea") {
			t.Fatalf("idea discovery error = %v", err)
		}
	})

	t.Run("issue discovery", func(t *testing.T) {
		root := setupIssueSpecRoot(t)
		withCwd(t, root)
		original := discoverIssuesForTransitions
		discoverIssuesForTransitions = func(string) ([]issue.Discovered, error) {
			return nil, errors.New("injected issue discovery failure")
		}
		t.Cleanup(func() { discoverIssuesForTransitions = original })
		if _, _, err := runIssue(t, "transitions", "missing"); err == nil || !strings.Contains(err.Error(), "discovering issues") {
			t.Fatalf("issue discovery error = %v", err)
		}
	})

	t.Run("issue parsing", func(t *testing.T) {
		root := setupIssueSpecRoot(t)
		withCwd(t, root)
		if _, stderr, err := runIssue(t, "new", "timeout", "--severity=high"); err != nil {
			t.Fatalf("issue new: %v\nstderr=%s", err, stderr)
		}
		original := parseIssueForTransitions
		parseIssueForTransitions = func(string) (*issue.Issue, error) {
			return nil, errors.New("injected issue parse failure")
		}
		t.Cleanup(func() { parseIssueForTransitions = original })
		if _, _, err := runIssue(t, "transitions", "timeout"); err == nil || !strings.Contains(err.Error(), "reading ") {
			t.Fatalf("issue parse error = %v", err)
		}
	})
}

func TestSidekickTransitions_StructuredFormatsArchivedAndFailures(t *testing.T) {
	root := stageQueuedSeed(t, "queue")
	for _, args := range [][]string{
		{"transitions", "--format=json"},
		{"transitions", "--format=yaml"},
		{"transitions", "queue", "--format=json"},
		{"transitions", "queue", "--format=yaml"},
	} {
		if out, stderr, err := runSidekick(t, args...); err != nil || strings.TrimSpace(out) == "" {
			t.Fatalf("sidekick %v: out=%q err=%v stderr=%s", args, out, err, stderr)
		}
	}
	if _, _, err := runSidekick(t, "transitions", "missing"); err == nil {
		t.Fatal("missing seed unexpectedly resolved")
	}
	if _, _, err := runSidekick(t, "transitions", "--format=toml"); err == nil {
		t.Fatal("unsupported format unexpectedly succeeded")
	}
	if _, _, err := runSidekick(t, "transitions", "queue", "--project="+t.TempDir()); err == nil {
		t.Fatal("empty non-project root unexpectedly resolved")
	}

	active := filepath.Join(root, "spec", "ideas", "seeds", "queue.md")
	archived := filepath.Join(root, "spec", "ideas", "archived", "queue.md")
	if err := os.MkdirAll(filepath.Dir(archived), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(active, archived); err != nil {
		t.Fatal(err)
	}
	if out, stderr, err := runSidekick(t, "transitions", "queue"); err != nil || !strings.Contains(out, "queue: Queued") {
		t.Fatalf("archived seed: out=%q err=%v stderr=%s", out, err, stderr)
	}

	broken := filepath.Join(root, "spec", "ideas", "seeds", "broken.md")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runSidekick(t, "transitions", "broken"); err == nil {
		t.Fatal("directory seed unexpectedly parsed")
	}
}
