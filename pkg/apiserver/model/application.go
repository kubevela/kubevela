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

import (
	"fmt"
	"strings"
	"time"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
)

func init() {
	RegisterModel(&ApplicationComponent{}, &ApplicationPolicy{}, &Application{}, &ApplicationRevision{}, &ApplicationTrigger{})
}

// Application application delivery model
type Application struct {
	BaseModel
	Name        string            `json:"name"`
	Alias       string            `json:"alias"`
	Project     string            `json:"project"`
	Description string            `json:"description"`
	Icon        string            `json:"icon"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// TableName return custom table name
func (a *Application) TableName() string {
	return tableNamePrefix + "application"
}

// ShortTableName is the compressed version of table name for kubeapi storage and others
func (a *Application) ShortTableName() string {
	return "app"
}

// PrimaryKey return custom primary key
// the app primary key is the app name, so the app name is globally unique in every namespace
// when the app is synced from CR, the first synced one be same with app name,
// if there's any conflicts, the name will be composed by <appname>-<namespace>
func (a *Application) PrimaryKey() string {
	return a.Name
}

// Index return custom index
func (a *Application) Index() map[string]string {
	index := make(map[string]string)
	if a.Name != "" {
		index["name"] = a.Name
	}
	if a.Project != "" {
		index["project"] = a.Project
	}
	return index
}

// GetAppNameForSynced will trim namespace suffix for synced CR
func (a *Application) GetAppNameForSynced() string {
	if a.Labels == nil {
		return a.Name
	}
	namespace := a.Labels[LabelSyncNamespace]
	if namespace == "" {
		return a.Name
	}
	return strings.TrimSuffix(a.Name, "-"+namespace)
}

// IsSynced answer if the app is synced one
func (a *Application) IsSynced() bool {
	if a.Labels == nil {
		return false
	}
	sot := a.Labels[LabelSourceOfTruth]
	if sot == FromCR || sot == FromInner {
		return true
	}
	return false
}

// ClusterSelector cluster selector
type ClusterSelector struct {
	Name string `json:"name"`
	// Adapt to a scenario where only one Namespace is available or a user-defined Namespace is available.
	Namespace string `json:"namespace,omitempty"`
}

// ComponentSelector component selector
type ComponentSelector struct {
	Components []string `json:"components"`
}

// ApplicationComponent component database model
type ApplicationComponent struct {
	BaseModel
	AppPrimaryKey string            `json:"appPrimaryKey"`
	Description   string            `json:"description,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	Icon          string            `json:"icon,omitempty"`
	Creator       string            `json:"creator"`
	Name          string            `json:"name"`
	Alias         string            `json:"alias"`
	Type          string            `json:"type"`
	Main          bool              `json:"main"`
	// ExternalRevision specified the component revisionName
	ExternalRevision string             `json:"externalRevision,omitempty"`
	Properties       *JSONStruct        `json:"properties,omitempty"`
	DependsOn        []string           `json:"dependsOn,omitempty"`
	Inputs           common.StepInputs  `json:"inputs,omitempty"`
	Outputs          common.StepOutputs `json:"outputs,omitempty"`
	// Traits define the trait of one component, the type must be array to keep the order.
	Traits []ApplicationTrait `json:"traits,omitempty"`
	// scopes in ApplicationComponent defines the component-level scopes
	// the format is <scope-type:scope-instance-name> pairs, the key represents type of `ScopeDefinition` while the value represent the name of scope instance.
	Scopes map[string]string `json:"scopes,omitempty"`
}

// TableName return custom table name
func (a *ApplicationComponent) TableName() string {
	return tableNamePrefix + "application_component"
}

// ShortTableName is the compressed version of table name for kubeapi storage and others
func (a *ApplicationComponent) ShortTableName() string {
	return "app_cmp"
}

// PrimaryKey return custom primary key
func (a *ApplicationComponent) PrimaryKey() string {
	return fmt.Sprintf("%s-%s", a.AppPrimaryKey, a.Name)
}

// Index return custom index
func (a *ApplicationComponent) Index() map[string]string {
	index := make(map[string]string)
	if a.Name != "" {
		index["name"] = a.Name
	}
	if a.AppPrimaryKey != "" {
		index["appPrimaryKey"] = a.AppPrimaryKey
	}
	if a.Type != "" {
		index["type"] = a.Type
	}
	return index
}

