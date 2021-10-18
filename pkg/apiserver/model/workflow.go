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
	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
)

// Workflow workflow database model
type Workflow struct {
	Model
	Name      string         `json:"name"`
	Namespace string         `json:"namespace"`
	Enable    bool           `json:"enable"`
	Steps     []WorkflowStep `json:"steps,omitempty"`
}

// WorkflowStep defines how to execute a workflow step.
type WorkflowStep struct {
	// Name is the unique name of the workflow step.
	Name       string             `json:"name"`
	Type       string             `json:"type"`
	Properties *JSONStruct        `json:"properties,omitempty"`
	DependsOn  []string           `json:"dependsOn,omitempty"`
	Inputs     common.StepInputs  `json:"inputs,omitempty"`
	Outputs    common.StepOutputs `json:"outputs,omitempty"`
}

// TableName return custom table name
func (w *Workflow) TableName() string {
	return tableNamePrefix + "application_component"
}

// PrimaryKey return custom primary key
func (w *Workflow) PrimaryKey() string {
	return w.Name
}

// Index return custom primary key
func (w *Workflow) Index() map[string]string {
	index := make(map[string]string)
	if w.Name != "" {
		index["name"] = w.Name
	}
	if w.Namespace != "" {
		index["namespace"] = w.Namespace
	}
	return index
}
