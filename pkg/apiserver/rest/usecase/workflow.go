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
	"context"
	"errors"

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

// WorkflowUsecase workflow manage api
type WorkflowUsecase interface {
	DeleteWorkflow(ctx context.Context, workflowName string) error
	CreateOrUpdateWorkflow(ctx context.Context, req apisv1.UpdateWorkflowRequest) (*apisv1.DetailWorkflowResponse, error)
}

// NewWorkflowUsecase new workflow usecase
func NewWorkflowUsecase(ds datastore.DataStore) WorkflowUsecase {
	return &workflowUsecaseImpl{ds: ds}
}

type workflowUsecaseImpl struct {
	ds datastore.DataStore
}

// DeleteWorkflow delete application workflow
func (w *workflowUsecaseImpl) DeleteWorkflow(ctx context.Context, workflowName string) error {
	var workflow = &model.Workflow{
		Name: workflowName,
	}
	if err := w.ds.Delete(ctx, workflow); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return bcode.ErrWorkflowNotExist
		}
		return err
	}
	return nil
}

func (w *workflowUsecaseImpl) CreateOrUpdateWorkflow(ctx context.Context, req apisv1.UpdateWorkflowRequest) (*apisv1.DetailWorkflowResponse, error) {
	var steps []model.WorkflowStep
	for _, step := range req.Steps {
		properties, err := model.NewJSONStructByString(step.Properties)
		if err != nil {
			log.Logger.Errorf("parse trait properties failire %w", err)
			return nil, bcode.ErrInvalidProperties
		}
		steps = append(steps, model.WorkflowStep{
			Name:       step.Name,
			Type:       step.Type,
			DependsOn:  step.DependsOn,
			Inputs:     step.Inputs,
			Outputs:    step.Outputs,
			Properties: properties,
		})
	}
	var workflow = model.Workflow{
		Steps:     steps,
		Name:      req.Name,
		Namespace: req.Namespace,
		Enable:    req.Enable,
	}
	if err := w.ds.Add(ctx, &workflow); err != nil {
		return nil, err
	}
	return w.DetailWorkflow(ctx, &workflow)
}

// DetailWorkflow detail workflow
func (w *workflowUsecaseImpl) DetailWorkflow(ctx context.Context, workflow *model.Workflow) (*apisv1.DetailWorkflowResponse, error) {
	var steps []apisv1.WorkflowStep
	for _, step := range workflow.Steps {
		apiStep := apisv1.WorkflowStep{
			Name:       step.Name,
			Type:       step.Type,
			DependsOn:  step.DependsOn,
			Inputs:     step.Inputs,
			Outputs:    step.Outputs,
			Properties: step.Properties.JSON(),
		}
		if step.Properties != nil {
			apiStep.Properties = step.Properties.JSON()
		}
		steps = append(steps, apiStep)
	}
	return &apisv1.DetailWorkflowResponse{Steps: steps, Enable: workflow.Enable}, nil
}
