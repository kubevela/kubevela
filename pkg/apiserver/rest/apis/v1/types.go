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

package v1

import (
	"time"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
)

// CtxKeyApplication request context key of application
var CtxKeyApplication = "application"

// AddonPhase defines the phase of an addon
type AddonPhase string

const (
	// AddonPhaseDisabled indicates the addon is disabled
	AddonPhaseDisabled AddonPhase = "disabled"
	// AddonPhaseDisabling indicates the addon is disabling
	AddonPhaseDisabling AddonPhase = "disabling"
	// AddonPhaseEnabled indicates the addon is enabled
	AddonPhaseEnabled AddonPhase = "enabled"
	// AddonPhaseEnabling indicates the addon is enabling
	AddonPhaseEnabling AddonPhase = "enabling"
)

// EmptyResponse empty response, it will used for delete api
type EmptyResponse struct{}

// CreateAddonRequest defines the format for addon create request
type CreateAddonRequest struct {
	Name string `json:"name" validate:"name"`

	Version string `json:"version" validate:"required"`

	// Short description about the addon.
	Description string `json:"description,omitempty"`

	Icon string `json:"icon"`

	Tags []string `json:"tags"`

	// The detail of the addon. Could be the entire README data.
	Detail string `json:"detail,omitempty"`

	// DeployData is the object to deploy to the cluster to enable addon
	DeployData string `json:"deploy_data,omitempty" validate:"required_without=deploy_url"`

	// DeployURL is the URL to the data file location in a Git repository
	DeployURL string `json:"deploy_url,omitempty"  validate:"required_without=deploy_data"`
}

// ListAddonResponse defines the format for addon list response
type ListAddonResponse struct {
	Addons []AddonMeta `json:"addons"`
}

// AddonMeta defines the format for a single addon
type AddonMeta struct {
	Name string `json:"name"`

	Version string `json:"version"`

	Description string `json:"description"`

	Icon string `json:"icon"`

	Tags []string `json:"tags"`

	Phase AddonPhase `json:"phase"`
}

// DetailAddonResponse defines the format for showing the addon details
type DetailAddonResponse struct {
	AddonMeta

	Detail string `json:"detail,omitempty"`

	// DeployData is the object to deploy to the cluster to enable addon
	DeployData string `json:"deploy_data,omitempty"`

	// DeployURL is the URL to the data file location in a Git repository
	DeployURL string `json:"deploy_url,omitempty"`
}

// AddonStatusResponse defines the format of addon status response
type AddonStatusResponse struct {
	Phase AddonPhase `json:"phase"`
}

// CreateClusterRequest request parameters to create a cluster
type CreateClusterRequest struct {
	Name             string            `json:"name" validate:"name"`
	Description      string            `json:"description,omitempty"`
	Icon             string            `json:"icon"`
	KubeConfig       string            `json:"kubeConfig" validate:"required_without=kubeConfigSecret"`
	KubeConfigSecret string            `json:"kubeConfigSecret,omitempty" validate:"required_without=kubeConfig"`
	Labels           map[string]string `json:"labels,omitempty"`
}

// DetailClusterResponse cluster detail information model
type DetailClusterResponse struct {
	ClusterBase
	ResourceInfo ClusterResourceInfo `json:"resourceInfo"`
	// remote manage url, eg. ACK cluster manage url.
	RemoteManageURL string `json:"remoteManageURL,omitempty"`
	// Dashboard URL
	DashboardURL string `json:"dashboardURL,omitempty"`
}

// ClusterResourceInfo resource info of cluster
type ClusterResourceInfo struct {
	WorkerNumber     int      `json:"workerNumber"`
	MasterNumber     int      `json:"masterNumber"`
	MemoryCapacity   int64    `json:"memoryCapacity"`
	CPUCapacity      int64    `json:"cpuCapacity"`
	GPUCapacity      int64    `json:"gpuCapacity,omitempty"`
	StorageClassList []string `json:"storageClassList,omitempty"`
}

// ListClusterResponse list cluster
type ListClusterResponse struct {
	Clusters []ClusterBase `json:"clusters"`
}

// ClusterBase cluster base model
type ClusterBase struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Icon        string            `json:"icon"`
	Labels      map[string]string `json:"labels"`
	Status      string            `json:"status"`
	Reason      string            `json:"reason"`
}

// ListApplicationResponse list applications by query params
type ListApplicationResponse struct {
	Applications []*ApplicationBase `json:"applications"`
}

