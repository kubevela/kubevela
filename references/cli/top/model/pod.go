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
	"fmt"
	"strconv"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/velaql/providers/query"
	"github.com/oam-dev/kubevela/references/cli/top/utils"
)

// Pod represent the k8s pod resource instance
type Pod struct {
	Name      string
	Namespace string
	Ready     string
	Status    string
	CPU       string
	Mem       string
	CPUR      string
	CPUL      string
	MemR      string
	MemL      string
	IP        string
	NodeName  string
	Age       string
}

// PodList is pod list
type PodList struct {
	title []string
	data  []Pod
}

// ListPods return pod list of component
func ListPods(ctx context.Context, cfg *rest.Config, c client.Client) (*PodList, error) {
	list := &PodList{title: []string{"Name", "Namespace", "Ready", "Status", "CPU", "MEM", "%CPU/R", "%CPU/L", "%MEM/R", "%MEM/L", "IP", "Node", "Age"}, data: []Pod{}}
	appName := ctx.Value(&CtxKeyAppName).(string)
	appNamespace := ctx.Value(&CtxKeyNamespace).(string)
	compCluster := ctx.Value(&CtxKeyCluster).(string)
	compNamespace := ctx.Value(&CtxKeyClusterNamespace).(string)
	compName := ctx.Value(&CtxKeyComponentName).(string)

	opt := query.Option{
		Name:      appName,
		Namespace: appNamespace,
		Filter: query.FilterOption{
			Cluster:          compCluster,
			ClusterNamespace: compNamespace,
			Components:       []string{compName},
			APIVersion:       "v1",
			Kind:             "Pod",
		},
		WithTree: true,
	}
	resource, err := collectResource(ctx, c, opt)
	if err != nil {
		return list, err
	}
	for _, object := range resource {
		pod := &v1.Pod{}
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(object.UnstructuredContent(), pod)
		if err != nil {
			continue
		}
		list.data = append(list.data, LoadPodDetail(cfg, pod))
	}
	return list, nil
}

// LoadPodDetail gather the pod detail info
func LoadPodDetail(cfg *rest.Config, pod *v1.Pod) Pod {
	podInfo := Pod{
		Name:      pod.Name,
		Namespace: pod.Namespace,
		Ready:     readyContainerNum(pod),
		Status:    string(pod.Status.Phase),
		Age:       utils.TimeFormat(time.Since(pod.CreationTimestamp.Time)),
		IP:        pod.Status.PodIP,
		NodeName:  pod.Spec.NodeName,
	}
	metric, err := utils.PodMetric(cfg, pod.Name, pod.Namespace)
	if err != nil {
		podInfo.CPU, podInfo.Mem, podInfo.CPUL, podInfo.MemL, podInfo.CPUR, podInfo.MemR = utils.NA, utils.NA, utils.NA, utils.NA, utils.NA, utils.NA
	} else {
		c, r := utils.GatherPodMX(pod, metric)
		podInfo.CPU, podInfo.Mem = strconv.FormatInt(c.CPU, 10), strconv.FormatInt(c.Mem/1000000, 10)
		podInfo.CPUR = utils.ToPercentageStr(c.CPU, r.CPU)
		podInfo.MemR = utils.ToPercentageStr(c.Mem, r.Mem)
		podInfo.CPUL = utils.ToPercentageStr(c.CPU, r.Lcpu)
		podInfo.MemL = utils.ToPercentageStr(c.CPU, r.Lmem)
	}

	return podInfo
}

func readyContainerNum(pod *v1.Pod) string {
	total := len(pod.Status.ContainerStatuses)
	ready := 0
	for _, c := range pod.Status.ContainerStatuses {
		if c.Ready {
			ready++
		}
	}
	return fmt.Sprintf("%d/%d", ready, total)
}

// Header generate header of table in pod view
func (l *PodList) Header() []string {
	return l.title
}

// Body generate body of table in pod view
func (l *PodList) Body() [][]string {
	data := make([][]string, 0)
	for _, pod := range l.data {
		data = append(data, []string{pod.Name, pod.Namespace, pod.Ready, pod.Status, pod.CPU, pod.Mem, pod.CPUR, pod.MemR, pod.CPUL, pod.MemL, pod.IP, pod.NodeName, pod.Age})
	}
	return data
}
