package cli

import (
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/spf13/cobra"
)

func NewCmdCommand(c common.Args, order string, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "cmd",
		DisableFlagsInUseLine: true,
		Short:                 "Manage environments for vela applications to run.",
		Long:                  "Manage environments for vela applications to run.",
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeStart,
		},
	}
	cmd.SetOut(ioStream.Out)
	return cmd
}
