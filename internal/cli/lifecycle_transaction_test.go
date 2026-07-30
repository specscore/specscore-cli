//go:build darwin || linux

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestRunLifecycleTransactionStagesPublishesAndRetainsPredecessor(t *testing.T) {
	project := t.TempDir()
	mustWriteLifecycleFile(t, filepath.Join(project, "spec", "README.md"), "# Spec\n")
	mustWriteLifecycleFile(t, filepath.Join(project, "specscore.yaml"), "schema: 1\n")
	mustWriteLifecycleFile(t, filepath.Join(project, ".github", "workflows", "ci.yml"), "name: ci\n")
	mustWriteLifecycleFile(t, filepath.Join(project, ".git", "config"), "[remote \"origin\"]\nurl = https://example.test/org/repo.git\n")

	receipt, err := RunLifecycleTransaction(project, func(stageRoot string) error {
		if stageRoot != "." {
			t.Fatalf("stage root = %q, want descriptor-anchored dot", stageRoot)
		}
		if _, err := os.ReadFile("specscore.yaml"); err != nil {
			return err
		}
		if _, err := os.ReadFile(filepath.Join(".github", "workflows", "ci.yml")); err != nil {
			return err
		}
		if _, err := os.ReadFile(filepath.Join(".git", "config")); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join("spec", "new.md"), []byte("new\n"), 0o600)
	})
	if err != nil {
		t.Fatalf("RunLifecycleTransaction: %v", err)
	}
	if receipt.State != "committed" || receipt.ID == "" || receipt.StagedDigest == "" {
		t.Fatalf("receipt = %#v", receipt)
	}
	intentData, err := os.ReadFile(filepath.Join(project, ".specscore-recovery", receipt.ID+".publishing.json"))
	if err != nil {
		t.Fatalf("retained publishing intent: %v", err)
	}
	var intent LifecycleTransactionReceipt
	if err := json.Unmarshal(intentData, &intent); err != nil {
		t.Fatalf("parse retained publishing intent: %v", err)
	}
	if intent.State != "publishing" || intent.ID != receipt.ID || intent.StagedDigest != receipt.StagedDigest {
		t.Fatalf("retained publishing intent = %#v", intent)
	}
	if _, err := os.Stat(filepath.Join(project, "spec", "new.md")); err != nil {
		t.Fatalf("published staged file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(receipt.RecoveryRoot, "spec", "new.md")); !os.IsNotExist(err) {
		t.Fatalf("predecessor unexpectedly has staged file: %v", err)
	}
	receipts, err := readLifecycleReceipts(project)
	if err != nil || len(receipts) != 1 || receipts[0].ID != receipt.ID {
		t.Fatalf("readLifecycleReceipts = %#v, %v", receipts, err)
	}

	list := lifecycleRecoveryCommand()
	var listOut bytes.Buffer
	list.SetOut(&listOut)
	list.SetArgs([]string{"list", "--project", project})
	if err := list.Execute(); err != nil {
		t.Fatalf("recovery list: %v", err)
	}
	if !strings.Contains(listOut.String(), receipt.ID+"\tcommitted") {
		t.Fatalf("recovery list output = %q", listOut.String())
	}
	diff := lifecycleRecoveryCommand()
	var diffOut bytes.Buffer
	diff.SetOut(&diffOut)
	diff.SetArgs([]string{"diff", receipt.ID, "--project", project})
	if err := diff.Execute(); err != nil {
		t.Fatalf("recovery diff: %v", err)
	}
	if !strings.Contains(diffOut.String(), "new.md") {
		t.Fatalf("recovery diff output = %q", diffOut.String())
	}
}

func TestRunLifecycleTransactionFailureRetainsPreparedReceipt(t *testing.T) {
	project := t.TempDir()
	mustWriteLifecycleFile(t, filepath.Join(project, "spec", "README.md"), "# Spec\n")
	receipt, err := RunLifecycleTransaction(project, func(string) error {
		return errors.New("staged operation failed")
	})
	if err == nil || !strings.Contains(err.Error(), "staged operation failed") {
		t.Fatalf("RunLifecycleTransaction error = %v", err)
	}
	if receipt.State != "prepared" || receipt.ID == "" {
		t.Fatalf("prepared receipt = %#v", receipt)
	}
	if _, err := os.Stat(filepath.Join(project, "spec", "README.md")); err != nil {
		t.Fatalf("live spec changed after staged failure: %v", err)
	}
	if _, err := readLifecycleReceipt(project, receipt.ID); err != nil {
		t.Fatalf("prepared receipt was not retained: %v", err)
	}
}

func TestRunLifecycleTransactionRejectsStagedManifestTampering(t *testing.T) {
	project := t.TempDir()
	mustWriteLifecycleFile(t, filepath.Join(project, "spec", "README.md"), "baseline\n")
	originalBeforePublication := lifecycleTransactionBeforePublication
	lifecycleTransactionBeforePublication = func(stageSpec *stagedSpecTree) error {
		return os.WriteFile(filepath.Join(stageSpec.path, "injected.md"), []byte("injected\n"), 0o600)
	}
	t.Cleanup(func() { lifecycleTransactionBeforePublication = originalBeforePublication })
	receipt, err := RunLifecycleTransaction(project, func(string) error {
		return os.WriteFile(filepath.Join("spec", "expected.md"), []byte("expected\n"), 0o600)
	})
	if err == nil || !strings.Contains(err.Error(), "staged lifecycle output changed") {
		t.Fatalf("staged manifest tamper error = %v", err)
	}
	if receipt.State != "publishing" {
		t.Fatalf("tamper receipt state = %q", receipt.State)
	}
	for _, name := range []string{"expected.md", "injected.md"} {
		if _, statErr := os.Stat(filepath.Join(project, "spec", name)); !os.IsNotExist(statErr) {
			t.Fatalf("tampered staged entry was published (%s): %v", name, statErr)
		}
	}
	if _, readErr := readLifecycleReceipt(project, receipt.ID); readErr == nil {
		t.Fatal("tampered publishing receipt was accepted as trustworthy")
	}
}

func TestRunLifecycleTransactionDetectsPostExchangeManifestTampering(t *testing.T) {
	project := t.TempDir()
	mustWriteLifecycleFile(t, filepath.Join(project, "spec", "README.md"), "baseline\n")
	originalAfterExchange := lifecycleTransactionAfterExchange
	lifecycleTransactionAfterExchange = func(liveSpec, _ *stagedSpecTree) error {
		return os.WriteFile(filepath.Join(liveSpec.path, "injected.md"), []byte("injected\n"), 0o600)
	}
	t.Cleanup(func() { lifecycleTransactionAfterExchange = originalAfterExchange })
	receipt, err := RunLifecycleTransaction(project, func(string) error {
		return os.WriteFile(filepath.Join("spec", "expected.md"), []byte("expected\n"), 0o600)
	})
	if err == nil || !strings.Contains(err.Error(), "published lifecycle output changed") {
		t.Fatalf("post-exchange manifest tamper error = %v", err)
	}
	if receipt.State != "publishing" {
		t.Fatalf("post-exchange receipt state = %q", receipt.State)
	}
	if _, statErr := os.Stat(filepath.Join(project, "spec", "injected.md")); statErr != nil {
		t.Fatalf("post-exchange tamper fixture missing: %v", statErr)
	}
	if _, readErr := readLifecycleReceipt(project, receipt.ID); readErr == nil {
		t.Fatal("post-exchange tampered receipt was accepted as trustworthy")
	}
}

func TestRunLifecycleTransactionRetainsPublishingReceiptWhenPublicationSyncFails(t *testing.T) {
	for _, failure := range []struct {
		name string
		call int
	}{
		{name: "live parent", call: 1},
		{name: "recovery parent", call: 2},
	} {
		t.Run(failure.name, func(t *testing.T) {
			resetLifecyclePublicationSeams(t)
			project := t.TempDir()
			mustWriteLifecycleFile(t, filepath.Join(project, "spec", "README.md"), "baseline\n")
			calls := 0
			lifecyclePublicationFsync = func(fd int) error {
				calls++
				if calls == failure.call {
					return errors.New("publication parent sync interrupted")
				}
				return unix.Fsync(fd)
			}
			receipt, err := RunLifecycleTransaction(project, func(string) error {
				return os.WriteFile(filepath.Join("spec", "published.md"), []byte("published\n"), 0o600)
			})
			if err == nil || !strings.Contains(err.Error(), "publication parent") {
				t.Fatalf("publication sync error = %v", err)
			}
			if receipt.State != "publishing" {
				t.Fatalf("receipt state after interrupted publication sync = %q", receipt.State)
			}
			stored, readErr := readLifecycleReceipt(project, receipt.ID)
			if readErr != nil {
				t.Fatalf("publishing receipt was not retained for recovery inspection: %v", readErr)
			}
			if stored.State != "publishing" {
				t.Fatalf("stored receipt state = %q, want publishing", stored.State)
			}
		})
	}
}

func TestRunLifecycleTransactionReportsOutcomeUncertainOnFinalReceiptDurabilityFailure(t *testing.T) {
	for _, failure := range []struct {
		name    string
		arrange func()
	}{
		{name: "final namespace", arrange: func() {
			calls := 0
			lifecycleReceiptRenameAt = func(oldDirFD int, oldName string, newDirFD int, newName string) error {
				calls++
				if calls == 3 {
					return errors.New("final receipt namespace interrupted")
				}
				return unix.Renameat(oldDirFD, oldName, newDirFD, newName)
			}
		}},
		{name: "final temporary receipt fsync", arrange: func() {
			failLifecycleReceiptFsyncAt(9, "final temporary receipt fsync interrupted")
		}},
		{name: "final journal fsync", arrange: func() {
			failLifecycleReceiptFsyncAt(10, "final journal fsync interrupted")
		}},
		{name: "final journal parent fsync", arrange: func() {
			failLifecycleReceiptFsyncAt(11, "final journal parent fsync interrupted")
		}},
	} {
		t.Run(failure.name, func(t *testing.T) {
			resetLifecycleReceiptSeams(t)
			project := t.TempDir()
			mustWriteLifecycleFile(t, filepath.Join(project, "spec", "README.md"), "baseline\n")
			failure.arrange()
			receipt, err := RunLifecycleTransaction(project, func(string) error {
				return os.WriteFile(filepath.Join("spec", "published.md"), []byte("published\n"), 0o600)
			})
			if err == nil || !strings.Contains(err.Error(), "outcome uncertain") ||
				!strings.Contains(err.Error(), receipt.ID) ||
				!strings.Contains(err.Error(), "specscore recovery list --project "+project) {
				t.Fatalf("outcome-uncertain error = %v", err)
			}
			if receipt.State != "outcome-uncertain" {
				t.Fatalf("terminal durability failure state = %q", receipt.State)
			}
			if _, statErr := os.Stat(filepath.Join(project, ".specscore-recovery", receipt.ID+".publishing.json")); statErr != nil {
				t.Fatalf("durable publishing intent missing: %v", statErr)
			}
			physical, readErr := readLifecycleReceipt(project, receipt.ID)
			if readErr != nil {
				t.Fatalf("recovery failed to validate physical receipt and exchange layout: %v", readErr)
			}
			if physical.State != "publishing" && physical.State != "committed" {
				t.Fatalf("physical recovery state = %q", physical.State)
			}
		})
	}
}

