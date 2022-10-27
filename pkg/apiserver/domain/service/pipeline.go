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
	"io"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kubevela/workflow/pkg/cue/model/value"

	types2 "github.com/oam-dev/kubevela/apis/types"
	pkgutils "github.com/oam-dev/kubevela/pkg/utils"
	querytypes "github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/kubevela/workflow/api/v1alpha1"
	wfTypes "github.com/kubevela/workflow/pkg/types"
	wfUtils "github.com/kubevela/workflow/pkg/utils"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/oam/util"

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

const (
	annotationDescription = "pipeline.oam.dev/description"
	annotationAlias       = "pipeline.oam.dev/alias"
	labelProject          = "pipeline.oam.dev/project"
	labelContext          = "pipeline.oam.dev/context"
	labelPipeline         = "pipeline.oam.dev/pipeline"
)

// PipelineService is the interface for pipeline service
type PipelineService interface {
	CreatePipeline(ctx context.Context, req apis.CreatePipelineRequest) (*apis.PipelineBase, error)
	ListPipelines(ctx context.Context, req apis.ListPipelineRequest) (*apis.ListPipelineResponse, error)
	GetPipeline(ctx context.Context, name string, getInfo bool) (*apis.GetPipelineResponse, error)
	UpdatePipeline(ctx context.Context, name string, req apis.UpdatePipelineRequest) (*apis.PipelineBase, error)
	DeletePipeline(ctx context.Context, base apis.PipelineBase) error
	RunPipeline(ctx context.Context, pipeline apis.PipelineBase, req apis.RunPipelineRequest) (*apis.PipelineRun, error)
}

type pipelineServiceImpl struct {
	ProjectService     ProjectService     `inject:""`
	ContextService     ContextService     `inject:""`
	KubeClient         client.Client      `inject:"kubeClient"`
	KubeConfig         *rest.Config       `inject:"kubeConfig"`
	Apply              apply.Applicator   `inject:"apply"`
	PipelineRunService PipelineRunService `inject:""`
}

// PipelineRunService is the interface for pipelineRun service
type PipelineRunService interface {
	GetPipelineRun(ctx context.Context, meta apis.PipelineRunMeta) (*apis.PipelineRun, error)
	ListPipelineRuns(ctx context.Context, base apis.PipelineBase) (apis.ListPipelineRunResponse, error)
	DeletePipelineRun(ctx context.Context, meta apis.PipelineRunMeta) error
	CleanPipelineRuns(ctx context.Context, base apis.PipelineBase) error
	StopPipelineRun(ctx context.Context, pipeline apis.PipelineRunBase) error
	GetPipelineRunOutput(ctx context.Context, meta apis.PipelineRun, step string) (apis.GetPipelineRunOutputResponse, error)
	GetPipelineRunLog(ctx context.Context, meta apis.PipelineRun, step string) (apis.GetPipelineRunLogResponse, error)
}

type pipelineRunServiceImpl struct {
	KubeClient     client.Client    `inject:"kubeClient"`
	KubeConfig     *rest.Config     `inject:"kubeConfig"`
	Apply          apply.Applicator `inject:"apply"`
	ContextService ContextService   `inject:""`
	ProjectService ProjectService   `inject:""`
}

