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

package model

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metrics "k8s.io/metrics/pkg/client/clientset/versioned"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/helm"
	clicommon "github.com/oam-dev/kubevela/references/common"
	"github.com/oam-dev/kubevela/version"
)

// Info is system info struct
type Info struct {
	config *api.Config
}

const (
	// Unknown info
	Unknown = "UNKNOWN"
	// VelaSystemNS is the namespace of vela-system, which is the namespace of vela-core and vela-cluster-gateway
	VelaSystemNS = "vela-system"
	// VelaCoreAppName is the app name of vela-core
	VelaCoreAppName = "app.kubernetes.io/name=vela-core"
	// ClusterGatewayAppName is the app name of vela-core
	ClusterGatewayAppName = "app.kubernetes.io/name=vela-core-cluster-gateway"
)

// NewInfo return a new info struct
func NewInfo() *Info {
	info := &Info{}
	k := genericclioptions.NewConfigFlags(true)
	rawConfig, err := k.ToRawKubeConfigLoader().RawConfig()
	if err == nil {
		info.config = &rawConfig
	}
	return info
}

// CurrentContext return current context info
func (i *Info) CurrentContext() string {
	if i.config != nil {
		return i.config.CurrentContext
	}
	return Unknown
}

// ClusterNum return cluster number
func (i *Info) ClusterNum() string {
	if i.config != nil {
		return strconv.Itoa(len(i.config.Clusters))
	}
	return "0"
}

// K8SVersion return k8s version info
func K8SVersion(cfg *rest.Config) string {
	c, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return Unknown
	}
	serverVersion, err := c.ServerVersion()
	if err != nil {
		return Unknown
	}
	vStr := fmt.Sprintf("%s.%s", serverVersion.Major, strings.Replace(serverVersion.Minor, "+", "", 1))
	return vStr
}

// VelaCLIVersion return vela cli version info
func VelaCLIVersion() string {
	return version.VelaVersion
}

// VelaCoreVersion return vela core version info
func VelaCoreVersion() string {
	results, err := helm.GetHelmRelease(types.DefaultKubeVelaNS)
	if err != nil {
		return Unknown
	}
	for _, result := range results {
		if result.Chart.ChartFullPath() == types.DefaultKubeVelaChartName {
			return result.Chart.AppVersion()
		}
	}
	return Unknown
}

// GOLangVersion return golang version info
func GOLangVersion() string {
	return runtime.Version()
}

// ApplicationRunningNum return the num of running application
func ApplicationRunningNum(cfg *rest.Config) string {
	ctx := context.Background()
	c, err := client.New(cfg, client.Options{Scheme: common.Scheme})
	if err != nil {
		return clicommon.MetricsNA
	}
	appNum, err := applicationNum(ctx, c)
	if err != nil {
		return clicommon.MetricsNA
	}
	runningAppNum, err := runningApplicationNum(ctx, c)
	if err != nil {
		return clicommon.MetricsNA
	}
	return fmt.Sprintf("%d/%d", runningAppNum, appNum)
}

func velaCorePodUsage(cfg *rest.Config) (*v1beta1.PodMetrics, error) {
	ctx := context.Background()
	c, err := metrics.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	metricsList, err := c.MetricsV1beta1().PodMetricses(VelaSystemNS).List(ctx, metav1.ListOptions{LabelSelector: VelaCoreAppName})
	if err != nil {
		return nil, err
	}
	if len(metricsList.Items) != 1 {
		return nil, errors.New("the num of vela core container isn't right")
	}
	return &metricsList.Items[0], nil
}

func velaCorePod(cfg *rest.Config) (*v1.Pod, error) {
	ctx := context.Background()
	pods := v1.PodList{}
	c, err := client.New(cfg, client.Options{Scheme: common.Scheme})
	if err != nil {
		return nil, err
	}
	opts := []client.ListOption{
		client.InNamespace(VelaSystemNS),
		client.MatchingLabels{"app.kubernetes.io/name": "vela-core"},
	}
	err = c.List(ctx, &pods, opts...)
	if err != nil {
		return nil, err
	}
	if len(pods.Items) != 1 {
		return nil, errors.New("the num of vela core container isn't right")
	}
	return &pods.Items[0], nil
}

