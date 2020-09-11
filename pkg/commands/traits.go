package commands

import (
	"strings"

	"github.com/oam-dev/kubevela/pkg/oam"

	"github.com/gosuri/uitable"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/spf13/cobra"
)

func NewTraitsCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	var workloadName string
	cmd := &cobra.Command{
		Use:                   "traits [--apply-to WORKLOADNAME]",
		DisableFlagsInUseLine: true,
		Short:                 "List traits",
		Long:                  "List traits",
		Example:               `vela traits`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return printTraitList(&workloadName, ioStreams)
		},
	}

	cmd.SetOut(ioStreams.Out)
	cmd.Flags().StringVar(&workloadName, "apply-to", "", "Workload name")
	return cmd
}

func printTraitList(workloadName *string, ioStreams cmdutil.IOStreams) error {
	table := uitable.New()
	table.Wrap = true
	table.MaxColWidth = 60
	traitDefinitionList, err := oam.ListTraitDefinitions(workloadName)
	if err != nil {
		return err
	}
	table.AddRow("NAME", "DEFINITION", "APPLIES TO")
	simplifiedTraits := oam.SimplifyCapabilityStruct(traitDefinitionList)
	for _, t := range simplifiedTraits {
		table.AddRow(t.Name, t.Definition, strings.Join(t.AppliesTo, "\n"))
	}
	ioStreams.Info(table.String())
	return nil
}
