package event

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MergeResult describes an event-ledger union. Existing target bytes are
// never rewritten; Added is the number of source-only events appended.
type MergeResult struct {
	Target   string
	Existing int
	Added    int
	Skipped  int
}

// MergeOptions controls MergeLedgersWithOptions.
type MergeOptions struct {
	// DryRun validates and plans the union without changing the target.
	DryRun bool
}

// MergeInputError identifies a merge refusal caused by invalid or ambiguous
// input. CLI callers can map it to their invalid-arguments exit status while
// retaining a different status for publication failures.
type MergeInputError struct{ Err error }

func (e *MergeInputError) Error() string { return e.Err.Error() }
func (e *MergeInputError) Unwrap() error { return e.Err }

// IsMergeInputError reports whether err is an input-validation refusal.
func IsMergeInputError(err error) bool {
	var target *MergeInputError
	return errors.As(err, &target)
}

func mergeInputErrorf(format string, args ...any) error {
	return &MergeInputError{Err: fmt.Errorf(format, args...)}
}

// MergeLedgers unions one or more JSONL event ledgers into target. Target
// records retain their exact bytes and order. Source-only records are sorted
// by UUID before being appended, making the result independent of source
// argument order and concurrent branch completion order.
func MergeLedgers(target string, sources []string) (MergeResult, error) {
	return MergeLedgersWithOptions(target, sources, MergeOptions{})
}

// MergeJSONL is the concise public spelling for MergeLedgers.
func MergeJSONL(target string, sources []string) (MergeResult, error) {
	return MergeLedgers(target, sources)
}

// MergeLedgersWithOptions validates all input before publishing one atomic,
// durable replacement of target. Any malformed or conflicting source leaves
// target untouched.
func MergeLedgersWithOptions(target string, sources []string, options MergeOptions) (MergeResult, error) {
	var result MergeResult
	if len(sources) == 0 {
		return result, mergeInputErrorf("event ledger merge requires at least one source ledger")
	}

	target, err := absoluteCleanPath(target)
	if err != nil {
		return result, &MergeInputError{Err: fmt.Errorf("resolve target ledger: %w", err)}
	}
	if err := rejectSymlink(target, "target ledger"); err != nil {
		return result, err
	}
	result.Target = target
	targetInfo, targetStatErr := os.Stat(target)
	if targetStatErr != nil && !errors.Is(targetStatErr, os.ErrNotExist) {
		return result, &MergeInputError{Err: fmt.Errorf("stat target ledger %s: %w", target, targetStatErr)}
	}
	if errors.Is(targetStatErr, os.ErrNotExist) {
		targetInfo = nil
	}

	uniqueSources := make([]string, 0, len(sources))
	seenPaths := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		path, err := absoluteCleanPath(source)
		if err != nil {
			return result, &MergeInputError{Err: fmt.Errorf("resolve source ledger %q: %w", source, err)}
		}
		if err := rejectSymlink(path, "source ledger"); err != nil {
			return result, err
		}
		if path == target {
			return result, mergeInputErrorf("source ledger %q is the target ledger; refusing ambiguous self-merge", source)
		}
		if targetInfo != nil {
			sourceInfo, statErr := os.Stat(path)
			if statErr == nil && os.SameFile(targetInfo, sourceInfo) {
				return result, mergeInputErrorf("source ledger %q aliases the target ledger; refusing ambiguous self-merge", source)
			}
			if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
				return result, &MergeInputError{Err: fmt.Errorf("stat source ledger %s: %w", path, statErr)}
			}
		}
		if _, duplicate := seenPaths[path]; duplicate {
			return result, mergeInputErrorf("source ledger %q is listed more than once", source)
		}
		seenPaths[path] = struct{}{}
		uniqueSources = append(uniqueSources, path)
	}

	targetData, err := readLedgerFile(target, true)
	if err != nil {
		return result, err
	}
	targetRecords, err := parseLedger(targetData, target)
	if err != nil {
		return result, &MergeInputError{Err: err}
	}
	result.Existing = len(targetRecords)
	byUUID := make(map[string]canonicalLedgerRecord, len(targetRecords))
	for _, record := range targetRecords {
		if prior, exists := byUUID[record.event.UUID]; exists && !bytes.Equal(prior.canonical, record.canonical) {
			return result, mergeInputErrorf("target ledger %s has conflicting event UUID %s", target, record.event.UUID)
		}
		byUUID[record.event.UUID] = record
	}

	var additions []canonicalLedgerRecord
	for _, source := range uniqueSources {
		data, err := readLedgerFile(source, false)
		if err != nil {
			return result, err
		}
		records, err := parseLedger(data, source)
		if err != nil {
			return result, &MergeInputError{Err: err}
		}
		for _, record := range records {
			if prior, exists := byUUID[record.event.UUID]; exists {
				if !bytes.Equal(prior.canonical, record.canonical) {
					return result, mergeInputErrorf("conflicting event UUID %s between ledgers (source %s)", record.event.UUID, source)
				}
				result.Skipped++
				continue
			}
			byUUID[record.event.UUID] = record
			additions = append(additions, record)
		}
	}

	sort.SliceStable(additions, func(i, j int) bool { return additions[i].event.UUID < additions[j].event.UUID })
	result.Added = len(additions)
	if result.Added == 0 || options.DryRun {
		return result, nil
	}

	output := make([]byte, 0, len(targetData)+len(additions)*128)
	output = append(output, targetData...)
	if len(output) > 0 && output[len(output)-1] != '\n' {
		output = append(output, '\n')
	}
	for _, addition := range additions {
		output = append(output, addition.raw...)
		if len(addition.raw) == 0 || addition.raw[len(addition.raw)-1] != '\n' {
			output = append(output, '\n')
		}
	}
	if err := writeLedgerAtomic(target, output); err != nil {
		return MergeResult{}, err
	}
	return result, nil
}

