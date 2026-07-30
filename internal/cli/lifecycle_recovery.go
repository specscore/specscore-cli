package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/spf13/cobra"
)

var (
	lifecycleRecoveryAbs          = filepath.Abs
	lifecycleRecoverySnapshot     = snapshotStagedSpecTreeNoFollow
	lifecycleRecoveryDiffSnapshot = snapshotStagedSpecTreeNoFollow
	lifecycleRecoveryValidate     = validateLifecycleReceiptSnapshots
)

type lifecycleRecoveryHandle struct {
	project *stagedSpecTree
	journal *stagedSpecTree
}

// lifecycleRecoveryCommand intentionally exposes inspection only. An exchanged
// predecessor may still receive a write through an old descriptor, so removal
// and restore require an explicit, separately designed authorization flow.
func lifecycleRecoveryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recovery",
		Short: "Inspect retained lifecycle transaction predecessors",
	}
	cmd.AddCommand(lifecycleRecoveryListCommand(), lifecycleRecoveryDiffCommand())
	return cmd
}

func lifecycleRecoveryListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List lifecycle transaction receipts (read-only)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := lifecycleRecoveryProjectRoot(cmd)
			if err != nil {
				return err
			}
			receipts, err := readLifecycleReceipts(root)
			if err != nil {
				return err
			}
			for _, receipt := range receipts {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", receipt.ID, receipt.State, receipt.RecoveryRoot)
			}
			return nil
		},
	}
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	return cmd
}

func lifecycleRecoveryDiffCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff <transaction-id>",
		Short: "Compare the live spec tree with a retained predecessor (read-only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := lifecycleRecoveryProjectRoot(cmd)
			if err != nil {
				return err
			}
			handle, err := openLifecycleRecoveryHandleNoFollow(root)
			if err != nil {
				return exitcode.UnexpectedErrorf("opening lifecycle recovery journal without following links: %v", err)
			}
			defer func() { _ = closeLifecycleRecoveryHandle(handle) }()
			receipt, live, prior, err := readAndValidateLifecycleReceiptWithHandle(root, handle, args[0], lifecycleRecoveryDiffSnapshot)
			if err != nil {
				return err
			}
			if receipt.State != "committed" {
				return exitcode.ConflictErrorf("transaction %s is %s; no exchanged predecessor is available to diff", receipt.ID, receipt.State)
			}
			paths := unionSnapshotPaths(
				changedSnapshotFiles(live, prior),
				changedSnapshotDirectories(live, prior),
			)
			if len(paths) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no differences")
				return nil
			}
			for _, path := range paths {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), path)
			}
			return nil
		},
	}
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	return cmd
}

func lifecycleRecoveryProjectRoot(cmd *cobra.Command) (string, error) {
	flag, _ := cmd.Flags().GetString("project")
	return resolveSpecRoot(flag)
}

func readLifecycleReceipts(projectRoot string) ([]LifecycleTransactionReceipt, error) {
	handle, err := openLifecycleRecoveryHandleNoFollow(projectRoot)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, exitcode.UnexpectedErrorf("opening lifecycle recovery journal without following links: %v", err)
	}
	defer func() { _ = closeLifecycleRecoveryHandle(handle) }()
	entries, err := readLifecycleRecoveryEntriesNoFollow(handle)
	if err != nil {
		return nil, exitcode.UnexpectedErrorf("reading lifecycle recovery journal: %v", err)
	}
	canonicalIDs := make(map[string]struct{}, len(entries))
	intentIDs := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".publishing.json") {
			id := strings.TrimSuffix(entry.Name(), ".publishing.json")
			if !validLifecycleTransactionID(id) {
				return nil, exitcode.UnexpectedErrorf("invalid lifecycle publishing-intent filename %q", entry.Name())
			}
			intentIDs[id] = struct{}{}
			continue
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if !validLifecycleTransactionID(id) {
			return nil, exitcode.UnexpectedErrorf("invalid lifecycle recovery receipt filename %q", entry.Name())
		}
		canonicalIDs[id] = struct{}{}
	}
	for id := range intentIDs {
		if _, ok := canonicalIDs[id]; !ok {
			return nil, exitcode.UnexpectedErrorf("orphaned lifecycle publishing intent for %s", id)
		}
	}
	ids := make([]string, 0, len(canonicalIDs))
	for id := range canonicalIDs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	receipts := make([]LifecycleTransactionReceipt, 0, len(ids))
	for _, id := range ids {
		receipt, err := readLifecycleReceiptWithHandle(projectRoot, handle, id)
		if err != nil {
			return nil, err
		}
		receipts = append(receipts, receipt)
	}
	return receipts, nil
}

