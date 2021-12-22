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
	"time"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
)

func init() {
	RegistModel(&ApplicationComponent{}, &ApplicationPolicy{}, &Application{}, &ApplicationRevision{}, &ApplicationWebhook{})
}

// Application application delivery model
type Application struct {
	BaseModel
	Name         string            `json:"name"`
	Alias        string            `json:"alias"`
	Project      string            `json:"project"`
	Description  string            `json:"description"`
	Icon         string            `json:"icon"`
	Labels       map[string]string `json:"labels,omitempty"`
	WebhookToken string            `json:"webhookToken"`
}

// TableName return custom table name
func (a *Application) TableName() string {
	return tableNamePrefix + "application"
}

// PrimaryKey return custom primary key
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
	// GitInfo is the git info of this application revision
	GitInfo *GitInfo `json:"gitInfo,omitempty"`
}

// GitInfo is the git info for webhook request
type GitInfo struct {
	// Commit is the commit hash
	Commit string `json:"commit,omitempty"`
	// Branch is the branch name
	Branch string `json:"branch,omitempty"`
	// User is the user name
	User string `json:"user,omitempty"`
}

// TableName return custom table name
func (a *ApplicationRevision) TableName() string {
	return tableNamePrefix + "application_revision"
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

// ApplicationWebhook is the model for webhook request
type ApplicationWebhook struct {
	BaseModel
	AppPrimaryKey string `json:"appPrimaryKey"`
	Token         string `json:"token"`
}

// TableName return custom table name
func (w *ApplicationWebhook) TableName() string {
	return tableNamePrefix + "webhook"
}

// PrimaryKey return custom primary key
func (w *ApplicationWebhook) PrimaryKey() string {
	return w.Token
}

// Index return custom index
func (w *ApplicationWebhook) Index() map[string]string {
	index := make(map[string]string)
	if w.Token != "" {
		index["token"] = w.Token
	}
	return index
}
