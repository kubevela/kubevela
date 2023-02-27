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
	"strconv"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	clicommon "github.com/oam-dev/kubevela/references/common"
)

// Container represent the container resource instance
type Container struct {
	name               string
	image              string
	ready              string
	state              string
	CPU                string
	Mem                string
	CPUR               string
	CPUL               string
	MemR               string
	MemL               string
	lastTerminationMsg string
	restartCount       string
}

// ContainerList is container list
type ContainerList []Container

// ListContainerOfPod get the container data of aimed pod
func ListContainerOfPod(ctx context.Context, client client.Client, cfg *rest.Config) (ContainerList, error) {
	name := ctx.Value(&CtxKeyPod).(string)
	namespace := ctx.Value(&CtxKeyNamespace).(string)
	cluster := ctx.Value(&CtxKeyCluster).(string)

	pod := v1.Pod{}
	err := client.Get(multicluster.ContextWithClusterName(context.Background(), cluster), types.NamespacedName{Name: name, Namespace: namespace}, &pod)
	if err != nil {
		return nil, err
	}

	usageMap := fetchContainerMetricsUsageMap(cfg, name, namespace, cluster)
	lrMap := fetchContainerMetricsLRMap(pod.Spec)

	containers := make([]Container, 0)
	for _, c := range pod.Status.ContainerStatuses {
		containers = append(containers, loadContainerDetail(c, usageMap, lrMap))
	}
	return containers, nil
}

func loadContainerDetail(c v1.ContainerStatus, usageMap map[string]v1.ResourceList, lrMap map[string]v1.ResourceRequirements) Container {
	containerInfo := Container{
		name:               c.Name,
		image:              c.Image,
		CPU:                clicommon.MetricsNA,
		Mem:                clicommon.MetricsNA,
		CPUR:               clicommon.MetricsNA,
		CPUL:               clicommon.MetricsNA,
		MemR:               clicommon.MetricsNA,
		MemL:               clicommon.MetricsNA,
		lastTerminationMsg: "",
		restartCount:       string(c.RestartCount),
	}
	if c.Ready {
		containerInfo.ready = "Yes"
	} else {
		containerInfo.ready = "No"
	}
	containerInfo.state = common.ContainerStateToString(c.State)

	usage, ok1 := usageMap[c.Name]
	lr, ok2 := lrMap[c.Name]
	if ok1 && ok2 {
		cpuUsage := usage.Cpu().MilliValue()
		memUsage := usage.Memory().Value()
		containerInfo.CPU, containerInfo.Mem = strconv.FormatInt(cpuUsage, 10), strconv.FormatInt(memUsage/1000000, 10)
		containerInfo.CPUR = clicommon.ToPercentageStr(cpuUsage, lr.Requests.Cpu().MilliValue())
		containerInfo.CPUL = clicommon.ToPercentageStr(cpuUsage, lr.Limits.Cpu().MilliValue())
		containerInfo.MemR = clicommon.ToPercentageStr(memUsage, lr.Requests.Memory().Value())
		containerInfo.MemL = clicommon.ToPercentageStr(memUsage, lr.Limits.Memory().Value())
	}
	if c.LastTerminationState.Terminated != nil {
		containerInfo.lastTerminationMsg = c.LastTerminationState.Terminated.Message
	}
	return containerInfo
}

func fetchContainerMetricsUsageMap(cfg *rest.Config, name, namespace, cluster string) map[string]v1.ResourceList {
	metric, err := clicommon.GetPodMetrics(cfg, name, namespace, cluster)
	if err != nil {
		return nil
	}
	cmx := make(map[string]v1.ResourceList, len(metric.Containers))
	for i := range metric.Containers {
		c := metric.Containers[i]
		cmx[c.Name] = c.Usage
	}
	return cmx
}

func fetchContainerMetricsLRMap(spec v1.PodSpec) map[string]v1.ResourceRequirements {
	lr := make(map[string]v1.ResourceRequirements, len(spec.Containers))
	for _, container := range spec.Containers {
		lr[container.Name] = container.Resources
	}
	return lr
}

// ToTableBody generate body of table in pod view
func (l ContainerList) ToTableBody() [][]string {
	data := make([][]string, len(l))
	for index, container := range l {
		data[index] = []string{container.name, container.image, container.ready, container.state, container.CPU, container.Mem, container.CPUR, container.CPUL, container.MemR, container.MemL, container.lastTerminationMsg, container.restartCount}
	}
	return data
}
