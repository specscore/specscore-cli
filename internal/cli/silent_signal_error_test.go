package cli

import (
	"bytes"
	"errors"
	"testing"

	"charm.land/fang/v2"
	"github.com/specscore/specscore-cli/pkg/exitcode"
)

func TestSilentSignalErrorHandler(t *testing.T) {
	styles := fang.Styles{}

	// Pure exit-code signal (empty-message exitcode.Error) → render nothing.
	var b bytes.Buffer
	silentSignalErrorHandler(&b, styles, exitcode.New(10, ""))
	if b.Len() != 0 {
		t.Errorf("empty-message exit-code signal must render nothing, got %q", b.String())
	}

	// exitcode.Error with a real message → renders (delegates to default).
	b.Reset()
	silentSignalErrorHandler(&b, styles, exitcode.New(4, "something broke"))
	if b.Len() == 0 {
		t.Error("a real-message error must be rendered")
	}

	// A plain error → renders.
	b.Reset()
	silentSignalErrorHandler(&b, styles, errors.New("plain failure"))
	if b.Len() == 0 {
		t.Error("a plain error must be rendered")
	}
}
