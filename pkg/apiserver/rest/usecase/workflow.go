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

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
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
	utils2 "github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// WorkflowUsecase workflow manage api
type WorkflowUsecase interface {
	ListApplicationWorkflow(ctx context.Context, app *model.Application) ([]*apisv1.WorkflowBase, error)
	GetWorkflow(ctx context.Context, app *model.Application, workflowName string) (*model.Workflow, error)
	DetailWorkflow(ctx context.Context, workflow *model.Workflow) (*apisv1.DetailWorkflowResponse, error)
	GetApplicationDefaultWorkflow(ctx context.Context, app *model.Application) (*model.Workflow, error)
	DeleteWorkflow(ctx context.Context, app *model.Application, workflowName string) error
	DeleteWorkflowByApp(ctx context.Context, app *model.Application) error
	CreateOrUpdateWorkflow(ctx context.Context, app *model.Application, req apisv1.CreateWorkflowRequest) (*apisv1.DetailWorkflowResponse, error)
	UpdateWorkflow(ctx context.Context, workflow *model.Workflow, req apisv1.UpdateWorkflowRequest) (*apisv1.DetailWorkflowResponse, error)
	CreateWorkflowRecord(ctx context.Context, appModel *model.Application, app *v1beta1.Application, workflow *model.Workflow) error
	ListWorkflowRecords(ctx context.Context, workflow *model.Workflow, page, pageSize int) (*apisv1.ListWorkflowRecordsResponse, error)
	DetailWorkflowRecord(ctx context.Context, workflow *model.Workflow, recordName string) (*apisv1.DetailWorkflowRecordResponse, error)
	SyncWorkflowRecord(ctx context.Context) error
	ResumeRecord(ctx context.Context, appModel *model.Application, workflow *model.Workflow, recordName string) error
	TerminateRecord(ctx context.Context, appModel *model.Application, workflow *model.Workflow, recordName string) error
	RollbackRecord(ctx context.Context, appModel *model.Application, workflow *model.Workflow, recordName, revisionName string) error
	CountWorkflow(ctx context.Context, app *model.Application) int64
}

// NewWorkflowUsecase new workflow usecase
func NewWorkflowUsecase(ds datastore.DataStore, envUsecase EnvUsecase) WorkflowUsecase {
	kubecli, err := clients.GetKubeClient()
	if err != nil {
		log.Logger.Fatalf("get kubeclient failure %s", err.Error())
	}
	return &workflowUsecaseImpl{
		ds:         ds,
		kubeClient: kubecli,
		apply:      apply.NewAPIApplicator(kubecli),
		envUsecase: envUsecase,
	}
}

type workflowUsecaseImpl struct {
	ds         datastore.DataStore
	kubeClient client.Client
	apply      apply.Applicator
	envUsecase EnvUsecase
}

// DeleteWorkflow delete application workflow
func (w *workflowUsecaseImpl) DeleteWorkflow(ctx context.Context, app *model.Application, workflowName string) error {
	var workflow = &model.Workflow{
		Name:          workflowName,
		AppPrimaryKey: app.PrimaryKey(),
	}
	var record = model.WorkflowRecord{
		AppPrimaryKey: workflow.AppPrimaryKey,
		WorkflowName:  workflow.Name,
	}
	records, err := w.ds.List(ctx, &record, &datastore.ListOptions{})
	if err != nil {
		log.Logger.Errorf("list workflow %s record failure %s", utils2.Sanitize(workflow.PrimaryKey()), err.Error())
	}
	for _, record := range records {
		if err := w.ds.Delete(ctx, record); err != nil {
			log.Logger.Errorf("delete workflow record %s failure %s", record.PrimaryKey(), err.Error())
		}
	}
	if err := w.ds.Delete(ctx, workflow); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return bcode.ErrWorkflowNotExist
		}
		return err
	}
	return nil
}