// ContextService is the interface for context service
type ContextService interface {
	InitContext(ctx context.Context, projectName, pipelineName string) (*model.PipelineContext, error)
	GetContext(ctx context.Context, projectName, pipelineName string, name string) (*apis.Context, error)
	CreateContext(ctx context.Context, projectName, pipelineName string, context apis.Context) (*model.PipelineContext, error)
	UpdateContext(ctx context.Context, projectName, pipelineName string, context apis.Context) (*model.PipelineContext, error)
	ListContexts(ctx context.Context, projectName, pipelineName string) (*apis.ListContextValueResponse, error)
	DeleteContext(ctx context.Context, projectName, pipelineName, name string) error
	DeleteAllContexts(ctx context.Context, projectName, pipelineName string) error
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
	project := ctx.Value(&apis.CtxKeyProject).(*model.Project)
	wf := v1alpha1.Workflow{}
	wf.SetName(req.Name)
	wf.SetNamespace(project.GetNamespace())
	wf.WorkflowSpec = req.Spec
	wf.SetAnnotations(
		map[string]string{
			annotationDescription: req.Description,
			annotationAlias:       req.Alias,
		},
	)
	wf.SetLabels(map[string]string{
		model.LabelSourceOfTruth: model.FromUX,
		labelProject:             project.Name,
	})
	if err := p.KubeClient.Create(ctx, &wf); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil, bcode.ErrPipelineExist
		}
		return nil, err
	}
	return &apis.PipelineBase{
		PipelineMeta: apis.PipelineMeta{
			Name:  req.Name,
			Alias: req.Alias,
			Project: apis.NameAlias{
				Name:  project.Name,
				Alias: project.Alias,
			},
			Description: req.Description,
		},
		Spec: wf.WorkflowSpec,
	}, nil
}

// ListPipelines will list all pipelines
func (p pipelineServiceImpl) ListPipelines(ctx context.Context, req apis.ListPipelineRequest) (*apis.ListPipelineResponse, error) {
	wfs := v1alpha1.WorkflowList{}
	nsOption := make([]client.ListOption, 0)
	userName, ok := ctx.Value(&apis.CtxKeyUser).(string)
	if !ok {
		return nil, bcode.ErrUnauthorized
	}
	projects, err := p.ProjectService.ListUserProjects(ctx, userName)
	if err != nil {
		return nil, err
	}
	var availableProjectNames []string
	var projectNamespace = make(map[string]string, len(projects))
	var nsProjectMap = make(map[string]model.Project, len(projects))
	var namespaces []string
	for _, project := range projects {
		availableProjectNames = append(availableProjectNames, project.Name)
		// We only need name and alias of project
		nsProjectMap[project.Namespace] = model.Project{Name: project.Name, Alias: project.Alias}
		projectNamespace[project.Name] = project.Namespace
		if len(req.Projects) == 0 || pkgutils.StringsContain(req.Projects, project.Name) {
			namespaces = append(namespaces, project.Namespace)
		}

	}
	if len(availableProjectNames) == 0 || len(namespaces) == 0 {
		return &apis.ListPipelineResponse{}, nil
	}
	if len(namespaces) == 1 {
		nsOption = append(nsOption, client.InNamespace(projectNamespace[req.Projects[0]]))
	}
	if err := p.KubeClient.List(ctx, &wfs, nsOption...); err != nil {
		return nil, err
	}
	res := apis.ListPipelineResponse{}
	for _, wf := range wfs.Items {
		if !pkgutils.StringsContain(namespaces, wf.Namespace) {
			continue
		}
		if fuzzyMatch(wf, req.Query) {
			base := workflow2PipelineBase(wf, nsProjectMap[wf.Namespace])
			info, err := p.getPipelineInfo(wf)
			if err != nil {
				// Since we are listing pipelines. We should not return directly if we cannot get pipeline info
				log.Logger.Errorf("get pipeline %s/%s info error: %v", wf.Namespace, wf.Name, err)
				continue
			}
			item := apis.PipelineListItem{
				PipelineMeta: base.PipelineMeta,
				Info:         info,
			}
			res.Pipelines = append(res.Pipelines, item)
		}
	}
	return &res, nil
}

// GetPipeline will get a pipeline
func (p pipelineServiceImpl) GetPipeline(ctx context.Context, name string, getInfo bool) (*apis.GetPipelineResponse, error) {
	project := ctx.Value(&apis.CtxKeyProject).(*model.Project)
	wf := v1alpha1.Workflow{}
	if err := p.KubeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: project.GetNamespace()}, &wf); err != nil {
		return nil, err
	}
	base := workflow2PipelineBase(wf, *project)
	var info = apis.PipelineInfo{}
	var err error
	if getInfo {
		info, err = p.getPipelineInfo(wf)
		if err != nil {
			log.Logger.Errorf("get pipeline %s/%s info error: %v", wf.Namespace, wf.Name, err)
			return nil, bcode.ErrGetPipelineInfo
		}
	}

	return &apis.GetPipelineResponse{
		PipelineBase: *base,
		PipelineInfo: info,
	}, nil
}

