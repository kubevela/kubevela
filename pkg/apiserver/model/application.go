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

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
)

// Application database model
type Application struct {
	Model
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Description string            `json:"description"`
	Icon        string            `json:"icon"`
	Labels      map[string]string `json:"labels,omitempty"`
	ClusterList []string          `json:"clusterList,omitempty"`
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
	if a.Namespace != "" {
		index["namespace"] = a.Namespace
	}
	return index
}

// ApplicationComponent component database model
type ApplicationComponent struct {
	Model
	AppPrimaryKey string            `json:"appPrimaryKey"`
	Description   string            `json:"description,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	Icon          string            `json:"icon,omitempty"`
	Creator       string            `json:"creator"`
	Name          string            `json:"name"`
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
	Model
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
	Type       string      `json:"type"`
	Properties *JSONStruct `json:"properties,omitempty"`
}

// DeployEventInit event status init
var DeployEventInit = "init"

// DeployEventRunning event status running
var DeployEventRunning = "running"

// DeployEventComplete event status complete
var DeployEventComplete = "complete"

// DeployEventFail event status failure
var DeployEventFail = "failure"

// DeployEvent record each application deployment event.
type DeployEvent struct {
	Model
	AppPrimaryKey string `json:"appPrimaryKey"`
	Version       string `json:"version"`
	// ApplyAppConfig Stores the application configuration during the current deploy.
	ApplyAppConfig string `json:"applyAppConfig,omitempty"`

	// Deploy event status
	Status string `json:"status"`
	Reason string `json:"reason"`

	// The user that triggers the deploy.
	DeployUser string `json:"deployUser"`

	// Information that users can note.
	Commit string `json:"commit"`
	// SourceType the event trigger source, Web or API
	SourceType string `json:"sourceType"`
}

// TableName return custom table name
func (a *DeployEvent) TableName() string {
	return tableNamePrefix + "deploy_event"
}

// PrimaryKey return custom primary key
func (a *DeployEvent) PrimaryKey() string {
	return fmt.Sprintf("%s-%s", a.AppPrimaryKey, a.Version)
}

// Index return custom index
func (a *DeployEvent) Index() map[string]string {
	index := make(map[string]string)
	if a.Version != "" {
		index["version"] = a.Version
	}
	if a.AppPrimaryKey != "" {
		index["appPrimaryKey"] = a.AppPrimaryKey
	}
	if a.DeployUser != "" {
		index["deployUser"] = a.DeployUser
	}
	if a.Status != "" {
		index["status"] = a.Status
	}
	if a.SourceType != "" {
		index["sourceType"] = a.SourceType
	}
	return index
}