func (w *workflowUsecaseImpl) DeleteWorkflowByApp(ctx context.Context, app *model.Application) error {
	var workflow = &model.Workflow{
		AppPrimaryKey: app.PrimaryKey(),
	}

	workflows, err := w.ds.List(ctx, workflow, &datastore.ListOptions{})
	if err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil
		}
		return err
	}
	for i := range workflows {
		workflow := workflows[i].(*model.Workflow)
		var record = model.WorkflowRecord{
			AppPrimaryKey: workflow.AppPrimaryKey,
			WorkflowName:  workflow.Name,
		}
		records, err := w.ds.List(ctx, &record, &datastore.ListOptions{})
		if err != nil {
			log.Logger.Errorf("list workflow %s record failure %s", workflow.PrimaryKey(), err.Error())
		}
		for _, record := range records {
			if err := w.ds.Delete(ctx, record); err != nil {
				log.Logger.Errorf("delete workflow record %s failure %s", record.PrimaryKey(), err.Error())
			}
		}
		if err := w.ds.Delete(ctx, workflow); err != nil {
			log.Logger.Errorf("delete workflow %s failure %s", workflow.PrimaryKey(), err.Error())
		}
	}
	return nil
}

func (w *workflowUsecaseImpl) CreateOrUpdateWorkflow(ctx context.Context, app *model.Application, req apisv1.CreateWorkflowRequest) (*apisv1.DetailWorkflowResponse, error) {
	if req.EnvName == "" {
		return nil, bcode.ErrWorkflowNoEnv
	}
	workflow, err := w.GetWorkflow(ctx, app, req.Name)
	if err != nil && errors.Is(err, datastore.ErrRecordNotExist) {
		return nil, err
	}
	var steps []model.WorkflowStep
	for _, step := range req.Steps {
		properties, err := model.NewJSONStructByString(step.Properties)
		if err != nil {
			log.Logger.Errorf("parse trait properties failire %w", err)
			return nil, bcode.ErrInvalidProperties
		}
		steps = append(steps, model.WorkflowStep{
			Name:        step.Name,
			Type:        step.Type,
			Alias:       step.Alias,
			Inputs:      step.Inputs,
			Outputs:     step.Outputs,
			Description: step.Description,
			DependsOn:   step.DependsOn,
			Properties:  properties,
		})
	}
	if workflow != nil {
		workflow.Steps = steps
		workflow.Alias = req.Alias
		workflow.Description = req.Description
		workflow.Default = req.Default
		if err := w.ds.Put(ctx, workflow); err != nil {
			return nil, err
		}
	} else {
		// It is allowed to set multiple workflows as default, and only one takes effect.
		workflow = &model.Workflow{
			Steps:         steps,
			Name:          req.Name,
			Alias:         req.Alias,
			Description:   req.Description,
			Default:       req.Default,
			EnvName:       req.EnvName,
			AppPrimaryKey: app.PrimaryKey(),
		}
		log.Logger.Infof("create workflow %s for app %s", utils2.Sanitize(req.Name), utils2.Sanitize(app.PrimaryKey()))
		if err := w.ds.Add(ctx, workflow); err != nil {
			return nil, err
		}
	}
	return w.DetailWorkflow(ctx, workflow)
}

// updateWorkflowSteps will update workflow with new steps
func updateWorkflowSteps(ctx context.Context, ds datastore.DataStore, workflow *model.Workflow, steps []model.WorkflowStep) error {
	workflow.Steps = steps
	return ds.Put(ctx, workflow)
}

func (w *workflowUsecaseImpl) UpdateWorkflow(ctx context.Context, workflow *model.Workflow, req apisv1.UpdateWorkflowRequest) (*apisv1.DetailWorkflowResponse, error) {
	modeSteps, err := convertAPIStep2ModelStep(req.Steps)
	if err != nil {
		return nil, err
	}
	workflow.Description = req.Description
	// It is allowed to set multiple workflows as default, and only one takes effect.
	if req.Default != nil {
		workflow.Default = req.Default
	}
	if err := updateWorkflowSteps(ctx, w.ds, workflow, modeSteps); err != nil {
		return nil, err
	}
	return w.DetailWorkflow(ctx, workflow)
}

// DetailWorkflow detail workflow
func (w *workflowUsecaseImpl) DetailWorkflow(ctx context.Context, workflow *model.Workflow) (*apisv1.DetailWorkflowResponse, error) {
	return &apisv1.DetailWorkflowResponse{
		WorkflowBase: convertWorkflowBase(workflow),
	}, nil
}

