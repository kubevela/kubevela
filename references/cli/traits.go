package cli

import (
	"context"
	"strings"

	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/apis/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/common"
)

// NewTraitsCommand creates `traits` command
func NewTraitsCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	var workloadName string
	var enforceRefresh bool
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:                   "traits [--apply-to WORKLOAD_NAME]",
		DisableFlagsInUseLine: true,
		Short:                 "List traits",
		Long:                  "List traits",
		Example:               `vela traits`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {

			if err := RefreshDefinitions(ctx, c, ioStreams, true, enforceRefresh); err != nil {
				return err
			}

			return printTraitList(&workloadName, ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCap,
		},
	}

	cmd.SetOut(ioStreams.Out)
	cmd.Flags().StringVar(&workloadName, "apply-to", "", "Workload name")
	cmd.Flags().BoolVarP(&enforceRefresh, "", "r", false, "Enforce refresh from cluster even if cache is not expired")
	return cmd
}

func printTraitList(workloadName *string, ioStreams cmdutil.IOStreams) error {
	table := newUITable()
	table.MaxColWidth = 120
	table.Wrap = true
	traitDefinitionList, err := common.ListTraitDefinitions(workloadName)
	if err != nil {
		return err
	}
	table.AddRow("NAME", "DESCRIPTION", "APPLIES TO")
	for _, t := range traitDefinitionList {
		table.AddRow(t.Name, t.Description, strings.Join(t.AppliesTo, "\n"))
	}
	ioStreams.Info(table.String())
	return nil
}
