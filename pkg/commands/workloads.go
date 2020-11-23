//nolint:golint
// TODO add lint back
package commands

import (
	"context"

	"github.com/oam-dev/kubevela/api/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/plugins"

	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
)

func NewWorkloadsCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	var syncCluster bool
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:                   "workloads",
		DisableFlagsInUseLine: true,
		Short:                 "List workloads",
		Long:                  "List workloads",
		Example:               `vela workloads`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if syncCluster {
				if err := RefreshDefinitions(ctx, c, ioStreams, true); err != nil {
					return err
				}
			}
			workloads, err := plugins.LoadInstalledCapabilityWithType(types.TypeWorkload)
			if err != nil {
				return err
			}
			return printWorkloadList(workloads, ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCap,
		},
	}
	cmd.SetOut(ioStreams.Out)
	cmd.Flags().BoolVarP(&syncCluster, "sync", "s", true, "Synchronize capabilities from cluster into local")
	return cmd
}

func printWorkloadList(workloadList []types.Capability, ioStreams cmdutil.IOStreams) error {
	table := uitable.New()
	table.AddRow("NAME", "DESCRIPTION")
	for _, r := range workloadList {
		table.AddRow(r.Name, r.Description)
	}
	ioStreams.Info(table.String())
	return nil
}
