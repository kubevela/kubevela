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
	"bufio"
	"bytes"
	"context"
	"fmt"
	"hash/fnv"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"

	"github.com/kubevela/workflow/api/v1alpha1"
	"github.com/kubevela/workflow/pkg/cue/model/value"
	wfTypes "github.com/kubevela/workflow/pkg/types"
	wfUtils "github.com/kubevela/workflow/pkg/utils"
	"github.com/modern-go/concurrent"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	types2 "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	pkgutils "github.com/oam-dev/kubevela/pkg/utils"
)

const (
	labelContext  = "pipeline.oam.dev/context"
	labelPipeline = "pipeline.oam.dev/name"
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
	Store              datastore.DataStore `inject:"datastore"`
	ProjectService     ProjectService      `inject:""`
	ContextService     ContextService      `inject:""`
	KubeClient         client.Client       `inject:"kubeClient"`
	KubeConfig         *rest.Config        `inject:"kubeConfig"`
	PipelineRunService PipelineRunService  `inject:""`
}

// PipelineRunService is the interface for pipelineRun service
type PipelineRunService interface {
	GetPipelineRun(ctx context.Context, meta apis.PipelineRunMeta) (*apis.PipelineRun, error)
	ListPipelineRuns(ctx context.Context, base apis.PipelineBase) (apis.ListPipelineRunResponse, error)
	DeletePipelineRun(ctx context.Context, meta apis.PipelineRunMeta) error
	CleanPipelineRuns(ctx context.Context, base apis.PipelineBase) error
	StopPipelineRun(ctx context.Context, pipeline apis.PipelineRunBase) error
	GetPipelineRunOutput(ctx context.Context, meta apis.PipelineRun, step string) (apis.GetPipelineRunOutputResponse, error)
	GetPipelineRunInput(ctx context.Context, meta apis.PipelineRun, step string) (apis.GetPipelineRunInputResponse, error)
	GetPipelineRunLog(ctx context.Context, meta apis.PipelineRun, step string) (apis.GetPipelineRunLogResponse, error)
}

