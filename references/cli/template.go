package cli

import (
	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/apis/types"
	mycue "github.com/oam-dev/kubevela/pkg/cue"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

// NewTemplateCommand creates `template` command and its nested children command
func NewTemplateCommand(ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "template",
		DisableFlagsInUseLine: true,
		Short:                 "Manage templates",
		Long:                  "Manage templates",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCap,
		},
	}
	cmd.SetOut(ioStream.Out)
	cmd.AddCommand(NewTemplateContextCommand(ioStream))
	return cmd
}

// NewTemplateContextCommand creates `context` command
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
