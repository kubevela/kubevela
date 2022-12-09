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

	"github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
	"github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"
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

const velaVersionNumberToUpgradeVelaQL = "v1.2.0-rc.1"

// CollectResourceFromApp collect resources created by application
func (c *AppCollector) CollectResourceFromApp(ctx context.Context) ([]Resource, error) {
	app := new(v1beta1.Application)
	appKey := client.ObjectKey{Name: c.opt.Name, Namespace: c.opt.Namespace}
	if err := c.k8sClient.Get(ctx, appKey, app); err != nil {
		return nil, err
	}
	var currentVersionNumber string
	if annotations := app.GetAnnotations(); annotations != nil && annotations[oam.AnnotationKubeVelaVersion] != "" {
		currentVersionNumber = annotations[oam.AnnotationKubeVelaVersion]
	}
	velaVersionToUpgradeVelaQL, _ := version.NewVersion(velaVersionNumberToUpgradeVelaQL)
	currentVersion, err := version.NewVersion(currentVersionNumber)
	if err != nil {
		resources, err := c.FindResourceFromResourceTrackerSpec(ctx, app)
		if err != nil {
			return c.FindResourceFromAppliedResourcesField(ctx, app)
		}
		return resources, nil
	}

	if velaVersionToUpgradeVelaQL.GreaterThan(currentVersion) {
		return c.FindResourceFromAppliedResourcesField(ctx, app)
	}
	return c.FindResourceFromResourceTrackerSpec(ctx, app)
}

