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
	RegisterModel(&Project{})
}

// Project basic model
type Project struct {
	BaseModel
	Name        string `json:"name"`
	Alias       string `json:"alias"`
	Description string `json:"description,omitempty"`
}

// TableName return custom table name
func (p *Project) TableName() string {
	return tableNamePrefix + "project"
}

// ShortTableName is the compressed version of table name for kubeapi storage and others
func (p *Project) ShortTableName() string {
	return "pj"
}

// PrimaryKey return custom primary key
func (p *Project) PrimaryKey() string {
	return p.Name
}

// Index return custom index
func (p *Project) Index() map[string]string {
	index := make(map[string]string)
	if p.Name != "" {
		index["name"] = p.Name
	}
	return index
}
