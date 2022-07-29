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
	"context"
	"fmt"

	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/appfile"
)

// NewPodCommand create `pod` command
func NewPodCommand(c common.Args, order string, ioStreams cmdutil.IOStreams) *cobra.Command {
	var podArgs PodArgs
	podArgs.Args = c
	cmd := &cobra.Command{
		Use:   "pods APP_NAME",
		Short: "Query and show the pod list of the application.",
		Long:  "Query and show the pod list of the application.",
		Args:  cobra.ExactArgs(1),
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeApp,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			podArgs.Namespace, err = GetFlagNamespaceOrEnv(cmd, c)
			if err != nil {
				return err
			}
			app, err := appfile.LoadApplication(podArgs.Namespace, args[0], c)
			if err != nil {
				return err
			}
			podArgs.App = app

			table, err := podArgs.listPods(context.Background())
			if err != nil {
				return err
			}
			fmt.Println(table.String())
			return nil
		},
	}
	cmd.Flags().StringVarP(&podArgs.ComponentName, "component", "c", "", "filter the pod by the component name")
	cmd.Flags().StringVarP(&podArgs.ClusterName, "cluster", "", "", "filter the pod by the cluster name")
	addNamespaceAndEnvArg(cmd)
	return cmd
}

// PodArgs creates arguments for `pods` command
type PodArgs struct {
	Args          common.Args
	Namespace     string
	ClusterName   string
	ComponentName string
	App           *v1beta1.Application
}

func (p *PodArgs) listPods(ctx context.Context) (*uitable.Table, error) {
	pods, err := GetApplicationPods(ctx, p.App.Name, p.Namespace, p.Args, Filter{
		Component: p.ComponentName,
		Cluster:   p.ClusterName,
	})
	if err != nil {
		return nil, err
	}
	table := uitable.New()
	table.AddRow("CLUSTER", "COMPONENT", "POD NAME", "NAMESPACE", "PHASE", "CREATE TIME", "REVISION", "HOST")
	for _, pod := range pods {
		table.AddRow(pod.Cluster, pod.Component, pod.Metadata.Name, pod.Metadata.Namespace, pod.Status.Phase, pod.Metadata.CreationTime, pod.Metadata.Version.DeployVersion, pod.Status.NodeName)
	}
	return table, nil
}
