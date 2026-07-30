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
	lifecycleRecoveryLstat        = os.Lstat
	lifecycleRecoverySnapshot     = snapshotSpecTreeNoFollow
	lifecycleRecoveryDiffSnapshot = snapshotSpecTreeNoFollow
)

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
			receipt, err := readLifecycleReceipt(root, args[0])
			if err != nil {
				return err
			}
			if receipt.State != "committed" {
				return exitcode.ConflictErrorf("transaction %s is %s; no exchanged predecessor is available to diff", receipt.ID, receipt.State)
			}
			live, err := lifecycleRecoveryDiffSnapshot(filepath.Join(root, "spec"))
			if err != nil {
				return exitcode.UnexpectedErrorf("snapshotting live spec tree: %v", err)
			}
			prior, err := lifecycleRecoveryDiffSnapshot(filepath.Join(receipt.RecoveryRoot, "spec"))
			if err != nil {
				return exitcode.UnexpectedErrorf("snapshotting retained predecessor: %v", err)
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
	dir := filepath.Join(projectRoot, ".specscore-recovery")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, exitcode.UnexpectedErrorf("reading lifecycle recovery journal: %v", err)
	}
	receipts := make([]LifecycleTransactionReceipt, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".publishing.json") {
			id := strings.TrimSuffix(entry.Name(), ".publishing.json")
			if !validLifecycleTransactionID(id) {
				return nil, exitcode.UnexpectedErrorf("invalid lifecycle publishing-intent filename %q", entry.Name())
			}
			info, err := lifecycleRecoveryLstat(filepath.Join(dir, entry.Name()))
			if err != nil {
				return nil, exitcode.UnexpectedErrorf("inspecting lifecycle publishing intent: %v", err)
			}
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return nil, exitcode.UnexpectedErrorf("refusing non-regular lifecycle publishing intent %q", entry.Name())
			}
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if !validLifecycleTransactionID(id) {
			return nil, exitcode.UnexpectedErrorf("invalid lifecycle recovery receipt filename %q", entry.Name())
		}
		info, err := lifecycleRecoveryLstat(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, exitcode.UnexpectedErrorf("inspecting lifecycle recovery receipt: %v", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, exitcode.UnexpectedErrorf("refusing non-regular lifecycle recovery receipt %q", entry.Name())
		}
		receipt, err := readLifecycleReceipt(projectRoot, id)
		if err != nil {
			return nil, err
		}
		receipts = append(receipts, receipt)
	}
	sort.Slice(receipts, func(i, j int) bool { return receipts[i].ID < receipts[j].ID })
	return receipts, nil
}

func readLifecycleReceipt(projectRoot, id string) (LifecycleTransactionReceipt, error) {
	if !validLifecycleTransactionID(id) {
		return LifecycleTransactionReceipt{}, exitcode.InvalidArgsErrorf("invalid lifecycle transaction id %q", id)
	}
	path := filepath.Join(projectRoot, ".specscore-recovery", id+".json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return LifecycleTransactionReceipt{}, exitcode.NotFoundErrorf("lifecycle transaction receipt not found: %s", id)
	}
	if err != nil {
		return LifecycleTransactionReceipt{}, exitcode.UnexpectedErrorf("reading lifecycle transaction receipt: %v", err)
	}
	var receipt LifecycleTransactionReceipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return LifecycleTransactionReceipt{}, exitcode.UnexpectedErrorf("parsing lifecycle transaction receipt: %v", err)
	}
	if receipt.ID != id {
		return LifecycleTransactionReceipt{}, exitcode.UnexpectedErrorf("lifecycle transaction receipt identity mismatch for %s", id)
	}
	if err := validateLifecycleReceipt(projectRoot, receipt); err != nil {
		return LifecycleTransactionReceipt{}, err
	}
	if receipt.State == "committed" {
		if err := validateLifecyclePublishingIntent(projectRoot, receipt); err != nil {
			return LifecycleTransactionReceipt{}, err
		}
	}
	return receipt, nil
}