// GetWorkflow get workflow model
func (w *workflowUsecaseImpl) GetWorkflow(ctx context.Context, app *model.Application, workflowName string) (*model.Workflow, error) {
	return getWorkflowForApp(ctx, w.ds, app, workflowName)
}

// ListApplicationWorkflow list application workflows
func (w *workflowUsecaseImpl) ListApplicationWorkflow(ctx context.Context, app *model.Application) ([]*apisv1.WorkflowBase, error) {
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
		base := convertWorkflowBase(wm)
		list = append(list, &base)
	}
	return list, nil
}

// GetApplicationDefaultWorkflow get application default workflow
func (w *workflowUsecaseImpl) GetApplicationDefaultWorkflow(ctx context.Context, app *model.Application) (*model.Workflow, error) {
	var defaultEnable = true
	var workflow = model.Workflow{
		AppPrimaryKey: app.PrimaryKey(),
		Default:       &defaultEnable,
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
func (w *workflowUsecaseImpl) ListWorkflowRecords(ctx context.Context, workflow *model.Workflow, page, pageSize int) (*apisv1.ListWorkflowRecordsResponse, error) {
	var record = model.WorkflowRecord{
		AppPrimaryKey: workflow.AppPrimaryKey,
		WorkflowName:  workflow.Name,
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
func (w *workflowUsecaseImpl) DetailWorkflowRecord(ctx context.Context, workflow *model.Workflow, recordName string) (*apisv1.DetailWorkflowRecordResponse, error) {
	var record = model.WorkflowRecord{
		AppPrimaryKey: workflow.AppPrimaryKey,
		WorkflowName:  workflow.Name,
		Name:          recordName,
	}
	err := w.ds.Get(ctx, &record)
	if err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, bcode.ErrWorkflowRecordNotExist
		}
		return nil, err
	}

	var revision = model.ApplicationRevision{
		AppPrimaryKey: record.AppPrimaryKey,
		Version:       record.RevisionPrimaryKey,
	}
	err = w.ds.Get(ctx, &revision)
	if err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, bcode.ErrApplicationRevisionNotExist
		}
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
		record := item.(*model.WorkflowRecord)
		workflow := &model.Workflow{
			Name:          record.WorkflowName,
			AppPrimaryKey: record.AppPrimaryKey,
		}
		if err := w.ds.Get(ctx, workflow); err != nil {
			klog.ErrorS(err, "failed to get workflow", "app name", record.AppPrimaryKey, "workflow name", record.WorkflowName, "record name", record.Name)
			continue
		}
		appName := record.AppPrimaryKey
		if err := w.kubeClient.Get(ctx, types.NamespacedName{
			Name:      appName,
			Namespace: record.Namespace,
		}, app); err != nil {
			klog.ErrorS(err, "failed to get app", "oam app name", appName, "workflow name", record.WorkflowName, "record name", record.Name)
			continue
		}

		// try to sync the status from the running application
		if app.Annotations != nil && app.Status.Workflow != nil && app.Status.Workflow.AppRevision == record.Name {
			if err := w.syncWorkflowStatus(ctx, app, record.Name, app.Name); err != nil {
				klog.ErrorS(err, "failed to sync workflow status", "oam app name", appName, "workflow name", record.WorkflowName, "record name", record.Name)
			}
			continue
		}

		// try to sync the status from the controller revision
		cr := &appsv1.ControllerRevision{}
		if err := w.kubeClient.Get(ctx, types.NamespacedName{
			Name:      fmt.Sprintf("record-%s-%s", appName, record.Name),
			Namespace: record.Namespace,
		}, cr); err != nil {
			klog.ErrorS(err, "failed to get controller revision", "oam app name", appName, "workflow name", record.WorkflowName, "record name", record.Name)
			continue
		}
		appInRevision, err := util.RawExtension2Application(cr.Data)
		if err != nil {
			klog.ErrorS(err, "failed to get app data in controller revision", "controller revision name", cr.Name, "app name", appName, "workflow name", record.WorkflowName, "record name", record.Name)
			continue
		}
		if err := w.syncWorkflowStatus(ctx, appInRevision, record.Name, cr.Name); err != nil {
			klog.ErrorS(err, "failed to sync workflow status", "oam app name", appName, "workflow name", record.WorkflowName, "record name", record.Name)
			continue
		}

	}

	return nil
}

