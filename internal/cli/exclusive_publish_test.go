package cli

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/specscore/specscore-cli/pkg/event"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/projectdef"
)

type exclusiveFaultFile struct {
	exclusivePublishFile
	role  string
	fail  string
	short bool
}

func (f exclusiveFaultFile) Chmod(mode os.FileMode) error {
	if f.fail == f.role+"-chmod" {
		return errors.New(f.fail)
	}
	return f.exclusivePublishFile.Chmod(mode)
}
func (f exclusiveFaultFile) Write(data []byte) (int, error) {
	if f.fail == f.role+"-write" {
		return 0, errors.New(f.fail)
	}
	if f.short {
		return len(data) - 1, nil
	}
	return f.exclusivePublishFile.Write(data)
}
func (f exclusiveFaultFile) Sync() error {
	if f.fail == f.role+"-sync" {
		return errors.New(f.fail)
	}
	return f.exclusivePublishFile.Sync()
}
func (f exclusiveFaultFile) Close() error {
	if f.fail == f.role+"-close" {
		return errors.New(f.fail)
	}
	return f.exclusivePublishFile.Close()
}

func exclusiveFaultOps(fail string) exclusivePublishOps {
	ops := defaultExclusivePublishOps()
	realMkdir, realCreate, realLink, realOpen := ops.mkdirAll, ops.createTemp, ops.link, ops.open
	ops.mkdirAll = func(path string, mode os.FileMode) error {
		if fail == "mkdir" {
			return errors.New(fail)
		}
		return realMkdir(path, mode)
	}
	ops.createTemp = func(dir, pattern string) (exclusivePublishFile, error) {
		if fail == "create" {
			return nil, errors.New(fail)
		}
		f, err := realCreate(dir, pattern)
		if err != nil {
			return nil, err
		}
		return exclusiveFaultFile{exclusivePublishFile: f, role: "file", fail: fail, short: fail == "short-write"}, nil
	}
	ops.link = func(old, new string) error {
		if fail == "link" {
			return errors.New(fail)
		}
		return realLink(old, new)
	}
	ops.open = func(path string) (exclusivePublishFile, error) {
		if fail == "dir-open" {
			return nil, errors.New(fail)
		}
		f, err := realOpen(path)
		if err != nil {
			return nil, err
		}
		return exclusiveFaultFile{exclusivePublishFile: f, role: "dir", fail: fail}, nil
	}
	return ops
}

func TestExclusivePublishPreservesConcurrentForeignTargetAndClassifiesVisibility(t *testing.T) {
	t.Run("target appears before link", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "README.md")
		foreign := []byte("foreign target\n")
		ops := defaultExclusivePublishOps()
		realLink := ops.link
		ops.link = func(old, new string) error {
			if err := os.WriteFile(new, foreign, 0o600); err != nil {
				return err
			}
			return realLink(old, new)
		}
		err := publishFileExclusiveWithOps(target, []byte("owned\n"), 0o644, ops)
		if err == nil || !errors.Is(err, fs.ErrExist) || lesson.MutationOutcomeOf(err) != lesson.MutationPrePublication {
			t.Fatalf("exclusive collision = %v, outcome=%v", err, lesson.MutationOutcomeOf(err))
		}
		if got, readErr := os.ReadFile(target); readErr != nil || !bytes.Equal(got, foreign) {
			t.Fatalf("foreign target was overwritten: %q, %v", got, readErr)
		}
		entries, readErr := os.ReadDir(dir)
		if readErr != nil || len(entries) != 1 || entries[0].Name() != "README.md" {
			t.Fatalf("collision cleanup leaked temp files: %#v, %v", entries, readErr)
		}
	})

	t.Run("replacement after link", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "README.md")
		foreign := []byte("foreign replacement\n")
		ops := defaultExclusivePublishOps()
		realLink := ops.link
		ops.link = func(old, new string) error {
			if err := realLink(old, new); err != nil {
				return err
			}
			if err := os.Remove(new); err != nil {
				return err
			}
			return os.WriteFile(new, foreign, 0o600)
		}
		ops.open = func(string) (exclusivePublishFile, error) { return nil, errors.New("directory fence") }
		err := publishFileExclusiveWithOps(target, []byte("owned\n"), 0o644, ops)
		if err == nil || lesson.MutationOutcomeOf(err) != lesson.MutationUncertain {
			t.Fatalf("post-link replacement = %v, outcome=%v", err, lesson.MutationOutcomeOf(err))
		}
		if got, readErr := os.ReadFile(target); readErr != nil || !bytes.Equal(got, foreign) {
			t.Fatalf("foreign replacement was deleted: %q, %v", got, readErr)
		}
	})
}

