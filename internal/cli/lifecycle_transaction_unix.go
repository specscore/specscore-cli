//go:build darwin || linux

package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

var (
	lifecycleStageOpenChild       = openLifecycleProjectChildNoFollow
	lifecycleStageMaterialize     = materializeStagedSpecTreeNoFollow
	lifecycleStageFstat           = unix.Fstat
	lifecycleChildStat            = func(file *os.File) (os.FileInfo, error) { return file.Stat() }
	lifecycleContextCopyFile      = copyOptionalLifecycleRegularFile
	lifecycleContextCopyDirectory = copyOptionalLifecycleDirectory
	lifecycleContextOpenChild     = openLifecycleProjectChildNoFollow
	lifecycleContextMkdirAt       = unix.Mkdirat
	lifecycleContextFileStat      = func(file *os.File) (os.FileInfo, error) { return file.Stat() }
	lifecycleContextReadAll       = io.ReadAll
	lifecycleContextWriteAll      = writeAllAtFD
	lifecycleContextFchmod        = unix.Fchmod
	lifecycleContextClose         = unix.Close
	lifecycleReceiptOpenAt        = unix.Openat
	lifecycleReceiptFstat         = unix.Fstat
	lifecycleReceiptWriteAll      = writeAllAtFD
	lifecycleReceiptFsync         = unix.Fsync
	lifecycleReceiptClose         = unix.Close
	lifecycleReceiptRenameAt      = unix.Renameat
	lifecycleReceiptTempName      = newLifecycleReceiptTempName
	lifecycleReceiptLinkAt        = unix.Linkat
	lifecycleJournalOpenAt        = unix.Openat
	lifecycleJournalMkdirAt       = unix.Mkdirat
	lifecyclePublicationExchange  = lifecycleExchangeSpecAt
	lifecyclePublicationFsync     = unix.Fsync
)

func openLifecycleProjectNoFollow(path string) (*stagedSpecTree, error) {
	return openStagedSpecTreeNoFollow(path)
}

func createLifecycleStageProjectNoFollow(project *stagedSpecTree, id string) (*stagedSpecTree, error) {
	if project == nil || project.root == nil {
		return nil, fmt.Errorf("lifecycle project descriptor is closed")
	}
	name := ".specscore-txn-" + id
	if err := unix.Mkdirat(int(project.root.Fd()), name, 0o700); err != nil {
		return nil, err
	}
	return openLifecycleProjectChildNoFollow(project, name)
}

func lifecycleStageIdentity(stage *stagedSpecTree) (string, error) {
	if stage == nil || stage.root == nil {
		return "", fmt.Errorf("lifecycle stage descriptor is closed")
	}
	var stat unix.Stat_t
	if err := lifecycleStageFstat(int(stage.root.Fd()), &stat); err != nil {
		return "", err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return "", fmt.Errorf("lifecycle stage descriptor is not a directory")
	}
	return fmt.Sprintf("%x:%x", uint64(stat.Dev), uint64(stat.Ino)), nil
}

