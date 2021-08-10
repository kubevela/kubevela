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

// Catalog defines the data model of a Catalog
type Catalog struct {
	Name string `json:"name,omitempty"`
	Desc string `json:"desc,omitempty"`
	// UpdatedAt is the unix time of the last time when the catalog is updated.
	UpdatedAt int64 `json:"updated_at,omitempty"`
	// Type of the Catalog, such as "github" for a github repo.
	Type string `json:"type,omitempty"`
	// URL of the Catalog.
	URL string `json:"url,omitempty"`
	// Auth token used to sync Catalog.
	Token string `json:"token,omitempty"`
}
