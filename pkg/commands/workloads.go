package commands

import (
	"context"

	"github.com/oam-dev/kubevela/api/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/plugins"

	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
				newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
				if err != nil {
					return err
				}
				if err := RefreshDefinitions(ctx, newClient, ioStreams); err != nil {
					return err
				}
				ioStreams.Info("\nListing workload capabilities ...\n")
			}
			workloads, err := plugins.LoadInstalledCapabilityWithType(types.TypeWorkload)
			if err != nil {
				return err
			}
			return printWorkloadList(workloads, ioStreams)
		},
	}
	cmd.SetOut(ioStreams.Out)
	cmd.Flags().BoolVarP(&syncCluster, "sync", "s", true, "Synchronize capabilities from cluster into local")
	return cmd
}

func printWorkloadList(workloadList []types.Capability, ioStreams cmdutil.IOStreams) error {
	table := uitable.New()
	table.MaxColWidth = 60
	table.AddRow("NAME", "DESCRIPTION")
	for _, r := range workloadList {
		table.AddRow(r.Name, r.Description)
	}
	ioStreams.Info(table.String())
	return nil
}