// VelaCoreRatio return the usage condition of vela-core pod
func VelaCoreRatio(c client.Client, cfg *rest.Config) (string, string, string, string) {
	mtx, err := velaCorePodUsage(cfg)
	if err != nil {
		return clicommon.MetricsNA, clicommon.MetricsNA, clicommon.MetricsNA, clicommon.MetricsNA
	}
	pod, err := velaCorePod(cfg)
	if err != nil {
		return clicommon.MetricsNA, clicommon.MetricsNA, clicommon.MetricsNA, clicommon.MetricsNA
	}
	spec, usage := clicommon.GetPodResourceSpecAndUsage(c, pod, mtx)
	cpuLRatio, memLRatio := clicommon.ToPercentageStr(usage.CPU, spec.Lcpu), clicommon.ToPercentageStr(usage.Mem, spec.Lmem)
	cpuRRatio, memRRatio := clicommon.ToPercentageStr(usage.CPU, spec.Rcpu), clicommon.ToPercentageStr(usage.Mem, spec.Rmem)
	return cpuLRatio, memLRatio, cpuRRatio, memRRatio
}

func velaCLusterGatewayPodUsage(cfg *rest.Config) (*v1beta1.PodMetrics, error) {
	ctx := context.Background()
	c, err := metrics.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	metricsList, err := c.MetricsV1beta1().PodMetricses(VelaSystemNS).List(ctx, metav1.ListOptions{LabelSelector: ClusterGatewayAppName})
	if err != nil {
		return nil, err
	}
	if len(metricsList.Items) != 1 || len(metricsList.Items[0].Containers) != 1 {
		return nil, errors.New("the num of vela core cluster gateway container isn't right")
	}
	return &metricsList.Items[0], nil
}

func velaCLusterGatewayPod(cfg *rest.Config) (*v1.Pod, error) {
	ctx := context.Background()
	pods := v1.PodList{}
	c, err := client.New(cfg, client.Options{Scheme: common.Scheme})
	if err != nil {
		return nil, err
	}
	opts := []client.ListOption{
		client.InNamespace(VelaSystemNS),
		client.MatchingLabels{"app.kubernetes.io/name": "vela-core-cluster-gateway"},
	}
	err = c.List(ctx, &pods, opts...)
	if err != nil {
		return nil, err
	}
	if len(pods.Items) != 1 || len(pods.Items[0].Spec.Containers) != 1 {
		return nil, errors.New("the num of vela core cluster gateway container isn't right")
	}
	return &pods.Items[0], nil
}

// CLusterGatewayRatio return the usage condition of vela-core cluster gateway pod
func CLusterGatewayRatio(c client.Client, cfg *rest.Config) (string, string, string, string) {
	mtx, err := velaCLusterGatewayPodUsage(cfg)
	if err != nil {
		return clicommon.MetricsNA, clicommon.MetricsNA, clicommon.MetricsNA, clicommon.MetricsNA
	}
	pod, err := velaCLusterGatewayPod(cfg)
	if err != nil {
		return clicommon.MetricsNA, clicommon.MetricsNA, clicommon.MetricsNA, clicommon.MetricsNA
	}
	spec, usage := clicommon.GetPodResourceSpecAndUsage(c, pod, mtx)
	cpuLRatio, memLRatio := clicommon.ToPercentageStr(usage.CPU, spec.Lcpu), clicommon.ToPercentageStr(usage.Mem, spec.Lmem)
	cpuRRatio, memRRatio := clicommon.ToPercentageStr(usage.CPU, spec.Rcpu), clicommon.ToPercentageStr(usage.Mem, spec.Rmem)
	return cpuLRatio, memLRatio, cpuRRatio, memRRatio
}
