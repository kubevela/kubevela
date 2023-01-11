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

	"github.com/kubevela/workflow/api/v1alpha1"
)

func init() {
	RegisterModel(&PipelineContext{})
	RegisterModel(&Pipeline{})
}

// Structs copied from workflow/api/v1alpha1/types.go

// WorkflowSpec defines workflow steps and other attributes
type WorkflowSpec struct {
	Mode  *v1alpha1.WorkflowExecuteMode `json:"mode,omitempty"`
	Steps []WorkflowStep                `json:"steps,omitempty"`
}

// Pipeline is the model of pipeline
type Pipeline struct {
	BaseModel
	Spec        WorkflowSpec
	Name        string `json:"name"`
	Project     string `json:"project"`
	Alias       string `json:"alias"`
	Description string `json:"description"`
}

// PrimaryKey return custom primary key
func (p Pipeline) PrimaryKey() string {
	return fmt.Sprintf("%s-%s", p.Project, p.Name)
}

// TableName return custom table name
func (p Pipeline) TableName() string {
	return tableNamePrefix + "pipeline"
}

// ShortTableName is the compressed version of table name for kubeapi storage and others
func (p Pipeline) ShortTableName() string {
	return "pipeline"
}

// Index return custom index
func (p Pipeline) Index() map[string]interface{} {
	var index = make(map[string]interface{})
	if p.Project != "" {
		index["project"] = p.Project
	}
	if p.Name != "" {
		index["name"] = p.Name
	}
	return index
}

// Value is a k-v pair
type Value struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// PipelineContext is pipeline's context groups
type PipelineContext struct {
	BaseModel
	PipelineName string             `json:"pipelineName"`
	ProjectName  string             `json:"projectName"`
	Contexts     map[string][]Value `json:"contexts"`
}

// TableName return custom table name
func (c *PipelineContext) TableName() string {
	return tableNamePrefix + "pipeline_context"
}

// ShortTableName is the compressed version of table name for kubeapi storage and others
func (c *PipelineContext) ShortTableName() string {
	return "pp-ctx"
}

// PrimaryKey return custom primary key
func (c *PipelineContext) PrimaryKey() string {
	return fmt.Sprintf("%s-%s", c.ProjectName, c.PipelineName)
}

// Index return custom index
func (c *PipelineContext) Index() map[string]interface{} {
	index := make(map[string]interface{})
	if c.ProjectName != "" {
		index["project_name"] = c.ProjectName
	}
	return index
}
