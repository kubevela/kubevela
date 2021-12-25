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
	RegistModel(&Env{})
}

// Env models the data of env in database
type Env struct {
	BaseModel
	EnvBase
}

// EnvBase defines the data of Env except the base model
type EnvBase struct {
	Name        string `json:"name" validate:"checkname"`
	Alias       string `json:"alias" validate:"checkalias" optional:"true"`
	Description string `json:"description,omitempty"  optional:"true"`

	// Project defines the project this Env belongs to
	Project string `json:"project"`
	// Namespace defines the K8s namespace of the Env in control plane
	Namespace string `json:"namespace"`

	// Targets defines the name of delivery target that belongs to this env
	// In one project, a delivery target can only belong to one env.
	Targets []string `json:"targets,omitempty"  optional:"true"`
}

// TableName return custom table name
func (p *Env) TableName() string {
	return tableNamePrefix + "env"
}

// PrimaryKey return custom primary key
func (p *Env) PrimaryKey() string {
	return p.Name
}

// Index return custom index
func (p *Env) Index() map[string]string {
	index := make(map[string]string)
	if p.Name != "" {
		index["name"] = p.Name
	}
	if p.Namespace != "" {
		index["namespace"] = p.Namespace
	}
	return index
}