// ListApplicationResources list application applied resources from tracker
func (c *AppCollector) ListApplicationResources(ctx context.Context, app *v1beta1.Application) ([]*types.AppliedResource, error) {
	rootRT, currentRT, historyRTs, _, err := resourcetracker.ListApplicationResourceTrackers(ctx, c.k8sClient, app)
	if err != nil {
		return nil, err
	}
	var managedResources []*types.AppliedResource
	existResources := make(map[common.ClusterObjectReference]bool, len(app.Spec.Components))
	for _, rt := range append(historyRTs, rootRT, currentRT) {
		if rt != nil {
			for _, managedResource := range rt.Spec.ManagedResources {
				if isResourceInTargetCluster(c.opt.Filter, managedResource.ClusterObjectReference) &&
					isResourceInTargetComponent(c.opt.Filter, managedResource.Component) &&
					(c.opt.WithTree || isResourceMatchKindAndVersion(c.opt.Filter, managedResource.Kind, managedResource.APIVersion)) {
					if c.opt.WithTree {
						// If we want to query the tree, we only need to query once for the same resource.
						if _, exist := existResources[managedResource.ClusterObjectReference]; exist {
							continue
						}
						existResources[managedResource.ClusterObjectReference] = true
					}
					managedResources = append(managedResources, &types.AppliedResource{
						Cluster: func() string {
							if managedResource.Cluster != "" {
								return managedResource.Cluster
							}
							return "local"
						}(),
						Kind:            managedResource.Kind,
						Component:       managedResource.Component,
						Trait:           managedResource.Trait,
						Name:            managedResource.Name,
						Namespace:       managedResource.Namespace,
						APIVersion:      managedResource.APIVersion,
						ResourceVersion: managedResource.ResourceVersion,
						UID:             managedResource.UID,
						PublishVersion:  oam.GetPublishVersion(rt),
						DeployVersion: func() string {
							obj, _ := managedResource.ToUnstructuredWithData()
							if obj != nil {
								return oam.GetDeployVersion(obj)
							}
							return ""
						}(),
						Revision: rt.GetLabels()[oam.LabelAppRevision],
						Latest:   currentRT != nil && rt.Name == currentRT.Name,
					})
				}
			}
		}
	}

	if !c.opt.WithTree {
		return managedResources, nil
	}

	// merge user defined customize rule before every request.
	err = mergeCustomRules(ctx, c.k8sClient)
	if err != nil {
		return managedResources, err
	}

	filter := func(node types.ResourceTreeNode) bool {
		return isResourceMatchKindAndVersion(c.opt.Filter, node.Kind, node.APIVersion)
	}
	var matchedResources []*types.AppliedResource
	// error from leaf nodes won't block the results
	for i := range managedResources {
		resource := managedResources[i]
		root := types.ResourceTreeNode{
			Cluster:    resource.Cluster,
			APIVersion: resource.APIVersion,
			Kind:       resource.Kind,
			Namespace:  resource.Namespace,
			Name:       resource.Name,
			UID:        resource.UID,
		}
		root.LeafNodes, err = iterateListSubResources(ctx, resource.Cluster, c.k8sClient, root, 1, filter)
		if err != nil {
			// if the resource has been deleted, continue access next appliedResource don't break the whole request
			if kerrors.IsNotFound(err) {
				continue
			}
			klog.Errorf("query leaf node resource apiVersion=%s kind=%s namespace=%s name=%s failure %s, skip this resource", root.APIVersion, root.Kind, root.Namespace, root.Name, err.Error())
			continue
		}
		if !filter(root) && len(root.LeafNodes) == 0 {
			continue
		}
		rootObject, err := fetchObjectWithResourceTreeNode(ctx, resource.Cluster, c.k8sClient, root)
		if err != nil {
			// if the resource has been deleted, continue access next appliedResource don't break the whole request
			if kerrors.IsNotFound(err) {
				continue
			}
			klog.Errorf("fetch object for resource apiVersion=%s kind=%s namespace=%s name=%s failure %s, skip this resource", root.APIVersion, root.Kind, root.Namespace, root.Name, err.Error())
			continue
		}
		rootStatus, err := CheckResourceStatus(*rootObject)
		if err != nil {
			klog.Errorf("check status for resource apiVersion=%s kind=%s namespace=%s name=%s failure %s, skip this resource", root.APIVersion, root.Kind, root.Namespace, root.Name, err.Error())
			continue
		}
		root.HealthStatus = *rootStatus
		addInfo, err := additionalInfo(*rootObject)
		if err != nil {
			klog.Errorf("check additionalInfo for resource apiVersion=%s kind=%s namespace=%s name=%s failure %s, skip this resource", root.APIVersion, root.Kind, root.Namespace, root.Name, err.Error())
			continue
		}
		root.AdditionalInfo = addInfo
		root.CreationTimestamp = rootObject.GetCreationTimestamp().Time
		if !rootObject.GetDeletionTimestamp().IsZero() {
			root.DeletionTimestamp = rootObject.GetDeletionTimestamp().Time
		}
		root.Object = *rootObject
		resource.ResourceTree = &root
		matchedResources = append(matchedResources, resource)
	}
	return matchedResources, nil
}

// FindResourceFromResourceTrackerSpec find resources from ResourceTracker spec
func (c *AppCollector) FindResourceFromResourceTrackerSpec(ctx context.Context, app *v1beta1.Application) ([]Resource, error) {
	rootRT, currentRT, historyRTs, _, err := resourcetracker.ListApplicationResourceTrackers(ctx, c.k8sClient, app)
	if err != nil {
		klog.Errorf("query the resourcetrackers failure %s", err.Error())
		return nil, err
	}
	var resources = []Resource{}
	existResources := make(map[common.ClusterObjectReference]bool, len(app.Spec.Components))
	for _, rt := range append([]*v1beta1.ResourceTracker{rootRT, currentRT}, historyRTs...) {
		if rt != nil {
			for _, managedResource := range rt.Spec.ManagedResources {
				if isResourceInTargetCluster(c.opt.Filter, managedResource.ClusterObjectReference) &&
					isResourceInTargetComponent(c.opt.Filter, managedResource.Component) &&
					isResourceMatchKindAndVersion(c.opt.Filter, managedResource.Kind, managedResource.APIVersion) {
					if _, exist := existResources[managedResource.ClusterObjectReference]; exist {
						continue
					}
					existResources[managedResource.ClusterObjectReference] = true
					obj, err := managedResource.ToUnstructuredWithData()
					if err != nil || c.opt.WithStatus {
						// For the application with apply once policy, there is no data in RT.
						// IF the WithStatus is true, get the object from cluster
						_, obj, err = getObjectCreatedByComponent(ctx, c.k8sClient, managedResource.ObjectReference, managedResource.Cluster)
						if err != nil {
							klog.Errorf("get obj from the cluster failure %s", err.Error())
							continue
						}
					}
					clusterName := managedResource.Cluster
					if clusterName == "" {
						clusterName = multicluster.ClusterLocalName
					}
					resources = append(resources, Resource{
						Cluster:   clusterName,
						Revision:  oam.GetPublishVersion(rt),
						Component: managedResource.Component,
						Object:    obj,
					})
				}
			}
		}
	}
	return resources, nil
}

