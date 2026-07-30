package cli

import (
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// LifecycleTransactionReceipt is the durable account of an exchanged staged
// project. Its predecessor is intentionally retained: an open descriptor may
// still write to that inode after exchange, so automatic reclamation would
// discard evidence that cannot safely be reconstructed.
type LifecycleTransactionReceipt struct {
	ID             string
	State          string
	ProjectRoot    string
	RecoveryRoot   string
	BaselineDigest string
	StagedDigest   string
	CreatedAt      string
}

var (
	lifecycleTransactionPlatformSupported = transactionPlatformSupportsSecureMutation
	lifecycleTransactionAbs               = filepath.Abs
	lifecycleTransactionAcquireLock       = acquireLifecycleLock
	lifecycleTransactionOpenProject       = openLifecycleProjectNoFollow
	lifecycleTransactionOpenChild         = openLifecycleProjectChildNoFollow
	lifecycleTransactionCreateStage       = createLifecycleStageProjectNoFollow
	lifecycleTransactionCreateStageSpec   = createLifecycleStageSpecNoFollow
	lifecycleTransactionFreezeContext     = materializeLifecycleProjectContext
	lifecycleTransactionRunStaged         = runLifecycleInStagedProject
	lifecycleTransactionChildMatches      = lifecycleProjectChildMatches
	lifecycleTransactionExchange          = exchangeLifecycleProjectSpecs
	lifecycleTransactionSnapshot          = snapshotStagedSpecTreeNoFollow
	lifecycleTransactionNewID             = newLifecycleTransactionID
	lifecycleTransactionRandomRead        = cryptorand.Read
	lifecycleTransactionWriteReceipt      = writeLifecycleReceipt
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
	stage, err := lifecycleTransactionCreateStage(project, id)
	if err != nil {
		return LifecycleTransactionReceipt{}, exitcode.UnexpectedErrorf("creating staged lifecycle project: %v", err)
	}
	stagePath := stage.path
	defer func() { _ = closeStagedSpecTree(stage) }()
	stageSpec, err := lifecycleTransactionCreateStageSpec(stage, baseline)
	if err != nil {
		return LifecycleTransactionReceipt{}, exitcode.UnexpectedErrorf("materializing staged spec tree: %v", err)
	}
	defer func() { _ = closeStagedSpecTree(stageSpec) }()
	if err := lifecycleTransactionFreezeContext(project, stage); err != nil {
		return LifecycleTransactionReceipt{}, exitcode.UnexpectedErrorf("freezing staged project context: %v", err)
	}

	receipt := LifecycleTransactionReceipt{
		ID:             id,
		State:          "prepared",
		ProjectRoot:    projectRoot,
		RecoveryRoot:   stagePath,
		BaselineDigest: lifecycleSnapshotDigest(baseline),
		CreatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := lifecycleTransactionWriteReceipt(project, receipt); err != nil {
		return LifecycleTransactionReceipt{}, err
	}

	if err := lifecycleTransactionRunStaged(stage, op); err != nil {
		return receipt, err
	}
	staged, err := lifecycleTransactionSnapshot(stageSpec)
	if err != nil {
		return receipt, exitcode.UnexpectedErrorf("snapshotting staged lifecycle output: %v", err)
	}
	receipt.StagedDigest = lifecycleSnapshotDigest(staged)
	if err := lifecycleTransactionWriteReceipt(project, receipt); err != nil {
		return receipt, err
	}
	if err := lifecycleTransactionChildMatches(project, "spec", liveSpec); err != nil {
		return receipt, exitcode.UnexpectedErrorf("validating live spec identity before publication: %v", err)
	}
	if err := lifecycleTransactionChildMatches(stage, "spec", stageSpec); err != nil {
		return receipt, exitcode.UnexpectedErrorf("validating staged spec identity before publication: %v", err)
	}
	if err := lifecycleTransactionExchange(project, stage); err != nil {
		return receipt, exitcode.UnexpectedErrorf("atomically publishing staged lifecycle project: %v", err)
	}
	if err := lifecycleTransactionChildMatches(project, "spec", stageSpec); err != nil {
		return receipt, exitcode.UnexpectedErrorf("published spec identity is uncertain; retained recovery tree at %s: %v", stagePath, err)
	}
	if err := lifecycleTransactionChildMatches(stage, "spec", liveSpec); err != nil {
		return receipt, exitcode.UnexpectedErrorf("recovery predecessor identity is uncertain; retained recovery tree at %s: %v", stagePath, err)
	}
	receipt.State = "committed"
	if err := lifecycleTransactionWriteReceipt(project, receipt); err != nil {
		return receipt, err
	}
	return receipt, nil
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
	paths := snapshotPaths(snapshot)
	for _, path := range paths {
		_, _ = h.Write([]byte(path))
		if file, ok := snapshot.files[strings.TrimPrefix(path, "f:")]; strings.HasPrefix(path, "f:") && ok {
			_, _ = h.Write(file.content)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func snapshotPaths(snapshot specTreeSnapshot) []string {
	paths := make([]string, 0, len(snapshot.files)+len(snapshot.directories))
	for path := range snapshot.directories {
		paths = append(paths, "d:"+path)
	}
	for path := range snapshot.files {
		paths = append(paths, "f:"+path)
	}
	sort.Strings(paths)
	return paths
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