type pipelineRunServiceImpl struct {
	Store          datastore.DataStore `inject:"datastore"`
	KubeClient     client.Client       `inject:"kubeClient"`
	KubeConfig     *rest.Config        `inject:"kubeConfig"`
	ContextService ContextService      `inject:""`
	ProjectService ProjectService      `inject:""`
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
	if err := checkPipelineSpec(req.Spec); err != nil {
		return nil, err
	}
	pipeline := &model.Pipeline{
		Name:        req.Name,
		Description: req.Description,
		Alias:       req.Alias,
		Project:     project.Name,
		Spec:        req.Spec,
	}
	if err := p.Store.Add(ctx, pipeline); err != nil {
		if errors.Is(err, datastore.ErrRecordExist) {
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
		Spec: pipeline.Spec,
	}, nil
}

// ListPipelines will list all pipelines
func (p pipelineServiceImpl) ListPipelines(ctx context.Context, req apis.ListPipelineRequest) (*apis.ListPipelineResponse, error) {
	userName, ok := ctx.Value(&apis.CtxKeyUser).(string)
	if !ok {
		return nil, bcode.ErrUnauthorized
	}
	projects, err := p.ProjectService.ListUserProjects(ctx, userName)
	if err != nil {
		return nil, err
	}
	var availableProjectNames []string
	var projectMap = make(map[string]model.Project, len(projects))
	for _, project := range projects {
		availableProjectNames = append(availableProjectNames, project.Name)
		// We only need name, namespace and alias of project
		projectMap[project.Name] = model.Project{Name: project.Name, Alias: project.Alias, Namespace: project.Namespace}

	}
	if len(availableProjectNames) == 0 {
		return &apis.ListPipelineResponse{}, nil
	}
	pp := model.Pipeline{}
	pipelines, err := p.Store.List(ctx, &pp, &datastore.ListOptions{
		FilterOptions: datastore.FilterOptions{
			In: []datastore.InQueryOption{
				{
					Key:    "project",
					Values: availableProjectNames,
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	res := apis.ListPipelineResponse{}
	for _, _p := range pipelines {
		pipeline := _p.(*model.Pipeline)
		if !pkgutils.StringsContain(availableProjectNames, pipeline.Project) {
			continue
		}
		if fuzzyMatch(pipeline, req.Query) {
			base := pipeline2PipelineBase(pipeline, projectMap[pipeline.Project])
			var info *apis.PipelineInfo
			if req.Detailed {
				info, err = p.getPipelineInfo(ctx, pipeline, projectMap[pipeline.Project].Namespace)
				if err != nil {
					// Since we are listing pipelines. We should not return directly if we cannot get pipeline info
					log.Logger.Errorf("get pipeline %s/%s info error: %v", pipeline.Project, pipeline.Name, err)
					continue
				}
			}
			item := apis.PipelineListItem{
				PipelineMeta: base.PipelineMeta,
				Info: func() apis.PipelineInfo {
					if info != nil {
						return *info
					}
					return apis.PipelineInfo{}
				}(),
			}
			res.Pipelines = append(res.Pipelines, item)
		}
	}
	res.Total = len(res.Pipelines)
	return &res, nil
}

// GetPipeline will get a pipeline
func (p pipelineServiceImpl) GetPipeline(ctx context.Context, name string, getInfo bool) (*apis.GetPipelineResponse, error) {
	project := ctx.Value(&apis.CtxKeyProject).(*model.Project)
	pipeline := &model.Pipeline{
		Name:    name,
		Project: project.Name,
	}
	if err := p.Store.Get(ctx, pipeline); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, bcode.ErrPipelineNotExist
		}
		return nil, err
	}
	base := pipeline2PipelineBase(pipeline, *project)
	var info = apis.PipelineInfo{}
	if getInfo {
		in, err := p.getPipelineInfo(ctx, pipeline, project.Namespace)
		if err != nil {
			log.Logger.Errorf("get pipeline %s/%s info error: %v", pipeline.Project, pipeline.Name, err)
			return nil, bcode.ErrGetPipelineInfo
		}
		if in != nil {
			info = *in
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
	if err := checkPipelineSpec(req.Spec); err != nil {
		return nil, err
	}
	pipeline := &model.Pipeline{
		Name:    name,
		Project: project.Name,
	}
	if err := p.Store.Get(ctx, pipeline); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, bcode.ErrPipelineNotExist
		}
		return nil, err
	}

	pipeline.Spec = req.Spec
	pipeline.Description = req.Description
	pipeline.Alias = req.Alias

	if err := p.Store.Put(ctx, pipeline); err != nil {
		return nil, err
	}
	return pipeline2PipelineBase(pipeline, *project), nil
}

// DeletePipeline will delete a pipeline
func (p pipelineServiceImpl) DeletePipeline(ctx context.Context, pl apis.PipelineBase) error {
	project := ctx.Value(&apis.CtxKeyProject).(*model.Project)
	pipeline := &model.Pipeline{
		Name:    pl.Name,
		Project: project.Name,
	}
	if err := p.Store.Get(ctx, pipeline); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return bcode.ErrPipelineNotExist
		}
		return err
	}
	// Clean up pipeline: 1. delete pipeline runs 2. delete contexts 3. delete pipeline
	if err := p.PipelineRunService.CleanPipelineRuns(ctx, pl); err != nil {
		log.Logger.Errorf("delete pipeline all pipeline-runs failure: %s", err.Error())
		return err
	}
	if err := p.ContextService.DeleteAllContexts(ctx, pl.Project.Name, pl.Name); err != nil {
		log.Logger.Errorf("delete pipeline all context failure: %s", err.Error())
		return err
	}
	if err := p.Store.Delete(ctx, pipeline); err != nil {
		return err
	}

	return nil
}

// StopPipelineRun will stop a pipelineRun
func (p pipelineRunServiceImpl) StopPipelineRun(ctx context.Context, pipelineRun apis.PipelineRunBase) error {
	run, err := p.checkRunNotFinished(ctx, pipelineRun)
	if err != nil {
		return err
	}
	if err := p.terminatePipelineRun(ctx, run); err != nil {
		return err
	}
	return nil
}

func (p pipelineRunServiceImpl) GetPipelineRunOutput(ctx context.Context, pipelineRun apis.PipelineRun, stepName string) (apis.GetPipelineRunOutputResponse, error) {
	outputsSpec := make(map[string]v1alpha1.StepOutputs)
	stepOutputs := make([]apis.StepOutputBase, 0)
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
		return apis.GetPipelineRunOutputResponse{}, nil
	}
	v, err := wfUtils.GetDataFromContext(ctx, p.KubeClient, ctxBackend.Name, pipelineRun.PipelineRunName, ctxBackend.Namespace)
	if err != nil {
		log.Logger.Errorf("get data from context backend failed: %v", err)
		return apis.GetPipelineRunOutputResponse{}, bcode.ErrGetContextBackendData
	}
	for _, s := range pipelineRun.Status.Steps {
		var (
			subStepStatus *v1alpha1.StepStatus
			ok            bool
		)
		if stepName != "" && s.Name != stepName {
			subStepStatus, ok = haveSubSteps(s, stepName)
			if !ok {
				continue
			}
			subVars := getStepOutputs(*subStepStatus, outputsSpec, v)
			stepOutputs = append(stepOutputs, subVars)
			break
		}
		stepOutputs = append(stepOutputs, getStepOutputs(s.StepStatus, outputsSpec, v))
		for _, sub := range s.SubStepsStatus {
			stepOutputs = append(stepOutputs, getStepOutputs(sub, outputsSpec, v))
		}
		if stepName != "" && s.Name == stepName {
			// already found the step
			break
		}
	}
	return apis.GetPipelineRunOutputResponse{StepOutputs: stepOutputs}, nil
}

func (p pipelineRunServiceImpl) GetPipelineRunInput(ctx context.Context, pipelineRun apis.PipelineRun, stepName string) (apis.GetPipelineRunInputResponse, error) {
	// valueFromStep know which step the value came from
	valueFromStep := make(map[string]string)
	inputsSpec := make(map[string]v1alpha1.StepInputs)
	stepInputs := make([]apis.StepInputBase, 0)
	if pipelineRun.Spec.WorkflowSpec != nil {
		for _, step := range pipelineRun.Spec.WorkflowSpec.Steps {
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
	}
	ctxBackend := pipelineRun.Status.ContextBackend
	if ctxBackend == nil {
		return apis.GetPipelineRunInputResponse{}, nil
	}
	v, err := wfUtils.GetDataFromContext(ctx, p.KubeClient, ctxBackend.Name, pipelineRun.PipelineRunName, ctxBackend.Namespace)
	if err != nil {
		log.Logger.Errorf("get data from context backend failed: %v", err)
		return apis.GetPipelineRunInputResponse{}, bcode.ErrGetContextBackendData
	}
	for _, s := range pipelineRun.Status.Steps {
		var (
			subStepStatus *v1alpha1.StepStatus
			ok            bool
		)
		if stepName != "" && s.Name != stepName {
			subStepStatus, ok = haveSubSteps(s, stepName)
			if !ok {
				continue
			}
			subVars := getStepInputs(*subStepStatus, inputsSpec, v, valueFromStep)
			stepInputs = append(stepInputs, subVars)
			break
		}
		stepInputs = append(stepInputs, getStepInputs(s.StepStatus, inputsSpec, v, valueFromStep))
		for _, sub := range s.SubStepsStatus {
			stepInputs = append(stepInputs, getStepInputs(sub, inputsSpec, v, valueFromStep))
		}
		if stepName != "" && s.Name == stepName {
			// already found the step
			break
		}
	}
	return apis.GetPipelineRunInputResponse{StepInputs: stepInputs}, nil
}

func haveSubSteps(step v1alpha1.WorkflowStepStatus, subStep string) (*v1alpha1.StepStatus, bool) {
	for _, s := range step.SubStepsStatus {
		if s.Name == subStep {
			return &s, true
		}
	}
	return nil, false
}

// Copied from stern/stern
var colorList = [][2]*color.Color{
	{color.New(color.FgHiCyan), color.New(color.FgCyan)},
	{color.New(color.FgHiGreen), color.New(color.FgGreen)},
	{color.New(color.FgHiMagenta), color.New(color.FgMagenta)},
	{color.New(color.FgHiYellow), color.New(color.FgYellow)},
	{color.New(color.FgHiBlue), color.New(color.FgBlue)},
	{color.New(color.FgHiRed), color.New(color.FgRed)},
}

func determineColor(podName string) (podColor, containerColor *color.Color) {
	hash := fnv.New32()
	hash.Write([]byte(podName))
	idx := hash.Sum32() % uint32(len(colorList))

	colors := colorList[idx]
	return colors[0], colors[1]
}

func (p pipelineRunServiceImpl) GetPipelineRunLog(ctx context.Context, pipelineRun apis.PipelineRun, step string) (apis.GetPipelineRunLogResponse, error) {
	project := ctx.Value(&apis.CtxKeyProject).(*model.Project)
	if pipelineRun.Status.ContextBackend == nil {
		return apis.GetPipelineRunLogResponse{}, nil
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
	values := make([]apis.OutputVar, 0)
	for _, output := range outputsSpec[step.Name] {
		outputValue, err := v.LookupValue(output.Name)
		if err != nil {
			continue
		}
		s, err := outputValue.String()
		if err != nil {
			continue
		}
		values = append(values, apis.OutputVar{
			Name:      output.Name,
			ValueFrom: output.ValueFrom,
			Value:     s,
		})
	}
	o.Values = values
	return o
}

func getStepInputs(step v1alpha1.StepStatus, inputsSpec map[string]v1alpha1.StepInputs, v *value.Value, valueFromStep map[string]string) apis.StepInputBase {
	o := apis.StepInputBase{
		StepBase: apis.StepBase{
			Name:  step.Name,
			ID:    step.ID,
			Phase: string(step.Phase),
			Type:  step.Type,
		},
	}
	values := make([]apis.InputVar, 0)
	for _, input := range inputsSpec[step.Name] {
		outputValue, err := v.LookupValue(input.From)
		if err != nil {
			continue
		}
		s, err := outputValue.String()
		if err != nil {
			continue
		}
		values = append(values, apis.InputVar{
			Value:        s,
			From:         input.From,
			FromStep:     valueFromStep[input.From],
			ParameterKey: input.ParameterKey,
		})
	}
	o.Values = values
	return o
}

func getResourceLogs(ctx context.Context, config *rest.Config, cli client.Client, resources []wfTypes.Resource, filters []string) (string, error) {
	var (
		linesOfLogKept int64 = 1000
		timeout              = 10 * time.Second
		errPrint             = color.New(color.FgRed, color.Bold).FprintfFunc()
	)

	type PodContainer struct {
		Name      string
		Namespace string
		Container string
		Label     map[string]string
	}
	pods, err := wfUtils.GetPodListFromResources(ctx, cli, resources)
	if err != nil {
		log.Logger.Errorf("fail to get pod list from resources: %v", err)
		return "", err
	}
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Logger.Errorf("fail to get clientset from kubeconfig: %v", err)
		return "", err
	}
	podContainers := make([]PodContainer, 0)
	for _, pod := range pods {
		for _, container := range pod.Spec.Containers {
			pc := PodContainer{
				Name:      pod.Name,
				Namespace: pod.Namespace,
				Container: container.Name,
				Label:     pod.Labels,
			}
			podContainers = append(podContainers, pc)
		}
	}

	wg := sync.WaitGroup{}
	logBuilder := strings.Builder{}
	logMap := concurrent.NewMap()
	wg.Add(len(podContainers))

	for _, pc := range podContainers {
		go func(pc PodContainer) {
			defer wg.Done()
			ctxQuery, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			podLogOpts := corev1.PodLogOptions{
				Container: pc.Container,
				Follow:    false,
				TailLines: &linesOfLogKept,
			}
			req := clientSet.CoreV1().Pods(pc.Namespace).GetLogs(pc.Name, &podLogOpts)
			podLogs, err := req.Stream(ctxQuery)
			if err != nil {
				log.Logger.Errorf("fail to get pod logs: %v", err)
				return
			}
			defer func() {
				_ = podLogs.Close()
			}()
			podColor, containerColor := determineColor(pc.Name)
			buf := new(bytes.Buffer)
			p := podColor.SprintfFunc()
			c := containerColor.SprintfFunc()
			fmt.Fprintf(buf, "%s %s\n", p("› %s", pc.Name), c("%s", pc.Container))
			var readErr error
			r := bufio.NewReader(podLogs)
			for {
				s, err := r.ReadString('\n')
				if err != nil {
					if !errors.Is(err, io.EOF) {
						readErr = err
					}
					break
				}
				shouldPrint := true
				if len(filters) != 0 {
					for _, f := range filters {
						if !strings.Contains(s, f) {
							shouldPrint = false
						}
					}
				}
				if shouldPrint {
					buf.WriteString(s)
				}
			}
			if readErr != nil {
				errPrint(buf, "error in copy information from APIServer to buffer: %s", err.Error())
				log.Logger.Errorf("fail to copy pod logs: %v", err)
			}
			logMap.Store(fmt.Sprintf("%s/%s", pc.Name, pc.Container), buf.String())
		}(pc)
	}
	wg.Wait()

	order := make([]string, 0)
	sort.Strings(order)
	logMap.Range(func(key, value any) bool {
		order = append(order, key.(string))
		return true
	})
	for _, key := range order {
		val, ok := logMap.Load(key)
		if ok {
			logBuilder.WriteString(fmt.Sprintf("%s", val))
		}
	}
	return logBuilder.String(), nil
}

func pipelineStep2WorkflowStep(step model.WorkflowStep) v1alpha1.WorkflowStep {
	res := v1alpha1.WorkflowStep{
		WorkflowStepBase: v1alpha1.WorkflowStepBase{
			Name:       step.Name,
			Type:       step.Type,
			Meta:       step.Meta,
			If:         step.If,
			Timeout:    step.Timeout,
			DependsOn:  step.DependsOn,
			Inputs:     step.Inputs,
			Outputs:    step.Outputs,
			Properties: step.Properties.RawExtension(),
		},
		SubSteps: make([]v1alpha1.WorkflowStepBase, 0),
	}
	for _, subStep := range step.SubSteps {
		res.SubSteps = append(res.SubSteps, v1alpha1.WorkflowStepBase{
			Name:       subStep.Name,
			Type:       subStep.Type,
			Meta:       subStep.Meta,
			If:         subStep.If,
			Timeout:    subStep.Timeout,
			DependsOn:  subStep.DependsOn,
			Inputs:     subStep.Inputs,
			Outputs:    subStep.Outputs,
			Properties: subStep.Properties.RawExtension(),
		})
	}
	return res
}

func pipelineSpec2WorkflowSpec(spec model.WorkflowSpec) *v1alpha1.WorkflowSpec {
	res := &v1alpha1.WorkflowSpec{
		Mode:  spec.Mode,
		Steps: make([]v1alpha1.WorkflowStep, 0),
	}
	for _, step := range spec.Steps {
		res.Steps = append(res.Steps, pipelineStep2WorkflowStep(step))
	}
	return res
}

// RunPipeline will run a pipeline
func (p pipelineServiceImpl) RunPipeline(ctx context.Context, pipeline apis.PipelineBase, req apis.RunPipelineRequest) (*apis.PipelineRun, error) {
	if err := checkRunMode(&req.Mode); err != nil {
		return nil, err
	}
	project := ctx.Value(&apis.CtxKeyProject).(*model.Project)
	run := v1alpha1.WorkflowRun{}
	version := utils.GenerateVersion("")
	name := fmt.Sprintf("%s-%s", pipeline.Name, version)
	s := pipeline.Spec
	run.Name = name
	run.Namespace = project.GetNamespace()
	run.Spec.WorkflowSpec = pipelineSpec2WorkflowSpec(s)
	run.Spec.Mode = &req.Mode

	run.SetLabels(map[string]string{
		labelPipeline:            pipeline.Name,
		model.LabelSourceOfTruth: model.FromUX,
	})

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
		run.Labels[labelContext] = req.ContextName
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
func (p pipelineServiceImpl) getPipelineInfo(ctx context.Context, wf *model.Pipeline, namespace string) (*apis.PipelineInfo, error) {
	var runs v1alpha1.WorkflowRunList
	err := p.KubeClient.List(context.Background(), &runs, client.InNamespace(namespace), client.MatchingLabels{labelPipeline: wf.Name})
	if err != nil {
		if meta.IsNoMatchError(err) {
			weekStat := make([]apis.RunStatInfo, 0)
			for i := 0; i < 7; i++ {
				weekStat = append(weekStat, apis.RunStatInfo{})
			}
			return &apis.PipelineInfo{
				LastRun: nil,
				RunStat: apis.RunStat{
					ActiveNum: 0,
					Total:     apis.RunStatInfo{},
					Week:      weekStat,
				},
			}, nil
		}
		return nil, err
	}
	run := getLastRun(runs.Items)
	runStat := getRunStat(runs.Items)
	pi := &apis.PipelineInfo{
		RunStat: runStat,
	}
	if run != nil {
		projectName := wf.Project
		project, err := p.ProjectService.GetProject(ctx, projectName)
		if err != nil {
			return nil, err
		}
		pi.LastRun, err = workflowRun2PipelineRun(*run, project, p.ContextService)
		if err != nil {
			return nil, err
		}
	}
	return pi, nil
}

func getRunStat(runs []v1alpha1.WorkflowRun) apis.RunStat {
	today := time.Now().Unix()
	isActive := func(run v1alpha1.WorkflowRun) bool {
		return !run.Status.Finished && !run.Status.Terminated && run.Status.Suspend
	}
	isSuccess := func(run v1alpha1.WorkflowRun) bool {
		return run.Status.Phase == v1alpha1.WorkflowStateSucceeded
	}
	// returned int x means the (x+1)/7 days of the week, valid number is 0-6
	inThisWeek := func(run v1alpha1.WorkflowRun) (int, bool) {
		startTime := run.Status.StartTime.Time.Unix()
		// one week, note this is not week of natual, but week of unix timestamp
		if today-startTime < 604800 {
			return 6 - int((today-startTime)/86400), true
		}
		return -1, false
	}
	var (
		act     int
		success int
		fail    int
		week    = make([]apis.RunStatInfo, 7)
	)

	for _, run := range runs {
		// total = success + fail + active
		day, inWeek := inThisWeek(run)
		if inWeek {
			week[day].Total++
		}
		if isActive(run) {
			act++
		} else {
			if isSuccess(run) {
				if inWeek {
					week[day].Success++
				}
				success++
			} else {
				fail++
				if inWeek {
					week[day].Fail++
				}
			}
		}

	}
	return apis.RunStat{
		ActiveNum: act,
		Total: apis.RunStatInfo{
			Total:   len(runs),
			Success: success,
			Fail:    fail,
		},
		Week: week,
	}
}

func getLastRun(runs []v1alpha1.WorkflowRun) *v1alpha1.WorkflowRun {
	if len(runs) == 0 {
		return nil
	}
	last := runs[0]
	lastStartTime := last.Status.StartTime.Time
	for _, wfr := range runs {
		wfr.Status.StartTime.After(lastStartTime)
		last = wfr
		lastStartTime = wfr.Status.StartTime.Time
	}
	// We don't need managed fields, save some bandwidth
	last.ManagedFields = nil
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
	if run.Labels != nil && run.Labels[labelPipeline] != "" {
		pipeline := &model.Pipeline{
			Name:    run.Labels[labelPipeline],
			Project: project.Name,
		}
		if err := p.Store.Get(ctx, pipeline); err != nil {
			log.Logger.Errorf("failed to load the workflow %s", err.Error())
			if errors.Is(err, datastore.ErrRecordNotExist) {
				return nil, bcode.ErrPipelineNotExist
			}
			return nil, err
		}
		run.Spec.WorkflowSpec = pipelineSpec2WorkflowSpec(pipeline.Spec)

	}
	return workflowRun2PipelineRun(run, project, p.ContextService)
}

// ListPipelineRuns will list all pipeline runs
func (p pipelineRunServiceImpl) ListPipelineRuns(ctx context.Context, base apis.PipelineBase) (apis.ListPipelineRunResponse, error) {
	project := ctx.Value(&apis.CtxKeyProject).(*model.Project)
	wfrs := v1alpha1.WorkflowRunList{}
	if err := p.KubeClient.List(ctx, &wfrs, client.InNamespace(project.GetNamespace()), client.MatchingLabels{labelPipeline: base.Name}); err != nil {
		return apis.ListPipelineRunResponse{}, err
	}
	res := apis.ListPipelineRunResponse{
		Runs: make([]apis.PipelineRunBriefing, 0),
	}
	for _, wfr := range wfrs.Items {
		res.Runs = append(res.Runs, p.workflowRun2runBriefing(ctx, wfr, project))
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
	if err := p.KubeClient.List(ctx, &wfrs, client.InNamespace(project.GetNamespace()), client.MatchingLabels{labelPipeline: base.Name}); err != nil {
		return err
	}
	for _, wfr := range wfrs.Items {
		if err := p.KubeClient.Delete(ctx, wfr.DeepCopy()); err != nil {
			return client.IgnoreNotFound(err)
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

func fuzzyMatch(wf *model.Pipeline, q string) bool {
	if strings.Contains(wf.Name, q) {
		return true
	}
	if strings.Contains(wf.Alias, q) {
		return true
	}
	if strings.Contains(wf.Description, q) {
		return true
	}
	return false
}

func pipeline2PipelineBase(wf *model.Pipeline, project model.Project) *apis.PipelineBase {
	return &apis.PipelineBase{
		PipelineMeta: apis.PipelineMeta{
			Name: wf.Name,
			Project: apis.NameAlias{
				Name:  project.Name,
				Alias: project.Alias,
			},
			Description: wf.Description,
			Alias:       wf.Alias,
			CreateTime:  wf.CreateTime,
		},
		Spec: wf.Spec,
	}
}

func workflowRun2PipelineRun(run v1alpha1.WorkflowRun, project *model.Project, ctxService ContextService) (*apis.PipelineRun, error) {
	mergeSteps(&run)
	pipelineName := run.Labels[labelPipeline]
	pipelineRun := &apis.PipelineRun{
		PipelineRunBase: apis.PipelineRunBase{
			PipelineRunMeta: apis.PipelineRunMeta{
				PipelineName: pipelineName,
				Project: apis.NameAlias{
					Name:  project.Name,
					Alias: project.Alias,
				},
				PipelineRunName: run.Name,
			},
			Spec: run.Spec,
		},
		Status: run.Status,
	}
	if labels := run.GetLabels(); labels != nil {
		ctxName := labels[labelContext]
		if ctxName != "" {
			ctx, err := ctxService.GetContext(context.Background(), project.Name, pipelineName, ctxName)
			if err != nil && !errors.Is(err, datastore.ErrRecordNotExist) {
				return nil, err
			}
			pipelineRun.PipelineRunBase.ContextName = ctxName
			pipelineRun.PipelineRunBase.ContextValues = ctx.Values
		}
	}
	return pipelineRun, nil
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

func (p pipelineRunServiceImpl) workflowRun2runBriefing(ctx context.Context, run v1alpha1.WorkflowRun, project *model.Project) apis.PipelineRunBriefing {
	var (
		apiContext *apis.Context
		err        error
	)
	pipelineName := run.Labels[labelPipeline]
	if contextName, ok := run.Labels[labelContext]; ok {
		apiContext, err = p.ContextService.GetContext(ctx, project.Name, pipelineName, contextName)
		if err != nil {
			log.Logger.Warnf("failed to get pipeline run context %s/%s/%s: %v", project, pipelineName, contextName, err)
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
func (p pipelineRunServiceImpl) checkRunNotFinished(ctx context.Context, pipelineRun apis.PipelineRunBase) (*v1alpha1.WorkflowRun, error) {
	project := ctx.Value(&apis.CtxKeyProject).(*model.Project)
	run := v1alpha1.WorkflowRun{}
	if err := p.KubeClient.Get(ctx, types.NamespacedName{
		Namespace: project.GetNamespace(),
		Name:      pipelineRun.PipelineRunName,
	}, &run); err != nil {
		return nil, err
	}
	if run.Status.Terminated || run.Status.Finished {
		return nil, bcode.ErrPipelineRunFinished
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

func checkPipelineSpec(spec model.WorkflowSpec) error {
	if len(spec.Steps) == 0 {
		return bcode.ErrNoSteps
	}
	return nil
}

func checkRunMode(mode *v1alpha1.WorkflowExecuteMode) error {
	if mode.Steps == "" {
		mode.Steps = v1alpha1.WorkflowModeStep
	}
	if mode.SubSteps == "" {
		mode.SubSteps = v1alpha1.WorkflowModeDAG
	}
	if mode.Steps != v1alpha1.WorkflowModeStep && mode.Steps != v1alpha1.WorkflowModeDAG &&
		mode.SubSteps != v1alpha1.WorkflowModeStep && mode.SubSteps != v1alpha1.WorkflowModeDAG {
		return bcode.ErrWrongMode
	}
	return nil
}

// NewTestPipelineService create the pipeline service instance for testing
func NewTestPipelineService(ds datastore.DataStore, c client.Client, cfg *rest.Config) PipelineService {
	projectService := NewTestProjectService(ds, c)
	contextService := NewTestContextService(ds)
	ppRunService := NewTestPipelineRunService(ds, c, cfg)
	pipelineService := &pipelineServiceImpl{
		ProjectService:     projectService,
		ContextService:     contextService,
		KubeClient:         c,
		KubeConfig:         cfg,
		PipelineRunService: ppRunService,
		Store:              ds,
	}
	return pipelineService
}

// NewTestPipelineRunService create the pipeline run service instance for testing
func NewTestPipelineRunService(ds datastore.DataStore, c client.Client, cfg *rest.Config) PipelineRunService {
	contextService := NewTestContextService(ds)
	projectService := NewTestProjectService(ds, c)
	return &pipelineRunServiceImpl{
		KubeClient:     c,
		KubeConfig:     cfg,
		ContextService: contextService,
		ProjectService: projectService,
	}
}

// NewTestContextService create the context service instance for testing
func NewTestContextService(ds datastore.DataStore) ContextService {
	return &contextServiceImpl{
		Store: ds,
	}
}
