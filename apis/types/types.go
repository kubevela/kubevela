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

package types

const (
	// DefaultKubeVelaNS defines the default KubeVela namespace in Kubernetes
	DefaultKubeVelaNS = "vela-system"
	// DefaultKubeVelaReleaseName defines the default name of KubeVela Release
	DefaultKubeVelaReleaseName = "kubevela"
	// DefaultKubeVelaChartName defines the default chart name of KubeVela, this variable MUST align to the chart name of this repo
	DefaultKubeVelaChartName = "vela-core"
	// DefaultKubeVelaVersion defines the default version needed for KubeVela chart
	DefaultKubeVelaVersion = ">0.0.0-0"
	// DefaultEnvName defines the default environment name for Apps created by KubeVela
	DefaultEnvName = "default"
	// DefaultAppNamespace defines the default K8s namespace for Apps created by KubeVela
	DefaultAppNamespace = "default"
)

const (
	// AnnDescription is the annotation which describe what is the capability used for in a WorkloadDefinition/TraitDefinition Object
	AnnDescription = "definition.oam.dev/description"
)

const (
	// StatusDeployed represents the App was deployed
	StatusDeployed = "Deployed"
	// StatusStaging represents the App was changed locally and it's spec is diff from the deployed one, or not deployed at all
	StatusStaging = "Staging"
)

// EnvMeta stores the info for app environment
type EnvMeta struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Email     string `json:"email,omitempty"`
	Domain    string `json:"domain,omitempty"`

	Current string `json:"current,omitempty"`
}

const (
	// TagCommandType used for tag cli category
	TagCommandType = "commandType"
	// TypeStart defines one category
	TypeStart = "Getting Started"
	// TypeApp defines one category
	TypeApp = "Managing Applications"
	// TypeCap defines one category
	TypeCap = "Managing Capabilities"
	// TypeSystem defines one category
	TypeSystem = "System"
	// TypePlugin defines one category used in Kubectl Plugin
	TypePlugin = "Debug and Test"
)
