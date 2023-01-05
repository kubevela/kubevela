/*
Copyright 2022 The KubeVela Authors.

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
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
)

// CreateEnvBindingModel assemble the EnvBinding model from DTO
func CreateEnvBindingModel(app *model.Application, req apisv1.CreateApplicationEnvbindingRequest) model.EnvBinding {
	envBinding := model.EnvBinding{
		AppPrimaryKey: app.Name,
		Name:          req.Name,
		AppDeployName: app.Name,
	}
	return envBinding
}

// ConvertToEnvBindingModel assemble the EnvBinding model from DTO
func ConvertToEnvBindingModel(app *model.Application, envBind apisv1.EnvBinding) *model.EnvBinding {
	re := model.EnvBinding{
		AppPrimaryKey: app.Name,
		Name:          envBind.Name,
		AppDeployName: app.Name,
	}
	return &re
}

// CreateWorkflowStepModel assemble the WorkflowStep model from DTO
func CreateWorkflowStepModel(apiSteps []apisv1.WorkflowStep) ([]model.WorkflowStep, error) {
	var steps []model.WorkflowStep
	for _, step := range apiSteps {
		base, err := CreateWorkflowStepBaseModel(step.WorkflowStepBase)
		if err != nil {
			return nil, err
		}
		stepModel := model.WorkflowStep{
			WorkflowStepBase: *base,
			SubSteps:         make([]model.WorkflowStepBase, 0),
		}
		for _, sub := range step.SubSteps {
			base, err := CreateWorkflowStepBaseModel(sub)
			if err != nil {
				return nil, err
			}
			stepModel.SubSteps = append(stepModel.SubSteps, *base)
		}
		steps = append(steps, stepModel)
	}
	return steps, nil
}

// CreateWorkflowStepBaseModel convert api to model
func CreateWorkflowStepBaseModel(step apisv1.WorkflowStepBase) (*model.WorkflowStepBase, error) {
	properties := model.JSONStruct(step.Properties)
	return &model.WorkflowStepBase{
		Name:        step.Name,
		Type:        step.Type,
		Alias:       step.Alias,
		Description: step.Description,
		Properties:  &properties,
		Inputs:      step.Inputs,
		Outputs:     step.Outputs,
		DependsOn:   step.DependsOn,
		Meta:        step.Meta,
		If:          step.If,
		Timeout:     step.Timeout,
	}, nil
}