// ApplicationBase application base model
type ApplicationBase struct {
	Name            string            `json:"name"`
	Namespace       string            `json:"namespace"`
	Description     string            `json:"description"`
	CreateTime      time.Time         `json:"createTime"`
	UpdateTime      time.Time         `json:"updateTime"`
	Icon            string            `json:"icon"`
	Labels          map[string]string `json:"labels,omitempty"`
	ClusterBindList []ClusterBase     `json:"clusterList,omitempty"`
	Status          string            `json:"status"`
	GatewayRuleList []GatewayRule     `json:"gatewayRule"`
}

// RuleType gateway rule type
type RuleType string

const (
	// HTTPRule Layer 7 HTTP policy.
	HTTPRule RuleType = "http"
	// StreamRule Layer 4 policy, such as TCP and UDP
	StreamRule RuleType = "stream"
)

// GatewayRule application gateway rule
type GatewayRule struct {
	RuleType      RuleType `json:"ruleType"`
	Address       string   `json:"address"`
	Protocol      string   `json:"protocol"`
	ComponentName string   `json:"componentName"`
	ComponentPort int32    `json:"componentPort"`
}

// CreateApplicationRequest create application request body
type CreateApplicationRequest struct {
	Name        string            `json:"name" validate:"checkname"`
	Namespace   string            `json:"namespace" validate:"checkname"`
	Description string            `json:"description"`
	Icon        string            `json:"icon"`
	Labels      map[string]string `json:"labels,omitempty"`
	ClusterList []string          `json:"clusterList,omitempty"`
	YamlConfig  string            `json:"yamlConfig,omitempty"`
	// Deploy Setting this to true means that the application is deployed directly after creation.
	Deploy bool `json:"deploy,omitempty"`
}

// DetailApplicationResponse application detail
type DetailApplicationResponse struct {
	ApplicationBase
	Policies       []string                `json:"policies"`
	Status         string                  `json:"status"`
	ResourceInfo   ApplicationResourceInfo `json:"resourceInfo"`
	WorkflowStatus []WorkflowStepStatus    `json:"workflowStatus"`
}

// WorkflowStepStatus workflow step status model
type WorkflowStepStatus struct {
	Name     string        `json:"name"`
	Status   string        `json:"status"`
	TakeTime time.Duration `json:"takeTime"`
}

// ApplicationResourceInfo application-level resource consumption statistics
type ApplicationResourceInfo struct {
	ComponentNum int `json:"componentNum"`
	// Others, such as: Memory、CPU、GPU、Storage
}

// ComponentBase component base model
type ComponentBase struct {
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	Labels        map[string]string `json:"labels,omitempty"`
	ComponentType string            `json:"componentType"`
	BindClusters  []string          `json:"bindClusters"`
	Icon          string            `json:"icon,omitempty"`
	DependsOn     []string          `json:"dependsOn"`
	Creator       string            `json:"creator,omitempty"`
	DeployVersion string            `json:"deployVersion"`
	CreateTime    time.Time         `json:"createTime"`
	UpdateTime    time.Time         `json:"updateTime"`
}

// ComponentListResponse list component
type ComponentListResponse struct {
	Components []*ComponentBase `json:"components"`
}

// CreateComponentRequest create component request model
type CreateComponentRequest struct {
	Name          string            `json:"name" validate:"checkname"`
	Description   string            `json:"description"`
	Icon          string            `json:"icon"`
	Labels        map[string]string `json:"labels,omitempty"`
	ComponentType string            `json:"componentType" validate:"checkname"`
	BindClusters  []string          `json:"bindClusters"`
	Properties    string            `json:"properties,omitempty"`
	DependsOn     []string          `json:"dependsOn"`
}

// DetailComponentResponse detail component model
type DetailComponentResponse struct {
	model.ApplicationComponent
	//TODO: Status
}

// CreateApplicationTemplateRequest create app template request model
type CreateApplicationTemplateRequest struct {
	TemplateName string `json:"templateName" validate:"checkname"`
	Version      string `json:"version" validate:"required"`
	Description  string `json:"description"`
}

// ApplicationTemplateBase app template model
type ApplicationTemplateBase struct {
	TemplateName string                        `json:"templateName"`
	Versions     []*ApplicationTemplateVersion `json:"versions,omitempty"`
	CreateTime   time.Time                     `json:"createTime"`
	UpdateTime   time.Time                     `json:"updateTime"`
}

// ApplicationTemplateVersion template version model
type ApplicationTemplateVersion struct {
	Version     string    `json:"version"`
	Description string    `json:"description"`
	CreateUser  string    `json:"createUser"`
	CreateTime  time.Time `json:"createTime"`
	UpdateTime  time.Time `json:"updateTime"`
}

