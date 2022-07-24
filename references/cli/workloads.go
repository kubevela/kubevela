/*
 Copyright 2021. The KubeVela Authors.

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
	"github.com/spf13/cobra"

	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/common"
	"github.com/oam-dev/kubevela/references/docgen"
)

// NewWorkloadsCommand creates `workloads` command
func NewWorkloadsCommand(c common2.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "workloads",
		DisableFlagsInUseLine: true,
		Short:                 "List workloads",
		Long:                  "List workloads",
		Example:               `vela workloads`,
		Hidden:                true,
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, err := GetFlagNamespaceOrEnv(cmd, c)
			if err != nil {
				return err
			}
			return printWorkloadList(namespace, c, ioStreams)
		},
		Annotations: map[string]string{},
	}
	cmd.SetOut(ioStreams.Out)
	addNamespaceAndEnvArg(cmd)
	return cmd
}

func printWorkloadList(userNamespace string, c common2.Args, ioStreams cmdutil.IOStreams) error {
	def, err := common.ListRawWorkloadDefinitions(userNamespace, c)
	if err != nil {
		return err
	}
	table := newUITable()
	table.AddRow("NAME", "NAMESPACE", "WORKLOAD", "DESCRIPTION")
	for _, r := range def {
		table.AddRow(r.Name, r.Namespace, r.Spec.Reference.Name, docgen.GetDescription(r.Annotations))
	}
	ioStreams.Info(table.String())
	return nil
}
