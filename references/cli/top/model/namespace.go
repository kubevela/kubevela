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
	name   string
	status string
	age    string
}

// NamespaceList is namespace list
type NamespaceList []Namespace

// ListNamespaces return all namespaces
func ListNamespaces(ctx context.Context, c client.Client) (NamespaceList, error) {
	var nsList v1.NamespaceList
	if err := c.List(ctx, &nsList); err != nil {
		return NamespaceList{}, err
	}
	nsInfoList := make(NamespaceList, len(nsList.Items))
	for index, ns := range nsList.Items {
		nsInfoList[index] = Namespace{
			name:   ns.Name,
			status: string(ns.Status.Phase),
			age:    utils.TimeFormat(time.Since(ns.CreationTimestamp.Time)),
		}
	}
	return nsInfoList, nil
}

// ToTableBody generate body of table in namespace view
func (l NamespaceList) ToTableBody() [][]string {
	data := make([][]string, len(l)+1)
	data[0] = []string{"all", "*", "*"}
	for index, ns := range l {
		data[index+1] = []string{ns.name, ns.status, ns.age}
	}
	return data
}