func openLifecycleProjectChildNoFollow(project *stagedSpecTree, name string) (*stagedSpecTree, error) {
	if project == nil || project.root == nil {
		return nil, fmt.Errorf("lifecycle project descriptor is closed")
	}
	fd, err := unix.Openat(int(project.root.Fd()), name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	return &stagedSpecTree{path: filepath.Join(project.path, name), root: os.NewFile(uintptr(fd), name)}, nil
}

func createLifecycleStageSpecNoFollow(project *stagedSpecTree, snapshot specTreeSnapshot) (*stagedSpecTree, error) {
	return createLifecycleStageDirectoryNoFollow(project, "spec", snapshot)
}

func createLifecycleStageDirectoryNoFollow(project *stagedSpecTree, name string, snapshot specTreeSnapshot) (*stagedSpecTree, error) {
	if project == nil || project.root == nil {
		return nil, fmt.Errorf("lifecycle project descriptor is closed")
	}
	if err := unix.Mkdirat(int(project.root.Fd()), name, 0o700); err != nil {
		return nil, err
	}
	stage, err := lifecycleStageOpenChild(project, name)
	if err != nil {
		return nil, err
	}
	if err := lifecycleStageMaterialize(stage, snapshot); err != nil {
		_ = closeStagedSpecTree(stage)
		return nil, err
	}
	return stage, nil
}

func lifecycleProjectChildMatches(project *stagedSpecTree, name string, expected *stagedSpecTree) error {
	if expected == nil || expected.root == nil {
		return fmt.Errorf("expected lifecycle child descriptor is closed")
	}
	candidate, err := openLifecycleProjectChildNoFollow(project, name)
	if err != nil {
		return err
	}
	defer func() { _ = closeStagedSpecTree(candidate) }()
	actualInfo, err := lifecycleChildStat(candidate.root)
	if err != nil {
		return err
	}
	expectedInfo, err := lifecycleChildStat(expected.root)
	if err != nil {
		return err
	}
	if !os.SameFile(actualInfo, expectedInfo) {
		return fmt.Errorf("descriptor identity changed")
	}
	return nil
}

func exchangeLifecycleProjectSpecs(realProject, stagedProject *stagedSpecTree) error {
	if err := lifecyclePublicationExchange(int(stagedProject.root.Fd()), int(realProject.root.Fd())); err != nil {
		return err
	}
	// The exchange changes the spec entry in two distinct parent directories:
	// the live project and its staged recovery project. Both must be durable
	// before the committed receipt can be persisted.
	if err := lifecyclePublicationFsync(int(realProject.root.Fd())); err != nil {
		return fmt.Errorf("syncing live lifecycle publication parent: %w", err)
	}
	if err := lifecyclePublicationFsync(int(stagedProject.root.Fd())); err != nil {
		return fmt.Errorf("syncing recovery lifecycle publication parent: %w", err)
	}
	return nil
}

func runLifecycleInStagedProject(stage *stagedSpecTree, op func(string) error) error {
	return runLintInStagedSpecTree(stage, op)
}

// materializeLifecycleProjectContext freezes every non-spec project input read
// by lifecycle lint through descriptors held from the real project. No
// pathname rooted in the live project is reopened after the transaction begins.
func materializeLifecycleProjectContext(project, stage *stagedSpecTree) error {
	if err := lifecycleContextCopyFile(project, "specscore.yaml", stage, "specscore.yaml"); err != nil {
		return fmt.Errorf("copying specscore.yaml: %w", err)
	}
	if err := lifecycleContextCopyDirectory(project, ".github", stage, ".github"); err != nil {
		return fmt.Errorf("copying .github: %w", err)
	}
	gitSource, err := lifecycleContextOpenChild(project, ".git")
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil
		}
		return fmt.Errorf("opening .git: %w", err)
	}
	defer func() { _ = closeStagedSpecTree(gitSource) }()
	if err := lifecycleContextMkdirAt(int(stage.root.Fd()), ".git", 0o700); err != nil {
		return fmt.Errorf("creating staged .git: %w", err)
	}
	gitStage, err := lifecycleContextOpenChild(stage, ".git")
	if err != nil {
		return err
	}
	defer func() { _ = closeStagedSpecTree(gitStage) }()
	if err := lifecycleContextCopyFile(gitSource, "config", gitStage, "config"); err != nil {
		return fmt.Errorf("copying .git/config: %w", err)
	}
	return nil
}

func copyOptionalLifecycleDirectory(sourceProject *stagedSpecTree, sourceName string, stageProject *stagedSpecTree, stageName string) error {
	if sourceProject == nil || sourceProject.root == nil || stageProject == nil || stageProject.root == nil {
		return fmt.Errorf("lifecycle project descriptor is closed")
	}
	source, err := openLifecycleProjectChildNoFollow(sourceProject, sourceName)
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil
		}
		return err
	}
	defer func() { _ = closeStagedSpecTree(source) }()
	snapshot, err := snapshotStagedSpecTreeNoFollow(source)
	if err != nil {
		return err
	}
	stage, err := createLifecycleStageDirectoryNoFollow(stageProject, stageName, snapshot)
	if err != nil {
		return err
	}
	return closeStagedSpecTree(stage)
}

func copyOptionalLifecycleRegularFile(sourceDirectory *stagedSpecTree, sourceName string, destinationDirectory *stagedSpecTree, destinationName string) error {
	if sourceDirectory == nil || sourceDirectory.root == nil || destinationDirectory == nil || destinationDirectory.root == nil {
		return fmt.Errorf("lifecycle project descriptor is closed")
	}
	fd, err := unix.Openat(int(sourceDirectory.root.Fd()), sourceName, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil
		}
		return err
	}
	source := os.NewFile(uintptr(fd), sourceName)
	defer func() { _ = source.Close() }()
	before, err := lifecycleContextFileStat(source)
	if err != nil {
		return err
	}
	if !before.Mode().IsRegular() {
		return fmt.Errorf("refusing non-regular project context %s", sourceName)
	}
	content, err := lifecycleContextReadAll(source)
	if err != nil {
		return err
	}
	after, err := lifecycleContextFileStat(source)
	if err != nil {
		return err
	}
	if before.Mode() != after.Mode() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return fmt.Errorf("project context changed while reading %s", sourceName)
	}
	destinationFD, err := unix.Openat(int(destinationDirectory.root.Fd()), destinationName, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, uint32(before.Mode().Perm()))
	if err != nil {
		return err
	}
	if err := lifecycleContextWriteAll(destinationFD, content); err != nil {
		_ = lifecycleContextClose(destinationFD)
		return err
	}
	if err := lifecycleContextFchmod(destinationFD, uint32(before.Mode().Perm())); err != nil {
		_ = lifecycleContextClose(destinationFD)
		return err
	}
	return lifecycleContextClose(destinationFD)
}

