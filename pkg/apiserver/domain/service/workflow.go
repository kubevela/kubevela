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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	"helm.sh/helm/v3/pkg/time"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	wfContext "github.com/kubevela/workflow/pkg/context"
	"github.com/kubevela/workflow/pkg/cue/model/value"
	wfTypes "github.com/kubevela/workflow/pkg/types"
	wfUtils "github.com/kubevela/workflow/pkg/utils"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/repository"
	"github.com/oam-dev/kubevela/pkg/apiserver/event/sync/convert"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	assembler "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/assembler/v1"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/oam"
	pkgUtils "github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// LogSourceResource Read the step logs from the pod stdout.
const LogSourceResource = "Resource"

// LogSourceURL Read the step logs from the URL.
const LogSourceURL = "URL"

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

	GetWorkflowRecord(ctx context.Context, workflow *model.Workflow, recordName string) (*model.WorkflowRecord, error)
	CreateWorkflowRecord(ctx context.Context, appModel *model.Application, app *v1beta1.Application, workflow *model.Workflow) (*model.WorkflowRecord, error)
	ListWorkflowRecords(ctx context.Context, workflow *model.Workflow, page, pageSize int) (*apisv1.ListWorkflowRecordsResponse, error)
	DetailWorkflowRecord(ctx context.Context, workflow *model.Workflow, recordName string) (*apisv1.DetailWorkflowRecordResponse, error)
	SyncWorkflowRecord(ctx context.Context) error
	ResumeRecord(ctx context.Context, appModel *model.Application, workflow *model.Workflow, recordName string) error
	TerminateRecord(ctx context.Context, appModel *model.Application, workflow *model.Workflow, recordName string) error
	RollbackRecord(ctx context.Context, appModel *model.Application, workflow *model.Workflow, recordName, revisionName string) (*apisv1.WorkflowRecordBase, error)
	GetWorkflowRecordLog(ctx context.Context, record *model.WorkflowRecord, step string) (apisv1.GetPipelineRunLogResponse, error)
	GetWorkflowRecordOutput(ctx context.Context, workflow *model.Workflow, record *model.WorkflowRecord, stepName string) (apisv1.GetPipelineRunOutputResponse, error)
	GetWorkflowRecordInput(ctx context.Context, workflow *model.Workflow, record *model.WorkflowRecord, stepName string) (apisv1.GetPipelineRunInputResponse, error)

	CountWorkflow(ctx context.Context, app *model.Application) int64
}

// NewWorkflowService new workflow service
func NewWorkflowService() WorkflowService {
	return &workflowServiceImpl{}
}

