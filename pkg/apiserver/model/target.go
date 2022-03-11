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
	RegisterModel(&Target{})
}

// Target defines the delivery target information for the application
// It includes kubernetes clusters or cloud service providers
type Target struct {
	BaseModel
	Name        string                 `json:"name"`
	Alias       string                 `json:"alias,omitempty"`
	Description string                 `json:"description,omitempty"`
	Cluster     *ClusterTarget         `json:"cluster,omitempty"`
	Variable    map[string]interface{} `json:"variable,omitempty"`
}

// TableName return custom table name
func (d *Target) TableName() string {
	return tableNamePrefix + "target"
}

// PrimaryKey return custom primary key
func (d *Target) PrimaryKey() string {
	return d.Name
}

// Index return custom index
func (d *Target) Index() map[string]string {
	index := make(map[string]string)
	if d.Name != "" {
		index["name"] = d.Name
	}
	return index
}

// ClusterTarget one kubernetes cluster delivery target
type ClusterTarget struct {
	ClusterName string `json:"clusterName" validate:"checkname"`
	Namespace   string `json:"namespace" optional:"true"`
}