// ListNamespaceResponse namesace list model
type ListNamespaceResponse struct {
	Namespaces []NamesapceBase `json:"namesapces"`
}

// NamesapceBase namespace base model
type NamesapceBase struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreateTime  time.Time `json:"createTime"`
	UpdateTime  time.Time `json:"updateTime"`
}

// CreateNamespaceRequest create namespace request body
type CreateNamespaceRequest struct {
	Name        string `json:"name" validate:"checkname"`
	Description string `json:"description"`
}

// NamesapceDetailResponse namespace detail response
type NamesapceDetailResponse struct {
	NamesapceBase
	ClusterBind map[string]string `json:"clusterBind"`
}

// ListComponentDefinitionResponse list component dedinition response model
type ListComponentDefinitionResponse struct {
	ComponentDefinitions []ComponentDefinitionBase `json:"componentDefinitions"`
}

// ComponentDefinitionBase component definition base model
type ComponentDefinitionBase struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Icon        string            `json:"icon"`
	Parameter   []types.Parameter `json:"requiredParams"`
}

// CreatePolicyRequest create app policy
type CreatePolicyRequest struct {
	// Name is the unique name of the policy.
	Name string `json:"name" validate:"checkname"`

	Description string `json:"description"`

	Type string `json:"type" validate:"checkname"`

	// Properties json data
	Properties string `json:"properties"`
}

// UpdatePolicyRequest update policy
type UpdatePolicyRequest struct {
	Description string `json:"description"`
	Type        string `json:"type" validate:"checkname"`
	// Properties json data
	Properties string `json:"properties"`
}

// PolicyBase application policy base info
type PolicyBase struct {
	// Name is the unique name of the policy.
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Creator     string `json:"creator"`
	// Properties json data
	Properties *model.JSONStruct `json:"properties"`
	CreateTime time.Time         `json:"createTime"`
	UpdateTime time.Time         `json:"updateTime"`
}

// DetailPolicyResponse app policy detail model
type DetailPolicyResponse struct {
	PolicyBase
}

// ListApplicationPolicy list app policies
type ListApplicationPolicy struct {
	Policies []*PolicyBase `json:"policies"`
}

// ListPolicyDefinitionResponse list available
type ListPolicyDefinitionResponse struct {
	PolicyDefinitions []PolicyDefinition `json:"policyDefinitions"`
}

// PolicyDefinition application policy definition
type PolicyDefinition struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Parameters  []types.Parameter `json:"parameters"`
}

// UpdateWorkflowRequest update or create application workflow
type UpdateWorkflowRequest struct {
	Name      string         `json:"name"  validate:"checkname"`
	Namespace string         `json:"namesapce"  validate:"checkname"`
	Steps     []WorkflowStep `json:"steps,omitempty"`
	Enable    bool           `json:"enable"`
}

// WorkflowStep workflow step config
type WorkflowStep struct {
	// Name is the unique name of the workflow step.
	Name       string             `json:"name" validate:"checkname"`
	Type       string             `json:"type" validate:"checkname"`
	DependsOn  []string           `json:"dependsOn"`
	Properties string             `json:"properties,omitempty"`
	Inputs     common.StepInputs  `json:"inputs,omitempty"`
	Outputs    common.StepOutputs `json:"outputs,omitempty"`
}

// DetailWorkflowResponse detail workflow response
type DetailWorkflowResponse struct {
	Steps      []WorkflowStep  `json:"steps,omitempty"`
	Enable     bool            `json:"enable"`
	LastRecord *WorkflowRecord `json:"workflowRecord"`
}

// ListWorkflowRecordsResponse list workflow execution record
type ListWorkflowRecordsResponse struct {
	Records []WorkflowRecord `json:"records"`
	Total   int64            `json:"total"`
}

// WorkflowRecord workflow record
type WorkflowRecord struct {
}

// ApplicationDeployRequest the application deploy or update event request
type ApplicationDeployRequest struct {
	// User note message, optional
	Commit string `json:"commit"`
	// SourceType the event trigger source, Web or API
	SourceType string `json:"sourceType" validate:"oneof=web api"`
	// Force set to True to ignore unfinished events.
	Force bool `json:"force"`
}

// ApplicationDeployResponse deploy response
type ApplicationDeployResponse struct {
	Version    string `json:"version"`
	Status     string `json:"status"`
	Reason     string `json:"reason"`
	DeployUser string `json:"deployUser"`
	Commit     string `json:"commit"`
	// SourceType the event trigger source, Web or API
	SourceType string `json:"sourceType"`
}
