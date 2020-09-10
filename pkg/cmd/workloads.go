package cmd

import (
	"github.com/cloud-native-application/rudrx/api/types"
	"github.com/cloud-native-application/rudrx/pkg/plugins"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
)

func NewWorkloadsCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "workloads",
		DisableFlagsInUseLine: true,
		Short:                 "List workloads",
		Long:                  "List workloads",
		Example:               `vela workloads`,
		RunE: func(cmd *cobra.Command, args []string) error {
			workloads, err := plugins.LoadInstalledCapabilityWithType(types.TypeWorkload)
			if err != nil {
				return err
			}
			return printWorkloadList(workloads, ioStreams)
		},
	}
	cmd.SetOut(ioStreams.Out)
	return cmd
}

func printWorkloadList(workloadList []types.Capability, ioStreams cmdutil.IOStreams) error {
	table := uitable.New()
	table.MaxColWidth = 60
	table.AddRow("NAME", "DEFINITION")
	for _, r := range workloadList {
		table.AddRow(r.Name, r.CrdName)
	}
	ioStreams.Info(table.String())
	return nil
}