// UpdatePipeline will update a pipeline
func (p pipelineServiceImpl) UpdatePipeline(ctx context.Context, name string, req apis.UpdatePipelineRequest) (*apis.PipelineBase, error) {
	project := ctx.Value(&apis.CtxKeyProject).(*model.Project)
	wf := v1alpha1.Workflow{}
	if err := p.KubeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: project.GetNamespace()}, &wf); err != nil {
		return nil, err
	}
	wf.WorkflowSpec = req.Spec
	if wf.Annotations == nil {
		wf.Annotations = map[string]string{}
	}
	wf.Annotations[annotationDescription] = req.Description
	wf.Annotations[annotationAlias] = req.Alias

	if err := p.KubeClient.Update(ctx, &wf); err != nil {
		return nil, err
	}
	return workflow2PipelineBase(wf, *project), nil
}

// DeletePipeline will delete a pipeline
func (p pipelineServiceImpl) DeletePipeline(ctx context.Context, pl apis.PipelineBase) error {
	project := ctx.Value(&apis.CtxKeyProject).(*model.Project)
	wf := v1alpha1.Workflow{}
	if err := p.KubeClient.Get(ctx, client.ObjectKey{Name: pl.Name, Namespace: project.GetNamespace()}, &wf); err != nil {
		return err
	}
	if err := p.KubeClient.Delete(ctx, &wf); err != nil {
		return err
	}
	if err := p.ContextService.DeleteAllContexts(ctx, pl.Project.Name, pl.Name); err != nil {
		log.Logger.Errorf("delete pipeline all context failure: %s", err.Error())
		return err
	}
	if err := p.PipelineRunService.CleanPipelineRuns(ctx, pl); err != nil {
		log.Logger.Errorf("delete pipeline all pipeline-runs failure: %s", err.Error())
		return err
	}
	return nil
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

func (p pipelineRunServiceImpl) GetPipelineRunOutput(ctx context.Context, pipelineRun apis.PipelineRun, step string) (apis.GetPipelineRunOutputResponse, error) {
	outputsSpec := make(map[string]v1alpha1.StepOutputs)
	stepOutputs := make([]apis.StepOutput, 0)
	if pipelineRun.Spec.WorkflowSpec != nil {
		for _, step := range pipelineRun.Spec.WorkflowSpec.Steps {
			if step.Outputs != nil {
				outputsSpec[step.Name] = step.Outputs
			}
			for _, sub := range step.SubSteps {
				if sub.Outputs != nil {
					outputsSpec[sub.Name] = sub.Outputs
				}
			}
		}
	}
	ctxBackend := pipelineRun.Status.ContextBackend
	if ctxBackend == nil {
		log.Logger.Errorf("context backend is nil")
		return apis.GetPipelineRunOutputResponse{}, bcode.ErrContextBackendNil
	}
	v, err := wfUtils.GetDataFromContext(ctx, p.KubeClient, ctxBackend.Name, pipelineRun.PipelineRunName, ctxBackend.Namespace)
	if err != nil {
		log.Logger.Errorf("get data from context backend failed: %v", err)
		return apis.GetPipelineRunOutputResponse{}, bcode.ErrGetContextBackendData
	}
	for _, s := range pipelineRun.Status.Steps {
		if step != "" && s.Name != step {
			continue
		}
		stepOutput := apis.StepOutput{
			Output:        getStepOutputs(s.StepStatus, outputsSpec, v),
			SubStepOutput: make([]apis.StepOutputBase, 0),
		}
		for _, sub := range s.SubStepsStatus {
			stepOutput.SubStepOutput = append(stepOutput.SubStepOutput, getStepOutputs(sub, outputsSpec, v))
		}
		stepOutputs = append(stepOutputs, stepOutput)
	}
	return apis.GetPipelineRunOutputResponse{StepOutput: stepOutputs}, nil
}

func (p pipelineRunServiceImpl) GetPipelineRunLog(ctx context.Context, pipelineRun apis.PipelineRun, step string) (apis.GetPipelineRunLogResponse, error) {
	project := ctx.Value(&apis.CtxKeyProject).(*model.Project)
	if pipelineRun.Status.ContextBackend == nil {
		return apis.GetPipelineRunLogResponse{}, bcode.ErrContextBackendNil
	}

	logConfig, err := wfUtils.GetLogConfigFromStep(ctx, p.KubeClient, pipelineRun.Status.ContextBackend.Name, pipelineRun.PipelineName, project.GetNamespace(), step)
	if err != nil {
		if strings.Contains(err.Error(), "no log config found") {
			return apis.GetPipelineRunLogResponse{
				StepBase: getStepBase(pipelineRun, step),
				Log:      "",
			}, nil
		}
		return apis.GetPipelineRunLogResponse{}, err
	}
	var logs string
	switch {
	case logConfig.Data:
		logs, err = getResourceLogs(ctx, p.KubeConfig, p.KubeClient, []wfTypes.Resource{{
			Namespace:     types2.DefaultKubeVelaNS,
			LabelSelector: map[string]string{"app.kubernetes.io/name": "vela-workflow"},
		}}, []string{fmt.Sprintf(`step_name="%s"`, step), fmt.Sprintf("%s/%s", project.GetNamespace(), pipelineRun.PipelineRunName), "cue logs"})
		if err != nil {
			return apis.GetPipelineRunLogResponse{}, err
		}
	case logConfig.Source != nil:
		if len(logConfig.Source.Resources) > 0 {
			logs, err = getResourceLogs(ctx, p.KubeConfig, p.KubeClient, logConfig.Source.Resources, nil)
			if err != nil {
				return apis.GetPipelineRunLogResponse{}, err
			}
		}
		if logConfig.Source.URL != "" {
			var logsBuilder strings.Builder
			readCloser, err := wfUtils.GetLogsFromURL(ctx, logConfig.Source.URL)
			if err != nil {
				log.Logger.Errorf("get logs from url %s failed: %v", logConfig.Source.URL, err)
				return apis.GetPipelineRunLogResponse{}, bcode.ErrReadSourceLog
			}
			//nolint:errcheck
			defer readCloser.Close()
			if _, err := io.Copy(&logsBuilder, readCloser); err != nil {
				log.Logger.Errorf("copy logs from url %s failed: %v", logConfig.Source.URL, err)
				return apis.GetPipelineRunLogResponse{}, bcode.ErrReadSourceLog
			}
			logs = logsBuilder.String()
		}
	}
	return apis.GetPipelineRunLogResponse{
		StepBase: getStepBase(pipelineRun, step),
		Log:      logs,
	}, nil
}

func getStepBase(run apis.PipelineRun, step string) apis.StepBase {
	for _, s := range run.Status.Steps {
		if s.Name == step {
			return apis.StepBase{
				ID:    s.ID,
				Name:  s.Name,
				Type:  s.Type,
				Phase: string(s.Phase),
			}
		}
	}
	return apis.StepBase{}
}

func getStepOutputs(step v1alpha1.StepStatus, outputsSpec map[string]v1alpha1.StepOutputs, v *value.Value) apis.StepOutputBase {
	o := apis.StepOutputBase{
		StepBase: apis.StepBase{
			Name:  step.Name,
			ID:    step.ID,
			Phase: string(step.Phase),
			Type:  step.Type,
		},
	}
	vars := make(map[string]string)
	for _, output := range outputsSpec[step.Name] {
		outputValue, err := v.LookupValue(output.Name)
		if err != nil {
			continue
		}
		s, err := outputValue.String()
		if err != nil {
			continue
		}
		vars[output.Name] = s
	}
	o.Vars = vars
	return o
}

func getResourceLogs(ctx context.Context, config *rest.Config, cli client.Client, resources []wfTypes.Resource, filters []string) (string, error) {
	pods, err := wfUtils.GetPodListFromResources(ctx, cli, resources)
	if err != nil {
		log.Logger.Errorf("fail to get pod list from resources: %v", err)
		return "", bcode.ErrFindingLogPods
	}
	podList := make([]*querytypes.PodBase, 0)
	for _, pod := range pods {
		podBase := &querytypes.PodBase{}
		podBase.Metadata.Name = pod.Name
		podBase.Metadata.Namespace = pod.Namespace
		podBase.Metadata.Labels = pod.Labels
		podList = append(podList, podBase)
	}
	logC := make(chan string, 1024)
	logCtx, cancel := context.WithCancel(ctx)
	var logs strings.Builder
	go func() {
		// No log sent in 2 seconds, stop getting log
		timer := time.AfterFunc(2*time.Second, func() {
			cancel()
		})
		for {
			select {
			case str := <-logC:
				timer.Reset(5 * time.Second)
				fmt.Println(str)
				show := true
				for _, filter := range filters {
					if !strings.Contains(str, filter) {
						show = false
						break
					}
				}
				if show {
					logs.WriteString(str)
				}
			case <-logCtx.Done():
				return
			}
		}
	}()

	// if there are multiple pod, watch them all.
	err = pkgutils.GetPodsLogs(logCtx, config, "", podList, "{{.PodName}}/{{.ContainerName}} {{.Message}}", logC)
	if err != nil {
		log.Logger.Errorf("Fail to get logs from pods: %v", err)
		return "", bcode.ErrGetPodsLogs
	}

	// Either logCtx or ctx is closed, return the logs collected
	select {
	case <-logCtx.Done():
		return logs.String(), nil
	case <-ctx.Done():
		return logs.String(), nil
	}
}

// RunPipeline will run a pipeline
func (p pipelineServiceImpl) RunPipeline(ctx context.Context, pipeline apis.PipelineBase, req apis.RunPipelineRequest) (*apis.PipelineRun, error) {
	project := ctx.Value(&apis.CtxKeyProject).(*model.Project)
	run := v1alpha1.WorkflowRun{}
	version := utils.GenerateVersion("")
	name := fmt.Sprintf("%s-%s", pipeline.Name, version)
	run.Name = name
	run.Namespace = project.GetNamespace()
	run.Spec.WorkflowRef = pipeline.Name
	run.Spec.Mode = &req.Mode

	// process the context
	if req.ContextName != "" {
		ppContext, err := p.ContextService.GetContext(ctx, pipeline.Project.Name, pipeline.Name, req.ContextName)
		if err != nil {
			return nil, err
		}
		contextData := make(map[string]interface{})
		for _, pair := range ppContext.Values {
			contextData[pair.Key] = pair.Value
		}
		run.SetLabels(map[string]string{
			labelContext:  req.ContextName,
			labelPipeline: pipeline.Name,
		})
		run.Spec.Context = util.Object2RawExtension(contextData)
	}

	if err := p.KubeClient.Create(ctx, &run); err != nil {
		return nil, err
	}
	return p.PipelineRunService.GetPipelineRun(ctx, apis.PipelineRunMeta{
		PipelineName:    pipeline.Name,
		Project:         apis.NameAlias{Name: project.Name},
		PipelineRunName: name,
	})
}

// getPipelineInfo returns the pipeline statistic info
// return error can be nil if pipeline hasn't been run
func (p pipelineServiceImpl) getPipelineInfo(wf v1alpha1.Workflow) (apis.PipelineInfo, error) {
	var wfrs v1alpha1.WorkflowRunList
	err := p.KubeClient.List(context.Background(), &wfrs, client.InNamespace(wf.Namespace), client.MatchingLabels(map[string]string{labelPipeline: wf.Name}))
	if err != nil {
		return apis.PipelineInfo{}, err
	}
	if wfrs.Len() == 0 {
		return apis.PipelineInfo{}, nil
	}
	wfr := getLastRun(wfrs.Items)
	// todo related apps and runstats
	return apis.PipelineInfo{
		RelatedApps: nil,
		LastRun:     wfr.Status,
		RunStat:     apis.RunStat{},
	}, nil
}

func getLastRun(wfrs []v1alpha1.WorkflowRun) *v1alpha1.WorkflowRun {
	last := wfrs[0]
	lastStartTime := last.Status.StartTime.Time
	for _, wfr := range wfrs {
		wfr.Status.StartTime.After(lastStartTime)
		last = wfr
		lastStartTime = wfr.Status.StartTime.Time
	}
	return &last
}

// GetPipelineRun will get a pipeline run
func (p pipelineRunServiceImpl) GetPipelineRun(ctx context.Context, meta apis.PipelineRunMeta) (*apis.PipelineRun, error) {
	project := ctx.Value(&apis.CtxKeyProject).(*model.Project)
	namespacedName := client.ObjectKey{Name: meta.PipelineRunName, Namespace: project.GetNamespace()}
	run := v1alpha1.WorkflowRun{}
	if err := p.KubeClient.Get(ctx, namespacedName, &run); err != nil {
		return nil, err
	}
	if run.Spec.WorkflowRef != "" {
		var workflow v1alpha1.Workflow
		if err := p.KubeClient.Get(ctx, client.ObjectKey{Name: run.Spec.WorkflowRef, Namespace: project.GetNamespace()}, &workflow); err != nil {
			log.Logger.Errorf("failed to load the workflow %s", err.Error())
		} else {
			run.Spec.WorkflowSpec = &workflow.WorkflowSpec
		}
	}
	return workflowRun2PipelineRun(run, project), nil
}

// ListPipelineRuns will list all pipeline runs
func (p pipelineRunServiceImpl) ListPipelineRuns(ctx context.Context, base apis.PipelineBase) (apis.ListPipelineRunResponse, error) {
	project := ctx.Value(&apis.CtxKeyProject).(*model.Project)
	wfrs := v1alpha1.WorkflowRunList{}
	if err := p.KubeClient.List(ctx, &wfrs, client.InNamespace(project.GetNamespace())); err != nil {
		return apis.ListPipelineRunResponse{}, err
	}
	res := apis.ListPipelineRunResponse{
		Runs: make([]apis.PipelineRunBriefing, 0),
	}
	for _, wfr := range wfrs.Items {
		if wfr.Spec.WorkflowRef == base.Name {
			res.Runs = append(res.Runs, p.workflowRun2runBriefing(ctx, wfr))
		}
	}
	res.Total = int64(len(res.Runs))
	return res, nil
}

// DeletePipelineRun will delete a pipeline run
func (p pipelineRunServiceImpl) DeletePipelineRun(ctx context.Context, meta apis.PipelineRunMeta) error {
	project := ctx.Value(&apis.CtxKeyProject).(*model.Project)
	run := v1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      meta.PipelineRunName,
			Namespace: project.GetNamespace(),
		},
	}
	err := p.KubeClient.Delete(ctx, &run)
	return client.IgnoreNotFound(err)
}

