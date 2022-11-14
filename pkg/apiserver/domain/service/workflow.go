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

package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	"helm.sh/helm/v3/pkg/time"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	wfTypes "github.com/kubevela/workflow/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/repository"
	"github.com/oam-dev/kubevela/pkg/apiserver/event/sync/convert"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	assembler "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/assembler/v1"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
	"github.com/oam-dev/kubevela/pkg/oam"
	pkgUtils "github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// WorkflowService workflow manage api
type WorkflowService interface {
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

// NewWorkflowService new workflow service
func NewWorkflowService() WorkflowService {
	return &workflowServiceImpl{}
}

type workflowServiceImpl struct {
	Store             datastore.DataStore `inject:"datastore"`
	KubeClient        client.Client       `inject:"kubeClient"`
	Apply             apply.Applicator    `inject:"apply"`
	EnvService        EnvService          `inject:""`
	EnvBindingService EnvBindingService   `inject:""`
}

// DeleteWorkflow delete application workflow
func (w *workflowServiceImpl) DeleteWorkflow(ctx context.Context, app *model.Application, workflowName string) error {
	var workflow = &model.Workflow{
		Name:          workflowName,
		AppPrimaryKey: app.PrimaryKey(),
	}
	var record = model.WorkflowRecord{
		AppPrimaryKey: workflow.AppPrimaryKey,
		WorkflowName:  workflow.Name,
	}
	records, err := w.Store.List(ctx, &record, &datastore.ListOptions{})
	if err != nil {
		log.Logger.Errorf("list workflow %s record failure %s", pkgUtils.Sanitize(workflow.PrimaryKey()), err.Error())
	}
	for _, record := range records {
		if err := w.Store.Delete(ctx, record); err != nil {
			log.Logger.Errorf("delete workflow record %s failure %s", record.PrimaryKey(), err.Error())
		}
	}
	if err := w.Store.Delete(ctx, workflow); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return bcode.ErrWorkflowNotExist
		}
		return err
	}
	return nil
}

func (w *workflowServiceImpl) DeleteWorkflowByApp(ctx context.Context, app *model.Application) error {
	var workflow = &model.Workflow{
		AppPrimaryKey: app.PrimaryKey(),
	}

	workflows, err := w.Store.List(ctx, workflow, &datastore.ListOptions{})
	if err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil
		}
		return err
	}
	for i := range workflows {
		workflow := workflows[i].(*model.Workflow)
		if err := w.Store.Delete(ctx, workflow); err != nil {
			log.Logger.Errorf("delete workflow %s failure %s", workflow.PrimaryKey(), err.Error())
		}
	}
	var record = model.WorkflowRecord{
		AppPrimaryKey: workflow.AppPrimaryKey,
	}
	records, err := w.Store.List(ctx, &record, &datastore.ListOptions{})
	if err != nil {
		log.Logger.Errorf("list workflow %s record failure %s", workflow.PrimaryKey(), err.Error())
	}
	for _, record := range records {
		if err := w.Store.Delete(ctx, record); err != nil {
			log.Logger.Errorf("delete workflow record %s failure %s", record.PrimaryKey(), err.Error())
		}
	}
	return nil
}

func (w *workflowServiceImpl) CreateOrUpdateWorkflow(ctx context.Context, app *model.Application, req apisv1.CreateWorkflowRequest) (*apisv1.DetailWorkflowResponse, error) {
	if req.EnvName == "" {
		return nil, bcode.ErrWorkflowNoEnv
	}
	workflow, err := w.GetWorkflow(ctx, app, req.Name)
	if err != nil && errors.Is(err, datastore.ErrRecordNotExist) {
		return nil, err
	}
	modelSteps, err := assembler.CreateWorkflowStepModel(req.Steps)
	if err != nil {
		return nil, err
	}
	if workflow != nil {
		workflow.Steps = modelSteps
		workflow.Alias = req.Alias
		workflow.Description = req.Description
		workflow.Default = req.Default
		if err := w.Store.Put(ctx, workflow); err != nil {
			return nil, err
		}
	} else {
		// It is allowed to set multiple workflows as default, and only one takes effect.
		workflow = &model.Workflow{
			Steps:         modelSteps,
			Name:          req.Name,
			Alias:         req.Alias,
			Description:   req.Description,
			Default:       req.Default,
			EnvName:       req.EnvName,
			AppPrimaryKey: app.PrimaryKey(),
		}
		log.Logger.Infof("create workflow %s for app %s", pkgUtils.Sanitize(req.Name), pkgUtils.Sanitize(app.PrimaryKey()))
		if err := w.Store.Add(ctx, workflow); err != nil {
			return nil, err
		}
	}
	return w.DetailWorkflow(ctx, workflow)
}

