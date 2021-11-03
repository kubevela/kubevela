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
	"github.com/oam-dev/kubevela/pkg/cloudprovider"
)

var (
	// CtxKeyApplication request context key of application
	CtxKeyApplication = "application"
)

// CtxKeyWorkflow request context key of workflow
var CtxKeyWorkflow = "workflow"

// AddonPhase defines the phase of an addon
type AddonPhase string

const (
	// AddonPhaseDisabled indicates the addon is disabled
	AddonPhaseDisabled AddonPhase = "disabled"
	// AddonPhaseEnabled indicates the addon is enabled
	AddonPhaseEnabled AddonPhase = "enabled"
	// AddonPhaseEnabling indicates the addon is enabling
	AddonPhaseEnabling AddonPhase = "enabling"
)

// EmptyResponse empty response, it will used for delete api
type EmptyResponse struct{}

// CreateAddonRegistryRequest defines the format for addon registry create request
type CreateAddonRegistryRequest struct {
	Name string                `json:"name" validate:"checkname"`
	Git  *model.GitAddonSource `json:"git,omitempty" validate:"required"`
}

// AddonRegistryMeta defines the format for a single addon registry
type AddonRegistryMeta struct {
	Name string                `json:"name" validate:"required"`
	Git  *model.GitAddonSource `json:"git,omitempty"`
}

// ListAddonRegistryResponse list addon registry
type ListAddonRegistryResponse struct {
	Registrys []*AddonRegistryMeta `json:"registrys"`
}

// EnableAddonRequest defines the format for enable addon request
type EnableAddonRequest struct {
	// Args is the key-value environment variables, e.g. AK/SK credentials.
	Args map[string]string `json:"args,omitempty"`
}

// ListAddonResponse defines the format for addon list response
type ListAddonResponse struct {
	Addons []*AddonMeta `json:"addons"`
}

// AddonMeta defines the format for a single addon
type AddonMeta struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Icon        string   `json:"icon"`
	Tags        []string `json:"tags"`
}

// DetailAddonResponse defines the format for showing the addon details
type DetailAddonResponse struct {
	AddonMeta

	// More details about the addon, e.g. README
	Detail string `json:"detail,omitempty"`

	// DeployData is the object to apply to enable addon, e.g. Application
	DeployData string `json:"deploy_data,omitempty"`
}

// AddonStatusResponse defines the format of addon status response
type AddonStatusResponse struct {
	Phase AddonPhase `json:"phase"`

	EnablingProgress *EnablingProgress `json:"enabling_progress,omitempty"`
}

// EnablingProgress defines the progress of enabling an addon
type EnablingProgress struct {
	EnabledComponents int `json:"enabled_components"`
	TotalComponents   int `json:"total_components"`
}

// AccessKeyRequest request parameters to access cloud provider
type AccessKeyRequest struct {
	AccessKeyID     string `json:"accessKeyID"`
	AccessKeySecret string `json:"accessKeySecret"`
}

// CreateClusterRequest request parameters to create a cluster
type CreateClusterRequest struct {
	Name             string            `json:"name" validate:"checkname"`
	Alias            string            `json:"alias" validate:"checkalias"`
	Description      string            `json:"description,omitempty"`
	Icon             string            `json:"icon"`
	KubeConfig       string            `json:"kubeConfig,omitempty" validate:"required_without=KubeConfigSecret"`
	KubeConfigSecret string            `json:"kubeConfigSecret,omitempty" validate:"required_without=KubeConfig"`
	Labels           map[string]string `json:"labels,omitempty"`
	DashboardURL     string            `json:"dashboardURL,omitempty"`
}

// ConnectCloudClusterRequest request parameters to create a cluster from cloud cluster
type ConnectCloudClusterRequest struct {
	AccessKeyID     string            `json:"accessKeyID"`
	AccessKeySecret string            `json:"accessKeySecret"`
	ClusterID       string            `json:"clusterID"`
	Name            string            `json:"name" validate:"checkname"`
	Alias           string            `json:"alias" validate:"checkalias"`
	Description     string            `json:"description,omitempty"`
	Icon            string            `json:"icon"`
	Labels          map[string]string `json:"labels,omitempty"`
}

// ClusterResourceInfo resource info of cluster
type ClusterResourceInfo struct {
	WorkerNumber     int      `json:"workerNumber"`
	MasterNumber     int      `json:"masterNumber"`
	MemoryCapacity   int64    `json:"memoryCapacity"`
	CPUCapacity      int64    `json:"cpuCapacity"`
	GPUCapacity      int64    `json:"gpuCapacity,omitempty"`
	PodCapacity      int64    `json:"podCapacity"`
	MemoryUsed       int64    `json:"memoryUsed"`
	CPUUsed          int64    `json:"cpuUsed"`
	GPUUsed          int64    `json:"gpuUsed,omitempty"`
	PodUsed          int64    `json:"podUsed"`
	StorageClassList []string `json:"storageClassList,omitempty"`
}

