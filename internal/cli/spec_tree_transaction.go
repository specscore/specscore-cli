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
	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

// specTreeSnapshot captures every regular file and directory below a spec root
// before a lifecycle post-mutation lint pass. The transition itself owns
// restoration of its artifact; this snapshot owns every additional path the
// lint fixers touch (indexes, footers, mirrors, and any newly-created artifact).
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

// specTreeEntryMetadata contains the descriptor-readable metadata that must
// travel with a copy-on-write tree.  The scanner captures xattrs (including
// filesystem ACL representations) by descriptor and the stage materialiser
// restores them by descriptor; unsupported metadata fails closed instead.
type specTreeEntryMetadata struct {
	extendedAttributes map[string][]byte
	modificationTime   time.Time
}

// stagedSpecTree keeps the descriptor used for every private-tree operation
// alive until publication has classified the exchange. The pathname is only a
// publication label; lint and materialisation use the descriptor handoff.
type stagedSpecTree struct {
	path string
	root *os.File
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
	recoveryPaths        []string
	passthrough          bool
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
	transactionReadFile  = os.ReadFile
	transactionOpenFile  = os.OpenFile
	transactionCloseFile = func(file *os.File) error { return file.Close() }
	transactionLstat     = os.Lstat
	transactionRel       = filepath.Rel
	transactionMkdirTemp = os.MkdirTemp
	transactionRemoveAll = os.RemoveAll
	// The no-follow traversal supplies an already-open descriptor to this
	// reader; keeping it injectable makes descriptor-read failures testable
	// without ever reopening a path by name.
	transactionReadSnapshotFile = io.ReadAll
	transactionSnapshot         = snapshotSpecTreeForTransaction
	transactionSnapshotStaged   = snapshotStagedSpecTreeNoFollow
	transactionStageMatchesPath = stagedSpecTreeMatchesPath
	transactionStagePublishedAt = stagedSpecTreePublishedAt
	transactionCloseStagedTree  = closeStagedSpecTree
	transactionPublishTree      = publishSpecTreeNoReplace
	// transactionAfterRecoveryClaim is a deterministic test seam between the
	// two no-replace renames. Production leaves it as a no-op; tests install a
	// raw successor here to prove the second rename never overwrites it.
	transactionAfterRecoveryClaim = func() {}
	// The test seam runs after the final stage-name identity check and before
	// exchange. Lint has already run through the retained descriptor, so a
	// replacement installed here must remain untouched and be classified after
	// the atomic exchange rather than published as trusted output.
	transactionAfterStageValidation = func() {}
	transactionLockFile             = acquireLifecycleFileLock
	transactionProcessAlive         = defaultProcessAlive
	// Platforms without the descriptor-relative implementation preserve the
	// historical lifecycle flow rather than silently disabling every mutating
	// command. This seam gives the platform adapter runtime coverage on Unix.
	transactionPlatformSupportsSecureMutation = platformSupportsSecureLifecycleTransaction
)

var errLifecycleLockHeld = errors.New("lifecycle lock is held")

const maxRetainedLifecycleTrees = 8

func snapshotSpecTreeForTransaction(specRoot string) (specTreeSnapshot, error) {
	return snapshotSpecTreeNoFollow(specRoot)
}

