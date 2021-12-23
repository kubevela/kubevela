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

package usecase

import (
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

func convertPolicyModelToBase(policy *model.ApplicationPolicy) *apisv1.PolicyBase {
	pb := &apisv1.PolicyBase{
		Name:        policy.Name,
		Type:        policy.Type,
		Properties:  policy.Properties,
		Description: policy.Description,
		Creator:     policy.Creator,
		CreateTime:  policy.CreateTime,
		UpdateTime:  policy.UpdateTime,
	}
	return pb
}

func convertWorkflowBase(workflow *model.Workflow) apisv1.WorkflowBase {
	var steps []apisv1.WorkflowStep
	for _, step := range workflow.Steps {
		steps = append(steps, convertFromWorkflowStepModel(step))
	}
	return apisv1.WorkflowBase{
		Name:        workflow.Name,
		Alias:       workflow.Alias,
		Description: workflow.Description,
		Default:     convertBool(workflow.Default),
		EnvName:     workflow.EnvName,
		CreateTime:  workflow.CreateTime,
		UpdateTime:  workflow.UpdateTime,
		Steps:       steps,
	}
}

// convertAPIStep2ModelStep will convert api types of workflow step to model type
func convertAPIStep2ModelStep(apiSteps []apisv1.WorkflowStep) ([]model.WorkflowStep, error) {
	var steps []model.WorkflowStep
	for _, step := range apiSteps {
		properties, err := model.NewJSONStructByString(step.Properties)
		if err != nil {
			log.Logger.Errorf("parse trait properties failire %w", err)
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
