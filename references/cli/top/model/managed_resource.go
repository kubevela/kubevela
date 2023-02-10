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

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/velaql/providers/query"
	"github.com/oam-dev/kubevela/references/common"
)

// ManagedResource is managed resource of application
type ManagedResource struct {
	name       string
	namespace  string
	kind       string
	apiVersion string
	cluster    string
	component  string
	status     string
}

// ManagedResourceList is managed resource list
type ManagedResourceList []ManagedResource

// ListManagedResource return managed resources of application
func ListManagedResource(ctx context.Context, c client.Client) (ManagedResourceList, error) {
	name := ctx.Value(&CtxKeyAppName).(string)
	namespace := ctx.Value(&CtxKeyNamespace).(string)
	appResList, err := common.ListApplicationResource(c, name, namespace)
	if err != nil {
		return ManagedResourceList{}, err
	}

	list := make(ManagedResourceList, len(appResList))

	for index, resource := range appResList {
		list[index] = LoadResourceDetail(resource)
	}

	cluster, ok := ctx.Value(&CtxKeyCluster).(string)
	if ok && cluster != "" {
		list.FilterCluster(cluster)
	}

	clusterNamespace, ok := ctx.Value(&CtxKeyClusterNamespace).(string)
	if ok && clusterNamespace != "" {
		list.FilterClusterNamespace(clusterNamespace)
	}

	return list, nil
}

// ToTableBody generate header of table in managed resource view
func (l *ManagedResourceList) ToTableBody() [][]string {
	data := make([][]string, 0)
	for _, resource := range *l {
		data = append(data, []string{resource.name, resource.namespace, resource.kind, resource.apiVersion, resource.cluster, resource.component, resource.status})
	}
	return data
}

// LoadResourceDetail return the aim resource detail info
func LoadResourceDetail(resource query.Resource) ManagedResource {
	if resource.Object == nil {
		return ManagedResource{}
	}
	object := ManagedResource{
		name:       resource.Object.GetName(),
		namespace:  resource.Object.GetNamespace(),
		kind:       resource.Object.GetKind(),
		apiVersion: resource.Object.GetAPIVersion(),
		cluster:    resource.Cluster,
		component:  resource.Component,
	}
	status, err := query.CheckResourceStatus(*resource.Object)
	if err == nil {
		object.status = string(status.Status)
	}
	return object
}

// FilterCluster filter out objects that belong to the target cluster
func (l *ManagedResourceList) FilterCluster(clusterName string) {
	data := make([]ManagedResource, 0)
	for _, resource := range *l {
		if resource.cluster == clusterName {
			data = append(data, ManagedResource{resource.name, resource.namespace, resource.kind, resource.apiVersion, resource.cluster, resource.component, resource.status})
		}
	}
	*l = data
}

// FilterClusterNamespace filter out objects that belong to the target namespace
func (l *ManagedResourceList) FilterClusterNamespace(clusterNS string) {
	data := make([]ManagedResource, 0)
	for _, resource := range *l {
		if resource.namespace == clusterNS {
			data = append(data, ManagedResource{resource.name, resource.namespace, resource.kind, resource.apiVersion, resource.cluster, resource.component, resource.status})
		}
	}
	*l = data
}
