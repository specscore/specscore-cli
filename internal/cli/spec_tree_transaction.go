package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

// specTreeSnapshot captures every regular file and directory below a spec root
// before a lifecycle post-mutation lint pass. The transition itself owns
// restoration of its artifact; this snapshot owns every additional path the
// lint fixers touch (indexes, footers, mirrors, and any newly-created artifact).
type specTreeSnapshot struct {
	files       map[string]specTreeFile
	directories map[string]os.FileMode
}

type specTreeFile struct {
	content []byte
	mode    os.FileMode
}

// specTreeTransaction marks the full lifecycle transaction boundary. It is
// created before a lifecycle verb mutates its source artifact; postMutationHook
// records the before/after state of the subsequent lint pass. The caller invokes
// finish after the lifecycle package has rolled its source back. finish restores
// only lint-owned paths that still match the captured post-lint state, so a raw
// filesystem writer is never overwritten by this transaction.
type specTreeTransaction struct {
	specRoot             string
	snapshot             specTreeSnapshot
	lockPath             string
	lockFile             *os.File
	postMutationStarted  bool
	preLintSnapshot      *specTreeSnapshot
	postLintSnapshot     *specTreeSnapshot
	provenLifecycleState *specTreeSnapshot
	ownedLifecyclePaths  map[string]bool
	preLintUnownedPaths  []string
	opaquePostMutation   bool
}

// lifecycleReleaseError retains the lifecycle command error as the unwrap
// target when releasing its exclusive lock also fails.  A release failure must
// not hide the action that the user needs to recover, nor may it be silently
// ignored because it leaves a lock that would block later invocations.
type lifecycleReleaseError struct {
	actionErr  error
	releaseErr error
}

type concurrentSpecChangesError struct {
	paths []string
}

func (err *concurrentSpecChangesError) Error() string {
	return fmt.Sprintf("concurrent changes at %s", strings.Join(err.paths, ", "))
}

func (err *lifecycleReleaseError) Error() string {
	return fmt.Sprintf("%v\nadditionally failed to release lifecycle transaction lock: %v", err.actionErr, err.releaseErr)
}

func (err *lifecycleReleaseError) Unwrap() error {
	return err.actionErr
}

// Testable indirections for rollback I/O failures.
var (
	transactionReadFile     = os.ReadFile
	transactionWriteFile    = os.WriteFile
	transactionRemove       = os.Remove
	transactionMkdirAll     = os.MkdirAll
	transactionOpenFile     = os.OpenFile
	transactionCloseFile    = func(file *os.File) error { return file.Close() }
	transactionLstat        = os.Lstat
	transactionRel          = filepath.Rel
	transactionMkdirTemp    = os.MkdirTemp
	transactionChmod        = os.Chmod
	transactionRemoveAll    = os.RemoveAll
	transactionScratchMkdir = os.MkdirAll
	transactionScratchWrite = os.WriteFile
	transactionSnapshot     = snapshotSpecTreeForTransaction
	transactionLockFile     = acquireLifecycleFileLock
	transactionProcessAlive = defaultProcessAlive
)

var errLifecycleLockHeld = errors.New("lifecycle lock is held")

func snapshotSpecTreeForTransaction(specRoot string) (specTreeSnapshot, error) {
	snapshot := specTreeSnapshot{
		files:       make(map[string]specTreeFile),
		directories: make(map[string]os.FileMode),
	}
	err := filepath.Walk(specRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, _ := filepath.Rel(specRoot, path) // filepath.Walk only yields descendants.
		if info.IsDir() {
			snapshot.directories[rel] = info.Mode()
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		content, err := transactionReadFile(path)
		if err != nil {
			return err
		}
		snapshot.files[rel] = specTreeFile{content: content, mode: info.Mode()}
		return nil
	})
	if err != nil {
		return specTreeSnapshot{}, fmt.Errorf("snapshotting spec tree: %w", err)
	}
	return snapshot, nil
}

