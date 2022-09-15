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
	"github.com/kubevela/workflow/api/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"

	apis "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
)

const (
	labelDescription = "pipeline.velaux.oam.dev/description"
	labelAlias       = "pipeline.velaux.oam.dev/alias"
)

type PipelineService interface {
	CreatePipeline(ctx context.Context, req apis.CreatePipelineRequest) (*apis.PipelineBase, error)
	ListPipelines(ctx context.Context, req apis.ListPipelineRequest) (*apis.ListPipelineResponse, error)
	GetPipeline(ctx context.Context, name, project string) (*apis.GetPipelineResponse, error)
	UpdatePipeline(ctx context.Context, name, project string, req apis.UpdatePipelineRequest) (*apis.PipelineBase, error)
	DeletePipeline(ctx context.Context, base apis.PipelineBase) error
	RunPipeline(ctx context.Context, pipeline apis.PipelineBase, req apis.RunPipelineRequest) error
}

type pipelineServiceImpl struct {
	KubeClient client.Client    `inject:"kubeClient"`
	KubeConfig *rest.Config     `inject:"kubeConfig"`
	Apply      apply.Applicator `inject:"apply"`
}

type PipelineRunService interface {
	GetPipelineRun(ctx context.Context, meta apis.PipelineRunMeta) (apis.PipelineRun, error)
	ListPipelineRuns(ctx context.Context, base apis.PipelineBase) (apis.ListPipelineRunResponse, error)
	DeletePipelineRun(ctx context.Context, meta apis.PipelineRunMeta) error
	StopPipeline(ctx context.Context, pipeline apis.PipelineRunBase) error
}

type pipelineRunServiceImpl struct {
	KubeClient client.Client    `inject:"kubeClient"`
	KubeConfig *rest.Config     `inject:"kubeConfig"`
	Apply      apply.Applicator `inject:"apply"`
}

type ContextService interface {
	CreateContext(ctx context.Context, projectName, pipelineName string, context apis.Context) (*model.PipelineContext, error)
	UpdateContext(ctx context.Context, projectName, pipelineName string, context apis.Context) (*model.PipelineContext, error)
	ListContexts(ctx context.Context, projectName, pipelineName string) (*apis.ListContextValueResponse, error)
	DeleteContext(ctx context.Context, projectName, pipelineName, name string) error
}

type contextServiceImpl struct {
	Store datastore.DataStore `inject:"datastore"`
}

// NewPipelineService new application service
func NewPipelineService() PipelineService {
	return &pipelineServiceImpl{}
}

func NewPipelineRunService() PipelineRunService {
	return &pipelineRunServiceImpl{}
}

func NewContextService() ContextService {
	return &contextServiceImpl{}
}

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

func (p pipelineServiceImpl) ListPipelines(ctx context.Context, req apis.ListPipelineRequest) (*apis.ListPipelineResponse, error) {
	wfs := v1alpha1.WorkflowList{}
	nsOption := make([]client.ListOption, len(req.Projects))
	for _, ns := range req.Projects {
		nsOption = append(nsOption, client.InNamespace(nsForProj(ns)))
	}
	if err := p.KubeClient.List(ctx, &wfs, nsOption...); err != nil {
		return nil, err
	}
	res := apis.ListPipelineResponse{}
	for _, wf := range wfs.Items {
		if fuzzyMatch(&wf, req.Query) {
			item := apis.PipelineListItem{
				PipelineMeta: workflow2PipelineBase(&wf).PipelineMeta,
				// todo info
			}
			res.Pipelines = append(res.Pipelines, item)
		}
	}
	return &res, nil
}

func (p pipelineServiceImpl) GetPipeline(ctx context.Context, name, project string) (*apis.GetPipelineResponse, error) {
	wf := v1alpha1.Workflow{}
	if err := p.KubeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: nsForProj(project)}, &wf); err != nil {
		return nil, err
	}
	return &apis.GetPipelineResponse{
		PipelineBase: *workflow2PipelineBase(&wf),
		// todo info
	}, nil
}

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
	return workflow2PipelineBase(&wf), nil
}

func (p pipelineServiceImpl) DeletePipeline(ctx context.Context, pl apis.PipelineBase) error {
	wf := v1alpha1.Workflow{}
	if err := p.KubeClient.Get(ctx, client.ObjectKey{Name: pl.Name, Namespace: nsForProj(pl.Project)}, &wf); err != nil {
		return err
	}
	return p.KubeClient.Delete(ctx, &wf)
}

func (p pipelineRunServiceImpl) StopPipeline(ctx context.Context, pipelineRun apis.PipelineRunBase) error {
	//TODO implement me
	panic("implement me")
}

func (p pipelineServiceImpl) RunPipeline(ctx context.Context, pipeline apis.PipelineBase, req apis.RunPipelineRequest) error {
	//TODO implement me
	panic("implement me")
}

func (p pipelineRunServiceImpl) GetPipelineRun(ctx context.Context, meta apis.PipelineRunMeta) (apis.PipelineRun, error) {
	//TODO implement me
	panic("implement me")
}

func (p pipelineRunServiceImpl) ListPipelineRuns(ctx context.Context, base apis.PipelineBase) (apis.ListPipelineRunResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (p pipelineRunServiceImpl) DeletePipelineRun(ctx context.Context, meta apis.PipelineRunMeta) error {
	//TODO implement me
	panic("implement me")
}

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

func getWfDescription(wf *v1alpha1.Workflow) string {
	if wf.Labels == nil {
		return ""
	}
	return wf.Labels[labelDescription]
}

func getWfAlias(wf *v1alpha1.Workflow) string {
	if wf.Labels == nil {
		return ""
	}
	return wf.Labels[labelAlias]
}

func fuzzyMatch(wf *v1alpha1.Workflow, q string) bool {
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

func workflow2PipelineBase(wf *v1alpha1.Workflow) *apis.PipelineBase {
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
