package selfupdate

import (
	"os"
	"path/filepath"
	"strings"
)

// Manager identifies a package manager that owns a managed install.
type Manager int

const (
	// ManagerNone means no package manager was detected.
	ManagerNone Manager = iota
	// Homebrew is the macOS/Linux Homebrew package manager.
	Homebrew
	// Scoop is the Windows Scoop package manager.
	Scoop
	// WinGet is the Windows Package Manager.
	WinGet
)

// InstallMethod classifies how the running binary was installed.
type InstallMethod int

const (
	// Managed means a package manager owns the binary; self-update is not eligible.
	Managed InstallMethod = iota
	// Manual means the binary was placed by the user (release archive or go install);
	// self-replace is eligible.
	Manual
	// Ambiguous means the location does not clearly indicate either method.
	Ambiguous
)

// Detection is the result of classifying an executable path.
type Detection struct {
	Method  InstallMethod
	Manager Manager
}

// Classify decides the install method purely from execPath. It is case-insensitive
// and treats both '/' and '\' as path separators so Windows paths can be tested on
// any host.
func Classify(execPath string) Detection {
	// Normalize: lowercase and unify separators to '/'.
	p := strings.ToLower(execPath)
	p = strings.ReplaceAll(p, `\`, "/")

	// Managed: Homebrew.
	if strings.Contains(p, "/cellar/") ||
		strings.Contains(p, "/homebrew/") ||
		strings.Contains(p, "/linuxbrew/") {
		return Detection{Method: Managed, Manager: Homebrew}
	}

	// Managed: Scoop.
	if strings.Contains(p, "/scoop/apps/") || strings.Contains(p, "/scoop/shims/") {
		return Detection{Method: Managed, Manager: Scoop}
	}

	// Managed: WinGet (under Microsoft/WinGet in LocalAppData).
	if strings.Contains(p, "/microsoft/winget/packages/") ||
		strings.Contains(p, "/microsoft/winget/links/") {
		return Detection{Method: Managed, Manager: WinGet}
	}

	// Manual: go install targets.
	if strings.Contains(p, "/go/bin/") {
		return Detection{Method: Manual, Manager: ManagerNone}
	}

	// Manual: release-archive / home bin locations (binary directly under a bin/).
	dir := p
	if i := strings.LastIndex(p, "/"); i >= 0 {
		dir = p[:i]
	}
	if strings.HasSuffix(dir, "/bin") {
		return Detection{Method: Manual, Manager: ManagerNone}
	}

	return Detection{Method: Ambiguous, Manager: ManagerNone}
}

// Test seams: overridable indirections for os/filepath calls so DetectSelf's
// error and symlink-fallback branches are exercisable without real symlinks.
var (
	osExecutable     = os.Executable
	evalSymlinksFunc = filepath.EvalSymlinks
)

// DetectSelf resolves the running executable's path (following symlinks when
// possible) and classifies it.
func DetectSelf() (Detection, error) {
	exe, err := osExecutable()
	if err != nil {
		return Detection{}, err
	}
	resolved, err := evalSymlinksFunc(exe)
	if err != nil {
		resolved = exe
	}
	return Classify(resolved), nil
}
