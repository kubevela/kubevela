package model

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/velaql/providers/query"
)

type K8SObject struct {
	name       string
	namespace  string
	kind       string
	apiVersion string
	cluster    string
	status     string
}

type K8SObjectList struct {
	title []string
	data  []K8SObject
}

func (l *K8SObjectList) Header() []string {
	return l.title
}

func (l *K8SObjectList) Body() [][]string {
	data := make([][]string, 0)
	for _, app := range l.data {
		data = append(data, []string{app.name, app.namespace, app.kind, app.apiVersion, app.cluster, app.status})
	}
	return data
}

func ListObjects(ctx context.Context, c client.Client) *K8SObjectList {
	list := &K8SObjectList{
		title: []string{"Name", "Namespace", "Kind", "APIVersion", "Cluster", "Status"},
	}
	name, ok := ctx.Value("appName").(string)
	if !ok {
		return list
	}
	namespace, ok := ctx.Value("appNs").(string)
	if !ok {
		return list
	}
	cluster, ok := ctx.Value("cluster").(string)
	if !ok {
		return list
	}
	opt := query.Option{
		Name:      name,
		Namespace: namespace,
	}
	if cluster != "local" {
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
