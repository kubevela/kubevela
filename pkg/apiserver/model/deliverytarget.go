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
	RegistModel(&DeliveryTarget{})
}

// DeliveryTarget defines the delivery target information for the application
// It includes kubernetes clusters or cloud service providers
type DeliveryTarget struct {
	Model
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Alias       string            `json:"alias,omitempty"`
	Description string            `json:"description,omitempty"`
	Kubernetes  *KubernetesTarget `json:"kubernetes,omitempty"`
	Cloud       *CloudTarget      `json:"cloud,omitempty"`
}

// TableName return custom table name
func (d *DeliveryTarget) TableName() string {
	return tableNamePrefix + "delivery_target"
}

// PrimaryKey return custom primary key
func (d *DeliveryTarget) PrimaryKey() string {
	return d.Name
}

// Index return custom index
func (d *DeliveryTarget) Index() map[string]string {
	index := make(map[string]string)
	if d.Name != "" {
		index["name"] = d.Name
	}
	if d.Namespace != "" {
		index["namespace"] = d.Namespace
	}
	return index
}

// KubernetesTarget kubernetes delivery target
type KubernetesTarget struct {
	ClusterName string `json:"clusterName" validate:"checkname"`
	Namespace   string `json:"namespace" optional:"true"`
}

// CloudTarget cloud target
type CloudTarget struct {
	TerraformProviderName string `json:"providerName" validate:"required"`
	Region                string `json:"region" validate:"required"`
	Zone                  string `json:"zone" optional:"true"`
	VpcID                 string `json:"vpcID" optional:"true"`
}
