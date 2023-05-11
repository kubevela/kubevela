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
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/references/cli/top/view"
)

// NewTopCommand will create command `top` for displaying the platform overview
func NewTopCommand(order string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "top",
		Short: "Launch UI to display the platform overview.",
		Long:  "Launch UI to display platform overview information and diagnose the status for any specific application.",
		Example: `  # Launch UI to display platform overview information and diagnose the status for any specific application
  vela top
  
  # Show applications which are in <vela-namespace> namespace
  vela top -n <vela-namespace>
  
  # Show applications which are in all namespaces
  vela top -A
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, err := GetFlagNamespaceOrEnv(cmd)
			if err != nil {
				return err
			}
			if AllNamespace {
				namespace = ""
			}
			klog.SetLogger(logr.New(log.NullLogSink{}))
			return launchUI(namespace)
		},
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypePlatform,
		},
	}
	addNamespaceAndEnvArg(cmd)
	cmd.Flags().BoolVarP(&AllNamespace, "all-namespaces", "A", false, "If true, check the specified action in all namespaces.")
	return cmd
}

func launchUI(namespace string) error {
	app := view.NewApp(cli, cfg, namespace)
	app.Init()

	return app.Run()
}
