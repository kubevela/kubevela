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

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appv1beta1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/velaql/providers/query"
)

// ApplicationMetrics is the metrics of application
type ApplicationMetrics struct {
	Status   *ApplicationMetricsStatus
	Resource *ApplicationResourceStatus
}

// ApplicationMetricsStatus is the status of application metrics
type ApplicationMetricsStatus struct {
	CPU     uint64
	Memory  uint64
	Storage uint64
}

// ApplicationResourceStatus is the status of application resource
type ApplicationResourceStatus struct {
	NodeNum        int
	ClusterNum     int
	SubresourceNum int
	PodNum         int
	ContainerNum   int
}

// GetPodMetrics get pod metrics
func GetPodMetrics(metricsClient metricsclientset.Interface, pod v1.Pod, allNamespaces bool) (*v1beta1.PodMetrics, error) {
	ns := metav1.NamespaceAll
	if !allNamespaces {
		ns = pod.Namespace
	}
	m, err := metricsClient.MetricsV1beta1().PodMetricses(ns).Get(context.Background(), pod.Name, metav1.GetOptions{})
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

// LoadApplicationMetrics load application metrics
func (appMetrics *ApplicationMetricsStatus) LoadApplicationMetrics(c client.Client, conf *rest.Config, pods []v1.Pod) {
	metricsClient := metricsclientset.NewForConfigOrDie(conf)
	cpu, memory, stroage := uint64(0), uint64(0), uint64(0)

	for _, pod := range pods {
		podMetrics, err := GetPodMetrics(metricsClient, pod, false)
		if err != nil {
			continue
		}

		for _, containerMetrics := range podMetrics.Containers {
			cpu += uint64(containerMetrics.Usage.Cpu().MilliValue())
			memory += uint64(containerMetrics.Usage.Memory().Value() / (1024 * 1024))
		}

		storages := GetPodStorage(c, pod)
		for _, s := range storages {
			stroage += uint64(s.Status.Capacity.Storage().Value() / (1024 * 1024 * 1024))
		}
	}
	appMetrics.CPU = cpu
	appMetrics.Memory = memory
	appMetrics.Storage = stroage
}

// LoadApplicationMetrics load application resource metrics
func LoadApplicationMetrics(c client.Client, conf *rest.Config, app *appv1beta1.Application) (*ApplicationMetrics, error) {
	appResList, err := ListApplicationResource(c, app.Name, app.Namespace)
	if err != nil {
		return nil, err
	}
	components := make([]string, 0)
	for _, resource := range appResList {
		components = append(components, resource.Object.GetName())
	}
	pods := ListApplicationPods(c, app, components)

	appMetrics := &ApplicationMetricsStatus{}
	appMetrics.LoadApplicationMetrics(c, conf, pods)

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

	appResource := &ApplicationResourceStatus{}
	appResource.NodeNum = len(nodes)
	appResource.ClusterNum = len(clusters)
	appResource.SubresourceNum = len(appResList)
	appResource.PodNum = len(pods)
	appResource.ContainerNum = containerNum

	return &ApplicationMetrics{
		Status:   appMetrics,
		Resource: appResource,
	}, nil
}
