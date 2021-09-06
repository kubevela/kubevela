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

package workflow

import (
	"context"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/utils"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/kube"
	"github.com/oam-dev/kubevela/pkg/workflow/tasks"
	wfTypes "github.com/oam-dev/kubevela/pkg/workflow/types"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Handler is the interface of application handler.
type Handler interface {
	HandleComponentsRevision(ctx context.Context, compManifests []*types.ComponentManifest) error
	Dispatch(ctx context.Context, manifests ...*unstructured.Unstructured) error
}

// GenerateApplicationSteps generate application steps.
func GenerateApplicationSteps(ctx context.Context,
	app *v1beta1.Application,
	appParser *appfile.Parser,
	af *appfile.Appfile,
	appRev *v1beta1.ApplicationRevision,
	h Handler,
	cli client.Client,
	dm discoverymapper.DiscoveryMapper,
	pd *packages.PackageDiscover) ([]wfTypes.TaskRunner, error) {

	handlerProviders := providers.NewProviders()
	kube.Install(handlerProviders, cli, h.Dispatch)
	taskDiscover := tasks.NewTaskDiscover(handlerProviders, pd, cli, dm)
	taskDiscover.RegisterGenerator("oam.dev/apply-component", func(_ wfContext.Context, options *wfTypes.TaskRunOptions, paramValue *value.Value) (common.WorkflowStepStatus, *wfTypes.Operation, *value.Value) {
		comp := common.ApplicationComponent{}

		if err := paramValue.UnmarshalTo(&comp); err != nil {
			return failedStepStatus(err, "Parameter Invalid"), nil, nil
		}

		wl, err := appParser.ParseWorkloadFromRevision(comp, appRev)
		if err != nil {
			return failedStepStatus(err, "ParseWorkload"), nil, nil
		}

		manifest, err := af.GenerateComponentManifest(wl)
		if err != nil {
			return failedStepStatus(err, "GenerateComponentManifest"), nil, nil
		}
		if err := af.SetOAMContract(manifest); err != nil {
			return failedStepStatus(err, "SetOAMContract"), nil, nil
		}
		if err := h.HandleComponentsRevision(context.Background(), []*types.ComponentManifest{manifest}); err != nil {
			return failedStepStatus(err, "HandleComponentsRevision"), nil, nil
		}

		taskValue, err := value.NewValue("{}", nil)
		if err != nil {
			return failedStepStatus(err, "MakeTaskValue"), nil, nil
		}
		skipStandardWorkload := false
		for _, trait := range wl.Traits {
			if trait.FullTemplate.TraitDefinition.Spec.ManageWorkload {
				skipStandardWorkload = true
			}
		}
		if !skipStandardWorkload {
			if err := h.Dispatch(context.Background(), manifest.StandardWorkload); err != nil {
				return failedStepStatus(err, "DispatchStandardWorkload"), nil, nil
			}

		}
		if err := h.Dispatch(context.Background(), manifest.Traits...); err != nil {
			return failedStepStatus(err, "DispatchTraits"), nil, nil
		}

		app := appRev.Spec.Application
		pCtx := wl.Ctx
		isHealth := true
		if ok, err := wl.EvalHealth(pCtx, cli, app.Namespace); err != nil || !ok {
			isHealth = false
		}
		if isHealth {
			for _, trait := range wl.Traits {
				if ok, err := trait.EvalHealth(pCtx, cli, app.Namespace); err != nil || !ok {
					isHealth = false
					break
				}
			}
		}
		if !isHealth {
			return common.WorkflowStepStatus{Phase: common.WorkflowStepPhaseRunning}, nil, taskValue
		}

		if !skipStandardWorkload {
			v := manifest.StandardWorkload.DeepCopy()
			if err := cli.Get(context.Background(), client.ObjectKeyFromObject(manifest.StandardWorkload), v); err != nil {
				return failedStepStatus(err, "TaskValueFillOutput"), nil, taskValue
			}
			if err := taskValue.FillObject(v.Object, "output"); err != nil {
				return failedStepStatus(err, "TaskValueFillOutput"), nil, taskValue
			}
		}

		for _, trait := range manifest.Traits {
			v := trait.DeepCopy()
			if err := cli.Get(context.Background(), client.ObjectKeyFromObject(trait), v); err != nil {
				return failedStepStatus(err, "TaskValueFillOutput"), nil, taskValue
			}
			if err := taskValue.FillObject(trait.Object, "output", trait.GetName()); err != nil {
				return failedStepStatus(err, "TaskValueFillOutputs"), nil, taskValue
			}
		}
		return common.WorkflowStepStatus{Phase: common.WorkflowStepPhaseSucceeded}, nil, taskValue
	})
	var tasks []wfTypes.TaskRunner
	for _, step := range af.WorkflowSteps {
		genTask, err := taskDiscover.GetTaskGenerator(ctx, step.Type)
		if err != nil {
			return nil, err
		}
		var id string
		if app.Status.Workflow != nil {
			for _, status := range app.Status.Workflow.Steps {
				if status.Name == step.Name {
					id = status.ID
				}
			}
		}
		if id == "" {
			id = utils.RandomString(10)
		}
		task, err := genTask(step, &wfTypes.GeneratorOptions{ID: id})
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func failedStepStatus(err error, reason string) common.WorkflowStepStatus {
	return common.WorkflowStepStatus{Phase: common.WorkflowStepPhaseFailed, Reason: reason, Message: err.Error()}
}
