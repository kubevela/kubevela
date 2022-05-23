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
	"github.com/oam-dev/kubevela/pkg/apiserver/utils"
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

	AvailableVersions []string `json:"availableVersions"`
}

// InstallPackage contains all necessary files that can be installed for an addon
type InstallPackage struct {
	Meta

	// Definitions and CUEDefinitions are converted as OAM X-Definitions, they will only in control plane cluster
	Definitions    []ElementFile `json:"definitions"`
	CUEDefinitions []ElementFile `json:"CUEDefinitions"`
	// DefSchemas are UI schemas read by VelaUX, it will only be installed in control plane clusters
	DefSchemas []ElementFile `json:"defSchemas,omitempty"`

	Parameters string `json:"parameters"`

	// CUETemplates and YAMLTemplates are resources needed to be installed in managed clusters
	CUETemplates  []ElementFile        `json:"CUETemplates"`
	YAMLTemplates []ElementFile        `json:"YAMLTemplates,omitempty"`
	AppTemplate   *v1beta1.Application `json:"appTemplate"`
}

// WholeAddonPackage contains all infos of an addon
type WholeAddonPackage struct {
	InstallPackage

	APISchema *openapi3.Schema `json:"schema"`

	// Detail is README.md in an addon
	Detail            string   `json:"detail,omitempty"`
	AvailableVersions []string `json:"availableVersions"`
	RegistryName      string   `json:"registryName"`
}

// Meta defines the format for a single addon
type Meta struct {
	Name               string              `json:"name" validate:"required"`
	Version            string              `json:"version"`
	Description        string              `json:"description"`
	Icon               string              `json:"icon"`
	URL                string              `json:"url,omitempty"`
	Tags               []string            `json:"tags,omitempty"`
	DeployTo           *DeployTo           `json:"deployTo,omitempty"`
	Dependencies       []*Dependency       `json:"dependencies,omitempty"`
	NeedNamespace      []string            `json:"needNamespace,omitempty"`
	Invisible          bool                `json:"invisible"`
	SystemRequirements *SystemRequirements `json:"system,omitempty"`
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

// SystemRequirements is this addon need version
type SystemRequirements struct {
	VelaVersion       string `json:"vela,omitempty"`
	KubernetesVersion string `json:"kubernetes,omitempty"`
}
