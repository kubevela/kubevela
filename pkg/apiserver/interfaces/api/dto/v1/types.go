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

	"helm.sh/helm/v3/pkg/repo"

	"github.com/getkin/kin-openapi/openapi3"
	registryv1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/addon"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils"
	"github.com/oam-dev/kubevela/pkg/cloudprovider"
)

var (
	// CtxKeyApplication request context key of application
	CtxKeyApplication = "application"
	// CtxKeyWorkflow request context key of workflow
	CtxKeyWorkflow = "workflow"
	// CtxKeyTarget request context key of workflow
	CtxKeyTarget = "delivery-target"
	// CtxKeyApplicationEnvBinding request context key of env binding
	CtxKeyApplicationEnvBinding = "envbinding-policy"
	// CtxKeyApplicationComponent request context key of component
	CtxKeyApplicationComponent = "component"
	// CtxKeyUser request context key of user
	CtxKeyUser = "user"
	// CtxKeyToken request context key of request token
	CtxKeyToken = "token"
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
	// AddonPhaseDisabling indicates the addon is enabling
	AddonPhaseDisabling AddonPhase = "disabling"
	// AddonPhaseSuspend indicates the addon is suspend
	AddonPhaseSuspend AddonPhase = "suspend"
)

// EmptyResponse empty response, it will used for delete api
type EmptyResponse struct{}

// NameAlias name and alias
type NameAlias struct {
	Name  string `json:"name"`
	Alias string `json:"alias"`
}

// CreateAddonRegistryRequest defines the format for addon registry create request
type CreateAddonRegistryRequest struct {
	Name   string                   `json:"name" validate:"checkname"`
	Helm   *addon.HelmSource        `json:"helm,omitempty"`
	Git    *addon.GitAddonSource    `json:"git,omitempty" `
	Oss    *addon.OSSAddonSource    `json:"oss,omitempty"`
	Gitee  *addon.GiteeAddonSource  `json:"gitee,omitempty" `
	Gitlab *addon.GitlabAddonSource `json:"gitlab,omitempty" `
}

// UpdateAddonRegistryRequest defines the format for addon registry update request
type UpdateAddonRegistryRequest struct {
	Helm   *addon.HelmSource        `json:"helm,omitempty"`
	Git    *addon.GitAddonSource    `json:"git,omitempty"`
	Oss    *addon.OSSAddonSource    `json:"oss,omitempty"`
	Gitee  *addon.GiteeAddonSource  `json:"gitee,omitempty" `
	Gitlab *addon.GitlabAddonSource `json:"gitlab,omitempty" `
}

// AddonRegistry defines the format for a single addon registry
type AddonRegistry struct {
	Name   string                   `json:"name" validate:"required"`
	Helm   *addon.HelmSource        `json:"helm,omitempty"`
	Git    *addon.GitAddonSource    `json:"git,omitempty"`
	OSS    *addon.OSSAddonSource    `json:"oss,omitempty"`
	Gitee  *addon.GiteeAddonSource  `json:"gitee,omitempty" `
	Gitlab *addon.GitlabAddonSource `json:"gitlab,omitempty" `
}

// ListAddonRegistryResponse list addon registry
type ListAddonRegistryResponse struct {
	Registries []*AddonRegistry `json:"registries"`
}

// EnableAddonRequest defines the format for enable addon request
type EnableAddonRequest struct {
	// Args is the key-value environment variables, e.g. AK/SK credentials.
	Args map[string]interface{} `json:"args,omitempty"`
	// Clusters specify the clusters this addon should be installed, if not specified, it will follow the configure in addon metadata.yaml
	Clusters []string `json:"clusters,omitempty"`
	// Version specify the version of addon to enable
	Version string `json:"version,omitempty"`
}

// ListAddonResponse defines the format for addon list response
type ListAddonResponse struct {
	Addons []*AddonInfo `json:"addons"`

	// Message demonstrate the error info if exists
	Message string `json:"message,omitempty"`
}

// AddonInfo contain addon metaData and some baseInfo
type AddonInfo struct {
	*addon.Meta
	RegistryName string `json:"registryName"`
}

// ListEnabledAddonResponse defines the format for enabled addon list response
type ListEnabledAddonResponse struct {
	EnabledAddons []*AddonBaseStatus `json:"enabledAddons"`
}

// AddonBaseStatus addon base status
type AddonBaseStatus struct {
	Name  string     `json:"name"`
	Phase AddonPhase `json:"phase"`
}

