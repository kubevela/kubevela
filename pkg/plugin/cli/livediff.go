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

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/cli"
)

// NewLiveDiffCommand creates `live-diff` command
func NewLiveDiffCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	var namespace string
	o := &cli.LiveDiffCmdOptions{
		DryRunCmdOptions: cli.DryRunCmdOptions{
			IOStreams: ioStreams,
		}}
	cmd := &cobra.Command{
		Use:                   "live-diff",
		DisableFlagsInUseLine: true,
		Short:                 "Dry-run an application, and do diff on a specific app revison",
		Long:                  "Dry-run an application, and do diff on a specific app revison. The provided capability definitions will be used during Dry-run. If any capabilities used in the app are not found in the provided ones, it will try to find from cluster.",
		Example:               "kubectl vela live-diff -f app-v2.yaml -r app-v1 --context 10",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {

			buff, err := cli.LiveDiffApplication(o, c, namespace)
			if err != nil {
				return err
			}
			o.Info(buff.String())
			return nil
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypePlugin,
		},
	}

	cmd.Flags().StringVarP(&o.ApplicationFile, "file", "f", "./app.yaml", "application file name")
	cmd.Flags().StringVarP(&o.DefinitionFile, "definition", "d", "", "specify a file or directory containing capability definitions, they will only be used in dry-run rather than applied to K8s cluster")
	cmd.Flags().StringVarP(&o.Revision, "revision", "r", "", "specify an application revision name, by default, it will compare with the latest revision")
	cmd.Flags().IntVarP(&o.Context, "context", "c", -1, "output number lines of context around changes, by default show all unchanged lines")
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "specify namespace of the application to be compared, by default is default namespace")
	cmd.SetOut(ioStreams.Out)
	return cmd
}
