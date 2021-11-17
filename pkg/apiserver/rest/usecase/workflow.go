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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/workflow/recorder"
)

const (
	labelControllerRevisionSync = "apiserver.oam.dev/cr-sync"
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
	SyncWorkflowRecord(ctx context.Context) error
}

// NewWorkflowUsecase new workflow usecase
func NewWorkflowUsecase(ds datastore.DataStore) WorkflowUsecase {
	kubecli, err := clients.GetKubeClient()
	if err != nil {
		log.Logger.Fatalf("get kubeclient failure %s", err.Error())
	}
	return &workflowUsecaseImpl{
		ds:         ds,
		kubeClient: kubecli,
	}
}

type workflowUsecaseImpl struct {
	ds         datastore.DataStore
	kubeClient client.Client
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
		Default:       true,
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
	var revision = model.ApplicationRevision{
		AppPrimaryKey: record.AppPrimaryKey,
		Version:       version,
	}
	err = w.ds.Get(ctx, &revision)
	if err != nil {
		return nil, err
	}

	return &apisv1.DetailWorkflowRecordResponse{
		WorkflowRecord: *convertFromRecordModel(&record),
		DeployTime:     revision.CreateTime,
		DeployUser:     revision.DeployUser,
		Note:           revision.Note,
		TriggerType:    revision.TriggerType,
	}, nil
}

func (w *workflowUsecaseImpl) SyncWorkflowRecord(ctx context.Context) error {
	crList := &appsv1.ControllerRevisionList{}
	matchLabels := metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      labelControllerRevisionSync,
				Operator: metav1.LabelSelectorOpDoesNotExist,
			},
			{
				Key:      recorder.LabelRecordVersion,
				Operator: metav1.LabelSelectorOpExists,
			},
		},
	}
	selector, err := metav1.LabelSelectorAsSelector(&matchLabels)
	if err != nil {
		return err
	}
	if err := w.kubeClient.List(ctx, crList, &client.ListOptions{
		LabelSelector: selector,
	}); err != nil {
		return err
	}

	for i, cr := range crList.Items {
		app, err := util.RawExtension2Application(cr.Data)
		if err != nil {
			klog.ErrorS(err, "failed to get app data", "controller revision name", cr.Name)
			continue
		}

		if app.Annotations == nil {
			klog.ErrorS(err, "empty application annotation", "controller revision name", cr.Name)
			continue
		}

		if _, ok := app.Annotations[oam.AnnotationWorkflowName]; !ok {
			klog.ErrorS(err, "missing application workflow name", "controller revision name", cr.Name)
			continue
		}
		revisionName, ok := app.Annotations[oam.AnnotationDeployVersion]
		if !ok {
			klog.ErrorS(err, "failed to get application revision name", "controller revision name", cr.Name)
			continue
		}

		if err := w.createWorkflowRecord(ctx, app, strings.TrimPrefix(cr.Name, "record-")); err != nil && !errors.Is(err, datastore.ErrRecordExist) {
			klog.ErrorS(err, "failed to create workflow record", "controller revision name", cr.Name)
			continue
		}

		err = w.updateRecordApplicationRevisionStatus(ctx, app.Name, revisionName, app.Status.Workflow.Terminated)
		if err != nil && !errors.Is(err, datastore.ErrRecordNotExist) {
			klog.ErrorS(err, "failed to update deploy event status", "controller revision name", cr.Name)
			continue
		}

		crList.Items[i].Labels[labelControllerRevisionSync] = "true"
		if err := w.kubeClient.Update(ctx, &crList.Items[i]); err != nil {
			klog.ErrorS(err, "failed to update annotation", "controller revision name", cr.Name)
			continue
		}
	}

	return nil
}

func (w *workflowUsecaseImpl) updateRecordApplicationRevisionStatus(ctx context.Context, appPrimaryKey, version string, terminated bool) error {
	var applicationRevision = &model.ApplicationRevision{
		AppPrimaryKey: appPrimaryKey,
		Version:       version,
	}
	if err := w.ds.Get(ctx, applicationRevision); err != nil {
		return err
	}
	if terminated {
		applicationRevision.Status = model.RevisionStatusTerminated
	} else {
		applicationRevision.Status = model.RevisionStatusComplete
	}

	if err := w.ds.Put(ctx, applicationRevision); err != nil {
		return err
	}

	return nil
}

func (w *workflowUsecaseImpl) createWorkflowRecord(ctx context.Context, app *v1beta1.Application, revisionName string) error {
	status := app.Status.Workflow

	return w.ds.Add(ctx, &model.WorkflowRecord{
		WorkflowPrimaryKey: app.Annotations[oam.AnnotationWorkflowName],
		AppPrimaryKey:      app.Name,
		Name:               strings.TrimPrefix(revisionName, "record-"),
		Namespace:          app.Namespace,
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
