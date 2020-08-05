package template

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gotest.tools/assert"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
)

var (
	commandRunE = func(cmd *cobra.Command, args []string) error {
		return nil
	}
	commandRun      = func(cmd *cobra.Command, args []string) {}
	testRudrCommand = &cobra.Command{
		Use:          "rudr",
		Short:        "rudr is a command-line tool to use OAM based micro-app engine.",
		Long:         "rudr is a command-line tool to use OAM based micro-app engine.",
		Run:          commandRun,
		SilenceUsage: true,
	}
	testRootCommand = &cobra.Command{
		Use:          "root",
		Short:        "root command.",
		Long:         "root command long.",
		SilenceUsage: true,
	}
	testSubCommand = &cobra.Command{
		Use:   "sub",
		Short: "sub command.",
		Long:  "sub command long.",
		RunE:  commandRunE,
	}
	testOptionsCommand = &cobra.Command{
		Use:   "options",
		Short: "options command.",
		Long:  "options command long.",
		RunE:  commandRunE,
	}
	testBaseCommands = []*cobra.Command{
		&cobra.Command{
			Use:   "base",
			Short: "base command.",
			Long:  "base command long.",
			RunE:  commandRunE,
		},
	}
	testAdvancedCommands = []*cobra.Command{
		&cobra.Command{
			Use:   "advanced",
			Short: "advanced command.",
			Long:  "advanced command long.",
			RunE:  commandRunE,
		},
	}
	testWorkloadPluginCommands = []*cobra.Command{
		&cobra.Command{
			Use:   "workloadPlugin",
			Short: "workloadPlugin command.",
			Long:  "workloadPlugin command long.",
			RunE:  commandRunE,
		},
	}
	testTraitPluginCommands = []*cobra.Command{
		&cobra.Command{
			Use:   "traitPlugin",
			Short: "traitPlugin command.",
			Long:  "traitPlugin command long.",
			RunE:  commandRunE,
		},
	}
	testGlobalFlags = pflag.NewFlagSet("global", pflag.ContinueOnError)
)

func TestTemplateVars(t *testing.T) {
	// prepare
	testRudrCommand.ResetCommands()
	testRudrCommand.ResetFlags()
	testRootCommand.ResetCommands()
	testRootCommand.ResetFlags()

	testGlobalFlags.StringP("testFlag", "t", "", "this is a test flag.")
	testRudrCommand.AddCommand(testSubCommand)
	testRootCommand.AddCommand(testSubCommand)

	rudrTemplate := &templateVars{
		Command:                testRudrCommand,
		baseCommands:           testBaseCommands,
		advancedCommands:       testAdvancedCommands,
		workloadPluginCommands: testWorkloadPluginCommands,
		traitPluginCommands:    testTraitPluginCommands,
		globalFlags:            testGlobalFlags,
	}
	subTemplate := &templateVars{
		Command:                testRootCommand,
		baseCommands:           testBaseCommands,
		advancedCommands:       testAdvancedCommands,
		workloadPluginCommands: testWorkloadPluginCommands,
		traitPluginCommands:    testTraitPluginCommands,
		globalFlags:            testGlobalFlags,
	}

	assert.Equal(t, true, rudrTemplate.isRoot(), "Expected teplate is a root command.")
	assert.Equal(t, false, subTemplate.isRoot(), "Expected teplate is not a root command.")
	assert.Equal(t, 1, len(rudrTemplate.Commands()), "Expected has one command")
	assert.Equal(t, 1, len(subTemplate.Commands()), "Expected has one command")

	testRudrCommand.AddCommand(testBaseCommands...)
	testRootCommand.AddCommand(testBaseCommands...)
	assert.Equal(t, 1, len(rudrTemplate.Commands()), "Expected Root command filter base commands, has one command")
	assert.Equal(t, true, rudrTemplate.HasBaseCommands(), "Expected has base command")
	assert.Equal(t, 1, len(rudrTemplate.BaseCommands()), "Expected has one base command")
	assert.Equal(t, 2, len(subTemplate.Commands()), "Expected Sub command doesn't filter base commands, Has two commands")
	assert.Equal(t, false, subTemplate.HasBaseCommands(), "Expected has no base command")

	testRudrCommand.AddCommand(testAdvancedCommands...)
	testRootCommand.AddCommand(testAdvancedCommands...)
	assert.Equal(t, 1, len(rudrTemplate.Commands()), "Expected Root command filter Advanced commands, has one command")
	assert.Equal(t, true, rudrTemplate.HasAdvancedCommands(), "Expected has Advanced command")
	assert.Equal(t, 1, len(rudrTemplate.AdvancedCommands()), "Expected has one Advanced command")
	assert.Equal(t, 3, len(subTemplate.Commands()), "Expected Sub command doesn't filter Advanced commands, has three commands")
	assert.Equal(t, false, subTemplate.HasAdvancedCommands(), "Expected has no Advanced command")

	testRudrCommand.AddCommand(testWorkloadPluginCommands...)
	testRootCommand.AddCommand(testWorkloadPluginCommands...)
	assert.Equal(t, 1, len(rudrTemplate.Commands()), "Expected Root command filter WorkloadPlugin commands, has one command")
	assert.Equal(t, true, rudrTemplate.HasWorkloadPluginCommands(), "Expected has WorkloadPlugin command")
	assert.Equal(t, 1, len(rudrTemplate.WorkloadPluginCommands()), "Expected has one WorkloadPlugin command")
	assert.Equal(t, 4, len(subTemplate.Commands()), "Expected Sub command filter WorkloadPlugin commands, has four commands")
	assert.Equal(t, false, subTemplate.HasWorkloadPluginCommands(), "Expected has no WorkloadPlugin command")

	testRudrCommand.AddCommand(testTraitPluginCommands...)
	testRootCommand.AddCommand(testTraitPluginCommands...)
	assert.Equal(t, 1, len(rudrTemplate.Commands()), "Expected Root command filter TraitPlugin commands, has one command")
	assert.Equal(t, true, rudrTemplate.HasTraitPluginCommands(), "Expected has TraitPlugin command")
	assert.Equal(t, 1, len(rudrTemplate.TraitPluginCommands()), "Expected has one TraitPlugin command")
	assert.Equal(t, 5, len(subTemplate.Commands()), "Expected Sub command filter TraitPlugin commands, has five commands")
	assert.Equal(t, false, subTemplate.HasTraitPluginCommands(), "Expected has no TraitPlugin command")

	rootFlags := testRudrCommand.PersistentFlags()
	subFlags := testRootCommand.PersistentFlags()
	rootFlags.AddFlagSet(testGlobalFlags)
	subFlags.StringP("rudrFlag", "r", "", "this is a rudr flag.")
	subFlags.AddFlagSet(testGlobalFlags)

	assert.Equal(t, false, rudrTemplate.HasAvailableLocalFlags(), "Expected has no flag")
	assert.Equal(t, true, subTemplate.HasAvailableLocalFlags(), "Expected has flags")

	rootFlagUsage := ``
	subFlagUsage := `  -r, --rudrFlag string   this is a rudr flag.
  -t, --testFlag string   this is a test flag.
`
	assert.Equal(t, rootFlagUsage, rudrTemplate.LocalFlags().FlagUsages(), "Expected root command's flag usage is: %s, actual is: %s",
		rootFlagUsage, rudrTemplate.LocalFlags().FlagUsages())
	assert.Equal(t, subFlagUsage, subTemplate.LocalFlags().FlagUsages(), "Expected sub command's flag usage is: %s, actual is: %s",
		subFlagUsage, subTemplate.LocalFlags().FlagUsages())
}

