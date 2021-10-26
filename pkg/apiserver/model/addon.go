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

// AddonRegistry defines the data model of a AddonRegistry
type AddonRegistry struct {
	Model
	Name string `json:"name"`

	Git *GitAddonSource `json:"git,omitempty"`
}

// GitAddonSource defines the information about the Git as addon source
type GitAddonSource struct {
	URL   string `json:"url,omitempty"`
	Path  string `json:"path,omitempty"`
	Token string `json:"token,omitempty"`
}

// TableName return custom table name
func (a *AddonRegistry) TableName() string {
	return tableNamePrefix + "addon_registry"
}

// PrimaryKey return custom primary key
func (a *AddonRegistry) PrimaryKey() string {
	return a.Name
}

// Index return custom index
func (a *AddonRegistry) Index() map[string]string {
	index := make(map[string]string)
	if a.Name != "" {
		index["name"] = a.Name
	}
	return index
}
