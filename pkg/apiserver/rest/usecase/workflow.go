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
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// WorkflowUsecase workflow manage api
type WorkflowUsecase interface {
	ListApplicationWorkflow(ctx context.Context, app *model.Application, enable *bool) ([]*apisv1.WorkflowBase, error)
	GetWorkflow(ctx context.Context, workflowName string) (*model.Workflow, error)
	DetailWorkflow(ctx context.Context, workflow *model.Workflow) (*apisv1.DetailWorkflowResponse, error)
	GetApplicationDefaultWorkflow(ctx context.Context, app *model.Application) (*model.Workflow, error)
	DeleteWorkflow(ctx context.Context, workflowName string) error
	CreateWorkflow(ctx context.Context, app *model.Application, req apisv1.CreateWorkflowRequest) (*apisv1.DetailWorkflowResponse, error)
	UpdateWorkflow(ctx context.Context, workflow *model.Workflow, req apisv1.UpdateWorkflowRequest) (*apisv1.DetailWorkflowResponse, error)
	ListWorkflowRecords(ctx context.Context, workflowName string, page, pageSize int) (*apisv1.ListWorkflowRecordsResponse, error)
	DetailWorkflowRecord(ctx context.Context, workflowName, recordName string) (*apisv1.DetailWorkflowRecordResponse, error)
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

func (w *workflowUsecaseImpl) CreateWorkflow(ctx context.Context, app *model.Application, req apisv1.CreateWorkflowRequest) (*apisv1.DetailWorkflowResponse, error) {
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
			Inputs:     step.Inputs,
			Outputs:    step.Outputs,
			Properties: properties,
		})
	}
	// It is allowed to set multiple workflows as default, and only one takes effect.
	var workflow = model.Workflow{
		Steps:         steps,
		Name:          req.Name,
		Enable:        req.Enable,
		Description:   req.Description,
		Default:       req.Default,
		AppPrimaryKey: app.PrimaryKey(),
	}
	if err := w.ds.Add(ctx, &workflow); err != nil {
		return nil, err
	}
	return w.DetailWorkflow(ctx, &workflow)
}

func (w *workflowUsecaseImpl) UpdateWorkflow(ctx context.Context, workflow *model.Workflow, req apisv1.UpdateWorkflowRequest) (*apisv1.DetailWorkflowResponse, error) {
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
			Inputs:     step.Inputs,
			Outputs:    step.Outputs,
			Properties: properties,
		})
	}
	workflow.Steps = steps
	workflow.Description = req.Description
	// It is allowed to set multiple workflows as default, and only one takes effect.
	workflow.Default = req.Default
	workflow.Enable = req.Enable
	if err := w.ds.Put(ctx, workflow); err != nil {
		return nil, err
	}
	return w.DetailWorkflow(ctx, workflow)
}

// DetailWorkflow detail workflow
func (w *workflowUsecaseImpl) DetailWorkflow(ctx context.Context, workflow *model.Workflow) (*apisv1.DetailWorkflowResponse, error) {
	var steps []apisv1.WorkflowStep
	for _, step := range workflow.Steps {
		apiStep := apisv1.WorkflowStep{
			Name:       step.Name,
			Type:       step.Type,
			Inputs:     step.Inputs,
			Outputs:    step.Outputs,
			Properties: step.Properties.JSON(),
		}
		if step.Properties != nil {
			apiStep.Properties = step.Properties.JSON()
		}
		steps = append(steps, apiStep)
	}
	return &apisv1.DetailWorkflowResponse{
		WorkflowBase: apisv1.WorkflowBase{
			Name:        workflow.Name,
			Description: workflow.Description,
			Enable:      workflow.Enable,
			Default:     workflow.Default,
			CreateTime:  workflow.CreateTime,
			UpdateTime:  workflow.UpdateTime,
		},
		Steps: steps,
	}, nil
}

// GetWorkflow get workflow model
func (w *workflowUsecaseImpl) GetWorkflow(ctx context.Context, workflowName string) (*model.Workflow, error) {
	var workflow = model.Workflow{
		Name: workflowName,
	}
	if err := w.ds.Get(ctx, &workflow); err != nil {
		return nil, err
	}
	return &workflow, nil
}

