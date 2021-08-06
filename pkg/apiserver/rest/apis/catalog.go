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

package apis

import "github.com/oam-dev/kubevela/pkg/apiserver/proto/model"

// CatalogType catalog for list capability
type CatalogType struct {
	Name     string `json:"name"`
	Desc     string `json:"desc,omitempty"`
	UpdateAt int64  `json:"updateAt,omitempty"`
	Type     string `json:"type,omitempty"`
	URL      string `json:"url,omitempty"`
	Token    string `json:"token,omitempty"`
}

// CatalogMeta catalog meta
type CatalogMeta struct {
	Catalog *model.Catalog `json:"catalog"`
}

// CatalogRequest catalog request
type CatalogRequest struct {
	CatalogType
	Method Action `json:"method"`
}
