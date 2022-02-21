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

package addon

import (
	"github.com/getkin/kin-openapi/openapi3"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
)

// UIData contains all information represent an addon for UI
type UIData struct {
	Meta

	APISchema *openapi3.Schema     `json:"schema"`
	UISchema  []*utils.UIParameter `json:"uiSchema"`

	// Detail is README.md in an addon
	Detail string `json:"detail,omitempty"`

	Definitions    []ElementFile `json:"definitions"`
	CUEDefinitions []ElementFile `json:"CUEDefinitions"`
	Parameters     string        `json:"parameters"`
	RegistryName   string        `json:"registryName"`
}

// InstallPackage contains all necessary files that can be installed for an addon
type InstallPackage struct {
	Meta

	Definitions    []ElementFile        `json:"definitions"`
	CUEDefinitions []ElementFile        `json:"CUEDefinitions"`
	Parameters     string               `json:"parameters"`
	CUETemplates   []ElementFile        `json:"CUETemplates"`
	YAMLTemplates  []ElementFile        `json:"YAMLTemplates,omitempty"`
	DefSchemas     []ElementFile        `json:"def_schemas,omitempty"`
	AppTemplate    *v1beta1.Application `json:"appTemplate"`
}

// Meta defines the format for a single addon
type Meta struct {
	Name            string           `json:"name" validate:"required"`
	Version         string           `json:"version"`
	Description     string           `json:"description"`
	Icon            string           `json:"icon"`
	URL             string           `json:"url,omitempty"`
	Tags            []string         `json:"tags,omitempty"`
	DeployTo        *DeployTo        `json:"deployTo,omitempty"`
	Dependencies    []*Dependency    `json:"dependencies,omitempty"`
	NeedNamespace   []string         `json:"needNamespace,omitempty"`
	Invisible       bool             `json:"invisible"`
	RequireVersions *RequireVersions `json:"system,omitempty"`
}

// DeployTo defines where the addon to deploy to
type DeployTo struct {
	// This field keep the compatible for older case
	LegacyRuntimeCluster bool `json:"runtime_cluster"`
	DisableControlPlane  bool `json:"disableControlPlane"`
	RuntimeCluster       bool `json:"runtimeCluster"`
}

// Dependency defines the other addons it depends on
type Dependency struct {
	Name string `json:"name,omitempty"`
}

// ElementFile can be addon's definition or addon's component
type ElementFile struct {
	Data string
	Name string
}

// RequireVersions is this addon need version
type RequireVersions struct {
	VelaVersion       string `json:"vela,omitempty"`
	KubernetesVersion string `json:"kubernetes,omitempty"`
}
