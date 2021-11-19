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
	"strconv"

	"helm.sh/helm/v3/pkg/time"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
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
	CreateWorkflowRecord(ctx context.Context, app *v1beta1.Application) error
	UpdateWorkflow(ctx context.Context, workflow *model.Workflow, req apisv1.UpdateWorkflowRequest) (*apisv1.DetailWorkflowResponse, error)
	ListWorkflowRecords(ctx context.Context, workflowName string, page, pageSize int) (*apisv1.ListWorkflowRecordsResponse, error)
	DetailWorkflowRecord(ctx context.Context, workflowName, recordName string) (*apisv1.DetailWorkflowRecordResponse, error)
	SyncWorkflowRecord(ctx context.Context) error
	ResumeRecord(ctx context.Context, appModel *model.Application, recordName string) error
	TerminateRecord(ctx context.Context, appModel *model.Application, recordName string) error
	RollbackRecord(ctx context.Context, appModel *model.Application, recordName, revisionName string) error
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
		EnvName:       req.EnvName,
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
	workflow.EnvName = req.EnvName
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
			EnvName:     workflow.EnvName,
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
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, bcode.ErrWorkflowNotExist
		}
		return nil, err
	}
	return &workflow, nil
}

// ListApplicationWorkflow list application workflows
func (w *workflowUsecaseImpl) ListApplicationWorkflow(ctx context.Context, app *model.Application, enable *bool) ([]*apisv1.WorkflowBase, error) {
	var workflow = model.Workflow{
		AppPrimaryKey: app.PrimaryKey(),
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
			EnvName:     wm.EnvName,
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
	count, err := w.ds.Count(ctx, &record, nil)
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

	var revision = model.ApplicationRevision{
		AppPrimaryKey: record.AppPrimaryKey,
		Version:       record.RevisionPrimaryKey,
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
	var record = model.WorkflowRecord{
		Finished: "false",
	}
	// list all unfinished workflow records
	records, err := w.ds.List(ctx, &record, &datastore.ListOptions{})
	if err != nil {
		return err
	}

	for _, item := range records {
		app := &v1beta1.Application{}
		index := item.Index()
		appPrimaryKey := index["appPrimaryKey"]
		namespace := index["namespace"]
		recordName := index["name"]

		if err := w.kubeClient.Get(ctx, types.NamespacedName{
			Name:      appPrimaryKey,
			Namespace: namespace,
		}, app); err != nil {
			klog.ErrorS(err, "failed to get app", "app name", appPrimaryKey)
			return err
		}

		// try to sync the status from the running application
		if app.Annotations != nil && app.Annotations[oam.AnnotationPublishVersion] == recordName {
			if err := w.syncWorkflowStatus(ctx, app, recordName); err != nil {
				klog.ErrorS(err, "failed to sync workflow status", "app name", appPrimaryKey, "workflow record name", recordName)
			}
			continue
		}

		// try to sync the status from the controller revision
		cr := &appsv1.ControllerRevision{}
		if err := w.kubeClient.Get(ctx, types.NamespacedName{
			Name:      fmt.Sprintf("record-%s-%s", appPrimaryKey, recordName),
			Namespace: namespace,
		}, cr); err != nil {
			klog.ErrorS(err, "failed to get controller revision", "app name", appPrimaryKey, "workflow record name", recordName)
			continue
		}
		appInRevision, err := util.RawExtension2Application(cr.Data)
		if err != nil {
			klog.ErrorS(err, "failed to get app data in controller revision", "controller revision name", cr.Name, "app name", appPrimaryKey, "workflow record name", recordName)
			continue
		}
		if err := w.syncWorkflowStatus(ctx, appInRevision, recordName); err != nil {
			klog.ErrorS(err, "failed to sync workflow status", "app name", appPrimaryKey, "workflow record version", recordName)
			continue
		}

	}

	return nil
}

func (w *workflowUsecaseImpl) syncWorkflowStatus(ctx context.Context, app *v1beta1.Application, recordName string) error {
	var record = &model.WorkflowRecord{
		AppPrimaryKey: app.Name,
		Name:          recordName,
	}
	if err := w.ds.Get(ctx, record); err != nil {
		return err
	}
	var revision = &model.ApplicationRevision{
		AppPrimaryKey: app.Name,
		Version:       record.RevisionPrimaryKey,
	}
	if err := w.ds.Get(ctx, revision); err != nil {
		return err
	}

	if app.Status.Workflow != nil {
		status := app.Status.Workflow
		summaryStatus := model.RevisionStatusRunning
		if status.Finished {
			summaryStatus = model.RevisionStatusComplete
		}
		if status.Suspend {
			summaryStatus = model.RevisionStatusSuspend
		}
		if status.Terminated {
			summaryStatus = model.RevisionStatusTerminated
		}

		record.Status = summaryStatus
		record.Steps = status.Steps
		record.Finished = strconv.FormatBool(status.Finished)

		if err := w.ds.Put(ctx, record); err != nil {
			return err
		}

		revision.Status = summaryStatus
		if err := w.ds.Put(ctx, revision); err != nil {
			return err
		}
	}

	return nil
}

func (w *workflowUsecaseImpl) CreateWorkflowRecord(ctx context.Context, app *v1beta1.Application) error {
	if app.Annotations == nil {
		return fmt.Errorf("empty annotations in application")
	}
	if app.Annotations[oam.AnnotationWorkflowName] == "" {
		return fmt.Errorf("failed to get workflow name from application")
	}
	if app.Annotations[oam.AnnotationPublishVersion] == "" {
		return fmt.Errorf("failed to get record version from application")
	}
	if app.Annotations[oam.AnnotationDeployVersion] == "" {
		return fmt.Errorf("failed to get deploy version from application")
	}

	return w.ds.Add(ctx, &model.WorkflowRecord{
		WorkflowPrimaryKey: app.Annotations[oam.AnnotationWorkflowName],
		AppPrimaryKey:      app.Name,
		RevisionPrimaryKey: app.Annotations[oam.AnnotationDeployVersion],
		Name:               app.Annotations[oam.AnnotationPublishVersion],
		Namespace:          app.Namespace,
		Finished:           "false",
		StartTime:          time.Now().Time,
		Status:             model.RevisionStatusInit,
	})
}

func (w *workflowUsecaseImpl) ResumeRecord(ctx context.Context, appModel *model.Application, recordName string) error {
	oamApp, err := w.checkRecordSuspended(ctx, appModel)
	if err != nil {
		return err
	}

	oamApp.Status.Workflow.Suspend = false
	if err := w.kubeClient.Status().Patch(ctx, oamApp, client.Merge); err != nil {
		return err
	}
	if err := w.syncWorkflowStatus(ctx, oamApp, recordName); err != nil {
		return err
	}

	return nil
}

func (w *workflowUsecaseImpl) TerminateRecord(ctx context.Context, appModel *model.Application, recordName string) error {
	oamApp, err := w.checkRecordSuspended(ctx, appModel)
	if err != nil {
		return err
	}

	oamApp.Status.Workflow.Terminated = true
	if err := w.kubeClient.Status().Patch(ctx, oamApp, client.Merge); err != nil {
		return err
	}
	if err := w.syncWorkflowStatus(ctx, oamApp, recordName); err != nil {
		return err
	}

	return nil
}

func (w *workflowUsecaseImpl) RollbackRecord(ctx context.Context, appModel *model.Application, recordName, revisionVersion string) error {
	// TODO: if revisionVersion is empty, rollback to the last revision
	oamApp, err := w.checkRecordSuspended(ctx, appModel)
	if err != nil {
		return err
	}

	var record = &model.WorkflowRecord{
		AppPrimaryKey: appModel.Name,
		Name:          recordName,
	}
	if err := w.ds.Get(ctx, record); err != nil {
		return err
	}
	var rollbackRevision = model.ApplicationRevision{
		AppPrimaryKey: appModel.Name,
		Version:       revisionVersion,
	}
	if err := w.ds.Get(ctx, &rollbackRevision); err != nil {
		return err
	}

	rollBackApp := &v1beta1.Application{}
	if err := yaml.Unmarshal([]byte(rollbackRevision.ApplyAppConfig), rollBackApp); err != nil {
		return err
	}
	// replace the application spec
	oamApp.Spec.Components = rollBackApp.Spec.Components
	if oamApp.Annotations == nil {
		oamApp.Annotations = make(map[string]string)
	}
	newRecordName := utils.GenerateVersion(record.WorkflowPrimaryKey)
	oamApp.Annotations[oam.AnnotationDeployVersion] = revisionVersion
	oamApp.Annotations[oam.AnnotationPublishVersion] = newRecordName

	// create a new workflow record
	if err := w.CreateWorkflowRecord(ctx, oamApp); err != nil {
		return err
	}

	if err := w.kubeClient.Update(ctx, oamApp); err != nil {
		// rollback error case
		if err := w.ds.Delete(ctx, &model.WorkflowRecord{Name: newRecordName}); err != nil {
			klog.Error(err, "failed to delete record", newRecordName)
		}
		return err
	}

	return nil
}

func (w *workflowUsecaseImpl) checkRecordSuspended(ctx context.Context, appModel *model.Application) (*v1beta1.Application, error) {
	oamApp := &v1beta1.Application{}
	if err := w.kubeClient.Get(ctx, types.NamespacedName{Name: appModel.Name, Namespace: appModel.Namespace}, oamApp); err != nil {
		return nil, err
	}
	if oamApp.Status.Workflow != nil && oamApp.Status.Workflow.Suspend != true {
		return nil, bcode.ErrRevisionNotSuspended
	}

	return oamApp, nil
}

func convertFromRecordModel(record *model.WorkflowRecord) *apisv1.WorkflowRecord {
	return &apisv1.WorkflowRecord{
		Name:      record.Name,
		Namespace: record.Namespace,
		StartTime: record.StartTime,
		Status:    record.Status,
		Steps:     record.Steps,
	}
}
