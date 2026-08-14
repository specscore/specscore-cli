package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// specTreeSnapshot is the complete descriptor-rooted representation used by
// lifecycle copy-on-write staging and recovery verification.
type specTreeSnapshot struct {
	files       map[string]specTreeFile
	directories map[string]specTreeDirectory
}

type specTreeFile struct {
	content  []byte
	mode     os.FileMode
	metadata specTreeEntryMetadata
}

type specTreeDirectory struct {
	mode     os.FileMode
	metadata specTreeEntryMetadata
}

// specTreeEntryMetadata contains metadata that must survive a copy-on-write
// publication. Unsupported metadata fails closed instead of being discarded.
type specTreeEntryMetadata struct {
	extendedAttributes map[string][]byte
	accessTime         time.Time
	modificationTime   time.Time
}

// stagedSpecTree keeps the descriptor used for every private-tree operation
// alive until publication has classified the exchange. The path is only a
// publication label.
type stagedSpecTree struct {
	path string
	root *os.File
}

var (
	transactionReadFile                       = os.ReadFile
	transactionOpenFile                       = os.OpenFile
	transactionCloseFile                      = func(file *os.File) error { return file.Close() }
	transactionLstat                          = os.Lstat
	transactionMkdirTemp                      = os.MkdirTemp
	transactionRemoveAll                      = os.RemoveAll
	transactionReadSnapshotFile               = io.ReadAll
	transactionCloseStagedTree                = closeStagedSpecTree
	transactionLockFile                       = acquireLifecycleFileLock
	transactionProcessAlive                   = defaultProcessAlive
	transactionPlatformSupportsSecureMutation = platformSupportsSecureLifecycleTransaction
	transactionAfterRecoveryClaim             = func() {}
)

var errLifecycleLockHeld = errors.New("lifecycle lock is held")

func snapshotSpecTreeForTransaction(specRoot string) (specTreeSnapshot, error) {
	return snapshotSpecTreeNoFollow(specRoot)
}

func lifecycleLockOwnerPath(lockPath string) string {
	return filepath.Join(lockPath, "owner")
}

func acquireLifecycleLock(projectRoot string) (string, *os.File, error) {
	lockPath := filepath.Join(projectRoot, ".specscore-lifecycle.lock")
	lockFile, err := transactionOpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err == nil {
		if err := transactionLockFile(lockFile); err != nil {
			_ = transactionCloseFile(lockFile)
			if errors.Is(err, errLifecycleLockHeld) {
				return "", nil, exitcode.UnexpectedErrorf("another SpecScore lifecycle transaction is active at %s", lockPath)
			}
			return "", nil, exitcode.UnexpectedErrorf("acquiring lifecycle transaction lock %s: %v", lockPath, err)
		}
		return lockPath, lockFile, nil
	}
	if _, inspectErr := legacyLifecycleLockInfo(lockPath); inspectErr != nil {
		if os.IsNotExist(inspectErr) {
			return "", nil, exitcode.UnexpectedErrorf("acquiring lifecycle transaction lock %s: %v", lockPath, err)
		}
		return "", nil, inspectErr
	}
	stale, staleErr := lifecycleLegacyLockIsStale(lockPath)
	if staleErr != nil {
		return "", nil, staleErr
	}
	if !stale {
		return "", nil, exitcode.UnexpectedErrorf("another SpecScore lifecycle transaction is active at %s", lockPath)
	}
	return "", nil, exitcode.UnexpectedErrorf("legacy lifecycle transaction lock at %s requires manual recovery; refusing non-atomic directory deletion", lockPath)
}

func legacyLifecycleLockInfo(lockPath string) (os.FileInfo, error) {
	info, err := transactionLstat(lockPath)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, exitcode.UnexpectedErrorf("refusing symlinked lifecycle transaction lock at %s", lockPath)
	}
	if !info.IsDir() {
		return nil, exitcode.UnexpectedErrorf("lifecycle transaction lock at %s is not a directory", lockPath)
	}
	return info, nil
}

func lifecycleLegacyLockIsStale(lockPath string) (bool, error) {
	if _, err := legacyLifecycleLockInfo(lockPath); err != nil {
		return false, err
	}
	ownerPath := lifecycleLockOwnerPath(lockPath)
	ownerInfo, err := transactionLstat(ownerPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, exitcode.UnexpectedErrorf("inspecting lifecycle transaction lock owner at %s: %v", ownerPath, err)
	}
	if ownerInfo.Mode()&os.ModeSymlink != 0 || !ownerInfo.Mode().IsRegular() {
		return false, exitcode.UnexpectedErrorf("refusing non-regular lifecycle transaction lock owner at %s", ownerPath)
	}
	owner, err := transactionReadFile(ownerPath)
	if err != nil {
		return false, exitcode.UnexpectedErrorf("reading lifecycle transaction lock owner at %s: %v", lockPath, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(owner)))
	if err != nil || pid <= 0 {
		return true, nil
	}
	return !transactionProcessAlive(pid), nil
}

func releaseLifecycleLockFile(lockPath string, lockFile *os.File) error {
	return releaseLifecycleLockedFile(lockPath, lockFile)
}