type canonicalLedgerRecord struct {
	event     Event
	canonical []byte
	raw       []byte
}

func parseLedger(data []byte, path string) ([]canonicalLedgerRecord, error) {
	if len(data) == 0 {
		return nil, nil
	}
	reader := bufio.NewReader(bytes.NewReader(data))
	var records []canonicalLedgerRecord
	lineNumber := 0
	for {
		raw, readErr := reader.ReadBytes('\n')
		if len(raw) != 0 {
			lineNumber++
			line := bytes.TrimSuffix(raw, []byte{'\n'})
			line = bytes.TrimSuffix(line, []byte{'\r'})
			if len(bytes.TrimSpace(line)) == 0 {
				return nil, fmt.Errorf("invalid event ledger %s line %d: empty JSONL record", path, lineNumber)
			}
			e, canonical, err := decodeLedgerEvent(line)
			if err != nil {
				return nil, fmt.Errorf("invalid event ledger %s line %d: %w", path, lineNumber, err)
			}
			record := canonicalLedgerRecord{event: e, canonical: canonical, raw: append([]byte(nil), line...)}
			records = append(records, record)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("read event ledger %s line %d: %w", path, lineNumber+1, readErr)
		}
	}
	return records, nil
}

func decodeLedgerEvent(raw []byte) (Event, []byte, error) {
	var e Event
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&e); err != nil {
		return e, nil, fmt.Errorf("malformed JSON or unknown field: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return e, nil, errors.New("trailing JSON")
		}
		return e, nil, fmt.Errorf("trailing JSON: %w", err)
	}
	if err := Validate(e); err != nil {
		return e, nil, err
	}
	canonical, err := canonicalEvent(e)
	if err != nil {
		return e, nil, fmt.Errorf("canonicalize event: %w", err)
	}
	return e, canonical, nil
}

func canonicalEvent(e Event) ([]byte, error) {
	payload, err := canonicalJSON(e.Payload)
	if err != nil {
		return nil, fmt.Errorf("canonicalize payload: %w", err)
	}
	e.Payload = payload
	e.Timestamp = e.Timestamp.UTC()
	return json.Marshal(e)
}

func absoluteCleanPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("path must not be empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func rejectSymlink(path, kind string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return &MergeInputError{Err: fmt.Errorf("inspect %s %s: %w", kind, path, err)}
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return mergeInputErrorf("%s %s is a symlink; refusing unsafe path ambiguity", kind, path)
	}
	return nil
}

func readLedgerFile(path string, target bool) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) && target {
		return nil, nil
	}
	if err != nil {
		return nil, &MergeInputError{Err: fmt.Errorf("read event ledger %s: %w", path, err)}
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, &MergeInputError{Err: fmt.Errorf("stat event ledger %s: %w", path, err)}
	}
	if !info.Mode().IsRegular() {
		return nil, mergeInputErrorf("event ledger %s is not a regular file", path)
	}
	return data, nil
}

func writeLedgerAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create event ledger directory %s: %w", dir, err)
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat event ledger %s: %w", path, err)
	}
	temp, err := os.CreateTemp(dir, ".events-merge-*")
	if err != nil {
		return fmt.Errorf("create temporary event ledger: %w", err)
	}
	tempPath := temp.Name()
	defer func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}()
	if err := temp.Chmod(mode); err != nil {
		return fmt.Errorf("set temporary event ledger mode: %w", err)
	}
	written, err := temp.Write(data)
	if err != nil {
		return fmt.Errorf("write temporary event ledger: %w", err)
	}
	if written != len(data) {
		return fmt.Errorf("write temporary event ledger: short write (%d/%d bytes)", written, len(data))
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync temporary event ledger: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary event ledger: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("publish event ledger %s: %w", path, err)
	}
	// Syncing the parent directory makes the rename durable on filesystems
	// that require an explicit directory fence.
	dirFile, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open event ledger directory: %w", err)
	}
	if err := dirFile.Sync(); err != nil {
		_ = dirFile.Close()
		return fmt.Errorf("sync event ledger directory: %w", err)
	}
	if err := dirFile.Close(); err != nil {
		return fmt.Errorf("close event ledger directory: %w", err)
	}
	return nil
}
