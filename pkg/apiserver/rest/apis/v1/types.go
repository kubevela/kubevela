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

	"github.com/oam-dev/kubevela/pkg/addon"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
	"github.com/oam-dev/kubevela/pkg/cloudprovider"
)

var (
	// CtxKeyApplication request context key of application
	CtxKeyApplication = "application"
	// CtxKeyWorkflow request context key of workflow
	CtxKeyWorkflow = "workflow"
	// CtxKeyDeliveryTarget request context key of workflow
	CtxKeyDeliveryTarget = "delivery-target"
	// CtxKeyApplicationEnvBinding request context key of env binding
	CtxKeyApplicationEnvBinding = "envbinding-policy"
	// CtxKeyApplicationComponent request context key of component
	CtxKeyApplicationComponent = "component"
)

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
	Git  *addon.GitAddonSource `json:"git,omitempty" validate:"required"`
}

// UpdateAddonRegistryRequest defines the format for addon registry update request
type UpdateAddonRegistryRequest struct {
	Git *addon.GitAddonSource `json:"git,omitempty" validate:"required"`
}

// AddonRegistryMeta defines the format for a single addon registry
type AddonRegistryMeta struct {
	Name string                `json:"name" validate:"required"`
	Git  *addon.GitAddonSource `json:"git,omitempty"`
}

// ListAddonRegistryResponse list addon registry
type ListAddonRegistryResponse struct {
	Registrys []*AddonRegistryMeta `json:"registrys"`
}

// EnableAddonRequest defines the format for enable addon request
type EnableAddonRequest struct {
	// Args is the key-value environment variables, e.g. AK/SK credentials.
	Args map[string]interface{} `json:"args,omitempty"`
}

// ListAddonResponse defines the format for addon list response
type ListAddonResponse struct {
	Addons []*types.AddonMeta `json:"addons"`
}

// DetailAddonResponse defines the format for showing the addon details
type DetailAddonResponse struct {
	types.AddonMeta

	APISchema *openapi3.Schema     `json:"schema"`
	UISchema  []*utils.UIParameter `json:"uiSchema"`

	// More details about the addon, e.g. README
	Detail      string             `json:"detail,omitempty"`
	Definitions []*AddonDefinition `json:"definitions"`
}

// AddonDefinition is definition an addon can provide
type AddonDefinition struct {
	Name string `json:"name,omitempty"`
	// can be component/trait...definition
	DefType     string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
}

// AddonStatusResponse defines the format of addon status response
type AddonStatusResponse struct {
	Phase AddonPhase        `json:"phase"`
	Args  map[string]string `json:"args"`

	EnablingProgress *EnablingProgress `json:"enabling_progress,omitempty"`
}

// EnablingProgress defines the progress of enabling an addon
type EnablingProgress struct {
	EnabledComponents int `json:"enabled_components"`
	TotalComponents   int `json:"total_components"`
}

// AddonArgsResponse defines the response of addon args
type AddonArgsResponse struct {
	Args map[string]string `json:"args"`
}

// AccessKeyRequest request parameters to access cloud provider
type AccessKeyRequest struct {
	AccessKeyID     string `json:"accessKeyID"`
	AccessKeySecret string `json:"accessKeySecret"`
}

// CreateClusterRequest request parameters to create a cluster
type CreateClusterRequest struct {
	Name             string            `json:"name" validate:"checkname"`
	Alias            string            `json:"alias" validate:"checkalias" optional:"true"`
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
	Alias           string            `json:"alias" optional:"true" validate:"checkalias"`
	Description     string            `json:"description,omitempty" optional:"true"`
	Icon            string            `json:"icon"`
	Labels          map[string]string `json:"labels,omitempty"`
}

