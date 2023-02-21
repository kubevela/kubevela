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

package common

import (
	"context"
	"math"
	"strconv"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pkgmulticluster "github.com/kubevela/pkg/multicluster"

	appv1beta1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/velaql/providers/query"
)

const (
	// MetricsNA is the value of metrics when it is not available
	MetricsNA = "N/A"
)

// ApplicationMetrics is the metrics of application
type ApplicationMetrics struct {
	Metrics     *ApplicationMetricsStatus
	ResourceNum *ApplicationResourceNum
}

// ApplicationMetricsStatus is the status of application metrics
type ApplicationMetricsStatus struct {
	CPUUsage      int64
	CPURequest    int64
	CPULimit      int64
	MemoryUsage   int64
	MemoryRequest int64
	MemoryLimit   int64
	Storage       int64
}

// ApplicationResourceNum is the resource number of application
type ApplicationResourceNum struct {
	Node        int
	Cluster     int
	Subresource int
	Pod         int
	Container   int
}

// MetricLR is the metric of resource requests and limits
type MetricLR struct {
	CPU, Mem   int64
	Lcpu, Lmem int64
}

// GetPodMetricsLR return the usage metrics of a pod and specified metric including requests and limits metrics
func GetPodMetricsLR(pod *v1.Pod, mx *v1beta1.PodMetrics) (MetricLR, MetricLR) {
	var c, r MetricLR
	rcpu, rmem := podRequests(pod.Spec)
	lcpu, lmem := podLimits(pod.Spec)
	r.CPU, r.Lcpu, r.Mem, r.Lmem = rcpu.MilliValue(), lcpu.MilliValue(), rmem.Value(), lmem.Value()

	if mx != nil {
		ccpu, cmem := podUsage(mx)
		c.CPU, c.Mem = ccpu.MilliValue(), cmem.Value()
	}
	return c, r
}

func podUsage(metrics *v1beta1.PodMetrics) (*resource.Quantity, *resource.Quantity) {
	cpu, mem := new(resource.Quantity), new(resource.Quantity)
	for _, co := range metrics.Containers {
		usage := co.Usage

		if len(usage) == 0 {
			continue
		}
		if usage.Cpu() != nil {
			cpu.Add(*usage.Cpu())
		}
		if co.Usage.Memory() != nil {
			mem.Add(*usage.Memory())
		}
	}
	return cpu, mem
}

func podLimits(spec v1.PodSpec) (*resource.Quantity, *resource.Quantity) {
	cpu, mem := new(resource.Quantity), new(resource.Quantity)
	for _, co := range spec.Containers {
		limits := co.Resources.Limits
		if len(limits) == 0 {
			continue
		}
		if limits.Cpu() != nil {
			cpu.Add(*limits.Cpu())
		}
		if limits.Memory() != nil {
			mem.Add(*limits.Memory())
		}
	}
	return cpu, mem
}

func podRequests(spec v1.PodSpec) (*resource.Quantity, *resource.Quantity) {
	cpu, mem := new(resource.Quantity), new(resource.Quantity)
	for _, co := range spec.Containers {
		req := co.Resources.Requests
		if len(req) == 0 {
			continue
		}
		if req.Cpu() != nil {
			cpu.Add(*req.Cpu())
		}
		if req.Memory() != nil {
			mem.Add(*req.Memory())
		}
	}
	return cpu, mem
}

// ToPercentage computes percentage as string otherwise n/aa.
func ToPercentage(v1, v2 int64) int {
	if v2 == 0 {
		return 0
	}
	return int(math.Floor((float64(v1) / float64(v2)) * 100))
}

// ToPercentageStr computes percentage, but if v2 is 0, it will return NAValue instead of 0.
func ToPercentageStr(v1, v2 int64) string {
	if v2 == 0 {
		return MetricsNA
	}
	return strconv.Itoa(ToPercentage(v1, v2)) + "%"
}