type workflowServiceImpl struct {
	Store             datastore.DataStore `inject:"datastore"`
	KubeClient        client.Client       `inject:"kubeClient"`
	KubeConfig        *rest.Config        `inject:"kubeConfig"`
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
		klog.Errorf("list workflow %s record failure %s", pkgUtils.Sanitize(workflow.PrimaryKey()), err.Error())
	}
	for _, record := range records {
		if err := w.Store.Delete(ctx, record); err != nil {
			klog.Errorf("delete workflow record %s failure %s", record.PrimaryKey(), err.Error())
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
			klog.Errorf("delete workflow %s failure %s", workflow.PrimaryKey(), err.Error())
		}
	}
	var record = model.WorkflowRecord{
		AppPrimaryKey: workflow.AppPrimaryKey,
	}
	records, err := w.Store.List(ctx, &record, &datastore.ListOptions{})
	if err != nil {
		klog.Errorf("list workflow %s record failure %s", workflow.PrimaryKey(), err.Error())
	}
	for _, record := range records {
		if err := w.Store.Delete(ctx, record); err != nil {
			klog.Errorf("delete workflow record %s failure %s", record.PrimaryKey(), err.Error())
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
	if req.Mode == "" {
		req.Mode = string(workflowv1alpha1.WorkflowModeStep)
	}
	if req.SubMode == "" {
		req.Mode = string(workflowv1alpha1.WorkflowModeDAG)
	}
	if workflow != nil {
		workflow.Steps = modelSteps
		workflow.Alias = req.Alias
		workflow.Description = req.Description
		workflow.Default = req.Default
		workflow.Mode.Steps = workflowv1alpha1.WorkflowMode(req.Mode)
		workflow.Mode.SubSteps = workflowv1alpha1.WorkflowMode(req.SubMode)
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
			Mode: workflowv1alpha1.WorkflowExecuteMode{
				Steps:    workflowv1alpha1.WorkflowMode(req.Mode),
				SubSteps: workflowv1alpha1.WorkflowMode(req.SubMode),
			},
		}
		klog.Infof("create workflow %s for app %s", pkgUtils.Sanitize(req.Name), pkgUtils.Sanitize(app.PrimaryKey()))
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
	workflow.Alias = req.Alias
	if req.Mode == "" {
		req.Mode = string(workflowv1alpha1.WorkflowModeStep)
	}
	if req.SubMode == "" {
		req.Mode = string(workflowv1alpha1.WorkflowModeDAG)
	}
	workflow.Mode.Steps = workflowv1alpha1.WorkflowMode(req.Mode)
	workflow.Mode.SubSteps = workflowv1alpha1.WorkflowMode(req.SubMode)
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
					klog.Errorf("failed to set the record status to terminated %s", err.Error())
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
			if err := w.syncWorkflowStatus(ctx, record.AppPrimaryKey, app, record.Name, app.Name, nil); err != nil {
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
						klog.Errorf("failed to set the workflow status is failure %s", err.Error())
					}
					continue
				}
			}
			klog.Errorf("failed to get the application revision from database %s", err.Error())
			continue
		}

		var appRevision v1beta1.ApplicationRevision
		if err := w.KubeClient.Get(ctx, types.NamespacedName{Namespace: app.Namespace, Name: revision.RevisionCRName}, &appRevision); err != nil {
			if apierrors.IsNotFound(err) {
				if err := w.setRecordToTerminated(ctx, record.AppPrimaryKey, record.Name); err != nil {
					klog.Errorf("failed to set the record status to terminated %s", err.Error())
				}
				continue
			}
			klog.Warningf("failed to get the application revision %s", err.Error())
			continue
		}

		if appRevision.Status.Workflow != nil {
			appRevision.Spec.Application.Status.Workflow = appRevision.Status.Workflow
			if !appRevision.Spec.Application.Status.Workflow.Finished {
				appRevision.Spec.Application.Status.Workflow.Finished = true
				appRevision.Spec.Application.Status.Workflow.Terminated = true
			}
		}
		if err := w.syncWorkflowStatus(ctx,
			record.AppPrimaryKey,
			&appRevision.Spec.Application,
			record.Name,
			revision.RevisionCRName,
			appRevision.Status.WorkflowContext,
		); err != nil {
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

func (w *workflowServiceImpl) syncWorkflowStatus(ctx context.Context,
	appPrimaryKey string,
	app *v1beta1.Application,
	recordName,
	source string,
	workflowContext map[string]string) error {
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

	if workflowContext != nil {
		record.ContextValue = workflowContext
	}

	if app.Status.Workflow != nil {
		if app.Status.Workflow.AppRevision != record.Name {
			klog.Warningf("the app(%s) revision is not match the record(%s), try next time..", app.Name, record.Name)
			return nil
		}
		status := app.Status.Workflow
		record.Status = string(status.Phase)
		record.Message = status.Message
		record.Mode = status.Mode

		if cb := app.Status.Workflow.ContextBackend; cb != nil && workflowContext == nil {
			var cm corev1.ConfigMap
			if err := w.KubeClient.Get(ctx, types.NamespacedName{Namespace: cb.Namespace, Name: cb.Name}, &cm); err != nil {
				klog.Error(err, "failed to load the context values", "Application", app.Name)
			}
			record.ContextValue = cm.Data
		}

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

		// the auto generated workflow steps should be sync
		if (len(record.Steps) == 0) && len(status.Steps) > 0 {
			for k := range stepStatus {
				record.Steps = append(record.Steps, *stepStatus[k])
			}
		}

		record.Finished = strconv.FormatBool(status.Finished)
		record.EndTime = status.EndTime.Time
		if err := w.Store.Put(ctx, record); err != nil {
			return err
		}

		revision.Status = generateRevisionStatus(status.Phase)
		if err := w.Store.Put(ctx, revision); err != nil {
			return err
		}
	}

	if record.Finished == "true" {
		klog.InfoS("successfully sync workflow status", "oam app name", app.Name, "workflow name", record.WorkflowName, "record name", record.Name, "status", record.Status, "sync source", source)
	}

	return nil
}

func generateRevisionStatus(phase workflowv1alpha1.WorkflowRunPhase) string {
	summaryStatus := model.RevisionStatusRunning
	switch {
	case phase == workflowv1alpha1.WorkflowStateFailed:
		summaryStatus = model.RevisionStatusFail
	case phase == workflowv1alpha1.WorkflowStateSucceeded:
		summaryStatus = model.RevisionStatusComplete
	case phase == workflowv1alpha1.WorkflowStateTerminated:
		summaryStatus = model.RevisionStatusTerminated
	}
	return summaryStatus
}

func (w *workflowServiceImpl) CreateWorkflowRecord(ctx context.Context, appModel *model.Application, app *v1beta1.Application, workflow *model.Workflow) (*model.WorkflowRecord, error) {
	if app.Annotations == nil {
		return nil, fmt.Errorf("empty annotations in application")
	}
	if app.Annotations[oam.AnnotationPublishVersion] == "" {
		return nil, fmt.Errorf("failed to get record version from application")
	}
	if app.Annotations[oam.AnnotationDeployVersion] == "" {
		return nil, fmt.Errorf("failed to get deploy version from application")
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

	workflowRecord := &model.WorkflowRecord{
		WorkflowName:       workflow.Name,
		WorkflowAlias:      workflow.Alias,
		AppPrimaryKey:      appModel.PrimaryKey(),
		RevisionPrimaryKey: app.Annotations[oam.AnnotationDeployVersion],
		Name:               app.Annotations[oam.AnnotationPublishVersion],
		Namespace:          app.Namespace,
		Finished:           "false",
		StartTime:          time.Now().Time,
		Steps:              steps,
		Status:             string(workflowv1alpha1.WorkflowStateInitializing),
	}

	if err := w.Store.Add(ctx, workflowRecord); err != nil {
		return nil, err
	}

	if err := resetRevisionsAndRecords(ctx, w.Store, appModel.PrimaryKey(), workflow.Name, app.Annotations[oam.AnnotationDeployVersion], app.Annotations[oam.AnnotationPublishVersion]); err != nil {
		return workflowRecord, err
	}

	return workflowRecord, nil
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
		klog.Errorf("count app %s workflow failure %s", app.PrimaryKey(), err.Error())
	}
	return count
}

func (w *workflowServiceImpl) GetWorkflowRecord(ctx context.Context, workflow *model.Workflow, recordName string) (*model.WorkflowRecord, error) {
	var record = &model.WorkflowRecord{
		WorkflowName: workflow.Name,
		Name:         recordName,
	}
	res, err := w.Store.List(ctx, record, nil)
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		return nil, bcode.ErrWorkflowRecordNotExist
	}
	return res[0].(*model.WorkflowRecord), nil
}

