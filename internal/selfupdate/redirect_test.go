package selfupdate

import "testing"

func TestUpgradeCommand(t *testing.T) {
	cases := []struct {
		name    string
		manager Manager
		want    string
		wantOK  bool
	}{
		{"homebrew", Homebrew, "brew upgrade specscore", true},
		{"scoop", Scoop, "scoop update specscore", true},
		{"winget", WinGet, "winget upgrade SpecScore.CLI", true},
		{"none", ManagerNone, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := UpgradeCommand(c.manager)
			if got != c.want || ok != c.wantOK {
				t.Errorf("UpgradeCommand(%v) = (%q, %v); want (%q, %v)",
					c.manager, got, ok, c.want, c.wantOK)
			}
		})
	}
}
