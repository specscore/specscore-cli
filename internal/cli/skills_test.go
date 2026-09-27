package cli

import (
	"bytes"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/strongo/cli-helpers/skillsync"
	skillscmd "github.com/strongo/cli-helpers/skillsync/cobracmd"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

func TestSkillsCmd_Registration(t *testing.T) {
	cmd := skillsCommand()
	if cmd.Name() != "skills" {
		t.Errorf("Name() = %q, want %q", cmd.Name(), "skills")
	}
	sub := cmd.Commands()
	hasSync := false
	for _, c := range sub {
		if c.Name() == "sync" {
			hasSync = true
			break
		}
	}
	if !hasSync {
		t.Errorf("skills command does not contain sync subcommand; subcommands = %v", sub)
	}
}

func TestSkillsCmd_RegisteredAtRoot(t *testing.T) {
	rootCmd, fangOpts := newRootCommand()
	var stdout, stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)
	rootCmd.SetArgs([]string{"skills", "--help"})

	err := executeWithPanicRecovery(rootCmd, fangOpts...)
	if err != nil {
		t.Fatalf("specscore skills --help failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "skills") {
		t.Errorf("help output does not mention skills:\n%s", stdout.String())
	}
}

func TestSkillsConfig_LoadsEmbeddedSkills(t *testing.T) {
	cfg, err := newSkillsConfig()
	if err != nil {
		t.Fatalf("newSkillsConfig failed: %v", err)
	}
	if cfg.CLI.Name != "specscore" || cfg.CLI.Publisher != "specscore" {
		t.Errorf("CLI identity = %+v, want specscore/specscore", cfg.CLI)
	}
	if len(cfg.Bundles) == 0 {
		t.Fatal("cfg.Bundles is empty")
	}
	bundle := cfg.Bundles[0]
	if bundle.Plugin.Name != "specscore" || bundle.Plugin.Publisher != "specscore" {
		t.Errorf("Bundle plugin identity = %+v, want specscore/specscore", bundle.Plugin)
	}
	if bundle.Source.Repository != "github.com/specscore/specscore-cli" {
		t.Errorf("Bundle repository = %q, want github.com/specscore/specscore-cli", bundle.Source.Repository)
	}
	if bundle.Source.Path != "ai/skills" {
		t.Errorf("Bundle path = %q, want ai/skills", bundle.Source.Path)
	}
	if bundle.Source.Digest == "" {
		t.Error("Bundle digest is empty")
	}
}

func TestSkillsSyncErrors_Failure(t *testing.T) {
	errUsage := (skillsSyncErrors{}).Failure(&skillscmd.UsageError{Err: errors.New("bad arg")})
	var codedUsage *exitcode.Error
	if !errors.As(errUsage, &codedUsage) {
		t.Fatalf("Failure(usage) did not return *exitcode.Error: %v", errUsage)
	}
	if codedUsage.ExitCode() != exitcode.InvalidArgs {
		t.Errorf("code = %d, want InvalidArgs (%d)", codedUsage.ExitCode(), exitcode.InvalidArgs)
	}

	errOther := (skillsSyncErrors{}).Failure(errors.New("sync failed"))
	var codedOther *exitcode.Error
	if !errors.As(errOther, &codedOther) {
		t.Fatalf("Failure(other) did not return *exitcode.Error: %v", errOther)
	}
	if codedOther.ExitCode() != exitcode.InvalidState {
		t.Errorf("code = %d, want InvalidState (%d)", codedOther.ExitCode(), exitcode.InvalidState)
	}
}

func TestSkillsSyncErrors_Conflict(t *testing.T) {
	report := skillsync.Report{
		Dir: "/tmp/fake-harness/skills",
		Changes: []skillsync.Change{
			{Name: "task", Action: skillsync.Conflict, Reason: "owned by someone else"},
		},
	}
	err := (skillsSyncErrors{}).Conflict(report)
	var coded *exitcode.Error
	if !errors.As(err, &coded) {
		t.Fatalf("Conflict did not return *exitcode.Error: %v", err)
	}
	if coded.ExitCode() != exitcode.Conflict {
		t.Errorf("code = %d, want Conflict (%d)", coded.ExitCode(), exitcode.Conflict)
	}
	if !strings.Contains(coded.Error(), "1 skill(s) could not be installed") {
		t.Errorf("unexpected error message: %q", coded.Error())
	}
}

type subErrFS struct{}

func (subErrFS) Open(name string) (fs.File, error) { return nil, errors.New("open error") }
func (subErrFS) Sub(dir string) (fs.FS, error)     { return nil, errors.New("sub error") }

type digestErrFS struct{}

func (digestErrFS) Open(name string) (fs.File, error) {
	return nil, errors.New("digest open error")
}

func TestSkillsConfig_Errors(t *testing.T) {
	origFS := specscoreSkillsFS
	origPlugin := specscoreSkillsPlugin
	defer func() {
		specscoreSkillsFS = origFS
		specscoreSkillsPlugin = origPlugin
	}()

	// 1. Sub error
	specscoreSkillsFS = subErrFS{}
	if _, err := newSkillsConfig(); err == nil {
		t.Error("newSkillsConfig with subErrFS expected error, got nil")
	}

	// 2. Digest error
	specscoreSkillsFS = digestErrFS{}
	if _, err := newSkillsConfig(); err == nil {
		t.Error("newSkillsConfig with digestErrFS expected error, got nil")
	}

	// 3. EmbeddedBundle error
	specscoreSkillsFS = origFS
	specscoreSkillsPlugin = skillsync.PluginIdentity{}
	if _, err := newSkillsConfig(); err == nil {
		t.Error("newSkillsConfig with empty plugin identity expected error, got nil")
	}
}

func TestSkillsCmd_ConfigError(t *testing.T) {
	origFS := specscoreSkillsFS
	defer func() {
		specscoreSkillsFS = origFS
	}()
	specscoreSkillsFS = subErrFS{}
	cmd := skillsCommand()
	if cmd.RunE == nil {
		t.Fatal("expected cmd.RunE to be set when config fails")
	}
	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected cmd.RunE to return error")
	}
}
