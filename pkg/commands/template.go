package commands

import (
	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/api/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	mycue "github.com/oam-dev/kubevela/pkg/cue"
)

func NewTemplateCommand(c types.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "template",
		DisableFlagsInUseLine: true,
		Short:                 "Manage templates",
		Long:                  "Manage templates",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
	}
	cmd.SetOut(ioStream.Out)
	cmd.AddCommand(NewTemplateContextCommand(ioStream))
	return cmd
}

func NewTemplateContextCommand(ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "context",
		DisableFlagsInUseLine: true,
		Short:                 "Show context parameters",
		Long:                  "Show context parameter",
		Example:               `vela template context`,
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ioStream.Info(mycue.BaseTemplate)
			return nil
		},
	}
	cmd.SetOut(ioStream.Out)
	return cmd
}
