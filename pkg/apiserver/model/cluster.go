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

func init() {
	RegistModel(&Cluster{})
}

// ProviderInfo describes the information from provider API
type ProviderInfo struct {
	Provider    string            `json:"provider"`
	ClusterName string            `json:"name"`
	ID          string            `json:"id"`
	Zone        string            `json:"zone"`
	Labels      map[string]string `json:"labels"`
}

const (
	// ClusterStatusHealthy healthy cluster
	ClusterStatusHealthy = "Healthy"
	// ClusterStatusUnhealthy unhealthy cluster
	ClusterStatusUnhealthy = "Unhealthy"
)

// Cluster describes the model of cluster in apiserver
type Cluster struct {
	Model
	Name        string            `json:"name"`
	Alias       string            `json:"alias"`
	Description string            `json:"description"`
	Icon        string            `json:"icon"`
	Labels      map[string]string `json:"labels"`
	Status      string            `json:"status"`
	Reason      string            `json:"reason"`

	Provider     ProviderInfo `json:"provider"`
	APIServerURL string       `json:"apiServerURL"`
	DashboardURL string       `json:"dashboardURL"`

	KubeConfig       string `json:"kubeConfig"`
	KubeConfigSecret string `json:"kubeConfigSecret"`
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
