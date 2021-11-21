/*
 Copyright 2021. The KubeVela Authors.

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

package query

import (
	"context"
	"fmt"
	"reflect"

	kruise "github.com/openkruise/kruise-api/apps/v1alpha1"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/dispatch"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

// AppCollector collect resource created by application
type AppCollector struct {
	k8sClient client.Client
	opt       Option
}

// NewAppCollector create a app collector
func NewAppCollector(cli client.Client, opt Option) *AppCollector {
	return &AppCollector{
		k8sClient: cli,
		opt:       opt,
	}
}

// CollectResourceFromApp collect resources created by application
func (c *AppCollector) CollectResourceFromApp() ([]AppResources, error) {
	if c.opt.EnableHistoryQuery {
		return c.CollectHistoryResourceFromApp()
	}
	return c.CollectLatestResourceFromApp()
}

// CollectLatestResourceFromApp collect resources created by latest application
func (c *AppCollector) CollectLatestResourceFromApp() ([]AppResources, error) {
	ctx := context.Background()
	app := new(v1beta1.Application)
	appKey := client.ObjectKey{Name: c.opt.Name, Namespace: c.opt.Namespace}
	if err := c.k8sClient.Get(ctx, appKey, app); err != nil {
		return nil, err
	}

	var revision int64
	if app.Status.LatestRevision != nil {
		revision = app.Status.LatestRevision.Revision
	}
	publishVersion := app.GetAnnotations()[oam.AnnotationPublishVersion]
	deployVersion := app.GetAnnotations()[oam.AnnotationDeployVersion]

	appRevName := fmt.Sprintf("%s-v%d", app.Name, revision)
	comps := make(map[string][]Resource, len(app.Spec.Components))
	for _, rsrcRef := range app.Status.AppliedResources {
		if !isTargetResource(c.opt.Filter, rsrcRef) {
			continue
		}
		compName, obj, err := getObjectCreatedByComponent(c.k8sClient, rsrcRef.ObjectReference, rsrcRef.Cluster, appRevName)
		if err != nil {
			return nil, err
		}
		if len(compName) == 0 {
			continue
		}
		comps[compName] = append(comps[compName], Resource{
			Cluster: rsrcRef.Cluster,
			Object:  obj,
		})
	}
	compResList := c.extractComponentResourceWithOption(comps)
	if len(compResList) == 0 {
		return nil, errors.Errorf("fail to find resources created by %v", c.opt.Components)
	}

	return []AppResources{{
		Revision:       revision,
		Metadata:       app.ObjectMeta,
		Components:     compResList,
		PublishVersion: publishVersion,
		DeployVersion:  deployVersion,
	}}, nil
}

// CollectHistoryResourceFromApp collect history resources created by application
func (c *AppCollector) CollectHistoryResourceFromApp() ([]AppResources, error) {
	var appResList []AppResources
	rts, err := listResourceTrackers(c.k8sClient, c.opt.Name, c.opt.Namespace)
	if err != nil {
		return nil, err
	}
	appResList = make([]AppResources, 0, len(rts))
	for _, rt := range rts {
		if len(rt.Status.TrackedResources) == 0 {
			continue
		}
		appRevName := dispatch.ExtractAppRevisionName(rt.Name, c.opt.Namespace)
		revision, err := oamutil.ExtractRevisionNum(appRevName, "-")
		if err != nil {
			return nil, err
		}
		comps := make(map[string][]Resource)
		for _, trackedResourceRef := range rt.Status.TrackedResources {
			compName, obj, err := getObjectCreatedByComponent(c.k8sClient, trackedResourceRef, "", appRevName)
			if err != nil {
				return nil, err
			}
			if len(compName) == 0 {
				continue
			}
			comps[compName] = append(comps[compName], Resource{
				Cluster: "",
				Object:  obj,
			})
		}
		compResList := c.extractComponentResourceWithOption(comps)
		if len(compResList) != 0 {
			appResList = append(appResList, AppResources{
				Revision:   int64(revision),
				Metadata:   rt.ObjectMeta,
				Components: compResList,
			})
		}
	}
	if len(appResList) == 0 {
		return nil, errors.Errorf("fail to find resources created by %v", c.opt.Components)
	}
	return appResList, nil
}

func (c *AppCollector) extractComponentResourceWithOption(comps map[string][]Resource) []Component {
	var result []Component

	// if not specify component, return all components resource created by app
	if len(c.opt.Components) == 0 {
		for name, resource := range comps {
			if len(resource) == 0 {
				continue
			}
			result = append(result, Component{
				Name:      name,
				Resources: resource,
			})
		}
		return result
	}

	for _, compName := range c.opt.Components {
		if len(comps[compName]) == 0 {
			continue
		}
		result = append(result, Component{
			Name:      compName,
			Resources: comps[compName],
		})
	}
	return result
}

// listResourceTrackers list all resourceTracker with specified app
func listResourceTrackers(cli client.Client, appName, appNs string) ([]v1beta1.ResourceTracker, error) {
	listOpts := []client.ListOption{
		client.MatchingLabels{
			oam.LabelAppName:      appName,
			oam.LabelAppNamespace: appNs,
		}}
	rtList := &v1beta1.ResourceTrackerList{}
	ctx := context.Background()
	if err := cli.List(ctx, rtList, listOpts...); err != nil {
		klog.ErrorS(err, "Failed to list Resource tracker of app", "name", appName)
		return nil, err
	}
	return rtList.Items, nil
}

// getObjectCreatedByComponent get k8s obj created by components
func getObjectCreatedByComponent(cli client.Client, objRef corev1.ObjectReference, cluster string, appRevName string) (componentName string, obj *unstructured.Unstructured, err error) {
	ctx := multicluster.ContextWithClusterName(context.Background(), cluster)
	obj = new(unstructured.Unstructured)
	obj.SetGroupVersionKind(objRef.GroupVersionKind())
	obj.SetNamespace(objRef.Namespace)
	obj.SetName(objRef.Name)

	key := client.ObjectKeyFromObject(obj)
	if key.Namespace == "" {
		key.Namespace = "default"
	}
	if err = cli.Get(ctx, key, obj); err != nil {
		if kerrors.IsNotFound(err) {
			return "", nil, nil
		}
		return "", nil, err
	}
	if obj.GetLabels()[oam.LabelAppRevision] != appRevName {
		return
	}
	componentName = obj.GetLabels()[oam.LabelAppComponent]
	return
}

var standardWorkloads = []schema.GroupVersionKind{
	appsv1.SchemeGroupVersion.WithKind(reflect.TypeOf(appsv1.Deployment{}).Name()),
	appsv1.SchemeGroupVersion.WithKind(reflect.TypeOf(appsv1.ReplicaSet{}).Name()),
	appsv1.SchemeGroupVersion.WithKind(reflect.TypeOf(appsv1.StatefulSet{}).Name()),
	appsv1.SchemeGroupVersion.WithKind(reflect.TypeOf(appsv1.DaemonSet{}).Name()),
	batchv1.SchemeGroupVersion.WithKind(reflect.TypeOf(batchv1.Job{}).Name()),
	kruise.SchemeGroupVersion.WithKind(reflect.TypeOf(kruise.CloneSet{}).Name()),
}

var podCollectorMap = map[schema.GroupVersionKind]PodCollector{
	batchv1.SchemeGroupVersion.WithKind(reflect.TypeOf(batchv1.CronJob{}).Name()):           cronJobPodCollector,
	batchv1beta1.SchemeGroupVersion.WithKind(reflect.TypeOf(batchv1beta1.CronJob{}).Name()): cronJobPodCollector,
}

// PodCollector collector pod created by workload
type PodCollector func(cli client.Client, obj *unstructured.Unstructured, cluster string) ([]*unstructured.Unstructured, error)

// NewPodCollector create a PodCollector
func NewPodCollector(gvk schema.GroupVersionKind) PodCollector {
	for _, workload := range standardWorkloads {
		if gvk == workload {
			return standardWorkloadPodCollector
		}
	}
	if collector, ok := podCollectorMap[gvk]; ok {
		return collector
	}
	return func(cli client.Client, obj *unstructured.Unstructured, cluster string) ([]*unstructured.Unstructured, error) {
		return nil, nil
	}
}

// standardWorkloadPodCollector collect pods created by standard workload
func standardWorkloadPodCollector(cli client.Client, obj *unstructured.Unstructured, cluster string) ([]*unstructured.Unstructured, error) {
	ctx := multicluster.ContextWithClusterName(context.Background(), cluster)
	selectorPath := []string{"spec", "selector", "matchLabels"}
	labels, found, err := unstructured.NestedStringMap(obj.UnstructuredContent(), selectorPath...)

	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.Errorf("fail to find matchLabels from %s %s", obj.GroupVersionKind().String(), klog.KObj(obj))
	}

	listOpts := []client.ListOption{
		client.MatchingLabels(labels),
		client.InNamespace(obj.GetNamespace()),
	}

	podList := corev1.PodList{}
	if err := cli.List(ctx, &podList, listOpts...); err != nil {
		return nil, err
	}

	pods := make([]*unstructured.Unstructured, len(podList.Items))
	for i := range podList.Items {
		pod, err := oamutil.Object2Unstructured(podList.Items[i])
		if err != nil {
			return nil, err
		}
		pod.SetGroupVersionKind(
			corev1.SchemeGroupVersion.WithKind(
				reflect.TypeOf(corev1.Pod{}).Name(),
			),
		)
		pods[i] = pod
	}
	return pods, nil
}

// cronJobPodCollector collect pods created by cronjob
func cronJobPodCollector(cli client.Client, obj *unstructured.Unstructured, cluster string) ([]*unstructured.Unstructured, error) {
	ctx := multicluster.ContextWithClusterName(context.Background(), cluster)

	jobList := new(batchv1.JobList)
	if err := cli.List(ctx, jobList, client.InNamespace(obj.GetNamespace())); err != nil {
		return nil, err
	}

	uid := obj.GetUID()
	var jobs []batchv1.Job
	for _, job := range jobList.Items {
		for _, owner := range job.GetOwnerReferences() {
			if owner.Kind == reflect.TypeOf(batchv1.CronJob{}).Name() && owner.UID == uid {
				jobs = append(jobs, job)
			}
		}
	}
	var pods []*unstructured.Unstructured
	podGVK := corev1.SchemeGroupVersion.WithKind(reflect.TypeOf(corev1.Pod{}).Name())
	for _, job := range jobs {
		labels := job.Spec.Selector.MatchLabels
		listOpts := []client.ListOption{
			client.MatchingLabels(labels),
			client.InNamespace(job.GetNamespace()),
		}
		podList := corev1.PodList{}
		if err := cli.List(ctx, &podList, listOpts...); err != nil {
			return nil, err
		}

		items := make([]*unstructured.Unstructured, len(podList.Items))
		for i := range podList.Items {
			pod, err := oamutil.Object2Unstructured(podList.Items[i])
			if err != nil {
				return nil, err
			}
			pod.SetGroupVersionKind(podGVK)
			items[i] = pod
		}
		pods = append(pods, items...)
	}
	return pods, nil
}

// HelmReleaseCollector HelmRelease resources collector
type HelmReleaseCollector struct {
	matchLabels  map[string]string
	workloadsGVK []schema.GroupVersionKind
	cli          client.Client
}

// NewHelmReleaseCollector create a HelmRelease collector
func NewHelmReleaseCollector(cli client.Client, hr *unstructured.Unstructured) *HelmReleaseCollector {
	return &HelmReleaseCollector{
		// matchLabels for resources created by HelmRelease refer to
		// https://github.com/fluxcd/helm-controller/blob/main/internal/runner/post_renderer_origin_labels.go#L31
		matchLabels: map[string]string{
			"helm.toolkit.fluxcd.io/name":      hr.GetName(),
			"helm.toolkit.fluxcd.io/namespace": hr.GetNamespace(),
		},
		workloadsGVK: []schema.GroupVersionKind{
			appsv1.SchemeGroupVersion.WithKind(reflect.TypeOf(appsv1.Deployment{}).Name()),
		},
		cli: cli,
	}
}

// CollectWorkloads collect workloads of HelmRelease
func (c *HelmReleaseCollector) CollectWorkloads(cluster string) ([]*unstructured.Unstructured, error) {
	ctx := multicluster.ContextWithClusterName(context.Background(), cluster)
	listOptions := []client.ListOption{
		client.MatchingLabels(c.matchLabels),
	}
	var workloads []*unstructured.Unstructured
	for _, workloadGVK := range c.workloadsGVK {
		unstructuredObjList := &unstructured.UnstructuredList{}
		unstructuredObjList.SetGroupVersionKind(workloadGVK)
		if err := c.cli.List(ctx, unstructuredObjList, listOptions...); err != nil {
			return nil, err
		}
		items := unstructuredObjList.Items
		for i := range items {
			items[i].SetGroupVersionKind(workloadGVK)
			workloads = append(workloads, &items[i])
		}
	}
	return workloads, nil
}

// helmReleasePodCollector collect pods created by helmRelease
func helmReleasePodCollector(cli client.Client, obj *unstructured.Unstructured, cluster string) ([]*unstructured.Unstructured, error) {
	hc := NewHelmReleaseCollector(cli, obj)
	workloads, err := hc.CollectWorkloads(cluster)
	if err != nil {
		return nil, err
	}
	var pods []*unstructured.Unstructured
	for _, workload := range workloads {
		collector := NewPodCollector(workload.GroupVersionKind())
		podList, err := collector(cli, workload, cluster)
		if err != nil {
			return nil, err
		}
		pods = append(pods, podList...)
	}
	return pods, nil
}

func getEventFieldSelector(obj *unstructured.Unstructured) fields.Selector {
	field := fields.Set{}
	field["involvedObject.name"] = obj.GetName()
	field["involvedObject.namespace"] = obj.GetNamespace()
	field["involvedObject.kind"] = obj.GetObjectKind().GroupVersionKind().Kind
	field["involvedObject.uid"] = string(obj.GetUID())
	return field.AsSelector()
}

func isTargetResource(opt ClusterFilter, resource common.ClusterObjectReference) bool {
	if opt.Cluster == "" && opt.ClusterNamespace == "" {
		return true
	}
	if opt.Cluster == resource.Cluster && opt.ClusterNamespace == resource.ObjectReference.Namespace {
		return true
	}
	return false
}
