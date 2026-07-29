package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	transactionMkdir        = os.Mkdir
	transactionOpenFile     = os.OpenFile
	transactionCloseFile    = func(file *os.File) error { return file.Close() }
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
	snapshot, err := snapshotSpecTreeForTransaction(specRoot)
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
	rel, err := filepath.Rel(specRoot, path)
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
		if !isDirectory(lockPath) {
			return "", nil, exitcode.UnexpectedErrorf("acquiring lifecycle transaction lock %s: %v", lockPath, err)
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

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// lifecycleLegacyLockIsStale handles only the directory lock format written
// by pre-file-lock releases. New locks are advisory OS file locks; an unlocked
// stale file never blocks a later command, even if it survives a crash.
func lifecycleLegacyLockIsStale(lockPath string) (bool, error) {
	owner, err := transactionReadFile(lifecycleLockOwnerPath(lockPath))
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, exitcode.UnexpectedErrorf("reading lifecycle transaction lock owner at %s: %v", lockPath, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(owner)))
	if err != nil || pid <= 0 {
		return true, nil
	}
	return !transactionProcessAlive(pid), nil
}

func releaseLegacyLifecycleLock(lockPath string) error {
	ownerPath := lifecycleLockOwnerPath(lockPath)
	if err := transactionRemove(ownerPath); err != nil && !os.IsNotExist(err) {
		return exitcode.UnexpectedErrorf("releasing lifecycle transaction lock owner %s: %v", ownerPath, err)
	}
	if err := transactionRemove(lockPath); err != nil && !os.IsNotExist(err) {
		return exitcode.UnexpectedErrorf("releasing lifecycle transaction lock %s: %v", lockPath, err)
	}
	return nil
}

// releaseLifecycleLockFile unlinks before closing. A process that opens the
// pathname after unlink creates and locks a fresh inode, but this transaction
// has already completed every spec mutation and is only releasing. Unlinking
// first prevents a new command from contending on an inode that can soon be
// orphaned; a crash leaves at most an unlocked file, not a permanent blocker.
func releaseLifecycleLockFile(lockPath string, lockFile *os.File) error {
	removeErr := transactionRemove(lockPath)
	closeErr := transactionCloseFile(lockFile)
	if removeErr != nil && !os.IsNotExist(removeErr) {
		if closeErr != nil {
			return exitcode.UnexpectedErrorf("releasing lifecycle transaction lock %s: %v; additionally closing it failed: %v", lockPath, removeErr, closeErr)
		}
		return exitcode.UnexpectedErrorf("releasing lifecycle transaction lock %s: %v", lockPath, removeErr)
	}
	if closeErr != nil {
		return exitcode.UnexpectedErrorf("closing lifecycle transaction lock %s: %v", lockPath, closeErr)
	}
	return nil
}

// postMutationHook runs the normal lint hook after a lifecycle package has
// rewritten or relocated its artifact. It must not restore eagerly: a normal
// hook failure is rolled back by the lifecycle package, while a deferred
// failure is rolled back conflict-safely by finish.
func (transaction *specTreeTransaction) postMutationHook() func() error {
	return transaction.postMutationHookWith(lintPostMutationHook(transaction.specRoot))
}

// captureLifecycleMutationState records the exact filesystem state produced by
// the lifecycle package after its final artifact write and before it calls the
// post-mutation hook. A later pre-lint snapshot must match this evidence for
// every declared lifecycle artifact; otherwise a raw writer won the race and
// rollback is conservatively withheld.
func (transaction *specTreeTransaction) captureLifecycleMutationState() error {
	snapshot, err := snapshotSpecTreeForTransaction(transaction.specRoot)
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
		before, err := snapshotSpecTreeForTransaction(transaction.specRoot)
		if err != nil {
			return exitcode.UnexpectedErrorf("snapshotting spec before lint --fix: %v", err)
		}
		transaction.postMutationStarted = true
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
		after, snapshotErr := snapshotSpecTreeForTransaction(transaction.specRoot)
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
	current, err := snapshotSpecTreeForTransaction(transaction.specRoot)
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
		if err := transactionMkdirAll(filepath.Join(transaction.specRoot, rel), directory.Perm()); err != nil {
			return fmt.Errorf("recreating pre-transition directory %s: %w", rel, err)
		}
	}
	for _, rel := range filePaths {
		original, existed := transaction.snapshot.files[rel]
		if !existed {
			continue
		}
		path := filepath.Join(transaction.specRoot, rel)
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
		if err := transactionRemove(filepath.Join(transaction.specRoot, rel)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing lint-created file %s: %w", rel, err)
		}
	}
	sort.Slice(directoryPaths, func(i, j int) bool { return len(directoryPaths[i]) > len(directoryPaths[j]) })
	for _, rel := range directoryPaths {
		if _, existed := transaction.snapshot.directories[rel]; existed || rel == "." {
			continue
		}
		if err := transactionRemove(filepath.Join(transaction.specRoot, rel)); err != nil && !os.IsNotExist(err) {
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
		return transactionRemove(path)
	})
	if err != nil {
		return fmt.Errorf("removing lint-created files: %w", err)
	}
	sort.Slice(createdDirectories, func(i, j int) bool {
		return len(createdDirectories[i]) > len(createdDirectories[j])
	})
	for _, path := range createdDirectories {
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
		if err := transactionMkdirAll(filepath.Join(specRoot, rel), directory.Perm()); err != nil {
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
		path := filepath.Join(specRoot, rel)
		if err := transactionMkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("recreating snapshot directory for %s: %w", rel, err)
		}
		if err := transactionWriteFile(path, file.content, file.mode.Perm()); err != nil {
			return fmt.Errorf("restoring snapshot file %s: %w", rel, err)
		}
	}
	return nil
}
