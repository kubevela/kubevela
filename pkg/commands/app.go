package commands

import (
	"github.com/oam-dev/kubevela/api/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"

	"github.com/spf13/cobra"
)

func NewAppsCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "app",
		DisableFlagsInUseLine: true,
		Short:                 "Manage applications",
		Long:                  "Manage applications with ls, show, delete, run",
		Example:               `vela app <command>`,
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
	}

	cmd.AddCommand(NewAppListCommand(c, ioStreams),
		NewAppStatusCommand(c, ioStreams),
		NewDeleteCommand(c, ioStreams),
		NewAppShowCommand(ioStreams),
		NewRunCommand(c, ioStreams))
	return cmd
}
