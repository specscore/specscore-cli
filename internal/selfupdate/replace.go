package selfupdate

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// ReplaceExecutable atomically swaps the binary at newBinaryPath into
// targetPath. The new binary is first staged to a temp file in the same
// directory as targetPath (same filesystem, so os.Rename is atomic), with
// executable permissions (0755), then renamed over the target.
//
// On POSIX, os.Rename within one directory atomically replaces the target —
// the install location is never left with a partial or truncated file: it
// holds either the original or the complete new binary. On Windows, a running
// .exe cannot be overwritten, so the existing target is first renamed aside to
// targetPath+".old" before the new binary is moved into place.
//
// Any staging error before the rename returns the error and leaves targetPath
// untouched.
func ReplaceExecutable(targetPath, newBinaryPath string) error {
	dir := filepath.Dir(targetPath)

	// Stage the new binary into a temp file in the target's directory so the
	// subsequent rename stays on the same filesystem and is atomic.
	staged, err := stage(dir, newBinaryPath)
	if err != nil {
		return err
	}

	if runtime.GOOS == "windows" {
		// A running .exe cannot be overwritten, but it can be renamed. Move the
		// current target aside, then move the new binary into place. The .old
		// copy may stay locked while the old process runs; removing it is
		// best-effort.
		old := targetPath + ".old"
		_ = os.Remove(old)
		if err := os.Rename(targetPath, old); err != nil {
			os.Remove(staged)
			return exitcode.UnexpectedErrorf("move aside current executable: %v", err)
		}
		if err := os.Rename(staged, targetPath); err != nil {
			// Best effort: restore the original so the install stays runnable.
			os.Rename(old, targetPath)
			os.Remove(staged)
			return exitcode.UnexpectedErrorf("install new executable: %v", err)
		}
		_ = os.Remove(old)
		return nil
	}

	// POSIX: a single rename atomically replaces the target.
	if err := os.Rename(staged, targetPath); err != nil {
		os.Remove(staged)
		return exitcode.UnexpectedErrorf("install new executable: %v", err)
	}
	return nil
}

// stage copies src into a new temp file in dir with 0755 permissions and
// returns its path. On any error the temp file is removed so nothing is left
// behind.
func stage(dir, src string) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", exitcode.UnexpectedErrorf("open new binary: %v", err)
	}
	defer in.Close()

	tmp, err := os.CreateTemp(dir, ".specscore-stage-*")
	if err != nil {
		return "", exitcode.UnexpectedErrorf("create staging file: %v", err)
	}
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", exitcode.UnexpectedErrorf("stage new binary: %v", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", exitcode.UnexpectedErrorf("close staging file: %v", err)
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		os.Remove(tmp.Name())
		return "", exitcode.UnexpectedErrorf("chmod staging file: %v", err)
	}
	return tmp.Name(), nil
}

// VerifyBinaryVersion runs "<path> --version" and returns an error unless the
// command output contains wantVersion. It is kept separate from
// ReplaceExecutable so the swap and the post-swap sanity check are
// independently testable.
func VerifyBinaryVersion(path, wantVersion string) error {
	out, err := exec.Command(path, "--version").CombinedOutput()
	if err != nil {
		return exitcode.UnexpectedErrorf("run %s --version: %v", path, err)
	}
	if !strings.Contains(string(out), wantVersion) {
		return exitcode.InvalidStateErrorf(
			"version check failed: %s --version did not report %q (got %q)",
			path, wantVersion, strings.TrimSpace(string(out)))
	}
	return nil
}
