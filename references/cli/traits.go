/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/apis/types"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/common"
	"github.com/oam-dev/kubevela/references/plugins"
)

// NewTraitsCommand creates `traits` command
func NewTraitsCommand(c common2.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
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

func printTraitList(userNamespace string, c common2.Args, ioStreams cmdutil.IOStreams) error {
	table := newUITable()
	table.Wrap = true

	traitDefinitionList, err := common.ListRawTraitDefinitions(userNamespace, c)
	if err != nil {
		return err
	}
	table.AddRow("NAME", "NAMESPACE", "APPLIES-TO", "CONFLICTS-WITH", "POD-DISRUPTIVE", "DESCRIPTION")
	for _, t := range traitDefinitionList {
		table.AddRow(t.Name, t.Namespace, strings.Join(t.Spec.AppliesToWorkloads, ","), strings.Join(t.Spec.ConflictsWith, ","), t.Spec.PodDisruptive, plugins.GetDescription(t.Annotations))
	}
	ioStreams.Info(table.String())
	return nil
}