func beginSpecTreeTransaction(specRoot string, lifecycleOwnedPaths ...string) (*specTreeTransaction, error) {
	ownedPaths, err := normalizeLifecycleOwnedPaths(lifecycleOwnedPaths)
	if err != nil {
		return nil, err
	}
	// The lock is outside spec/ so it is never part of the snapshot that might
	// be restored. Every mutating SpecScore CLI path acquires this same lock,
	// making the read → lifecycle → lint → rollback window exclusive.
	lockPath, lockFile, err := acquireLifecycleLock(filepath.Dir(specRoot))
	if err != nil {
		return nil, err
	}
	snapshot, err := transactionSnapshot(specRoot)
	if err != nil {
		_ = releaseLifecycleLockFile(lockPath, lockFile)
		return nil, exitcode.UnexpectedErrorf("snapshotting spec before lifecycle mutation: %v", err)
	}
	return &specTreeTransaction{
		specRoot:            specRoot,
		snapshot:            snapshot,
		lockPath:            lockPath,
		lockFile:            lockFile,
		ownedLifecyclePaths: ownedPaths,
	}, nil
}

func normalizeLifecycleOwnedPaths(paths []string) (map[string]bool, error) {
	owned := make(map[string]bool, len(paths))
	for _, path := range paths {
		path = filepath.Clean(path)
		if path == "." || filepath.IsAbs(path) || path == ".." || strings.HasPrefix(path, ".."+string(os.PathSeparator)) {
			return nil, exitcode.UnexpectedErrorf("invalid lifecycle-owned spec path %q", path)
		}
		owned[path] = true
	}
	return owned, nil
}

func relativeLifecycleOwnedPath(specRoot, path string) (string, error) {
	rel, err := transactionRel(specRoot, path)
	if err != nil {
		return "", exitcode.UnexpectedErrorf("resolving lifecycle-owned spec path %s: %v", path, err)
	}
	if _, err := normalizeLifecycleOwnedPaths([]string{rel}); err != nil {
		return "", err
	}
	return filepath.Clean(rel), nil
}

func (transaction *specTreeTransaction) release() error {
	if transaction.lockFile == nil {
		return nil
	}
	if err := releaseLifecycleLockFile(transaction.lockPath, transaction.lockFile); err != nil {
		return err
	}
	transaction.lockPath = ""
	transaction.lockFile = nil
	return nil
}

// releaseSpecTreeTransaction is used in a named-return defer by every
// lifecycle command.  It turns a successful action plus a failed release into
// an error, and retains a failed action as the primary (unwrap) error if both
// failures occur.
func releaseSpecTreeTransaction(transaction *specTreeTransaction, result *error) {
	if releaseErr := transaction.release(); releaseErr != nil {
		if *result == nil {
			*result = releaseErr
			return
		}
		*result = &lifecycleReleaseError{actionErr: *result, releaseErr: releaseErr}
	}
}

func lifecycleLockOwnerPath(lockPath string) string {
	return filepath.Join(lockPath, "owner")
}

