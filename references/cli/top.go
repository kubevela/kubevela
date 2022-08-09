/*
Copyright 2022 The KubeVela Authors.

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
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/cli/top/view"
)

// NewTopCommand will create command `top` for displaying the platform overview
func NewTopCommand(c common.Args, order string, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "top",
		Short: "Launch UI to display the platform overview.",
		Long:  "Launch UI to display platform overview information and diagnose the status for any specific application.",
		Example: `  # Launch UI to display platform overview information and diagnose the status for any specific application
  vela top`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return launchUI(c, cmd)
		},
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeApp,
		},
	}
	return cmd
}

func launchUI(c common.Args, _ *cobra.Command) error {
	k8sClient, err := c.GetClient()
	if err != nil {
		return fmt.Errorf("cannot get k8s client: %w", err)
	}
	restConfig, err := c.GetConfig()
	if err != nil {
		return err
	}
	app := view.NewApp(k8sClient, restConfig)
	app.Init()

	return app.Run()
}