func (w *workflowServiceImpl) UpdateWorkflow(ctx context.Context, workflow *model.Workflow, req apisv1.UpdateWorkflowRequest) (*apisv1.DetailWorkflowResponse, error) {
	modeSteps, err := assembler.CreateWorkflowStepModel(req.Steps)
	if err != nil {
		return nil, err
	}
	workflow.Description = req.Description
	// It is allowed to set multiple workflows as default, and only one takes effect.
	if req.Default != nil {
		workflow.Default = req.Default
	}
	if err := repository.UpdateWorkflowSteps(ctx, w.Store, workflow, modeSteps); err != nil {
		return nil, err
	}
	return w.DetailWorkflow(ctx, workflow)
}

// DetailWorkflow detail workflow
func (w *workflowServiceImpl) DetailWorkflow(ctx context.Context, workflow *model.Workflow) (*apisv1.DetailWorkflowResponse, error) {
	return &apisv1.DetailWorkflowResponse{
		WorkflowBase: assembler.ConvertWorkflowBase(workflow),
	}, nil
}

// GetWorkflow get workflow model
func (w *workflowServiceImpl) GetWorkflow(ctx context.Context, app *model.Application, workflowName string) (*model.Workflow, error) {
	return repository.GetWorkflowForApp(ctx, w.Store, app, workflowName)
}

// ListApplicationWorkflow list application workflows
func (w *workflowServiceImpl) ListApplicationWorkflow(ctx context.Context, app *model.Application) ([]*apisv1.WorkflowBase, error) {
	var workflow = model.Workflow{
		AppPrimaryKey: app.PrimaryKey(),
	}
	workflows, err := w.Store.List(ctx, &workflow, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	var list []*apisv1.WorkflowBase
	for _, workflow := range workflows {
		wm := workflow.(*model.Workflow)
		base := assembler.ConvertWorkflowBase(wm)
		list = append(list, &base)
	}
	return list, nil
}

// GetApplicationDefaultWorkflow get application default workflow
func (w *workflowServiceImpl) GetApplicationDefaultWorkflow(ctx context.Context, app *model.Application) (*model.Workflow, error) {
	var defaultEnable = true
	var workflow = model.Workflow{
		AppPrimaryKey: app.PrimaryKey(),
		Default:       &defaultEnable,
	}
	workflows, err := w.Store.List(ctx, &workflow, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	if len(workflows) > 0 {
		return workflows[0].(*model.Workflow), nil
	}
	return nil, bcode.ErrWorkflowNoDefault
}

// ListWorkflowRecords list workflow record
func (w *workflowServiceImpl) ListWorkflowRecords(ctx context.Context, workflow *model.Workflow, page, pageSize int) (*apisv1.ListWorkflowRecordsResponse, error) {
	var record = model.WorkflowRecord{
		AppPrimaryKey: workflow.AppPrimaryKey,
		WorkflowName:  workflow.Name,
	}
	records, err := w.Store.List(ctx, &record, &datastore.ListOptions{Page: page, PageSize: pageSize})
	if err != nil {
		return nil, err
	}

	resp := &apisv1.ListWorkflowRecordsResponse{
		Records: []apisv1.WorkflowRecord{},
	}
	for _, raw := range records {
		record, ok := raw.(*model.WorkflowRecord)
		if ok {
			resp.Records = append(resp.Records, *assembler.ConvertFromRecordModel(record))
		}
	}
	count, err := w.Store.Count(ctx, &record, nil)
	if err != nil {
		return nil, err
	}
	resp.Total = count

	return resp, nil
}

// DetailWorkflowRecord get workflow record detail with name
func (w *workflowServiceImpl) DetailWorkflowRecord(ctx context.Context, workflow *model.Workflow, recordName string) (*apisv1.DetailWorkflowRecordResponse, error) {
	var record = model.WorkflowRecord{
		AppPrimaryKey: workflow.AppPrimaryKey,
		WorkflowName:  workflow.Name,
		Name:          recordName,
	}
	err := w.Store.Get(ctx, &record)
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
	err = w.Store.Get(ctx, &revision)
	if err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, bcode.ErrApplicationRevisionNotExist
		}
		return nil, err
	}

	return &apisv1.DetailWorkflowRecordResponse{
		WorkflowRecord: *assembler.ConvertFromRecordModel(&record),
		DeployTime:     revision.CreateTime,
		DeployUser:     revision.DeployUser,
		Note:           revision.Note,
		TriggerType:    revision.TriggerType,
	}, nil
}