func acquireLifecycleLock(projectRoot string) (string, *os.File, error) {
	lockPath := filepath.Join(projectRoot, ".specscore-lifecycle.lock")
	for attempt := 0; attempt < 2; attempt++ {
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
		_, inspectErr := legacyLifecycleLockInfo(lockPath)
		if inspectErr != nil {
			// A non-directory open failure (permissions, I/O, a failing test
			// seam) is not evidence of a legacy lock. Only a verified directory
			// enters recovery; otherwise preserve the original open error.
			if os.IsNotExist(inspectErr) {
				return "", nil, exitcode.UnexpectedErrorf("acquiring lifecycle transaction lock %s: %v", lockPath, err)
			}
			return "", nil, inspectErr
		}
		stale, err := lifecycleLegacyLockIsStale(lockPath)
		if err != nil {
			return "", nil, err
		}
		if !stale {
			return "", nil, exitcode.UnexpectedErrorf("another SpecScore lifecycle transaction is active at %s", lockPath)
		}
		if err := releaseLegacyLifecycleLock(lockPath); err != nil {
			return "", nil, exitcode.UnexpectedErrorf("removing stale lifecycle transaction lock %s: %v", lockPath, err)
		}
	}
	return "", nil, exitcode.UnexpectedErrorf("acquiring lifecycle transaction lock %s: legacy lock was recreated concurrently", lockPath)
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

// lifecycleLegacyLockIsStale handles only the directory lock format written
// by pre-file-lock releases. New locks are advisory OS file locks; an unlocked
// stale file never blocks a later command, even if it survives a crash.
func lifecycleLegacyLockIsStale(lockPath string) (bool, error) {
	if _, err := legacyLifecycleLockInfo(lockPath); err != nil {
		return false, err
	}
	ownerPath := lifecycleLockOwnerPath(lockPath)
	ownerInfo, err := transactionLstat(ownerPath)
	if err != nil {
		if os.IsNotExist(err) {
			// A freshly-created legacy directory may not have written its owner
			// yet. Treat it as active; deleting it can erase a live transaction.
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

func releaseLegacyLifecycleLock(lockPath string) error {
	lockInfo, err := legacyLifecycleLockInfo(lockPath)
	if err != nil {
		return err
	}
	stale, err := lifecycleLegacyLockIsStale(lockPath)
	if err != nil {
		return err
	}
	if !stale {
		return exitcode.UnexpectedErrorf("refusing to remove active or ownerless lifecycle transaction lock at %s", lockPath)
	}
	ownerPath := lifecycleLockOwnerPath(lockPath)
	ownerInfo, err := transactionLstat(ownerPath)
	if err != nil {
		return exitcode.UnexpectedErrorf("revalidating lifecycle transaction lock owner %s: %v", ownerPath, err)
	}
	if ownerInfo.Mode()&os.ModeSymlink != 0 || !ownerInfo.Mode().IsRegular() {
		return exitcode.UnexpectedErrorf("refusing changed lifecycle transaction lock owner at %s", ownerPath)
	}
	if current, err := legacyLifecycleLockInfo(lockPath); err != nil || !os.SameFile(lockInfo, current) {
		if err != nil {
			return err
		}
		return exitcode.UnexpectedErrorf("lifecycle transaction lock was replaced before stale-lock recovery at %s", lockPath)
	}
	currentOwner, err := transactionLstat(ownerPath)
	if err != nil {
		return exitcode.UnexpectedErrorf("revalidating lifecycle transaction lock owner %s immediately before removal: %v", ownerPath, err)
	}
	if currentOwner.Mode()&os.ModeSymlink != 0 || !currentOwner.Mode().IsRegular() || !os.SameFile(ownerInfo, currentOwner) {
		return exitcode.UnexpectedErrorf("lifecycle transaction lock owner was replaced before stale-lock recovery at %s", ownerPath)
	}
	if err := transactionRemove(ownerPath); err != nil && !os.IsNotExist(err) {
		return exitcode.UnexpectedErrorf("releasing lifecycle transaction lock owner %s: %v", ownerPath, err)
	}
	// os.Remove only removes an empty directory. Re-check the directory identity
	// after removing the stale owner, so a replacement (including a fresh lock)
	// is never removed by this recovery path.
	current, err := legacyLifecycleLockInfo(lockPath)
	if err != nil {
		return err
	}
	if !os.SameFile(lockInfo, current) {
		return exitcode.UnexpectedErrorf("lifecycle transaction lock was replaced during stale-lock recovery at %s", lockPath)
	}
	if err := transactionRemove(lockPath); err != nil && !os.IsNotExist(err) {
		return exitcode.UnexpectedErrorf("releasing lifecycle transaction lock %s: %v", lockPath, err)
	}
	return nil
}

// releaseLifecycleLockFile delegates platform-specific ordering. Unix can
// unlink an open locked file; Windows must unlock and close first because an
// open non-delete-sharing handle cannot be unlinked. A leftover unlocked file
// is benign because acquisition locks the opened inode rather than trusting
// pathname existence.
func releaseLifecycleLockFile(lockPath string, lockFile *os.File) error {
	return releaseLifecycleLockedFile(lockPath, lockFile)
}

// postMutationHook runs the normal lint hook after a lifecycle package has
// rewritten or relocated its artifact. It must not restore eagerly: a normal
// hook failure is rolled back by the lifecycle package, while a deferred
// failure is rolled back conflict-safely by finish.
func (transaction *specTreeTransaction) postMutationHook() func() error {
	return transaction.postMutationHookWithLint(runLintPostMutation)
}

// postMutationHookWithLint runs a known lint implementation against an exact
// private copy of the pre-lint tree. Only its resulting manifest is applied to
// the real tree, after a compare against that pre-lint snapshot. This is the
// ownership evidence an opaque callback cannot provide: raw writers during the
// lint pass cannot be mistaken for the linter and overwritten by rollback.
func (transaction *specTreeTransaction) postMutationHookWithLint(run func(string) error) func() error {
	return func() error {
		before, err := transactionSnapshot(transaction.specRoot)
		if err != nil {
			return exitcode.UnexpectedErrorf("snapshotting spec before lint --fix: %v", err)
		}
		transaction.postMutationStarted = true
		transaction.preLintSnapshot = &before
		if unownedPaths := transaction.unownedPreLintPaths(before); len(unownedPaths) > 0 {
			transaction.preLintUnownedPaths = unownedPaths
			transaction.postLintSnapshot = &before
			return lifecycle.DeferRollback(exitcode.UnexpectedErrorf(
				"uncoordinated spec changes before lint --fix at %s", strings.Join(unownedPaths, ", ")))
		}

		cloneRoot, err := transactionMkdirTemp("", "specscore-lint-transaction-*")
		if err != nil {
			transaction.postLintSnapshot = &before
			return lifecycle.DeferRollback(exitcode.UnexpectedErrorf("creating isolated lint tree: %v", err))
		}
		defer func() { _ = transactionRemoveAll(cloneRoot) }()
		if rootMode, ok := before.directories["."]; ok {
			if err := transactionChmod(cloneRoot, rootMode.Perm()); err != nil {
				transaction.postLintSnapshot = &before
				return lifecycle.DeferRollback(exitcode.UnexpectedErrorf("preparing isolated lint tree: %v", err))
			}
		}
		if err := materializeSpecSnapshot(cloneRoot, before); err != nil {
			transaction.postLintSnapshot = &before
			return lifecycle.DeferRollback(exitcode.UnexpectedErrorf("materializing isolated lint tree: %v", err))
		}
		runErr := run(cloneRoot)
		if runErr != nil {
			// No linter bytes were applied to the live tree, so only the
			// lifecycle mutation needs rollback. Capture live state so finish
			// still detects a concurrent writer while this hook was running.
			current, snapshotErr := transactionSnapshot(transaction.specRoot)
			if snapshotErr != nil {
				return lifecycle.DeferRollback(exitcode.UnexpectedErrorf("snapshotting spec after isolated lint failure: %v", snapshotErr))
			}
			transaction.postLintSnapshot = &current
			if changed := changedSnapshotPaths(before, current); len(changed) > 0 {
				transaction.preLintUnownedPaths = changed
			}
			return lifecycle.DeferRollback(runErr)
		}
		afterClone, err := transactionSnapshot(cloneRoot)
		if err != nil {
			transaction.postLintSnapshot = &before
			return lifecycle.DeferRollback(exitcode.UnexpectedErrorf("snapshotting isolated lint output: %v", err))
		}
		current, err := transactionSnapshot(transaction.specRoot)
		if err != nil {
			return lifecycle.DeferRollback(exitcode.UnexpectedErrorf("snapshotting spec before applying lint manifest: %v", err))
		}
		if changed := changedSnapshotPaths(before, current); len(changed) > 0 {
			transaction.postLintSnapshot = &current
			transaction.preLintUnownedPaths = changed
			return lifecycle.DeferRollback(exitcode.UnexpectedErrorf("uncoordinated spec changes during lint --fix at %s", strings.Join(changed, ", ")))
		}
		if err := applySpecSnapshotDiff(transaction.specRoot, before, afterClone); err != nil {
			current, _ := transactionSnapshot(transaction.specRoot)
			transaction.postLintSnapshot = &current
			return lifecycle.DeferRollback(exitcode.UnexpectedErrorf("applying lint mutation manifest: %v", err))
		}
		after, err := transactionSnapshot(transaction.specRoot)
		if err != nil {
			return lifecycle.DeferRollback(exitcode.UnexpectedErrorf("snapshotting spec after lint manifest: %v", err))
		}
		if !reflect.DeepEqual(after, afterClone) {
			transaction.postLintSnapshot = &after
			transaction.preLintUnownedPaths = changedSnapshotPaths(afterClone, after)
			return lifecycle.DeferRollback(exitcode.UnexpectedErrorf("uncoordinated spec changes while applying lint manifest at %s", strings.Join(transaction.preLintUnownedPaths, ", ")))
		}
		transaction.postLintSnapshot = &after
		return lifecycle.DeferRollback(nil)
	}
}

// materializeSpecSnapshot creates an isolated copy for lint execution. It
// deliberately uses direct filesystem I/O rather than transaction restore
// seams: fault-injection of the real rollback path must not prevent a private
// scratch tree from being populated before that rollback is exercised.
func materializeSpecSnapshot(root string, snapshot specTreeSnapshot) error {
	directories := make([]string, 0, len(snapshot.directories))
	for rel := range snapshot.directories {
		directories = append(directories, rel)
	}
	sort.Strings(directories)
	for _, rel := range directories {
		if err := transactionScratchMkdir(filepath.Join(root, rel), snapshot.directories[rel].Perm()); err != nil {
			return fmt.Errorf("creating directory %s: %w", rel, err)
		}
	}
	files := make([]string, 0, len(snapshot.files))
	for rel := range snapshot.files {
		files = append(files, rel)
	}
	sort.Strings(files)
	for _, rel := range files {
		file := snapshot.files[rel]
		path := filepath.Join(root, rel)
		if err := transactionScratchMkdir(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("creating file parent %s: %w", rel, err)
		}
		if err := transactionScratchWrite(path, file.content, file.mode.Perm()); err != nil {
			return fmt.Errorf("writing file %s: %w", rel, err)
		}
	}
	return nil
}

// captureLifecycleMutationState records the exact filesystem state produced by
// the lifecycle package after its final artifact write and before it calls the
// post-mutation hook. A later pre-lint snapshot must match this evidence for
// every declared lifecycle artifact; otherwise a raw writer won the race and
// rollback is conservatively withheld.
func (transaction *specTreeTransaction) captureLifecycleMutationState() error {
	snapshot, err := transactionSnapshot(transaction.specRoot)
	if err != nil {
		transaction.postMutationStarted = true
		return lifecycle.DeferRollback(exitcode.UnexpectedErrorf("capturing lifecycle-owned state before lint --fix: %v", err))
	}
	transaction.provenLifecycleState = &snapshot
	return nil
}

// postMutationHookWith records a custom lint hook as part of this transaction.
// Issue lifecycle transitions use a scoped verifier, while other artifacts use
// the whole-tree verifier above; both require the same rollback boundary.
func (transaction *specTreeTransaction) postMutationHookWith(run func() error) func() error {
	return func() error {
		before, err := transactionSnapshot(transaction.specRoot)
		if err != nil {
			return exitcode.UnexpectedErrorf("snapshotting spec before lint --fix: %v", err)
		}
		transaction.postMutationStarted = true
		transaction.opaquePostMutation = true
		transaction.preLintSnapshot = &before
		if unownedPaths := transaction.unownedPreLintPaths(before); len(unownedPaths) > 0 {
			transaction.preLintUnownedPaths = unownedPaths
			// Do not let lint touch an unproven external edit. Returning a
			// deferred error prevents the lifecycle package's blind source
			// rollback; finish leaves every path in place and gives the caller
			// an explicit recovery error.
			transaction.postLintSnapshot = &before
			return lifecycle.DeferRollback(exitcode.UnexpectedErrorf(
				"uncoordinated spec changes before lint --fix at %s", strings.Join(unownedPaths, ", ")))
		}
		runErr := run()
		after, snapshotErr := transactionSnapshot(transaction.specRoot)
		if snapshotErr != nil {
			return lifecycle.DeferRollback(exitcode.UnexpectedErrorf("snapshotting spec after lint --fix: %v", snapshotErr))
		}
		transaction.postLintSnapshot = &after
		return lifecycle.DeferRollback(runErr)
	}
}

// finish restores lint-owned paths after a post-mutation failure. Errors before
// the hook began are handled by the lifecycle package's own rollback and require
// no transaction restoration.
func (transaction *specTreeTransaction) finish(actionErr error) error {
	if actionErr == nil || !transaction.postMutationStarted {
		return actionErr
	}
	if transaction.preLintSnapshot == nil || transaction.postLintSnapshot == nil {
		return exitcode.UnexpectedErrorf("%v\nrollback preserved the current spec tree because lint mutation ownership could not be established; inspect and reconcile manually", actionErr)
	}
	if len(transaction.preLintUnownedPaths) > 0 {
		return exitcode.UnexpectedErrorf("%v\nrollback preserved unowned changes detected before lint --fix at %s; inspect and reconcile manually", actionErr, strings.Join(transaction.preLintUnownedPaths, ", "))
	}
	if transaction.opaquePostMutation && len(changedSnapshotPaths(*transaction.preLintSnapshot, *transaction.postLintSnapshot)) > 0 {
		return exitcode.UnexpectedErrorf("%v\nrollback preserved mutations from an opaque post-mutation hook; inspect and reconcile manually", actionErr)
	}
	if err := transaction.restoreLintMutations(); err != nil {
		var concurrent *concurrentSpecChangesError
		if errors.As(err, &concurrent) {
			return exitcode.UnexpectedErrorf("%v\nrollback preserved concurrent spec changes; inspect and reconcile manually: %v", actionErr, err)
		}
		return exitcode.UnexpectedErrorf("%v\nrollback could not restore all transaction-owned state; inspect and reconcile manually: %v", actionErr, err)
	}
	return actionErr
}

// unownedPreLintPaths identifies changes that happened between the initial
// transaction snapshot and the lifecycle package invoking its post-mutation
// hook. A declared artifact path is proven only when its exact bytes (or mode
// for a directory) match the state captured by MutationReady. Everything else
// is an external edit and must stop rollback before lint has a chance to
// modify it.
func (transaction *specTreeTransaction) unownedPreLintPaths(before specTreeSnapshot) []string {
	paths := make(map[string]bool)
	for _, path := range changedSnapshotFiles(transaction.snapshot, before) {
		if !transaction.lifecyclePathMatchesProof(before, path, false) {
			paths[path] = true
		}
	}
	for _, path := range changedSnapshotDirectories(transaction.snapshot, before) {
		if !transaction.lifecyclePathMatchesProof(before, path, true) {
			paths[path+"/"] = true
		}
	}
	if len(paths) == 0 {
		return nil
	}
	result := make([]string, 0, len(paths))
	for path := range paths {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

func (transaction *specTreeTransaction) lifecyclePathMatchesProof(before specTreeSnapshot, path string, directory bool) bool {
	if !transaction.ownedLifecyclePaths[path] || transaction.provenLifecycleState == nil {
		return false
	}
	if directory {
		return snapshotDirectoryEqual(before, *transaction.provenLifecycleState, path)
	}
	return snapshotFileEqual(before, *transaction.provenLifecycleState, path)
}

// restoreLintMutations rolls back only paths whose bytes still equal the state
// captured immediately after this transaction's lint pass. A path that differs
// from both that tool-owned state and the pre-transition state belongs to an
// uncoordinated writer, so it is left untouched and surfaced for recovery.
func (transaction *specTreeTransaction) restoreLintMutations() error {
	current, err := transactionSnapshot(transaction.specRoot)
	if err != nil {
		return fmt.Errorf("snapshotting current spec tree: %w", err)
	}

	// The initial snapshot includes the lifecycle source mutation as well as
	// lint output. It is intentionally the rollback baseline: packages receive
	// DeferredRollbackError and therefore do not blindly restore their source
	// before this compare-and-restore step can protect raw filesystem edits.
	filePaths := changedSnapshotFiles(transaction.snapshot, *transaction.postLintSnapshot)
	directoryPaths := changedSnapshotDirectories(transaction.snapshot, *transaction.postLintSnapshot)
	var conflicts []string
	for _, rel := range filePaths {
		if snapshotFileEqual(current, transaction.snapshot, rel) {
			continue
		}
		if !snapshotFileEqual(current, *transaction.postLintSnapshot, rel) {
			conflicts = append(conflicts, rel)
		}
	}
	for _, rel := range directoryPaths {
		if snapshotDirectoryEqual(current, transaction.snapshot, rel) {
			continue
		}
		if !snapshotDirectoryEqual(current, *transaction.postLintSnapshot, rel) || snapshotDirectoryHasExternalDescendant(current, *transaction.postLintSnapshot, rel) {
			conflicts = append(conflicts, rel+"/")
		}
	}
	if len(conflicts) > 0 {
		sort.Strings(conflicts)
		return &concurrentSpecChangesError{paths: conflicts}
	}

	for _, rel := range directoryPaths {
		if _, existed := transaction.snapshot.directories[rel]; !existed {
			continue
		}
		if _, existsNow := current.directories[rel]; existsNow {
			continue
		}
		directory := transaction.snapshot.directories[rel]
		path, err := transactionSafePath(transaction.specRoot, rel)
		if err != nil {
			return fmt.Errorf("refusing to recreate pre-transition directory %s: %w", rel, err)
		}
		if err := transactionMkdirAll(path, directory.Perm()); err != nil {
			return fmt.Errorf("recreating pre-transition directory %s: %w", rel, err)
		}
	}
	for _, rel := range filePaths {
		original, existed := transaction.snapshot.files[rel]
		if !existed {
			continue
		}
		path, err := transactionSafePath(transaction.specRoot, rel)
		if err != nil {
			return fmt.Errorf("refusing to restore pre-transition file %s: %w", rel, err)
		}
		if err := transactionMkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("recreating pre-transition directory for %s: %w", rel, err)
		}
		if err := transactionWriteFile(path, original.content, original.mode.Perm()); err != nil {
			return fmt.Errorf("restoring pre-transition file %s: %w", rel, err)
		}
	}
	for _, rel := range filePaths {
		if _, existed := transaction.snapshot.files[rel]; existed {
			continue
		}
		path, err := transactionSafePath(transaction.specRoot, rel)
		if err != nil {
			return fmt.Errorf("refusing to remove lint-created file %s: %w", rel, err)
		}
		if err := transactionRemove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing lint-created file %s: %w", rel, err)
		}
	}
	sort.Slice(directoryPaths, func(i, j int) bool { return len(directoryPaths[i]) > len(directoryPaths[j]) })
	for _, rel := range directoryPaths {
		if _, existed := transaction.snapshot.directories[rel]; existed || rel == "." {
			continue
		}
		path, err := transactionSafePath(transaction.specRoot, rel)
		if err != nil {
			return fmt.Errorf("refusing to remove lint-created directory %s: %w", rel, err)
		}
		if err := transactionRemove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing lint-created directory %s: %w", rel, err)
		}
	}
	return nil
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

func changedSnapshotPaths(before, after specTreeSnapshot) []string {
	paths := changedSnapshotFiles(before, after)
	for _, path := range changedSnapshotDirectories(before, after) {
		paths = append(paths, path+"/")
	}
	sort.Strings(paths)
	return paths
}

// applySpecSnapshotDiff applies a manifest materialized in an isolated tree.
// Each target is checked for a symlink before it is mutated; callers first
// compare the entire live tree with before, which makes this a fail-closed
// compare-and-apply step rather than an opaque in-place lint invocation.
func applySpecSnapshotDiff(specRoot string, before, after specTreeSnapshot) error {
	directories := changedSnapshotDirectories(before, after)
	for _, rel := range directories {
		if _, existed := after.directories[rel]; !existed {
			continue
		}
		path, err := transactionSafePath(specRoot, rel)
		if err != nil {
			return err
		}
		if err := transactionMkdirAll(path, after.directories[rel].Perm()); err != nil {
			return err
		}
	}
	for _, rel := range changedSnapshotFiles(before, after) {
		file, exists := after.files[rel]
		if !exists {
			continue
		}
		path, err := transactionSafePath(specRoot, rel)
		if err != nil {
			return err
		}
		if err := transactionMkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := transactionWriteFile(path, file.content, file.mode.Perm()); err != nil {
			return err
		}
	}
	for _, rel := range changedSnapshotFiles(before, after) {
		if _, exists := after.files[rel]; exists {
			continue
		}
		path, err := transactionSafePath(specRoot, rel)
		if err != nil {
			return err
		}
		if err := transactionRemove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	sort.Slice(directories, func(i, j int) bool { return len(directories[i]) > len(directories[j]) })
	for _, rel := range directories {
		if _, exists := after.directories[rel]; exists || rel == "." {
			continue
		}
		path, err := transactionSafePath(specRoot, rel)
		if err != nil {
			return err
		}
		if err := transactionRemove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func snapshotFileEqual(left, right specTreeSnapshot, rel string) bool {
	leftFile, leftExists := left.files[rel]
	rightFile, rightExists := right.files[rel]
	return leftExists == rightExists && (!leftExists || (leftFile.mode == rightFile.mode && bytes.Equal(leftFile.content, rightFile.content)))
}

func snapshotDirectoryEqual(left, right specTreeSnapshot, rel string) bool {
	leftDirectory, leftExists := left.directories[rel]
	rightDirectory, rightExists := right.directories[rel]
	return leftExists == rightExists && (!leftExists || leftDirectory == rightDirectory)
}

func snapshotDirectoryHasExternalDescendant(current, expected specTreeSnapshot, rel string) bool {
	prefix := rel + string(os.PathSeparator)
	for path := range current.files {
		if strings.HasPrefix(path, prefix) && !snapshotFileEqual(current, expected, path) {
			return true
		}
	}
	for path := range current.directories {
		if strings.HasPrefix(path, prefix) && !snapshotDirectoryEqual(current, expected, path) {
			return true
		}
	}
	return false
}

// transactionSafePath refuses to restore through a symlinked spec root, path
// component, or final path. Snapshotting deliberately does not follow links;
// restoration must preserve that guarantee or a link swapped in after the
// snapshot can redirect a rollback write outside the specification tree.
func transactionSafePath(specRoot, rel string) (string, error) {
	rootInfo, err := transactionLstat(specRoot)
	if err != nil {
		return "", fmt.Errorf("inspecting spec root: %w", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return "", fmt.Errorf("refusing non-directory or symlinked spec root %s", specRoot)
	}
	clean := filepath.Clean(rel)
	if clean == "." {
		return specRoot, nil
	}
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid transaction path %q", rel)
	}
	path := specRoot
	parts := strings.Split(clean, string(os.PathSeparator))
	for i, part := range parts {
		path = filepath.Join(path, part)
		info, err := transactionLstat(path)
		if os.IsNotExist(err) {
			return filepath.Join(specRoot, clean), nil
		}
		if err != nil {
			return "", fmt.Errorf("inspecting transaction path %s: %w", path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("refusing symlinked transaction path %s", path)
		}
		if i < len(parts)-1 && !info.IsDir() {
			return "", fmt.Errorf("refusing non-directory transaction path component %s", path)
		}
	}
	return filepath.Join(specRoot, clean), nil
}

// restore returns the spec tree's regular files and directories to their
// snapshot state. It removes files and empty directories created by the lint
// pass and recreates files or directories it removed.
func (snapshot specTreeSnapshot) restore(specRoot string) error {
	var createdDirectories []string
	err := filepath.Walk(specRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, _ := filepath.Rel(specRoot, path) // filepath.Walk only yields descendants.
		if info.IsDir() {
			if rel != "." {
				if _, existed := snapshot.directories[rel]; !existed {
					createdDirectories = append(createdDirectories, path)
				}
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if _, existed := snapshot.files[rel]; existed {
			return nil
		}
		if _, err := transactionSafePath(specRoot, rel); err != nil {
			return err
		}
		return transactionRemove(path)
	})
	if err != nil {
		return fmt.Errorf("removing lint-created files: %w", err)
	}
	sort.Slice(createdDirectories, func(i, j int) bool {
		return len(createdDirectories[i]) > len(createdDirectories[j])
	})
	for _, path := range createdDirectories {
		rel, _ := filepath.Rel(specRoot, path)
		if _, err := transactionSafePath(specRoot, rel); err != nil {
			return fmt.Errorf("refusing to remove lint-created directory %s: %w", path, err)
		}
		if err := transactionRemove(path); err != nil {
			return fmt.Errorf("removing lint-created directory %s: %w", path, err)
		}
	}

	directories := make([]string, 0, len(snapshot.directories))
	for rel := range snapshot.directories {
		directories = append(directories, rel)
	}
	sort.Strings(directories)
	for _, rel := range directories {
		directory := snapshot.directories[rel]
		path, err := transactionSafePath(specRoot, rel)
		if err != nil {
			return fmt.Errorf("refusing to recreate snapshot directory %s: %w", rel, err)
		}
		if err := transactionMkdirAll(path, directory.Perm()); err != nil {
			return fmt.Errorf("recreating snapshot directory %s: %w", rel, err)
		}
	}

	paths := make([]string, 0, len(snapshot.files))
	for rel := range snapshot.files {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	for _, rel := range paths {
		file := snapshot.files[rel]
		path, err := transactionSafePath(specRoot, rel)
		if err != nil {
			return fmt.Errorf("refusing to restore snapshot file %s: %w", rel, err)
		}
		if err := transactionMkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("recreating snapshot directory for %s: %w", rel, err)
		}
		if err := transactionWriteFile(path, file.content, file.mode.Perm()); err != nil {
			return fmt.Errorf("restoring snapshot file %s: %w", rel, err)
		}
	}
	return nil
}
