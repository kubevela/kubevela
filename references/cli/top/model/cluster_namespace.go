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
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ListClusterNamespaces return namespace of application's resource
func ListClusterNamespaces(ctx context.Context, c client.Client) ResourceList {
	list := &NamespaceList{
		title: []string{"Name", "Status", "Age"},
		data:  []Namespace{{"all", "*", "*"}},
	}
	name := ctx.Value(&CtxKeyAppName).(string)
	ns := ctx.Value(&CtxKeyNamespace).(string)
	app, err := LoadApplication(c, name, ns)
	if err != nil {
		return list
	}
	clusterNSSet := make(map[string]interface{})
	for _, svc := range app.Status.AppliedResources {
		if svc.Namespace != "" {
			clusterNSSet[svc.Namespace] = struct{}{}
		}
	}

	for clusterNS := range clusterNSSet {
		namespaceInfo := LoadNamespaceDetail(ctx, c, clusterNS)
		if namespaceInfo != nil {
			list.data = append(list.data, Namespace{
				Name:   namespaceInfo.Name,
				Status: string(namespaceInfo.Status.Phase),
				Age:    timeFormat(time.Since(namespaceInfo.CreationTimestamp.Time)),
			})
		}
	}

	return list
}

// LoadNamespaceDetail query detail info of a namespace by name
func LoadNamespaceDetail(ctx context.Context, c client.Client, namespace string) *v1.Namespace {
	ns := new(v1.Namespace)
	if err := c.Get(ctx, types.NamespacedName{Name: namespace}, ns); err != nil {
		return nil
	}
	return ns
}
