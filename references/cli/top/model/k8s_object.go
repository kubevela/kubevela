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

	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/velaql/providers/query"
)

// K8SObject is k8s resource struct
type K8SObject struct {
	name       string
	namespace  string
	kind       string
	apiVersion string
	cluster    string
	status     string
}

// K8SObjectList is k8s struct resource list
type K8SObjectList struct {
	title []string
	data  []K8SObject
}

// Header generate header of table in k8s object view
func (l *K8SObjectList) Header() []string {
	return l.title
}

// Body generate header of table in k8s object view
func (l *K8SObjectList) Body() [][]string {
	data := make([][]string, 0)
	for _, app := range l.data {
		data = append(data, []string{app.name, app.namespace, app.kind, app.apiVersion, app.cluster, app.status})
	}
	return data
}

// ListObjects return k8s object resource list
func ListObjects(ctx context.Context, c client.Client) *K8SObjectList {
	list := &K8SObjectList{
		title: []string{"Name", "Namespace", "Kind", "APIVersion", "Cluster", "Status"},
	}
	name := ctx.Value(&CtxKeyAppName).(string)
	namespace := ctx.Value(&CtxKeyNamespace).(string)
	cluster := ctx.Value(&CtxKeyCluster).(string)

	opt := query.Option{
		Name:      name,
		Namespace: namespace,
	}
	if cluster != multicluster.ClusterLocalName {
		opt.Filter = query.FilterOption{Cluster: cluster}
	}
	collector := query.NewAppCollector(c, opt)
	appResList, err := collector.CollectResourceFromApp()

	if err != nil {
		return list
	}

	for _, resource := range appResList {
		list.data = append(list.data, LoadObjectDetail(resource))
	}

	return list
}

// LoadObjectDetail return the aim k8s object detail info
func LoadObjectDetail(resource query.Resource) K8SObject {
	object := K8SObject{
		name:       resource.Object.GetName(),
		namespace:  resource.Object.GetNamespace(),
		kind:       resource.Object.GetKind(),
		apiVersion: resource.Object.GetAPIVersion(),
		cluster:    resource.Cluster,
	}
	status, err := query.CheckResourceStatus(*resource.Object)
	if err == nil {
		object.status = string(status.Status)
	}
	return object
}
