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
	"fmt"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"strings"

	"github.com/pkg/errors"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/workflow/api/v1alpha1"
	wfTypes "github.com/kubevela/workflow/pkg/types"

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

const (
	labelDescription = "pipeline.velaux.oam.dev/description"
	labelAlias       = "pipeline.velaux.oam.dev/alias"
	labelContext     = "pipeline.velaux.oam.dev/context"
)

const (
	labelContextName = "context.velaux.oam.dev/name"
)

// PipelineService is the interface for pipeline service
type PipelineService interface {
	CreatePipeline(ctx context.Context, req apis.CreatePipelineRequest) (*apis.PipelineBase, error)
	ListPipelines(ctx context.Context, req apis.ListPipelineRequest) (*apis.ListPipelineResponse, error)
	GetPipeline(ctx context.Context, name, project string) (*apis.GetPipelineResponse, error)
	UpdatePipeline(ctx context.Context, name, project string, req apis.UpdatePipelineRequest) (*apis.PipelineBase, error)
	DeletePipeline(ctx context.Context, base apis.PipelineBase) error
	RunPipeline(ctx context.Context, pipeline apis.PipelineBase, req apis.RunPipelineRequest) error
}

type pipelineServiceImpl struct {
	ContextService ContextService   `inject:""`
	KubeClient     client.Client    `inject:"kubeClient"`
	KubeConfig     *rest.Config     `inject:"kubeConfig"`
	Apply          apply.Applicator `inject:"apply"`
}

// PipelineRunService is the interface for pipelineRun service
type PipelineRunService interface {
	GetPipelineRun(ctx context.Context, meta apis.PipelineRunMeta) (apis.PipelineRun, error)
	ListPipelineRuns(ctx context.Context, base apis.PipelineBase) (apis.ListPipelineRunResponse, error)
	DeletePipelineRun(ctx context.Context, meta apis.PipelineRunMeta) error
	StopPipelineRun(ctx context.Context, pipeline apis.PipelineRunBase) error
}

type pipelineRunServiceImpl struct {
	KubeClient     client.Client    `inject:"kubeClient"`
	KubeConfig     *rest.Config     `inject:"kubeConfig"`
	Apply          apply.Applicator `inject:"apply"`
	ContextService ContextService   `inject:""`
}

// ContextService is the interface for context service
type ContextService interface {
	GetContext(ctx context.Context, projectName, pipelineName string, name string) (*apis.Context, error)
	CreateContext(ctx context.Context, projectName, pipelineName string, context apis.Context) (*model.PipelineContext, error)
	UpdateContext(ctx context.Context, projectName, pipelineName string, context apis.Context) (*model.PipelineContext, error)
	ListContexts(ctx context.Context, projectName, pipelineName string) (*apis.ListContextValueResponse, error)
	DeleteContext(ctx context.Context, projectName, pipelineName, name string) error
}

type contextServiceImpl struct {
	Store datastore.DataStore `inject:"datastore"`
}

// NewPipelineService new pipeline service
func NewPipelineService() PipelineService {
	return &pipelineServiceImpl{}
}

// NewPipelineRunService new pipelineRun service
func NewPipelineRunService() PipelineRunService {
	return &pipelineRunServiceImpl{}
}

// NewContextService new context service
func NewContextService() ContextService {
	return &contextServiceImpl{}
}

// CreatePipeline will create a pipeline
func (p pipelineServiceImpl) CreatePipeline(ctx context.Context, req apis.CreatePipelineRequest) (*apis.PipelineBase, error) {
	wf := v1alpha1.Workflow{}
	wf.SetName(req.Name)
	wf.SetNamespace(nsForProj(req.Project))
	wf.WorkflowSpec = req.Spec
	wf.SetLabels(map[string]string{
		labelDescription: req.Description,
		labelAlias:       req.Alias})
	if err := p.KubeClient.Create(ctx, &wf); err != nil {
		return nil, err
	}
	return &apis.PipelineBase{
		PipelineMeta: apis.PipelineMeta{
			Name:        req.Name,
			Alias:       req.Alias,
			Project:     req.Project,
			Description: req.Description,
		},
		Spec: wf.WorkflowSpec,
	}, nil
}