func TestTemplater(t *testing.T) {
	// prepare
	testRudrCommand.ResetCommands()
	testRudrCommand.ResetFlags()
	testRootCommand.ResetCommands()
	testRootCommand.ResetFlags()

	iostream, _, outPut, _ := cmdutil.NewTestIOStreams()
	rudrTemplate := NewTemplater(testRudrCommand, testBaseCommands, testAdvancedCommands, testWorkloadPluginCommands,
		testTraitPluginCommands, testOptionsCommand, testGlobalFlags, iostream)
	subTemplate := NewTemplater(testRootCommand, testBaseCommands, testAdvancedCommands, testWorkloadPluginCommands,
		testTraitPluginCommands, testOptionsCommand, testGlobalFlags, iostream)
	expectedRootCommandUsage := `rudr is a command-line tool to use OAM based micro-app engine.

Usage:
  rudr [flags]
  rudr [command]

Available Commands:
  help           Help about any command
  options        options command.

Base Commands:
  base           base command.

Advanced Commands:
  advanced       advanced command.

WorkloadPlugin Commands:
  workloadPlugin workloadPlugin command.

TraitPlugin Commands:
  traitPlugin    traitPlugin command.

Flags:
  -h, --help   help for rudr

Use "rudr [command] --help" for more information about a command.
`
	expectedSubCommandUsage := `root command long.

Usage:
  root [command]

Available Commands:
  advanced       advanced command.
  base           base command.
  help           Help about any command
  options        options command.
  traitPlugin    traitPlugin command.
  workloadPlugin workloadPlugin command.

Flags:
  -h, --help              help for root
  -t, --testFlag string   this is a test flag.

Use "root [command] --help" for more information about a command.
`

	rudrTemplate.AddCommandsAndFlags()
	subTemplate.AddCommandsAndFlags()

	testRudrCommand.SetUsageFunc(rudrTemplate.UsageFunc())
	testRudrCommand.SetHelpFunc(rudrTemplate.HelpFunc())

	testRudrCommand.SetArgs([]string{"-h"})
	testRudrCommand.Execute()
	assert.Equal(t, expectedRootCommandUsage, outPut.String(), "Expected root command help is:\n%s, actual is:\n%s",
		expectedRootCommandUsage, outPut.String())

	outPut.Reset()
	testRootCommand.SetUsageFunc(rudrTemplate.UsageFunc())
	testRootCommand.SetHelpFunc(rudrTemplate.HelpFunc())

	testRootCommand.SetArgs([]string{"-h"})
	testRootCommand.Execute()
	assert.Equal(t, expectedSubCommandUsage, outPut.String(), "Expected help is:\n%s, actual is:\n%s",
		expectedSubCommandUsage, outPut.String())
}
