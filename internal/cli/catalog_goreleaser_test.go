package cli

import (
	"os"
	"sort"
	"testing"

	"github.com/strongo/cli-helpers/cliinstall"
	"github.com/strongo/cli-helpers/selfupdate"
	"gopkg.in/yaml.v3"
)

// goreleaserConfig decodes only the .goreleaser.yml fields this consumer
// test needs to compare against the compiled-in catalog entry
// (cli-install#req:catalog-identity-single-source): archive/checksums
// naming, the supported platform matrix, and the Homebrew cask coordinates.
// It is deliberately not a full GoReleaser schema.
type goreleaserConfig struct {
	ProjectName string `yaml:"project_name"`
	Builds      []struct {
		Binary string   `yaml:"binary"`
		GOOS   []string `yaml:"goos"`
		GOARCH []string `yaml:"goarch"`
		Ignore []struct {
			GOOS   string `yaml:"goos"`
			GOARCH string `yaml:"goarch"`
		} `yaml:"ignore"`
	} `yaml:"builds"`
	Archives []struct {
		NameTemplate string `yaml:"name_template"`
	} `yaml:"archives"`
	Checksum struct {
		NameTemplate string `yaml:"name_template"`
	} `yaml:"checksum"`
	HomebrewCasks []struct {
		Name       string `yaml:"name"`
		Repository struct {
			Owner string `yaml:"owner"`
			Name  string `yaml:"name"`
		} `yaml:"repository"`
	} `yaml:"homebrew_casks"`
}

