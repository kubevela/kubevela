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
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/gosuri/uitable"
	"github.com/oam-dev/cluster-gateway/pkg/generated/clientset/versioned"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	apiregistrationV1beta "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1beta1"
	apiregistration "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset/typed/apiregistration/v1beta1"
	metrics "k8s.io/metrics/pkg/client/clientset/versioned"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

const (
	// FlagSpecify specifies the deployment name
	FlagSpecify = "specify"
	// FlagOutputFormat specifies the output format. One of: (wide | yaml)
	FlagOutputFormat = "output"
	// APIServiceName is the name of APIService
	APIServiceName = "v1alpha1.cluster.core.oam.dev"
	// UnknownMetric represent that we can't compute the metric data
	UnknownMetric = "N/A"
)

// NewSystemCommand print system detail info
func NewSystemCommand(c common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "system",
		Short: "Manage system.",
		Long:  "Manage system, including printing the system deployment information in vela-system namespace and diagnosing the system's health.",
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
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get deploymentName from flag
			deployName, err := cmd.Flags().GetString(FlagSpecify)
			if err != nil {
				return errors.Wrapf(err, "failed to get deployment name flag")
			}
			// Get output format from flag
			outputFormat, err := cmd.Flags().GetString(FlagOutputFormat)
			if err != nil {
				return errors.Wrapf(err, "failed to get output format flag")
			}
			if outputFormat != "" {
				outputFormatOptions := map[string]struct{}{
					"wide": {},
					"yaml": {},
				}
				if _, exist := outputFormatOptions[outputFormat]; !exist {
					return errors.Errorf("Outputformat must in wide | yaml !")
				}
			}
			// Get kube config
			if outputFormat != "" {
				outputFormatOptions := map[string]struct{}{
					"wide": {},
					"yaml": {},
				}
				if _, exist := outputFormatOptions[outputFormat]; !exist {
					return errors.Errorf("Outputformat must in wide | yaml !")
				}
			}
			// Get kube config
			config, err := c.GetConfig()
			if err != nil {
				return err
			}
			// Get clientset
			clientset, err := kubernetes.NewForConfig(config)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
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
			if deployName != "" {
				// DeployName is not empty, print the specified deployment's information
				found := false
				for _, deployment := range deployments.Items {
					if deployment.Name == deployName {
						table := SpecifiedFormatPrinter(deployment)
						cmd.Println(table.String())
						found = true
						break
					}
				}
				if !found {
					return errors.Errorf("deployment \"%s\" not found", deployName)
				}
			} else {
				// Get metrics clientset
				mc, err := metrics.NewForConfig(config)
				if err != nil {
					return err
				}
				switch outputFormat {
				case "":
					table, err := NormalFormatPrinter(ctx, deployments, mc)
					if err != nil {
						return err
					}
					cmd.Println(table.String())
				case "wide":
					table, err := WideFormatPrinter(ctx, deployments, mc)
					if err != nil {
						return err
					}
					cmd.Println(table.String())
				case "yaml":
					str, err := YamlFormatPrinter(deployments)
					if err != nil {
						return err
					}
					cmd.Println(str)
				}
			}
			return nil
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
	}
	cmd.Flags().StringP(FlagSpecify, "s", "", "Specify the name of the deployment to check detail information. If empty, it will print all deployments information. Default to be empty.")
	cmd.Flags().StringP(FlagOutputFormat, "o", "", "Specifies the output format. One of: (wide | yaml)")
	return cmd
}

// SpecifiedFormatPrinter prints the specified deployment's information
func SpecifiedFormatPrinter(deployment v1.Deployment) *uitable.Table {
	table := newUITable().
		AddRow("Name:", deployment.Name).
		AddRow("Namespace:", deployment.Namespace).
		AddRow("CreationTimestamp:", deployment.CreationTimestamp).
		AddRow("Labels:", Map2Str(deployment.Labels)).
		AddRow("Annotations:", Map2Str(deployment.Annotations)).
		AddRow("Selector:", Map2Str(deployment.Spec.Selector.MatchLabels)).
		AddRow("Image:", deployment.Spec.Template.Spec.Containers[0].Image).
		AddRow("Args:", strings.Join(deployment.Spec.Template.Spec.Containers[0].Args, "\n")).
		AddRow("Envs:", GetEnvVariable(deployment.Spec.Template.Spec.Containers[0].Env)).
		AddRow("Limits:", CPUMem(deployment.Spec.Template.Spec.Containers[0].Resources.Limits)).
		AddRow("Requests:", CPUMem(deployment.Spec.Template.Spec.Containers[0].Resources.Requests))
	table.MaxColWidth = 120
	return table
}

// CPUMem returns the upsage of cpu and memory
func CPUMem(resourceList corev1.ResourceList) string {
	b := new(bytes.Buffer)
	fmt.Fprintf(b, "cpu=%s\n", resourceList.Cpu())
	fmt.Fprintf(b, "memory=%s", resourceList.Memory())
	return b.String()
}

// Map2Str converts map to string
func Map2Str(m map[string]string) string {
	b := new(bytes.Buffer)
	for key, value := range m {
		fmt.Fprintf(b, "%s=%s\n", key, value)
	}
	if len(b.String()) > 1 {
		return b.String()[:len(b.String())-1]
	}
	return b.String()
}

