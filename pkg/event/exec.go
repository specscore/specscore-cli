package event

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

// Exec is a Subscriber that delivers events by spawning a child process and
// piping the JSON-serialized envelope to its stdin. It enforces a wall-clock
// timeout: on expiry the command tree is terminated using platform-specific
// process-group or tree controls. The configured env mapping is appended to
// the inherited process environment (additive, not replacement).
type Exec struct {
	argv    []string
	env     map[string]string
	timeout time.Duration
}

type execProcessTree interface {
	afterStart() error
	close() error
}

// execSIGTERMGrace is the Unix wait window between SIGTERM and SIGKILL.
const execSIGTERMGrace = 100 * time.Millisecond

// cmdStdinPipeFn is a testable indirection for cmd.StdinPipe. Tests can
// replace it to simulate pipe-creation failures.
var cmdStdinPipeFn = func(cmd *exec.Cmd) (io.WriteCloser, error) {
	return cmd.StdinPipe()
}

var configureExecProcessTreeFn = configureExecProcessTree

// NewExec constructs an Exec subscriber. argv[0] is the executable and
// argv[1:] are positional arguments. env may be nil. timeout is the wall-clock
// budget for the child; the config-loader (task 6) enforces the [100, 30000]
// ms bounds — this constructor does not.
func NewExec(argv []string, env map[string]string, timeout time.Duration) *Exec {
	return &Exec{argv: argv, env: env, timeout: timeout}
}

// Name returns "exec:<argv[0]>" so the dispatcher's stderr failure log can
// identify which exec subscriber failed.
func (x *Exec) Name() string {
	if len(x.argv) == 0 {
		return "exec:"
	}
	return "exec:" + x.argv[0]
}

// Deliver runs the configured command with the event JSON piped to stdin.
// Returns *ExecTimeoutError on wall-clock timeout, *ExecExitError on non-zero
// exit, or a plain error for setup failures (serialization, pipe creation).
func (x *Exec) Deliver(ctx context.Context, e Event) error {
	if len(x.argv) == 0 {
		return errors.New("exec: empty argv")
	}

	payload, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("exec: marshal event: %w", err)
	}

	cctx, cancel := context.WithTimeout(ctx, x.timeout)
	defer cancel()

	cmd := exec.CommandContext(cctx, x.argv[0], x.argv[1:]...)
	processTree, err := configureExecProcessTreeFn(cmd)
	if err != nil {
		return fmt.Errorf("exec: configure process tree: %w", err)
	}
	defer func() {
		// Cleanup happens after Cmd.Wait has released its process handle. Its
		// error cannot change the delivery result, but must be made explicit so
		// static analysis does not mistake this intentional best-effort release.
		_ = processTree.close()
	}()

	// Additive env: inherit parent environment, then append configured pairs.
	env := os.Environ()
	for k, v := range x.env {
		env = append(env, k+"="+v)
	}
	cmd.Env = env

	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr

	stdin, err := cmdStdinPipeFn(cmd)
	if err != nil {
		return fmt.Errorf("exec: stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("exec: start: %w", err)
	}
	if err := processTree.afterStart(); err != nil {
		// Fail closed: a command without owned tree cleanup must not continue.
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("exec: initialize process tree: %w", err)
	}

	// Write the envelope and close stdin so the child sees EOF.
	if _, werr := stdin.Write(payload); werr != nil {
		// The child may have exited early; surface the write error only if
		// Wait below does not produce a more specific error.
		_ = stdin.Close()
		if waitErr := cmd.Wait(); waitErr != nil {
			return classifyWaitError(waitErr, cctx, x.timeout)
		}
		return fmt.Errorf("exec: write stdin: %w", werr)
	}
	if cerr := stdin.Close(); cerr != nil {
		if waitErr := cmd.Wait(); waitErr != nil {
			return classifyWaitError(waitErr, cctx, x.timeout)
		}
		return fmt.Errorf("exec: close stdin: %w", cerr)
	}

	if waitErr := cmd.Wait(); waitErr != nil {
		return classifyWaitError(waitErr, cctx, x.timeout)
	}
	return nil
}

// classifyWaitError converts the result of cmd.Wait into one of the typed
// errors callers (the dispatcher's stderr log) discriminate on.
func classifyWaitError(waitErr error, cctx context.Context, timeout time.Duration) error {
	if cctx.Err() == context.DeadlineExceeded {
		return &ExecTimeoutError{Timeout: timeout, Cause: waitErr}
	}
	var ee *exec.ExitError
	if errors.As(waitErr, &ee) {
		return &ExecExitError{ExitCode: ee.ExitCode(), Cause: waitErr}
	}
	return fmt.Errorf("exec: wait: %w", waitErr)
}

// ExecTimeoutError is returned when the configured wall-clock timeout elapsed
// before the child completed. It is distinguishable from *ExecExitError so the
// dispatcher's stderr log can name the failure mode.
type ExecTimeoutError struct {
	Timeout time.Duration
	Cause   error
}

func (e *ExecTimeoutError) Error() string {
	return fmt.Sprintf("exec: timeout after %s", e.Timeout)
}

func (e *ExecTimeoutError) Unwrap() error { return e.Cause }

// ExecExitError is returned when the child exited non-zero without hitting
// the timeout. ExitCode is the OS-reported exit status.
type ExecExitError struct {
	ExitCode int
	Cause    error
}

func (e *ExecExitError) Error() string {
	return fmt.Sprintf("exec: child exited with code %d", e.ExitCode)
}

func (e *ExecExitError) Unwrap() error { return e.Cause }