func openStagedSpecTreeSnapshot(specRoot string, snapshot specTreeSnapshot) (*stagedSpecTree, error) {
	if err := validateSpecTreeSnapshot(snapshot); err != nil {
		return nil, err
	}
	stageRoot, err := transactionMkdirTemp(filepath.Dir(specRoot), ".specscore-lint-stage-")
	if err != nil {
		return nil, err
	}
	stage, err := openStagedSpecTreeNoFollow(stageRoot)
	if err != nil {
		_ = transactionRemoveAll(stageRoot)
		return nil, fmt.Errorf("opening isolated lint tree without following links: %w", err)
	}
	if err := materializeStagedSpecTreeNoFollow(stage, snapshot); err != nil {
		_ = closeStagedSpecTree(stage)
		_ = transactionRemoveAll(stageRoot)
		return nil, fmt.Errorf("materializing isolated lint tree: %w", err)
	}
	return stage, nil
}

func stageSpecTreeSnapshot(specRoot string, snapshot specTreeSnapshot) (string, error) {
	stage, err := openStagedSpecTreeSnapshot(specRoot, snapshot)
	if err != nil {
		return "", err
	}
	if err := transactionCloseStagedTree(stage); err != nil {
		_ = transactionRemoveAll(stage.path)
		return "", err
	}
	return stage.path, nil
}

func materializeSpecSnapshot(root string, snapshot specTreeSnapshot) error {
	stage, err := openStagedSpecTreeNoFollow(root)
	if err != nil {
		return fmt.Errorf("opening isolated materialisation root without following links: %w", err)
	}
	defer func() { _ = closeStagedSpecTree(stage) }()
	return materializeStagedSpecTreeNoFollow(stage, snapshot)
}

func validateSpecTreeSnapshot(snapshot specTreeSnapshot) error {
	if _, ok := snapshot.directories["."]; !ok {
		return errors.New("snapshot is missing its root directory")
	}
	for rel := range snapshot.directories {
		if rel == "." {
			continue
		}
		if err := validateSnapshotRelativePath(rel); err != nil {
			return err
		}
	}
	for rel := range snapshot.files {
		if err := validateSnapshotRelativePath(rel); err != nil {
			return err
		}
	}
	return nil
}

func validateSnapshotRelativePath(rel string) error {
	clean := filepath.Clean(rel)
	if clean == "." || clean != rel || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("invalid snapshot path %q", rel)
	}
	return nil
}

func unionSnapshotPaths(left, right []string) []string {
	paths := make(map[string]bool, len(left)+len(right))
	for _, path := range left {
		paths[path] = true
	}
	for _, path := range right {
		paths[path] = true
	}
	result := make([]string, 0, len(paths))
	for path := range paths {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

func changedSnapshotFiles(before, after specTreeSnapshot) []string {
	paths := make(map[string]bool, len(before.files)+len(after.files))
	for rel := range before.files {
		paths[rel] = true
	}
	for rel := range after.files {
		paths[rel] = true
	}
	var changed []string
	for rel := range paths {
		if !snapshotFileEqual(before, after, rel) {
			changed = append(changed, rel)
		}
	}
	sort.Strings(changed)
	return changed
}

func changedSnapshotDirectories(before, after specTreeSnapshot) []string {
	paths := make(map[string]bool, len(before.directories)+len(after.directories))
	for rel := range before.directories {
		paths[rel] = true
	}
	for rel := range after.directories {
		paths[rel] = true
	}
	var changed []string
	for rel := range paths {
		if !snapshotDirectoryEqual(before, after, rel) {
			changed = append(changed, rel)
		}
	}
	sort.Strings(changed)
	return changed
}

func snapshotFileEqual(left, right specTreeSnapshot, rel string) bool {
	leftFile, leftExists := left.files[rel]
	rightFile, rightExists := right.files[rel]
	return leftExists == rightExists && (!leftExists || (leftFile.mode == rightFile.mode && bytes.Equal(leftFile.content, rightFile.content) && snapshotEntryMetadataEqual(leftFile.metadata, rightFile.metadata)))
}

func snapshotEntryMetadataEqual(left, right specTreeEntryMetadata) bool {
	return left.modificationTime.Equal(right.modificationTime) &&
		reflect.DeepEqual(left.extendedAttributes, right.extendedAttributes)
}

func snapshotDirectoryEqual(left, right specTreeSnapshot, rel string) bool {
	leftDirectory, leftExists := left.directories[rel]
	rightDirectory, rightExists := right.directories[rel]
	return leftExists == rightExists && (!leftExists || (leftDirectory.mode == rightDirectory.mode && reflect.DeepEqual(leftDirectory.metadata.extendedAttributes, rightDirectory.metadata.extendedAttributes)))
}

func specTreeSnapshotsEqual(left, right specTreeSnapshot) bool {
	if len(left.files) != len(right.files) || len(left.directories) != len(right.directories) {
		return false
	}
	for rel := range left.files {
		if !snapshotFileEqual(left, right, rel) {
			return false
		}
	}
	for rel := range left.directories {
		if !snapshotDirectoryEqual(left, right, rel) {
			return false
		}
	}
	return true
}