func TestExclusivePublishFaultBoundariesAreTypedAndNeverDeletePublishedState(t *testing.T) {
	for _, fault := range []string{"mkdir", "create", "file-chmod", "file-write", "short-write", "file-sync", "file-close", "link"} {
		t.Run(fault, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "child", "README.md")
			err := publishFileExclusiveWithOps(target, []byte("owned\n"), 0o640, exclusiveFaultOps(fault))
			if err == nil || lesson.MutationOutcomeOf(err) != lesson.MutationPrePublication {
				t.Fatalf("prepublication %s = %v, outcome=%v", fault, err, lesson.MutationOutcomeOf(err))
			}
			if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
				t.Fatalf("%s published target: %v", fault, statErr)
			}
		})
	}
	for _, fault := range []string{"dir-open", "dir-sync", "dir-close"} {
		t.Run(fault, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "child", "README.md")
			err := publishFileExclusiveWithOps(target, []byte("owned\n"), 0o640, exclusiveFaultOps(fault))
			if err == nil || lesson.MutationOutcomeOf(err) != lesson.MutationUncertain {
				t.Fatalf("postpublication %s = %v, outcome=%v", fault, err, lesson.MutationOutcomeOf(err))
			}
			if got, readErr := os.ReadFile(target); readErr != nil || string(got) != "owned\n" {
				t.Fatalf("%s lost publication: %q, %v", fault, got, readErr)
			}
		})
	}
	if !isExclusivePublishCollision(fs.ErrExist) || isExclusivePublishCollision(errors.New("other")) {
		t.Fatal("exclusive collision classification mismatch")
	}
}

func TestLessonNewExclusiveCollisionPreservesForeignTargetAndAbortsPreparedEvent(t *testing.T) {
	root := setupSpecRoot(t)
	requireCLISuccess(t, projectdef.WriteSpecConfig(root, lessonTestConfig()))
	configureNoopLessonEvents(t, root)
	foreign := []byte("foreign concurrent Lesson\n")
	deps := defaultLessonCLIDeps()
	deps.publishExclusive = func(path string, _ []byte, _ os.FileMode) error {
		requireCLISuccess(t, os.WriteFile(path, foreign, 0o600))
		return &lesson.MutationError{Outcome: lesson.MutationPrePublication, Err: fs.ErrExist}
	}
	cmd := lessonNewCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"project": root})
	requireCLIError(t, runLessonNewWithDeps(cmd, []string{"foreign-target"}, deps))
	target := filepath.Join(root, "spec", "lessons", "foreign-target", "README.md")
	if got, err := os.ReadFile(target); err != nil || !bytes.Equal(got, foreign) {
		t.Fatalf("foreign Lesson was overwritten: %q, %v", got, err)
	}
	prepared, err := event.NewOutbox(root).Prepared()
	if err != nil || len(prepared) != 0 {
		t.Fatalf("prepublication collision retained prepared event: %#v, %v", prepared, err)
	}
	cmd = lessonNewCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"project": root})
	if err := runLessonNewWithDeps(cmd, []string{"foreign-target"}, defaultLessonCLIDeps()); err == nil {
		t.Fatal("clean retry did not report the retained foreign target conflict")
	}
}

func TestWriteMissingIndexUsesExclusivePublicationOnRacingTarget(t *testing.T) {
	root := t.TempDir()
	if err := writeMissingIndex(root, "spec/README.md", "owned\n"); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(root, "spec", "README.md"))
	if err := writeMissingIndex(root, "spec/README.md", "replacement\n"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(root, "spec", "README.md"))
	if !bytes.Equal(before, after) {
		t.Fatalf("existing index was overwritten: %q", after)
	}
}

func TestWriteMissingIndexTreatsExclusivePublishCollisionAsConcurrentSuccess(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "spec", "README.md")
	foreign := []byte("foreign concurrent index\n")
	err := writeMissingIndexWithPublisher(root, "spec/README.md", "owned\n", func(path string, _ []byte, _ os.FileMode) error {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, foreign, 0o600); err != nil {
			return err
		}
		return &lesson.MutationError{Outcome: lesson.MutationPrePublication, Err: fs.ErrExist}
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, readErr := os.ReadFile(target); readErr != nil || !bytes.Equal(got, foreign) {
		t.Fatalf("foreign index changed: %q, %v", got, readErr)
	}
}