// ListPipelines will list all pipelines
func (p pipelineServiceImpl) ListPipelines(ctx context.Context, req apis.ListPipelineRequest) (*apis.ListPipelineResponse, error) {
	wfs := v1alpha1.WorkflowList{}
	nsOption := make([]client.ListOption, 0)
	for _, ns := range req.Projects {
		nsOption = append(nsOption, client.InNamespace(nsForProj(ns)))
	}
	if err := p.KubeClient.List(ctx, &wfs, nsOption...); err != nil {
		return nil, err
	}
	res := apis.ListPipelineResponse{}
	for _, wf := range wfs.Items {
		if fuzzyMatch(wf, req.Query) {
			item := apis.PipelineListItem{
				PipelineMeta: workflow2PipelineBase(wf).PipelineMeta,
				// todo info
			}
			res.Pipelines = append(res.Pipelines, item)
		}
	}
	return &res, nil
}

// GetPipeline will get a pipeline
func (p pipelineServiceImpl) GetPipeline(ctx context.Context, name, project string) (*apis.GetPipelineResponse, error) {
	wf := v1alpha1.Workflow{}
	if err := p.KubeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: nsForProj(project)}, &wf); err != nil {
		return nil, err
	}
	return &apis.GetPipelineResponse{
		PipelineBase: *workflow2PipelineBase(wf),
		// todo info
	}, nil
}

// UpdatePipeline will update a pipeline
func (p pipelineServiceImpl) UpdatePipeline(ctx context.Context, name, project string, req apis.UpdatePipelineRequest) (*apis.PipelineBase, error) {
	wf := v1alpha1.Workflow{}
	if err := p.KubeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: nsForProj(project)}, &wf); err != nil {
		return nil, err
	}
	wf.WorkflowSpec = req.Spec
	wf.SetLabels(map[string]string{
		labelDescription: req.Description,
		labelAlias:       req.Alias})
	if err := p.KubeClient.Update(ctx, &wf); err != nil {
		return nil, err
	}
	return workflow2PipelineBase(wf), nil
}

// DeletePipeline will delete a pipeline
func (p pipelineServiceImpl) DeletePipeline(ctx context.Context, pl apis.PipelineBase) error {
	wf := v1alpha1.Workflow{}
	if err := p.KubeClient.Get(ctx, client.ObjectKey{Name: pl.Name, Namespace: nsForProj(pl.Project)}, &wf); err != nil {
		return err
	}
	return p.KubeClient.Delete(ctx, &wf)
}

// StopPipelineRun will stop a pipelineRun
func (p pipelineRunServiceImpl) StopPipelineRun(ctx context.Context, pipelineRun apis.PipelineRunBase) error {
	run, err := p.checkRecordRunning(ctx, pipelineRun)
	if err != nil {
		return err
	}
	if err := p.terminatePipelineRun(ctx, run); err != nil {
		return err
	}
	return nil
}

// RunPipeline will run a pipeline
func (p pipelineServiceImpl) RunPipeline(ctx context.Context, pipeline apis.PipelineBase, req apis.RunPipelineRequest) error {
	run := v1alpha1.WorkflowRun{}
	version := utils.GenerateVersion("")
	name := fmt.Sprintf("%s-%s", pipeline.Name, version)
	run.Name = name
	run.Namespace = fmt.Sprintf("%s-project", pipeline.Project)
	run.Spec.WorkflowRef = pipeline.Name
	run.Spec.Mode = &req.Mode

	// process the context
	reqCtx, err := p.ContextService.GetContext(ctx, pipeline.Project, pipeline.Name, req.ContextName)
	if err != nil {
		return err
	}
	contextData := make(map[string]interface{})
	for _, pair := range reqCtx.Values {
		contextData[pair.Key] = pair.Value
	}
	run.SetLabels(map[string]string{
		labelContext: req.ContextName,
	})
	run.Spec.Context = util.Object2RawExtension(contextData)

	return p.KubeClient.Create(ctx, &run)
}

// GetPipelineRun will get a pipeline run
func (p pipelineRunServiceImpl) GetPipelineRun(ctx context.Context, meta apis.PipelineRunMeta) (apis.PipelineRun, error) {
	namespacedName := client.ObjectKey{Name: meta.PipelineName, Namespace: nsForProj(meta.Project)}
	run := v1alpha1.WorkflowRun{}
	if err := p.KubeClient.Get(ctx, namespacedName, &run); err != nil {
		return apis.PipelineRun{}, err
	}
	return workflowRun2PipelineRun(run), nil
}

