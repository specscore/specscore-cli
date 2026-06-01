package selfupdate

// UpgradeCommand returns the exact upgrade command a user should run for a
// package-manager-managed install, and true when m is a recognized manager.
// For ManagerNone or any unknown manager it returns ("", false).
func UpgradeCommand(m Manager) (string, bool) {
	switch m {
	case Homebrew:
		return "brew upgrade specscore", true
	case Scoop:
		return "scoop update specscore", true
	case WinGet:
		return "winget upgrade SpecScore.CLI", true
	default:
		return "", false
	}
}

// ManagerName returns a human-readable name for a manager, for display in the
// redirect message.
func ManagerName(m Manager) string {
	switch m {
	case Homebrew:
		return "Homebrew"
	case Scoop:
		return "Scoop"
	case WinGet:
		return "WinGet"
	default:
		return "unknown"
	}
}
