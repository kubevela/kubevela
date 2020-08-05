package template

import (
	"fmt"
	"os"
	"strings"
	"text/template"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
)

var rootCommand = "rudr"

// usageTemplate used to render usage
var usageTemplate = `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
  {{.Example}}{{end}}{{if .HasAvailableSubCommands}}

Available Commands:{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasBaseCommands}}

Base Commands:{{range .BaseCommands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAdvancedCommands}}

Advanced Commands:{{range .AdvancedCommands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasWorkloadPluginCommands}}

WorkloadPlugin Commands:{{range .WorkloadPluginCommands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasTraitPluginCommands}}

TraitPlugin Commands:{{range .TraitPluginCommands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`

// usageTemplate used to render help
var helpTemplate = `{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}`

var templateFuncs = template.FuncMap{
	"trim":                    strings.TrimSpace,
	"trimRightSpace":          trimRightSpace,
	"trimTrailingWhitespaces": trimRightSpace,
	"rpad":                    rpad,
	"gt":                      cobra.Gt,
	"eq":                      cobra.Eq,
}

// Templater render usage
type Templater interface {
	UsageFunc() func(cmd *cobra.Command) error
	HelpFunc() func(*cobra.Command, []string)
	AddCommandsAndFlags()
}

type templater struct {
	// usage template
	UsageTemplate string
	// help template
	HelpTemplate string
	// toot command
	RootCmd *cobra.Command
	// base usage command
	BaseCommands []*cobra.Command
	// advanced usage command
	AdvancedCommands []*cobra.Command
	// WorkloadPlugin command
	WorkloadPluginCommands []*cobra.Command
	// TraitPlugin command
	TraitPluginCommands []*cobra.Command
	// a list of global command-line options
	OptionsCommand *cobra.Command
	// global flags used by all sub commands
	GlobalFlags *pflag.FlagSet
	// extra flags used by controller-runtime
	ioStream cmdutil.IOStreams
}

// vars for template
type templateVars struct {
	*cobra.Command
	baseCommands           []*cobra.Command
	advancedCommands       []*cobra.Command
	workloadPluginCommands []*cobra.Command
	traitPluginCommands    []*cobra.Command
	globalFlags            *pflag.FlagSet
}

// NewTemplater return templater
func NewTemplater(rootCmd *cobra.Command, baseCommands, advancedCommands, workloadPluginCommands, traitPluginCommands []*cobra.Command,
	optionsCommand *cobra.Command, globalFlags *pflag.FlagSet, ioStream cmdutil.IOStreams) Templater {
	return &templater{
		RootCmd:                rootCmd,
		ioStream:               ioStream,
		UsageTemplate:          usageTemplate,
		HelpTemplate:           helpTemplate,
		GlobalFlags:            globalFlags,
		BaseCommands:           baseCommands,
		AdvancedCommands:       advancedCommands,
		OptionsCommand:         optionsCommand,
		WorkloadPluginCommands: workloadPluginCommands,
		TraitPluginCommands:    traitPluginCommands,
	}
}

// UsageFunc render usage
func (t *templater) UsageFunc() func(cmd *cobra.Command) error {
	return func(cmd *cobra.Command) error {
		tmp := template.Must(template.New("usage").Funcs(templateFuncs).Parse(t.UsageTemplate))
		vars := &templateVars{
			Command:                cmd,
			baseCommands:           t.BaseCommands,
			advancedCommands:       t.AdvancedCommands,
			workloadPluginCommands: t.WorkloadPluginCommands,
			traitPluginCommands:    t.TraitPluginCommands,
			globalFlags:            t.GlobalFlags,
		}

		return tmp.Execute(t.ioStream.Out, vars)
	}
}

// UsageFunc render usage
func (t *templater) HelpFunc() func(*cobra.Command, []string) {
	return func(cmd *cobra.Command, args []string) {
		tmp := template.Must(template.New("help").Funcs(templateFuncs).Parse(t.HelpTemplate))
		vars := &templateVars{
			Command: cmd,
		}

		if err := tmp.Execute(t.ioStream.Out, vars); err != nil {
			t.ioStream.Errorf("Render help error: %s", err)
			os.Exit(1)
		}
	}
}

// AddCommandsAndFlags add sub commands and flags
func (t *templater) AddCommandsAndFlags() {
	t.RootCmd.AddCommand(t.BaseCommands...)
	t.RootCmd.AddCommand(t.AdvancedCommands...)
	t.RootCmd.AddCommand(t.WorkloadPluginCommands...)
	t.RootCmd.AddCommand(t.TraitPluginCommands...)
	t.RootCmd.AddCommand(t.OptionsCommand)

	flags := t.RootCmd.PersistentFlags()
	flags.AddFlagSet(t.GlobalFlags)
}

func (v *templateVars) HasAvailableSubCommands() bool {

	return len(v.Commands()) != 0
}

