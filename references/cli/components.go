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
	"fmt"

	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/apis/types"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/common"
	"github.com/oam-dev/kubevela/references/plugins"
)

// NewComponentsCommand creates `components` command
func NewComponentsCommand(c common2.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "components",
		DisableFlagsInUseLine: true,
		Short:                 "List components",
		Long:                  "List components",
		Example:               `vela components`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := GetEnv(cmd)
			if err != nil {
				return err
			}
			return printComponentList(env.Namespace, c, ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCap,
		},
	}
	cmd.SetOut(ioStreams.Out)
	return cmd
}

func printComponentList(userNamespace string, c common2.Args, ioStreams cmdutil.IOStreams) error {
	def, err := common.ListRawComponentDefinitions(userNamespace, c)
	if err != nil {
		return err
	}

	dm, err := c.GetDiscoveryMapper()
	if err != nil {
		return fmt.Errorf("get discoveryMapper error %w", err)
	}

	table := newUITable()
	table.AddRow("NAME", "NAMESPACE", "WORKLOAD", "DESCRIPTION")

	for _, r := range def {
		var workload string
		if r.Spec.Workload.Type != "" {
			workload = r.Spec.Workload.Type
		} else {
			definition, err := oamutil.ConvertWorkloadGVK2Definition(dm, r.Spec.Workload.Definition)
			if err != nil {
				return fmt.Errorf("get workload definitionReference error %w", err)
			}
			workload = definition.Name
		}
		table.AddRow(r.Name, r.Namespace, workload, plugins.GetDescription(r.Annotations))
	}
	ioStreams.Info(table.String())
	return nil
}
