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
	"fmt"
	"strconv"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
			Age:    timeFormat(time.Since(ns.CreationTimestamp.Time)),
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

// timeFormat format time data of `time.Duration` type to string type
func timeFormat(t time.Duration) string {
	str := t.String()
	// remove "."
	tmp := strings.Split(str, ".")
	tmp[0] += "s"

	tmp = strings.Split(tmp[0], "h")
	// hour num
	hour, err := strconv.Atoi(tmp[0])
	if err != nil {
		return ""
	}

	return fmt.Sprintf("%dd%dh%2s", hour/24, hour%24, tmp[1])
}