// Commands root command filter filtes baseCommand, advancedCommands, workloadPluginCommands,
// traitPluginCommands. child command not filter
func (v *templateVars) Commands() []*cobra.Command {
	if !v.isRoot() {
		return v.Command.Commands()
	}

	commands := []*cobra.Command{}
	baseCommandsMap := toCommandMap(v.baseCommands)
	advancedCommandsMap := toCommandMap(v.advancedCommands)
	workloadPluginCommandsMap := toCommandMap(v.workloadPluginCommands)
	traitPluginCommandsMap := toCommandMap(v.traitPluginCommands)

	for _, command := range v.Command.Commands() {
		if advancedCommandsMap[command] {
			continue
		}
		if baseCommandsMap[command] {
			continue
		}
		if workloadPluginCommandsMap[command] {
			continue
		}
		if traitPluginCommandsMap[command] {
			continue
		}
		commands = append(commands, command)
	}

	return commands
}

// HasBaseCommands BaseCommands not empty
func (v *templateVars) HasBaseCommands() bool {
	return len(v.BaseCommands()) != 0
}

// HasBaseCommands BaseCommands not empty
func (v *templateVars) BaseCommands() []*cobra.Command {
	baseCommands := []*cobra.Command{}
	if !v.isRoot() {
		return baseCommands
	}

	// the root command show base command and advanced command
	// sub command show availad command only
	baseCommands = v.baseCommands

	return baseCommands
}

// HasAdvancedCommands AdvancedCommands not empty
func (v *templateVars) HasAdvancedCommands() bool {
	return len(v.AdvancedCommands()) != 0
}

func (v *templateVars) AdvancedCommands() []*cobra.Command {
	advancedCommands := []*cobra.Command{}

	if !v.isRoot() {
		return advancedCommands
	}

	// the root command show base command and advanced command
	// sub command show availad command only
	advancedCommands = v.advancedCommands

	return advancedCommands
}

// HasWorkloadPluginCommands AdvancedCommands not empty
func (v *templateVars) HasWorkloadPluginCommands() bool {
	return len(v.WorkloadPluginCommands()) != 0
}

func (v *templateVars) WorkloadPluginCommands() []*cobra.Command {
	workloadPluginCommands := []*cobra.Command{}

	if !v.isRoot() {
		return workloadPluginCommands
	}

	// the root command show base command and advanced command
	// sub command show availad command only
	workloadPluginCommands = v.workloadPluginCommands

	return workloadPluginCommands
}

// HasTraitPluginCommands AdvancedCommands not empty
func (v *templateVars) HasTraitPluginCommands() bool {
	return len(v.TraitPluginCommands()) != 0
}

func (v *templateVars) TraitPluginCommands() []*cobra.Command {
	traitPluginCommands := []*cobra.Command{}

	if !v.isRoot() {
		return traitPluginCommands
	}

	// the root command show base command and advanced command
	// sub command show availad command only
	traitPluginCommands = v.traitPluginCommands

	return traitPluginCommands
}

// HasAvailableInheritedFlags Commands not empty after filted
// baseCommand and advancedCommands
func (v *templateVars) HasAvailableLocalFlags() bool {
	if !v.isRoot() {
		return v.Command.LocalFlags().HasAvailableFlags()
	}
	return v.LocalFlags().HasAvailableFlags()
}

// InheritedFlags filter extral flgas
func (v *templateVars) LocalFlags() *pflag.FlagSet {
	if !v.isRoot() {
		return v.Command.LocalFlags()
	}

	tmpFlagSet := pflag.NewFlagSet("tmp", pflag.ContinueOnError)
	globalFlagMap := make(map[*pflag.Flag]bool)
	v.globalFlags.VisitAll(func(flag *pflag.Flag) {
		globalFlagMap[flag] = true
	})

	v.Command.LocalFlags().VisitAll(func(flag *pflag.Flag) {
		if globalFlagMap[flag] {
			return
		}
		tmpFlagSet.AddFlag(flag)
	})

	return tmpFlagSet
}

// HasAvailableInheritedFlags Commands not empty after filted
// baseCommand and advancedCommands
func (v *templateVars) HasAvailableInheritedFlags() bool {
	return v.InheritedFlags().HasAvailableFlags()
}

// InheritedFlags filter extral flgas
func (v *templateVars) InheritedFlags() *pflag.FlagSet {
	tmpFlagSet := pflag.NewFlagSet("tmp", pflag.ContinueOnError)
	globalFlagMap := make(map[*pflag.Flag]bool)
	v.globalFlags.VisitAll(func(flag *pflag.Flag) {
		globalFlagMap[flag] = true
	})

	v.Command.InheritedFlags().VisitAll(func(flag *pflag.Flag) {
		if globalFlagMap[flag] {
			return
		}
		tmpFlagSet.AddFlag(flag)
	})

	return tmpFlagSet
}

// check current command is root command
func (v *templateVars) isRoot() bool {
	return v.Name() == rootCommand
}

func trimRightSpace(s string) string {
	return strings.TrimRightFunc(s, unicode.IsSpace)
}

// rpad adds padding to the right of a string.
func rpad(s string, padding int) string {
	template := fmt.Sprintf("%%-%ds", padding)
	return fmt.Sprintf(template, s)
}

// transform command array to map
func toCommandMap(commands []*cobra.Command) map[*cobra.Command]bool {
	commandsMap := make(map[*cobra.Command]bool)

	for _, command := range commands {
		commandsMap[command] = true
	}

	return commandsMap
}
