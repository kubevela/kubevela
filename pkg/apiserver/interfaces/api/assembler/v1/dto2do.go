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
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
)

// CreateEnvBindingModel assemble the EnvBinding model from DTO
func CreateEnvBindingModel(app *model.Application, req apisv1.CreateApplicationEnvbindingRequest) model.EnvBinding {
	envBinding := model.EnvBinding{
		AppPrimaryKey: app.Name,
		Name:          req.Name,
		AppDeployName: app.GetAppNameForSynced(),
	}
	return envBinding
}

// ConvertToEnvBindingModel assemble the EnvBinding model from DTO
func ConvertToEnvBindingModel(app *model.Application, envBind apisv1.EnvBinding) *model.EnvBinding {
	re := model.EnvBinding{
		AppPrimaryKey: app.Name,
		Name:          envBind.Name,
		AppDeployName: app.GetAppNameForSynced(),
	}
	return &re
}

// CreateWorkflowStepModel assemble the WorkflowStep model from DTO
func CreateWorkflowStepModel(apiSteps []apisv1.WorkflowStep) ([]model.WorkflowStep, error) {
	var steps []model.WorkflowStep
	for _, step := range apiSteps {
		properties, err := model.NewJSONStructByString(step.Properties)
		if err != nil {
			log.Logger.Errorf("parse trait properties failure %w", err)
			return nil, bcode.ErrInvalidProperties
		}
		steps = append(steps, model.WorkflowStep{
			Name:        step.Name,
			Alias:       step.Alias,
			Description: step.Description,
			DependsOn:   step.DependsOn,
			Type:        step.Type,
			Inputs:      step.Inputs,
			Outputs:     step.Outputs,
			Properties:  properties,
		})
	}
	return steps, nil
}