func beginSpecTreeTransaction(specRoot string, lifecycleOwnedPaths ...string) (*specTreeTransaction, error) {
	ownedPaths, err := normalizeLifecycleOwnedPaths(lifecycleOwnedPaths)
	if err != nil {
		return nil, err
	}
	if !transactionPlatformSupportsSecureMutation() {
		return &specTreeTransaction{
			specRoot:            specRoot,
			ownedLifecyclePaths: ownedPaths,
			passthrough:         true,
		}, nil
	}
	if err := ensureLifecycleRecoveryCapacity(specRoot); err != nil {
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

// ensureLifecycleRecoveryCapacity bounds the predecessor trees intentionally
// retained by atomic exchange. Removing an exchanged directory by pathname
// after an identity check has an unavoidable final pathname race, so this
// transaction never does that. Once the cap is reached we fail before the
// lifecycle package mutates anything and tell the operator where to inspect
// and deliberately reclaim verified historical trees.
func ensureLifecycleRecoveryCapacity(specRoot string) error {
	entries, err := os.ReadDir(filepath.Dir(specRoot))
	if err != nil {
		return fmt.Errorf("inspecting retained lifecycle trees: %w", err)
	}
	retained := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".specscore-lint-stage-") {
			retained++
		}
	}
	if retained >= maxRetainedLifecycleTrees {
		return exitcode.ConflictErrorf(
			"lifecycle recovery retention limit (%d) reached beside %s; inspect retained .specscore-lint-stage-* trees, preserve any raw work, then manually reclaim only verified predecessors before retrying",
			maxRetainedLifecycleTrees,
			specRoot,
		)
	}
	return nil
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
	// A legacy lock is a directory and can be replaced between any pathname
	// revalidation and rmdir. Do not reclaim it automatically: deleting a
	// successor would be worse than requiring a one-time manual cleanup of a
	// stale pre-file-lock artifact.
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
	return exitcode.UnexpectedErrorf("legacy lifecycle transaction lock at %s requires manual recovery; refusing non-atomic directory deletion", lockPath)
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
	if transaction.passthrough {
		return func() error { return run(transaction.specRoot) }
	}
	return func() error {
		// The lifecycle package has already changed its source artifact before
		// calling us. Claim the outer rollback boundary before the first read:
		// a failed snapshot cannot prove that a pathname is still command-owned,
		// so returning a normal error here would let the package blindly write
		// through a raw successor.
		transaction.postMutationStarted = true
		before, err := transactionSnapshot(transaction.specRoot)
		if err != nil {
			return lifecycle.DeferRollback(exitcode.UnexpectedErrorf("snapshotting spec before lint --fix: %v", err))
		}
		transaction.preLintSnapshot = &before
		if unownedPaths := transaction.unownedPreLintPaths(before); len(unownedPaths) > 0 {
			transaction.preLintUnownedPaths = unownedPaths
			transaction.postLintSnapshot = &before
			return lifecycle.DeferRollback(exitcode.UnexpectedErrorf(
				"uncoordinated spec changes before lint --fix at %s", strings.Join(unownedPaths, ", ")))
		}

		stage, err := openStagedSpecTreeSnapshot(transaction.specRoot, before)
		if err != nil {
			transaction.postLintSnapshot = &before
			return lifecycle.DeferRollback(exitcode.UnexpectedErrorf("creating isolated lint tree: %v", err))
		}
		stageRetained, stagePublished := false, false
		defer func() {
			_ = closeStagedSpecTree(stage)
			if !stageRetained && !stagePublished {
				_ = transactionRemoveAll(stage.path)
			}
		}()
		runErr := runLintInStagedSpecTree(stage, run)
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
		afterClone, err := transactionSnapshotStaged(stage)
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
		if specTreeSnapshotsEqual(before, afterClone) {
			transaction.postLintSnapshot = &before
			return lifecycle.DeferRollback(nil)
		}
		stageMatches, err := transactionStageMatchesPath(stage)
		if err != nil || !stageMatches {
			transaction.postLintSnapshot = &before
			if err != nil {
				return lifecycle.DeferRollback(exitcode.UnexpectedErrorf("validating staged lint tree identity: %v", err))
			}
			return lifecycle.DeferRollback(exitcode.UnexpectedErrorf("staged lint tree was replaced before publication; preserved live spec tree"))
		}
		transactionAfterStageValidation()
		recoveryPath, err := transactionPublishTree(transaction.specRoot, stage.path)
		if err != nil {
			if recoveryPath != "" {
				// The publisher already moved the old tree aside. Retain the
				// stage as well: the subsequent no-replace claim found a raw
				// successor, so deleting either candidate would lose evidence.
				stageRetained = true
				transaction.recoveryPaths = append(transaction.recoveryPaths, recoveryPath, stage.path)
				err = fmt.Errorf("%w; retained staged lint tree at %s", err, stage.path)
			}
			current, _ := transactionSnapshot(transaction.specRoot)
			transaction.postLintSnapshot = &current
			return lifecycle.DeferRollback(exitcode.UnexpectedErrorf("atomically publishing isolated lint tree: %v", err))
		}
		stagePublished = true
		publishedMatches, matchErr := transactionStagePublishedAt(stage, transaction.specRoot)
		if matchErr != nil || !publishedMatches {
			stageRetained = true
			transaction.recoveryPaths = append(transaction.recoveryPaths, recoveryPath)
			if matchErr != nil {
				return lifecycle.DeferRollback(exitcode.UnexpectedErrorf("validating published staged tree identity: %v; retained prior live tree at %s", matchErr, recoveryPath))
			}
			return lifecycle.DeferRollback(exitcode.UnexpectedErrorf("staged lint tree was replaced during publication; raw successor remains live and prior live tree is retained at %s", recoveryPath))
		}
		previous, previousErr := transactionSnapshot(recoveryPath)
		if previousErr != nil || !specTreeSnapshotsEqual(previous, before) {
			stageRetained = true
			transaction.recoveryPaths = append(transaction.recoveryPaths, recoveryPath)
			if previousErr != nil {
				return lifecycle.DeferRollback(exitcode.UnexpectedErrorf("snapshotting prior live tree after publication: %v; retained it at %s", previousErr, recoveryPath))
			}
			return lifecycle.DeferRollback(exitcode.UnexpectedErrorf("raw changes raced lifecycle publication; raw predecessor is retained at %s", recoveryPath))
		}
		after, err := transactionSnapshot(transaction.specRoot)
		if err != nil {
			return lifecycle.DeferRollback(exitcode.UnexpectedErrorf("snapshotting spec after lint manifest: %v", err))
		}
		if !specTreeSnapshotsEqual(after, afterClone) {
			transaction.postLintSnapshot = &after
			transaction.preLintUnownedPaths = changedSnapshotPaths(afterClone, after)
			return lifecycle.DeferRollback(exitcode.UnexpectedErrorf("uncoordinated spec changes while applying lint manifest at %s", strings.Join(transaction.preLintUnownedPaths, ", ")))
		}
		transaction.postLintSnapshot = &after
		return lifecycle.DeferRollback(nil)
	}
}

