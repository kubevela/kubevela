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
	"strings"

	"github.com/oam-dev/cluster-gateway/pkg/generated/clientset/versioned"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	// FlagDeploymentName specifies the deployment name
	FlagDeploymentName = "name"
)

// NewSystemCommand print system detail info
func NewSystemCommand(c common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "system",
		Short: "Manage system.",
		Long:  "Manage system, incluing printing the system deployment information in vela-system namespace and diagnosing the system's health.",
		Example: "# Check all deployments information:\n" +
			"> vela system info\n" +
			"# Specify a deployment name to check detail information:\n" +
			"> vela system info --name kubevela-vela-core\n" +
			"# Diagnose the system's health:\n" +
			"> vela system diagnose\n",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
	}
	cmd.AddCommand(
		NewSystemInfoCommand(c),
		NewSystemDiagnoseCommand(c))
	return cmd
}

// NewSystemInfoCommand prints system detail info
func NewSystemInfoCommand(c common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Print the system deployment detail information in vela-system namespace.",
		Long:  "Print the system deployment detail information in vela-system namespace.",
		Args:  cobra.ExactValidArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get deploymentName from flag
			deployName, err := cmd.Flags().GetString(FlagDeploymentName)
			if err != nil {
				return errors.Wrapf(err, "failed to get deployment name flag")
			}
			table := newUITable().AddRow("NAME", "NAMESPACE", "IMAGE", "ARGS")
			table.MaxColWidth = 120
			config, err := c.GetConfig()
			if err != nil {
				return err
			}
			// Get clientset
			clientset, err := kubernetes.NewForConfig(config)
			if err != nil {
				panic(err)
			}
			// Get deploymentsClient
			deploymentsClient := clientset.AppsV1().Deployments(types.DefaultKubeVelaNS)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if deployName != "" {
				// DeployName is not empty, print the specified deployment's all args
				deployment, err := deploymentsClient.Get(ctx, deployName, metav1.GetOptions{})
				if err != nil {
					panic(err)
				}
				table.AddRow(deployment.Name, deployment.Namespace, deployment.Spec.Template.Spec.Containers[0].Image, strings.Join(deployment.Spec.Template.Spec.Containers[0].Args, " "))
			} else {
				// DeployName is empty, print all deployment's partial args
				deployments, err := deploymentsClient.List(ctx, metav1.ListOptions{})
				if err != nil {
					panic(err)
				}
				for _, deploy := range deployments.Items {
					allArgs := deploy.Spec.Template.Spec.Containers[0].Args
					table.AddRow(deploy.Name, deploy.Namespace, deploy.Spec.Template.Spec.Containers[0].Image, limitStringLength(strings.Join(allArgs, " "), 60))
				}
			}
			if len(table.Rows) == 1 {
				cmd.Println("No deployment found.")
			} else {
				cmd.Println(table.String())
			}
			return nil
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
	}
	cmd.Flags().StringP(FlagDeploymentName, "n", "", "Specify the deployment name to check detail information. If empty, it will print all deployments information. Default to be empty.")
	return cmd
}

// NewSystemDiagnoseCommand create command to help user to diagnose system's health
func NewSystemDiagnoseCommand(c common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diagnose",
		Short: "Diagnoses system problems.",
		Long:  "Diagnoses system problems.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Diagnoses APIService of cluster-gateway
			fmt.Println("------------------------------------------------------")
			fmt.Println("Diagnosing APIService of cluster-gateway...")
			k8sClient, err := c.GetClient()
			if err != nil {
				return errors.Wrapf(err, "failed to get k8s client")
			}
			_, err = multicluster.GetClusterGatewayService(context.Background(), k8sClient)
			if err != nil {
				return errors.Wrapf(err, "failed to get cluster secret namespace, please ensure cluster gateway is correctly deployed")
			}
			fmt.Println("Result: APIService of cluster-gateway is fine~")
			fmt.Println("------------------------------------------------------")
			// Diagnose clusters' health
			fmt.Println("------------------------------------------------------")
			fmt.Println("Diagnosing health of clusters...")
			clusters, err := multicluster.ListVirtualClusters(context.Background(), k8sClient)
			if err != nil {
				return errors.Wrap(err, "fail to get registered cluster")
			}
			config, err := c.GetConfig()
			if err != nil {
				return err
			}
			for _, cluster := range clusters {
				clusterName := cluster.Name
				if clusterName == multicluster.ClusterLocalName {
					continue
				}
				content, err := versioned.NewForConfigOrDie(config).ClusterV1alpha1().ClusterGateways().RESTClient(clusterName).Get().AbsPath("healthz").DoRaw(context.TODO())
				if err != nil {
					return errors.Wrapf(err, "failed connect cluster %s", clusterName)
				}
				cmd.Printf("Connect to cluster %s successfully.\n%s\n", clusterName, string(content))
			}
			fmt.Println("Result: Clusters are fine~")
			fmt.Println("------------------------------------------------------")
			// Todo: Diagnose others
			return nil
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
	}
	return cmd
}
