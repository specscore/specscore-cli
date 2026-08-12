package lifecycle

import (
	"errors"
	"testing"
)

func TestTransactionByteTransformsEmptyAndMalformedInputs(t *testing.T) {
	if got, wrote, err := SetSupersededByBytes([]byte("body\n"), "   "); err != nil || wrote || string(got) != "body\n" {
		t.Fatalf("empty successor: got=%q wrote=%v err=%v", got, wrote, err)
	}
	if got, wrote, err := SetSupersedesBytes([]byte("body\n"), "   "); err != nil || wrote || string(got) != "body\n" {
		t.Fatalf("empty predecessor: got=%q wrote=%v err=%v", got, wrote, err)
	}
	if got, wrote, err := AppendResolutionNoteBytes([]byte("body\n"), " \t\n"); err != nil || wrote || string(got) != "body\n" {
		t.Fatalf("empty note: got=%q wrote=%v err=%v", got, wrote, err)
	}
	if status, err := StatusFromBytes([]byte("# Task\n\n**Status:** blocked\n")); err != nil || status != TaskBlocked {
		t.Fatalf("status=%q err=%v", status, err)
	}
	if _, err := StatusFromBytes([]byte("# Task\n\nno status\n")); !errors.Is(err, ErrStatusLineNotFound) {
		t.Fatalf("missing status err=%v", err)
	}
}