func TestExchangeLifecycleProjectSpecsSyncFailureBranches(t *testing.T) {
	projectRoot := t.TempDir()
	project, err := openLifecycleProjectNoFollow(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeStagedSpecTree(project) })
	stage, err := createLifecycleStageProjectNoFollow(project, "publication-sync")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeStagedSpecTree(stage) })

	for _, failure := range []struct {
		name    string
		arrange func()
	}{
		{name: "exchange", arrange: func() {
			lifecyclePublicationExchange = func(int, int) error {
				return errors.New("exchange interrupted")
			}
		}},
		{name: "live parent sync", arrange: func() {
			lifecyclePublicationExchange = func(int, int) error { return nil }
			lifecyclePublicationFsync = func(int) error {
				return errors.New("live parent sync interrupted")
			}
		}},
		{name: "recovery parent sync", arrange: func() {
			lifecyclePublicationExchange = func(int, int) error { return nil }
			calls := 0
			lifecyclePublicationFsync = func(int) error {
				calls++
				if calls == 2 {
					return errors.New("recovery parent sync interrupted")
				}
				return nil
			}
		}},
	} {
		t.Run(failure.name, func(t *testing.T) {
			resetLifecyclePublicationSeams(t)
			failure.arrange()
			if err := exchangeLifecycleProjectSpecs(project, stage); err == nil {
				t.Fatalf("%s failure accepted", failure.name)
			}
		})
	}

	resetLifecyclePublicationSeams(t)
	lifecyclePublicationExchange = func(int, int) error { return nil }
	var synced []int
	lifecyclePublicationFsync = func(fd int) error {
		synced = append(synced, fd)
		return nil
	}
	if err := exchangeLifecycleProjectSpecs(project, stage); err != nil {
		t.Fatalf("syncing both publication parents: %v", err)
	}
	if len(synced) != 2 || synced[0] != int(project.root.Fd()) || synced[1] != int(stage.root.Fd()) {
		t.Fatalf("publication parent sync fds = %v", synced)
	}
}

func TestLifecycleRecoveryAndContextRejectUnsafeInputs(t *testing.T) {
	project := t.TempDir()
	if _, err := readLifecycleReceipt(project, "../escape"); err == nil {
		t.Fatal("unsafe receipt id accepted")
	}
	if _, err := readLifecycleReceipt(project, "missing"); err == nil {
		t.Fatal("missing receipt accepted")
	}
	mustWriteLifecycleFile(t, filepath.Join(project, ".specscore-recovery", "broken.json"), "not json")
	if _, err := readLifecycleReceipts(project); err == nil {
		t.Fatal("broken receipt accepted")
	}
	mustWriteLifecycleFile(t, filepath.Join(project, "spec", "README.md"), "# Spec\n")
	if err := os.Symlink("missing", filepath.Join(project, ".github")); err != nil {
		t.Fatal(err)
	}
	if _, err := RunLifecycleTransaction(project, func(string) error { return nil }); err == nil {
		t.Fatal("symlinked project context was accepted")
	}
}