// CleanPipelineRuns will clean all pipeline runs, it equals to call ListPipelineRuns and multiple DeletePipelineRun
func (p pipelineRunServiceImpl) CleanPipelineRuns(ctx context.Context, base apis.PipelineBase) error {
	project := ctx.Value(&apis.CtxKeyProject).(*model.Project)
	wfrs := v1alpha1.WorkflowRunList{}
	if err := p.KubeClient.List(ctx, &wfrs, client.InNamespace(project.GetNamespace())); err != nil {
		return err
	}
	for _, wfr := range wfrs.Items {
		if wfr.Spec.WorkflowRef == base.Name {
			if err := p.KubeClient.Delete(ctx, wfr.DeepCopy()); err != nil {
				return client.IgnoreNotFound(err)
			}
		}
	}
	return nil
}

// InitContext will init pipeline context record
func (c contextServiceImpl) InitContext(ctx context.Context, projectName, pipelineName string) (*model.PipelineContext, error) {
	modelCtx := model.PipelineContext{
		ProjectName:  projectName,
		PipelineName: pipelineName,
	}
	if err := c.Store.Get(ctx, &modelCtx); err == nil {
		return nil, bcode.ErrContextAlreadyExist
	}
	modelCtx.Contexts = make(map[string][]model.Value)
	if err := c.Store.Add(ctx, &modelCtx); err != nil {
		return nil, err
	}
	return &modelCtx, nil
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
		return nil, bcode.ErrContextNotFound
	}
	return &apis.Context{Name: name, Values: vals}, nil
}

