package event

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mergeTestEvent(id string, payload string) Event {
	ts, _ := time.Parse(time.RFC3339, "2026-08-31T12:00:00Z")
	return Event{
		Name: "lesson.observed", Version: 1, UUID: id, Timestamp: ts,
		Actor:    Actor{Kind: "external", ID: "merge-test"},
		Artifact: Artifact{Type: "lesson", ID: "ledger", Path: "spec/lessons/ledger/README.md", Revision: "uncommitted"},
		Payload:  json.RawMessage(payload),
	}
}

func writeMergeLedger(t *testing.T, path string, events ...Event) []byte {
	t.Helper()
	var b []byte
	for _, e := range events {
		line, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		b = append(b, line...)
		b = append(b, '\n')
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestMergeLedgers_ConcurrentSourcesPreserveTargetAndSortAdditions(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.jsonl")
	sourceA := filepath.Join(dir, "branch-a.jsonl")
	sourceB := filepath.Join(dir, "branch-b.jsonl")
	targetBytes := writeMergeLedger(t, target, mergeTestEvent("00000000-0000-4000-8000-000000000001", `{"target":true}`))
	writeMergeLedger(t, sourceA, mergeTestEvent("00000000-0000-4000-8000-000000000003", `{"branch":"a"}`))
	writeMergeLedger(t, sourceB, mergeTestEvent("00000000-0000-4000-8000-000000000002", `{"branch":"b"}`))

	result, err := MergeLedgers(target, []string{sourceB, sourceA})
	if err != nil {
		t.Fatalf("MergeLedgers: %v", err)
	}
	if result.Existing != 1 || result.Added != 2 {
		t.Fatalf("result = %#v, want existing=1 added=2", result)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(got), string(targetBytes)) {
		t.Fatalf("target prefix was rewritten:\n got %q\nwant prefix %q", got, targetBytes)
	}
	lines := strings.Split(strings.TrimSuffix(string(got), "\n"), "\n")
	if len(lines) != 3 || !strings.Contains(lines[1], "000000000002") || !strings.Contains(lines[2], "000000000003") {
		t.Fatalf("merged lines are not target-then-UUID order: %v", lines)
	}
}

func TestMergeLedgers_IdenticalCanonicalDuplicateIsSkippedAndIdempotent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.jsonl")
	source := filepath.Join(dir, "source.jsonl")
	e := mergeTestEvent("00000000-0000-4000-8000-000000000010", `{"b":2,"a":1}`)
	original := []byte(`{"payload":{"a":1,"b":2},"artifact":{"revision":"uncommitted","path":"spec/lessons/ledger/README.md","id":"ledger","type":"lesson"},"actor":{"id":"merge-test","kind":"external"},"timestamp":"2026-08-31T12:00:00+00:00","uuid":"00000000-0000-4000-8000-000000000010","version":1,"name":"lesson.observed"}` + "\n")
	if err := os.WriteFile(target, original, 0o644); err != nil {
		t.Fatal(err)
	}
	writeMergeLedger(t, source, e)

	result, err := MergeLedgers(target, []string{source})
	if err != nil {
		t.Fatalf("MergeLedgers duplicate: %v", err)
	}
	if result.Added != 0 || result.Skipped != 1 {
		t.Fatalf("result = %#v, want skipped=1 and no append", result)
	}
	got, _ := os.ReadFile(target)
	if string(got) != string(original) {
		t.Fatalf("identical duplicate rewrote target bytes")
	}
	second, err := MergeLedgers(target, []string{source})
	if err != nil || second.Added != 0 {
		t.Fatalf("idempotent rerun = %#v, %v", second, err)
	}
	got, _ = os.ReadFile(target)
	if string(got) != string(original) {
		t.Fatalf("idempotent rerun changed target bytes")
	}
}

func TestMergeLedgers_ConflictMalformedAndSelfLeaveTargetUnchanged(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.jsonl")
	source := filepath.Join(dir, "source.jsonl")
	e := mergeTestEvent("00000000-0000-4000-8000-000000000020", `{"ok":true}`)
	original := writeMergeLedger(t, target, mergeTestEvent("00000000-0000-4000-8000-000000000001", `{"target":true}`))

	writeMergeLedger(t, source, e)
	if err := os.WriteFile(source, append(mustMergeJSON(t, e), '\n', '{', 'b'), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := MergeLedgers(target, []string{source}); err == nil || !IsMergeInputError(err) {
		t.Fatal("expected malformed source merge to fail")
	}
	assertMergeUnchanged(t, target, original)

	writeMergeLedger(t, source, mergeTestEvent("00000000-0000-4000-8000-000000000001", `{"ok":false}`))
	if _, err := MergeLedgers(target, []string{source}); err == nil || !IsMergeInputError(err) {
		t.Fatalf("expected conflicting duplicate input error, got %v", err)
	}
	assertMergeUnchanged(t, target, original)

	if _, err := MergeLedgers(target, []string{target}); err == nil || !IsMergeInputError(err) {
		t.Fatalf("expected self-merge input error, got %v", err)
	}
	assertMergeUnchanged(t, target, original)
}

func mustMergeJSON(t *testing.T, e Event) []byte {
	t.Helper()
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func assertMergeUnchanged(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("target changed after failed merge: got %q want %q", got, want)
	}
}

func TestMergeLedgers_RejectsInvalidAndAmbiguousInputs(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.jsonl")
	source := filepath.Join(dir, "source.jsonl")
	e := mergeTestEvent("00000000-0000-4000-8000-000000000040", `{"ok":true}`)
	writeMergeLedger(t, target, e)
	writeMergeLedger(t, source, mergeTestEvent("00000000-0000-4000-8000-000000000041", `{"source":true}`))

	tests := []struct {
		name    string
		target  string
		sources []string
		prepare func(t *testing.T)
	}{
		{name: "no sources", target: target},
		{name: "empty target path", target: "", sources: []string{source}},
		{name: "empty source path", target: target, sources: []string{""}},
		{name: "duplicate source path", target: target, sources: []string{source, source}},
		{name: "missing source", target: target, sources: []string{filepath.Join(dir, "missing.jsonl")}},
		{name: "source directory", target: target, sources: []string{filepath.Join(dir, "source-dir")}, prepare: func(t *testing.T) {
			if err := os.Mkdir(filepath.Join(dir, "source-dir"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "target symlink", target: filepath.Join(dir, "target-link"), sources: []string{source}, prepare: func(t *testing.T) {
			if err := os.Symlink(target, filepath.Join(dir, "target-link")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "source symlink", target: target, sources: []string{filepath.Join(dir, "source-link")}, prepare: func(t *testing.T) {
			if err := os.Symlink(source, filepath.Join(dir, "source-link")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "self merge", target: target, sources: []string{target}},
		{name: "hard link aliases target", target: target, sources: []string{filepath.Join(dir, "target-hardlink")}, prepare: func(t *testing.T) {
			if err := os.Link(target, filepath.Join(dir, "target-hardlink")); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.prepare != nil {
				tt.prepare(t)
			}
			_, err := MergeLedgers(tt.target, tt.sources)
			if err == nil || !IsMergeInputError(err) {
				t.Fatalf("MergeLedgers error = %v, want MergeInputError", err)
			}
		})
	}
}

func TestMergeLedgers_RejectsMalformedRecordsAndConflicts(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.jsonl")
	source := filepath.Join(dir, "source.jsonl")
	e := mergeTestEvent("00000000-0000-4000-8000-000000000050", `{"ok":true}`)
	original := writeMergeLedger(t, target, mergeTestEvent("00000000-0000-4000-8000-000000000051", `{"target":true}`))

	cases := []struct {
		name string
		path string
		data []byte
	}{
		{name: "empty target line", path: target, data: append(append([]byte(nil), original...), '\n')},
		{name: "unknown field", path: source, data: []byte(`{"name":"lesson.observed","unexpected":true}` + "\n")},
		{name: "trailing json", path: source, data: append(mustMergeJSON(t, e), []byte(` {}`+"\n")...)},
		{name: "invalid event", path: source, data: []byte(`{"name":"not-valid"}` + "\n")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.path != target {
				writeMergeLedger(t, target, e)
			}
			if err := os.WriteFile(tc.path, tc.data, 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := MergeLedgers(target, []string{source})
			if err == nil || !IsMergeInputError(err) {
				t.Fatalf("MergeLedgers error = %v, want MergeInputError", err)
			}
		})
	}

	prior := mergeTestEvent("00000000-0000-4000-8000-000000000051", `{"target":true}`)
	conflict := mergeTestEvent(prior.UUID, `{"target":false}`)
	writeMergeLedger(t, target, prior, conflict)
	writeMergeLedger(t, source, e)
	if _, err := MergeLedgers(target, []string{source}); err == nil || !IsMergeInputError(err) || !strings.Contains(err.Error(), "target ledger") {
		t.Fatalf("target conflict error = %v, want MergeInputError", err)
	}

	writeMergeLedger(t, target, e)
	conflict = mergeTestEvent("00000000-0000-4000-8000-000000000052", `{"first":true}`)
	writeMergeLedger(t, source, conflict, mergeTestEvent(conflict.UUID, `{"first":false}`))
	unchanged, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := MergeLedgers(target, []string{source}); err == nil || !IsMergeInputError(err) || !strings.Contains(err.Error(), "between ledgers") {
		t.Fatalf("source conflict error = %v, want MergeInputError", err)
	}
	assertMergeUnchanged(t, target, unchanged)
}

func TestMergeLedgers_DuplicatePayloadKeysAreRejected(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.jsonl")
	source := filepath.Join(dir, "source.jsonl")
	writeMergeLedger(t, target, mergeTestEvent("00000000-0000-4000-8000-000000000070", `{"target":true}`))
	bad := mergeTestEvent("00000000-0000-4000-8000-000000000071", `{"duplicate":1,"duplicate":2}`)
	writeMergeLedger(t, source, bad)
	if _, err := MergeLedgers(target, []string{source}); err == nil || !IsMergeInputError(err) || !strings.Contains(err.Error(), "canonicalize") {
		t.Fatalf("duplicate payload error = %v, want canonicalization MergeInputError", err)
	}
}

func TestMergeLedgers_DryRunAndJSONLAliasPreserveBytes(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.jsonl")
	source := filepath.Join(dir, "source.jsonl")
	targetEvent := mergeTestEvent("00000000-0000-4000-8000-000000000060", `{"target":true}`)
	addition := mergeTestEvent("00000000-0000-4000-8000-000000000061", `{"source":true}`)
	targetRaw := mustMergeJSON(t, targetEvent)
	if err := os.WriteFile(target, targetRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	sourceRaw := mustMergeJSON(t, addition)
	if err := os.WriteFile(source, sourceRaw, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := MergeLedgersWithOptions(target, []string{source}, MergeOptions{DryRun: true})
	if err != nil || result.Added != 1 {
		t.Fatalf("dry-run result = %#v, err=%v", result, err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(targetRaw) {
		t.Fatalf("dry-run changed target: got %q want %q", got, targetRaw)
	}

	result, err = MergeJSONL(target, []string{source})
	if err != nil || result.Added != 1 {
		t.Fatalf("MergeJSONL result = %#v, err=%v", result, err)
	}
	got, err = os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(got), string(targetRaw)+"\n") || !strings.HasSuffix(string(got), string(sourceRaw)+"\n") {
		t.Fatalf("no-newline target was not safely appended: %q", got)
	}
}

func TestMergeInputError_WrapsUnderlyingError(t *testing.T) {
	underlying := errors.New("underlying")
	err := &MergeInputError{Err: underlying}
	if err.Error() != underlying.Error() {
		t.Fatalf("Error() = %q, want %q", err.Error(), underlying.Error())
	}
	if !errors.Is(err, underlying) || !errors.Is(err, err) {
		t.Fatalf("Unwrap/Is did not preserve underlying error")
	}
}

func TestParseLedger_EmptyAndBlankRecords(t *testing.T) {
	records, err := parseLedger(nil, "empty.jsonl")
	if err != nil || records != nil {
		t.Fatalf("empty parse = %#v, %v; want nil, nil", records, err)
	}
	for _, data := range [][]byte{[]byte("\n"), []byte("  \r\n")} {
		if _, err := parseLedger(data, "blank.jsonl"); err == nil {
			t.Fatalf("parseLedger(%q) succeeded, want empty-record error", data)
		}
	}
}

func TestDecodeLedgerEvent_TrailingMalformedJSON(t *testing.T) {
	e := mergeTestEvent("00000000-0000-4000-8000-000000000080", `{"ok":true}`)
	if _, _, err := decodeLedgerEvent(append(mustMergeJSON(t, e), []byte(" x")...)); err == nil || !strings.Contains(err.Error(), "trailing JSON") {
		t.Fatalf("trailing malformed JSON error = %v", err)
	}
}

func TestRejectSymlink_InspectError(t *testing.T) {
	if err := rejectSymlink("\x00", "test ledger"); err == nil || !IsMergeInputError(err) {
		t.Fatalf("rejectSymlink invalid path error = %v, want MergeInputError", err)
	}
}

func TestReadLedgerFile_MissingTargetAndDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.jsonl")
	data, err := readLedgerFile(missing, true)
	if err != nil || data != nil {
		t.Fatalf("missing target read = %q, %v; want nil, nil", data, err)
	}
	dir := t.TempDir()
	if _, err := readLedgerFile(dir, false); err == nil || !IsMergeInputError(err) {
		t.Fatalf("directory read error = %v, want MergeInputError", err)
	}
	device := filepath.Join(t.TempDir(), "device")
	if err := os.Symlink("/dev/null", device); err != nil {
		t.Fatal(err)
	}
	if _, err := readLedgerFile(device, false); err == nil || !IsMergeInputError(err) || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("non-regular read error = %v, want non-regular-file MergeInputError", err)
	}
}

func TestAbsoluteCleanPath_Error(t *testing.T) {
	original := mergeAbsFn
	mergeAbsFn = func(string) (string, error) { return "", errors.New("abs") }
	t.Cleanup(func() { mergeAbsFn = original })
	if _, err := absoluteCleanPath("relative.jsonl"); err == nil {
		t.Fatal("absoluteCleanPath unexpectedly succeeded")
	}
}

type mergeFakeTemp struct {
	name               string
	chmodErr, writeErr error
	writeCount         int
	shortWrite         bool
	syncErr, closeErr  error
}

func (f *mergeFakeTemp) Name() string            { return f.name }
func (f *mergeFakeTemp) Chmod(os.FileMode) error { return f.chmodErr }
func (f *mergeFakeTemp) Write(data []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	if f.shortWrite {
		return len(data) - 1, nil
	}
	f.writeCount++
	return len(data), nil
}
func (f *mergeFakeTemp) Sync() error  { return f.syncErr }
func (f *mergeFakeTemp) Close() error { return f.closeErr }

type mergeFakeDir struct{ syncErr, closeErr error }

func (f mergeFakeDir) Sync() error  { return f.syncErr }
func (f mergeFakeDir) Close() error { return f.closeErr }

func TestWriteLedgerAtomic_ErrorPaths(t *testing.T) {
	originalStat := mergeStatFn
	originalMkdir := mergeMkdirAllFn
	originalCreate := mergeCreateTempFn
	originalRename := mergeRenameFn
	originalOpen := mergeOpenDirFn
	t.Cleanup(func() {
		mergeStatFn = originalStat
		mergeMkdirAllFn = originalMkdir
		mergeCreateTempFn = originalCreate
		mergeRenameFn = originalRename
		mergeOpenDirFn = originalOpen
	})

	dir := t.TempDir()
	target := filepath.Join(dir, "ledger.jsonl")
	assertAtomicError := func(t *testing.T, name string) {
		t.Helper()
		if err := writeLedgerAtomic(target, []byte("data")); err == nil || !strings.Contains(err.Error(), name) {
			t.Fatalf("writeLedgerAtomic error = %v, want %q", err, name)
		}
	}

	mergeMkdirAllFn = func(string, os.FileMode) error { return errors.New("mkdir") }
	assertAtomicError(t, "create event ledger directory")
	mergeMkdirAllFn = os.MkdirAll
	mergeStatFn = func(string) (os.FileInfo, error) { return nil, errors.New("stat") }
	assertAtomicError(t, "stat event ledger")
	mergeStatFn = os.Stat
	mergeCreateTempFn = func(string, string) (mergeTempFile, error) { return nil, errors.New("temp") }
	assertAtomicError(t, "create temporary event ledger")

	for _, tc := range []struct {
		name string
		file mergeFakeTemp
	}{
		{name: "set temporary event ledger mode", file: mergeFakeTemp{name: filepath.Join(dir, "tmp"), chmodErr: errors.New("chmod")}},
		{name: "write temporary event ledger", file: mergeFakeTemp{name: filepath.Join(dir, "tmp"), writeErr: errors.New("write")}},
		{name: "short write", file: mergeFakeTemp{name: filepath.Join(dir, "tmp"), shortWrite: true}},
		{name: "sync temporary event ledger", file: mergeFakeTemp{name: filepath.Join(dir, "tmp"), syncErr: errors.New("sync")}},
		{name: "close temporary event ledger", file: mergeFakeTemp{name: filepath.Join(dir, "tmp"), closeErr: errors.New("close")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mergeCreateTempFn = func(string, string) (mergeTempFile, error) { return &tc.file, nil }
			assertAtomicError(t, tc.name)
			mergeCreateTempFn = func(dir, pattern string) (mergeTempFile, error) { return os.CreateTemp(dir, pattern) }
		})
	}

	mergeRenameFn = func(string, string) error { return errors.New("rename") }
	assertAtomicError(t, "publish event ledger")
	mergeRenameFn = os.Rename
	mergeOpenDirFn = func(string) (mergeDirFile, error) { return nil, errors.New("open") }
	assertAtomicError(t, "open event ledger directory")
	mergeOpenDirFn = func(string) (mergeDirFile, error) { return mergeFakeDir{syncErr: errors.New("dir sync")}, nil }
	assertAtomicError(t, "sync event ledger directory")
	mergeOpenDirFn = func(string) (mergeDirFile, error) { return mergeFakeDir{closeErr: errors.New("dir close")}, nil }
	assertAtomicError(t, "close event ledger directory")
}

func TestMergeLedgers_FilesystemValidationErrors(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.jsonl")
	source := filepath.Join(dir, "source.jsonl")
	writeMergeLedger(t, source, mergeTestEvent("00000000-0000-4000-8000-000000000090", `{"source":true}`))

	originalStat := mergeStatFn
	t.Cleanup(func() { mergeStatFn = originalStat })
	mergeStatFn = func(path string) (os.FileInfo, error) {
		if path == target || path == filepath.Clean(target) {
			return nil, errors.New("target stat")
		}
		return os.Stat(path)
	}
	if _, err := MergeLedgers(target, []string{source}); err == nil || !IsMergeInputError(err) {
		t.Fatalf("target stat error = %v, want MergeInputError", err)
	}

	mergeStatFn = func(path string) (os.FileInfo, error) {
		if path == source {
			return nil, errors.New("source stat")
		}
		return os.Stat(path)
	}
	if _, err := MergeLedgers(target, []string{source}); err == nil || !IsMergeInputError(err) {
		t.Fatalf("source stat error = %v, want MergeInputError", err)
	}

	mergeStatFn = os.Stat
	if err := os.Mkdir(filepath.Join(dir, "target-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := MergeLedgers(filepath.Join(dir, "target-dir"), []string{source}); err == nil || !IsMergeInputError(err) {
		t.Fatalf("target read error = %v, want MergeInputError", err)
	}

	writeMergeLedger(t, target, mergeTestEvent("00000000-0000-4000-8000-000000000091", `{"target":true}`))
	originalWrite := writeLedgerAtomicFn
	writeLedgerAtomicFn = func(string, []byte) error { return errors.New("publish") }
	t.Cleanup(func() { writeLedgerAtomicFn = originalWrite })
	if _, err := MergeLedgers(target, []string{source}); err == nil || !strings.Contains(err.Error(), "publish") {
		t.Fatalf("atomic publication error = %v, want publish error", err)
	}
}

func TestWriteLedgerAtomic_RenameError(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target-dir")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeLedgerAtomic(target, []byte("data")); err == nil || !strings.Contains(err.Error(), "publish event ledger") {
		t.Fatalf("rename error = %v, want publication error", err)
	}
}
