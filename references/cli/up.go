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
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	corev1beta1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/common"
)

// NewUpCommand will create command for applying an AppFile
func NewUpCommand(c common2.Args, order string, ioStream cmdutil.IOStreams) *cobra.Command {
	appFilePath := new(string)
	cmd := &cobra.Command{
		Use:                   "up",
		DisableFlagsInUseLine: true,
		Short:                 "Apply an appfile or application from file",
		Long:                  "Apply an appfile or application from file, vela will convert appfile to application automatically",
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeStart,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, err := GetFlagNamespaceOrEnv(cmd, c)
			if err != nil {
				return err
			}
			kubecli, err := c.GetClient()
			if err != nil {
				return err
			}
			fileContent, err := os.ReadFile(filepath.Clean(*appFilePath))
			if err != nil {
				return err
			}
			var app corev1beta1.Application
			err = yaml.Unmarshal(fileContent, &app)
			if err != nil {
				return errors.Wrap(err, "File format is illegal, only support vela appfile format or OAM Application object yaml")
			}
			if app.APIVersion != "" && app.Kind != "" {
				err = common.ApplyApplication(app, ioStream, kubecli)
				if err != nil {
					return err
				}
			} else {
				o := &common.AppfileOptions{
					Kubecli:   kubecli,
					IO:        ioStream,
					Namespace: namespace,
				}
				return o.Run(*appFilePath, o.Namespace, c)
			}
			return nil
		},
	}
	cmd.SetOut(ioStream.Out)
	cmd.Flags().StringVarP(appFilePath, "file", "f", "", "specify file path for appfile or application")

	addNamespaceAndEnvArg(cmd)
	return cmd
}
