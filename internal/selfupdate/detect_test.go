package selfupdate

import "testing"

func TestClassify(t *testing.T) {
	tests := []struct {
		name        string
		execPath    string
		wantMethod  InstallMethod
		wantManager Manager
	}{
		{
			name:        "manual is eligible: release archive in /usr/local/bin",
			execPath:    "/usr/local/bin/specscore",
			wantMethod:  Manual,
			wantManager: ManagerNone,
		},
		{
			name:        "manual is eligible: go install target under go/bin",
			execPath:    "/home/u/go/bin/specscore",
			wantMethod:  Manual,
			wantManager: ManagerNone,
		},
		{
			name:        "manual: home bin",
			execPath:    "/home/u/bin/specscore",
			wantMethod:  Manual,
			wantManager: ManagerNone,
		},
		{
			name:        "homebrew apple silicon cellar",
			execPath:    "/opt/homebrew/Cellar/specscore/0.6.0/bin/specscore",
			wantMethod:  Managed,
			wantManager: Homebrew,
		},
		{
			name:        "homebrew intel cellar",
			execPath:    "/usr/local/Cellar/specscore/0.6.0/bin/specscore",
			wantMethod:  Managed,
			wantManager: Homebrew,
		},
		{
			name:        "linuxbrew prefix",
			execPath:    "/home/linuxbrew/.linuxbrew/bin/specscore",
			wantMethod:  Managed,
			wantManager: Homebrew,
		},
		{
			name:        "scoop apps",
			execPath:    `C:\Users\u\scoop\apps\specscore\current\specscore.exe`,
			wantMethod:  Managed,
			wantManager: Scoop,
		},
		{
			name:        "scoop shims",
			execPath:    `C:\Users\u\scoop\shims\specscore.exe`,
			wantMethod:  Managed,
			wantManager: Scoop,
		},
		{
			name:        "winget packages",
			execPath:    `C:\Users\u\AppData\Local\Microsoft\WinGet\Packages\SpecScore.CLI_abc\specscore.exe`,
			wantMethod:  Managed,
			wantManager: WinGet,
		},
		{
			name:        "winget links",
			execPath:    `C:\Users\u\AppData\Local\Microsoft\WinGet\Links\specscore.exe`,
			wantMethod:  Managed,
			wantManager: WinGet,
		},
		{
			name:        "ambiguous unrecognized path",
			execPath:    "/tmp/random/specscore",
			wantMethod:  Ambiguous,
			wantManager: ManagerNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.execPath)
			if got.Method != tt.wantMethod {
				t.Errorf("Classify(%q).Method = %v, want %v", tt.execPath, got.Method, tt.wantMethod)
			}
			if got.Manager != tt.wantManager {
				t.Errorf("Classify(%q).Manager = %v, want %v", tt.execPath, got.Manager, tt.wantManager)
			}
		})
	}
}

func TestClassifyNeverAmbiguousForClearCases(t *testing.T) {
	clear := []string{
		"/usr/local/bin/specscore",
		"/home/u/go/bin/specscore",
		"/opt/homebrew/Cellar/specscore/0.6.0/bin/specscore",
		`C:\Users\u\scoop\apps\specscore\current\specscore.exe`,
		`C:\Users\u\AppData\Local\Microsoft\WinGet\Packages\SpecScore.CLI_abc\specscore.exe`,
	}
	for _, p := range clear {
		if got := Classify(p); got.Method == Ambiguous {
			t.Errorf("Classify(%q) returned Ambiguous for a clearly-classified path", p)
		}
	}
}
