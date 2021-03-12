package cli

import (
	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/apis/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/common"
	"github.com/oam-dev/kubevela/references/plugins"
)

// NewWorkloadsCommand creates `workloads` command
func NewWorkloadsCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "workloads",
		DisableFlagsInUseLine: true,
		Short:                 "List workloads",
		Long:                  "List workloads",
		Example:               `vela workloads`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := GetEnv(cmd)
			if err != nil {
				return err
			}
			return printWorkloadList(env.Namespace, c, ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCap,
		},
	}
	cmd.SetOut(ioStreams.Out)
	return cmd
}

func printWorkloadList(userNamespace string, c types.Args, ioStreams cmdutil.IOStreams) error {
	def, err := common.ListRawWorkloadDefinitions(userNamespace, c)
	if err != nil {
		return err
	}
	table := newUITable()
	table.AddRow("NAME", "NAMESPACE", "WORKLOAD", "DESCRIPTION")
	for _, r := range def {
		table.AddRow(r.Name, r.Namespace, r.Spec.Reference.Name, plugins.GetDescription(r.Annotations))
	}
	ioStreams.Info(table.String())
	return nil
}