func (w *workflowUsecaseImpl) syncWorkflowStatus(ctx context.Context, app *v1beta1.Application, recordName, source string) error {
	var record = &model.WorkflowRecord{
		AppPrimaryKey: app.Annotations[oam.AnnotationAppName],
		Name:          recordName,
	}
	if err := w.ds.Get(ctx, record); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return bcode.ErrWorkflowRecordNotExist
		}
		return err
	}
	var revision = &model.ApplicationRevision{
		AppPrimaryKey: app.Annotations[oam.AnnotationAppName],
		Version:       record.RevisionPrimaryKey,
	}

	if err := w.ds.Get(ctx, revision); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return bcode.ErrApplicationRevisionNotExist
		}
		return err
	}

	if app.Status.Workflow != nil {
		status := app.Status.Workflow
		summaryStatus := model.RevisionStatusRunning
		if status.Finished {
			summaryStatus = model.RevisionStatusComplete
		}
		if status.Terminated {
			summaryStatus = model.RevisionStatusTerminated
		}

		record.Status = summaryStatus
		stepStatus := make(map[string]*common.WorkflowStepStatus, len(status.Steps))
		for i, step := range status.Steps {
			stepStatus[step.Name] = &status.Steps[i]
		}
		for i, step := range record.Steps {
			if stepStatus[step.Name] != nil {
				record.Steps[i].Phase = stepStatus[step.Name].Phase
				record.Steps[i].Message = stepStatus[step.Name].Message
				record.Steps[i].Reason = stepStatus[step.Name].Reason
				record.Steps[i].FirstExecuteTime = stepStatus[step.Name].FirstExecuteTime.Time
				record.Steps[i].LastExecuteTime = stepStatus[step.Name].LastExecuteTime.Time
			}
		}
		record.Finished = strconv.FormatBool(status.Finished)

		if err := w.ds.Put(ctx, record); err != nil {
			return err
		}

		revision.Status = summaryStatus
		if err := w.ds.Put(ctx, revision); err != nil {
			return err
		}
	}

	if record.Finished == "true" {
		klog.InfoS("successfully sync workflow status", "oam app name", app.Name, "workflow name", record.WorkflowName, "record name", record.Name, "status", record.Status, "sync source", source)
	}

	return nil
}

func (w *workflowUsecaseImpl) CreateWorkflowRecord(ctx context.Context, appModel *model.Application, app *v1beta1.Application, workflow *model.Workflow) error {
	if app.Annotations == nil {
		return fmt.Errorf("empty annotations in application")
	}
	if app.Annotations[oam.AnnotationPublishVersion] == "" {
		return fmt.Errorf("failed to get record version from application")
	}
	if app.Annotations[oam.AnnotationDeployVersion] == "" {
		return fmt.Errorf("failed to get deploy version from application")
	}
	steps := make([]model.WorkflowStepStatus, len(workflow.Steps))
	for i, step := range workflow.Steps {
		steps[i] = model.WorkflowStepStatus{
			Name:  step.Name,
			Alias: step.Alias,
			Type:  step.Type,
		}
	}

	if err := w.ds.Add(ctx, &model.WorkflowRecord{
		WorkflowName:       workflow.Name,
		WorkflowAlias:      workflow.Alias,
		AppPrimaryKey:      appModel.PrimaryKey(),
		RevisionPrimaryKey: app.Annotations[oam.AnnotationDeployVersion],
		Name:               app.Annotations[oam.AnnotationPublishVersion],
		Namespace:          app.Namespace,
		Finished:           "false",
		StartTime:          time.Now().Time,
		Steps:              steps,
		Status:             model.RevisionStatusRunning,
	}); err != nil {
		return err
	}

	if err := resetRevisionsAndRecords(ctx, w.ds, appModel.PrimaryKey(), workflow.Name, app.Annotations[oam.AnnotationDeployVersion], app.Annotations[oam.AnnotationPublishVersion]); err != nil {
		return err
	}

	return nil
}

