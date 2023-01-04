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

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
)

func init() {
	RegisterModel(&Workflow{})
	RegisterModel(&WorkflowRecord{})
}

// Finished means the workflow record is finished
const Finished = "true"

// UnFinished means the workflow record is not finished
const UnFinished = "false"

// Workflow application delivery database model
type Workflow struct {
	BaseModel
	Name        string `json:"name"`
	Alias       string `json:"alias"`
	Description string `json:"description"`
	// Workflow used by the default
	Default       *bool                                `json:"default"`
	AppPrimaryKey string                               `json:"appPrimaryKey"`
	EnvName       string                               `json:"envName"`
	Mode          workflowv1alpha1.WorkflowExecuteMode `json:"mode,omitempty"`
	Steps         []WorkflowStep                       `json:"steps,omitempty"`
}

// WorkflowStep defines how to execute a workflow step.
type WorkflowStep struct {
	WorkflowStepBase `json:",inline"`
	SubSteps         []WorkflowStepBase `json:"subSteps,omitempty"`
}

// WorkflowStepBase is the step base of workflow
type WorkflowStepBase struct {
	// Name is the unique name of the workflow step.
	Name        string                             `json:"name"`
	Alias       string                             `json:"alias"`
	Type        string                             `json:"type"`
	Description string                             `json:"description"`
	OrderIndex  int                                `json:"orderIndex"`
	Inputs      workflowv1alpha1.StepInputs        `json:"inputs,omitempty"`
	Outputs     workflowv1alpha1.StepOutputs       `json:"outputs,omitempty"`
	DependsOn   []string                           `json:"dependsOn"`
	Properties  *JSONStruct                        `json:"properties,omitempty"`
	Meta        *workflowv1alpha1.WorkflowStepMeta `json:"meta,omitempty"`
	If          string                             `json:"if,omitempty"`
	Timeout     string                             `json:"timeout,omitempty"`
}

// TableName return custom table name
func (w *Workflow) TableName() string {
	return tableNamePrefix + "workflow"
}

// ShortTableName is the compressed version of table name for kubeapi storage and others
func (w *Workflow) ShortTableName() string {
	return "wf"
}

// PrimaryKey return custom primary key
func (w *Workflow) PrimaryKey() string {
	return fmt.Sprintf("%s-%s", w.AppPrimaryKey, w.Name)
}

// Index return custom primary key
func (w *Workflow) Index() map[string]interface{} {
	index := make(map[string]interface{})
	if w.Name != "" {
		index["name"] = w.Name
	}
	if w.AppPrimaryKey != "" {
		index["appPrimaryKey"] = w.AppPrimaryKey
	}
	if w.EnvName != "" {
		index["envName"] = w.EnvName
	}
	if w.Default != nil {
		index["default"] = *w.Default
	}

	return index
}

// WorkflowRecord is the workflow record database model
type WorkflowRecord struct {
	BaseModel
	WorkflowName  string `json:"workflowName"`
	WorkflowAlias string `json:"workflowAlias"`
	AppPrimaryKey string `json:"appPrimaryKey"`
	// RevisionPrimaryKey: should be assigned the version(PublishVersion)
	RevisionPrimaryKey string               `json:"revisionPrimaryKey"`
	Name               string               `json:"name"`
	Namespace          string               `json:"namespace"`
	StartTime          time.Time            `json:"startTime,omitempty"`
	EndTime            time.Time            `json:"endTime,omitempty"`
	Finished           string               `json:"finished"`
	Steps              []WorkflowStepStatus `json:"steps,omitempty"`
	Status             string               `json:"status"`
	Message            string               `json:"message"`
	Mode               string               `json:"mode"`
	ContextValue       map[string]string    `json:"contextValue,omitempty"`
}

// WorkflowStepStatus is the workflow step status database model
type WorkflowStepStatus struct {
	StepStatus     `json:",inline"`
	SubStepsStatus []StepStatus `json:"subSteps,omitempty"`
}

// StepStatus is the workflow step status database model
type StepStatus struct {
	ID               string                             `json:"id"`
	Name             string                             `json:"name"`
	Alias            string                             `json:"alias"`
	Type             string                             `json:"type,omitempty"`
	Phase            workflowv1alpha1.WorkflowStepPhase `json:"phase,omitempty"`
	Message          string                             `json:"message,omitempty"`
	Reason           string                             `json:"reason,omitempty"`
	FirstExecuteTime time.Time                          `json:"firstExecuteTime,omitempty"`
	LastExecuteTime  time.Time                          `json:"lastExecuteTime,omitempty"`
}

// TableName return custom table name
func (w *WorkflowRecord) TableName() string {
	return tableNamePrefix + "workflow_record"
}

// ShortTableName is the compressed version of table name for kubeapi storage and others
func (w *WorkflowRecord) ShortTableName() string {
	return "wfr"
}

// PrimaryKey return custom primary key
func (w *WorkflowRecord) PrimaryKey() string {
	return w.Name
}

// Index return custom primary key
func (w *WorkflowRecord) Index() map[string]interface{} {
	index := make(map[string]interface{})
	if w.Name != "" {
		index["name"] = w.Name
	}
	if w.Namespace != "" {
		index["namespace"] = w.Namespace
	}
	if w.WorkflowName != "" {
		index["workflowName"] = w.WorkflowName
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
	if w.Status != "" {
		index["status"] = w.Status
	}
	return index
}