func (w *workflowServiceImpl) SyncWorkflowRecord(ctx context.Context) error {
	var record = model.WorkflowRecord{
		Finished: "false",
	}
	// list all unfinished workflow records
	records, err := w.Store.List(ctx, &record, &datastore.ListOptions{})
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
		if err := w.Store.Get(ctx, workflow); err != nil {
			klog.ErrorS(err, "failed to get workflow", "app name", record.AppPrimaryKey, "workflow name", record.WorkflowName, "record name", record.Name)
			continue
		}
		envbinding, err := w.EnvBindingService.GetEnvBinding(ctx, &model.Application{Name: record.AppPrimaryKey}, workflow.EnvName)
		if err != nil {
			klog.ErrorS(err, "failed to get envbinding", "app name", record.AppPrimaryKey, "workflow name", record.WorkflowName, "record name", record.Name)
		}
		var appName string
		if envbinding != nil {
			appName = envbinding.AppDeployName
		}
		if appName == "" {
			appName = record.AppPrimaryKey
		}
		if err := w.KubeClient.Get(ctx, types.NamespacedName{
			Name:      appName,
			Namespace: record.Namespace,
		}, app); err != nil {
			if apierrors.IsNotFound(err) {
				if err := w.setRecordToTerminated(ctx, record.AppPrimaryKey, record.Name); err != nil {
					log.Logger.Errorf("failed to set the record status to terminated %s", err.Error())
				}
				continue
			}
			klog.ErrorS(err, "failed to get app", "oam app name", appName, "workflow name", record.WorkflowName, "record name", record.Name)
			continue
		}

		if app.Status.Workflow == nil {
			continue
		}

		// there is a ":" in the default app revision
		recordName := strings.Replace(app.Status.Workflow.AppRevision, ":", "-", 1)

		// try to sync the status from the running application
		if app.Annotations != nil && app.Status.Workflow != nil && recordName == record.Name {
			if err := w.syncWorkflowStatus(ctx, record.AppPrimaryKey, app, record.Name, app.Name); err != nil {
				klog.ErrorS(err, "failed to sync workflow status", "oam app name", appName, "workflow name", record.WorkflowName, "record name", record.Name)
			}
			continue
		}

		// try to sync the status from the application revision
		var revision = &model.ApplicationRevision{AppPrimaryKey: record.AppPrimaryKey, Version: record.RevisionPrimaryKey}
		if err := w.Store.Get(ctx, revision); err != nil {
			if errors.Is(err, datastore.ErrRecordNotExist) {
				// If the application revision is not exist, the record do not need be synced
				var record = &model.WorkflowRecord{
					AppPrimaryKey: record.AppPrimaryKey,
					Name:          recordName,
				}
				if err := w.Store.Get(ctx, record); err == nil {
					record.Finished = "true"
					record.Status = model.RevisionStatusFail
					err := w.Store.Put(ctx, record)
					if err != nil {
						log.Logger.Errorf("failed to set the workflow status is failure %s", err.Error())
					}
					continue
				}
			}
			log.Logger.Errorf("failed to get the application revision from database %s", err.Error())
			continue
		}

		var appRevision v1beta1.ApplicationRevision
		if err := w.KubeClient.Get(ctx, types.NamespacedName{Namespace: app.Namespace, Name: revision.RevisionCRName}, &appRevision); err != nil {
			if apierrors.IsNotFound(err) {
				if err := w.setRecordToTerminated(ctx, record.AppPrimaryKey, record.Name); err != nil {
					log.Logger.Errorf("failed to set the record status to terminated %s", err.Error())
				}
				continue
			}
			log.Logger.Warnf("failed to get the application revision %s", err.Error())
			continue
		}

		if appRevision.Status.Workflow != nil {
			appRevision.Spec.Application.Status.Workflow = appRevision.Status.Workflow
			if !appRevision.Spec.Application.Status.Workflow.Finished {
				appRevision.Spec.Application.Status.Workflow.Finished = true
				appRevision.Spec.Application.Status.Workflow.Terminated = true
			}
		}
		if err := w.syncWorkflowStatus(ctx, record.AppPrimaryKey, &appRevision.Spec.Application, record.Name, revision.RevisionCRName); err != nil {
			klog.ErrorS(err, "failed to sync workflow status", "oam app name", appName, "workflow name", record.WorkflowName, "record name", record.Name)
			continue
		}
	}

	return nil
}

