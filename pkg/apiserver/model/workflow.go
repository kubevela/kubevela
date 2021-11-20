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
	"strconv"
	"time"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
)

func init() {
	RegistModel(&Workflow{})
	RegistModel(&WorkflowRecord{})
}

// Workflow application delivery database model
type Workflow struct {
	Model
	Name        string `json:"name"`
	Alias       string `json:"alias"`
	Description string `json:"description"`
	// Workflow used by the default
	Default       bool           `json:"default"`
	AppPrimaryKey string         `json:"appPrimaryKey"`
	EnvName       string         `json:"envName"`
	Steps         []WorkflowStep `json:"steps,omitempty"`
}

// WorkflowStep defines how to execute a workflow step.
type WorkflowStep struct {
	// Name is the unique name of the workflow step.
	Name        string             `json:"name"`
	Type        string             `json:"type"`
	Group       string             `json:"group"`
	Description string             `json:"description"`
	OrderIndex  int                `json:"orderIndex"`
	Inputs      common.StepInputs  `json:"inputs,omitempty"`
	Outputs     common.StepOutputs `json:"outputs,omitempty"`
	DependsOn   []string           `json:"dependsOn"`
	Properties  *JSONStruct        `json:"properties,omitempty"`
}

// TableName return custom table name
func (w *Workflow) TableName() string {
	return tableNamePrefix + "workflow"
}

// PrimaryKey return custom primary key
func (w *Workflow) PrimaryKey() string {
	return fmt.Sprintf("%s-%s", w.AppPrimaryKey, w.Name)
}

// Index return custom primary key
func (w *Workflow) Index() map[string]string {
	index := make(map[string]string)
	if w.Name != "" {
		index["name"] = w.Name
	}
	if w.AppPrimaryKey != "" {
		index["appPrimaryKey"] = w.AppPrimaryKey
	}
	if w.EnvName != "" {
		index["envName"] = w.EnvName
	}
	index["default"] = strconv.FormatBool(w.Default)
	return index
}

// WorkflowRecord is the workflow record database model
type WorkflowRecord struct {
	Model
	WorkflowName       string                      `json:"workflowName"`
	AppPrimaryKey      string                      `json:"appPrimaryKey"`
	RevisionPrimaryKey string                      `json:"revisionPrimaryKey"`
	Name               string                      `json:"name"`
	Namespace          string                      `json:"namespace"`
	StartTime          time.Time                   `json:"startTime,omitempty"`
	Finished           string                      `json:"finished"`
	Steps              []common.WorkflowStepStatus `json:"steps,omitempty"`
	Status             string                      `json:"status"`
}

// TableName return custom table name
func (w *WorkflowRecord) TableName() string {
	return tableNamePrefix + "workflow_record"
}

// PrimaryKey return custom primary key
func (w *WorkflowRecord) PrimaryKey() string {
	return w.Name
}

// Index return custom primary key
func (w *WorkflowRecord) Index() map[string]string {
	index := make(map[string]string)
	if w.Name != "" {
		index["name"] = w.Name
	}
	if w.Namespace != "" {
		index["namespace"] = w.Namespace
	}
	if w.WorkflowName != "" {
		index["workflowPrimaryKey"] = w.WorkflowName
	}
	if w.AppPrimaryKey != "" {
		index["appPrimaryKey"] = w.AppPrimaryKey
	}
	if w.RevisionPrimaryKey != "" {
		index["revisionPrimaryKey"] = w.RevisionPrimaryKey
	}
	if w.Finished != "" {
		index["finished"] = w.Finished
	}
	return index
}