// AC: cli-install#ac:catalog-matrix-is-valid — an offline test asserting
// that specscore's own .goreleaser.yml archive name template, checksum name
// template, supported platform matrix, and Homebrew cask coordinates match
// its `strongo/cli-helpers` catalog entry, so drift here fails specscore's
// own CI rather than surfacing as a 404 or a wrong `brew` command in someone
// else's `install specscore` (cli-install#req:catalog-identity-single-source).
func TestSpecscoreCatalogEntry_MatchesGoReleaserConfig(t *testing.T) {
	t.Parallel()

	entry, ok := cliinstall.ByID("specscore")
	if !ok {
		t.Fatal(`no catalog entry for "specscore"`)
	}

	raw, err := os.ReadFile("../../.goreleaser.yml")
	if err != nil {
		t.Fatalf("read .goreleaser.yml: %v", err)
	}
	var gr goreleaserConfig
	if err := yaml.Unmarshal(raw, &gr); err != nil {
		t.Fatalf("parse .goreleaser.yml: %v", err)
	}

	if gr.ProjectName != entry.ID {
		t.Errorf("goreleaser project_name = %q, want catalog id %q", gr.ProjectName, entry.ID)
	}
	if len(gr.Builds) == 0 {
		t.Fatal(".goreleaser.yml declares no builds")
	}
	if gr.Builds[0].Binary != entry.ID {
		t.Errorf("goreleaser builds[0].binary = %q, want catalog id %q", gr.Builds[0].Binary, entry.ID)
	}
	if entry.TagPrefix != "" {
		t.Errorf("catalog TagPrefix = %q, want empty: specscore publishes one product per repository", entry.TagPrefix)
	}
	// .goreleaser.yml declares no explicit `release:` github block (GoReleaser
	// infers the repository from the git remote), so this asserts the
	// catalog's Repository against the known literal directly — the same
	// value selfUpdateConfig hardcoded before this migration.
	const wantRepository = "specscore/specscore-cli"
	if entry.Repository != wantRepository {
		t.Errorf("catalog Repository = %q, want %q", entry.Repository, wantRepository)
	}

	// Checksum naming: the catalog entry leaves ChecksumsName nil, meaning
	// "use the library's own GoReleaser-shaped default"
	// (<binary>_<version>_checksums.txt) — verify .goreleaser.yml's own
	// template is exactly that shape.
	wantChecksumTemplate := entry.ID + "_{{ .Version }}_checksums.txt"
	if gr.Checksum.NameTemplate != wantChecksumTemplate {
		t.Errorf(".goreleaser.yml checksum.name_template = %q, want %q", gr.Checksum.NameTemplate, wantChecksumTemplate)
	}
	if entry.ChecksumsName != nil {
		t.Error("catalog entry.ChecksumsName is overridden; want nil (the shared GoReleaser default matches .goreleaser.yml)")
	}

	// Archive naming: the catalog entry leaves AssetName nil for the same
	// reason — verify .goreleaser.yml's own template matches the library's
	// default shape (<binary>_<version>_<os>_<arch>).
	if len(gr.Archives) == 0 {
		t.Fatal(".goreleaser.yml declares no archives")
	}
	wantArchiveTemplate := entry.ID + "_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
	if gr.Archives[0].NameTemplate != wantArchiveTemplate {
		t.Errorf(".goreleaser.yml archives[0].name_template = %q, want %q", gr.Archives[0].NameTemplate, wantArchiveTemplate)
	}
	if entry.AssetName != nil {
		t.Error("catalog entry.AssetName is overridden; want nil (the shared GoReleaser default matches .goreleaser.yml)")
	}

	// Platform matrix: builds[0].goos x goarch, minus builds[0].ignore, must
	// equal entry.SupportedPlatforms exactly (order-independent).
	build := gr.Builds[0]
	ignored := make(map[[2]string]bool, len(build.Ignore))
	for _, ig := range build.Ignore {
		ignored[[2]string{ig.GOOS, ig.GOARCH}] = true
	}
	var goreleaserPlatforms []selfupdate.Platform
	for _, goos := range build.GOOS {
		for _, goarch := range build.GOARCH {
			if ignored[[2]string{goos, goarch}] {
				continue
			}
			goreleaserPlatforms = append(goreleaserPlatforms, selfupdate.Platform{GOOS: goos, GOARCH: goarch})
		}
	}
	if got, want := sortedPlatforms(goreleaserPlatforms), sortedPlatforms(entry.SupportedPlatforms); !platformsEqual(got, want) {
		t.Errorf("goreleaser platform matrix = %v, want catalog entry.SupportedPlatforms %v", got, want)
	}

	// Homebrew cask coordinates: entry.CaskToken ("<owner>/tap/<cask-name>",
	// the tap-abbreviated form brew resolves) must be built from the SAME
	// owner and cask name .goreleaser.yml's homebrew_casks stanza publishes,
	// whose repository.name is the full "homebrew-<tap>" the abbreviation
	// stands for.
	if len(gr.HomebrewCasks) == 0 {
		t.Fatal(".goreleaser.yml declares no homebrew_casks")
	}
	cask := gr.HomebrewCasks[0]
	wantCaskToken := cask.Repository.Owner + "/tap/" + cask.Name
	if entry.CaskToken != wantCaskToken {
		t.Errorf("catalog CaskToken = %q, want %q (derived from .goreleaser.yml homebrew_casks[0])", entry.CaskToken, wantCaskToken)
	}
	if cask.Repository.Name != "homebrew-tap" {
		t.Errorf(".goreleaser.yml homebrew_casks[0].repository.name = %q, want %q (the tap CaskToken's \"tap\" segment abbreviates)", cask.Repository.Name, "homebrew-tap")
	}
	wantCaskOS := []string{"darwin", "linux"}
	gotCaskOS := append([]string(nil), entry.CaskOS...)
	sort.Strings(gotCaskOS)
	if len(gotCaskOS) != len(wantCaskOS) {
		t.Fatalf("catalog CaskOS = %v, want %v", entry.CaskOS, wantCaskOS)
	}
	for i, os := range wantCaskOS {
		if gotCaskOS[i] != os {
			t.Errorf("catalog CaskOS = %v, want %v", entry.CaskOS, wantCaskOS)
			break
		}
	}
}

func sortedPlatforms(in []selfupdate.Platform) []selfupdate.Platform {
	out := append([]selfupdate.Platform(nil), in...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].GOOS != out[j].GOOS {
			return out[i].GOOS < out[j].GOOS
		}
		return out[i].GOARCH < out[j].GOARCH
	})
	return out
}

func platformsEqual(a, b []selfupdate.Platform) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
