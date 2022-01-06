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
	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/apis/types"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/common"
)

// NewExportCommand will create command for exporting deploy manifests from an AppFile
func NewExportCommand(c common2.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	appFilePath := new(string)
	cmd := &cobra.Command{
		Use:                   "export",
		DisableFlagsInUseLine: true,
		Short:                 "Export deploy manifests from appfile",
		Long:                  "Export deploy manifests from appfile",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, err := GetFlagNamespaceOrEnv(cmd, c)
			if err != nil {
				return err
			}
			o := &common.AppfileOptions{
				IO: ioStream,
			}
			_, data, err := o.Export(*appFilePath, namespace, true, c)
			if err != nil {
				return err
			}
			_, err = ioStream.Out.Write(data)
			return err
		},
	}
	cmd.SetOut(ioStream.Out)

	addNamespaceAndEnvArg(cmd)
	cmd.Flags().StringVarP(appFilePath, "file", "f", "", "specify file path for appfile")
	return cmd
}
