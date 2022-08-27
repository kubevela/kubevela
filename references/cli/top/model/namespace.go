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
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/references/cli/top/utils"
)

// Namespace is namespace struct
type Namespace struct {
	Name   string
	Status string
	Age    string
}

// AllNamespace is the key which represents all namespaces
const AllNamespace = "all"

// ListNamespaces return all namespaces
func ListNamespaces(ctx context.Context, c client.Reader) *NamespaceList {
	list := &NamespaceList{title: []string{"Name", "Status", "Age"}, data: []Namespace{{Name: AllNamespace, Status: "*", Age: "*"}}}
	var nsList v1.NamespaceList
	if err := c.List(ctx, &nsList); err != nil {
		return list
	}
	for _, ns := range nsList.Items {
		list.data = append(list.data, Namespace{
			Name:   ns.Name,
			Status: string(ns.Status.Phase),
			Age:    utils.TimeFormat(time.Since(ns.CreationTimestamp.Time)),
		})
	}
	return list
}

// Header generate header of table in namespace view
func (l *NamespaceList) Header() []string {
	return l.title
}

// Body generate body of table in namespace view
func (l *NamespaceList) Body() [][]string {
	data := make([][]string, 0)
	for _, ns := range l.data {
		data = append(data, []string{ns.Name, ns.Status, ns.Age})
	}
	return data
}