func resetRevisionsAndRecords(ctx context.Context, ds datastore.DataStore, appName, workflowName, skipRevision, skipRecord string) error {
	// set revision status' status to terminate
	var revision = model.ApplicationRevision{
		AppPrimaryKey: appName,
		Status:        model.RevisionStatusRunning,
	}
	// list all running revisions
	revisions, err := ds.List(ctx, &revision, &datastore.ListOptions{})
	if err != nil {
		return err
	}
	for _, raw := range revisions {
		revision, ok := raw.(*model.ApplicationRevision)
		if ok {
			if revision.Version == skipRevision {
				continue
			}
			revision.Status = model.RevisionStatusTerminated
			if err := ds.Put(ctx, revision); err != nil {
				klog.Info("failed to set rest revisions' status to terminate", "app name", appName, "revision version", revision.Version, "error", err)
			}
		}
	}

	// set rest records' status to terminate
	var record = model.WorkflowRecord{
		WorkflowName:  workflowName,
		AppPrimaryKey: appName,
		Finished:      "false",
	}
	// list all unfinished workflow records
	records, err := ds.List(ctx, &record, &datastore.ListOptions{})
	if err != nil {
		return err
	}
	for _, raw := range records {
		record, ok := raw.(*model.WorkflowRecord)
		if ok {
			if record.Name == skipRecord {
				continue
			}
			record.Status = model.RevisionStatusTerminated
			record.Finished = "true"
			if err := ds.Put(ctx, record); err != nil {
				klog.Info("failed to set rest records' status to terminate", "app name", appName, "workflow name", record.WorkflowName, "record name", record.Name, "error", err)
			}
		}
	}

	return nil
}

func (w *workflowUsecaseImpl) CountWorkflow(ctx context.Context, app *model.Application) int64 {
	count, err := w.ds.Count(ctx, &model.Workflow{AppPrimaryKey: app.PrimaryKey()}, &datastore.FilterOptions{})
	if err != nil {
		log.Logger.Errorf("count app %s workflow failure %s", app.PrimaryKey(), err.Error())
	}
	return count
}

func (w *workflowUsecaseImpl) ResumeRecord(ctx context.Context, appModel *model.Application, workflow *model.Workflow, recordName string) error {
	oamApp, err := w.checkRecordRunning(ctx, appModel, workflow.EnvName)
	if err != nil {
		return err
	}

	oamApp.Status.Workflow.Suspend = false
	if err := w.kubeClient.Status().Patch(ctx, oamApp, client.Merge); err != nil {
		return err
	}
	if err := w.syncWorkflowStatus(ctx, oamApp, recordName, oamApp.Name); err != nil {
		return err
	}

	return nil
}

func (w *workflowUsecaseImpl) TerminateRecord(ctx context.Context, appModel *model.Application, workflow *model.Workflow, recordName string) error {
	oamApp, err := w.checkRecordRunning(ctx, appModel, workflow.EnvName)
	if err != nil {
		return err
	}

	oamApp.Status.Workflow.Terminated = true
	if err := w.kubeClient.Status().Patch(ctx, oamApp, client.Merge); err != nil {
		return err
	}
	if err := w.syncWorkflowStatus(ctx, oamApp, recordName, oamApp.Name); err != nil {
		return err
	}

	return nil
}