func readLifecycleReceipt(projectRoot, id string) (LifecycleTransactionReceipt, error) {
	if !validLifecycleTransactionID(id) {
		return LifecycleTransactionReceipt{}, exitcode.InvalidArgsErrorf("invalid lifecycle transaction id %q", id)
	}
	handle, err := openLifecycleRecoveryHandleNoFollow(projectRoot)
	if os.IsNotExist(err) {
		return LifecycleTransactionReceipt{}, exitcode.NotFoundErrorf("lifecycle transaction receipt not found: %s", id)
	}
	if err != nil {
		return LifecycleTransactionReceipt{}, exitcode.UnexpectedErrorf("opening lifecycle recovery journal without following links: %v", err)
	}
	defer func() { _ = closeLifecycleRecoveryHandle(handle) }()
	return readLifecycleReceiptWithHandle(projectRoot, handle, id)
}

func readLifecycleReceiptWithHandle(projectRoot string, handle *lifecycleRecoveryHandle, id string) (LifecycleTransactionReceipt, error) {
	receipt, _, _, err := readAndValidateLifecycleReceiptWithHandle(projectRoot, handle, id, lifecycleRecoverySnapshot)
	return receipt, err
}

// readAndValidateLifecycleReceiptWithHandle reads the canonical receipt and
// publishing intent from the held journal first. It then captures one
// descriptor-rooted tree pair and validates both records against that same
// immutable snapshot, so callers such as recovery diff never reopen a
// different predecessor after validation.
func readAndValidateLifecycleReceiptWithHandle(
	projectRoot string,
	handle *lifecycleRecoveryHandle,
	id string,
	snapshot func(*stagedSpecTree) (specTreeSnapshot, error),
) (LifecycleTransactionReceipt, specTreeSnapshot, specTreeSnapshot, error) {
	if !validLifecycleTransactionID(id) {
		return LifecycleTransactionReceipt{}, specTreeSnapshot{}, specTreeSnapshot{}, exitcode.InvalidArgsErrorf("invalid lifecycle transaction id %q", id)
	}
	data, err := readLifecycleRecoveryRegularFileNoFollow(handle, id+".json")
	if os.IsNotExist(err) {
		return LifecycleTransactionReceipt{}, specTreeSnapshot{}, specTreeSnapshot{}, exitcode.NotFoundErrorf("lifecycle transaction receipt not found: %s", id)
	}
	if err != nil {
		return LifecycleTransactionReceipt{}, specTreeSnapshot{}, specTreeSnapshot{}, exitcode.UnexpectedErrorf("reading lifecycle transaction receipt: %v", err)
	}
	var receipt LifecycleTransactionReceipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return LifecycleTransactionReceipt{}, specTreeSnapshot{}, specTreeSnapshot{}, exitcode.UnexpectedErrorf("parsing lifecycle transaction receipt: %v", err)
	}
	if receipt.ID != id {
		return LifecycleTransactionReceipt{}, specTreeSnapshot{}, specTreeSnapshot{}, exitcode.UnexpectedErrorf("lifecycle transaction receipt identity mismatch for %s", id)
	}
	intent, err := readLifecyclePublishingIntentIdentityWithHandle(handle, receipt)
	if err != nil {
		return LifecycleTransactionReceipt{}, specTreeSnapshot{}, specTreeSnapshot{}, err
	}
	live, prior, err := lifecycleRecoveryReceiptSnapshots(projectRoot, handle.project, receipt, snapshot)
	if err != nil {
		return LifecycleTransactionReceipt{}, specTreeSnapshot{}, specTreeSnapshot{}, exitcode.UnexpectedErrorf("snapshotting descriptor-rooted lifecycle receipt trees: %v", err)
	}
	if err := lifecycleRecoveryValidate(receipt, live, prior); err != nil {
		return LifecycleTransactionReceipt{}, specTreeSnapshot{}, specTreeSnapshot{}, err
	}
	if intent != nil {
		if err := lifecycleRecoveryValidate(*intent, live, prior); err != nil {
			return LifecycleTransactionReceipt{}, specTreeSnapshot{}, specTreeSnapshot{}, exitcode.UnexpectedErrorf("validating lifecycle publishing intent: %v", err)
		}
	}
	return receipt, live, prior, nil
}

func validateLifecyclePublishingIntent(projectRoot string, receipt LifecycleTransactionReceipt) error {
	handle, err := openLifecycleRecoveryHandleNoFollow(projectRoot)
	if os.IsNotExist(err) {
		if receipt.State == "committed" && receipt.PublishingIntentRequired {
			return exitcode.UnexpectedErrorf("marked committed lifecycle receipt %s is missing its publishing intent", receipt.ID)
		}
		return nil
	}
	if err != nil {
		return exitcode.UnexpectedErrorf("opening lifecycle recovery journal without following links: %v", err)
	}
	defer func() { _ = closeLifecycleRecoveryHandle(handle) }()
	return validateLifecyclePublishingIntentWithHandle(projectRoot, handle, receipt)
}

