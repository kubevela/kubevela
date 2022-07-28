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
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metrics "k8s.io/metrics/pkg/client/clientset/versioned"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

const (
	// FlagSpecify specifies the deployment name
	FlagSpecify = "specify"
	// FlagDeploymentNamespace specifies the namespace of deployment
	FlagDeploymentNamespace = "namespace"
)

// NewSystemCommand print system detail info
func NewSystemCommand(c common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "system",
		Short: "Manage system.",
		Long:  "Manage system, incluing printing the system deployment information in vela-system namespace and diagnosing the system's health.",
		Example: "# Check all deployments information in all namespaces with label app.kubernetes.io/name=vela-core :\n" +
			"> vela system info\n" +
			"# Specify a deployment name with a namespace to check detail information:\n" +
			"> vela system info -s kubevela-vela-core -n vela-system\n" +
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
		Short: "Print the system deployment detail information in all namespaces with label app.kubernetes.io/name=vela-core.",
		Long:  "Print the system deployment detail information in all namespaces with label app.kubernetes.io/name=vela-core.",
		Args:  cobra.ExactValidArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get deploymentName from flag
			deployName, err := cmd.Flags().GetString(FlagSpecify)
			if err != nil {
				return errors.Wrapf(err, "failed to get deployment name flag")
			}
			namespace, err := cmd.Flags().GetString(FlagDeploymentNamespace)
			if err != nil {
				return errors.Wrapf(err, "failed to get deployment namespace flag")
			}
			config, err := c.GetConfig()
			if err != nil {
				return err
			}
			// Get clientset
			clientset, err := kubernetes.NewForConfig(config)
			if err != nil {
				return err
			}
			mc, err := metrics.NewForConfig(config)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if deployName != "" {
				// DeployName is not empty, print the specified deployment's information in yaml type
				if namespace == "" {
					return errors.Errorf("An empty namespace can not be set when a resource name is provided. Use -n to specify the namespace.")
				}
				// Get specified deployment in specified namespace
				deployment, err := clientset.AppsV1().Deployments(namespace).Get(
					ctx,
					deployName,
					metav1.GetOptions{},
				)
				if err != nil {
					return err
				}
				// Set ManagedFields to nil because it's too long to read
				deployment.ManagedFields = nil
				deploymentYaml, _ := yaml.Marshal(deployment)
				if err != nil {
					return err
				}
				cmd.Println(string(deploymentYaml))
			} else {
				// DeployName is empty, print all deployment's partial args
				// Get deploymentsClient in all namespace
				deployments, err := clientset.AppsV1().Deployments(metav1.NamespaceAll).List(
					ctx,
					metav1.ListOptions{
						LabelSelector: "app.kubernetes.io/name=vela-core",
					},
				)
				if err != nil {
					return err
				}
				podMetricsList, err := mc.MetricsV1beta1().PodMetricses(metav1.NamespaceAll).List(
					ctx,
					metav1.ListOptions{},
				)
				if err != nil {
					return err
				}
				table := newUITable().AddRow("NAME", "NAMESPACE", "READY PODS", "IMAGE", "CPU(cores)", "MEMORY(bytes)", "ARGS", "ENVS")
				cpuMetricMap, memMetricMap := ComputeMetricByDeploymentName(deployments, podMetricsList)
				for _, deploy := range deployments.Items {
					table.AddRow(
						deploy.Name,
						deploy.Namespace,
						fmt.Sprintf("%d/%d", deploy.Status.ReadyReplicas, deploy.Status.Replicas),
						deploy.Spec.Template.Spec.Containers[0].Image,
						fmt.Sprintf("%dm", cpuMetricMap[deploy.Name]),
						fmt.Sprintf("%dMi", memMetricMap[deploy.Name]),
						limitStringLength(strings.Join(deploy.Spec.Template.Spec.Containers[0].Args, " "), 50),
						limitStringLength(GetEnvVariable(deploy.Spec.Template.Spec.Containers[0].Env), 50),
					)
				}
				cmd.Println(table.String())
			}
			return nil
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
	}
	cmd.Flags().StringP(FlagSpecify, "s", "", "Specify the name of the deployment to check detail information. If empty, it will print all deployments information. Default to be empty.")
	cmd.Flags().StringP(FlagDeploymentNamespace, "n", "", "Specify the namespace of the deployment to check detail information. If empty, it will prints deployments information of all namespaces. Default to be empty. An empty namespace may not be can when a resource name is provided. ")
	return cmd
}

// ComputeMetricByDeploymentName computes cpu and memory metric of deployment
func ComputeMetricByDeploymentName(deployments *v1.DeploymentList, podMetricsList *v1beta1.PodMetricsList) (cpuMetricMap, memMetricMap map[string]int64) {
	cpuMetricMap = make(map[string]int64)
	memMetricMap = make(map[string]int64)
	for _, deploy := range deployments.Items {
		cpuUsage, memUsage := int64(0), int64(0)
		for _, pod := range podMetricsList.Items {
			if strings.HasPrefix(pod.Name, deploy.Name) {
				for _, container := range pod.Containers {
					cpuUsage += container.Usage.Cpu().MilliValue()
					memUsage += container.Usage.Memory().Value() / (1024 * 1024)
				}
			}
		}
		cpuMetricMap[deploy.Name] = cpuUsage
		memMetricMap[deploy.Name] = memUsage
	}
	return
}

// GetEnvVariable gets the environment variables
func GetEnvVariable(envList []corev1.EnvVar) (envStr string) {
	for _, env := range envList {
		envStr += fmt.Sprintf("%s=%s ", env.Name, env.Value)
	}
	if len(envStr) == 0 {
		return "-"
	}
	return
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