func TestLifecycleDescriptorHelpersRejectUnsafeStates(t *testing.T) {
	projectRoot := t.TempDir()
	project, err := openLifecycleProjectNoFollow(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeStagedSpecTree(project) })
	if _, err := openLifecycleProjectChildNoFollow(&stagedSpecTree{}, "spec"); err == nil {
		t.Fatal("closed project accepted")
	}
	if _, err := createLifecycleStageProjectNoFollow(&stagedSpecTree{}, "closed"); err == nil {
		t.Fatal("closed project stage creation accepted")
	}
	stage, err := createLifecycleStageProjectNoFollow(project, "helper-stage")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeStagedSpecTree(stage) })
	if _, err := createLifecycleStageProjectNoFollow(project, "helper-stage"); err == nil {
		t.Fatal("duplicate stage accepted")
	}
	if _, err := openLifecycleProjectChildNoFollow(project, "missing"); err == nil {
		t.Fatal("missing child accepted")
	}
	snapshot := rootSnapshot(map[string]string{"README.md": "context\n"})
	ctx, err := createLifecycleStageDirectoryNoFollow(stage, "context", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := closeStagedSpecTree(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := createLifecycleStageDirectoryNoFollow(stage, "context", snapshot); err == nil {
		t.Fatal("duplicate context directory accepted")
	}
	other, err := createLifecycleStageDirectoryNoFollow(stage, "other", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closeStagedSpecTree(other) }()
	if err := lifecycleProjectChildMatches(stage, "context", other); err == nil {
		t.Fatal("different descriptor identity accepted")
	}
	if err := lifecycleProjectChildMatches(&stagedSpecTree{}, "context", other); err == nil {
		t.Fatal("closed project identity check accepted")
	}
	if err := copyOptionalLifecycleRegularFile(stage, "missing", stage, "missing-copy"); err != nil {
		t.Fatalf("missing optional file: %v", err)
	}
	if err := unix.Mkdirat(int(stage.root.Fd()), "directory", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := copyOptionalLifecycleRegularFile(stage, "directory", stage, "directory-copy"); err == nil {
		t.Fatal("directory copied as regular context file")
	}
	if err := unix.Symlinkat("missing", int(stage.root.Fd()), "link"); err != nil {
		t.Fatal(err)
	}
	if err := copyOptionalLifecycleRegularFile(stage, "link", stage, "link-copy"); err == nil {
		t.Fatal("symlink copied as context file")
	}
	fd, err := unix.Openat(int(stage.root.Fd()), "regular", unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unix.Write(fd, []byte("regular\n")); err != nil {
		_ = unix.Close(fd)
		t.Fatal(err)
	}
	if err := unix.Close(fd); err != nil {
		t.Fatal(err)
	}
	if err := copyOptionalLifecycleRegularFile(stage, "regular", stage, "regular-copy"); err != nil {
		t.Fatal(err)
	}
	if err := copyOptionalLifecycleRegularFile(stage, "regular", stage, "regular-copy"); err == nil {
		t.Fatal("existing destination was overwritten")
	}
	if err := copyOptionalLifecycleDirectory(stage, "missing-dir", stage, "missing-dir-copy"); err != nil {
		t.Fatalf("missing optional directory: %v", err)
	}
}

func TestLifecycleReceiptValidationRejectsForgedRootsAndJournalEntries(t *testing.T) {
	project := t.TempDir()
	recoveryRoot := filepath.Join(project, ".specscore-txn-receipt-1")
	mustWriteLifecycleFile(t, filepath.Join(project, "spec", "README.md"), "staged\n")
	mustWriteLifecycleFile(t, filepath.Join(recoveryRoot, "spec", "README.md"), "baseline\n")
	receipt := LifecycleTransactionReceipt{
		ID:             "receipt-1",
		State:          "committed",
		ProjectRoot:    project,
		RecoveryRoot:   recoveryRoot,
		BaselineDigest: lifecycleDigestAt(t, filepath.Join(recoveryRoot, "spec")),
		StagedDigest:   lifecycleDigestAt(t, filepath.Join(project, "spec")),
	}
	if err := validateLifecycleReceipt(project, receipt); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "spec", "README.md"), []byte("tampered\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateLifecycleReceipt(project, receipt); err == nil {
		t.Fatal("tampered live spec accepted by receipt validation")
	}
	mustWriteLifecycleFile(t, filepath.Join(project, "spec", "README.md"), "staged\n")
	receipt.StagedDigest = lifecycleDigestAt(t, filepath.Join(project, "spec"))
	if err := os.WriteFile(filepath.Join(recoveryRoot, "spec", "README.md"), []byte("tampered\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateLifecycleReceipt(project, receipt); err == nil {
		t.Fatal("tampered predecessor accepted by receipt validation")
	}
	mustWriteLifecycleFile(t, filepath.Join(recoveryRoot, "spec", "README.md"), "baseline\n")
	publishing := receipt
	publishing.State = "publishing"
	mustWriteLifecycleFile(t, filepath.Join(project, "spec", "README.md"), "baseline\n")
	mustWriteLifecycleFile(t, filepath.Join(recoveryRoot, "spec", "README.md"), "staged\n")
	publishing.BaselineDigest = lifecycleDigestAt(t, filepath.Join(project, "spec"))
	publishing.StagedDigest = lifecycleDigestAt(t, filepath.Join(recoveryRoot, "spec"))
	if err := validateLifecycleReceipt(project, publishing); err != nil {
		t.Fatalf("pre-exchange publishing receipt: %v", err)
	}
	mustWriteLifecycleFile(t, filepath.Join(project, "spec", "README.md"), "staged\n")
	mustWriteLifecycleFile(t, filepath.Join(recoveryRoot, "spec", "README.md"), "baseline\n")
	receipt.BaselineDigest = lifecycleDigestAt(t, filepath.Join(recoveryRoot, "spec"))
	receipt.StagedDigest = lifecycleDigestAt(t, filepath.Join(project, "spec"))
	forged := receipt
	forged.RecoveryRoot = t.TempDir()
	if err := validateLifecycleReceipt(project, forged); err == nil {
		t.Fatal("forged recovery root accepted")
	}
	forged = receipt
	forged.ProjectRoot = t.TempDir()
	if err := validateLifecycleReceipt(project, forged); err == nil {
		t.Fatal("forged project root accepted")
	}
	forged = receipt
	forged.State = "deleted"
	if err := validateLifecycleReceipt(project, forged); err == nil {
		t.Fatal("invalid state accepted")
	}
	forged = receipt
	forged.StagedDigest = ""
	if err := validateLifecycleReceipt(project, forged); err == nil {
		t.Fatal("committed receipt without digest accepted")
	}
	forged = receipt
	forged.ID = "../forged"
	if err := validateLifecycleReceipt(project, forged); err == nil {
		t.Fatal("receipt with unsafe identity accepted")
	}
	prepared := receipt
	prepared.State = "prepared"
	prepared.BaselineDigest = lifecycleDigestAt(t, filepath.Join(project, "spec"))
	prepared.StagedDigest = ""
	if err := validateLifecycleReceipt(project, prepared); err != nil {
		t.Fatalf("prepared receipt without staged digest: %v", err)
	}
	prepared.BaselineDigest = "wrong"
	if err := validateLifecycleReceipt(project, prepared); err == nil {
		t.Fatal("prepared receipt with mismatched live digest accepted")
	}
	prepared.BaselineDigest = lifecycleDigestAt(t, filepath.Join(project, "spec"))
	prepared.BaselineDigest = ""
	if err := validateLifecycleReceipt(project, prepared); err == nil {
		t.Fatal("receipt without baseline digest accepted")
	}
	mustWriteLifecycleFile(t, filepath.Join(project, ".specscore-recovery", "receipt-1.json"), mustMarshalReceipt(t, receipt))
	if _, err := readLifecycleReceipt(project, receipt.ID); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("receipt-1.json", filepath.Join(project, ".specscore-recovery", "receipt-2.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := readLifecycleReceipts(project); err == nil {
		t.Fatal("symlinked journal entry accepted")
	}
}

func TestLifecycleTransactionPrimitiveFailuresAreSafe(t *testing.T) {
	if _, err := RunLifecycleTransaction(".", nil); err == nil {
		t.Fatal("nil lifecycle operation accepted")
	}
	project := t.TempDir()
	mustWriteLifecycleFile(t, filepath.Join(project, "spec", "README.md"), "# Spec\n")

	oldRandomRead := lifecycleTransactionRandomRead
	lifecycleTransactionRandomRead = func([]byte) (int, error) { return 0, errors.New("entropy unavailable") }
	t.Cleanup(func() { lifecycleTransactionRandomRead = oldRandomRead })
	if _, err := newLifecycleTransactionID(); err == nil || !strings.Contains(err.Error(), "entropy unavailable") {
		t.Fatalf("newLifecycleTransactionID error = %v", err)
	}
	lifecycleTransactionRandomRead = oldRandomRead
	if id, err := newLifecycleTransactionID(); err != nil || !validLifecycleTransactionID(id) {
		t.Fatalf("newLifecycleTransactionID = %q, %v", id, err)
	}

	oldMarshal := lifecycleReceiptMarshal
	lifecycleReceiptMarshal = func(any, string, string) ([]byte, error) { return nil, errors.New("receipt marshal failed") }
	t.Cleanup(func() { lifecycleReceiptMarshal = oldMarshal })
	opened, err := openLifecycleProjectNoFollow(project)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeStagedSpecTree(opened) })
	receipt := LifecycleTransactionReceipt{ID: "receipt-1"}
	if err := writeLifecycleReceipt(opened, receipt); err == nil || !strings.Contains(err.Error(), "receipt marshal failed") {
		t.Fatalf("writeLifecycleReceipt marshal error = %v", err)
	}
	lifecycleReceiptMarshal = oldMarshal
	for _, id := range []string{"", "has.dot", "has/slash", "has_space"} {
		if validLifecycleTransactionID(id) {
			t.Fatalf("unsafe lifecycle id accepted: %q", id)
		}
	}
	if !validLifecycleTransactionID("ABC-123") {
		t.Fatal("safe lifecycle id rejected")
	}
	if err := writeLifecycleReceipt(opened, receipt); err != nil {
		t.Fatalf("writeLifecycleReceipt: %v", err)
	}
}

func TestLifecycleDescriptorContextAndReceiptWriterDefences(t *testing.T) {
	projectRoot := t.TempDir()
	mustWriteLifecycleFile(t, filepath.Join(projectRoot, "specscore.yaml"), "schema: 1\n")
	mustWriteLifecycleFile(t, filepath.Join(projectRoot, ".github", "workflows", "ci.yml"), "name: ci\n")
	mustWriteLifecycleFile(t, filepath.Join(projectRoot, ".git", "config"), "[core]\n")
	project, err := openLifecycleProjectNoFollow(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeStagedSpecTree(project) })
	stage, err := createLifecycleStageProjectNoFollow(project, "context-stage")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeStagedSpecTree(stage) })
	if err := materializeLifecycleProjectContext(project, stage); err != nil {
		t.Fatalf("materializeLifecycleProjectContext: %v", err)
	}
	for _, name := range []string{"specscore.yaml", filepath.Join(".github", "workflows", "ci.yml"), filepath.Join(".git", "config")} {
		if _, err := os.Stat(filepath.Join(stage.path, name)); err != nil {
			t.Fatalf("frozen context %s missing: %v", name, err)
		}
	}
	if err := copyOptionalLifecycleDirectory(&stagedSpecTree{}, "missing", stage, "target"); err == nil {
		t.Fatal("closed source accepted for context directory copy")
	}
	if err := copyOptionalLifecycleRegularFile(stage, "specscore.yaml", &stagedSpecTree{}, "target"); err == nil {
		t.Fatal("closed destination accepted for context file copy")
	}
	if err := lifecycleProjectChildMatches(stage, ".git", nil); err == nil {
		t.Fatal("closed expected descriptor accepted")
	}
	if err := writeLifecycleReceiptNoFollow(stage, "../escape.json", []byte("bad")); err == nil {
		t.Fatal("unsafe receipt name accepted")
	}
	if err := writeLifecycleReceiptNoFollow(stage, "receipt-1", []byte("bad")); err == nil {
		t.Fatal("receipt without json suffix accepted")
	}
	if err := writeLifecycleReceiptNoFollow(&stagedSpecTree{}, "receipt-1.json", []byte("bad")); err == nil {
		t.Fatal("closed project accepted for receipt write")
	}
	if err := writeLifecycleReceiptNoFollow(stage, "receipt-1.json", []byte("first\n")); err != nil {
		t.Fatalf("creating receipt: %v", err)
	}
	if err := writeLifecycleReceiptNoFollow(stage, "receipt-1.json", []byte("second\n")); err != nil {
		t.Fatalf("updating receipt: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(stage.path, ".specscore-recovery", "receipt-1.json"))
	if err != nil || string(data) != "second\n" {
		t.Fatalf("receipt content = %q, %v", data, err)
	}
	if _, err := openOrCreateLifecycleJournalDirectory(-1); err == nil {
		t.Fatal("invalid journal parent descriptor accepted")
	}
}

func TestLifecycleProjectContextRejectsNonDirectoryGit(t *testing.T) {
	projectRoot := t.TempDir()
	mustWriteLifecycleFile(t, filepath.Join(projectRoot, ".git"), "not a directory\n")
	project, err := openLifecycleProjectNoFollow(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeStagedSpecTree(project) })
	stage, err := createLifecycleStageProjectNoFollow(project, "git-file-stage")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeStagedSpecTree(stage) })
	if err := materializeLifecycleProjectContext(project, stage); err == nil {
		t.Fatal("regular .git file accepted as frozen context")
	}
}