// DetailAddonResponse defines the format for showing the addon details
type DetailAddonResponse struct {
	addon.Meta

	APISchema *openapi3.Schema `json:"schema"`
	UISchema  utils.UISchema   `json:"uiSchema"`

	// More details about the addon, e.g. README
	Detail            string             `json:"detail,omitempty"`
	Definitions       []*AddonDefinition `json:"definitions"`
	RegistryName      string             `json:"registryName,omitempty"`
	AvailableVersions []string           `json:"availableVersions"`
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
	AddonBaseStatus
	Args             map[string]interface{} `json:"args"`
	EnablingProgress *EnablingProgress      `json:"enabling_progress,omitempty"`
	AppStatus        common.AppStatus       `json:"appStatus,omitempty"`
	InstalledVersion string                 `json:"installedVersion,omitempty"`

	// the status of multiple clusters
	Clusters    map[string]map[string]interface{} `json:"clusters,omitempty"`
	AllClusters []NameAlias                       `json:"allClusters,omitempty"`
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

// ConfigType define the format for listing configuration types
type ConfigType struct {
	Definitions []string `json:"definitions"`
	Alias       string   `json:"alias"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
}

// Config define the metadata of a config
type Config struct {
	ConfigType        string                  `json:"configType"`
	ConfigTypeAlias   string                  `json:"configTypeAlias"`
	Name              string                  `json:"name"`
	Project           string                  `json:"project"`
	Identifier        string                  `json:"identifier"`
	Alias             string                  `json:"alias"`
	Description       string                  `json:"description"`
	CreatedTime       *time.Time              `json:"createdTime"`
	UpdatedTime       *time.Time              `json:"updatedTime"`
	ApplicationStatus common.ApplicationPhase `json:"applicationStatus"`
	Status            string                  `json:"status"`
}

// ImageResponse is the response for checking image
type ImageResponse struct {
	Existed bool   `json:"existed"`
	Secret  string `json:"secret"`
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

// ListApplicationOptions list application  query options
type ListApplicationOptions struct {
	Projects   []string `json:"projects"`
	Env        string   `json:"env"`
	TargetName string   `json:"targetName"`
	Query      string   `json:"query"`
}

// ListApplicationResponse list applications by query params
type ListApplicationResponse struct {
	Applications []*ApplicationBase `json:"applications"`
}

// EnvBindingList env binding list
type EnvBindingList []*EnvBinding

// ApplicationBase application base model
type ApplicationBase struct {
	Name        string            `json:"name"`
	Alias       string            `json:"alias"`
	Project     *ProjectBase      `json:"project"`
	Description string            `json:"description"`
	CreateTime  time.Time         `json:"createTime"`
	UpdateTime  time.Time         `json:"updateTime"`
	Icon        string            `json:"icon"`
	Labels      map[string]string `json:"labels,omitempty"`
	ReadOnly    bool              `json:"readOnly,omitempty"`
}

// AppCompareResponse application compare result
type AppCompareResponse struct {
	IsDiff        bool   `json:"isDiff"`
	DiffReport    string `json:"diffReport"`
	BaseAppYAML   string `json:"baseAppYAML"`
	TargetAppYAML string `json:"targetAppYAML"`
}

// AppResetResponse application reset result
type AppResetResponse struct {
	IsReset bool `json:"isReset"`
}

// AppCompareReq  application compare req
type AppCompareReq struct {
	CompareRevisionWithRunning *CompareRevisionWithRunningOption `json:"compareRevisionWithRunning,omitempty"`
	CompareRevisionWithLatest  *CompareRevisionWithLatestOption  `json:"compareRevisionWithLatest,omitempty"`
	CompareLatestWithRunning   *CompareLatestWithRunningOption   `json:"compareLatestWithRunning,omitempty"`
}

// CompareRevisionWithRunningOption means compare the specified version with the application in cluster.
type CompareRevisionWithRunningOption struct {
	// Revision, If not specified, means use the latest revision.
	Revision string `json:"revision" optional:"true"`
}

// CompareRevisionWithLatestOption means compare the the specified version with the latest application configuration
type CompareRevisionWithLatestOption struct {
	// Revision, If not specified, means use the latest revision.
	Revision string `json:"revision" optional:"true"`
}

// CompareLatestWithRunningOption means compare the latest configuration with the app in cluster.
type CompareLatestWithRunningOption struct {
	Env string `json:"env" validate:"required"`
}

// AppDryRunReq application dry-run req
type AppDryRunReq struct {
	DryRunType string `json:"dryRunType" validate:"oneof=APP REVISION"`
	Env        string `json:"env"`
	Workflow   string `json:"workflow"`
	Version    string `json:"version"`
}

// AppDryRunResponse application dry-run result
type AppDryRunResponse struct {
	YAML    string `json:"yaml"`
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// ApplicationStatusResponse application status response body
type ApplicationStatusResponse struct {
	EnvName string            `json:"envName"`
	Status  *common.AppStatus `json:"status"`
}

// ApplicationStatisticsResponse application statistics response body
type ApplicationStatisticsResponse struct {
	EnvCount      int64 `json:"envCount"`
	TargetCount   int64 `json:"targetCount"`
	RevisionCount int64 `json:"revisionCount"`
	WorkflowCount int64 `json:"workflowCount"`
}

// CreateApplicationRequest create application request body
type CreateApplicationRequest struct {
	Name        string                  `json:"name" validate:"checkname"`
	Alias       string                  `json:"alias" validate:"checkalias" optional:"true"`
	Project     string                  `json:"project" validate:"checkname"`
	Description string                  `json:"description" optional:"true"`
	Icon        string                  `json:"icon"`
	Labels      map[string]string       `json:"labels,omitempty"`
	EnvBinding  []*EnvBinding           `json:"envBinding,omitempty"`
	Component   *CreateComponentRequest `json:"component"`
}

// CreateConfigRequest is the request body to creates a config
type CreateConfigRequest struct {
	Name          string `json:"name" validate:"checkname"`
	Alias         string `json:"alias"`
	Description   string `json:"description"`
	Project       string `json:"project"`
	ComponentType string `json:"componentType" validate:"checkname"`
	Properties    string `json:"properties,omitempty"`
}

// UpdateApplicationRequest update application base config
type UpdateApplicationRequest struct {
	Alias       string            `json:"alias" validate:"checkalias" optional:"true"`
	Description string            `json:"description" optional:"true"`
	Icon        string            `json:"icon" optional:"true"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// CreateApplicationTriggerRequest create application trigger
type CreateApplicationTriggerRequest struct {
	Name          string `json:"name" validate:"checkname"`
	Alias         string `json:"alias" validate:"checkalias" optional:"true"`
	Description   string `json:"description" optional:"true"`
	WorkflowName  string `json:"workflowName"`
	Type          string `json:"type" validate:"oneof=webhook"`
	PayloadType   string `json:"payloadType" validate:"checkpayloadtype"`
	ComponentName string `json:"componentName,omitempty" optional:"true"`
}

// ApplicationTriggerBase application trigger base model
type ApplicationTriggerBase struct {
	Name          string    `json:"name"`
	Alias         string    `json:"alias,omitempty"`
	Description   string    `json:"description,omitempty"`
	WorkflowName  string    `json:"workflowName"`
	Type          string    `json:"type"`
	PayloadType   string    `json:"payloadType"`
	Token         string    `json:"token"`
	ComponentName string    `json:"componentName,omitempty"`
	CreateTime    time.Time `json:"createTime"`
	UpdateTime    time.Time `json:"updateTime"`
}

// ListApplicationTriggerResponse list application triggers response body
type ListApplicationTriggerResponse struct {
	Triggers []*ApplicationTriggerBase `json:"triggers"`
}

// HandleApplicationTriggerWebhookRequest handles application trigger webhook request
type HandleApplicationTriggerWebhookRequest struct {
	Upgrade  map[string]*model.JSONStruct `json:"upgrade,omitempty"`
	CodeInfo *model.CodeInfo              `json:"codeInfo,omitempty"`
}

// HandleApplicationTriggerACRRequest handles application trigger ACR request
type HandleApplicationTriggerACRRequest struct {
	PushData   ACRPushData   `json:"push_data"`
	Repository ACRRepository `json:"repository"`
}

// ACRPushData is the push data of ACR
type ACRPushData struct {
	Digest   string `json:"digest"`
	PushedAt string `json:"pushed_at"`
	Tag      string `json:"tag"`
}

// ACRRepository is the repository of ACR
type ACRRepository struct {
	DateCreated            string `json:"date_created"`
	Name                   string `json:"name"`
	Namespace              string `json:"namespace"`
	Region                 string `json:"region"`
	RepoAuthenticationType string `json:"repo_authentication_type"`
	RepoFullName           string `json:"repo_full_name"`
	RepoOriginType         string `json:"repo_origin_type"`
	RepoType               string `json:"repo_type"`
}

// HandleApplicationHarborReq handles application trigger harbor request
type HandleApplicationHarborReq struct {
	Type      string    `json:"type"`
	OccurAt   int64     `json:"occur_at"`
	Operator  string    `json:"operator"`
	EventData EventData `json:"event_data"`
}

// Resources is the image info of harbor
type Resources struct {
	Digest      string `json:"digest"`
	Tag         string `json:"tag"`
	ResourceURL string `json:"resource_url"`
}

// Repository is the repository of harbor
type Repository struct {
	DateCreated  int64  `json:"date_created"`
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	RepoFullName string `json:"repo_full_name"`
	RepoType     string `json:"repo_type"`
}

// EventData is the event info of harbor
type EventData struct {
	Resources  []Resources `json:"resources"`
	Repository Repository  `json:"repository"`
}

// HandleApplicationTriggerDockerHubRequest application trigger DockerHub webhook request
type HandleApplicationTriggerDockerHubRequest struct {
	CallbackURL string              `json:"callback_url"`
	PushData    DockerHubData       `json:"push_data"`
	Repository  DockerHubRepository `json:"repository"`
}

// DockerHubData is the push data of dockerhub
type DockerHubData struct {
	Images   []string `json:"images"`
	PushedAt int64    `json:"pushed_at"`
	Pusher   string   `json:"pusher"`
	Tag      string   `json:"tag"`
}

// DockerHubRepository is the repository of dockerhub
type DockerHubRepository struct {
	CommentCount    int    `json:"comment_count"`
	DateCreated     int64  `json:"date_created"`
	Description     string `json:"description"`
	Dockerfile      string `json:"dockerfile"`
	FullDescription string `json:"full_description"`
	IsOfficial      bool   `json:"is_official"`
	IsPrivate       bool   `json:"is_private"`
	IsTrusted       bool   `json:"is_trusted"`
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
	Owner           string `json:"owner"`
	RepoName        string `json:"repo_name"`
	RepoURL         string `json:"repo_url"`
	StartCount      int    `json:"star_count"`
	Status          string `json:"status"`
}

// HandleApplicationTriggerJFrogRequest application trigger JFrog webhook request
type HandleApplicationTriggerJFrogRequest struct {
	Domain    string           `json:"domain"`
	EventType string           `json:"event_type"`
	Data      JFrogWebhookData `json:"data"`
}

// JFrogWebhookData is the data of JFrog webhook request
type JFrogWebhookData struct {
	URL       string
	ImageName string `json:"image_name"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	RepoKey   string `json:"repo_key"`
	Digest    string `json:"sha256"`
	Tag       string `json:"tag"`
}

// EnvBinding application env binding
type EnvBinding struct {
	Name string `json:"name" validate:"checkname"`
	// TODO: support componentsPatch
}

// EnvBindingTarget the target struct in the envbinding base struct
type EnvBindingTarget struct {
	NameAlias
	Cluster *ClusterTarget `json:"cluster,omitempty"`
}

// EnvBindingBase application env binding
type EnvBindingBase struct {
	Name               string             `json:"name" validate:"checkname"`
	Alias              string             `json:"alias" validate:"checkalias" optional:"true"`
	Description        string             `json:"description,omitempty" optional:"true"`
	TargetNames        []string           `json:"targetNames"`
	Targets            []EnvBindingTarget `json:"targets,omitempty"`
	ComponentSelector  *ComponentSelector `json:"componentSelector" optional:"true"`
	CreateTime         time.Time          `json:"createTime"`
	UpdateTime         time.Time          `json:"updateTime"`
	AppDeployName      string             `json:"appDeployName"`
	AppDeployNamespace string             `json:"appDeployNamespace"`
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
	Policies     []string                `json:"policies"`
	EnvBindings  []string                `json:"envBindings"`
	ResourceInfo ApplicationResourceInfo `json:"resourceInfo"`
}

// ApplicationResourceInfo application-level resource consumption statistics
type ApplicationResourceInfo struct {
	ComponentNum int64 `json:"componentNum"`
	// Others, such as: Memory、CPU、GPU、Storage
}

// ComponentBase component  base model
type ComponentBase struct {
	Name          string                        `json:"name"`
	Alias         string                        `json:"alias"`
	Description   string                        `json:"description"`
	Labels        map[string]string             `json:"labels,omitempty"`
	ComponentType string                        `json:"componentType"`
	Main          bool                          `json:"main"`
	Icon          string                        `json:"icon,omitempty"`
	DependsOn     []string                      `json:"dependsOn"`
	Creator       string                        `json:"creator,omitempty"`
	CreateTime    time.Time                     `json:"createTime"`
	UpdateTime    time.Time                     `json:"updateTime"`
	Inputs        common.StepInputs             `json:"inputs,omitempty"`
	Outputs       common.StepOutputs            `json:"outputs,omitempty"`
	Traits        []*ApplicationTrait           `json:"traits"`
	WorkloadType  common.WorkloadTypeDescriptor `json:"workloadType,omitempty"`
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
	Inputs        common.StepInputs                `json:"inputs,omitempty" optional:"true"`
	Outputs       common.StepOutputs               `json:"outputs,omitempty" optional:"true"`
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
	Definition v1beta1.ComponentDefinitionSpec `json:"definition"`
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

// ListProjectResponse list project response body
type ListProjectResponse struct {
	Projects []*ProjectBase `json:"projects"`
	Total    int64          `json:"total"`
}

// ProjectBase project base model
type ProjectBase struct {
	Name        string    `json:"name"`
	Alias       string    `json:"alias"`
	Description string    `json:"description"`
	CreateTime  time.Time `json:"createTime"`
	UpdateTime  time.Time `json:"updateTime"`
	Owner       NameAlias `json:"owner,omitempty"`
}

// CreateProjectRequest create project request body
type CreateProjectRequest struct {
	Name        string `json:"name" validate:"checkname"`
	Alias       string `json:"alias" validate:"checkalias" optional:"true"`
	Description string `json:"description" optional:"true"`
	Owner       string `json:"owner" optional:"true"`
}

// UpdateProjectRequest update a project request body
type UpdateProjectRequest struct {
	Alias       string `json:"alias" validate:"checkalias" optional:"true"`
	Description string `json:"description" optional:"true"`
	Owner       string `json:"owner" optional:"true"`
}

// Env models the data of env in API
type Env struct {
	Name        string `json:"name"`
	Alias       string `json:"alias"`
	Description string `json:"description,omitempty"  optional:"true"`

	// Project defines the project this Env belongs to
	Project NameAlias `json:"project"`
	// Namespace defines the K8s namespace of the Env in control plane
	Namespace string `json:"namespace"`

	// Targets defines the name of delivery target that belongs to this env
	// In one project, a delivery target can only belong to one env.
	Targets []NameAlias `json:"targets,omitempty"  optional:"true"`

	CreateTime time.Time `json:"createTime"`
	UpdateTime time.Time `json:"updateTime"`
}

// ListEnvOptions list envs by query options
type ListEnvOptions struct {
	Project string `json:"project"`
}

// ListEnvResponse response the while env list
type ListEnvResponse struct {
	Envs  []*Env `json:"envs"`
	Total int64  `json:"total"`
}

// CreateEnvRequest contains the env data as request body
type CreateEnvRequest struct {
	Name        string `json:"name" validate:"checkname"`
	Alias       string `json:"alias" validate:"checkalias" optional:"true"`
	Description string `json:"description,omitempty"  optional:"true"`

	// Project defines the project this Env belongs to
	Project string `json:"project"`
	// Namespace defines the K8s namespace of the Env in control plane
	Namespace string `json:"namespace"`

	// Targets defines the name of delivery target that belongs to this env
	// In one project, a delivery target can only belong to one env.
	Targets []string `json:"targets,omitempty"  optional:"true"`

	// AllowTargetConflict means allow binding the targets that belong to other envs
	AllowTargetConflict bool `json:"allowTargetConflict,omitempty"  optional:"true"`
}

// UpdateEnvRequest defines the data of Env for update
type UpdateEnvRequest struct {
	Alias       string `json:"alias" validate:"checkalias" optional:"true"`
	Description string `json:"description,omitempty"  optional:"true"`
	// Targets defines the name of delivery target that belongs to this env
	// In one project, a delivery target can only belong to one env.
	Targets []string `json:"targets,omitempty"  optional:"true"`
}

// ListDefinitionResponse list definition response model
type ListDefinitionResponse struct {
	Definitions []*DefinitionBase `json:"definitions"`
}

// DetailDefinitionResponse get definition detail
type DetailDefinitionResponse struct {
	DefinitionBase
	APISchema *openapi3.Schema `json:"schema"`
	UISchema  utils.UISchema   `json:"uiSchema"`
}

// UpdateUISchemaRequest the request body struct about updated ui schema
type UpdateUISchemaRequest struct {
	DefinitionType string         `json:"type"`
	UISchema       utils.UISchema `json:"uiSchema"`
}

// UpdateDefinitionStatusRequest the request body struct about updated definition
// Only support set the status of definition
type UpdateDefinitionStatusRequest struct {
	DefinitionType string `json:"type"`
	HiddenInUI     bool   `json:"hiddenInUI"`
}

// DefinitionBase is the definition base model
type DefinitionBase struct {
	Name        string            `json:"name"`
	Alias       string            `json:"alias"`
	Description string            `json:"description"`
	Icon        string            `json:"icon"`
	Status      string            `json:"status"`
	Labels      map[string]string `json:"labels"`
	// WorkloadType the component workload type
	// Deprecated: it same as component.workload.type
	WorkloadType string `json:"workloadType,omitempty"`
	// OwnerAddon indicates which addon created this definition
	OwnerAddon   string                              `json:"ownerAddon"`
	Trait        *v1beta1.TraitDefinitionSpec        `json:"trait,omitempty"`
	Component    *v1beta1.ComponentDefinitionSpec    `json:"component,omitempty"`
	Policy       *v1beta1.PolicyDefinitionSpec       `json:"policy,omitempty"`
	WorkflowStep *v1beta1.WorkflowStepDefinitionSpec `json:"workflowStep,omitempty"`
}

// CreatePolicyRequest create app policy
type CreatePolicyRequest struct {
	// Name is the unique name of the policy.
	Name        string `json:"name" validate:"checkname"`
	Alias       string `json:"alias"`
	EnvName     string `json:"envName"`
	Description string `json:"description"`
	Type        string `json:"type" validate:"checkname"`
	// Properties json data
	Properties string `json:"properties"`

	// Bind this policy to workflow
	WorkflowPolicyBindings []WorkflowPolicyBinding `json:"workflowPolicyBind"`
}

// WorkflowPolicyBinding define the relation binding relationShip between policy and workflowStep
type WorkflowPolicyBinding struct {
	Name  string   `json:"name"`
	Steps []string `json:"steps"`
}

// UpdatePolicyRequest update policy
type UpdatePolicyRequest struct {
	Alias       string `json:"alias"`
	EnvName     string `json:"envName"`
	Description string `json:"description"`
	Type        string `json:"type" validate:"checkname"`
	// Properties json data
	Properties string `json:"properties"`

	// Bind this policy to workflow
	WorkflowPolicyBindings []WorkflowPolicyBinding `json:"workflowPolicyBind"`
}

// PolicyBase application policy base info
type PolicyBase struct {
	// Name is the unique name of the policy.
	Name        string `json:"name"`
	Alias       string `json:"alias"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Creator     string `json:"creator"`
	// Properties json data
	Properties *model.JSONStruct `json:"properties"`
	CreateTime time.Time         `json:"createTime"`
	UpdateTime time.Time         `json:"updateTime"`
	EnvName    string            `json:"envName"`
}

// DetailPolicyResponse app policy detail model
type DetailPolicyResponse struct {
	PolicyBase
	// Binding relationShip
	WorkflowPolicyBindings []WorkflowPolicyBinding `json:"workflowPolicyBind,omitempty"`
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
	Default     *bool          `json:"default"`
	EnvName     string         `json:"envName"`
}

// UpdateWorkflowRequest update or create application workflow
type UpdateWorkflowRequest struct {
	Alias       string         `json:"alias"  validate:"checkalias" optional:"true"`
	Description string         `json:"description" optional:"true"`
	Steps       []WorkflowStep `json:"steps,omitempty"`
	Default     *bool          `json:"default"`
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

const (
	// TriggerTypeWeb means trigger by web
	TriggerTypeWeb string = "web"
	// TriggerTypeAPI means trigger by api
	TriggerTypeAPI string = "api"
	// TriggerTypeWebhook means trigger by webhook
	TriggerTypeWebhook string = "webhook"
)

// DetailWorkflowRecordResponse get workflow record detail
type DetailWorkflowRecordResponse struct {
	WorkflowRecord
	DeployTime time.Time `json:"deployTime"`
	DeployUser string    `json:"deployUser"`
	Note       string    `json:"note"`
	// TriggerType the event trigger source, Web or API or Webhook
	TriggerType string `json:"triggerType"`
}

// WorkflowRecord workflow record
type WorkflowRecord struct {
	Name                string                     `json:"name"`
	Namespace           string                     `json:"namespace"`
	WorkflowName        string                     `json:"workflowName"`
	WorkflowAlias       string                     `json:"workflowAlias"`
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
	// TriggerType the event trigger source, Web or API or Webhook
	TriggerType string `json:"triggerType" validate:"oneof=web api webhook"`
	// Force set to True to ignore unfinished events.
	Force bool `json:"force"`
	// CodeInfo is the source code info of this deploy
	CodeInfo *model.CodeInfo `json:"codeInfo,omitempty"`
	// ImageInfo is the image code info of this deploy
	ImageInfo *model.ImageInfo `json:"imageInfo,omitempty"`
}

// ApplicationDeployResponse application deploy response body
type ApplicationDeployResponse struct {
	ApplicationRevisionBase
}

// ApplicationDockerhubWebhookResponse dockerhub webhook response body
type ApplicationDockerhubWebhookResponse struct {
	State       string `json:"state,omitempty"`
	Description string `json:"description,omitempty"`
	Context     string `json:"context,omitempty"`
	TargetURL   string `json:"target_url,omitempty"`
}

// VelaQLViewResponse query response
type VelaQLViewResponse map[string]interface{}

// PutApplicationEnvBindingRequest update app envbinding request body
type PutApplicationEnvBindingRequest struct {
}

// ListApplicationEnvBinding list app envBindings
type ListApplicationEnvBinding struct {
	EnvBindings []*EnvBindingBase `json:"envBindings"`
}

// CreateApplicationEnvbindingRequest new application env
type CreateApplicationEnvbindingRequest struct {
	EnvBinding
}

// CreateApplicationTraitRequest create application trait request
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
	Type        string `json:"type"`
	Alias       string `json:"alias,omitempty"`
	Description string `json:"description,omitempty"`
	// Properties json data
	Properties *model.JSONStruct `json:"properties"`
	CreateTime time.Time         `json:"createTime"`
	UpdateTime time.Time         `json:"updateTime"`
}

// CreateTargetRequest  create delivery target request body
type CreateTargetRequest struct {
	Name        string                 `json:"name" validate:"checkname"`
	Alias       string                 `json:"alias,omitempty" validate:"checkalias" optional:"true"`
	Project     string                 `json:"project" validate:"checkname"`
	Description string                 `json:"description,omitempty" optional:"true"`
	Cluster     *ClusterTarget         `json:"cluster,omitempty"`
	Variable    map[string]interface{} `json:"variable,omitempty"`
}

// UpdateTargetRequest only support full quantity update
type UpdateTargetRequest struct {
	Alias       string                 `json:"alias,omitempty" validate:"checkalias" optional:"true"`
	Description string                 `json:"description,omitempty" optional:"true"`
	Variable    map[string]interface{} `json:"variable,omitempty"`
}

// ClusterTarget kubernetes delivery target
type ClusterTarget struct {
	ClusterName string `json:"clusterName" validate:"checkname"`
	Namespace   string `json:"namespace" optional:"true"`
}

// DetailTargetResponse detail Target response
type DetailTargetResponse struct {
	TargetBase
}

// ListTargetResponse list delivery target response body
type ListTargetResponse struct {
	Targets []TargetBase `json:"targets"`
	Total   int64        `json:"total"`
}

// TargetBase Target base model
type TargetBase struct {
	Name         string                 `json:"name"`
	Alias        string                 `json:"alias,omitempty" validate:"checkalias" optional:"true"`
	Description  string                 `json:"description,omitempty" optional:"true"`
	Cluster      *ClusterTarget         `json:"cluster,omitempty"`
	ClusterAlias string                 `json:"clusterAlias,omitempty"`
	Variable     map[string]interface{} `json:"variable,omitempty"`
	CreateTime   time.Time              `json:"createTime"`
	UpdateTime   time.Time              `json:"updateTime"`
	AppNum       int64                  `json:"appNum,omitempty"`
	Project      NameAlias              `json:"project"`
}

// ApplicationRevisionBase application revision base spec
type ApplicationRevisionBase struct {
	CreateTime time.Time  `json:"createTime"`
	Version    string     `json:"version"`
	Status     string     `json:"status"`
	Reason     string     `json:"reason,omitempty"`
	DeployUser *NameAlias `json:"deployUser,omitempty"`
	Note       string     `json:"note"`
	EnvName    string     `json:"envName"`
	// SourceType the event trigger source, Web or API or Webhook
	TriggerType string `json:"triggerType"`
	// CodeInfo is the code info of this application revision
	CodeInfo *model.CodeInfo `json:"codeInfo,omitempty"`
	// ImageInfo is the image info of this application revision
	ImageInfo *model.ImageInfo `json:"imageInfo,omitempty"`
}

// ListRevisionsResponse list application revisions
type ListRevisionsResponse struct {
	Revisions []ApplicationRevisionBase `json:"revisions"`
	Total     int64                     `json:"total"`
}

// DetailRevisionResponse get application revision detail
type DetailRevisionResponse struct {
	model.ApplicationRevision
	DeployUser NameAlias `json:"deployUser,omitempty"`
}

// SystemInfoResponse get SystemInfo
type SystemInfoResponse struct {
	SystemInfo
	SystemVersion SystemVersion `json:"systemVersion"`
	StatisticInfo StatisticInfo `json:"statisticInfo,omitempty"`
}

// SystemInfo system info
type SystemInfo struct {
	PlatformID                  string             `json:"platformID"`
	EnableCollection            bool               `json:"enableCollection"`
	LoginType                   string             `json:"loginType" validate:"oneof=dex local"`
	InstallTime                 time.Time          `json:"installTime,omitempty"`
	DexUserDefaultProjects      []model.ProjectRef `json:"dexUserDefaultProjects,omitempty"`
	DexUserDefaultPlatformRoles []string           `json:"dexUserDefaultPlatformRoles,omitempty"`
}

// StatisticInfo generated by cronJob running in backend
type StatisticInfo struct {
	ClusterCount               string            `json:"clusterCount,omitempty"`
	AppCount                   string            `json:"appCount,omitempty"`
	EnableAddonList            map[string]string `json:"enableAddonList,omitempty"`
	ComponentDefinitionTopList []string          `json:"componentDefinitionTopList,omitempty"`
	TraitDefinitionTopList     []string          `json:"traitDefinitionTopList,omitempty"`
	WorkflowDefinitionTopList  []string          `json:"workflowDefinitionTopList,omitempty"`
	PolicyDefinitionTopList    []string          `json:"policyDefinitionTopList,omitempty"`
	UpdateTime                 time.Time         `json:"updateTime,omitempty"`
}

// SystemInfoRequest request by update SystemInfo
type SystemInfoRequest struct {
	EnableCollection       bool               `json:"enableCollection"`
	LoginType              string             `json:"loginType"`
	VelaAddress            string             `json:"velaAddress,omitempty"`
	DexUserDefaultProjects []model.ProjectRef `json:"dexUserDefaultProjects,omitempty"`
}

// SystemVersion contains KubeVela version
type SystemVersion struct {
	VelaVersion string `json:"velaVersion"`
	GitVersion  string `json:"gitVersion"`
}

// ChartVersionListResponse contains helm chart versions info
type ChartVersionListResponse struct {
	Versions repo.ChartVersions `json:"versions"`
}

// SimpleResponse simple response model for temporary
type SimpleResponse struct {
	Status string `json:"status"`
}

// LoginRequest is the request body for login
type LoginRequest struct {
	Code     string `json:"code,omitempty" optional:"true"`
	Username string `json:"username,omitempty" optional:"true"`
	Password string `json:"password,omitempty" optional:"true"`
}

// LoginResponse is the response of login request
type LoginResponse struct {
	User         *UserBase `json:"user"`
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken"`
}

// RefreshTokenResponse is the response of refresh token request
type RefreshTokenResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

// DexConfigResponse is the response of dex config
type DexConfigResponse struct {
	ClientID     string `json:"clientID"`
	ClientSecret string `json:"clientSecret"`
	RedirectURL  string `json:"redirectURL"`
	Issuer       string `json:"issuer"`
}

// DetailUserResponse is the response of user detail
type DetailUserResponse struct {
	UserBase
	Projects []*ProjectBase `json:"projects"`
	Roles    []NameAlias    `json:"roles"`
}

// ProjectUserBase project user base
type ProjectUserBase struct {
	UserName   string    `json:"name"`
	UserAlias  string    `json:"alias"`
	UserRoles  []string  `json:"userRoles"`
	CreateTime time.Time `json:"createTime"`
	UpdateTime time.Time `json:"updateTime"`
}

// ListProjectUsersResponse the response body that list users belong to a project
type ListProjectUsersResponse struct {
	Users []*ProjectUserBase `json:"users"`
	Total int64              `json:"total"`
}

// CreateUserRequest create user request
type CreateUserRequest struct {
	Name     string   `json:"name" validate:"checkname"`
	Alias    string   `json:"alias,omitempty" validate:"checkalias" optional:"true"`
	Email    string   `json:"email" validate:"checkemail"`
	Password string   `json:"password" validate:"checkpassword"`
	Roles    []string `json:"roles"`
}

// UpdateUserRequest update user request
type UpdateUserRequest struct {
	Alias    string    `json:"alias,omitempty" optional:"true"`
	Password string    `json:"password,omitempty" validate:"checkpassword" optional:"true"`
	Email    string    `json:"email,omitempty" validate:"checkemail" optional:"true"`
	Roles    *[]string `json:"roles"`
}

// ListUserResponse list user response
type ListUserResponse struct {
	Users []*DetailUserResponse `json:"users"`
	Total int64                 `json:"total"`
}

// UserBase is the base info of user
type UserBase struct {
	CreateTime    time.Time `json:"createTime"`
	LastLoginTime time.Time `json:"lastLoginTime"`
	Name          string    `json:"name"`
	Email         string    `json:"email"`
	Alias         string    `json:"alias,omitempty"`
	Disabled      bool      `json:"disabled"`
}

// ListUserOptions list user options
type ListUserOptions struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Alias string `json:"alias"`
}

// GetLoginTypeResponse get login type response
type GetLoginTypeResponse struct {
	LoginType string `json:"loginType"`
}

// AddProjectUserRequest the request body that add user to project
type AddProjectUserRequest struct {
	UserName  string   `json:"userName" validate:"checkname"`
	UserRoles []string `json:"userRoles"`
}

// UpdateProjectUserRequest the request body that update user role in a project
type UpdateProjectUserRequest struct {
	UserRoles []string `json:"userRoles"`
}

// CreateRoleRequest the request body that create a role
type CreateRoleRequest struct {
	Name        string   `json:"name" validate:"checkname"`
	Alias       string   `json:"alias" validate:"checkalias"`
	Permissions []string `json:"permissions"`
}

// UpdateRoleRequest the request body that update a role
type UpdateRoleRequest struct {
	Alias       string   `json:"alias" validate:"checkalias"`
	Permissions []string `json:"permissions"`
}

// RoleBase the base struct of role
type RoleBase struct {
	CreateTime  time.Time   `json:"createTime"`
	UpdateTime  time.Time   `json:"updateTime"`
	Name        string      `json:"name"`
	Alias       string      `json:"alias,omitempty"`
	Permissions []NameAlias `json:"permissions"`
}

// ListRolesResponse the response body of list roles
type ListRolesResponse struct {
	Total int64       `json:"total"`
	Roles []*RoleBase `json:"roles"`
}

// PermissionTemplateBase the perm policy template base struct
type PermissionTemplateBase struct {
	Name       string    `json:"name"`
	Alias      string    `json:"alias"`
	Resources  []string  `json:"resources"`
	Actions    []string  `json:"actions"`
	Effect     string    `json:"effect"`
	CreateTime time.Time `json:"createTime"`
	UpdateTime time.Time `json:"updateTime"`
}

// PermissionBase the perm policy base struct
type PermissionBase struct {
	Name       string    `json:"name"`
	Alias      string    `json:"alias"`
	Resources  []string  `json:"resources"`
	Actions    []string  `json:"actions"`
	Effect     string    `json:"effect"`
	CreateTime time.Time `json:"createTime"`
	UpdateTime time.Time `json:"updateTime"`
}

// UpdatePermissionRequest the request body that updating a permission policy
type UpdatePermissionRequest struct {
	Alias     string   `json:"alias" validate:"checkalias"`
	Resources []string `json:"resources"`
	Actions   []string `json:"actions"`
	Effect    string   `json:"effect" validate:"oneof=Allow Deny"`
}

// CreatePermissionRequest the request body that creating a permission policy
type CreatePermissionRequest struct {
	Name      string   `json:"name" validate:"checkname"`
	Alias     string   `json:"alias" validate:"checkalias"`
	Resources []string `json:"resources"`
	Actions   []string `json:"actions"`
	Effect    string   `json:"effect" validate:"oneof=Allow Deny"`
}

// LoginUserInfoResponse the response body of login user info
type LoginUserInfoResponse struct {
	UserBase
	Projects            []*ProjectBase              `json:"projects"`
	PlatformPermissions []PermissionBase            `json:"platformPermissions"`
	ProjectPermissions  map[string][]PermissionBase `json:"projectPermissions"`
}

// ChartRepoResponse the response body of  chart repo
type ChartRepoResponse struct {
	URL        string `json:"url"`
	SecretName string `json:"secretName"`
}

// ChartRepoResponseList the response body of list chart repo
type ChartRepoResponseList struct {
	ChartRepoResponse []*ChartRepoResponse `json:"repos"`
}

// ImageInfo the docker image info
type ImageInfo struct {
	Name        string                 `json:"name"`
	SecretNames []string               `json:"secretNames"`
	Registry    string                 `json:"registry"`
	Message     string                 `json:"message,omitempty"`
	Info        *registryv1.ConfigFile `json:"info,omitempty"`
	Size        int64                  `json:"size"`
	Manifest    *registryv1.Manifest   `json:"manifest"`
}

// ImageRegistry the image repository info
type ImageRegistry struct {
	Name       string `json:"name"`
	SecretName string `json:"secretName"`
	Domain     string `json:"domain"`
}

// ListImageRegistryResponse the response struct of listing the image registries
type ListImageRegistryResponse struct {
	Registries []ImageRegistry `json:"registries"`
}

// CloudShellPrepareResponse the response for the cloud shell environment creation
type CloudShellPrepareResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}