// DetailClusterResponse cluster detail information model
type DetailClusterResponse struct {
	model.Cluster
	ResourceInfo ClusterResourceInfo `json:"resourceInfo"`
}

// ListClusterResponse list cluster
type ListClusterResponse struct {
	Clusters []ClusterBase `json:"clusters"`
}

// ListCloudClusterResponse list cloud clusters
type ListCloudClusterResponse struct {
	Clusters []cloudprovider.CloudCluster `json:"clusters"`
	Total    int                          `json:"total"`
}

// ClusterBase cluster base model
type ClusterBase struct {
	Name        string            `json:"name"`
	Alias       string            `json:"alias" validate:"checkalias"`
	Description string            `json:"description"`
	Icon        string            `json:"icon"`
	Labels      map[string]string `json:"labels"`

	Provider     model.ProviderInfo `json:"providerInfo"`
	APIServerURL string             `json:"apiServerURL"`
	DashboardURL string             `json:"dashboardURL"`

	Status string `json:"status"`
	Reason string `json:"reason"`
}

// ListApplicatioPlanOptions list application plan query options
type ListApplicatioPlanOptions struct {
	Namespace string `json:"namespace"`
	Cluster   string `json:"cluster"`
	Query     string `json:"query"`
}

// ListApplicationPlanResponse list applications by query params
type ListApplicationPlanResponse struct {
	ApplicationPlans []*ApplicationPlanBase `json:"applicationplans"`
}

// EnvBindList env bind list
type EnvBindList []*EnvBind

// ContainCluster contain cluster name
func (e EnvBindList) ContainCluster(name string) bool {
	for _, eb := range e {
		if eb.ClusterSelector != nil && eb.ClusterSelector.Name == name {
			return true
		}
	}
	return false
}