// ApplicationPolicy app policy
type ApplicationPolicy struct {
	BaseModel
	AppPrimaryKey string      `json:"appPrimaryKey"`
	Name          string      `json:"name"`
	Description   string      `json:"description"`
	Type          string      `json:"type"`
	Creator       string      `json:"creator"`
	Properties    *JSONStruct `json:"properties,omitempty"`
}

// TableName return custom table name
func (a *ApplicationPolicy) TableName() string {
	return tableNamePrefix + "application_policy"
}

// ShortTableName is the compressed version of table name for kubeapi storage and others
func (a *ApplicationPolicy) ShortTableName() string {
	return "app_plc"
}

// PrimaryKey return custom primary key
func (a *ApplicationPolicy) PrimaryKey() string {
	return fmt.Sprintf("%s-%s", a.AppPrimaryKey, a.Name)
}

// Index return custom index
func (a *ApplicationPolicy) Index() map[string]string {
	index := make(map[string]string)
	if a.Name != "" {
		index["name"] = a.Name
	}
	if a.AppPrimaryKey != "" {
		index["appPrimaryKey"] = a.AppPrimaryKey
	}
	if a.Type != "" {
		index["type"] = a.Type
	}
	return index
}

// ApplicationTrait application trait
type ApplicationTrait struct {
	Alias       string      `json:"alias"`
	Description string      `json:"description"`
	Type        string      `json:"type"`
	Properties  *JSONStruct `json:"properties,omitempty"`
	CreateTime  time.Time   `json:"createTime"`
	UpdateTime  time.Time   `json:"updateTime"`
}

// RevisionStatusInit event status init
var RevisionStatusInit = "init"

// RevisionStatusRunning event status running
var RevisionStatusRunning = "running"

// RevisionStatusComplete event status complete
var RevisionStatusComplete = "complete"

// RevisionStatusFail event status failure
var RevisionStatusFail = "failure"

// RevisionStatusTerminated event status terminated
var RevisionStatusTerminated = "terminated"

// RevisionStatusRollback event status rollback
var RevisionStatusRollback = "rollback"

// ApplicationRevision be created when an application initiates deployment and describes the phased version of the application.
type ApplicationRevision struct {
	BaseModel
	AppPrimaryKey   string `json:"appPrimaryKey"`
	Version         string `json:"version"`
	RollbackVersion string `json:"rollbackVersion,omitempty"`
	// ApplyAppConfig Stores the application configuration during the current deploy.
	ApplyAppConfig string `json:"applyAppConfig,omitempty"`

	// Deploy event status
	Status string `json:"status"`
	Reason string `json:"reason"`

	// The user that triggers the deploy.
	DeployUser string `json:"deployUser"`

	// Information that users can note.
	Note string `json:"note"`
	// TriggerType the event trigger source, Web or API
	TriggerType string `json:"triggerType"`

	// WorkflowName deploy controller by workflow
	WorkflowName string `json:"workflowName"`
	// EnvName is the env name of this application revision
	EnvName string `json:"envName"`
	// CodeInfo is the code info of this application revision
	CodeInfo *CodeInfo `json:"codeInfo,omitempty"`
	// ImageInfo is the image info of this application revision
	ImageInfo *ImageInfo `json:"imageInfo,omitempty"`
}

// CodeInfo is the code info for webhook request
type CodeInfo struct {
	// Commit is the commit hash
	Commit string `json:"commit,omitempty"`
	// Branch is the branch name
	Branch string `json:"branch,omitempty"`
	// User is the user name
	User string `json:"user,omitempty"`
}

// ImageInfo is the image info for webhook request
type ImageInfo struct {
	// Type is the image type, ACR or Harbor or DockerHub
	Type string `json:"type"`
	// Resource is the image resource
	Resource *ImageResource `json:"resource,omitempty"`
	// Repository is the image repository
	Repository *ImageRepository `json:"repository,omitempty"`
}

// ImageResource is the image resource
type ImageResource struct {
	// Digest is the image digest
	Digest string `json:"digest"`
	// Tag is the image tag
	Tag string `json:"tag"`
	// URL is the image url
	URL string `json:"url"`
	// CreateTime is the image create time
	CreateTime time.Time `json:"createTime,omitempty"`
}