func validateLifecyclePublishingIntentWithHandle(projectRoot string, handle *lifecycleRecoveryHandle, receipt LifecycleTransactionReceipt) error {
	intent, err := readLifecyclePublishingIntentIdentityWithHandle(handle, receipt)
	if err != nil {
		return err
	}
	if intent == nil {
		return nil
	}
	if err := validateLifecycleReceiptWithProject(projectRoot, handle.project, *intent); err != nil {
		return exitcode.UnexpectedErrorf("validating lifecycle publishing intent: %v", err)
	}
	return nil
}

// readLifecyclePublishingIntentIdentityWithHandle validates the sidecar's
// filename-derived identity and its immutable linkage to receipt before any
// project, stage, or spec descriptor is traversed.
func readLifecyclePublishingIntentIdentityWithHandle(handle *lifecycleRecoveryHandle, receipt LifecycleTransactionReceipt) (*LifecycleTransactionReceipt, error) {
	data, err := readLifecycleRecoveryRegularFileNoFollow(handle, receipt.ID+".publishing.json")
	if os.IsNotExist(err) {
		if receipt.State == "committed" && receipt.PublishingIntentRequired {
			return nil, exitcode.UnexpectedErrorf("marked committed lifecycle receipt %s is missing its publishing intent", receipt.ID)
		}
		return nil, nil
	}
	if err != nil {
		return nil, exitcode.UnexpectedErrorf("reading lifecycle publishing intent: %v", err)
	}
	var intent LifecycleTransactionReceipt
	if err := json.Unmarshal(data, &intent); err != nil {
		return nil, exitcode.UnexpectedErrorf("parsing lifecycle publishing intent: %v", err)
	}
	if (receipt.State != "publishing" && receipt.State != "committed") ||
		intent.State != "publishing" ||
		intent.ID != receipt.ID ||
		intent.ProjectRoot != receipt.ProjectRoot ||
		intent.RecoveryRoot != receipt.RecoveryRoot ||
		intent.RecoveryRootIdentity != receipt.RecoveryRootIdentity ||
		intent.BaselineDigest != receipt.BaselineDigest ||
		intent.StagedDigest != receipt.StagedDigest ||
		intent.CreatedAt != receipt.CreatedAt ||
		intent.PublishingIntentRequired != receipt.PublishingIntentRequired {
		return nil, exitcode.UnexpectedErrorf("lifecycle publishing intent does not match receipt %s", receipt.ID)
	}
	return &intent, nil
}

func validateLifecycleReceipt(projectRoot string, receipt LifecycleTransactionReceipt) error {
	root, _, err := lifecycleRecoveryReceiptStageName(projectRoot, receipt)
	if err != nil {
		return err
	}
	project, err := openLifecycleProjectNoFollow(root)
	if err != nil {
		return exitcode.UnexpectedErrorf("opening lifecycle project without following links: %v", err)
	}
	defer func() { _ = closeStagedSpecTree(project) }()
	return validateLifecycleReceiptWithProject(root, project, receipt)
}

func validateLifecycleReceiptWithProject(projectRoot string, project *stagedSpecTree, receipt LifecycleTransactionReceipt) error {
	_, _, err := lifecycleRecoveryReceiptStageName(projectRoot, receipt)
	if err != nil {
		return err
	}
	live, prior, err := lifecycleRecoveryReceiptSnapshots(projectRoot, project, receipt, lifecycleRecoverySnapshot)
	if err != nil {
		return exitcode.UnexpectedErrorf("snapshotting descriptor-rooted lifecycle receipt trees: %v", err)
	}
	return validateLifecycleReceiptSnapshots(receipt, live, prior)
}

// validateLifecycleReceiptSnapshots verifies a receipt against one already
// captured live/predecessor pair. Keeping this separate from descriptor
// traversal lets a receipt and its publishing intent share exactly the same
// evidence and prevents diff from describing a later replacement.
func validateLifecycleReceiptSnapshots(receipt LifecycleTransactionReceipt, live, prior specTreeSnapshot) error {
	liveDigest := lifecycleSnapshotDigest(live)
	if receipt.State == "prepared" {
		if liveDigest != receipt.BaselineDigest {
			return exitcode.UnexpectedErrorf("prepared lifecycle receipt digest does not match the live spec tree")
		}
		return nil
	}
	priorDigest := lifecycleSnapshotDigest(prior)
	if receipt.State == "publishing" {
		if (liveDigest == receipt.BaselineDigest && priorDigest == receipt.StagedDigest) ||
			(liveDigest == receipt.StagedDigest && priorDigest == receipt.BaselineDigest) {
			return nil
		}
		return exitcode.UnexpectedErrorf("publishing lifecycle receipt digest does not match either atomic-exchange state")
	}
	if liveDigest != receipt.StagedDigest {
		return exitcode.UnexpectedErrorf("lifecycle receipt digest does not match the live spec tree")
	}
	if priorDigest != receipt.BaselineDigest {
		return exitcode.UnexpectedErrorf("lifecycle receipt digest does not match its retained predecessor")
	}
	return nil
}

