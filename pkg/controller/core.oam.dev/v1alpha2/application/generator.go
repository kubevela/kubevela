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

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/assemble"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/kube"
	multiclusterProvider "github.com/oam-dev/kubevela/pkg/workflow/providers/multicluster"
	oamProvider "github.com/oam-dev/kubevela/pkg/workflow/providers/oam"
	"github.com/oam-dev/kubevela/pkg/workflow/tasks"
	wfTypes "github.com/oam-dev/kubevela/pkg/workflow/types"
)

// GenerateApplicationSteps generate application steps.
// nolint:gocyclo
func (h *AppHandler) GenerateApplicationSteps(ctx context.Context,
	app *v1beta1.Application,
	appParser *appfile.Parser,
	af *appfile.Appfile,
	appRev *v1beta1.ApplicationRevision) ([]wfTypes.TaskRunner, error) {
	handlerProviders := providers.NewProviders()
	kube.Install(handlerProviders, h.r.Client, h.Dispatch, h.Delete)
	oamProvider.Install(handlerProviders, app, h.applyComponentFunc(
		appParser, appRev, af), h.renderComponentFunc(appParser, appRev, af))
	taskDiscover := tasks.NewTaskDiscover(handlerProviders, h.r.pd, h.r.Client, h.r.dm)
	multiclusterProvider.Install(handlerProviders, h.r.Client, app)
	var tasks []wfTypes.TaskRunner
	for _, step := range af.WorkflowSteps {
		options := &wfTypes.GeneratorOptions{
			ID: generateStepID(step.Name, app.Status.Workflow),
		}
		generatorName := step.Type
		if generatorName == "apply-component" {
			generatorName = "builtin-apply-component"
			options.StepConvertor = func(lstep v1beta1.WorkflowStep) (v1beta1.WorkflowStep, error) {
				copierStep := lstep.DeepCopy()
				if err := convertStepProperties(copierStep, app); err != nil {
					return lstep, errors.WithMessage(err, "convert [apply-component]")
				}
				return *copierStep, nil
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
		tasks = append(tasks, task)
	}
	return tasks, nil
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

	for _, c := range app.Spec.Components {
		if c.Name == o.Component {
			step.Inputs = append(step.Inputs, c.Inputs...)
			for index := range step.Inputs {
				parameterKey := strings.TrimSpace(step.Inputs[index].ParameterKey)
				if !strings.HasPrefix(parameterKey, "properties") && !strings.HasPrefix(parameterKey, "traits[") {
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

func (h *AppHandler) renderComponentFunc(appParser *appfile.Parser, appRev *v1beta1.ApplicationRevision, af *appfile.Appfile) oamProvider.ComponentRender {
	return func(comp common.ApplicationComponent, patcher *value.Value, clusterName string, overrideNamespace string) (*unstructured.Unstructured, []*unstructured.Unstructured, error) {
		ctx := multicluster.ContextWithClusterName(context.Background(), clusterName)

		_, manifest, err := h.prepareWorkloadAndManifests(ctx, appParser, comp, appRev, patcher, af)
		if err != nil {
			return nil, nil, err
		}
		return renderComponentsAndTraits(h.r.Client, manifest, appRev, overrideNamespace)
	}
}

func (h *AppHandler) applyComponentFunc(appParser *appfile.Parser, appRev *v1beta1.ApplicationRevision, af *appfile.Appfile) oamProvider.ComponentApply {
	return func(comp common.ApplicationComponent, patcher *value.Value, clusterName string, overrideNamespace string) (*unstructured.Unstructured, []*unstructured.Unstructured, bool, error) {
		ctx := multicluster.ContextWithClusterName(context.Background(), clusterName)
		if overrideNamespace != "" {
			oldNamespace := h.app.Namespace
			h.app.Namespace = overrideNamespace
			defer func() {
				h.app.Namespace = oldNamespace
			}()
		}

		wl, manifest, err := h.prepareWorkloadAndManifests(ctx, appParser, comp, appRev, patcher, af)
		if err != nil {
			return nil, nil, false, err
		}
		if len(manifest.PackagedWorkloadResources) != 0 {
			if err := h.Dispatch(ctx, clusterName, common.WorkflowResourceCreator, manifest.PackagedWorkloadResources...); err != nil {
				return nil, nil, false, errors.WithMessage(err, "cannot dispatch packaged workload resources")
			}
		}
		wl.Ctx.SetCtx(ctx)

		readyWorkload, readyTraits, err := renderComponentsAndTraits(h.r.Client, manifest, appRev, overrideNamespace)
		if err != nil {
			return nil, nil, false, err
		}
		skipStandardWorkload := skipApplyWorkload(wl)
		if !skipStandardWorkload {
			if err := h.Dispatch(ctx, clusterName, common.WorkflowResourceCreator, readyWorkload); err != nil {
				return nil, nil, false, errors.WithMessage(err, "DispatchStandardWorkload")
			}
		}

		if err := h.Dispatch(ctx, clusterName, common.WorkflowResourceCreator, readyTraits...); err != nil {
			return nil, nil, false, errors.WithMessage(err, "DispatchTraits")
		}

		_, isHealth, err := h.collectHealthStatus(wl, appRev)
		if err != nil {
			return nil, nil, false, errors.WithMessage(err, "CollectHealthStatus")
		}

		if !isHealth {
			return nil, nil, false, nil
		}
		workload, traits, err := getComponentResources(ctx, manifest, skipStandardWorkload, h.r.Client)
		return workload, traits, true, err
	}
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
	manifest, err := af.GenerateComponentManifest(wl)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "GenerateComponentManifest")
	}
	if err := af.SetOAMContract(manifest); err != nil {
		return nil, nil, errors.WithMessage(err, "SetOAMContract")
	}
	if err := h.HandleComponentsRevision(ctx, []*types.ComponentManifest{manifest}); err != nil {
		return nil, nil, errors.WithMessage(err, "HandleComponentsRevision")
	}

	return wl, manifest, nil
}

func renderComponentsAndTraits(client client.Client, manifest *types.ComponentManifest, appRev *v1beta1.ApplicationRevision, overrideNamespace string) (*unstructured.Unstructured, []*unstructured.Unstructured, error) {
	readyWorkload, readyTraits, err := assemble.PrepareBeforeApply(manifest, appRev, []assemble.WorkloadOption{assemble.DiscoveryHelmBasedWorkload(context.TODO(), client)})
	if err != nil {
		return nil, nil, errors.WithMessage(err, "assemble resources before apply fail")
	}
	if overrideNamespace != "" {
		readyWorkload.SetNamespace(overrideNamespace)
		for _, readyTrait := range readyTraits {
			readyTrait.SetNamespace(overrideNamespace)
		}
	}
	return readyWorkload, readyTraits, nil
}

func skipApplyWorkload(wl *appfile.Workload) bool {
	for _, trait := range wl.Traits {
		if trait.FullTemplate.TraitDefinition.Spec.ManageWorkload {
			return true
		}
	}
	return false
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
		if err := cli.Get(ctx, client.ObjectKeyFromObject(trait), v); err != nil {
			return workload, nil, err
		}
		traits = append(traits, v)
	}
	return workload, traits, nil
}

func generateStepID(stepName string, wfStatus *common.WorkflowStatus) string {
	var id string
	if wfStatus != nil {
		for _, status := range wfStatus.Steps {
			if status.Name == stepName {
				id = status.ID
			}
		}
	}
	if id == "" {
		id = utils.RandomString(10)
	}
	return id
}