func (w *workflowServiceImpl) setRecordToTerminated(ctx context.Context, appPrimaryKey, recordName string) error {
	var record = &model.WorkflowRecord{
		AppPrimaryKey: appPrimaryKey,
		Name:          recordName,
	}
	if err := w.Store.Get(ctx, record); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return bcode.ErrWorkflowRecordNotExist
		}
		return err
	}
	var revision = &model.ApplicationRevision{AppPrimaryKey: appPrimaryKey, Version: record.RevisionPrimaryKey}
	if err := w.Store.Get(ctx, revision); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return bcode.ErrApplicationRevisionNotExist
		}
		return err
	}
	record.Status = model.RevisionStatusTerminated
	record.Finished = "true"

	revision.Status = model.RevisionStatusTerminated

	if err := w.Store.Put(ctx, record); err != nil {
		return err
	}

	if err := w.Store.Put(ctx, revision); err != nil {
		return err
	}
	return nil
}

func (w *workflowServiceImpl) syncWorkflowStatus(ctx context.Context, appPrimaryKey string, app *v1beta1.Application, recordName, source string) error {
	var record = &model.WorkflowRecord{
		AppPrimaryKey: appPrimaryKey,
		Name:          recordName,
	}
	if err := w.Store.Get(ctx, record); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return bcode.ErrWorkflowRecordNotExist
		}
		return err
	}
	var revision = &model.ApplicationRevision{AppPrimaryKey: appPrimaryKey, Version: record.RevisionPrimaryKey}
	if err := w.Store.Get(ctx, revision); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return bcode.ErrApplicationRevisionNotExist
		}
		return err
	}

	if app.Status.Workflow != nil {
		status := app.Status.Workflow
		summaryStatus := model.RevisionStatusRunning
		switch {
		case status.Phase == workflowv1alpha1.WorkflowStateFailed:
			summaryStatus = model.RevisionStatusFail
		case status.Finished:
			summaryStatus = model.RevisionStatusComplete
		case status.Terminated:
			summaryStatus = model.RevisionStatusTerminated
		}

		record.Status = summaryStatus
		stepStatus := make(map[string]*model.WorkflowStepStatus, len(status.Steps))
		stepAlias := make(map[string]string)
		for _, step := range record.Steps {
			stepAlias[step.Name] = step.Alias
			for _, sub := range step.SubStepsStatus {
				stepAlias[sub.Name] = sub.Alias
			}
		}
		for _, step := range status.Steps {
			stepStatus[step.Name] = &model.WorkflowStepStatus{
				StepStatus:     convert.FromCRWorkflowStepStatus(step.StepStatus, stepAlias[step.Name]),
				SubStepsStatus: make([]model.StepStatus, 0),
			}
			for _, sub := range step.SubStepsStatus {
				stepStatus[step.Name].SubStepsStatus = append(stepStatus[step.Name].SubStepsStatus, convert.FromCRWorkflowStepStatus(sub, stepAlias[sub.Name]))
			}
		}
		for i, step := range record.Steps {
			if stepStatus[step.Name] != nil {
				record.Steps[i] = *stepStatus[step.Name]
			}
		}
		record.Finished = strconv.FormatBool(status.Finished)

		if err := w.Store.Put(ctx, record); err != nil {
			return err
		}

		revision.Status = summaryStatus
		if err := w.Store.Put(ctx, revision); err != nil {
			return err
		}
	}

	if record.Finished == "true" {
		klog.InfoS("successfully sync workflow status", "oam app name", app.Name, "workflow name", record.WorkflowName, "record name", record.Name, "status", record.Status, "sync source", source)
	}

	return nil
}

