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

// ClusterResourceInfo resource info of cluster
type ClusterResourceInfo struct {
	WorkerNumber     int      `json:"workerNumber"`
	MasterNumber     int      `json:"masterNumber"`
	MemoryCapacity   int64    `json:"memoryCapacity"`
	CPUCapacity      int64    `json:"cpuCapacity"`
	GPUCapacity      int64    `json:"gpuCapacity,omitempty"`
	PodCapacity      int64    `json:"podCapacity"`
	MemoryUsed       int64    `json:"memoryUsed"`
	CPUUsed          int64    `json:"cpuUsed"`
	GPUUsed          int64    `json:"gpuUsed,omitempty"`
	PodUsed          int64    `json:"podUsed"`
	StorageClassList []string `json:"storageClassList,omitempty"`
}

// Cluster describes the model of cluster in apiserver
type Cluster struct {
	Model
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Icon        string            `json:"icon"`
	Labels      map[string]string `json:"labels"`
	Status      string            `json:"status"`
	Reason      string            `json:"reason"`

	KubeConfig       string `json:"kubeConfig"`
	KubeConfigSecret string `json:"kubeConfigSecret"`

	ResourceInfo ClusterResourceInfo `json:"resourceInfo"`
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

// DeepCopy create a copy of cluster
func (c *Cluster) DeepCopy() *Cluster {
	return deepCopy(c).(*Cluster)
}
