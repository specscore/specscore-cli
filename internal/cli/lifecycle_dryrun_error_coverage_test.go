package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/feature"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
	"github.com/specscore/specscore-cli/pkg/projectdef"
)

type failingYAMLMarshaler struct{}

func (failingYAMLMarshaler) MarshalYAML() (any, error) {
	return nil, errors.New("injected yaml marshal failure")
}

func TestTransitionResolvers_CoverArchivedAndProjectFailures(t *testing.T) {
	t.Run("decision active", func(t *testing.T) {
		_, slug := stageDecisionCLI(t, "auth", "Draft")
		cmd := decisionTransitionsCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs([]string{slug})
		if err := cmd.Execute(); err != nil || !strings.Contains(out.String(), slug+": Draft") {
			t.Fatalf("active decision: out=%q err=%v", out.String(), err)
		}
	})
	t.Run("decision archived", func(t *testing.T) {
		root, slug := stageDecisionCLI(t, "auth", "Draft")
		active := filepath.Join(root, "spec", "decisions", slug+".md")
		archived := filepath.Join(root, "spec", "decisions", "archived", slug+".md")
		if err := os.MkdirAll(filepath.Dir(archived), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(active, archived); err != nil {
			t.Fatal(err)
		}
		if out, stderr, err := runDecision(t, "transitions", slug); err != nil || !strings.Contains(out, slug+": Draft") {
			t.Fatalf("archived decision: out=%q err=%v stderr=%s", out, err, stderr)
		}
	})
	t.Run("decision project", func(t *testing.T) {
		if _, _, err := runDecision(t, "transitions", "missing", "--project=/definitely/missing"); err == nil {
			t.Fatal("missing project unexpectedly resolved")
		}
	})
	t.Run("idea proposal", func(t *testing.T) {
		root := stageActiveIdea(t, "auth", "Draft", "")
		active := filepath.Join(root, "spec", "ideas", "auth.md")
		if err := os.Remove(active); err != nil {
			t.Fatal(err)
		}
		proposal := filepath.Join(root, "spec", "features", "target", "proposals", "auth.md")
		if err := os.MkdirAll(filepath.Dir(proposal), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(proposal, []byte("# Proposal: Auth\n\n**Status:** Draft\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if out, stderr, err := runIdea(t, "transitions", "auth"); err != nil || !strings.Contains(out, "auth: Draft") {
			t.Fatalf("proposal: out=%q err=%v stderr=%s", out, err, stderr)
		}
	})
	for _, tt := range []struct {
		name string
		run  func(t *testing.T, args ...string) (string, string, error)
	}{
		{"feature", runFeature},
		{"idea", runIdea},
		{"lesson", runLesson},
		{"plan", runPlan},
		{"task", runTask},
	} {
		t.Run(tt.name+" project", func(t *testing.T) {
			if _, _, err := tt.run(t, "transitions", "missing", "--project=/definitely/missing"); err == nil {
				t.Fatal("missing project unexpectedly resolved")
			}
		})
	}
}

func TestWriteEnrichedOutput_RendersParkedFeature(t *testing.T) {
	var out bytes.Buffer
	parked := true
	if err := writeEnrichedOutput(&out, []*feature.EnrichedFeature{{Path: "auth", Parked: &parked}}, []string{"parked"}, "text"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "parked=true") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestTransitionEncoders_SurfaceMarshalFailures(t *testing.T) {
	var out bytes.Buffer
	if err := printJSON(&out, make(chan int)); err == nil {
		t.Fatal("printJSON accepted an unsupported value")
	}
	if err := printYAML(&out, func() {}); err == nil {
		t.Fatal("printYAML accepted an unsupported value")
	}
	if err := printYAML(&out, failingYAMLMarshaler{}); err == nil {
		t.Fatal("printYAML suppressed a marshal error")
	}
}

func TestTransitionsCommand_SurfacesStatusReadFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "README.md")
	if err := os.WriteFile(path, []byte("# Missing status\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := transitionsCommand(lifecycle.KindFeature, "feature_id", "test", func(string, string) (string, error) {
		return path, nil
	})
	cmd.SetArgs([]string{"auth"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "reading feature status") {
		t.Fatalf("status read error = %v", err)
	}
}

func TestLessonNew_OccurrenceMarkerFailures(t *testing.T) {
	setup := func(t *testing.T) string {
		t.Helper()
		root := setupSpecRoot(t)
		if err := projectdef.WriteSpecConfig(root, lessonTestConfig()); err != nil {
			t.Fatal(err)
		}
		return root
	}

	t.Run("force marker preflight", func(t *testing.T) {
		root := setup(t)
		create := lessonNewCommand()
		setLessonCommandFlags(t, create, map[string]string{"owner": "codex", "project": root})
		requireCLISuccess(t, runLessonNewWithDeps(create, []string{"marker-force"}, defaultLessonCLIDeps()))

		keep := filepath.Join(root, "spec", "lessons", "marker-force", "occurrences", lesson.OccurrenceStoreKeepFile)
		deps := defaultLessonCLIDeps()
		stat := deps.fs.stat
		deps.fs.stat = func(path string) (os.FileInfo, error) {
			if path == keep {
				return nil, errors.New("injected marker stat failure")
			}
			return stat(path)
		}
		force := lessonNewCommand()
		setLessonCommandFlags(t, force, map[string]string{"owner": "codex", "project": root, "force": "true"})
		requireCLIError(t, runLessonNewWithDeps(force, []string{"marker-force"}, deps))
	})

	t.Run("new marker stat", func(t *testing.T) {
		root := setup(t)
		keep := filepath.Join(root, "spec", "lessons", "marker-stat", "occurrences", lesson.OccurrenceStoreKeepFile)
		deps := defaultLessonCLIDeps()
		stat := deps.fs.stat
		deps.fs.stat = func(path string) (os.FileInfo, error) {
			if path == keep {
				return nil, errors.New("injected marker stat failure")
			}
			return stat(path)
		}
		cmd := lessonNewCommand()
		setLessonCommandFlags(t, cmd, map[string]string{"owner": "codex", "project": root})
		requireCLIError(t, runLessonNewWithDeps(cmd, []string{"marker-stat"}, deps))
	})

	t.Run("marker publication", func(t *testing.T) {
		root := setup(t)
		keep := filepath.Join(root, "spec", "lessons", "marker-publish", "occurrences", lesson.OccurrenceStoreKeepFile)
		deps := defaultLessonCLIDeps()
		publish := deps.publishExclusive
		deps.publishExclusive = func(path string, body []byte, mode os.FileMode) error {
			if path == keep {
				return errors.New("injected marker publication failure")
			}
			return publish(path, body, mode)
		}
		cmd := lessonNewCommand()
		setLessonCommandFlags(t, cmd, map[string]string{"owner": "codex", "project": root})
		requireCLIError(t, runLessonNewWithDeps(cmd, []string{"marker-publish"}, deps))
	})
}

func TestLessonAgentsCommand_SurfacesCanonicalRootFailure(t *testing.T) {
	root := setupSpecRoot(t)
	original := canonicalLessonAgentsProjectRootForCommand
	canonicalLessonAgentsProjectRootForCommand = func(string) (string, error) {
		return "", errors.New("injected canonical root failure")
	}
	t.Cleanup(func() { canonicalLessonAgentsProjectRootForCommand = original })
	if _, _, err := runLesson(t, "agents", "missing", "--project="+root); err == nil {
		t.Fatal("canonical root failure was suppressed")
	}
}

func TestOwnedMarkerIdentityFences_RemainingBranches(t *testing.T) {
	t.Run("identity changes after second pair verification", func(t *testing.T) {
		dir := t.TempDir()
		prepared := filepath.Join(dir, "task.prepared")
		expected := []byte("owned marker\n")
		if err := os.WriteFile(prepared, expected, 0o600); err != nil {
			t.Fatal(err)
		}
		ops := defaultOwnedMarkerOps()
		sameFile := ops.sameFile
		calls := 0
		ops.sameFile = func(left, right os.FileInfo) bool {
			calls++
			if calls == 8 {
				return false
			}
			return sameFile(left, right)
		}
		if err := removeOwnedFileDurableWithOps(prepared, expected, ops); err == nil || !strings.Contains(err.Error(), "identity changed") {
			t.Fatalf("identity change error = %v (sameFile calls=%d)", err, calls)
		}
	})

	t.Run("second lstat fails", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "marker")
		expected := []byte("owned marker\n")
		if err := os.WriteFile(path, expected, 0o600); err != nil {
			t.Fatal(err)
		}
		ops := defaultOwnedMarkerOps()
		lstat := ops.lstat
		calls := 0
		boom := errors.New("injected second lstat failure")
		ops.lstat = func(path string) (os.FileInfo, error) {
			calls++
			if calls == 2 {
				return nil, boom
			}
			return lstat(path)
		}
		if _, err := verifyOwnedMarkerIdentity(ops, path, expected); !errors.Is(err, boom) {
			t.Fatalf("second lstat error = %v", err)
		}
	})

	t.Run("same bytes have different identity", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "marker")
		expected := []byte("owned marker\n")
		if err := os.WriteFile(path, expected, 0o600); err != nil {
			t.Fatal(err)
		}
		ops := defaultOwnedMarkerOps()
		ops.sameFile = func(os.FileInfo, os.FileInfo) bool { return false }
		if _, err := verifyOwnedMarkerIdentity(ops, path, expected); err == nil || !strings.Contains(err.Error(), "identity changed") {
			t.Fatalf("identity error = %v", err)
		}
	})
}