// CreateContext will create a context
func (c contextServiceImpl) CreateContext(ctx context.Context, projectName, pipelineName string, context apis.Context) (*model.PipelineContext, error) {
	modelCtx := &model.PipelineContext{
		ProjectName:  projectName,
		PipelineName: pipelineName,
	}
	if err := c.Store.Get(ctx, modelCtx); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			modelCtx, err = c.InitContext(ctx, projectName, pipelineName)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	if _, ok := modelCtx.Contexts[context.Name]; ok {
		log.Logger.Errorf("context %s already exists", context.Name)
		return nil, bcode.ErrContextAlreadyExist
	}
	modelCtx.Contexts[context.Name] = context.Values
	if err := c.Store.Put(ctx, modelCtx); err != nil {
		return nil, err
	}
	return modelCtx, nil
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
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return &apis.ListContextValueResponse{
				Total:    0,
				Contexts: make(map[string][]model.Value),
			}, nil
		}
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
	return c.Store.Put(ctx, &modelCtx)
}

// DeleteAllContexts will delete all contexts of a pipeline
func (c contextServiceImpl) DeleteAllContexts(ctx context.Context, projectName, pipelineName string) error {
	modelCtx := model.PipelineContext{
		ProjectName:  projectName,
		PipelineName: pipelineName,
	}
	return c.Store.Delete(ctx, &modelCtx)
}

