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

import "github.com/oam-dev/kubevela/pkg/oam"

const (
	// KubeVelaName name of kubevela
	KubeVelaName = "kubevela"
	// VelaCoreName name of vela-core
	VelaCoreName = "vela-core"
)

const (
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
	// AutoDetectWorkloadDefinition defines the default workload type for ComponentDefinition which doesn't specify a workload
	AutoDetectWorkloadDefinition = "autodetects.core.oam.dev"
	// KubeVelaControllerDeployment defines the KubeVela controller's deployment name
	KubeVelaControllerDeployment = "kubevela-vela-core"
)

// DefaultKubeVelaNS defines the default KubeVela namespace in Kubernetes
var DefaultKubeVelaNS = "vela-system"

const (
	// AnnoDefinitionDescription is the annotation which describe what is the capability used for in a WorkloadDefinition/TraitDefinition Object
	AnnoDefinitionDescription = "definition.oam.dev/description"
	// AnnoDefinitionExampleURL is the annotation which describe url of usage examples of the capability, it will be loaded in documentation generate.
	AnnoDefinitionExampleURL = "definition.oam.dev/example-url"
	// AnnoDefinitionAlias is the annotation for definition alias
	AnnoDefinitionAlias = "definition.oam.dev/alias"
	// AnnoDefinitionIcon is the annotation which describe the icon url
	AnnoDefinitionIcon = "definition.oam.dev/icon"
	// AnnoDefinitionAppliedWorkloads is the annotation which describe what is the workloads used for in a TraitDefinition Object
	AnnoDefinitionAppliedWorkloads = "definition.oam.dev/appliedWorkloads"
	// LabelDefinition is the label for definition
	LabelDefinition = "definition.oam.dev"
	// LabelDefinitionName is the label for definition name
	LabelDefinitionName = "definition.oam.dev/name"
	// LabelDefinitionDeprecated is the label which describe whether the capability is deprecated
	LabelDefinitionDeprecated = "custom.definition.oam.dev/deprecated"
	// LabelDefinitionHidden is the label which describe whether the capability is hidden by UI
	LabelDefinitionHidden = "custom.definition.oam.dev/ui-hidden"
	// LabelDefinitionScope is the label which describe whether the capability's scope
	LabelDefinitionScope = "custom.definition.oam.dev/scope"
	// LabelNodeRoleGateway gateway role of node
	LabelNodeRoleGateway = "node-role.kubernetes.io/gateway"
	// LabelNodeRoleWorker worker role of node
	LabelNodeRoleWorker = "node-role.kubernetes.io/worker"
	// AnnoIngressControllerHTTPSPort define ingress controller listen port for https
	AnnoIngressControllerHTTPSPort = "ingress.controller/https-port"
	// AnnoIngressControllerHTTPPort define ingress controller listen port for http
	AnnoIngressControllerHTTPPort = "ingress.controller/http-port"
	// AnnoIngressControllerHost define ingress controller externally host
	AnnoIngressControllerHost = "ingress.controller/host"
	// LabelConfigType is the label marked as the template that generated the config.
	LabelConfigType = "config.oam.dev/type"
	// LabelConfigCatalog is the label marked as the secret generated from the config.
	LabelConfigCatalog = "config.oam.dev/catalog"
	// LabelConfigSubType is the sub-type for a config type
	LabelConfigSubType = "config.oam.dev/sub-type"
	// LabelConfigProject is the label for config project
	LabelConfigProject = "config.oam.dev/project"
	// LabelConfigSyncToMultiCluster is the label to decide whether a config will be synchronized to multi-cluster
	LabelConfigSyncToMultiCluster = "config.oam.dev/multi-cluster"
	// LabelConfigIdentifier is the label for config identifier
	LabelConfigIdentifier = "config.oam.dev/identifier"
	// LabelConfigScope is the label for config scope
	LabelConfigScope = "config.oam.dev/scope"
	// AnnotationConfigSensitive is the annotation for the sensitization
	AnnotationConfigSensitive = "config.oam.dev/sensitive"
	// AnnotationConfigTemplateNamespace is the annotation for the template namespace
	AnnotationConfigTemplateNamespace = "config.oam.dev/template-namespace"
	// AnnotationConfigDescription is the annotation for config description
	AnnotationConfigDescription = "config.oam.dev/description"
	// AnnotationConfigAlias is the annotation for config alias
	AnnotationConfigAlias = "config.oam.dev/alias"
	// AnnotationConfigDistributionSpec is the annotation key of the application that distributes the configs
	AnnotationConfigDistributionSpec = "config.oam.dev/distribution-spec"
)

const (
	// StatusDeployed represents the App was deployed
	StatusDeployed = "Deployed"
	// StatusStaging represents the App was changed locally and it's spec is diff from the deployed one, or not deployed at all
	StatusStaging = "Staging"
)

// Config contains key/value pairs
type Config map[string]string

// EnvMeta stores the namespace for app environment
type EnvMeta struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Labels    string `json:"labels"`
	Current   string `json:"current"`
}

const (
	// TagCommandType used for tag cli category
	TagCommandType = "commandType"

	// TagCommandOrder defines the order
	TagCommandOrder = "commandOrder"

	// TypeStart defines one category
	TypeStart = "Getting Started"

	// TypeApp defines one category
	TypeApp = "Managing Applications"

	// TypeCD defines workflow Management operations
	TypeCD = "Continuous Delivery"

	// TypeExtension defines one category
	TypeExtension = "Managing Extension"

	// TypeSystem defines one category
	TypeSystem = "Others"

	// TypePlugin defines one category used in Kubectl Plugin
	TypePlugin = "Plugin Command"
)

// LabelArg is the argument `label` of a definition
const LabelArg = "label"

// DefaultFilterAnnots are annotations that won't pass to workload or trait
var DefaultFilterAnnots = []string{
	oam.AnnotationAppRollout,
	oam.AnnotationRollingComponent,
	oam.AnnotationInplaceUpgrade,
	oam.AnnotationFilterLabelKeys,
	oam.AnnotationFilterAnnotationKeys,
	oam.AnnotationLastAppliedConfiguration,
}

// ConfigType is the type of config
type ConfigType string

const (
	// TerraformProvider is the config type for terraform provider
	TerraformProvider = "terraform-provider"
	// DexConnector is the config type for dex connector
	DexConnector = "dex-connector"
	// ImageRegistry is the config type for image registry
	ImageRegistry = "image-registry"
	// HelmRepository is the config type for Helm chart repository
	HelmRepository = "helm-repository"
	// CatalogConfigDistribution is the catalog type
	CatalogConfigDistribution = "config-distribution"
)

const (
	// TerraformComponentPrefix is the prefix of component type of terraform-xxx
	TerraformComponentPrefix = "terraform-"

	// ProviderAppPrefix is the prefix of the application to create a Terraform Provider
	ProviderAppPrefix = "config-terraform-provider"
	// ProviderNamespace is the namespace of Terraform Cloud Provider
	ProviderNamespace = "default"
	// VelaCoreConfig is to mark application, config and its secret or Terraform provider lelong to a KubeVela config
	VelaCoreConfig = "velacore-config"
)

const (
	// LabelSourceOfTruth describes the source of this app
	LabelSourceOfTruth = "app.oam.dev/source-of-truth"

	// FromCR means the data source of truth is from k8s CR
	FromCR = "from-k8s-resource"
	// FromUX means the data source of truth is from velaux data store
	FromUX = "from-velaux"
	// FromInner means the data source of truth is from KubeVela inner usage
	// the configuration that don't want to be synced
	// the addon application should be synced, but set to readonly mode
	FromInner = "from-inner-system"
)