func (w *workflowUsecaseImpl) RollbackRecord(ctx context.Context, appModel *model.Application, workflow *model.Workflow, recordName, revisionVersion string) error {
	if revisionVersion == "" {
		// find the latest complete revision version
		var revision = model.ApplicationRevision{
			AppPrimaryKey: appModel.Name,
			Status:        model.RevisionStatusComplete,
			WorkflowName:  workflow.Name,
			EnvName:       workflow.EnvName,
		}
		revisions, err := w.ds.List(ctx, &revision, &datastore.ListOptions{
			Page:     1,
			PageSize: 1,
			SortBy:   []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderDescending}},
		})
		if err != nil {
			return err
		}
		if len(revisions) == 0 {
			return bcode.ErrApplicationNoReadyRevision
		}
		revisionVersion = revisions[0].Index()["version"]
		log.Logger.Infof("select lastest complete revision %s", revisions[0].Index()["version"])
	}

	var record = &model.WorkflowRecord{
		AppPrimaryKey: appModel.PrimaryKey(),
		Name:          recordName,
	}
	if err := w.ds.Get(ctx, record); err != nil {
		return err
	}

	oamApp, err := w.checkRecordRunning(ctx, appModel, workflow.EnvName)
	if err != nil {
		return err
	}

	var originalRevision = &model.ApplicationRevision{
		AppPrimaryKey: appModel.Name,
		Version:       record.RevisionPrimaryKey,
	}
	if err := w.ds.Get(ctx, originalRevision); err != nil {
		return err
	}

	var rollbackRevision = &model.ApplicationRevision{
		AppPrimaryKey: appModel.Name,
		Version:       revisionVersion,
	}
	if err := w.ds.Get(ctx, rollbackRevision); err != nil {
		return err
	}

	// update the original revision status to rollback
	originalRevision.Status = model.RevisionStatusRollback
	originalRevision.RollbackVersion = revisionVersion
	originalRevision.UpdateTime = time.Now().Time
	if err := w.ds.Put(ctx, originalRevision); err != nil {
		return err
	}

	rollBackApp := &v1beta1.Application{}
	if err := yaml.Unmarshal([]byte(rollbackRevision.ApplyAppConfig), rollBackApp); err != nil {
		return err
	}
	// replace the application spec
	oamApp.Spec.Components = rollBackApp.Spec.Components
	oamApp.Spec.Policies = rollBackApp.Spec.Policies
	if oamApp.Annotations == nil {
		oamApp.Annotations = make(map[string]string)
	}
	newRecordName := utils.GenerateVersion(record.WorkflowName)
	oamApp.Annotations[oam.AnnotationDeployVersion] = revisionVersion
	oamApp.Annotations[oam.AnnotationPublishVersion] = newRecordName
	// create a new workflow record
	if err := w.CreateWorkflowRecord(ctx, appModel, oamApp, workflow); err != nil {
		return err
	}

	if err := w.apply.Apply(ctx, oamApp); err != nil {
		// rollback error case
		if err := w.ds.Delete(ctx, &model.WorkflowRecord{Name: newRecordName}); err != nil {
			klog.Error(err, "failed to delete record", newRecordName)
		}
		return err
	}

	return nil
}

func (w *workflowUsecaseImpl) checkRecordRunning(ctx context.Context, appModel *model.Application, envName string) (*v1beta1.Application, error) {
	oamApp := &v1beta1.Application{}
	env, err := w.envUsecase.GetEnv(ctx, envName)
	if err != nil {
		return nil, err
	}
	if err := w.kubeClient.Get(ctx, types.NamespacedName{Name: appModel.Name, Namespace: env.Namespace}, oamApp); err != nil {
		return nil, err
	}
	if oamApp.Status.Workflow != nil && !oamApp.Status.Workflow.Suspend && !oamApp.Status.Workflow.Terminated && !oamApp.Status.Workflow.Finished {
		return nil, fmt.Errorf("workflow is still running, can not operate a running workflow")
	}

	oamApp.SetGroupVersionKind(v1beta1.ApplicationKindVersionKind)
	return oamApp, nil
}

func convertFromRecordModel(record *model.WorkflowRecord) *apisv1.WorkflowRecord {
	return &apisv1.WorkflowRecord{
		Name:                record.Name,
		Namespace:           record.Namespace,
		WorkflowName:        record.WorkflowName,
		WorkflowAlias:       record.WorkflowAlias,
		ApplicationRevision: record.RevisionPrimaryKey,
		StartTime:           record.StartTime,
		Status:              record.Status,
		Steps:               record.Steps,
	}
}

func convertFromWorkflowStepModel(step model.WorkflowStep) apisv1.WorkflowStep {
	apiStep := apisv1.WorkflowStep{
		Name:        step.Name,
		Type:        step.Type,
		Alias:       step.Alias,
		Description: step.Description,
		Inputs:      step.Inputs,
		Outputs:     step.Outputs,
		Properties:  step.Properties.JSON(),
		DependsOn:   step.DependsOn,
	}
	if step.Properties != nil {
		apiStep.Properties = step.Properties.JSON()
	}
	return apiStep
}

func convertBool(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}
