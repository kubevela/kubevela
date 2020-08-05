package cmd

import (
	"fmt"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// NewOptionsCommand return bind command
func NewOptionsCommand(flags *pflag.FlagSet, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "options",
		Short: "Displays rudr global options",
		Args:  cobra.ExactArgs(0),
	}

	cmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		ioStreams.Info(fmt.Sprintf("The following options can be passed to any command:\n %s", flags.FlagUsages()))
	})

	return cmd
}