func TestLifecycleRecoveryReadOnlyInspectionFailureModes(t *testing.T) {
	project := t.TempDir()
	mustWriteLifecycleFile(t, filepath.Join(project, "spec", "README.md"), "# Spec\n")
	mustWriteLifecycleFile(t, filepath.Join(project, "specscore.yaml"), "schema: 1\n")
	if receipts, err := readLifecycleReceipts(project); err != nil || receipts != nil {
		t.Fatalf("empty recovery journal = %#v, %v", receipts, err)
	}
	list := lifecycleRecoveryCommand()
	var listOut bytes.Buffer
	list.SetOut(&listOut)
	list.SetArgs([]string{"list", "--project", project})
	if err := list.Execute(); err != nil || listOut.String() != "" {
		t.Fatalf("empty recovery list = %q, %v", listOut.String(), err)
	}

	fileJournalProject := t.TempDir()
	mustWriteLifecycleFile(t, filepath.Join(fileJournalProject, "spec", "README.md"), "# Spec\n")
	mustWriteLifecycleFile(t, filepath.Join(fileJournalProject, "specscore.yaml"), "schema: 1\n")
	mustWriteLifecycleFile(t, filepath.Join(fileJournalProject, ".specscore-recovery"), "not a directory\n")
	if _, err := readLifecycleReceipts(fileJournalProject); err == nil {
		t.Fatal("file recovery journal accepted")
	}
	list = lifecycleRecoveryCommand()
	list.SetArgs([]string{"list", "--project", fileJournalProject})
	if err := list.Execute(); err == nil {
		t.Fatal("recovery list accepted file journal")
	}

	journal := filepath.Join(project, ".specscore-recovery")
	if err := os.MkdirAll(filepath.Join(journal, "ignored-dir"), 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteLifecycleFile(t, filepath.Join(journal, "ignored.txt"), "ignored\n")
	mustWriteLifecycleFile(t, filepath.Join(journal, "bad.name.json"), "{}")
	if _, err := readLifecycleReceipts(project); err == nil {
		t.Fatal("unsafe recovery receipt filename accepted")
	}
	if err := os.Remove(filepath.Join(journal, "bad.name.json")); err != nil {
		t.Fatal(err)
	}
	mustWriteLifecycleFile(t, filepath.Join(journal, "bad.name.publishing.json"), "{}")
	if _, err := readLifecycleReceipts(project); err == nil {
		t.Fatal("unsafe publishing-intent filename accepted")
	}
	if err := os.Remove(filepath.Join(journal, "bad.name.publishing.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("ignored.txt", filepath.Join(journal, "receipt-1.publishing.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := readLifecycleReceipts(project); err == nil {
		t.Fatal("symlinked publishing intent accepted")
	}
	if err := os.Remove(filepath.Join(journal, "receipt-1.publishing.json")); err != nil {
		t.Fatal(err)
	}
	mustWriteLifecycleFile(t, filepath.Join(journal, "receipt-1.publishing.json"), "{}")
	if _, err := readLifecycleReceipts(project); err == nil {
		t.Fatal("orphaned publishing intent accepted")
	}
	if err := os.Remove(filepath.Join(journal, "receipt-1.publishing.json")); err != nil {
		t.Fatal(err)
	}
	mustWriteLifecycleFile(t, filepath.Join(journal, "dir-entry.json", "nested"), "x")
	if _, err := readLifecycleReceipts(project); err != nil {
		t.Fatalf("directory receipt should be skipped: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(journal, "dir-entry.json")); err != nil {
		t.Fatal(err)
	}
	mustWriteLifecycleFile(t, filepath.Join(journal, "receipt-1.json"), "{}")
	if _, err := readLifecycleReceipt(project, "receipt-1"); err == nil {
		t.Fatal("receipt missing identity accepted")
	}
	mustWriteLifecycleFile(t, filepath.Join(journal, "receipt-1.json"), mustMarshalReceipt(t, LifecycleTransactionReceipt{ID: "other", State: "prepared", ProjectRoot: project, RecoveryRoot: filepath.Join(project, ".specscore-txn-other"), BaselineDigest: "baseline"}))
	if _, err := readLifecycleReceipt(project, "receipt-1"); err == nil {
		t.Fatal("receipt with mismatched identity accepted")
	}
	if err := os.Remove(filepath.Join(journal, "receipt-1.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(journal, "receipt-1.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := readLifecycleReceipt(project, "receipt-1"); err == nil {
		t.Fatal("directory receipt accepted as readable JSON")
	}
}

func TestLifecycleRecoveryDiffFailureModes(t *testing.T) {
	project := t.TempDir()
	mustWriteLifecycleFile(t, filepath.Join(project, "spec", "README.md"), "# Spec\n")
	mustWriteLifecycleFile(t, filepath.Join(project, "specscore.yaml"), "schema: 1\n")
	journal := filepath.Join(project, ".specscore-recovery")
	write := func(receipt LifecycleTransactionReceipt) {
		t.Helper()
		mustWriteLifecycleFile(t, filepath.Join(journal, receipt.ID+".json"), mustMarshalReceipt(t, receipt))
	}
	runDiff := func(id string) error {
		t.Helper()
		command := lifecycleRecoveryCommand()
		command.SetArgs([]string{"diff", id, "--project", project})
		return command.Execute()
	}
	if err := runDiff("missing"); err == nil {
		t.Fatal("missing receipt accepted for diff")
	}
	prepared := LifecycleTransactionReceipt{ID: "prepared", State: "prepared", ProjectRoot: project, RecoveryRoot: filepath.Join(project, ".specscore-txn-prepared"), BaselineDigest: lifecycleDigestAt(t, filepath.Join(project, "spec"))}
	write(prepared)
	if err := runDiff(prepared.ID); err == nil {
		t.Fatal("prepared receipt accepted for predecessor diff")
	}
	missingPrior := LifecycleTransactionReceipt{ID: "missing-prior", State: "committed", ProjectRoot: project, RecoveryRoot: filepath.Join(project, ".specscore-txn-missing-prior"), BaselineDigest: "baseline", StagedDigest: lifecycleDigestAt(t, filepath.Join(project, "spec"))}
	write(missingPrior)
	if err := runDiff(missingPrior.ID); err == nil {
		t.Fatal("missing predecessor spec accepted for diff")
	}
	noDiffRoot := filepath.Join(project, ".specscore-txn-no-diff")
	mustWriteLifecycleFile(t, filepath.Join(noDiffRoot, "spec", "README.md"), "# Spec\n")
	liveFile, err := os.Stat(filepath.Join(project, "spec", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(noDiffRoot, "spec", "README.md"), liveFile.ModTime(), liveFile.ModTime()); err != nil {
		t.Fatal(err)
	}
	noDiff := LifecycleTransactionReceipt{ID: "no-diff", State: "committed", ProjectRoot: project, RecoveryRoot: noDiffRoot, BaselineDigest: lifecycleDigestAt(t, filepath.Join(noDiffRoot, "spec")), StagedDigest: lifecycleDigestAt(t, filepath.Join(project, "spec"))}
	write(noDiff)
	command := lifecycleRecoveryCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"diff", noDiff.ID, "--project", project})
	if err := command.Execute(); err != nil || output.String() != "no differences\n" {
		t.Fatalf("no-diff recovery output = %q, %v", output.String(), err)
	}
	originalDiffSnapshot := lifecycleRecoveryDiffSnapshot
	lifecycleRecoveryDiffSnapshot = func(string) (specTreeSnapshot, error) {
		return specTreeSnapshot{}, errors.New("live diff snapshot failed")
	}
	if err := runDiff(noDiff.ID); err == nil {
		t.Fatal("live diff snapshot failure accepted")
	}
	lifecycleRecoveryDiffSnapshot = originalDiffSnapshot
	diffSnapshots := 0
	lifecycleRecoveryDiffSnapshot = func(path string) (specTreeSnapshot, error) {
		diffSnapshots++
		if diffSnapshots == 2 {
			return specTreeSnapshot{}, errors.New("prior diff snapshot failed")
		}
		return originalDiffSnapshot(path)
	}
	if err := runDiff(noDiff.ID); err == nil {
		t.Fatal("prior diff snapshot failure accepted")
	}
	lifecycleRecoveryDiffSnapshot = originalDiffSnapshot
	if err := os.RemoveAll(filepath.Join(project, "spec")); err != nil {
		t.Fatal(err)
	}
	if err := runDiff(noDiff.ID); err == nil {
		t.Fatal("missing live spec accepted for diff")
	}
}

func TestLifecycleRecoveryRemainingValidationBranches(t *testing.T) {
	project := t.TempDir()
	mustWriteLifecycleFile(t, filepath.Join(project, "spec", "README.md"), "# Spec\n")
	mustWriteLifecycleFile(t, filepath.Join(project, "specscore.yaml"), "schema: 1\n")
	for _, verb := range [][]string{{"list", "--project", t.TempDir()}, {"diff", "receipt-1", "--project", t.TempDir()}} {
		command := lifecycleRecoveryCommand()
		command.SetArgs(verb)
		if err := command.Execute(); err == nil {
			t.Fatalf("recovery %v accepted a non-project root", verb)
		}
	}
	journal := filepath.Join(project, ".specscore-recovery")
	writeReceipt := func(id string) {
		t.Helper()
		recoveryRoot := filepath.Join(project, ".specscore-txn-"+id)
		mustWriteLifecycleFile(t, filepath.Join(recoveryRoot, "spec", "README.md"), "# Spec\n")
		receipt := LifecycleTransactionReceipt{ID: id, State: "committed", ProjectRoot: project, RecoveryRoot: recoveryRoot, BaselineDigest: lifecycleDigestAt(t, filepath.Join(recoveryRoot, "spec")), StagedDigest: lifecycleDigestAt(t, filepath.Join(project, "spec"))}
		mustWriteLifecycleFile(t, filepath.Join(journal, id+".json"), mustMarshalReceipt(t, receipt))
	}
	writeReceipt("receipt-2")
	writeReceipt("receipt-1")
	receipts, err := readLifecycleReceipts(project)
	if err != nil || len(receipts) != 2 || receipts[0].ID != "receipt-1" || receipts[1].ID != "receipt-2" {
		t.Fatalf("sorted receipts = %#v, %v", receipts, err)
	}
	mustWriteLifecycleFile(t, filepath.Join(journal, "invalid.json"), mustMarshalReceipt(t, LifecycleTransactionReceipt{ID: "invalid", State: "bad", ProjectRoot: project, RecoveryRoot: filepath.Join(project, ".specscore-txn-invalid"), BaselineDigest: "baseline"}))
	if _, err := readLifecycleReceipt(project, "invalid"); err == nil {
		t.Fatal("invalid receipt state accepted")
	}
	resetLifecycleRecoveryReadSeams(t)
	lifecycleRecoveryOpenProject = func(string) (*stagedSpecTree, error) {
		return nil, errors.New("receipt project open failed")
	}
	if _, err := readLifecycleReceipts(project); err == nil {
		t.Fatal("receipt descriptor open failure accepted")
	}
	resetLifecycleRecoveryReadSeams(t)
	lifecycleRecoveryReadDir = func(*os.File, int) ([]os.DirEntry, error) {
		return nil, errors.New("recovery journal read failed")
	}
	if _, err := readLifecycleReceipts(project); err == nil {
		t.Fatal("recovery journal descriptor read failure accepted")
	}
	originalAbs := lifecycleRecoveryAbs
	lifecycleRecoveryAbs = func(string) (string, error) { return "", errors.New("receipt root abs failed") }
	t.Cleanup(func() { lifecycleRecoveryAbs = originalAbs })
	if err := validateLifecycleReceipt(project, LifecycleTransactionReceipt{ID: "receipt-1", State: "prepared", ProjectRoot: project, RecoveryRoot: filepath.Join(project, ".specscore-txn-receipt-1"), BaselineDigest: "baseline"}); err == nil {
		t.Fatal("receipt root resolution failure accepted")
	}
}

func TestLifecyclePublishingIntentValidationBranches(t *testing.T) {
	project := t.TempDir()
	recoveryRoot := filepath.Join(project, ".specscore-txn-intent-1")
	mustWriteLifecycleFile(t, filepath.Join(project, "spec", "README.md"), "staged\n")
	mustWriteLifecycleFile(t, filepath.Join(recoveryRoot, "spec", "README.md"), "baseline\n")
	receipt := LifecycleTransactionReceipt{
		ID:                       "intent-1",
		State:                    "committed",
		ProjectRoot:              project,
		RecoveryRoot:             recoveryRoot,
		BaselineDigest:           lifecycleDigestAt(t, filepath.Join(recoveryRoot, "spec")),
		StagedDigest:             lifecycleDigestAt(t, filepath.Join(project, "spec")),
		CreatedAt:                "2026-07-30T00:00:00Z",
		PublishingIntentRequired: true,
	}
	intent := receipt
	intent.State = "publishing"
	journal := filepath.Join(project, ".specscore-recovery")
	intentPath := filepath.Join(journal, receipt.ID+".publishing.json")
	writeIntent := func(value LifecycleTransactionReceipt) {
		t.Helper()
		mustWriteLifecycleFile(t, intentPath, mustMarshalReceipt(t, value))
	}
	if err := validateLifecyclePublishingIntent(project, receipt); err == nil {
		t.Fatal("marked committed receipt without retained intent accepted")
	}
	legacy := receipt
	legacy.PublishingIntentRequired = false
	if err := validateLifecyclePublishingIntent(project, legacy); err != nil {
		t.Fatalf("legacy receipt without retained intent: %v", err)
	}
	writeIntent(intent)
	if err := validateLifecyclePublishingIntent(project, receipt); err != nil {
		t.Fatalf("valid retained publishing intent: %v", err)
	}
	if _, err := readLifecycleReceipt(project, receipt.ID); err == nil {
		t.Fatal("committed receipt file missing despite valid intent accepted")
	}
	mustWriteLifecycleFile(t, filepath.Join(journal, receipt.ID+".json"), mustMarshalReceipt(t, receipt))
	if _, err := readLifecycleReceipt(project, receipt.ID); err != nil {
		t.Fatalf("committed receipt with valid retained intent: %v", err)
	}
	if err := os.Remove(intentPath); err != nil {
		t.Fatal(err)
	}
	if _, err := readLifecycleReceipt(project, receipt.ID); err == nil {
		t.Fatal("marked committed receipt with deleted retained intent accepted")
	}
	writeIntent(intent)
	if err := os.Remove(intentPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(intentPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := validateLifecyclePublishingIntent(project, receipt); err == nil {
		t.Fatal("directory publishing intent accepted")
	}
	if err := os.Remove(intentPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", intentPath); err != nil {
		t.Fatal(err)
	}
	if err := validateLifecyclePublishingIntent(project, receipt); err == nil {
		t.Fatal("symlinked publishing intent accepted")
	}
	if err := os.Remove(intentPath); err != nil {
		t.Fatal(err)
	}
	mustWriteLifecycleFile(t, intentPath, "not json")
	if err := validateLifecyclePublishingIntent(project, receipt); err == nil {
		t.Fatal("invalid publishing intent JSON accepted")
	}
	mismatch := intent
	mismatch.CreatedAt = "2026-07-30T00:00:01Z"
	writeIntent(mismatch)
	if _, err := readLifecycleReceipt(project, receipt.ID); err == nil {
		t.Fatal("mismatched publishing intent accepted")
	}
	writeIntent(intent)
	originalSnapshot := lifecycleRecoverySnapshot
	lifecycleRecoverySnapshot = func(string) (specTreeSnapshot, error) {
		return specTreeSnapshot{}, errors.New("publishing intent snapshot failed")
	}
	if err := validateLifecyclePublishingIntent(project, receipt); err == nil {
		t.Fatal("publishing intent snapshot failure accepted")
	}
	lifecycleRecoverySnapshot = originalSnapshot
	resetLifecycleRecoveryReadSeams(t)
	lifecycleRecoveryOpenProject = func(string) (*stagedSpecTree, error) {
		return nil, errors.New("publishing intent project open failed")
	}
	if err := validateLifecyclePublishingIntent(project, receipt); err == nil {
		t.Fatal("publishing intent descriptor open failure accepted")
	}
	resetLifecycleRecoveryReadSeams(t)
	lifecycleRecoveryReadAll = func(io.Reader) ([]byte, error) {
		return nil, errors.New("publishing intent read failed")
	}
	if err := validateLifecyclePublishingIntent(project, receipt); err == nil {
		t.Fatal("publishing intent read failure accepted")
	}
}

func TestLifecycleRecoveryDescriptorReadFailureAndSwapBranches(t *testing.T) {
	project := t.TempDir()
	journal := filepath.Join(project, ".specscore-recovery")
	name := "receipt-1.json"
	path := filepath.Join(journal, name)
	write := func(content string) {
		t.Helper()
		mustWriteLifecycleFile(t, path, content)
	}
	write("original\n")
	if _, err := readLifecycleRecoveryRegularFileNoFollow(project, "../escape.json"); err == nil {
		t.Fatal("unsafe recovery filename accepted")
	}
	if _, err := readLifecycleRecoveryRegularFileNoFollow(t.TempDir(), name); err == nil {
		t.Fatal("missing recovery journal accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("other.json", path); err != nil {
		t.Fatal(err)
	}
	if _, err := readLifecycleRecoveryRegularFileNoFollow(project, name); err == nil {
		t.Fatal("symlinked recovery receipt accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := readLifecycleRecoveryRegularFileNoFollow(project, name); err == nil {
		t.Fatal("directory recovery receipt accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	write("original\n")

	resetLifecycleRecoveryReadSeams(t)
	lifecycleRecoveryOpenProject = func(string) (*stagedSpecTree, error) {
		return nil, errors.New("project descriptor open failed")
	}
	if _, err := readLifecycleRecoveryRegularFileNoFollow(project, name); err == nil {
		t.Fatal("project descriptor open failure accepted")
	}
	resetLifecycleRecoveryReadSeams(t)
	lifecycleRecoveryOpenChild = func(*stagedSpecTree, string) (*stagedSpecTree, error) {
		return nil, errors.New("journal descriptor open failed")
	}
	if _, err := readLifecycleRecoveryRegularFileNoFollow(project, name); err == nil {
		t.Fatal("journal descriptor open failure accepted")
	}
	resetLifecycleRecoveryReadSeams(t)
	lifecycleRecoveryOpenFileAt = func(int, string, int, uint32) (int, error) {
		return -1, errors.New("receipt descriptor open failed")
	}
	if _, err := readLifecycleRecoveryRegularFileNoFollow(project, name); err == nil {
		t.Fatal("receipt descriptor open failure accepted")
	}
	resetLifecycleRecoveryReadSeams(t)
	lifecycleRecoveryFileStat = func(*os.File) (os.FileInfo, error) {
		return nil, errors.New("pre-read stat failed")
	}
	if _, err := readLifecycleRecoveryRegularFileNoFollow(project, name); err == nil {
		t.Fatal("pre-read stat failure accepted")
	}
	resetLifecycleRecoveryReadSeams(t)
	lifecycleRecoveryBeforeRead = func(*os.File) error {
		return errors.New("pre-read hook failed")
	}
	if _, err := readLifecycleRecoveryRegularFileNoFollow(project, name); err == nil {
		t.Fatal("pre-read hook failure accepted")
	}
	resetLifecycleRecoveryReadSeams(t)
	lifecycleRecoveryReadAll = func(io.Reader) ([]byte, error) {
		return nil, errors.New("receipt read failed")
	}
	if _, err := readLifecycleRecoveryRegularFileNoFollow(project, name); err == nil {
		t.Fatal("receipt read failure accepted")
	}
	resetLifecycleRecoveryReadSeams(t)
	statCalls := 0
	lifecycleRecoveryFileStat = func(file *os.File) (os.FileInfo, error) {
		statCalls++
		if statCalls == 2 {
			return nil, errors.New("post-read stat failed")
		}
		return file.Stat()
	}
	if _, err := readLifecycleRecoveryRegularFileNoFollow(project, name); err == nil {
		t.Fatal("post-read stat failure accepted")
	}
	resetLifecycleRecoveryReadSeams(t)
	lifecycleRecoveryBeforeRead = func(*os.File) error {
		return os.WriteFile(path, []byte("changed\n"), 0o600)
	}
	if _, err := readLifecycleRecoveryRegularFileNoFollow(project, name); err == nil {
		t.Fatal("receipt changed while reading accepted")
	}
	write("original\n")
	resetLifecycleRecoveryReadSeams(t)
	lifecycleRecoveryBeforeRead = func(*os.File) error {
		if err := os.Remove(path); err != nil {
			return err
		}
		return os.Symlink("other.json", path)
	}
	data, err := readLifecycleRecoveryRegularFileNoFollow(project, name)
	if err != nil || string(data) != "original\n" {
		t.Fatalf("descriptor-held receipt after pathname swap = %q, %v", data, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	write("not-json")
	if _, err := readLifecycleReceipt(project, "receipt-1"); err == nil {
		t.Fatal("invalid current receipt accepted through descriptor reader")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("other.json", path); err != nil {
		t.Fatal(err)
	}
	if _, err := readLifecycleReceipt(project, "receipt-1"); err == nil {
		t.Fatal("symlinked current receipt accepted through descriptor reader")
	}
}

func TestLifecycleDigestAndReceiptWriterErrorBranches(t *testing.T) {
	withoutFiles := rootSnapshot(nil)
	withFiles := rootSnapshot(map[string]string{"README.md": "content\n"})
	changedContent := rootSnapshot(map[string]string{"README.md": "changed\n"})
	if lifecycleSnapshotDigest(withoutFiles) == lifecycleSnapshotDigest(withFiles) || lifecycleSnapshotDigest(withFiles) == lifecycleSnapshotDigest(changedContent) {
		t.Fatal("snapshot digest ignored file content")
	}
	ambiguousLeft := rootSnapshot(map[string]string{"a": "bc"})
	ambiguousRight := rootSnapshot(map[string]string{"ab": "c"})
	if lifecycleSnapshotDigest(ambiguousLeft) == lifecycleSnapshotDigest(ambiguousRight) {
		t.Fatal("snapshot digest accepted ambiguous path/content framing")
	}
	modeChanged := rootSnapshot(map[string]string{"README.md": "content\n"})
	modeChanged.files["README.md"] = specTreeFile{content: []byte("content\n"), mode: 0o600}
	if lifecycleSnapshotDigest(withFiles) == lifecycleSnapshotDigest(modeChanged) {
		t.Fatal("snapshot digest ignored file mode")
	}
	xattrChanged := rootSnapshot(map[string]string{"README.md": "content\n"})
	xattrChanged.files["README.md"] = specTreeFile{content: []byte("content\n"), mode: 0o644, metadata: specTreeEntryMetadata{extendedAttributes: map[string][]byte{"user.test": []byte("x")}}}
	if lifecycleSnapshotDigest(withFiles) == lifecycleSnapshotDigest(xattrChanged) {
		t.Fatal("snapshot digest ignored extended attributes")
	}
	if err := writeLifecycleReceipt(&stagedSpecTree{}, LifecycleTransactionReceipt{ID: "bad.id"}); err == nil {
		t.Fatal("unsafe receipt id accepted for write")
	}
	if err := writeLifecycleReceipt(&stagedSpecTree{}, LifecycleTransactionReceipt{ID: "receipt-1"}); err == nil {
		t.Fatal("closed receipt project accepted for write")
	}
	originalSnapshot := lifecycleTransactionSnapshot
	lifecycleTransactionSnapshot = func(*stagedSpecTree) (specTreeSnapshot, error) {
		return specTreeSnapshot{}, errors.New("manifest snapshot failed")
	}
	if err := verifyLifecycleSnapshot(&stagedSpecTree{}, rootSnapshot(nil)); err == nil {
		t.Fatal("manifest snapshot failure accepted")
	}
	lifecycleTransactionSnapshot = originalSnapshot
	originalRandomRead := lifecycleTransactionRandomRead
	lifecycleTransactionRandomRead = func([]byte) (int, error) { return 0, errors.New("receipt temp entropy failed") }
	if _, err := newLifecycleReceiptTempName("receipt-1.json"); err == nil {
		t.Fatal("receipt temporary-name entropy failure accepted")
	}
	lifecycleTransactionRandomRead = originalRandomRead
}

func TestRunLifecycleTransactionFailsClosedAtEveryBoundary(t *testing.T) {
	project := t.TempDir()
	mustWriteLifecycleFile(t, filepath.Join(project, "spec", "README.md"), "# Spec\n")
	originalPlatform := lifecycleTransactionPlatformSupported
	originalAbs := lifecycleTransactionAbs
	originalAcquireLock := lifecycleTransactionAcquireLock
	originalOpenProject := lifecycleTransactionOpenProject
	originalOpenChild := lifecycleTransactionOpenChild
	originalCreateStage := lifecycleTransactionCreateStage
	originalCreateStageSpec := lifecycleTransactionCreateStageSpec
	originalFreeze := lifecycleTransactionFreezeContext
	originalRun := lifecycleTransactionRunStaged
	originalMatches := lifecycleTransactionChildMatches
	originalExchange := lifecycleTransactionExchange
	originalSnapshot := lifecycleTransactionSnapshot
	originalVerify := lifecycleTransactionVerifySnapshot
	originalBeforePublication := lifecycleTransactionBeforePublication
	originalAfterExchange := lifecycleTransactionAfterExchange
	originalNewID := lifecycleTransactionNewID
	originalWrite := lifecycleTransactionWriteReceipt
	originalRetainIntent := lifecycleTransactionRetainIntent
	reset := func() {
		lifecycleTransactionPlatformSupported = originalPlatform
		lifecycleTransactionAbs = originalAbs
		lifecycleTransactionAcquireLock = originalAcquireLock
		lifecycleTransactionOpenProject = originalOpenProject
		lifecycleTransactionOpenChild = originalOpenChild
		lifecycleTransactionCreateStage = originalCreateStage
		lifecycleTransactionCreateStageSpec = originalCreateStageSpec
		lifecycleTransactionFreezeContext = originalFreeze
		lifecycleTransactionRunStaged = originalRun
		lifecycleTransactionChildMatches = originalMatches
		lifecycleTransactionExchange = originalExchange
		lifecycleTransactionSnapshot = originalSnapshot
		lifecycleTransactionVerifySnapshot = originalVerify
		lifecycleTransactionBeforePublication = originalBeforePublication
		lifecycleTransactionAfterExchange = originalAfterExchange
		lifecycleTransactionNewID = originalNewID
		lifecycleTransactionWriteReceipt = originalWrite
		lifecycleTransactionRetainIntent = originalRetainIntent
	}
	t.Cleanup(reset)
	fails := func(name string, arrange func()) {
		t.Helper()
		reset()
		arrange()
		if _, err := RunLifecycleTransaction(project, func(string) error { return nil }); err == nil {
			t.Fatalf("%s: transaction unexpectedly succeeded", name)
		}
	}
	fails("unsupported platform", func() {
		lifecycleTransactionPlatformSupported = func() bool { return false }
	})
	fails("project root resolution", func() {
		lifecycleTransactionAbs = func(string) (string, error) { return "", errors.New("abs failed") }
	})
	fails("lock acquisition", func() {
		lifecycleTransactionAcquireLock = func(string) (string, *os.File, error) { return "", nil, errors.New("lock failed") }
	})
	fails("project open", func() {
		lifecycleTransactionOpenProject = func(string) (*stagedSpecTree, error) { return nil, errors.New("project open failed") }
	})
	fails("spec open", func() {
		lifecycleTransactionOpenChild = func(*stagedSpecTree, string) (*stagedSpecTree, error) { return nil, errors.New("spec open failed") }
	})
	fails("baseline snapshot", func() {
		lifecycleTransactionSnapshot = func(*stagedSpecTree) (specTreeSnapshot, error) {
			return specTreeSnapshot{}, errors.New("baseline snapshot failed")
		}
	})
	fails("transaction id", func() {
		lifecycleTransactionNewID = func() (string, error) { return "", errors.New("id failed") }
	})
	fails("stage project", func() {
		lifecycleTransactionCreateStage = func(*stagedSpecTree, string) (*stagedSpecTree, error) { return nil, errors.New("stage failed") }
	})
	fails("stage spec", func() {
		lifecycleTransactionCreateStageSpec = func(*stagedSpecTree, specTreeSnapshot) (*stagedSpecTree, error) {
			return nil, errors.New("stage spec failed")
		}
	})
	fails("context freeze", func() {
		lifecycleTransactionFreezeContext = func(*stagedSpecTree, *stagedSpecTree) error { return errors.New("freeze failed") }
	})
	fails("prepared receipt", func() {
		lifecycleTransactionWriteReceipt = func(*stagedSpecTree, LifecycleTransactionReceipt) error { return errors.New("receipt failed") }
	})
	fails("staged operation", func() {
		lifecycleTransactionRunStaged = func(*stagedSpecTree, func(string) error) error { return errors.New("operation failed") }
	})
	fails("staged snapshot", func() {
		calls := 0
		lifecycleTransactionSnapshot = func(tree *stagedSpecTree) (specTreeSnapshot, error) {
			calls++
			if calls == 2 {
				return specTreeSnapshot{}, errors.New("staged snapshot failed")
			}
			return originalSnapshot(tree)
		}
	})
	fails("staged receipt", func() {
		calls := 0
		lifecycleTransactionWriteReceipt = func(project *stagedSpecTree, receipt LifecycleTransactionReceipt) error {
			calls++
			if calls == 2 {
				return errors.New("staged receipt failed")
			}
			return originalWrite(project, receipt)
		}
	})
	fails("before publication hook", func() {
		lifecycleTransactionBeforePublication = func(*stagedSpecTree) error { return errors.New("before publication failed") }
	})
	fails("live identity", func() {
		lifecycleTransactionChildMatches = func(*stagedSpecTree, string, *stagedSpecTree) error { return errors.New("live identity failed") }
	})
	fails("live manifest", func() {
		lifecycleTransactionVerifySnapshot = func(*stagedSpecTree, specTreeSnapshot) error { return errors.New("live manifest failed") }
	})
	fails("staged manifest", func() {
		calls := 0
		lifecycleTransactionVerifySnapshot = func(*stagedSpecTree, specTreeSnapshot) error {
			calls++
			if calls == 2 {
				return errors.New("staged manifest failed")
			}
			return nil
		}
	})
	fails("staged identity", func() {
		calls := 0
		lifecycleTransactionChildMatches = func(project *stagedSpecTree, name string, child *stagedSpecTree) error {
			calls++
			if calls == 2 {
				return errors.New("staged identity failed")
			}
			return nil
		}
	})
	fails("atomic exchange", func() {
		lifecycleTransactionExchange = func(*stagedSpecTree, *stagedSpecTree) error { return errors.New("exchange failed") }
	})
	fails("after exchange hook", func() {
		lifecycleTransactionAfterExchange = func(*stagedSpecTree, *stagedSpecTree) error { return errors.New("after exchange failed") }
	})
	fails("published identity", func() {
		calls := 0
		lifecycleTransactionChildMatches = func(*stagedSpecTree, string, *stagedSpecTree) error {
			calls++
			if calls == 3 {
				return errors.New("published identity failed")
			}
			return nil
		}
		lifecycleTransactionExchange = func(*stagedSpecTree, *stagedSpecTree) error { return nil }
		lifecycleTransactionVerifySnapshot = func(*stagedSpecTree, specTreeSnapshot) error { return nil }
	})
	fails("recovery identity", func() {
		calls := 0
		lifecycleTransactionChildMatches = func(*stagedSpecTree, string, *stagedSpecTree) error {
			calls++
			if calls == 4 {
				return errors.New("recovery identity failed")
			}
			return nil
		}
		lifecycleTransactionExchange = func(*stagedSpecTree, *stagedSpecTree) error { return nil }
		lifecycleTransactionVerifySnapshot = func(*stagedSpecTree, specTreeSnapshot) error { return nil }
	})
	fails("published manifest", func() {
		calls := 0
		lifecycleTransactionVerifySnapshot = func(*stagedSpecTree, specTreeSnapshot) error {
			calls++
			if calls == 3 {
				return errors.New("published manifest failed")
			}
			return nil
		}
	})
	fails("recovery manifest", func() {
		calls := 0
		lifecycleTransactionVerifySnapshot = func(*stagedSpecTree, specTreeSnapshot) error {
			calls++
			if calls == 4 {
				return errors.New("recovery manifest failed")
			}
			return nil
		}
	})
	fails("publishing intent", func() {
		lifecycleTransactionRetainIntent = func(*stagedSpecTree, LifecycleTransactionReceipt) error {
			return errors.New("publishing intent failed")
		}
	})
	fails("committed receipt", func() {
		calls := 0
		lifecycleTransactionWriteReceipt = func(project *stagedSpecTree, receipt LifecycleTransactionReceipt) error {
			calls++
			if calls == 3 {
				return errors.New("committed receipt failed")
			}
			return originalWrite(project, receipt)
		}
		lifecycleTransactionChildMatches = func(*stagedSpecTree, string, *stagedSpecTree) error { return nil }
		lifecycleTransactionExchange = func(*stagedSpecTree, *stagedSpecTree) error { return nil }
		lifecycleTransactionVerifySnapshot = func(*stagedSpecTree, specTreeSnapshot) error { return nil }
	})
}

func TestLifecycleDescriptorContextNativeFailureBranches(t *testing.T) {
	resetLifecycleUnixSeams(t)
	projectRoot := t.TempDir()
	mustWriteLifecycleFile(t, filepath.Join(projectRoot, "specscore.yaml"), "schema: 1\n")
	mustWriteLifecycleFile(t, filepath.Join(projectRoot, ".github", "workflows", "ci.yml"), "name: ci\n")
	mustWriteLifecycleFile(t, filepath.Join(projectRoot, ".git", "config"), "[core]\n")
	mustWriteLifecycleFile(t, filepath.Join(projectRoot, "source"), "source\n")
	project, err := openLifecycleProjectNoFollow(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeStagedSpecTree(project) })
	stage, err := createLifecycleStageProjectNoFollow(project, "native-failures")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeStagedSpecTree(stage) })
	contextStage := func(name string) *stagedSpecTree {
		t.Helper()
		fresh, stageErr := createLifecycleStageProjectNoFollow(project, "context-"+name)
		if stageErr != nil {
			t.Fatal(stageErr)
		}
		t.Cleanup(func() { _ = closeStagedSpecTree(fresh) })
		return fresh
	}
	if _, err := createLifecycleStageDirectoryNoFollow(&stagedSpecTree{}, "closed", specTreeSnapshot{}); err == nil {
		t.Fatal("closed stage descriptor accepted")
	}
	lifecycleStageOpenChild = func(*stagedSpecTree, string) (*stagedSpecTree, error) {
		return nil, errors.New("open staged directory failed")
	}
	if _, err := createLifecycleStageDirectoryNoFollow(stage, "open-fails", specTreeSnapshot{}); err == nil {
		t.Fatal("staged child open error was accepted")
	}
	resetLifecycleUnixSeams(t)
	lifecycleStageMaterialize = func(*stagedSpecTree, specTreeSnapshot) error { return errors.New("materialize failed") }
	if _, err := createLifecycleStageDirectoryNoFollow(stage, "materialize-fails", specTreeSnapshot{}); err == nil {
		t.Fatal("staged materialization error was accepted")
	}
	resetLifecycleUnixSeams(t)
	child, err := createLifecycleStageDirectoryNoFollow(stage, "identity", rootSnapshot(nil))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeStagedSpecTree(child) })
	lifecycleChildStat = func(*os.File) (os.FileInfo, error) { return nil, errors.New("candidate stat failed") }
	if err := lifecycleProjectChildMatches(stage, "identity", child); err == nil {
		t.Fatal("candidate stat error accepted")
	}
	resetLifecycleUnixSeams(t)
	calls := 0
	lifecycleChildStat = func(file *os.File) (os.FileInfo, error) {
		calls++
		if calls == 2 {
			return nil, errors.New("expected stat failed")
		}
		return file.Stat()
	}
	if err := lifecycleProjectChildMatches(stage, "identity", child); err == nil {
		t.Fatal("expected stat error accepted")
	}

	resetLifecycleUnixSeams(t)
	lifecycleContextCopyFile = func(*stagedSpecTree, string, *stagedSpecTree, string) error { return errors.New("config copy failed") }
	if err := materializeLifecycleProjectContext(project, contextStage("copy-config")); err == nil {
		t.Fatal("specscore context-copy failure accepted")
	}
	resetLifecycleUnixSeams(t)
	fileCopies := 0
	lifecycleContextCopyFile = func(source *stagedSpecTree, sourceName string, destination *stagedSpecTree, destinationName string) error {
		fileCopies++
		if fileCopies == 2 {
			return errors.New("git config copy failed")
		}
		return copyOptionalLifecycleRegularFile(source, sourceName, destination, destinationName)
	}
	if err := materializeLifecycleProjectContext(project, contextStage("copy-directory")); err == nil {
		t.Fatal("git config context-copy failure accepted")
	}
	resetLifecycleUnixSeams(t)
	lifecycleContextCopyDirectory = func(*stagedSpecTree, string, *stagedSpecTree, string) error { return errors.New("github copy failed") }
	if err := materializeLifecycleProjectContext(project, contextStage("open-source")); err == nil {
		t.Fatal("github context-copy failure accepted")
	}
	resetLifecycleUnixSeams(t)
	lifecycleContextOpenChild = func(*stagedSpecTree, string) (*stagedSpecTree, error) {
		return nil, errors.New("git source open failed")
	}
	if err := materializeLifecycleProjectContext(project, contextStage("mkdir")); err == nil {
		t.Fatal("git source-open failure accepted")
	}
	resetLifecycleUnixSeams(t)
	lifecycleContextMkdirAt = func(int, string, uint32) error { return errors.New("git stage mkdir failed") }
	if err := materializeLifecycleProjectContext(project, contextStage("open-stage-child")); err == nil {
		t.Fatal("git stage mkdir failure accepted")
	}
	resetLifecycleUnixSeams(t)
	openCalls := 0
	lifecycleContextOpenChild = func(parent *stagedSpecTree, name string) (*stagedSpecTree, error) {
		openCalls++
		if openCalls == 2 {
			return nil, errors.New("git stage open failed")
		}
		return openLifecycleProjectChildNoFollow(parent, name)
	}
	if err := materializeLifecycleProjectContext(project, contextStage("open-stage")); err == nil {
		t.Fatal("git stage-open failure accepted")
	}

	resetLifecycleUnixSeams(t)
	lifecycleStageMaterialize = func(*stagedSpecTree, specTreeSnapshot) error {
		return errors.New("github snapshot materialization failed")
	}
	if err := copyOptionalLifecycleDirectory(project, ".github", stage, "github-fails"); err == nil {
		t.Fatal("github snapshot materialization failure accepted")
	}
	resetLifecycleUnixSeams(t)
	if err := unix.Mkdirat(int(stage.root.Fd()), "github-exists", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := copyOptionalLifecycleDirectory(project, ".github", stage, "github-exists"); err == nil {
		t.Fatal("existing copied directory accepted")
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, ".github-bad"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(projectRoot, ".github-bad", "link")); err != nil {
		t.Fatal(err)
	}
	if err := copyOptionalLifecycleDirectory(project, ".github-bad", stage, "github-bad"); err == nil {
		t.Fatal("unsafe copied directory snapshot accepted")
	}

	for _, failure := range []struct {
		name    string
		arrange func()
	}{
		{"source stat", func() {
			lifecycleContextFileStat = func(*os.File) (os.FileInfo, error) { return nil, errors.New("source stat failed") }
		}},
		{"source read", func() {
			lifecycleContextReadAll = func(io.Reader) ([]byte, error) { return nil, errors.New("source read failed") }
		}},
		{"post-read stat", func() {
			statCalls := 0
			lifecycleContextFileStat = func(file *os.File) (os.FileInfo, error) {
				statCalls++
				if statCalls == 2 {
					return nil, errors.New("post-read stat failed")
				}
				return file.Stat()
			}
		}},
		{"concurrent read", func() {
			statCalls := 0
			lifecycleContextFileStat = func(file *os.File) (os.FileInfo, error) {
				statCalls++
				if statCalls == 2 {
					return lifecycleTestFileInfo{mode: 0o600}, nil
				}
				return file.Stat()
			}
		}},
		{"destination write", func() {
			lifecycleContextWriteAll = func(int, []byte) error { return errors.New("destination write failed") }
		}},
		{"destination chmod", func() {
			lifecycleContextFchmod = func(int, uint32) error { return errors.New("destination chmod failed") }
		}},
		{"destination close", func() {
			lifecycleContextClose = func(fd int) error {
				_ = unix.Close(fd)
				return errors.New("destination close failed")
			}
		}},
	} {
		resetLifecycleUnixSeams(t)
		failure.arrange()
		if err := copyOptionalLifecycleRegularFile(project, "source", stage, "source-"+strings.ReplaceAll(failure.name, " ", "-")); err == nil {
			t.Fatalf("%s failure accepted", failure.name)
		}
	}
}

func TestLifecycleReceiptNativeFailureBranches(t *testing.T) {
	resetLifecycleReceiptSeams(t)
	projectRoot := t.TempDir()
	project, err := openLifecycleProjectNoFollow(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeStagedSpecTree(project) })
	for _, failure := range []struct {
		name    string
		arrange func()
	}{
		{"journal open", func() {
			lifecycleJournalOpenAt = func(int, string, int, uint32) (int, error) { return -1, errors.New("journal open failed") }
		}},
		{"receipt open", func() {
			lifecycleReceiptOpenAt = func(int, string, int, uint32) (int, error) { return -1, errors.New("receipt open failed") }
		}},
		{"receipt stat", func() {
			lifecycleReceiptFstat = func(int, *unix.Stat_t) error { return errors.New("receipt stat failed") }
		}},
		{"receipt temporary name", func() {
			lifecycleReceiptTempName = func(string) (string, error) { return "", errors.New("receipt temporary-name failed") }
		}},
		{"receipt invalid temporary name", func() {
			lifecycleReceiptTempName = func(string) (string, error) { return "../receipt.json", nil }
		}},
		{"receipt write", func() {
			lifecycleReceiptWriteAll = func(int, []byte) error { return errors.New("receipt write failed") }
		}},
		{"receipt fsync", func() { lifecycleReceiptFsync = func(int) error { return errors.New("receipt fsync failed") } }},
		{"receipt close", func() {
			lifecycleReceiptClose = func(fd int) error {
				_ = unix.Close(fd)
				return errors.New("receipt close failed")
			}
		}},
		{"receipt rename", func() {
			lifecycleReceiptRenameAt = func(int, string, int, string) error { return errors.New("receipt rename failed") }
		}},
		{"journal fsync", func() {
			calls := 0
			lifecycleReceiptFsync = func(fd int) error {
				calls++
				if calls == 2 {
					return errors.New("journal fsync failed")
				}
				return unix.Fsync(fd)
			}
		}},
		{"journal parent fsync", func() {
			calls := 0
			lifecycleReceiptFsync = func(fd int) error {
				calls++
				if calls == 3 {
					return errors.New("journal parent fsync failed")
				}
				return unix.Fsync(fd)
			}
		}},
	} {
		resetLifecycleReceiptSeams(t)
		failure.arrange()
		if err := writeLifecycleReceiptNoFollow(project, "receipt-"+strings.ReplaceAll(failure.name, " ", "-")+".json", []byte("receipt\n")); err == nil {
			t.Fatalf("%s failure accepted", failure.name)
		}
	}
	resetLifecycleReceiptSeams(t)
	lifecycleReceiptFstat = func(_ int, stat *unix.Stat_t) error {
		stat.Mode = unix.S_IFDIR
		return nil
	}
	if err := writeLifecycleReceiptNoFollow(project, "nonregular.json", []byte("receipt\n")); err == nil {
		t.Fatal("non-regular receipt descriptor accepted")
	}
	resetLifecycleReceiptSeams(t)
	if err := writeLifecycleReceiptNoFollow(project, "atomic.json", []byte("old\n")); err != nil {
		t.Fatal(err)
	}
	lifecycleReceiptRenameAt = func(int, string, int, string) error { return errors.New("rename interrupted") }
	if err := writeLifecycleReceiptNoFollow(project, "atomic.json", []byte("new\n")); err == nil {
		t.Fatal("receipt rename interruption accepted")
	}
	contents, err := os.ReadFile(filepath.Join(project.path, ".specscore-recovery", "atomic.json"))
	if err != nil || string(contents) != "old\n" {
		t.Fatalf("non-atomic receipt update replaced prior record: %q, %v", contents, err)
	}
	resetLifecycleReceiptSeams(t)
	lifecycleJournalOpenAt = func(int, string, int, uint32) (int, error) { return -1, unix.ENOENT }
	lifecycleJournalMkdirAt = func(int, string, uint32) error { return errors.New("journal mkdir failed") }
	if _, err := openOrCreateLifecycleJournalDirectory(int(project.root.Fd())); err == nil {
		t.Fatal("journal mkdir failure accepted")
	}
}

func TestLifecyclePublishingIntentNativeFailureBranches(t *testing.T) {
	if err := retainLifecyclePublishingIntent(&stagedSpecTree{}, LifecycleTransactionReceipt{ID: "intent-1", State: "publishing"}); err == nil {
		t.Fatal("closed publishing-intent project accepted")
	}
	projectRoot := t.TempDir()
	project, err := openLifecycleProjectNoFollow(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeStagedSpecTree(project) })
	if err := retainLifecyclePublishingIntent(project, LifecycleTransactionReceipt{ID: "intent-1", State: "prepared"}); err == nil {
		t.Fatal("non-publishing intent accepted")
	}
	if err := retainLifecyclePublishingIntent(project, LifecycleTransactionReceipt{ID: "has.dot", State: "publishing"}); err == nil {
		t.Fatal("unsafe publishing-intent id accepted")
	}
	for _, failure := range []struct {
		name    string
		arrange func()
	}{
		{name: "journal open", arrange: func() {
			lifecycleJournalOpenAt = func(int, string, int, uint32) (int, error) {
				return -1, errors.New("publishing-intent journal open failed")
			}
		}},
		{name: "intent link", arrange: func() {
			lifecycleReceiptLinkAt = func(int, string, int, string, int) error {
				return errors.New("publishing-intent link failed")
			}
		}},
		{name: "journal fsync", arrange: func() {
			lifecycleReceiptFsync = func(int) error {
				return errors.New("publishing-intent journal fsync failed")
			}
		}},
		{name: "journal parent fsync", arrange: func() {
			failLifecycleReceiptFsyncAt(2, "publishing-intent journal parent fsync failed")
		}},
	} {
		t.Run(failure.name, func(t *testing.T) {
			resetLifecycleReceiptSeams(t)
			stage, stageErr := createLifecycleStageProjectNoFollow(project, "intent-"+strings.ReplaceAll(failure.name, " ", "-"))
			if stageErr != nil {
				t.Fatal(stageErr)
			}
			t.Cleanup(func() { _ = closeStagedSpecTree(stage) })
			receipt := LifecycleTransactionReceipt{ID: "intent-" + strings.ReplaceAll(failure.name, " ", "-"), State: "publishing"}
			if err := writeLifecycleReceipt(stage, receipt); err != nil {
				t.Fatal(err)
			}
			failure.arrange()
			if err := retainLifecyclePublishingIntent(stage, receipt); err == nil {
				t.Fatalf("%s failure accepted", failure.name)
			}
		})
	}
	resetLifecycleReceiptSeams(t)
	stage, err := createLifecycleStageProjectNoFollow(project, "intent-success")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeStagedSpecTree(stage) })
	receipt := LifecycleTransactionReceipt{ID: "intent-success", State: "publishing"}
	if err := writeLifecycleReceipt(stage, receipt); err != nil {
		t.Fatal(err)
	}
	if err := retainLifecyclePublishingIntent(stage, receipt); err != nil {
		t.Fatalf("retain publishing intent: %v", err)
	}
	if err := retainLifecyclePublishingIntent(stage, receipt); err == nil {
		t.Fatal("existing publishing intent was replaced")
	}
}

func resetLifecycleUnixSeams(t *testing.T) {
	t.Helper()
	originalStageOpenChild := lifecycleStageOpenChild
	originalStageMaterialize := lifecycleStageMaterialize
	originalChildStat := lifecycleChildStat
	originalContextCopyFile := lifecycleContextCopyFile
	originalContextCopyDirectory := lifecycleContextCopyDirectory
	originalContextOpenChild := lifecycleContextOpenChild
	originalContextMkdirAt := lifecycleContextMkdirAt
	originalContextFileStat := lifecycleContextFileStat
	originalContextReadAll := lifecycleContextReadAll
	originalContextWriteAll := lifecycleContextWriteAll
	originalContextFchmod := lifecycleContextFchmod
	originalContextClose := lifecycleContextClose
	lifecycleStageOpenChild = openLifecycleProjectChildNoFollow
	lifecycleStageMaterialize = materializeStagedSpecTreeNoFollow
	lifecycleChildStat = func(file *os.File) (os.FileInfo, error) { return file.Stat() }
	lifecycleContextCopyFile = copyOptionalLifecycleRegularFile
	lifecycleContextCopyDirectory = copyOptionalLifecycleDirectory
	lifecycleContextOpenChild = openLifecycleProjectChildNoFollow
	lifecycleContextMkdirAt = unix.Mkdirat
	lifecycleContextFileStat = func(file *os.File) (os.FileInfo, error) { return file.Stat() }
	lifecycleContextReadAll = io.ReadAll
	lifecycleContextWriteAll = writeAllAtFD
	lifecycleContextFchmod = unix.Fchmod
	lifecycleContextClose = unix.Close
	t.Cleanup(func() {
		lifecycleStageOpenChild = originalStageOpenChild
		lifecycleStageMaterialize = originalStageMaterialize
		lifecycleChildStat = originalChildStat
		lifecycleContextCopyFile = originalContextCopyFile
		lifecycleContextCopyDirectory = originalContextCopyDirectory
		lifecycleContextOpenChild = originalContextOpenChild
		lifecycleContextMkdirAt = originalContextMkdirAt
		lifecycleContextFileStat = originalContextFileStat
		lifecycleContextReadAll = originalContextReadAll
		lifecycleContextWriteAll = originalContextWriteAll
		lifecycleContextFchmod = originalContextFchmod
		lifecycleContextClose = originalContextClose
	})
}

func resetLifecycleReceiptSeams(t *testing.T) {
	t.Helper()
	originalReceiptOpenAt := lifecycleReceiptOpenAt
	originalReceiptFstat := lifecycleReceiptFstat
	originalReceiptWriteAll := lifecycleReceiptWriteAll
	originalReceiptFsync := lifecycleReceiptFsync
	originalReceiptClose := lifecycleReceiptClose
	originalReceiptRenameAt := lifecycleReceiptRenameAt
	originalReceiptTempName := lifecycleReceiptTempName
	originalReceiptLinkAt := lifecycleReceiptLinkAt
	originalJournalOpenAt := lifecycleJournalOpenAt
	originalJournalMkdirAt := lifecycleJournalMkdirAt
	lifecycleReceiptOpenAt = unix.Openat
	lifecycleReceiptFstat = unix.Fstat
	lifecycleReceiptWriteAll = writeAllAtFD
	lifecycleReceiptFsync = unix.Fsync
	lifecycleReceiptClose = unix.Close
	lifecycleReceiptRenameAt = unix.Renameat
	lifecycleReceiptTempName = newLifecycleReceiptTempName
	lifecycleReceiptLinkAt = unix.Linkat
	lifecycleJournalOpenAt = unix.Openat
	lifecycleJournalMkdirAt = unix.Mkdirat
	t.Cleanup(func() {
		lifecycleReceiptOpenAt = originalReceiptOpenAt
		lifecycleReceiptFstat = originalReceiptFstat
		lifecycleReceiptWriteAll = originalReceiptWriteAll
		lifecycleReceiptFsync = originalReceiptFsync
		lifecycleReceiptClose = originalReceiptClose
		lifecycleReceiptRenameAt = originalReceiptRenameAt
		lifecycleReceiptTempName = originalReceiptTempName
		lifecycleReceiptLinkAt = originalReceiptLinkAt
		lifecycleJournalOpenAt = originalJournalOpenAt
		lifecycleJournalMkdirAt = originalJournalMkdirAt
	})
}

func failLifecycleReceiptFsyncAt(failAt int, message string) {
	calls := 0
	lifecycleReceiptFsync = func(fd int) error {
		calls++
		if calls == failAt {
			return errors.New(message)
		}
		return unix.Fsync(fd)
	}
}

func resetLifecyclePublicationSeams(t *testing.T) {
	t.Helper()
	originalExchange := lifecyclePublicationExchange
	originalFsync := lifecyclePublicationFsync
	lifecyclePublicationExchange = lifecycleExchangeSpecAt
	lifecyclePublicationFsync = unix.Fsync
	t.Cleanup(func() {
		lifecyclePublicationExchange = originalExchange
		lifecyclePublicationFsync = originalFsync
	})
}

func resetLifecycleRecoveryReadSeams(t *testing.T) {
	t.Helper()
	originalOpenProject := lifecycleRecoveryOpenProject
	originalOpenChild := lifecycleRecoveryOpenChild
	originalOpenFileAt := lifecycleRecoveryOpenFileAt
	originalFileStat := lifecycleRecoveryFileStat
	originalReadAll := lifecycleRecoveryReadAll
	originalBeforeRead := lifecycleRecoveryBeforeRead
	originalReadDir := lifecycleRecoveryReadDir
	lifecycleRecoveryOpenProject = openLifecycleProjectNoFollow
	lifecycleRecoveryOpenChild = openLifecycleProjectChildNoFollow
	lifecycleRecoveryOpenFileAt = unix.Openat
	lifecycleRecoveryFileStat = func(file *os.File) (os.FileInfo, error) { return file.Stat() }
	lifecycleRecoveryReadAll = io.ReadAll
	lifecycleRecoveryBeforeRead = func(*os.File) error { return nil }
	lifecycleRecoveryReadDir = func(file *os.File, count int) ([]os.DirEntry, error) { return file.ReadDir(count) }
	t.Cleanup(func() {
		lifecycleRecoveryOpenProject = originalOpenProject
		lifecycleRecoveryOpenChild = originalOpenChild
		lifecycleRecoveryOpenFileAt = originalOpenFileAt
		lifecycleRecoveryFileStat = originalFileStat
		lifecycleRecoveryReadAll = originalReadAll
		lifecycleRecoveryBeforeRead = originalBeforeRead
		lifecycleRecoveryReadDir = originalReadDir
	})
}

func mustMarshalReceipt(t *testing.T, receipt LifecycleTransactionReceipt) string {
	t.Helper()
	data, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func lifecycleDigestAt(t *testing.T, specRoot string) string {
	t.Helper()
	snapshot, err := snapshotSpecTreeNoFollow(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	return lifecycleSnapshotDigest(snapshot)
}

func mustWriteLifecycleFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
