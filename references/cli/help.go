package cli

import (
	"github.com/oam-dev/kubevela/apis/types"

	"github.com/spf13/cobra"
)

// NewHelpCommand get any command help
func NewHelpCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "help [command] ",
		DisableFlagsInUseLine: true,
		Short:                 "Help about any command",
		Run:                   RunHelp,
	}
	return cmd
}

// RunHelp exec help [command]
func RunHelp(cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		allCommands := cmd.Root().Commands()
		// print error message at first, since it can contain suggestions
		cmd.Printf("A Highly Extensible Platform Engine based on Kubernetes and Open Application Model.\n\nUsage:\n  vela [flags]\n  vela [command]\n\nAvailable Commands:\n\n")
		PrintHelpByTag(cmd, allCommands, types.TypeStart)
		PrintHelpByTag(cmd, allCommands, types.TypeApp)
		PrintHelpByTag(cmd, allCommands, types.TypeCap)
		PrintHelpByTag(cmd, allCommands, types.TypeSystem)
	} else {
		foundCmd, _, err := cmd.Root().Find(args)
		if foundCmd != nil && err == nil {
			foundCmd.HelpFunc()(cmd, args)
		}
	}
}

// PrintHelpByTag print custom defined help message
func PrintHelpByTag(cmd *cobra.Command, all []*cobra.Command, tag string) {
	cmd.Printf("  %s:\n\n", tag)
	table := newUITable()
	for _, c := range all {
		if val, ok := c.Annotations[types.TagCommandType]; ok && val == tag {
			table.AddRow("    "+c.Use, c.Long)
		}
	}
	cmd.Println(table.String())
	cmd.Println()
}

// AddTokenVarFlags adds token flag to a command
func AddTokenVarFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringP("token", "t", "", "Github Repo token")
}
