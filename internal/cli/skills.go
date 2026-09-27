package cli

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/spf13/cobra"
	"github.com/strongo/cli-helpers/skillsync"
	skillscmd "github.com/strongo/cli-helpers/skillsync/cobracmd"
	"github.com/strongo/cli-helpers/skillsync/githubrelease"

	"github.com/specscore/specscore-cli/ai"
	"github.com/specscore/specscore-cli/pkg/exitcode"
)

const (
	specscoreSkillsPluginVersion = "0.0.0"
	specscoreSkillsUnknownSource = "0000000000000000000000000000000000000000"
)

var (
	specscoreSkillsCLI          = skillsync.Identity{Publisher: "specscore", Name: "specscore"}
	specscoreSkillsPlugin       = skillsync.PluginIdentity{Publisher: "specscore", Name: "specscore"}
	specscoreSkillsFS     fs.FS = ai.SkillsFS
)

func newSkillsConfig() (skillsync.Config, error) {
	source, err := fs.Sub(specscoreSkillsFS, "skills")
	if err != nil {
		return skillsync.Config{}, err
	}
	digest, err := skillsync.Digest(source)
	if err != nil {
		return skillsync.Config{}, err
	}
	revision := buildInfo.Commit
	if len(revision) != 40 {
		revision = specscoreSkillsUnknownSource
	}
	pluginVersion := buildInfo.Version
	if _, err := skillsync.CompareVersions(pluginVersion, pluginVersion); err != nil {
		pluginVersion = specscoreSkillsPluginVersion
	}
	bundle, err := skillsync.EmbeddedBundle(skillsync.BundleDescriptor{
		Plugin: specscoreSkillsPlugin,
		Source: skillsync.Source{
			Repository: "github.com/specscore/specscore-cli",
			Path:       "ai/skills",
			Revision:   revision,
			Version:    pluginVersion,
			Digest:     digest,
		},
	}, source)
	if err != nil {
		return skillsync.Config{}, err
	}
	return skillsync.Config{
		CLI:            specscoreSkillsCLI,
		CurrentVersion: buildInfo.Version,
		Bundles:        []skillsync.Bundle{bundle},
	}, nil
}

func skillsCommand() *cobra.Command {
	cfg, cfgErr := newSkillsConfig()
	options := skillscmd.CommandOptions{
		Use:    "skills",
		Short:  "Install SpecScore's Agent Skills into a harness's skills directory",
		Errors: skillsSyncErrors{},
		Resolver: skillsync.ReleaseResolver{
			Source:         githubrelease.Source{},
			CurrentVersion: cfg.CurrentVersion,
		},
	}
	cmd := skillscmd.New(cfg, options)
	cmd.Long = `Install SpecScore's Agent Skills into a harness's skills directory.

SpecScore ships agent-facing skills under ai/skills/, and harnesses (Claude Code, Cursor, Codex)
auto-discover them once synchronized.

'specscore skills sync' copies every shipped skill into each present harness's skills directory.`
	if cfgErr != nil {
		cmd.RunE = func(*cobra.Command, []string) error {
			return skillsSyncErrors{}.Failure(fmt.Errorf("prepare embedded SpecScore skills: %w", cfgErr))
		}
	}
	return cmd
}

type skillsSyncErrors struct{}

func (skillsSyncErrors) Failure(err error) error {
	var usage *skillscmd.UsageError
	if errors.As(err, &usage) {
		return exitcode.InvalidArgsErrorf("%v", err)
	}
	return exitcode.InvalidStateErrorf("skills: %v", err)
}

func (skillsSyncErrors) Conflict(report skillsync.Report) error {
	return exitcode.ConflictErrorf(
		"skills: %d skill(s) could not be installed because another plugin or an unmanaged directory already owns the name; see %s",
		len(report.Names(skillsync.Conflict)), report.Dir)
}
