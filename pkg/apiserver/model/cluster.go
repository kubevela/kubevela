/*
Copyright 2021 The KubeVela Authors.

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

import v1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"

// Cluster describes the model of cluster in apiserver
type Cluster struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Icon        string            `json:"icon"`
	Labels      map[string]string `json:"labels"`
	Status      string            `json:"status"`
	Reason      string            `json:"reason"`

	KubeConfig       string `json:"kubeConfig"`
	KubeConfigSecret string `json:"kubeConfigSecret"`

	ResourceInfo v1.ClusterResourceInfo `json:"resourceInfo"`
}

// TableName table name for datastore
func (c *Cluster) TableName() string {
	return tableNamePrefix + "cluster"
}

// PrimaryKey primary key for datastore
func (c *Cluster) PrimaryKey() string {
	return c.Name
}

// Index set to nil for list
func (c *Cluster) Index() map[string]string {
	return nil
}

// ToClusterBase converts to ClusterBase
func (c *Cluster) ToClusterBase() *v1.ClusterBase {
	return &v1.ClusterBase{
		Name:        c.Name,
		Description: c.Description,
		Icon:        c.Icon,
		Labels:      c.Labels,
		Status:      c.Status,
		Reason:      c.Reason,
	}
}

// DeepCopy create a copy of cluster
func (c *Cluster) DeepCopy() *Cluster {
	return deepCopy(c).(*Cluster)
}
