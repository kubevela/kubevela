/*Copyright 2021 The KubeVela Authors.

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

package application

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/assemble"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	monitorContext "github.com/oam-dev/kubevela/pkg/monitor/context"
	"github.com/oam-dev/kubevela/pkg/monitor/metrics"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/policy/envbinding"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/velaql/providers/query"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/http"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/kube"
	multiclusterProvider "github.com/oam-dev/kubevela/pkg/workflow/providers/multicluster"
	oamProvider "github.com/oam-dev/kubevela/pkg/workflow/providers/oam"
	terraformProvider "github.com/oam-dev/kubevela/pkg/workflow/providers/terraform"
	"github.com/oam-dev/kubevela/pkg/workflow/tasks"
	wfTypes "github.com/oam-dev/kubevela/pkg/workflow/types"
)

var (
	// DisableResourceApplyDoubleCheck optimize applyComponentFunc by disable post resource existing check after dispatch
	DisableResourceApplyDoubleCheck = false
)

// GenerateApplicationSteps generate application steps.
// nolint:gocyclo
func (h *AppHandler) GenerateApplicationSteps(ctx monitorContext.Context,
	app *v1beta1.Application,
	appParser *appfile.Parser,
	af *appfile.Appfile,
	appRev *v1beta1.ApplicationRevision) ([]wfTypes.TaskRunner, error) {

	handlerProviders := providers.NewProviders()
	kube.Install(handlerProviders, app, h.r.Client, h.Dispatch, h.Delete)
	oamProvider.Install(handlerProviders, app, af, h.r.Client, h.applyComponentFunc(
		appParser, appRev, af), h.renderComponentFunc(appParser, appRev, af))
	http.Install(handlerProviders, h.r.Client, app.Namespace)
	pCtx := process.NewContext(generateContextDataFromApp(app, appRev.Name))
	taskDiscover := tasks.NewTaskDiscoverFromRevision(ctx, handlerProviders, h.r.pd, appRev, h.r.dm, pCtx)
	multiclusterProvider.Install(handlerProviders, h.r.Client, app, af,
		h.applyComponentFunc(appParser, appRev, af),
		h.checkComponentHealth(appParser, appRev, af),
		func(comp common.ApplicationComponent) (*appfile.Workload, error) {
			return appParser.ParseWorkloadFromRevision(comp, appRev)
		},
	)
	terraformProvider.Install(handlerProviders, app, func(comp common.ApplicationComponent) (*appfile.Workload, error) {
		return appParser.ParseWorkloadFromRevision(comp, appRev)
	})
	query.Install(handlerProviders, h.r.Client, nil)

	var tasks []wfTypes.TaskRunner
	for _, step := range af.WorkflowSteps {
		task, err := generateStep(ctx, app, step, taskDiscover, h.r.pd, pCtx, "")
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func generateStep(ctx context.Context,
	app *v1beta1.Application,
	step v1beta1.WorkflowStep,
	taskDiscover wfTypes.TaskDiscover,
	pd *packages.PackageDiscover,
	pCtx process.Context,
	parentStepName string) (wfTypes.TaskRunner, error) {
	options := &wfTypes.GeneratorOptions{
		ID:              generateStepID(step.Name, app.Status.Workflow, parentStepName),
		PackageDiscover: pd,
		ProcessContext:  pCtx,
	}
	generatorName := step.Type
	switch {
	case generatorName == wfTypes.WorkflowStepTypeApplyComponent:
		generatorName = wfTypes.WorkflowStepTypeBuiltinApplyComponent
		options.StepConvertor = func(lstep v1beta1.WorkflowStep) (v1beta1.WorkflowStep, error) {
			copierStep := lstep.DeepCopy()
			if err := convertStepProperties(copierStep, app); err != nil {
				return lstep, errors.WithMessage(err, "convert [apply-component]")
			}
			return *copierStep, nil
		}
	case generatorName == wfTypes.WorkflowStepTypeStepGroup:
		var subTaskRunners []wfTypes.TaskRunner
		for _, subStep := range step.SubSteps {
			workflowStep := v1beta1.WorkflowStep{
				Name:       subStep.Name,
				Type:       subStep.Type,
				Properties: subStep.Properties,
				DependsOn:  subStep.DependsOn,
				Inputs:     subStep.Inputs,
				Outputs:    subStep.Outputs,
				If:         subStep.If,
				Timeout:    subStep.Timeout,
				Meta:       subStep.Meta,
			}
			subTask, err := generateStep(ctx, app, workflowStep, taskDiscover, pd, pCtx, step.Name)
			if err != nil {
				return nil, err
			}
			subTaskRunners = append(subTaskRunners, subTask)
		}
		options.SubTaskRunners = subTaskRunners
		options.ExecuteMode = common.WorkflowModeDAG
		if app.Spec.Workflow != nil && app.Spec.Workflow.Mode != nil {
			options.ExecuteMode = app.Spec.Workflow.Mode.SubSteps
		}
	}

	genTask, err := taskDiscover.GetTaskGenerator(ctx, generatorName)
	if err != nil {
		return nil, err
	}

	task, err := genTask(step, options)
	if err != nil {
		return nil, err
	}
	return task, nil
}

func convertStepProperties(step *v1beta1.WorkflowStep, app *v1beta1.Application) error {
	o := struct {
		Component string `json:"component"`
	}{}
	js, err := common.RawExtensionPointer{RawExtension: step.Properties}.MarshalJSON()
	if err != nil {
		return err
	}
	if err := json.Unmarshal(js, &o); err != nil {
		return err
	}

	var componentNames []string
	for _, c := range app.Spec.Components {
		componentNames = append(componentNames, c.Name)
	}

	for _, c := range app.Spec.Components {
		if c.Name == o.Component {
			if dcName, ok := checkDependsOnValidComponent(c.DependsOn, componentNames); !ok {
				return errors.Errorf("component %s not found, which is depended by %s", dcName, c.Name)
			}
			step.Inputs = append(step.Inputs, c.Inputs...)
			for index := range step.Inputs {
				parameterKey := strings.TrimSpace(step.Inputs[index].ParameterKey)
				if parameterKey != "" && !strings.HasPrefix(parameterKey, "properties") && !strings.HasPrefix(parameterKey, "traits[") {
					parameterKey = "properties." + parameterKey
				}
				step.Inputs[index].ParameterKey = parameterKey
			}
			step.Outputs = append(step.Outputs, c.Outputs...)
			step.DependsOn = append(step.DependsOn, c.DependsOn...)
			c.Inputs = nil
			c.Outputs = nil
			c.DependsOn = nil
			step.Properties = util.Object2RawExtension(c)
			return nil
		}
	}
	return errors.Errorf("component %s not found", o.Component)
}

func checkDependsOnValidComponent(dependsOnComponentNames, allComponentNames []string) (string, bool) {
	// does not depend on other components
	if dependsOnComponentNames == nil {
		return "", true
	}
	for _, dc := range dependsOnComponentNames {
		if !utils.StringsContain(allComponentNames, dc) {
			return dc, false
		}
	}
	return "", true
}

func (h *AppHandler) renderComponentFunc(appParser *appfile.Parser, appRev *v1beta1.ApplicationRevision, af *appfile.Appfile) oamProvider.ComponentRender {
	return func(comp common.ApplicationComponent, patcher *value.Value, clusterName string, overrideNamespace string, env string) (*unstructured.Unstructured, []*unstructured.Unstructured, error) {
		ctx := multicluster.ContextWithClusterName(context.Background(), clusterName)

		_, manifest, err := h.prepareWorkloadAndManifests(ctx, appParser, comp, appRev, patcher, af)
		if err != nil {
			return nil, nil, err
		}
		return renderComponentsAndTraits(h.r.Client, manifest, appRev, clusterName, overrideNamespace, env)
	}
}

func (h *AppHandler) checkComponentHealth(appParser *appfile.Parser, appRev *v1beta1.ApplicationRevision, af *appfile.Appfile) oamProvider.ComponentHealthCheck {
	return func(comp common.ApplicationComponent, patcher *value.Value, clusterName string, overrideNamespace string, env string) (bool, error) {
		ctx := multicluster.ContextWithClusterName(context.Background(), clusterName)
		ctx = contextWithComponentNamespace(ctx, overrideNamespace)

		wl, manifest, err := h.prepareWorkloadAndManifests(ctx, appParser, comp, appRev, patcher, af)
		if err != nil {
			return false, err
		}
		wl.Ctx.SetCtx(auth.ContextWithUserInfo(ctx, h.app))

		readyWorkload, readyTraits, err := renderComponentsAndTraits(h.r.Client, manifest, appRev, clusterName, overrideNamespace, env)
		if err != nil {
			return false, err
		}
		checkSkipApplyWorkload(wl)

		dispatchResources := readyTraits
		if !wl.SkipApplyWorkload {
			dispatchResources = append([]*unstructured.Unstructured{readyWorkload}, readyTraits...)
		}
		if !h.resourceKeeper.ContainsResources(dispatchResources) {
			return false, err
		}

		_, isHealth, err := h.collectHealthStatus(auth.ContextWithUserInfo(ctx, h.app), wl, appRev, overrideNamespace)
		return isHealth, err
	}
}

func (h *AppHandler) applyComponentFunc(appParser *appfile.Parser, appRev *v1beta1.ApplicationRevision, af *appfile.Appfile) oamProvider.ComponentApply {
	return func(comp common.ApplicationComponent, patcher *value.Value, clusterName string, overrideNamespace string, env string) (*unstructured.Unstructured, []*unstructured.Unstructured, bool, error) {
		t := time.Now()
		defer func() { metrics.ApplyComponentTimeHistogram.WithLabelValues("-").Observe(time.Since(t).Seconds()) }()

		ctx := multicluster.ContextWithClusterName(context.Background(), clusterName)
		ctx = contextWithComponentNamespace(ctx, overrideNamespace)
		ctx = envbinding.ContextWithEnvName(ctx, env)

		wl, manifest, err := h.prepareWorkloadAndManifests(ctx, appParser, comp, appRev, patcher, af)
		if err != nil {
			return nil, nil, false, err
		}
		if len(manifest.PackagedWorkloadResources) != 0 {
			if err := h.Dispatch(ctx, clusterName, common.WorkflowResourceCreator, manifest.PackagedWorkloadResources...); err != nil {
				return nil, nil, false, errors.WithMessage(err, "cannot dispatch packaged workload resources")
			}
		}
		wl.Ctx.SetCtx(auth.ContextWithUserInfo(ctx, h.app))

		readyWorkload, readyTraits, err := renderComponentsAndTraits(h.r.Client, manifest, appRev, clusterName, overrideNamespace, env)
		if err != nil {
			return nil, nil, false, err
		}
		checkSkipApplyWorkload(wl)

		dispatchResources := readyTraits
		if !wl.SkipApplyWorkload {
			dispatchResources = append([]*unstructured.Unstructured{readyWorkload}, readyTraits...)
		}

		if err := h.Dispatch(ctx, clusterName, common.WorkflowResourceCreator, dispatchResources...); err != nil {
			return nil, nil, false, errors.WithMessage(err, "Dispatch")
		}

		_, isHealth, err := h.collectHealthStatus(ctx, wl, appRev, overrideNamespace)
		if err != nil {
			return nil, nil, false, errors.WithMessage(err, "CollectHealthStatus")
		}

		if DisableResourceApplyDoubleCheck {
			return readyWorkload, readyTraits, isHealth, nil
		}
		workload, traits, err := getComponentResources(auth.ContextWithUserInfo(ctx, h.app), manifest, wl.SkipApplyWorkload, h.r.Client)
		return workload, traits, isHealth, err
	}
}

// overrideTraits will override cluster field to be local for traits which are control plane only
func overrideTraits(appRev *v1beta1.ApplicationRevision, readyTraits []*unstructured.Unstructured) []*unstructured.Unstructured {
	traits := readyTraits
	for index, readyTrait := range readyTraits {
		for _, trait := range appRev.Spec.TraitDefinitions {
			if trait.Spec.ControlPlaneOnly && trait.Name == readyTrait.GetLabels()[oam.TraitTypeLabel] {
				oam.SetCluster(traits[index], "local")
				traits[index].SetNamespace(appRev.GetNamespace())
				break
			}
		}
	}
	return traits
}

func (h *AppHandler) prepareWorkloadAndManifests(ctx context.Context,
	appParser *appfile.Parser,
	comp common.ApplicationComponent,
	appRev *v1beta1.ApplicationRevision,
	patcher *value.Value,
	af *appfile.Appfile) (*appfile.Workload, *types.ComponentManifest, error) {
	wl, err := appParser.ParseWorkloadFromRevision(comp, appRev)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "ParseWorkload")
	}
	wl.Patch = patcher
	manifest, err := af.GenerateComponentManifest(wl, func(ctxData *process.ContextData) {
		if ns := componentNamespaceFromContext(ctx); ns != "" {
			ctxData.Namespace = ns
		}
	})
	if err != nil {
		return nil, nil, errors.WithMessage(err, "GenerateComponentManifest")
	}
	if err := af.SetOAMContract(manifest); err != nil {
		return nil, nil, errors.WithMessage(err, "SetOAMContract")
	}
	if err := h.HandleComponentsRevision(contextWithComponent(ctx, &comp), []*types.ComponentManifest{manifest}); err != nil {
		return nil, nil, errors.WithMessage(err, "HandleComponentsRevision")
	}

	return wl, manifest, nil
}

func renderComponentsAndTraits(client client.Client, manifest *types.ComponentManifest, appRev *v1beta1.ApplicationRevision, clusterName string, overrideNamespace string, env string) (*unstructured.Unstructured, []*unstructured.Unstructured, error) {
	readyWorkload, readyTraits, err := assemble.PrepareBeforeApply(manifest, appRev, []assemble.WorkloadOption{assemble.DiscoveryHelmBasedWorkload(context.TODO(), client)})
	if err != nil {
		return nil, nil, errors.WithMessage(err, "assemble resources before apply fail")
	}
	if clusterName != "" {
		oam.SetCluster(readyWorkload, clusterName)
		for _, readyTrait := range readyTraits {
			oam.SetCluster(readyTrait, clusterName)
		}
	}
	if overrideNamespace != "" {
		readyWorkload.SetNamespace(overrideNamespace)
		for _, readyTrait := range readyTraits {
			readyTrait.SetNamespace(overrideNamespace)
		}
	}
	readyTraits = overrideTraits(appRev, readyTraits)
	if env != "" {
		meta.AddLabels(readyWorkload, map[string]string{oam.LabelAppEnv: env})
		for _, readyTrait := range readyTraits {
			meta.AddLabels(readyTrait, map[string]string{oam.LabelAppEnv: env})
		}
	}
	return readyWorkload, readyTraits, nil
}

func checkSkipApplyWorkload(wl *appfile.Workload) {
	for _, trait := range wl.Traits {
		if trait.FullTemplate.TraitDefinition.Spec.ManageWorkload {
			wl.SkipApplyWorkload = true
			break
		}
	}
}

func getComponentResources(ctx context.Context, manifest *types.ComponentManifest, skipStandardWorkload bool, cli client.Client) (*unstructured.Unstructured, []*unstructured.Unstructured, error) {
	var (
		workload *unstructured.Unstructured
		traits   []*unstructured.Unstructured
	)
	if !skipStandardWorkload {
		v := manifest.StandardWorkload.DeepCopy()
		if err := cli.Get(ctx, client.ObjectKeyFromObject(manifest.StandardWorkload), v); err != nil {
			return nil, nil, err
		}
		workload = v
	}

	for _, trait := range manifest.Traits {
		v := trait.DeepCopy()
		remoteCtx := multicluster.ContextWithClusterName(ctx, oam.GetCluster(v))
		if err := cli.Get(remoteCtx, client.ObjectKeyFromObject(trait), v); err != nil {
			return workload, nil, err
		}
		traits = append(traits, v)
	}
	return workload, traits, nil
}

func getStepID(stepName string, stepsStatus []common.StepStatus) string {
	for _, status := range stepsStatus {
		if status.Name == stepName {
			return status.ID
		}
	}
	return ""
}

func generateStepID(stepName string, workflowStatus *common.WorkflowStatus, parentStepName string) string {
	var id string
	if workflowStatus != nil {
		workflowStepsStatus := workflowStatus.Steps
		if parentStepName != "" {
			for _, status := range workflowStepsStatus {
				if status.Name == parentStepName {
					var stepsStatus []common.StepStatus
					for _, status := range status.SubStepsStatus {
						stepsStatus = append(stepsStatus, status.StepStatus)
					}
					id = getStepID(stepName, stepsStatus)
				}
			}
		} else {
			var stepsStatus []common.StepStatus
			for _, status := range workflowStepsStatus {
				stepsStatus = append(stepsStatus, status.StepStatus)
			}
			id = getStepID(stepName, stepsStatus)
		}
	}

	if id == "" {
		id = utils.RandomString(10)
	}
	return id
}

func generateContextDataFromApp(app *v1beta1.Application, appRev string) process.ContextData {
	data := process.ContextData{
		Namespace:       app.Namespace,
		AppName:         app.Name,
		CompName:        app.Name,
		AppRevisionName: appRev,
	}
	if app.Annotations != nil {
		data.WorkflowName = app.Annotations[oam.AnnotationWorkflowName]
		data.PublishVersion = app.Annotations[oam.AnnotationPublishVersion]
	}
	return data
}