func validateLifecyclePublishingIntent(projectRoot string, receipt LifecycleTransactionReceipt) error {
	path := filepath.Join(projectRoot, ".specscore-recovery", receipt.ID+".publishing.json")
	info, err := lifecycleRecoveryLstat(path)
	if os.IsNotExist(err) {
		// Receipts created before immutable publishing intent retention remain
		// readable; all new lifecycle transactions retain one before finality.
		return nil
	}
	if err != nil {
		return exitcode.UnexpectedErrorf("inspecting lifecycle publishing intent: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return exitcode.UnexpectedErrorf("refusing non-regular lifecycle publishing intent %q", receipt.ID)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return exitcode.UnexpectedErrorf("reading lifecycle publishing intent: %v", err)
	}
	var intent LifecycleTransactionReceipt
	if err := json.Unmarshal(data, &intent); err != nil {
		return exitcode.UnexpectedErrorf("parsing lifecycle publishing intent: %v", err)
	}
	if intent.State != "publishing" ||
		intent.ID != receipt.ID ||
		intent.ProjectRoot != receipt.ProjectRoot ||
		intent.RecoveryRoot != receipt.RecoveryRoot ||
		intent.BaselineDigest != receipt.BaselineDigest ||
		intent.StagedDigest != receipt.StagedDigest ||
		intent.CreatedAt != receipt.CreatedAt {
		return exitcode.UnexpectedErrorf("lifecycle publishing intent does not match committed receipt %s", receipt.ID)
	}
	if err := validateLifecycleReceipt(projectRoot, intent); err != nil {
		return exitcode.UnexpectedErrorf("validating lifecycle publishing intent: %v", err)
	}
	return nil
}

func validateLifecycleReceipt(projectRoot string, receipt LifecycleTransactionReceipt) error {
	if !validLifecycleTransactionID(receipt.ID) {
		return exitcode.UnexpectedErrorf("invalid lifecycle transaction receipt id %q", receipt.ID)
	}
	if receipt.State != "prepared" && receipt.State != "publishing" && receipt.State != "committed" {
		return exitcode.UnexpectedErrorf("invalid lifecycle transaction state %q", receipt.State)
	}
	root, err := lifecycleRecoveryAbs(projectRoot)
	if err != nil {
		return exitcode.UnexpectedErrorf("resolving recovery project root: %v", err)
	}
	if filepath.Clean(receipt.ProjectRoot) != root {
		return exitcode.UnexpectedErrorf("lifecycle receipt project-root mismatch")
	}
	recoveryRoot := filepath.Clean(receipt.RecoveryRoot)
	rel, err := filepath.Rel(root, recoveryRoot)
	if err != nil || filepath.Dir(rel) != "." || !strings.HasPrefix(filepath.Base(rel), ".specscore-txn-") {
		return exitcode.UnexpectedErrorf("lifecycle receipt recovery-root escapes its project")
	}
	if receipt.BaselineDigest == "" || ((receipt.State == "publishing" || receipt.State == "committed") && receipt.StagedDigest == "") {
		return exitcode.UnexpectedErrorf("lifecycle receipt is missing its integrity digest")
	}
	live, err := lifecycleRecoverySnapshot(filepath.Join(root, "spec"))
	if err != nil {
		return exitcode.UnexpectedErrorf("snapshotting live spec for lifecycle receipt validation: %v", err)
	}
	liveDigest := lifecycleSnapshotDigest(live)
	if receipt.State == "prepared" {
		if liveDigest != receipt.BaselineDigest {
			return exitcode.UnexpectedErrorf("prepared lifecycle receipt digest does not match the live spec tree")
		}
		return nil
	}
	prior, snapshotErr := lifecycleRecoverySnapshot(filepath.Join(recoveryRoot, "spec"))
	if snapshotErr != nil {
		return exitcode.UnexpectedErrorf("snapshotting retained predecessor for lifecycle receipt validation: %v", snapshotErr)
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
	if receipt.State == "committed" {
		if priorDigest != receipt.BaselineDigest {
			return exitcode.UnexpectedErrorf("lifecycle receipt digest does not match its retained predecessor")
		}
	}
	return nil
}