func writeLifecycleReceiptNoFollow(project *stagedSpecTree, name string, data []byte) error {
	if project == nil || project.root == nil {
		return fmt.Errorf("lifecycle project descriptor is closed")
	}
	id := strings.TrimSuffix(name, ".json")
	if !strings.HasSuffix(name, ".json") || id+".json" != name || filepath.Base(name) != name || !validLifecycleTransactionID(id) {
		return fmt.Errorf("invalid lifecycle recovery receipt name %q", name)
	}
	journalFD, err := openOrCreateLifecycleJournalDirectory(int(project.root.Fd()))
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(journalFD) }()
	temporaryName, err := lifecycleReceiptTempName(name)
	if err != nil {
		return err
	}
	if filepath.Base(temporaryName) != temporaryName || temporaryName == name {
		return fmt.Errorf("invalid lifecycle recovery receipt temporary name %q", temporaryName)
	}
	fd, err := lifecycleReceiptOpenAt(journalFD, temporaryName, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	var stat unix.Stat_t
	if err := lifecycleReceiptFstat(fd, &stat); err != nil {
		_ = lifecycleReceiptClose(fd)
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG {
		_ = lifecycleReceiptClose(fd)
		return fmt.Errorf("recovery receipt is not a regular file")
	}
	if err := lifecycleReceiptWriteAll(fd, data); err != nil {
		_ = lifecycleReceiptClose(fd)
		return err
	}
	if err := lifecycleReceiptFsync(fd); err != nil {
		_ = lifecycleReceiptClose(fd)
		return err
	}
	if err := lifecycleReceiptClose(fd); err != nil {
		return err
	}
	if err := lifecycleReceiptRenameAt(journalFD, temporaryName, journalFD, name); err != nil {
		return err
	}
	if err := lifecycleReceiptFsync(journalFD); err != nil {
		return err
	}
	// The journal directory itself is a child of the project. Syncing its
	// parent makes a newly created journal durable as well as its receipt.
	return lifecycleReceiptFsync(int(project.root.Fd()))
}

// retainLifecyclePublishingIntent creates an immutable hard-link to the
// already-durable publishing receipt before the canonical receipt is replaced
// by its terminal form. A final fsync error is inherently ambiguous, but this
// retained intent lets recovery validate the physical receipt and exchange
// layout without losing the prior publication account.
func retainLifecyclePublishingIntent(project *stagedSpecTree, receipt LifecycleTransactionReceipt) error {
	if project == nil || project.root == nil {
		return fmt.Errorf("lifecycle project descriptor is closed")
	}
	if receipt.State != "publishing" || !validLifecycleTransactionID(receipt.ID) {
		return fmt.Errorf("invalid publishing lifecycle receipt")
	}
	journalFD, err := openOrCreateLifecycleJournalDirectory(int(project.root.Fd()))
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(journalFD) }()
	if err := lifecycleReceiptLinkAt(
		journalFD,
		receipt.ID+".json",
		journalFD,
		receipt.ID+".publishing.json",
		0,
	); err != nil {
		return err
	}
	if err := lifecycleReceiptFsync(journalFD); err != nil {
		return err
	}
	return lifecycleReceiptFsync(int(project.root.Fd()))
}

func newLifecycleReceiptTempName(name string) (string, error) {
	id, err := newLifecycleTransactionID()
	if err != nil {
		return "", err
	}
	return "." + name + "." + id + ".tmp", nil
}

func openOrCreateLifecycleJournalDirectory(projectFD int) (int, error) {
	fd, err := lifecycleJournalOpenAt(projectFD, ".specscore-recovery", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err == nil {
		return fd, nil
	}
	if !errors.Is(err, unix.ENOENT) {
		return -1, err
	}
	if err := lifecycleJournalMkdirAt(projectFD, ".specscore-recovery", 0o700); err != nil && !errors.Is(err, unix.EEXIST) {
		return -1, err
	}
	return lifecycleJournalOpenAt(projectFD, ".specscore-recovery", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
}