// ImageRepository is the image repository
type ImageRepository struct {
	// Name is the image repository name
	Name string `json:"name"`
	// Namespace is the image repository namespace
	Namespace string `json:"namespace"`
	// FullName is the image repository full name
	FullName string `json:"fullName"`
	// Region is the image repository region
	Region string `json:"region,omitempty"`
	// Type is the image repository type, public or private
	Type string `json:"type"`
	// CreateTime is the image repository create time
	CreateTime time.Time `json:"createTime,omitempty"`
}

// TableName return custom table name
func (a *ApplicationRevision) TableName() string {
	return tableNamePrefix + "application_revision"
}

// ShortTableName is the compressed version of table name for kubeapi storage and others
func (a *ApplicationRevision) ShortTableName() string {
	return "app_rev"
}

// PrimaryKey return custom primary key
func (a *ApplicationRevision) PrimaryKey() string {
	return fmt.Sprintf("%s-%s", a.AppPrimaryKey, a.Version)
}

// Index return custom index
func (a *ApplicationRevision) Index() map[string]string {
	index := make(map[string]string)
	if a.Version != "" {
		index["version"] = a.Version
	}
	if a.AppPrimaryKey != "" {
		index["appPrimaryKey"] = a.AppPrimaryKey
	}
	if a.WorkflowName != "" {
		index["workflowName"] = a.WorkflowName
	}
	if a.DeployUser != "" {
		index["deployUser"] = a.DeployUser
	}
	if a.Status != "" {
		index["status"] = a.Status
	}
	if a.TriggerType != "" {
		index["triggerType"] = a.TriggerType
	}
	if a.EnvName != "" {
		index["envName"] = a.EnvName
	}
	return index
}

// ApplicationTrigger is the model for trigger
type ApplicationTrigger struct {
	BaseModel
	AppPrimaryKey string `json:"appPrimaryKey"`
	WorkflowName  string `json:"workflowName,omitempty"`
	Name          string `json:"name"`
	Alias         string `json:"alias,omitempty"`
	Description   string `json:"description,omitempty"`
	Token         string `json:"token"`
	Type          string `json:"type"`
	PayloadType   string `json:"payloadType"`
}

const (
	// PayloadTypeCustom is the payload type custom
	PayloadTypeCustom = "custom"
	// PayloadTypeDockerhub is the payload type dockerhub
	PayloadTypeDockerhub = "dockerhub"
	// PayloadTypeACR is the payload type acr
	PayloadTypeACR = "acr"
	// PayloadTypeHarbor is the payload type harbor
	PayloadTypeHarbor = "harbor"
	// PayloadTypeJFrog is the payload type jfrog
	PayloadTypeJFrog = "jfrog"

	// ComponentTypeWebservice is the component type webservice
	ComponentTypeWebservice = "webservice"
	// ComponentTypeWorker is the component type worker
	ComponentTypeWorker = "worker"
	// ComponentTypeTask is the component type task
	ComponentTypeTask = "task"
)

const (
	// HarborEventTypePushArtifact is the event type PUSH_ARTIFACT
	HarborEventTypePushArtifact = "PUSH_ARTIFACT"
	// JFrogEventTypePush is push event type of jfrog webhook
	JFrogEventTypePush = "pushed"
	// JFrogDomainDocker is webhook domain of jfrog docker
	JFrogDomainDocker = "docker"
)

// TableName return custom table name
func (w *ApplicationTrigger) TableName() string {
	return tableNamePrefix + "trigger"
}

// ShortTableName is the compressed version of table name for kubeapi storage and others
func (w *ApplicationTrigger) ShortTableName() string {
	return "app_tg"
}

// PrimaryKey return custom primary key
func (w *ApplicationTrigger) PrimaryKey() string {
	return w.Token
}

// Index return custom index
func (w *ApplicationTrigger) Index() map[string]string {
	index := make(map[string]string)
	if w.AppPrimaryKey != "" {
		index["appPrimaryKey"] = w.AppPrimaryKey
	}
	if w.Token != "" {
		index["token"] = w.Token
	}
	if w.Name != "" {
		index["name"] = w.Name
	}
	if w.Type != "" {
		index["type"] = w.Type
	}
	return index
}