// ApplicationPlanBase application base model
type ApplicationPlanBase struct {
	Name            string            `json:"name"`
	Alias           string            `json:"alias"`
	Namespace       string            `json:"namespace"`
	Description     string            `json:"description"`
	CreateTime      time.Time         `json:"createTime"`
	UpdateTime      time.Time         `json:"updateTime"`
	Icon            string            `json:"icon"`
	Labels          map[string]string `json:"labels,omitempty"`
	Status          string            `json:"status"`
	EnvBind         EnvBindList       `json:"envBind,omitempty"`
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

// CreateApplicationPlanRequest create application plan request body
type CreateApplicationPlanRequest struct {
	Name        string            `json:"name" validate:"checkname"`
	Alias       string            `json:"alias" validate:"checkalias"`
	Namespace   string            `json:"namespace" validate:"checkname"`
	Description string            `json:"description"`
	Icon        string            `json:"icon"`
	Labels      map[string]string `json:"labels,omitempty"`
	EnvBind     []*EnvBind        `json:"envBind,omitempty"`
	YamlConfig  string            `json:"yamlConfig,omitempty"`
	// Deploy Setting this to true means that the application is deployed directly after creation.
	Deploy bool `json:"deploy,omitempty"`
}

// EnvBind application env bind
type EnvBind struct {
	Name            string           `json:"name" validate:"checkname"`
	Description     string           `json:"description,omitempty"`
	ClusterSelector *ClusterSelector `json:"clusterSelector"`
}

// ClusterSelector cluster selector
type ClusterSelector struct {
	Name string `json:"name" validate:"checkname"`
	// Adapt to a scenario where only one Namespace is available or a user-defined Namespace is available.
	Namespace string `json:"namespace,omitempty"`
}

// DetailApplicationPlanResponse application plan detail
type DetailApplicationPlanResponse struct {
	ApplicationPlanBase
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

// ComponentPlanBase component plan base model
type ComponentPlanBase struct {
	Name          string            `json:"name"`
	Alias         string            `json:"alias"`
	Description   string            `json:"description"`
	Labels        map[string]string `json:"labels,omitempty"`
	ComponentType string            `json:"componentType"`
	EnvNames      []string          `json:"envNames"`
	Icon          string            `json:"icon,omitempty"`
	DependsOn     []string          `json:"dependsOn"`
	Creator       string            `json:"creator,omitempty"`
	DeployVersion string            `json:"deployVersion"`
	CreateTime    time.Time         `json:"createTime"`
	UpdateTime    time.Time         `json:"updateTime"`
}

// ComponentPlanListResponse list component plan
type ComponentPlanListResponse struct {
	ComponentPlans []*ComponentPlanBase `json:"componentplans"`
}

// CreateComponentPlanRequest create component plan request model
type CreateComponentPlanRequest struct {
	Name          string            `json:"name" validate:"checkname"`
	Alias         string            `json:"alias" validate:"checkalias"`
	Description   string            `json:"description"`
	Icon          string            `json:"icon"`
	Labels        map[string]string `json:"labels,omitempty"`
	ComponentType string            `json:"componentType" validate:"checkname"`
	EnvNames      []string          `json:"envNames,omitempty"`
	Properties    string            `json:"properties,omitempty"`
	DependsOn     []string          `json:"dependsOn"`
}

// DetailComponentPlanResponse detail component plan model
type DetailComponentPlanResponse struct {
	model.ApplicationComponentPlan
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
	Namespaces []NamespaceBase `json:"namespaces"`
}

// NamespaceBase namespace base model
type NamespaceBase struct {
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

// NamespaceDetailResponse namespace detail response
type NamespaceDetailResponse struct {
	NamespaceBase
}

// ListDefinitionResponse list definition response model
type ListDefinitionResponse struct {
	Definitions []*DefinitionBase `json:"definitions"`
}

// DetailDefinitionResponse get definition detail
type DetailDefinitionResponse struct {
	Schema *DefinitionSchema `json:"schema"`
}

type DefinitionSchema struct {
	Properties map[string]DefinitionProperties `json:"properties"`
	Required   []string                        `json:"required"`
	Type       string                          `json:"type"`
}

type DefinitionProperties struct {
	Default     string `json:"default"`
	Description string `json:"description"`
	Title       string `json:"title"`
	Type        string `json:"type"`
}

// DefinitionBase is the definition base model
type DefinitionBase struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
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

// CreateWorkflowPlanRequest create workflow plan request
type CreateWorkflowPlanRequest struct {
	AppName     string         `json:"appName" validate:"checkname"`
	Name        string         `json:"name"  validate:"checkname"`
	Alias       string         `json:"alias"  validate:"checkalias"`
	Description string         `json:"description"`
	Steps       []WorkflowStep `json:"steps,omitempty"`
	Enable      bool           `json:"enable"`
	Default     bool           `json:"default"`
}

// UpdateWorkflowPlanRequest update or create application workflow
type UpdateWorkflowPlanRequest struct {
	Alias       string         `json:"alias"  validate:"checkalias"`
	Description string         `json:"description"`
	Steps       []WorkflowStep `json:"steps,omitempty"`
	Enable      bool           `json:"enable"`
	Default     bool           `json:"default"`
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

// DetailWorkflowPlanResponse detail workflow response
type DetailWorkflowPlanResponse struct {
	WorkflowPlanBase
	Steps      []WorkflowStep  `json:"steps,omitempty"`
	LastRecord *WorkflowRecord `json:"workflowRecord"`
}

// ListWorkflowPlanResponse list application workflows
type ListWorkflowPlanResponse struct {
	WorkflowPlans []*WorkflowPlanBase `json:"workflowplans"`
}

// WorkflowPlanBase workflow base model
type WorkflowPlanBase struct {
	Name        string    `json:"name"`
	Alias       string    `json:"alias"`
	Description string    `json:"description"`
	Enable      bool      `json:"enable"`
	Default     bool      `json:"default"`
	CreateTime  time.Time `json:"createTime"`
	UpdateTime  time.Time `json:"updateTime"`
}

// ListWorkflowRecordsResponse list workflow execution record
type ListWorkflowRecordsResponse struct {
	Records []WorkflowRecord `json:"records"`
	Total   int64            `json:"total"`
}

// DetailWorkflowRecordResponse get workflow record detail
type DetailWorkflowRecordResponse struct {
	WorkflowRecord
	DeployTime time.Time `json:"deployTime"`
	DeployUser string    `json:"deployUser"`
	Commit     string    `json:"commit"`
	// SourceType the event trigger source, Web or API
	SourceType string `json:"sourceType"`
}

// WorkflowRecord workflow record
type WorkflowRecord struct {
	Name       string                      `json:"name"`
	Namespace  string                      `json:"namespace"`
	StartTime  time.Time                   `json:"startTime,omitempty"`
	Suspend    bool                        `json:"suspend"`
	Terminated bool                        `json:"terminated"`
	Steps      []common.WorkflowStepStatus `json:"steps,omitempty"`
}

// ApplicationDeployRequest the application deploy or update event request
type ApplicationDeployRequest struct {
	WorkflowName string `json:"workflowName"`
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

// VelaQLViewResponse query response
type VelaQLViewResponse map[string]interface{}