// stageSpecTreeSnapshot materializes a private sibling of specRoot.  It must
// be a sibling, rather than a system-temp directory, because publication uses
// a same-directory no-replace rename as its atomic claim.
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

// stageSpecTreeSnapshot remains a focused-test compatibility helper. Runtime
// lifecycle code must use openStagedSpecTreeSnapshot so it never loses the
// descriptor identity before lint or publication.
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

// materializeSpecSnapshot is retained for focused callers and tests. Runtime
// lifecycle code uses openStagedSpecTreeSnapshot; this helper nevertheless
// uses the same descriptor-relative materialiser so it cannot silently lose
// xattrs, modes, or nanosecond timestamps.
func materializeSpecSnapshot(root string, snapshot specTreeSnapshot) error {
	stage, err := openStagedSpecTreeNoFollow(root)
	if err != nil {
		return fmt.Errorf("opening isolated materialisation root without following links: %w", err)
	}
	defer func() { _ = closeStagedSpecTree(stage) }()
	if err := materializeStagedSpecTreeNoFollow(stage, snapshot); err != nil {
		return err
	}
	return nil
}

// validateSpecTreeSnapshot accepts only the relative names produced by the
// descriptor-relative scanner. It is deliberately pure validation: copy-on-
// write publication must never inspect a live path and then later mutate it.
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

// captureLifecycleMutationState records the exact filesystem state produced by
// the lifecycle package after its final artifact write and before it calls the
// post-mutation hook. A later pre-lint snapshot must match this evidence for
// every declared lifecycle artifact; otherwise a raw writer won the race and
// rollback is conservatively withheld.
func (transaction *specTreeTransaction) captureLifecycleMutationState() error {
	if transaction.passthrough {
		return nil
	}
	snapshot, err := transactionSnapshot(transaction.specRoot)
	if err != nil {
		transaction.postMutationStarted = true
		return lifecycle.DeferRollback(exitcode.UnexpectedErrorf("capturing lifecycle-owned state before lint --fix: %v", err))
	}
	transaction.provenLifecycleState = &snapshot
	return nil
}

// finish restores lint-owned paths after a post-mutation failure. Errors before
// the hook began are handled by the lifecycle package's own rollback and require
// no transaction restoration.
func (transaction *specTreeTransaction) finish(actionErr error) error {
	if transaction.passthrough {
		return actionErr
	}
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
	current, err := transactionSnapshot(transaction.specRoot)
	if err != nil {
		return fmt.Errorf("snapshotting current spec tree: %w", err)
	}

	// The initial snapshot includes the lifecycle source mutation as well as
	// lint output. It is intentionally the rollback baseline: packages receive
	// DeferredRollbackError and therefore do not blindly restore their source
	// before this compare-and-restore step can protect raw filesystem edits.
	// Include paths that appeared after the lint manifest as well as paths the
	// manifest itself changed. A raw writer can add an otherwise unrelated
	// file; publishing the old snapshot in that case would hide its work in a
	// recovery tree without surfacing a conflict.
	filePaths := unionSnapshotPaths(
		changedSnapshotFiles(transaction.snapshot, *transaction.postLintSnapshot),
		changedSnapshotFiles(*transaction.postLintSnapshot, current),
	)
	directoryPaths := unionSnapshotPaths(
		changedSnapshotDirectories(transaction.snapshot, *transaction.postLintSnapshot),
		changedSnapshotDirectories(*transaction.postLintSnapshot, current),
	)
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

	recoveryPath, err := publishSnapshotFromHeldStage(transaction.specRoot, transaction.snapshot, current)
	if err != nil {
		if recoveryPath != "" {
			transaction.recoveryPaths = append(transaction.recoveryPaths, recoveryPath)
			return fmt.Errorf("atomically publishing pre-transition snapshot: %w; retained prior live tree at %s", err, recoveryPath)
		}
		return fmt.Errorf("atomically publishing pre-transition snapshot: %w", err)
	}
	return nil
}

