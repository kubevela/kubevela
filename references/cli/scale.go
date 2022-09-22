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
	"errors"
	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/apis/types"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/common"
)

// NewScaleCommand Scale App
func NewScaleCommand(c common2.Args, order string, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "scale",
		DisableFlagsInUseLine: true,
		Short:                 "Scale a component",
		Long:                  "Scale a component.",
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeApp,
		},
		Example: "scale app component 1",
	}
	cmd.SetOut(ioStreams.Out)

	o := &common.ScaleOptions{
		C: c,
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		namespace, err := GetFlagNamespaceOrEnv(cmd, c)
		if err != nil {
			return err
		}
		o.Namespace = namespace
		newClient, err := c.GetClient()
		if err != nil {
			return err
		}
		o.Client = newClient

		if len(args) < 1 {
			return errors.New("must specify name for the app")
		}

		o.AppName = args[0]
		component, _ := cmd.Flags().GetString("component")
		replicas, _ := cmd.Flags().GetInt64("replicas")
		o.ScaleComponent(component, replicas, ioStreams)
		ioStreams.Info(green.Sprintf("app \"%s\" scale %s to $d from namespace \"%s\"", o.AppName, component, replicas, o.Namespace))
		return nil
	}

	cmd.Flags().StringP("component", "c", "", "filter the endpoints or pods by component name")
	cmd.Flags().Int64("replicas", 1, "filter the endpoints or pods by component name")
	addNamespaceAndEnvArg(cmd)
	return cmd
}