func (w *workflowServiceImpl) CreateWorkflowRecord(ctx context.Context, appModel *model.Application, app *v1beta1.Application, workflow *model.Workflow) error {
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
			StepStatus: model.StepStatus{
				Name:  step.Name,
				Alias: step.Alias,
				Type:  step.Type,
			},
			SubStepsStatus: make([]model.StepStatus, 0),
		}
		for _, sub := range step.SubSteps {
			steps[i].SubStepsStatus = append(steps[i].SubStepsStatus, model.StepStatus{
				Name:  sub.Name,
				Alias: sub.Alias,
				Type:  sub.Type,
			})
		}
	}

	if err := w.Store.Add(ctx, &model.WorkflowRecord{
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

	if err := resetRevisionsAndRecords(ctx, w.Store, appModel.PrimaryKey(), workflow.Name, app.Annotations[oam.AnnotationDeployVersion], app.Annotations[oam.AnnotationPublishVersion]); err != nil {
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
			for i, step := range record.Steps {
				if step.Phase == workflowv1alpha1.WorkflowStepPhaseRunning {
					record.Steps[i].Phase = model.WorkflowStepPhaseStopped
				}
			}
			if err := ds.Put(ctx, record); err != nil {
				klog.Info("failed to set rest records' status to terminate", "app name", appName, "workflow name", record.WorkflowName, "record name", record.Name, "error", err)
			}
		}
	}

	return nil
}

func (w *workflowServiceImpl) CountWorkflow(ctx context.Context, app *model.Application) int64 {
	count, err := w.Store.Count(ctx, &model.Workflow{AppPrimaryKey: app.PrimaryKey()}, &datastore.FilterOptions{})
	if err != nil {
		log.Logger.Errorf("count app %s workflow failure %s", app.PrimaryKey(), err.Error())
	}
	return count
}

func (w *workflowServiceImpl) ResumeRecord(ctx context.Context, appModel *model.Application, workflow *model.Workflow, recordName string) error {
	oamApp, err := w.checkRecordRunning(ctx, appModel, workflow.EnvName)
	if err != nil {
		return err
	}

	if err := ResumeWorkflow(ctx, w.KubeClient, oamApp); err != nil {
		return err
	}

	if err := w.syncWorkflowStatus(ctx, appModel.PrimaryKey(), oamApp, recordName, oamApp.Name); err != nil {
		return err
	}

	return nil
}

func (w *workflowServiceImpl) TerminateRecord(ctx context.Context, appModel *model.Application, workflow *model.Workflow, recordName string) error {
	oamApp, err := w.checkRecordRunning(ctx, appModel, workflow.EnvName)
	if err != nil {
		return err
	}
	if err := TerminateWorkflow(ctx, w.KubeClient, oamApp); err != nil {
		return err
	}
	if err := w.syncWorkflowStatus(ctx, appModel.PrimaryKey(), oamApp, recordName, oamApp.Name); err != nil {
		return err
	}

	return nil
}

// ResumeWorkflow resume workflow
func ResumeWorkflow(ctx context.Context, kubecli client.Client, app *v1beta1.Application) error {
	app.Status.Workflow.Suspend = false
	steps := app.Status.Workflow.Steps
	for i, step := range steps {
		if step.Type == wfTypes.WorkflowStepTypeSuspend && step.Phase == workflowv1alpha1.WorkflowStepPhaseRunning {
			steps[i].Phase = workflowv1alpha1.WorkflowStepPhaseSucceeded
		}
		for j, sub := range step.SubStepsStatus {
			if sub.Type == wfTypes.WorkflowStepTypeSuspend && sub.Phase == workflowv1alpha1.WorkflowStepPhaseRunning {
				steps[i].SubStepsStatus[j].Phase = workflowv1alpha1.WorkflowStepPhaseSucceeded
			}
		}
	}
	if err := kubecli.Status().Patch(ctx, app, client.Merge); err != nil {
		return err
	}
	return nil
}

