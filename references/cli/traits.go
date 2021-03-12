package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/apis/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/common"
	"github.com/oam-dev/kubevela/references/plugins"
)

// NewTraitsCommand creates `traits` command
func NewTraitsCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "traits",
		DisableFlagsInUseLine: true,
		Short:                 "List traits",
		Long:                  "List traits",
		Example:               `vela traits`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := GetEnv(cmd)
			if err != nil {
				return err
			}
			return printTraitList(env.Namespace, c, ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCap,
		},
	}

	cmd.SetOut(ioStreams.Out)
	return cmd
}

func printTraitList(userNamespace string, c types.Args, ioStreams cmdutil.IOStreams) error {
	table := newUITable()
	table.Wrap = true

	traitDefinitionList, err := common.ListRawTraitDefinitions(userNamespace, c)
	if err != nil {
		return err
	}
	table.AddRow("NAME", "NAMESPACE", "APPLIES-TO", "CONFLICTS-WITH", "DESCRIPTION")
	for _, t := range traitDefinitionList {
		table.AddRow(t.Name, t.Namespace, strings.Join(t.Spec.AppliesToWorkloads, ","), strings.Join(t.Spec.ConflictsWith, ","), plugins.GetDescription(t.Annotations))
	}
	ioStreams.Info(table.String())
	return nil
}
