package cli

import (
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// LifecycleTransactionReceipt is the durable account of an exchanged staged
// project. Its predecessor is intentionally retained: an open descriptor may
// still write to that inode after exchange, so automatic reclamation would
// discard evidence that cannot safely be reconstructed.
type LifecycleTransactionReceipt struct {
	ID           string
	State        string
	ProjectRoot  string
	RecoveryRoot string
	// RecoveryRootIdentity is the device/inode identity of the held recovery
	// stage directory. Protocol-v2 publishing/committed receipts require it so
	// reopening RecoveryRoot cannot silently bind another transaction's tree.
	RecoveryRootIdentity string
	BaselineDigest       string
	StagedDigest         string
	CreatedAt            string
	// PublishingIntentRequired marks protocol-v2 transactions. It is written
	// before any staged mutation so a committed receipt must retain and validate
	// its publishing intent rather than silently accepting a missing sidecar.
	PublishingIntentRequired bool
}

var (
	lifecycleTransactionPlatformSupported = transactionPlatformSupportsSecureMutation
	lifecycleTransactionAbs               = filepath.Abs
	lifecycleTransactionAcquireLock       = acquireLifecycleLock
	lifecycleTransactionOpenProject       = openLifecycleProjectNoFollow
	lifecycleTransactionOpenChild         = openLifecycleProjectChildNoFollow
	lifecycleTransactionCreateStage       = createLifecycleStageProjectNoFollow
	lifecycleTransactionCreateStageSpec   = createLifecycleStageSpecNoFollow
	lifecycleTransactionStageIdentity     = lifecycleStageIdentity
	lifecycleTransactionFreezeContext     = materializeLifecycleProjectContext
	lifecycleTransactionRunStaged         = runLifecycleInStagedProject
	lifecycleTransactionChildMatches      = lifecycleProjectChildMatches
	lifecycleTransactionExchange          = exchangeLifecycleProjectSpecs
	lifecycleTransactionSnapshot          = snapshotStagedSpecTreeNoFollow
	lifecycleTransactionVerifySnapshot    = verifyLifecycleSnapshot
	lifecycleTransactionBeforePublication = func(*stagedSpecTree) error { return nil }
	lifecycleTransactionAfterExchange     = func(*stagedSpecTree, *stagedSpecTree) error { return nil }
	lifecycleTransactionNewID             = newLifecycleTransactionID
	lifecycleTransactionRandomRead        = cryptorand.Read
	lifecycleTransactionWriteReceipt      = writeLifecycleReceipt
	lifecycleTransactionRetainIntent      = retainLifecyclePublishingIntent
	lifecycleReceiptMarshal               = json.MarshalIndent
)

// RunLifecycleTransaction performs a lifecycle mutation in an isolated staged
// project. The callback receives "." while the process CWD is descriptor-anchored
// to that staged project; it must not retain the path after return. This makes
// package-level lifecycle code unable to reopen the live project during its
// mutation or lint pass.
//
// The returned receipt is written before and after publication. Recovery is
// deliberately read-only in this first surface: raw old-FD writes remain an
// unavoidable operating-system limitation, so a predecessor is never removed
// automatically or behind a retention cap.
func RunLifecycleTransaction(realProjectRoot string, op func(stagedProjectRoot string) error) (LifecycleTransactionReceipt, error) {
	if !lifecycleTransactionPlatformSupported() {
		return LifecycleTransactionReceipt{}, exitcode.UnexpectedErrorf(
			"secure lifecycle transactions are unavailable on this platform; refusing filesystem mutation")
	}
	if op == nil {
		return LifecycleTransactionReceipt{}, exitcode.UnexpectedErrorf("lifecycle transaction operation is required")
	}

	projectRoot, err := lifecycleTransactionAbs(realProjectRoot)
	if err != nil {
		return LifecycleTransactionReceipt{}, exitcode.UnexpectedErrorf("resolving lifecycle project root: %v", err)
	}
	lockPath, lockFile, err := lifecycleTransactionAcquireLock(projectRoot)
	if err != nil {
		return LifecycleTransactionReceipt{}, err
	}
	defer func() { _ = releaseLifecycleLockFile(lockPath, lockFile) }()

	project, err := lifecycleTransactionOpenProject(projectRoot)
	if err != nil {
		return LifecycleTransactionReceipt{}, exitcode.UnexpectedErrorf("opening lifecycle project without following links: %v", err)
	}
	defer func() { _ = closeStagedSpecTree(project) }()
	liveSpec, err := lifecycleTransactionOpenChild(project, "spec")
	if err != nil {
		return LifecycleTransactionReceipt{}, exitcode.UnexpectedErrorf("opening live spec tree without following links: %v", err)
	}
	defer func() { _ = closeStagedSpecTree(liveSpec) }()
	baseline, err := lifecycleTransactionSnapshot(liveSpec)
	if err != nil {
		return LifecycleTransactionReceipt{}, exitcode.UnexpectedErrorf("snapshotting live spec tree: %v", err)
	}

	id, err := lifecycleTransactionNewID()
	if err != nil {
		return LifecycleTransactionReceipt{}, exitcode.UnexpectedErrorf("generating lifecycle transaction id: %v", err)
	}
	receipt := LifecycleTransactionReceipt{
		ID:                       id,
		State:                    "prepared",
		ProjectRoot:              projectRoot,
		RecoveryRoot:             filepath.Join(projectRoot, ".specscore-txn-"+id),
		BaselineDigest:           lifecycleSnapshotDigest(baseline),
		CreatedAt:                time.Now().UTC().Format(time.RFC3339Nano),
		PublishingIntentRequired: true,
	}
	if err := lifecycleTransactionWriteReceipt(project, receipt); err != nil {
		return LifecycleTransactionReceipt{}, err
	}
	stage, err := lifecycleTransactionCreateStage(project, id)
	if err != nil {
		return receipt, exitcode.UnexpectedErrorf("creating staged lifecycle project: %v", err)
	}
	stagePath := stage.path
	defer func() { _ = closeStagedSpecTree(stage) }()
	receipt.RecoveryRootIdentity, err = lifecycleTransactionStageIdentity(stage)
	if err != nil {
		return receipt, exitcode.UnexpectedErrorf("identifying staged lifecycle project: %v", err)
	}
	// The first receipt is intentionally durable before the stage exists. This
	// second prepared update binds that now-held stage directory before any of
	// its spec/context descendants are created or materialized.
	if err := lifecycleTransactionWriteReceipt(project, receipt); err != nil {
		return receipt, err
	}
	stageSpec, err := lifecycleTransactionCreateStageSpec(stage, baseline)
	if err != nil {
		return receipt, exitcode.UnexpectedErrorf("materializing staged spec tree: %v", err)
	}
	defer func() { _ = closeStagedSpecTree(stageSpec) }()
	if err := lifecycleTransactionFreezeContext(project, stage); err != nil {
		return receipt, exitcode.UnexpectedErrorf("freezing staged project context: %v", err)
	}

	if err := lifecycleTransactionRunStaged(stage, op); err != nil {
		return receipt, err
	}
	staged, err := lifecycleTransactionSnapshot(stageSpec)
	if err != nil {
		return receipt, exitcode.UnexpectedErrorf("snapshotting staged lifecycle output: %v", err)
	}
	receipt.StagedDigest = lifecycleSnapshotDigest(staged)
	receipt.State = "publishing"
	if err := lifecycleTransactionWriteReceipt(project, receipt); err != nil {
		return receipt, err
	}
	if err := lifecycleTransactionBeforePublication(stageSpec); err != nil {
		return receipt, err
	}
	if err := lifecycleTransactionChildMatches(project, "spec", liveSpec); err != nil {
		return receipt, exitcode.UnexpectedErrorf("validating live spec identity before publication: %v", err)
	}
	if err := lifecycleTransactionChildMatches(stage, "spec", stageSpec); err != nil {
		return receipt, exitcode.UnexpectedErrorf("validating staged spec identity before publication: %v", err)
	}
	if err := lifecycleTransactionRecoveryStageMatches(project, id, stage, receipt.RecoveryRootIdentity); err != nil {
		return receipt, exitcode.UnexpectedErrorf("validating staged recovery identity before publication: %v", err)
	}
	if err := lifecycleTransactionVerifySnapshot(liveSpec, baseline); err != nil {
		return receipt, exitcode.UnexpectedErrorf("live spec changed before lifecycle publication: %v", err)
	}
	if err := lifecycleTransactionVerifySnapshot(stageSpec, staged); err != nil {
		return receipt, exitcode.UnexpectedErrorf("staged lifecycle output changed before publication: %v", err)
	}
	if err := lifecycleTransactionExchange(project, stage); err != nil {
		return receipt, exitcode.UnexpectedErrorf("atomically publishing staged lifecycle project: %v", err)
	}
	if err := lifecycleTransactionAfterExchange(liveSpec, stageSpec); err != nil {
		return receipt, err
	}
	if err := lifecycleTransactionChildMatches(project, "spec", stageSpec); err != nil {
		return receipt, exitcode.UnexpectedErrorf("published spec identity is uncertain; retained recovery tree at %s: %v", stagePath, err)
	}
	if err := lifecycleTransactionRecoveryStageMatches(project, id, stage, receipt.RecoveryRootIdentity); err != nil {
		return receipt, exitcode.UnexpectedErrorf("recovery root identity is uncertain; retained recovery tree at %s: %v", stagePath, err)
	}
	if err := lifecycleTransactionChildMatches(stage, "spec", liveSpec); err != nil {
		return receipt, exitcode.UnexpectedErrorf("recovery predecessor identity is uncertain; retained recovery tree at %s: %v", stagePath, err)
	}
	if err := lifecycleTransactionVerifySnapshot(stageSpec, staged); err != nil {
		return receipt, exitcode.UnexpectedErrorf("published lifecycle output changed during publication; retained recovery tree at %s: %v", stagePath, err)
	}
	if err := lifecycleTransactionVerifySnapshot(liveSpec, baseline); err != nil {
		return receipt, exitcode.UnexpectedErrorf("recovery predecessor changed during lifecycle publication; retained recovery tree at %s: %v", stagePath, err)
	}
	if err := lifecycleTransactionRetainIntent(project, receipt); err != nil {
		return lifecycleOutcomeUncertainReceipt(receipt), lifecycleOutcomeUncertainError(projectRoot, receipt, err)
	}
	if err := lifecycleTransactionRecoveryStageMatches(project, id, stage, receipt.RecoveryRootIdentity); err != nil {
		return lifecycleOutcomeUncertainReceipt(receipt), lifecycleOutcomeUncertainError(projectRoot, receipt, err)
	}
	committedReceipt := receipt
	committedReceipt.State = "committed"
	if err := lifecycleTransactionWriteReceipt(project, committedReceipt); err != nil {
		return lifecycleOutcomeUncertainReceipt(receipt), lifecycleOutcomeUncertainError(projectRoot, receipt, err)
	}
	if err := lifecycleTransactionRecoveryStageMatches(project, id, stage, receipt.RecoveryRootIdentity); err != nil {
		return lifecycleOutcomeUncertainReceipt(receipt), lifecycleOutcomeUncertainError(projectRoot, receipt, err)
	}
	return committedReceipt, nil
}

// lifecycleTransactionRecoveryStageMatches proves that the recovery-root name
// is still the held stage directory and that its durable identity has not been
// replaced. It is checked before publication and immediately before and after
// recording a terminal receipt; recovery repeats the token check before
// opening spec.
func lifecycleTransactionRecoveryStageMatches(project *stagedSpecTree, id string, stage *stagedSpecTree, expectedIdentity string) error {
	if err := lifecycleTransactionChildMatches(project, ".specscore-txn-"+id, stage); err != nil {
		return err
	}
	actualIdentity, err := lifecycleTransactionStageIdentity(stage)
	if err != nil {
		return err
	}
	if expectedIdentity == "" || actualIdentity != expectedIdentity {
		return fmt.Errorf("recovery stage identity changed")
	}
	return nil
}

// lifecycleOutcomeUncertainReceipt is returned only when durable publication
// succeeded but the final receipt protocol reported an error. POSIX fsync
// errors are outcome-ambiguous: the final namespace update may or may not
// already survive a power loss, so callers must inspect the recovery receipt
// rather than infer either publishing or committed.
func lifecycleOutcomeUncertainReceipt(receipt LifecycleTransactionReceipt) LifecycleTransactionReceipt {
	receipt.State = "outcome-uncertain"
	return receipt
}

func lifecycleOutcomeUncertainError(projectRoot string, receipt LifecycleTransactionReceipt, cause error) error {
	return exitcode.ConflictErrorf(
		"lifecycle transaction %s outcome uncertain after durable publication; inspect recovery receipt .specscore-recovery/%s.json with specscore recovery list --project %s before retrying: %v",
		receipt.ID,
		receipt.ID,
		projectRoot,
		cause,
	)
}

func newLifecycleTransactionID() (string, error) {
	var entropy [12]byte
	if _, err := lifecycleTransactionRandomRead(entropy[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d-%d-%s", time.Now().UTC().UnixNano(), os.Getpid(), hex.EncodeToString(entropy[:])), nil
}

func lifecycleSnapshotDigest(snapshot specTreeSnapshot) string {
	h := sha256.New()
	digestWriteString(h, "specscore.lifecycle.snapshot.v1")
	directories := make([]string, 0, len(snapshot.directories))
	for path := range snapshot.directories {
		directories = append(directories, path)
	}
	sort.Strings(directories)
	for _, path := range directories {
		directory := snapshot.directories[path]
		digestWriteString(h, "directory")
		digestWriteString(h, path)
		digestWriteUint64(h, uint64(directory.mode))
		digestWriteMetadata(h, directory.metadata)
	}
	files := make([]string, 0, len(snapshot.files))
	for path := range snapshot.files {
		files = append(files, path)
	}
	sort.Strings(files)
	for _, path := range files {
		file := snapshot.files[path]
		digestWriteString(h, "file")
		digestWriteString(h, path)
		digestWriteUint64(h, uint64(file.mode))
		digestWriteBytes(h, file.content)
		digestWriteMetadata(h, file.metadata)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// digestWriteMetadata intentionally excludes atime. Descriptor reads made by
// transaction and recovery verification can update atime, whereas the staged
// materialiser still preserves the captured value. Every other observable
// snapshot field that can affect lifecycle publication is length-delimited.
func digestWriteMetadata(h io.Writer, metadata specTreeEntryMetadata) {
	digestWriteInt64(h, metadata.modificationTime.Unix())
	digestWriteUint64(h, uint64(metadata.modificationTime.Nanosecond()))
	names := make([]string, 0, len(metadata.extendedAttributes))
	for name := range metadata.extendedAttributes {
		names = append(names, name)
	}
	sort.Strings(names)
	digestWriteUint64(h, uint64(len(names)))
	for _, name := range names {
		digestWriteString(h, name)
		digestWriteBytes(h, metadata.extendedAttributes[name])
	}
}

func digestWriteString(h io.Writer, value string) { digestWriteBytes(h, []byte(value)) }

func digestWriteBytes(h io.Writer, value []byte) {
	digestWriteUint64(h, uint64(len(value)))
	_, _ = h.Write(value)
}

func digestWriteUint64(h io.Writer, value uint64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	_, _ = h.Write(encoded[:])
}

func digestWriteInt64(h io.Writer, value int64) { digestWriteUint64(h, uint64(value)) }

func verifyLifecycleSnapshot(tree *stagedSpecTree, expected specTreeSnapshot) error {
	actual, err := lifecycleTransactionSnapshot(tree)
	if err != nil {
		return err
	}
	if lifecycleSnapshotDigest(actual) != lifecycleSnapshotDigest(expected) {
		return fmt.Errorf("descriptor-rooted snapshot digest changed")
	}
	return nil
}

func writeLifecycleReceipt(project *stagedSpecTree, receipt LifecycleTransactionReceipt) error {
	if !validLifecycleTransactionID(receipt.ID) {
		return exitcode.UnexpectedErrorf("invalid lifecycle transaction receipt id %q", receipt.ID)
	}
	data, err := lifecycleReceiptMarshal(receipt, "", "  ")
	if err != nil {
		return exitcode.UnexpectedErrorf("encoding lifecycle recovery receipt: %v", err)
	}
	if err := writeLifecycleReceiptNoFollow(project, receipt.ID+".json", append(data, '\n')); err != nil {
		return exitcode.UnexpectedErrorf("writing lifecycle recovery receipt: %v", err)
	}
	return nil
}

func validLifecycleTransactionID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		if (r < '0' || r > '9') && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && r != '-' {
			return false
		}
	}
	return true
}