// TerminateWorkflow terminate workflow
func TerminateWorkflow(ctx context.Context, kubecli client.Client, app *v1beta1.Application) error {
	// set the workflow terminated to true
	app.Status.Workflow.Terminated = true
	// set the workflow suspend to false
	app.Status.Workflow.Suspend = false
	steps := app.Status.Workflow.Steps
	for i, step := range steps {
		switch step.Phase {
		case workflowv1alpha1.WorkflowStepPhaseFailed:
			if step.Reason != wfTypes.StatusReasonFailedAfterRetries && step.Reason != wfTypes.StatusReasonTimeout {
				steps[i].Reason = wfTypes.StatusReasonTerminate
			}
		case workflowv1alpha1.WorkflowStepPhaseRunning:
			steps[i].Phase = workflowv1alpha1.WorkflowStepPhaseFailed
			steps[i].Reason = wfTypes.StatusReasonTerminate
		default:
		}
		for j, sub := range step.SubStepsStatus {
			switch sub.Phase {
			case workflowv1alpha1.WorkflowStepPhaseFailed:
				if sub.Reason != wfTypes.StatusReasonFailedAfterRetries && sub.Reason != wfTypes.StatusReasonTimeout {
					steps[i].SubStepsStatus[j].Reason = wfTypes.StatusReasonTerminate
				}
			case workflowv1alpha1.WorkflowStepPhaseRunning:
				steps[i].SubStepsStatus[j].Phase = workflowv1alpha1.WorkflowStepPhaseFailed
				steps[i].SubStepsStatus[j].Reason = wfTypes.StatusReasonTerminate
			default:
			}
		}
	}

	if err := kubecli.Status().Patch(ctx, app, client.Merge); err != nil {
		return err
	}
	return nil
}

func (w *workflowServiceImpl) RollbackRecord(ctx context.Context, appModel *model.Application, workflow *model.Workflow, recordName, revisionVersion string) error {
	if revisionVersion == "" {
		// find the latest complete revision version
		var revision = model.ApplicationRevision{
			AppPrimaryKey: appModel.Name,
			Status:        model.RevisionStatusComplete,
			WorkflowName:  workflow.Name,
			EnvName:       workflow.EnvName,
		}
		revisions, err := w.Store.List(ctx, &revision, &datastore.ListOptions{
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
	if err := w.Store.Get(ctx, record); err != nil {
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
	if err := w.Store.Get(ctx, originalRevision); err != nil {
		return err
	}

	var rollbackRevision = &model.ApplicationRevision{
		AppPrimaryKey: appModel.Name,
		Version:       revisionVersion,
	}
	if err := w.Store.Get(ctx, rollbackRevision); err != nil {
		return err
	}

	// update the original revision status to rollback
	originalRevision.Status = model.RevisionStatusRollback
	originalRevision.RollbackVersion = revisionVersion
	originalRevision.UpdateTime = time.Now().Time
	if err := w.Store.Put(ctx, originalRevision); err != nil {
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

	if err := w.Apply.Apply(ctx, oamApp); err != nil {
		// rollback error case
		if err := w.Store.Delete(ctx, &model.WorkflowRecord{Name: newRecordName}); err != nil {
			klog.Error(err, "failed to delete record", newRecordName)
		}
		return err
	}

	return nil
}

func (w *workflowServiceImpl) checkRecordRunning(ctx context.Context, appModel *model.Application, envName string) (*v1beta1.Application, error) {
	oamApp := &v1beta1.Application{}
	env, err := w.EnvService.GetEnv(ctx, envName)
	if err != nil {
		return nil, err
	}
	envbinding, err := w.EnvBindingService.GetEnvBinding(ctx, appModel, envName)
	if err != nil {
		return nil, err
	}
	name := envbinding.AppDeployName
	if name == "" {
		name = appModel.Name
	}
	if err := w.KubeClient.Get(ctx, types.NamespacedName{Name: name, Namespace: env.Namespace}, oamApp); err != nil {
		return nil, err
	}
	if oamApp.Status.Workflow != nil && !oamApp.Status.Workflow.Suspend && !oamApp.Status.Workflow.Terminated && !oamApp.Status.Workflow.Finished {
		return nil, fmt.Errorf("workflow is still running, can not operate a running workflow")
	}

	oamApp.SetGroupVersionKind(v1beta1.ApplicationKindVersionKind)
	return oamApp, nil
}