// FindResourceFromAppliedResourcesField find resources from AppliedResources field
func (c *AppCollector) FindResourceFromAppliedResourcesField(ctx context.Context, app *v1beta1.Application) ([]Resource, error) {
	resources := make([]Resource, 0, len(app.Spec.Components))
	for _, res := range app.Status.AppliedResources {
		if !isResourceInTargetCluster(c.opt.Filter, res) {
			continue
		}
		if !isResourceMatchKindAndVersion(c.opt.Filter, res.APIVersion, res.Kind) {
			continue
		}
		compName, obj, err := getObjectCreatedByComponent(ctx, c.k8sClient, res.ObjectReference, res.Cluster)
		if err != nil {
			return nil, err
		}
		if len(compName) != 0 && isResourceInTargetComponent(c.opt.Filter, compName) {
			resources = append(resources, Resource{
				Component: compName,
				Revision:  obj.GetLabels()[oam.LabelAppRevision],
				Cluster:   res.Cluster,
				Object:    obj,
			})
		}
	}
	if len(resources) == 0 {
		return nil, errors.Errorf("fail to find resources created by application: %v", c.opt.Name)
	}
	return resources, nil
}

// getObjectCreatedByComponent get k8s obj created by components
func getObjectCreatedByComponent(ctx context.Context, cli client.Client, objRef corev1.ObjectReference, cluster string) (string, *unstructured.Unstructured, error) {
	ctx = multicluster.ContextWithClusterName(ctx, cluster)
	obj := new(unstructured.Unstructured)
	obj.SetGroupVersionKind(objRef.GroupVersionKind())
	obj.SetNamespace(objRef.Namespace)
	obj.SetName(objRef.Name)
	if err := cli.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
		if kerrors.IsNotFound(err) {
			return "", nil, nil
		}
		return "", nil, err
	}
	componentName := obj.GetLabels()[oam.LabelAppComponent]
	return componentName, obj, nil
}

func getEventFieldSelector(obj *unstructured.Unstructured) fields.Selector {
	field := fields.Set{}
	field["involvedObject.name"] = obj.GetName()
	field["involvedObject.namespace"] = obj.GetNamespace()
	field["involvedObject.kind"] = obj.GetObjectKind().GroupVersionKind().Kind
	field["involvedObject.uid"] = string(obj.GetUID())
	return field.AsSelector()
}

func isResourceInTargetCluster(opt FilterOption, resource common.ClusterObjectReference) bool {
	if opt.Cluster == "" && opt.ClusterNamespace == "" {
		return true
	}
	if (opt.Cluster == resource.Cluster || (opt.Cluster == "local" && resource.Cluster == "")) &&
		(opt.ClusterNamespace == resource.ObjectReference.Namespace || opt.ClusterNamespace == "") {
		return true
	}

	return false
}

func isResourceInTargetComponent(opt FilterOption, componentName string) bool {
	if len(opt.Components) == 0 {
		return true
	}
	for _, component := range opt.Components {
		if component == componentName {
			return true
		}
	}
	return false
}

func isResourceMatchKindAndVersion(opt FilterOption, kind, version string) bool {
	if opt.APIVersion != "" && opt.APIVersion != version {
		return false
	}
	if opt.Kind != "" && opt.Kind != kind {
		return false
	}
	return true
}