func getWfDescription(wf v1alpha1.Workflow) string {
	if wf.Annotations == nil {
		return ""
	}
	return wf.Annotations[annotationDescription]
}

func getWfAlias(wf v1alpha1.Workflow) string {
	if wf.Annotations == nil {
		return ""
	}
	return wf.Annotations[annotationAlias]
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

func workflow2PipelineBase(wf v1alpha1.Workflow, project model.Project) *apis.PipelineBase {
	return &apis.PipelineBase{
		PipelineMeta: apis.PipelineMeta{
			Name: wf.Name,
			Project: apis.NameAlias{
				Name:  project.Name,
				Alias: project.Alias,
			},
			Description: getWfDescription(wf),
			Alias:       getWfAlias(wf),
			CreateTime:  wf.CreationTimestamp.Time,
		},
		Spec: wf.WorkflowSpec,
	}
}

func workflowRun2PipelineRun(run v1alpha1.WorkflowRun, project *model.Project) *apis.PipelineRun {
	mergeSteps(&run)
	return &apis.PipelineRun{
		PipelineRunBase: apis.PipelineRunBase{
			PipelineRunMeta: apis.PipelineRunMeta{
				PipelineName: run.Spec.WorkflowRef,
				Project: apis.NameAlias{
					Name:  project.Name,
					Alias: project.Alias,
				},
				PipelineRunName: run.Name,
			},
			ContextName: run.GetLabels()[labelContext],
			Spec:        run.Spec,
		},
		Status: run.Status,
	}

}

func mergeSteps(run *v1alpha1.WorkflowRun) {
	if run.Spec.WorkflowSpec == nil {
		return
	}
	if run.Status.Steps == nil {
		run.Status.Steps = make([]v1alpha1.WorkflowStepStatus, 0)
	}
	var stepStatus = make(map[string]*v1alpha1.WorkflowStepStatus, len(run.Status.Steps))
	for i, step := range run.Status.Steps {
		stepStatus[step.Name] = &run.Status.Steps[i]
	}
	for _, step := range run.Spec.WorkflowSpec.Steps {
		if stepStatusCache, exist := stepStatus[step.Name]; !exist {
			var subSteps []v1alpha1.StepStatus
			for _, subStep := range step.SubSteps {
				subSteps = append(subSteps, v1alpha1.StepStatus{
					Name:  subStep.Name,
					Type:  subStep.Type,
					Phase: v1alpha1.WorkflowStepPhasePending,
				})
			}
			run.Status.Steps = append(run.Status.Steps, v1alpha1.WorkflowStepStatus{
				StepStatus: v1alpha1.StepStatus{
					Name:  step.Name,
					Type:  step.Type,
					Phase: v1alpha1.WorkflowStepPhasePending,
				},
				SubStepsStatus: subSteps,
			})
		} else if len(step.SubSteps) > len(stepStatusCache.SubStepsStatus) {
			var subStepStatus = make(map[string]v1alpha1.StepStatus, len(stepStatusCache.SubStepsStatus))
			for i, step := range stepStatusCache.SubStepsStatus {
				subStepStatus[step.Name] = stepStatusCache.SubStepsStatus[i]
			}
			for _, subStep := range step.SubSteps {
				if _, exist := subStepStatus[subStep.Name]; !exist {
					stepStatusCache.SubStepsStatus = append(stepStatusCache.SubStepsStatus, v1alpha1.StepStatus{
						Name:  subStep.Name,
						Type:  subStep.Type,
						Phase: v1alpha1.WorkflowStepPhasePending,
					})
				}
			}
		}
	}
}

func (p pipelineRunServiceImpl) workflowRun2runBriefing(ctx context.Context, run v1alpha1.WorkflowRun) apis.PipelineRunBriefing {
	project := strings.TrimPrefix(run.Namespace, "project-")
	var (
		apiContext *apis.Context
		err        error
	)
	if contextName, ok := run.Labels[labelContext]; ok {
		apiContext, err = p.ContextService.GetContext(ctx, project, run.Spec.WorkflowRef, contextName)
		if err != nil {
			log.Logger.Warnf("failed to get pipeline run context %s/%s/%s: %v", project, run.Spec.WorkflowRef, contextName, err)
			apiContext = nil
		}
	}

	briefing := apis.PipelineRunBriefing{
		PipelineRunName: run.Name,
		Finished:        run.Status.Finished,
		Phase:           run.Status.Phase,
		Message:         run.Status.Message,
		StartTime:       run.Status.StartTime,
		EndTime:         run.Status.EndTime,
	}
	if apiContext != nil {
		briefing.ContextName = apiContext.Name
		briefing.ContextValues = apiContext.Values
	}
	return briefing
}
func (p pipelineRunServiceImpl) checkRecordRunning(ctx context.Context, pipelineRun apis.PipelineRunBase) (*v1alpha1.WorkflowRun, error) {
	project := ctx.Value(&apis.CtxKeyProject).(*model.Project)
	run := v1alpha1.WorkflowRun{}
	if err := p.KubeClient.Get(ctx, types.NamespacedName{
		Namespace: project.GetNamespace(),
		Name:      pipelineRun.PipelineRunName,
	}, &run); err != nil {
		return nil, err
	}
	if !run.Status.Suspend && !run.Status.Terminated && !run.Status.Finished {
		return nil, bcode.ErrPipelineRunStillRunning
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
