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
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	referencesCommon "github.com/oam-dev/kubevela/references/common"
)

// NewAppMetricsCommand creates metrics command
func NewAppMetricsCommand(c common.Args, order string, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metrics APP_NAME",
		Short: "Show metrics of an application.",
		Long:  "Show metrics info of vela application.",
		Example: `  # Get basic app info
  vela metrics APP_NAME`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// check args
			argsLength := len(args)
			if argsLength == 0 {
				return fmt.Errorf("please specify an application")
			}
			appName := args[0]
			// get namespace
			namespace, err := GetFlagNamespaceOrEnv(cmd, c)
			if err != nil {
				return err
			}
			client, err := c.GetClient()
			if err != nil {
				return err
			}
			config, err := c.GetConfig()
			if err != nil {
				return err
			}
			return printMetrics(client, config, appName, namespace)
		},
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeApp,
		},
	}
	addNamespaceAndEnvArg(cmd)
	cmd.Flags().BoolVarP(&AllNamespace, "all-namespaces", "A", false, "If true, check the specified action in all namespaces.")
	return cmd
}

// printMetrics prints the resource num and resource metrics of an application
func printMetrics(c client.Client, conf *rest.Config, appName, appNamespace string) error {
	app := new(v1beta1.Application)
	err := c.Get(context.Background(), client.ObjectKey{
		Name:      appName,
		Namespace: appNamespace,
	}, app)
	if err != nil {
		return err
	}
	metrics, err := referencesCommon.LoadApplicationMetrics(c, conf, app)
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Printf("Kubernetes Resources created:\n")
	fmt.Printf("    * Number of Pods: %d\n", metrics.Resource.PodNum)
	fmt.Printf("    * Number of Containers: %d\n", metrics.Resource.ContainerNum)
	fmt.Printf("    * Number of Managed Resource: %d\n", metrics.Resource.SubresourceNum)
	fmt.Printf("    * Number of Nodes: %d\n", metrics.Resource.NodeNum)
	fmt.Printf("    * Number of Clusters: %d\n", metrics.Resource.ClusterNum)
	fmt.Println()
	fmt.Printf("Underlying Physical Resoures consumed:\n")
	fmt.Printf("    * Total CPU(cores): %d m\n", metrics.Status.CPU)
	fmt.Printf("    * Total MEMORY(bytes): %d Mi\n", metrics.Status.Memory)
	fmt.Printf("    * Total Storage(bytes): %d Gi\n", metrics.Status.Storage)
	fmt.Println()
	return nil
}