func TestChangeStatusDryRun_PropagatesMutationErrors(t *testing.T) {
	tests := []struct {
		name  string
		stage func(t *testing.T)
		run   func(t *testing.T, args ...string) (string, string, error)
		args  []string
	}{
		{
			name:  "decision",
			stage: func(t *testing.T) { stageDecisionCLI(t, "auth", "Draft") },
			run:   runDecision,
			args:  []string{"change-status", "0001-missing", "--to=approved", "--dry-run"},
		},
		{
			name:  "issue",
			stage: func(t *testing.T) { root := setupIssueSpecRoot(t); withCwd(t, root) },
			run:   runIssue,
			args:  []string{"change-status", "missing", "--to=investigating", "--dry-run"},
		},
		{
			name:  "lesson",
			stage: func(t *testing.T) { stageLesson(t, "auth", "Recorded") },
			run:   runLesson,
			args:  []string{"change-status", "missing", "--to=stated", "--dry-run"},
		},
		{
			name:  "plan",
			stage: func(t *testing.T) { stagePlan(t, "auth", "Draft") },
			run:   runPlan,
			args:  []string{"change-status", "missing", "--to=in review", "--dry-run"},
		},
		{
			name:  "sidekick",
			stage: func(t *testing.T) { stageQueuedSeed(t, "auth") },
			run:   runSidekick,
			args:  []string{"change-status", "missing", "--to=implemented", "--note=x", "--dry-run"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.stage(t)
			if _, _, err := tt.run(t, tt.args...); err == nil {
				t.Fatal("dry-run mutation unexpectedly succeeded")
			}
		})
	}
}