// CreateCloudClusterRequest request parameters to create a cloud cluster (buy one)
type CreateCloudClusterRequest struct {
	AccessKeyID       string `json:"accessKeyID"`
	AccessKeySecret   string `json:"accessKeySecret"`
	Name              string `json:"name" validate:"checkname"`
	Zone              string `json:"zone"`
	WorkerNumber      int    `json:"workerNumber"`
	CPUCoresPerWorker int64  `json:"cpuCoresPerWorker"`
	MemoryPerWorker   int64  `json:"memoryPerWorker"`
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

// CreateClusterNamespaceRequest request parameter to create namespace in cluster
type CreateClusterNamespaceRequest struct {
	Namespace string `json:"namespace"`
}

// CreateClusterNamespaceResponse response parameter for created namespace in cluster
type CreateClusterNamespaceResponse struct {
	Exists bool `json:"exists"`
}

// DetailClusterResponse cluster detail information model
type DetailClusterResponse struct {
	model.Cluster
	ResourceInfo ClusterResourceInfo `json:"resourceInfo"`
}

// ListClusterResponse list cluster
type ListClusterResponse struct {
	Clusters []ClusterBase `json:"clusters"`
	Total    int64         `json:"total"`
}

// ListCloudClusterResponse list cloud clusters
type ListCloudClusterResponse struct {
	Clusters []cloudprovider.CloudCluster `json:"clusters"`
	Total    int                          `json:"total"`
}

// CreateCloudClusterResponse return values for cloud cluster create request
type CreateCloudClusterResponse struct {
	Name      string `json:"clusterName"`
	ClusterID string `json:"clusterID"`
	Status    string `json:"status"`
}

// ListCloudClusterCreationResponse return the cluster names of creation process of cloud clusters
type ListCloudClusterCreationResponse struct {
	Creations []CreateCloudClusterResponse `json:"creations"`
}

// ClusterBase cluster base model
type ClusterBase struct {
	Name        string            `json:"name"`
	Alias       string            `json:"alias" optional:"true" validate:"checkalias"`
	Description string            `json:"description" optional:"true"`
	Icon        string            `json:"icon" optional:"true"`
	Labels      map[string]string `json:"labels" optional:"true"`

	Provider     model.ProviderInfo `json:"providerInfo"`
	APIServerURL string             `json:"apiServerURL"`
	DashboardURL string             `json:"dashboardURL"`

	Status string `json:"status"`
	Reason string `json:"reason"`
}

// ListApplicatioOptions list application  query options
type ListApplicatioOptions struct {
	Namespace  string `json:"namespace"`
	TargetName string `json:"targetName"`
	Query      string `json:"query"`
}

// ListApplicationResponse list applications by query params
type ListApplicationResponse struct {
	Applications []*ApplicationBase `json:"applications"`
}

// EnvBindingList env binding list
type EnvBindingList []*EnvBinding

// ContainTarget contain cluster name
func (e EnvBindingList) ContainTarget(name string) bool {
	for _, eb := range e {
		if utils.StringsContain(eb.TargetNames, name) {
			return true
		}
	}
	return false
}

// ApplicationBase application base model
type ApplicationBase struct {
	Name        string            `json:"name"`
	Alias       string            `json:"alias"`
	Namespace   string            `json:"namespace"`
	Description string            `json:"description"`
	CreateTime  time.Time         `json:"createTime"`
	UpdateTime  time.Time         `json:"updateTime"`
	Icon        string            `json:"icon"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// ApplicationStatusResponse application status response body
type ApplicationStatusResponse struct {
	EnvName string            `json:"envName"`
	Status  *common.AppStatus `json:"status"`
}

// ApplicationStatisticsResponse application statistics response body
type ApplicationStatisticsResponse struct {
	EnvCount            int64 `json:"envCount"`
	DeliveryTargetCount int64 `json:"deliveryTargetCount"`
	RevisonCount        int64 `json:"revisonCount"`
	WorkflowCount       int64 `json:"workflowCount"`
}

// CreateApplicationRequest create application  request body
type CreateApplicationRequest struct {
	Name        string                  `json:"name" validate:"checkname"`
	Alias       string                  `json:"alias" validate:"checkalias" optional:"true"`
	Namespace   string                  `json:"namespace" validate:"checkname"`
	Description string                  `json:"description" optional:"true"`
	Icon        string                  `json:"icon"`
	Labels      map[string]string       `json:"labels,omitempty"`
	EnvBinding  []*EnvBinding           `json:"envBinding,omitempty"`
	YamlConfig  string                  `json:"yamlConfig,omitempty"`
	Component   *CreateComponentRequest `json:"component"`
}

// UpdateApplicationRequest update application  base config
type UpdateApplicationRequest struct {
	Alias       string            `json:"alias" validate:"checkalias" optional:"true"`
	Description string            `json:"description" optional:"true"`
	Icon        string            `json:"icon" optional:"true"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// EnvBinding application env binding
type EnvBinding struct {
	Name              string             `json:"name" validate:"checkname"`
	Alias             string             `json:"alias" validate:"checkalias" optional:"true"`
	Description       string             `json:"description,omitempty" optional:"true"`
	TargetNames       []string           `json:"targetNames"`
	ComponentSelector *ComponentSelector `json:"componentSelector" optional:"true"`
}

// EnvBindingBase application env binding
type EnvBindingBase struct {
	Name              string               `json:"name" validate:"checkname"`
	Alias             string               `json:"alias" validate:"checkalias" optional:"true"`
	Description       string               `json:"description,omitempty" optional:"true"`
	TargetNames       []string             `json:"targetNames"`
	Targets           []DeliveryTargetBase `json:"deliveryTargets,omitempty"`
	ComponentSelector *ComponentSelector   `json:"componentSelector" optional:"true"`
	CreateTime        time.Time            `json:"createTime"`
	UpdateTime        time.Time            `json:"updateTime"`
	AppDeployName     string               `json:"appDeployName"`
}

// DetailEnvBindingResponse defines the response of env-binding details
type DetailEnvBindingResponse struct {
	EnvBindingBase
}

// ClusterSelector cluster selector
type ClusterSelector struct {
	Name string `json:"name" validate:"checkname"`
	// Adapt to a scenario where only one Namespace is available or a user-defined Namespace is available.
	Namespace string `json:"namespace,omitempty"`
}

// ComponentSelector component selector
type ComponentSelector struct {
	Components []string `json:"components"`
}

// DetailApplicationResponse application  detail
type DetailApplicationResponse struct {
	ApplicationBase
	Policies        []string                `json:"policies"`
	EnvBindings     []string                `json:"envBindings"`
	Status          string                  `json:"status"`
	ApplicationType string                  `json:"applicationType"`
	ResourceInfo    ApplicationResourceInfo `json:"resourceInfo"`
}

// ApplicationResourceInfo application-level resource consumption statistics
type ApplicationResourceInfo struct {
	ComponentNum int64 `json:"componentNum"`
	// Others, such as: Memory、CPU、GPU、Storage
}

// ComponentBase component  base model
type ComponentBase struct {
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

// ComponentListResponse list component
type ComponentListResponse struct {
	Components []*ComponentBase `json:"components"`
}

// CreateComponentRequest create component  request model
type CreateComponentRequest struct {
	Name          string                           `json:"name" validate:"checkname"`
	Alias         string                           `json:"alias" validate:"checkalias" optional:"true"`
	Description   string                           `json:"description" optional:"true"`
	Icon          string                           `json:"icon" optional:"true"`
	Labels        map[string]string                `json:"labels,omitempty"`
	ComponentType string                           `json:"componentType" validate:"checkname"`
	Properties    string                           `json:"properties,omitempty"`
	DependsOn     []string                         `json:"dependsOn" optional:"true"`
	Traits        []*CreateApplicationTraitRequest `json:"traits,omitempty" optional:"true"`
}

// UpdateApplicationComponentRequest update component request body
type UpdateApplicationComponentRequest struct {
	Alias       *string            `json:"alias" optional:"true"`
	Description *string            `json:"description" optional:"true"`
	Icon        *string            `json:"icon" optional:"true"`
	Labels      *map[string]string `json:"labels,omitempty"`
	Properties  *string            `json:"properties,omitempty"`
	DependsOn   *[]string          `json:"dependsOn" optional:"true"`
}

// DetailComponentResponse detail component response body
type DetailComponentResponse struct {
	model.ApplicationComponent
}

// ListApplicationComponentOptions list app  component list
type ListApplicationComponentOptions struct {
	EnvName string `json:"envName"`
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
	APISchema *openapi3.Schema     `json:"schema"`
	UISchema  []*utils.UIParameter `json:"uiSchema"`
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

// CreateWorkflowRequest create workflow  request
type CreateWorkflowRequest struct {
	Name        string         `json:"name"  validate:"checkname"`
	Alias       string         `json:"alias"  validate:"checkalias" optional:"true"`
	Description string         `json:"description" optional:"true"`
	Steps       []WorkflowStep `json:"steps,omitempty"`
	Default     bool           `json:"default"`
	EnvName     string         `json:"envName"`
}

// UpdateWorkflowRequest update or create application workflow
type UpdateWorkflowRequest struct {
	Alias       string         `json:"alias"  validate:"checkalias" optional:"true"`
	Description string         `json:"description" optional:"true"`
	Steps       []WorkflowStep `json:"steps,omitempty"`
	Enable      bool           `json:"enable"`
	Default     bool           `json:"default"`
}

// WorkflowStep workflow step config
type WorkflowStep struct {
	// Name is the unique name of the workflow step.
	Name        string             `json:"name" validate:"checkname"`
	Alias       string             `json:"alias" validate:"checkalias" optional:"true"`
	Type        string             `json:"type" validate:"checkname"`
	Description string             `json:"description" optional:"true"`
	DependsOn   []string           `json:"dependsOn" optional:"true"`
	Properties  string             `json:"properties,omitempty"`
	Inputs      common.StepInputs  `json:"inputs,omitempty" optional:"true"`
	Outputs     common.StepOutputs `json:"outputs,omitempty" optional:"true"`
}

// DetailWorkflowResponse detail workflow response
type DetailWorkflowResponse struct {
	WorkflowBase
}

// ListWorkflowResponse list application workflows
type ListWorkflowResponse struct {
	Workflows []*WorkflowBase `json:"workflows"`
}

// WorkflowBase workflow base model
type WorkflowBase struct {
	Name        string         `json:"name"`
	Alias       string         `json:"alias"`
	Description string         `json:"description"`
	Enable      bool           `json:"enable"`
	Default     bool           `json:"default"`
	EnvName     string         `json:"envName"`
	CreateTime  time.Time      `json:"createTime"`
	UpdateTime  time.Time      `json:"updateTime"`
	Steps       []WorkflowStep `json:"steps,omitempty"`
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
	Note       string    `json:"note"`
	// TriggerType the event trigger source, Web or API
	TriggerType string `json:"triggerType"`
}

// WorkflowRecord workflow record
type WorkflowRecord struct {
	Name                string                     `json:"name"`
	Namespace           string                     `json:"namespace"`
	WorkflowName        string                     `json:"workflowName"`
	ApplicationRevision string                     `json:"applicationRevision"`
	StartTime           time.Time                  `json:"startTime,omitempty"`
	Status              string                     `json:"status"`
	Steps               []model.WorkflowStepStatus `json:"steps,omitempty"`
}

// ApplicationDeployRequest the application deploy or update event request
type ApplicationDeployRequest struct {
	WorkflowName string `json:"workflowName"`
	// User note message, optional
	Note string `json:"note"`
	// TriggerType the event trigger source, Web or API
	TriggerType string `json:"triggerType" validate:"oneof=web api"`
	// Force set to True to ignore unfinished events.
	Force bool `json:"force"`
}

// ApplicationDeployResponse application deploy response body
type ApplicationDeployResponse struct {
	ApplicationRevisionBase
}

// VelaQLViewResponse query response
type VelaQLViewResponse map[string]interface{}

// PutApplicationEnvRequest set diff request
type PutApplicationEnvRequest struct {
	ComponentSelector *ComponentSelector `json:"componentSelector,omitempty"`
	Alias             string             `json:"alias,omitempty" validate:"checkalias" optional:"true"`
	Description       string             `json:"description,omitempty" optional:"true"`
	TargetNames       []string           `json:"targetNames"`
}

// ListApplicationEnvBinding list app envBindings
type ListApplicationEnvBinding struct {
	EnvBindings []*EnvBindingBase `json:"envBindings"`
}

// CreateApplicationEnvRequest new application env
type CreateApplicationEnvRequest struct {
	EnvBinding
}

// CreateApplicationTraitRequest create application triat  req
type CreateApplicationTraitRequest struct {
	Type        string `json:"type" validate:"checkname"`
	Alias       string `json:"alias,omitempty" validate:"checkalias" optional:"true"`
	Description string `json:"description,omitempty" optional:"true"`
	Properties  string `json:"properties"`
}

// UpdateApplicationTraitRequest update application trait req
type UpdateApplicationTraitRequest struct {
	Alias       string `json:"alias,omitempty" validate:"checkalias" optional:"true"`
	Description string `json:"description,omitempty" optional:"true"`
	Properties  string `json:"properties"`
}

// ApplicationTrait application trait
type ApplicationTrait struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Alias       string `json:"alias,omitempty"`
	Description string `json:"description,omitempty"`
	// Properties json data
	Properties *model.JSONStruct `json:"properties"`
	CreateTime time.Time         `json:"createTime"`
	UpdateTime time.Time         `json:"updateTime"`
}

// CreateDeliveryTargetRequest  create delivery target request body
type CreateDeliveryTargetRequest struct {
	Name        string                 `json:"name" validate:"checkname"`
	Namespace   string                 `json:"namespace"  validate:"checkname"`
	Alias       string                 `json:"alias,omitempty" validate:"checkalias" optional:"true"`
	Description string                 `json:"description,omitempty" optional:"true"`
	Cluster     *ClusterTarget         `json:"cluster,omitempty"`
	Variable    map[string]interface{} `json:"variable,omitempty"`
}

// UpdateDeliveryTargetRequest only support full quantity update
type UpdateDeliveryTargetRequest struct {
	Alias       string                 `json:"alias,omitempty" validate:"checkalias" optional:"true"`
	Description string                 `json:"description,omitempty" optional:"true"`
	Cluster     *ClusterTarget         `json:"cluster,omitempty"`
	Variable    map[string]interface{} `json:"variable,omitempty"`
}

// ClusterTarget kubernetes delivery target
type ClusterTarget struct {
	ClusterName string `json:"clusterName" validate:"checkname"`
	Namespace   string `json:"namespace" optional:"true"`
}

// DetailDeliveryTargetResponse detail deliveryTarget response
type DetailDeliveryTargetResponse struct {
	DeliveryTargetBase
}

// ListDeliveryTargetResponse list delivery target response body
type ListDeliveryTargetResponse struct {
	DeliveryTargets []DeliveryTargetBase `json:"deliveryTargets"`
	Total           int64                `json:"total"`
}

// DeliveryTargetBase deliveryTarget base model
type DeliveryTargetBase struct {
	Name        string                 `json:"name" validate:"checkname"`
	Namespace   string                 `json:"namespace"  validate:"checkname"`
	Alias       string                 `json:"alias,omitempty" validate:"checkalias" optional:"true"`
	Description string                 `json:"description,omitempty" optional:"true"`
	Cluster     *ClusterTarget         `json:"cluster,omitempty"`
	Variable    map[string]interface{} `json:"variable,omitempty"`
	CreateTime  time.Time              `json:"createTime"`
	UpdateTime  time.Time              `json:"updateTime"`
	AppNum      int64                  `json:"appNum,omitempty"`
}

// ApplicationRevisionBase application revision base spec
type ApplicationRevisionBase struct {
	CreateTime time.Time `json:"createTime"`
	Version    string    `json:"version"`
	Status     string    `json:"status"`
	Reason     string    `json:"reason"`
	DeployUser string    `json:"deployUser"`
	Note       string    `json:"note"`
	EnvName    string    `json:"envName"`
	// SourceType the event trigger source, Web or API
	TriggerType string `json:"triggerType"`
}

// ListRevisionsResponse list application revisions
type ListRevisionsResponse struct {
	Revisions []ApplicationRevisionBase `json:"revisions"`
	Total     int64                     `json:"total"`
}

// DetailRevisionResponse get application revision detail
type DetailRevisionResponse struct {
	model.ApplicationRevision
}