// publishSnapshotFromHeldStage is the rollback counterpart to the post-lint
// publisher. It holds the staged tree descriptor through materialisation,
// final name validation, exchange, and published-identity validation. The
// previous live tree is checked against expectedPrevious after exchange; on
// any uncertainty its pathname is returned for explicit recovery rather than
// being cleaned by name.
func publishSnapshotFromHeldStage(specRoot string, snapshot, expectedPrevious specTreeSnapshot) (string, error) {
	stage, err := openStagedSpecTreeSnapshot(specRoot, snapshot)
	if err != nil {
		return "", err
	}
	defer func() { _ = closeStagedSpecTree(stage) }()
	matches, err := transactionStageMatchesPath(stage)
	if err != nil {
		return "", fmt.Errorf("validating staged rollback tree identity: %w", err)
	}
	if !matches {
		return "", errors.New("staged rollback tree was replaced before publication")
	}
	recoveryPath, err := transactionPublishTree(specRoot, stage.path)
	if err != nil {
		return recoveryPath, err
	}
	published, err := transactionStagePublishedAt(stage, specRoot)
	if err != nil {
		return recoveryPath, fmt.Errorf("validating published rollback tree identity: %w", err)
	}
	if !published {
		return recoveryPath, errors.New("staged rollback tree was replaced during publication")
	}
	previous, err := transactionSnapshot(recoveryPath)
	if err != nil {
		return recoveryPath, fmt.Errorf("snapshotting prior live tree after rollback publication: %w", err)
	}
	if !specTreeSnapshotsEqual(previous, expectedPrevious) {
		return recoveryPath, errors.New("raw changes raced rollback publication")
	}
	return recoveryPath, nil
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

func changedSnapshotPaths(before, after specTreeSnapshot) []string {
	paths := changedSnapshotFiles(before, after)
	for _, path := range changedSnapshotDirectories(before, after) {
		paths = append(paths, path+"/")
	}
	sort.Strings(paths)
	return paths
}

// applySpecSnapshotDiff is retained for focused callers and tests. It never
// mutates a path inside the live tree: after a conservative snapshot comparison
// it publishes a complete sibling tree with the same no-replace primitive used
// by lifecycle transactions.
func applySpecSnapshotDiff(specRoot string, before, after specTreeSnapshot) error {
	current, err := transactionSnapshot(specRoot)
	if err != nil {
		return fmt.Errorf("snapshotting live tree before atomic manifest publication: %w", err)
	}
	if !specTreeSnapshotsEqual(current, before) {
		return &concurrentSpecChangesError{paths: changedSnapshotPaths(before, current)}
	}
	stageRoot, err := stageSpecTreeSnapshot(specRoot, after)
	if err != nil {
		return err
	}
	if recoveryPath, err := transactionPublishTree(specRoot, stageRoot); err != nil {
		if recoveryPath != "" {
			return fmt.Errorf("%w; retained staged manifest at %s", err, stageRoot)
		}
		return err
	}
	return nil
}

func snapshotFileEqual(left, right specTreeSnapshot, rel string) bool {
	leftFile, leftExists := left.files[rel]
	rightFile, rightExists := right.files[rel]
	return leftExists == rightExists && (!leftExists || (leftFile.mode == rightFile.mode && bytes.Equal(leftFile.content, rightFile.content) && reflect.DeepEqual(leftFile.metadata, rightFile.metadata)))
}

func snapshotDirectoryEqual(left, right specTreeSnapshot, rel string) bool {
	leftDirectory, leftExists := left.directories[rel]
	rightDirectory, rightExists := right.directories[rel]
	// Moving or creating an owned lifecycle artifact updates every ancestor
	// directory mtime. That is expected filesystem bookkeeping, not an
	// unowned tree edit. Modes and xattrs remain part of the conflict proof;
	// the staged materialiser still restores the captured timestamp exactly.
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

// restore rolls a tree back through the same sibling copy-on-write publication
// used by live lifecycle commands. It deliberately never walks a live path and
// then writes it: a raw editor's old inode is retained in the recovery tree.
func (snapshot specTreeSnapshot) restore(specRoot string) error {
	stageRoot, err := stageSpecTreeSnapshot(specRoot, snapshot)
	if err != nil {
		return fmt.Errorf("staging snapshot restore: %w", err)
	}
	if recoveryPath, err := transactionPublishTree(specRoot, stageRoot); err != nil {
		if recoveryPath != "" {
			return fmt.Errorf("atomically publishing snapshot restore: %w; retained staged snapshot at %s", err, stageRoot)
		}
		return fmt.Errorf("atomically publishing snapshot restore: %w", err)
	}
	return nil
}