// ListApplicationWorkflow list application workflows
func (w *workflowUsecaseImpl) ListApplicationWorkflow(ctx context.Context, app *model.Application, enable *bool) ([]*apisv1.WorkflowBase, error) {
	var workflow = model.Workflow{
		AppPrimaryKey: app.PrimaryKey(),
	}
	if enable != nil {
		workflow.Enable = *enable
	}
	workflows, err := w.ds.List(ctx, &workflow, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	var list []*apisv1.WorkflowBase
	for _, workflow := range workflows {
		wm := workflow.(*model.Workflow)
		list = append(list, &apisv1.WorkflowBase{
			Name:        wm.Name,
			Description: wm.Description,
			Enable:      wm.Enable,
			Default:     wm.Default,
			CreateTime:  wm.CreateTime,
			UpdateTime:  wm.UpdateTime,
		})
	}
	return list, nil
}

// GetApplicationDefaultWorkflow get application default workflow
func (w *workflowUsecaseImpl) GetApplicationDefaultWorkflow(ctx context.Context, app *model.Application) (*model.Workflow, error) {
	var workflow = model.Workflow{
		AppPrimaryKey: app.PrimaryKey(),
		Default:       true,
	}
	workflows, err := w.ds.List(ctx, &workflow, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	if len(workflows) > 0 {
		return workflows[0].(*model.Workflow), nil
	}
	return nil, bcode.ErrWorkflowNoDefault
}

// ListWorkflowRecords list workflow record
func (w *workflowUsecaseImpl) ListWorkflowRecords(ctx context.Context, workflowName string, page, pageSize int) (*apisv1.ListWorkflowRecordsResponse, error) {
	var record = model.WorkflowRecord{
		WorkflowPrimaryKey: workflowName,
	}
	records, err := w.ds.List(ctx, &record, &datastore.ListOptions{Page: page, PageSize: pageSize})
	if err != nil {
		return nil, err
	}

	resp := &apisv1.ListWorkflowRecordsResponse{
		Records: []apisv1.WorkflowRecord{},
	}
	for _, raw := range records {
		record, ok := raw.(*model.WorkflowRecord)
		if ok {
			resp.Records = append(resp.Records, *convertFromRecordModel(record))
		}
	}
	count, err := w.ds.Count(ctx, &record)
	if err != nil {
		return nil, err
	}
	resp.Total = count

	return resp, nil
}

// DetailWorkflowRecord get workflow record detail with name
func (w *workflowUsecaseImpl) DetailWorkflowRecord(ctx context.Context, workflowName, recordName string) (*apisv1.DetailWorkflowRecordResponse, error) {
	var record = model.WorkflowRecord{
		WorkflowPrimaryKey: workflowName,
		Name:               recordName,
	}
	err := w.ds.Get(ctx, &record)
	if err != nil {
		return nil, err
	}

	version := strings.TrimPrefix(recordName, fmt.Sprintf("%s-", record.AppPrimaryKey))
	var deployEvent = model.DeployEvent{
		AppPrimaryKey: record.AppPrimaryKey,
		Version:       version,
	}
	err = w.ds.Get(ctx, &deployEvent)
	if err != nil {
		return nil, err
	}

	return &apisv1.DetailWorkflowRecordResponse{
		WorkflowRecord: *convertFromRecordModel(&record),
		DeployTime:     deployEvent.CreateTime,
		DeployUser:     deployEvent.DeployUser,
		Commit:         deployEvent.Commit,
		SourceType:     deployEvent.SourceType,
	}, nil
}

func (w *workflowUsecaseImpl) createWorkflowRecord(ctx context.Context, revision *appsv1.ControllerRevision) error {
	app, err := util.RawExtension2Application(revision.Data)
	if err != nil {
		return err
	}
	if app.Annotations == nil || app.Annotations[oam.AnnotationWorkflowName] == "" {
		return fmt.Errorf("missing workflow name")
	}
	status := app.Status.Workflow

	return w.ds.Add(ctx, &model.WorkflowRecord{
		WorkflowPrimaryKey: app.Annotations[oam.AnnotationWorkflowName],
		AppPrimaryKey:      app.Name,
		Name:               strings.TrimPrefix(revision.Name, "record-"),
		Namespace:          revision.Namespace,
		StartTime:          status.StartTime.Time,
		Suspend:            status.Suspend,
		Terminated:         status.Terminated,
		Steps:              status.Steps,
	})
}

func convertFromRecordModel(record *model.WorkflowRecord) *apisv1.WorkflowRecord {
	return &apisv1.WorkflowRecord{
		Name:       record.Name,
		Namespace:  record.Namespace,
		StartTime:  record.StartTime,
		Suspend:    record.Suspend,
		Terminated: record.Terminated,
		Steps:      record.Steps,
	}
}