// ListPipelineRuns will list all pipeline runs
func (p pipelineRunServiceImpl) ListPipelineRuns(ctx context.Context, base apis.PipelineBase) (apis.ListPipelineRunResponse, error) {
	wfrs := v1alpha1.WorkflowRunList{}
	if err := p.KubeClient.List(ctx, &wfrs, client.InNamespace(nsForProj(base.Project))); err != nil {
		return apis.ListPipelineRunResponse{}, err
	}
	res := apis.ListPipelineRunResponse{}
	for _, wfr := range wfrs.Items {
		if wfr.Spec.WorkflowRef == base.Name {
			res.Runs = append(res.Runs, p.workflowRun2runBriefing(ctx, wfr))
		}
	}
	return res, nil
}

// DeletePipelineRun will delete a pipeline run
func (p pipelineRunServiceImpl) DeletePipelineRun(ctx context.Context, meta apis.PipelineRunMeta) error {
	namespacedName := client.ObjectKey{Name: meta.PipelineName, Namespace: nsForProj(meta.Project)}
	run := v1alpha1.WorkflowRun{}
	if err := p.KubeClient.Get(ctx, namespacedName, &run); err != nil {
		return err
	}
	return p.KubeClient.Delete(ctx, &run)
}

// GetContext will get a context
func (c contextServiceImpl) GetContext(ctx context.Context, projectName, pipelineName, name string) (*apis.Context, error) {
	modelCtx := model.PipelineContext{
		ProjectName:  projectName,
		PipelineName: pipelineName,
	}
	if err := c.Store.Get(ctx, &modelCtx); err != nil {
		return nil, err
	}
	vals, ok := modelCtx.Contexts[name]
	if !ok {
		return nil, errors.New("context not found")
	}
	return &apis.Context{Name: name, Values: vals}, nil
}

// CreateContext will create a context
func (c contextServiceImpl) CreateContext(ctx context.Context, projectName, pipelineName string, context apis.Context) (*model.PipelineContext, error) {
	modelCtx := model.PipelineContext{
		ProjectName:  projectName,
		PipelineName: pipelineName,
	}
	if err := c.Store.Get(ctx, &modelCtx); err != nil {
		return nil, err
	}
	if _, ok := modelCtx.Contexts[context.Name]; ok {
		log.Logger.Errorf("context %s already exists", context.Name)
		return nil, bcode.ErrContextAlreadyExist
	}
	modelCtx.Contexts[context.Name] = context.Values
	if err := c.Store.Put(ctx, &modelCtx); err != nil {
		return nil, err
	}
	return &modelCtx, nil
}

// UpdateContext will update a context
func (c contextServiceImpl) UpdateContext(ctx context.Context, projectName, pipelineName string, context apis.Context) (*model.PipelineContext, error) {
	modelCtx := model.PipelineContext{
		ProjectName:  projectName,
		PipelineName: pipelineName,
	}
	if err := c.Store.Get(ctx, &modelCtx); err != nil {
		return nil, err
	}
	modelCtx.Contexts[context.Name] = context.Values
	if err := c.Store.Put(ctx, &modelCtx); err != nil {
		return nil, err
	}
	return &modelCtx, nil
}

// ListContexts will list all contexts
func (c contextServiceImpl) ListContexts(ctx context.Context, projectName, pipelineName string) (*apis.ListContextValueResponse, error) {
	modelCtx := model.PipelineContext{
		ProjectName:  projectName,
		PipelineName: pipelineName,
	}
	if err := c.Store.Get(ctx, &modelCtx); err != nil {
		return nil, err
	}
	return &apis.ListContextValueResponse{
		Total:    len(modelCtx.Contexts),
		Contexts: modelCtx.Contexts,
	}, nil
}

// DeleteContext will delete a context
func (c contextServiceImpl) DeleteContext(ctx context.Context, projectName, pipelineName, name string) error {
	modelCtx := model.PipelineContext{
		ProjectName:  projectName,
		PipelineName: pipelineName,
	}
	if err := c.Store.Get(ctx, &modelCtx); err != nil {
		return err
	}
	delete(modelCtx.Contexts, name)
	if err := c.Store.Put(ctx, &modelCtx); err != nil {
		return err
	}
	return nil
}

func nsForProj(proj string) string {
	return fmt.Sprintf("project-%s", proj)
}

func getWfDescription(wf v1alpha1.Workflow) string {
	if wf.Labels == nil {
		return ""
	}
	return wf.Labels[labelDescription]
}

func getWfAlias(wf v1alpha1.Workflow) string {
	if wf.Labels == nil {
		return ""
	}
	return wf.Labels[labelAlias]
}

