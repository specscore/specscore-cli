package lifecycle

import (
	"errors"
	"testing"
)

func TestDeferredRollback(t *testing.T) {
	if got := DeferRollback(nil); got != nil {
		t.Fatalf("DeferRollback(nil) = %v, want nil", got)
	}
	cause := errors.New("post-mutation verification failed")
	deferred := DeferRollback(cause)
	if !IsDeferredRollback(deferred) {
		t.Fatalf("IsDeferredRollback(%T) = false, want true", deferred)
	}
	if !errors.Is(deferred, cause) {
		t.Fatalf("deferred error did not preserve cause: %v", deferred)
	}
	if got := deferred.Error(); got != cause.Error() {
		t.Fatalf("deferred Error() = %q, want %q", got, cause.Error())
	}
	if got := DeferRollback(deferred); got != deferred {
		t.Fatalf("DeferRollback wrapped an already-deferred error: got %T, want original %T", got, deferred)
	}
	if IsDeferredRollback(cause) {
		t.Fatal("ordinary error was identified as deferred")
	}
}