func lifecycleRecoveryReceiptStageName(projectRoot string, receipt LifecycleTransactionReceipt) (string, string, error) {
	if !validLifecycleTransactionID(receipt.ID) {
		return "", "", exitcode.UnexpectedErrorf("invalid lifecycle transaction receipt id %q", receipt.ID)
	}
	if receipt.State != "prepared" && receipt.State != "publishing" && receipt.State != "committed" {
		return "", "", exitcode.UnexpectedErrorf("invalid lifecycle transaction state %q", receipt.State)
	}
	root, err := lifecycleRecoveryAbs(projectRoot)
	if err != nil {
		return "", "", exitcode.UnexpectedErrorf("resolving recovery project root: %v", err)
	}
	if filepath.Clean(receipt.ProjectRoot) != root {
		return "", "", exitcode.UnexpectedErrorf("lifecycle receipt project-root mismatch")
	}
	recoveryRoot := filepath.Clean(receipt.RecoveryRoot)
	rel, err := filepath.Rel(root, recoveryRoot)
	stageName := ".specscore-txn-" + receipt.ID
	if err != nil || rel != stageName {
		return "", "", exitcode.UnexpectedErrorf("lifecycle receipt recovery-root does not match its transaction")
	}
	if receipt.BaselineDigest == "" ||
		((receipt.State == "publishing" || receipt.State == "committed") && receipt.StagedDigest == "") ||
		(receipt.PublishingIntentRequired && (receipt.State == "publishing" || receipt.State == "committed") && receipt.RecoveryRootIdentity == "") {
		return "", "", exitcode.UnexpectedErrorf("lifecycle receipt is missing its integrity digest")
	}
	return root, stageName, nil
}

// lifecycleRecoveryReceiptSnapshots traverses project → stage → spec entirely
// through held no-follow descriptors. RecoveryRoot is used only to derive the
// validated direct child name; it is never reopened as a pathname.
func lifecycleRecoveryReceiptSnapshots(
	projectRoot string,
	project *stagedSpecTree,
	receipt LifecycleTransactionReceipt,
	snapshot func(*stagedSpecTree) (specTreeSnapshot, error),
) (specTreeSnapshot, specTreeSnapshot, error) {
	_, stageName, err := lifecycleRecoveryReceiptStageName(projectRoot, receipt)
	if err != nil {
		return specTreeSnapshot{}, specTreeSnapshot{}, err
	}
	if project == nil || project.root == nil {
		return specTreeSnapshot{}, specTreeSnapshot{}, fmt.Errorf("lifecycle recovery project descriptor is closed")
	}
	liveSpec, err := openLifecycleProjectChildNoFollow(project, "spec")
	if err != nil {
		return specTreeSnapshot{}, specTreeSnapshot{}, fmt.Errorf("opening live spec descriptor: %w", err)
	}
	defer func() { _ = closeStagedSpecTree(liveSpec) }()
	live, err := snapshot(liveSpec)
	if err != nil {
		return specTreeSnapshot{}, specTreeSnapshot{}, fmt.Errorf("snapshotting live spec descriptor: %w", err)
	}
	if receipt.State == "prepared" {
		return live, specTreeSnapshot{}, nil
	}
	stage, err := openLifecycleProjectChildNoFollow(project, stageName)
	if err != nil {
		return specTreeSnapshot{}, specTreeSnapshot{}, fmt.Errorf("opening retained predecessor descriptor: %w", err)
	}
	defer func() { _ = closeStagedSpecTree(stage) }()
	if receipt.RecoveryRootIdentity != "" {
		identity, err := lifecycleStageIdentity(stage)
		if err != nil {
			return specTreeSnapshot{}, specTreeSnapshot{}, fmt.Errorf("identifying retained predecessor descriptor: %w", err)
		}
		if identity != receipt.RecoveryRootIdentity {
			return specTreeSnapshot{}, specTreeSnapshot{}, fmt.Errorf("retained predecessor identity does not match receipt")
		}
	}
	priorSpec, err := openLifecycleProjectChildNoFollow(stage, "spec")
	if err != nil {
		return specTreeSnapshot{}, specTreeSnapshot{}, fmt.Errorf("opening retained predecessor spec descriptor: %w", err)
	}
	defer func() { _ = closeStagedSpecTree(priorSpec) }()
	prior, err := snapshot(priorSpec)
	if err != nil {
		return specTreeSnapshot{}, specTreeSnapshot{}, fmt.Errorf("snapshotting retained predecessor spec descriptor: %w", err)
	}
	return live, prior, nil
}