// NormalFormatPrinter prints information in format of normal
func NormalFormatPrinter(ctx context.Context, deployments *v1.DeploymentList, mc *metrics.Clientset) (*uitable.Table, error) {
	table := newUITable().AddRow("NAME", "NAMESPACE", "READY PODS", "IMAGE", "CPU(cores)", "MEMORY(bytes)")
	cpuMetricMap, memMetricMap := ComputeMetricByDeploymentName(ctx, deployments, mc)
	for _, deploy := range deployments.Items {
		table.AddRow(
			deploy.Name,
			deploy.Namespace,
			fmt.Sprintf("%d/%d", deploy.Status.ReadyReplicas, deploy.Status.Replicas),
			deploy.Spec.Template.Spec.Containers[0].Image,
			cpuMetricMap[deploy.Name],
			memMetricMap[deploy.Name],
		)
	}
	return table, nil
}

// WideFormatPrinter prints information in format of wide
func WideFormatPrinter(ctx context.Context, deployments *v1.DeploymentList, mc *metrics.Clientset) (*uitable.Table, error) {
	table := newUITable().AddRow("NAME", "NAMESPACE", "READY PODS", "IMAGE", "CPU(cores)", "MEMORY(bytes)", "ARGS", "ENVS")
	table.MaxColWidth = 100
	cpuMetricMap, memMetricMap := ComputeMetricByDeploymentName(ctx, deployments, mc)
	for _, deploy := range deployments.Items {
		table.AddRow(
			deploy.Name,
			deploy.Namespace,
			fmt.Sprintf("%d/%d", deploy.Status.ReadyReplicas, deploy.Status.Replicas),
			deploy.Spec.Template.Spec.Containers[0].Image,
			cpuMetricMap[deploy.Name],
			memMetricMap[deploy.Name],
			strings.Join(deploy.Spec.Template.Spec.Containers[0].Args, " "),
			limitStringLength(GetEnvVariable(deploy.Spec.Template.Spec.Containers[0].Env), 180),
		)
	}
	return table, nil
}

// YamlFormatPrinter prints information in format of yaml
func YamlFormatPrinter(deployments *v1.DeploymentList) (string, error) {
	str := ""
	for _, deployment := range deployments.Items {
		// Set ManagedFields to nil because it's too long to read
		deployment.ManagedFields = nil
		deploymentYaml, err := yaml.Marshal(deployment)
		if err != nil {
			return "", err
		}
		str += string(deploymentYaml)
	}
	return str, nil
}

// ComputeMetricByDeploymentName computes cpu and memory metric of deployment
func ComputeMetricByDeploymentName(ctx context.Context, deployments *v1.DeploymentList, mc *metrics.Clientset) (cpuMetricMap, memMetricMap map[string]string) {
	cpuMetricMap = make(map[string]string)
	memMetricMap = make(map[string]string)
	podMetricsList, err := mc.MetricsV1beta1().PodMetricses(metav1.NamespaceAll).List(
		ctx,
		metav1.ListOptions{},
	)
	if err != nil {
		for _, deploy := range deployments.Items {
			cpuMetricMap[deploy.Name] = UnknownMetric
			memMetricMap[deploy.Name] = UnknownMetric
		}
		return
	}

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
		cpuMetricMap[deploy.Name] = fmt.Sprintf("%dm", cpuUsage)
		memMetricMap[deploy.Name] = fmt.Sprintf("%dMi", memUsage)
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
			// Diagnose clusters' health
			fmt.Println("------------------------------------------------------")
			fmt.Println("Diagnosing health of clusters...")
			k8sClient, err := c.GetClient()
			if err != nil {
				return errors.Wrapf(err, "failed to get k8s client")
			}
			clusters, err := multicluster.ListVirtualClusters(context.Background(), k8sClient)
			if err != nil {
				return errors.Wrap(err, "fail to get registered cluster")
			}
			// Get kube config
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
			// Diagnoses the link of hub APIServer to cluster-gateway
			fmt.Println("------------------------------------------------------")
			fmt.Println("Diagnosing the link of hub APIServer to cluster-gateway...")
			// Get clientset
			clientset, err := apiregistration.NewForConfig(config)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			apiService, err := clientset.APIServices().Get(ctx, APIServiceName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			for _, condition := range apiService.Status.Conditions {
				if condition.Type == "Available" {
					if condition.Status != "True" {
						cmd.Printf("APIService \"%s\" is not available! \nMessage: %s\n", APIServiceName, condition.Message)
						return CheckAPIService(ctx, config, apiService)
					}
					cmd.Printf("APIService \"%s\" is available!\n", APIServiceName)
				}
			}
			fmt.Println("Result: The link of hub APIServer to cluster-gateway is fine~")
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

// CheckAPIService checks the APIService
func CheckAPIService(ctx context.Context, config *rest.Config, apiService *apiregistrationV1beta.APIService) error {
	svcName := apiService.Spec.Service.Name
	svcNamespace := apiService.Spec.Service.Namespace
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	svc, err := clientset.CoreV1().Services(svcNamespace).Get(ctx, svcName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	set := labels.Set(svc.Spec.Selector)
	listOptions := metav1.ListOptions{LabelSelector: set.AsSelector().String()}
	pods, err := clientset.CoreV1().Pods(svcNamespace).List(ctx, listOptions)
	if err != nil {
		return err
	}
	if len(pods.Items) == 0 {
		return errors.Errorf("No available pods in %s namespace with label %s.", svcNamespace, set.AsSelector().String())
	}
	for _, pod := range pods.Items {
		for _, status := range pod.Status.ContainerStatuses {
			if !status.Ready {
				for _, condition := range pod.Status.Conditions {
					if condition.Status != "True" {
						return errors.Errorf("Pod %s is not ready. Condition \"%s\" status: %s.", pod.Name, condition.Type, condition.Status)
					}
				}
				return errors.Errorf("Pod %s is not ready.", pod.Name)
			}
		}
	}
	return nil
}
