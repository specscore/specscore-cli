//go:build darwin

package cli

import (
	"errors"
	"testing"
	"time"
)

func TestSetStagedEntryModificationTime_DarwinError(t *testing.T) {
	original := stageDarwinFsetattrlist
	stageDarwinFsetattrlist = func(int, time.Time) error { return errors.New("fsetattrlist failed") }
	t.Cleanup(func() { stageDarwinFsetattrlist = original })
	if err := setStagedEntryModificationTime(-1, nowForLifecycleTimeTest()); err == nil || !contains(err, "fsetattrlist failed") {
		t.Fatalf("timestamp error = %v", err)
	}
	if err := applyStagedEntryMetadata(-1, specTreeEntryMetadata{modificationTime: nowForLifecycleTimeTest()}); err == nil || !contains(err, "fsetattrlist failed") {
		t.Fatalf("metadata timestamp error = %v", err)
	}
	if err := fsetStagedEntryModificationTime(-1, nowForLifecycleTimeTest()); err == nil {
		t.Fatal("raw fsetattrlist unexpectedly succeeded for an invalid descriptor")
	}
}