func fuzzyMatch(wf v1alpha1.Workflow, q string) bool {
	if strings.Contains(wf.Name, q) {
		return true
	}
	if strings.Contains(getWfAlias(wf), q) {
		return true
	}
	if strings.Contains(getWfDescription(wf), q) {
		return true
	}
	return false
}

func workflow2PipelineBase(wf v1alpha1.Workflow) *apis.PipelineBase {
	project := strings.TrimRight(wf.Namespace, "-project")
	return &apis.PipelineBase{
		PipelineMeta: apis.PipelineMeta{
			Name:        wf.Name,
			Project:     project,
			Description: getWfDescription(wf),
			Alias:       getWfAlias(wf),
		},
		Spec: wf.WorkflowSpec,
	}
}

func workflowRun2PipelineRun(run v1alpha1.WorkflowRun) apis.PipelineRun {
	return apis.PipelineRun{
		PipelineRunBase: apis.PipelineRunBase{
			PipelineRunMeta: apis.PipelineRunMeta{
				PipelineName:    run.Spec.WorkflowRef,
				Project:         strings.TrimRight(run.Namespace, "-project"),
				PipelineRunName: run.Name,
			},
		},
		Status: run.Status,
	}
}

func (p pipelineRunServiceImpl) workflowRun2runBriefing(ctx context.Context, run v1alpha1.WorkflowRun) apis.PipelineRunBriefing {
	contextName := run.Labels[labelContextName]
	project := strings.TrimRight(run.Namespace, "-project")
	apiContext, err := p.ContextService.GetContext(ctx, project, run.Spec.WorkflowRef, contextName)
	if err != nil {
		log.Logger.Warnf("failed to get pipeline run context %s/%s/%s: %v", project, run.Spec.WorkflowRef, contextName, err)
		apiContext = nil
	}

	return apis.PipelineRunBriefing{
		PipelineRunName: run.Name,
		Finished:        run.Status.Finished,
		Phase:           run.Status.Phase,
		Message:         run.Status.Message,
		StartTime:       run.Status.StartTime,
		EndTime:         run.Status.EndTime,
		ContextName:     apiContext.Name,
		ContextValues:   apiContext.Values,
	}
}
func (p pipelineRunServiceImpl) checkRecordRunning(ctx context.Context, pipelineRun apis.PipelineRunBase) (*v1alpha1.WorkflowRun, error) {
	run := v1alpha1.WorkflowRun{}
	if err := p.KubeClient.Get(ctx, types.NamespacedName{
		Namespace: nsForProj(pipelineRun.Project),
		Name:      pipelineRun.PipelineRunName,
	}, &run); err != nil {
		return nil, err
	}
	if !run.Status.Suspend && !run.Status.Terminated && !run.Status.Finished {
		return nil, fmt.Errorf("workflow is still running, can not operate a running workflow")
	}
	return &run, nil
}

func (p pipelineRunServiceImpl) terminatePipelineRun(ctx context.Context, run *v1alpha1.WorkflowRun) error {
	run.Status.Terminated = true
	run.Status.Suspend = false
	steps := run.Status.Steps
	for i, step := range steps {
		switch step.Phase {
		case v1alpha1.WorkflowStepPhaseFailed:
			if step.Reason != wfTypes.StatusReasonFailedAfterRetries && step.Reason != wfTypes.StatusReasonTimeout {
				steps[i].Reason = wfTypes.StatusReasonTerminate
			}
		case v1alpha1.WorkflowStepPhaseRunning:
			steps[i].Phase = v1alpha1.WorkflowStepPhaseFailed
			steps[i].Reason = wfTypes.StatusReasonTerminate
		default:
		}
		for j, sub := range step.SubStepsStatus {
			switch sub.Phase {
			case v1alpha1.WorkflowStepPhaseFailed:
				if sub.Reason != wfTypes.StatusReasonFailedAfterRetries && sub.Reason != wfTypes.StatusReasonTimeout {
					steps[i].SubStepsStatus[j].Phase = wfTypes.StatusReasonTerminate
				}
			case v1alpha1.WorkflowStepPhaseRunning:
				steps[i].SubStepsStatus[j].Phase = v1alpha1.WorkflowStepPhaseFailed
				steps[i].SubStepsStatus[j].Reason = wfTypes.StatusReasonTerminate
			default:
			}
		}
	}

	if err := p.KubeClient.Status().Patch(ctx, run, client.Merge); err != nil {
		return err
	}
	return nil

}
