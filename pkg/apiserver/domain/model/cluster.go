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

import (
	"time"

	"github.com/oam-dev/kubevela/pkg/multicluster"
)

func init() {
	RegisterModel(&Cluster{})
}

// ProviderInfo describes the information from provider API
type ProviderInfo struct {
	Provider    string            `json:"provider"`
	ClusterID   string            `json:"clusterID"`
	ClusterName string            `json:"clusterName,omitempty"`
	Zone        string            `json:"zone,omitempty"`
	ZoneID      string            `json:"zoneID,omitempty"`
	RegionID    string            `json:"regionID,omitempty"`
	VpcID       string            `json:"vpcID,omitempty"`
	Labels      map[string]string `json:"labels"`
}

const (
	// ClusterStatusHealthy healthy cluster
	ClusterStatusHealthy = "Healthy"
	// ClusterStatusUnhealthy unhealthy cluster
	ClusterStatusUnhealthy = "Unhealthy"
)

var (
	// LocalClusterCreatedTime create time for local cluster, set to late date in order to ensure it is sorted to first
	LocalClusterCreatedTime = time.Date(2999, 1, 1, 0, 0, 0, 0, time.UTC)
)

// Cluster describes the model of cluster in apiserver
type Cluster struct {
	BaseModel
	Name             string            `json:"name"`
	Alias            string            `json:"alias"`
	Description      string            `json:"description"`
	Icon             string            `json:"icon"`
	Labels           map[string]string `json:"labels"`
	Status           string            `json:"status"`
	Reason           string            `json:"reason"`
	Provider         ProviderInfo      `json:"provider"`
	APIServerURL     string            `json:"apiServerURL"`
	DashboardURL     string            `json:"dashboardURL"`
	KubeConfig       string            `json:"kubeConfig"`
	KubeConfigSecret string            `json:"kubeConfigSecret"`
}

// SetCreateTime for local cluster, create time is set to a large date which ensures the order of list
func (c *Cluster) SetCreateTime(t time.Time) {
	if c.Name == multicluster.ClusterLocalName {
		c.CreateTime = LocalClusterCreatedTime
		c.SetUpdateTime(t)
	} else {
		c.CreateTime = t
	}
}

// TableName table name for datastore
func (c *Cluster) TableName() string {
	return tableNamePrefix + "cluster"
}

// ShortTableName is the compressed version of table name for kubeapi storage and others
func (c *Cluster) ShortTableName() string {
	return "cls"
}

// PrimaryKey primary key for datastore
func (c *Cluster) PrimaryKey() string {
	return c.Name
}

// Index set to nil for list
func (c *Cluster) Index() map[string]string {
	index := make(map[string]string)
	if c.Name != "" {
		index["name"] = c.Name
	}
	return index
}

// DeepCopy create a copy of cluster
func (c *Cluster) DeepCopy() *Cluster {
	return deepCopy(c).(*Cluster)
}
