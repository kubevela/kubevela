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

import "fmt"

func init() {
	RegistModel(&EnvBinding{})
}

// EnvBinding application env binding
type EnvBinding struct {
	BaseModel
	AppPrimaryKey     string             `json:"appPrimaryKey"`
	Name              string             `json:"name"`
	Alias             string             `json:"alias"`
	Description       string             `json:"description,omitempty"`
	TargetNames       []string           `json:"targetNames"`
	ComponentSelector *ComponentSelector `json:"componentSelector"`
	//TODO: componentPatchs
}

// TableName return custom table name
func (e *EnvBinding) TableName() string {
	return tableNamePrefix + "envbinding"
}

// PrimaryKey return custom primary key
func (e *EnvBinding) PrimaryKey() string {
	return fmt.Sprintf("%s-%s", e.AppPrimaryKey, e.Name)
}

// Index return custom index
func (e *EnvBinding) Index() map[string]string {
	index := make(map[string]string)
	if e.Name != "" {
		index["name"] = e.Name
	}
	if e.AppPrimaryKey != "" {
		index["appPrimaryKey"] = e.AppPrimaryKey
	}
	return index
}