func (w *workflowServiceImpl) ResumeRecord(ctx context.Context, appModel *model.Application, workflow *model.Workflow, recordName string) error {
	oamApp, err := w.checkRecordRunning(ctx, appModel, workflow.EnvName)
	if err != nil {
		return err
	}

	if err := ResumeWorkflow(ctx, w.KubeClient, oamApp); err != nil {
		return err
	}

	if err := w.syncWorkflowStatus(ctx, appModel.PrimaryKey(), oamApp, recordName, oamApp.Name, nil); err != nil {
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
	if err := w.syncWorkflowStatus(ctx, appModel.PrimaryKey(), oamApp, recordName, oamApp.Name, nil); err != nil {
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

func (w *workflowServiceImpl) RollbackRecord(ctx context.Context, appModel *model.Application, workflow *model.Workflow, recordName, revisionVersion string) (*apisv1.WorkflowRecordBase, error) {
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
			return nil, err
		}
		if len(revisions) == 0 {
			return nil, bcode.ErrApplicationNoReadyRevision
		}
		revisionVersion = pkgUtils.ToString(revisions[0].Index()["version"])
		klog.Infof("select lastest complete revision %s", revisionVersion)
	}

	var record = &model.WorkflowRecord{
		AppPrimaryKey: appModel.PrimaryKey(),
		Name:          recordName,
	}
	if err := w.Store.Get(ctx, record); err != nil {
		return nil, err
	}

	oamApp, err := w.checkRecordRunning(ctx, appModel, workflow.EnvName)
	if err != nil {
		return nil, err
	}

	var originalRevision = &model.ApplicationRevision{
		AppPrimaryKey: appModel.Name,
		Version:       record.RevisionPrimaryKey,
	}
	if err := w.Store.Get(ctx, originalRevision); err != nil {
		return nil, err
	}

	var rollbackRevision = &model.ApplicationRevision{
		AppPrimaryKey: appModel.Name,
		Version:       revisionVersion,
	}
	if err := w.Store.Get(ctx, rollbackRevision); err != nil {
		return nil, err
	}

	// update the original revision status to rollback
	originalRevision.Status = model.RevisionStatusRollback
	originalRevision.RollbackVersion = revisionVersion
	originalRevision.UpdateTime = time.Now().Time
	if err := w.Store.Put(ctx, originalRevision); err != nil {
		return nil, err
	}

	rollBackApp := &v1beta1.Application{}
	if err := yaml.Unmarshal([]byte(rollbackRevision.ApplyAppConfig), rollBackApp); err != nil {
		return nil, err
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
	newRecord, err := w.CreateWorkflowRecord(ctx, appModel, oamApp, workflow)
	if err != nil {
		return nil, err
	}

	if err := w.Apply.Apply(ctx, oamApp); err != nil {
		// rollback error case
		if err := w.Store.Delete(ctx, &model.WorkflowRecord{Name: newRecordName}); err != nil {
			klog.Error(err, "failed to delete record", newRecordName)
		}
		return nil, err
	}

	return &assembler.ConvertFromRecordModel(newRecord).WorkflowRecordBase, nil
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

func (w *workflowServiceImpl) GetWorkflowRecordLog(ctx context.Context, record *model.WorkflowRecord, step string) (apisv1.GetPipelineRunLogResponse, error) {
	if len(record.ContextValue) == 0 {
		return apisv1.GetPipelineRunLogResponse{}, nil
	}
	logConfig, err := getLogConfigFromStep(record.ContextValue, step)
	if err != nil {
		if strings.Contains(err.Error(), "no log config found") {
			return apisv1.GetPipelineRunLogResponse{
				StepBase: getWorkflowStepBase(*record, step),
				Log:      "",
			}, nil
		}
		return apisv1.GetPipelineRunLogResponse{}, err
	}
	var logs string
	var source string
	if logConfig.Source != nil {
		if len(logConfig.Source.Resources) > 0 {
			source = LogSourceResource
			logs, err = getResourceLogs(ctx, w.KubeConfig, w.KubeClient, logConfig.Source.Resources, nil)
			if err != nil {
				return apisv1.GetPipelineRunLogResponse{LogSource: source}, err
			}
		}
		if logConfig.Source.URL != "" {
			source = LogSourceURL
			var logsBuilder strings.Builder
			readCloser, err := wfUtils.GetLogsFromURL(ctx, logConfig.Source.URL)
			if err != nil {
				klog.Errorf("get logs from url %s failed: %v", logConfig.Source.URL, err)
				return apisv1.GetPipelineRunLogResponse{LogSource: source}, bcode.ErrReadSourceLog
			}
			//nolint:errcheck
			defer readCloser.Close()
			if _, err := io.Copy(&logsBuilder, readCloser); err != nil {
				klog.Errorf("copy logs from url %s failed: %v", logConfig.Source.URL, err)
				return apisv1.GetPipelineRunLogResponse{LogSource: source}, bcode.ErrReadSourceLog
			}
			logs = logsBuilder.String()
		}
	}
	return apisv1.GetPipelineRunLogResponse{
		LogSource: source,
		StepBase:  getWorkflowStepBase(*record, step),
		Log:       logs,
	}, nil
}

func (w *workflowServiceImpl) GetWorkflowRecordOutput(ctx context.Context, workflow *model.Workflow, record *model.WorkflowRecord, stepName string) (apisv1.GetPipelineRunOutputResponse, error) {
	outputsSpec := make(map[string]workflowv1alpha1.StepOutputs)
	stepOutputs := make([]apisv1.StepOutputBase, 0)

	for _, step := range workflow.Steps {
		if step.Outputs != nil {
			outputsSpec[step.Name] = step.Outputs
		}
		for _, sub := range step.SubSteps {
			if sub.Outputs != nil {
				outputsSpec[sub.Name] = sub.Outputs
			}
		}
	}

	ctxBackend := record.ContextValue
	if ctxBackend == nil {
		return apisv1.GetPipelineRunOutputResponse{}, nil
	}
	v, err := getDataFromContext(ctxBackend)
	if err != nil {
		klog.Errorf("get data from context backend failed: %v", err)
		return apisv1.GetPipelineRunOutputResponse{}, bcode.ErrGetContextBackendData
	}
	for _, s := range record.Steps {
		if stepName != "" && s.Name != stepName {
			subStepStatus, ok := haveSubStep(s, stepName)
			if !ok {
				continue
			}
			subVars := getStepOutputs(convertWorkflowStep(*subStepStatus), outputsSpec, v)
			stepOutputs = append(stepOutputs, subVars)
			break
		}
		stepOutputs = append(stepOutputs, getStepOutputs(convertWorkflowStep(s.StepStatus), outputsSpec, v))
		for _, sub := range s.SubStepsStatus {
			stepOutputs = append(stepOutputs, getStepOutputs(convertWorkflowStep(sub), outputsSpec, v))
		}
		if stepName != "" && s.Name == stepName {
			// already found the step
			break
		}
	}
	return apisv1.GetPipelineRunOutputResponse{StepOutputs: stepOutputs}, nil
}

func (w *workflowServiceImpl) GetWorkflowRecordInput(ctx context.Context, workflow *model.Workflow, record *model.WorkflowRecord, stepName string) (apisv1.GetPipelineRunInputResponse, error) {
	// valueFromStep know which step the value came from
	valueFromStep := make(map[string]string)
	inputsSpec := make(map[string]workflowv1alpha1.StepInputs)
	stepInputs := make([]apisv1.StepInputBase, 0)

	for _, step := range workflow.Steps {
		if step.Inputs != nil {
			inputsSpec[step.Name] = step.Inputs
		}
		if step.Outputs != nil {
			for _, o := range step.Outputs {
				valueFromStep[o.Name] = step.Name
			}
		}
		for _, sub := range step.SubSteps {
			if sub.Inputs != nil {
				inputsSpec[sub.Name] = sub.Inputs
			}
			if sub.Outputs != nil {
				for _, o := range sub.Outputs {
					valueFromStep[o.Name] = sub.Name
				}
			}
		}
	}

	ctxBackend := record.ContextValue
	if ctxBackend == nil {
		return apisv1.GetPipelineRunInputResponse{}, nil
	}
	v, err := getDataFromContext(ctxBackend)
	if err != nil {
		klog.Errorf("get data from context backend failed: %v", err)
		return apisv1.GetPipelineRunInputResponse{}, bcode.ErrGetContextBackendData
	}
	for _, s := range record.Steps {
		if stepName != "" && s.Name != stepName {
			subStepStatus, ok := haveSubStep(s, stepName)
			if !ok {
				continue
			}
			subVars := getStepInputs(convertWorkflowStep(*subStepStatus), inputsSpec, v, valueFromStep)
			stepInputs = append(stepInputs, subVars)
			break
		}
		stepInputs = append(stepInputs, getStepInputs(convertWorkflowStep(s.StepStatus), inputsSpec, v, valueFromStep))
		for _, sub := range s.SubStepsStatus {
			stepInputs = append(stepInputs, getStepInputs(convertWorkflowStep(sub), inputsSpec, v, valueFromStep))
		}
		if stepName != "" && s.Name == stepName {
			// already found the step
			break
		}
	}
	return apisv1.GetPipelineRunInputResponse{StepInputs: stepInputs}, nil
}

func getWorkflowStepBase(record model.WorkflowRecord, step string) apisv1.StepBase {
	for _, s := range record.Steps {
		if s.Name == step {
			return apisv1.StepBase{
				ID:    s.ID,
				Name:  s.Name,
				Type:  s.Type,
				Phase: string(s.Phase),
			}
		}
	}
	return apisv1.StepBase{}
}

func getLogConfigFromStep(ctxValue map[string]string, step string) (*wfTypes.LogConfig, error) {
	wc := wfContext.WorkflowContext{}
	if err := wc.LoadFromConfigMap(corev1.ConfigMap{
		Data: ctxValue,
	}); err != nil {
		return nil, err
	}
	config := make(map[string]wfTypes.LogConfig)
	c := wc.GetMutableValue(wfTypes.ContextKeyLogConfig)
	if c == "" {
		return nil, fmt.Errorf("no log config found")
	}

	if err := json.Unmarshal([]byte(c), &config); err != nil {
		return nil, err
	}

	stepConfig, ok := config[step]
	if !ok {
		return nil, fmt.Errorf("no log config found for step %s", step)
	}
	return &stepConfig, nil
}

func getDataFromContext(ctxValue map[string]string) (*value.Value, error) {
	wc := wfContext.WorkflowContext{}
	if err := wc.LoadFromConfigMap(corev1.ConfigMap{
		Data: ctxValue,
	}); err != nil {
		return nil, err
	}
	v, err := wc.GetVar()
	if err != nil {
		return nil, err
	}
	if v.Error() != nil {
		return nil, v.Error()
	}
	return v, nil
}

func haveSubStep(step model.WorkflowStepStatus, subStep string) (*model.StepStatus, bool) {
	for _, s := range step.SubStepsStatus {
		if s.Name == subStep {
			return &s, true
		}
	}
	return nil, false
}

func convertWorkflowStep(stepStatus model.StepStatus) workflowv1alpha1.StepStatus {
	return workflowv1alpha1.StepStatus{
		Name:    stepStatus.Name,
		ID:      stepStatus.ID,
		Type:    stepStatus.Type,
		Phase:   stepStatus.Phase,
		Message: stepStatus.Message,
		Reason:  stepStatus.Reason,
	}
}