// GetPodMetrics get pod metrics object
func GetPodMetrics(conf *rest.Config, podName, namespace, cluster string) (*v1beta1.PodMetrics, error) {
	ctx := multicluster.ContextWithClusterName(context.Background(), cluster)
	conf.Wrap(pkgmulticluster.NewTransportWrapper())
	metricsClient := metricsclientset.NewForConfigOrDie(conf)
	m, err := metricsClient.MetricsV1beta1().PodMetricses(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return m, nil
}

// GetPodStorage get pod storage
func GetPodStorage(client client.Client, pod v1.Pod) (storages []v1.PersistentVolumeClaim) {
	for _, v := range pod.Spec.Volumes {
		if v.PersistentVolumeClaim != nil {
			storage := v1.PersistentVolumeClaim{}
			err := client.Get(context.Background(), types.NamespacedName{Name: v.PersistentVolumeClaim.ClaimName, Namespace: pod.Namespace}, &storage)
			if err != nil {
				continue
			}
			storages = append(storages, storage)
		}
	}
	return
}

// ListApplicationResource list application resource
func ListApplicationResource(c client.Client, name, namespace string) ([]query.Resource, error) {
	opt := query.Option{
		Name:      name,
		Namespace: namespace,
		Filter:    query.FilterOption{},
	}
	collector := query.NewAppCollector(c, opt)
	appResList, err := collector.CollectResourceFromApp(context.Background())
	if err != nil {
		return []query.Resource{}, err
	}
	return appResList, err
}

// ListApplicationPods list application pods
func ListApplicationPods(c client.Client, app *appv1beta1.Application, components []string) []v1.Pod {
	pods := make([]v1.Pod, 0)
	opt := query.Option{
		Name:      app.Name,
		Namespace: app.Namespace,
		Filter: query.FilterOption{
			Components: components,
			APIVersion: "v1",
			Kind:       "Pod",
		},
		WithTree: true,
	}
	objects, err := CollectApplicationResource(context.Background(), c, opt)
	if err != nil {
		return pods
	}
	for _, object := range objects {
		pod := v1.Pod{}
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(object.UnstructuredContent(), &pod)
		if err != nil {
			continue
		}
		pods = append(pods, pod)
	}
	return pods
}

// GetApplicationMetricsStatus get application metrics status
func GetApplicationMetricsStatus(c client.Client, conf *rest.Config, pods []v1.Pod) *ApplicationMetricsStatus {
	metricsStatus := &ApplicationMetricsStatus{}
	cpuUsage, memoryUsage, storage := int64(0), int64(0), int64(0)
	cpuLimit, cpuRequest := int64(0), int64(0)
	memLimit, memRequest := int64(0), int64(0)
	for _, pod := range pods {
		podMetrics, err := GetPodMetrics(conf, pod.Name, pod.Namespace, "")
		if err != nil {
			continue
		}
		// get pod CPU and Memory usage
		cu, mu := podUsage(podMetrics)
		cpuUsage += cu.MilliValue()
		memoryUsage += mu.Value()
		// get pod CPU and Memory limit and request
		cl, ml := podLimits(pod.Spec)
		cr, mr := podRequests(pod.Spec)
		cpuLimit += cl.MilliValue()
		cpuRequest += cr.MilliValue()
		memLimit += ml.Value()
		memRequest += mr.Value()
		// get pod storage
		storages := GetPodStorage(c, pod)
		for _, s := range storages {
			storage += s.Status.Capacity.Storage().Value()
		}
	}

	metricsStatus.CPUUsage = cpuUsage
	metricsStatus.CPULimit = cpuLimit
	metricsStatus.CPURequest = cpuRequest
	metricsStatus.MemoryUsage = memoryUsage / (1024 * 1024)
	metricsStatus.MemoryLimit = memLimit / (1024 * 1024)
	metricsStatus.MemoryRequest = memRequest / (1024 * 1024)
	metricsStatus.Storage = storage / (1024 * 1024 * 1024)

	return metricsStatus
}

// GetApplicationMetrics get application metrics
func GetApplicationMetrics(c client.Client, conf *rest.Config, app *appv1beta1.Application) (*ApplicationMetrics, error) {
	appResList, err := ListApplicationResource(c, app.Name, app.Namespace)
	if err != nil {
		return nil, err
	}
	components := make([]string, 0)
	for _, res := range appResList {
		components = append(components, res.Object.GetName())
	}
	pods := ListApplicationPods(c, app, components)
	clusters := make(map[string]struct{})
	nodes := make(map[string]struct{})
	containerNum := 0
	for _, r := range appResList {
		clusters[r.Cluster] = struct{}{}
	}
	for _, pod := range pods {
		nodes[pod.Spec.NodeName] = struct{}{}
		containerNum += len(pod.Spec.Containers)
	}

	appResource := &ApplicationResourceNum{
		Cluster:     len(clusters),
		Node:        len(nodes),
		Subresource: len(appResList),
		Pod:         len(pods),
		Container:   containerNum,
	}
	appMetrics := GetApplicationMetricsStatus(c, conf, pods)
	return &ApplicationMetrics{
		Metrics:     appMetrics,
		ResourceNum: appResource,
	}, nil
}
