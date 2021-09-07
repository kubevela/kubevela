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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/utils"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/kube"
	"github.com/oam-dev/kubevela/pkg/workflow/tasks"
	wfTypes "github.com/oam-dev/kubevela/pkg/workflow/types"
)

// GenerateApplicationSteps generate application steps.
// nolint:gocyclo
func (h *AppHandler) GenerateApplicationSteps(ctx context.Context,
	app *v1beta1.Application,
	appParser *appfile.Parser,
	af *appfile.Appfile,
	appRev *v1beta1.ApplicationRevision,
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

		skipStandardWorkload := skipApplyWorkload(wl)
		appliedResources := []corev1.ObjectReference{}
		if !skipStandardWorkload {
			if err := h.Dispatch(context.Background(), manifest.StandardWorkload); err != nil {
				return failedStepStatus(err, "DispatchStandardWorkload"), nil, nil
			}
			appliedResources = append(appliedResources, genObjectReferences(manifest.StandardWorkload)...)
		}

		if err := h.Dispatch(context.Background(), manifest.Traits...); err != nil {
			return failedStepStatusWithApplied(err, "DispatchTraits", appliedResources), nil, nil
		}
		appliedResources = append(appliedResources, genObjectReferences(manifest.Traits...)...)

		_, isHealth, err := h.collectHealthStatus(wl, appRev)
		if err != nil {
			return failedStepStatusWithApplied(err, "CollectHealthStatus", appliedResources), nil, nil
		}

		if !isHealth {
			return common.WorkflowStepStatus{Phase: common.WorkflowStepPhaseRunning, AppliedResources: appliedResources}, nil, nil
		}

		taskValue, err := makeTaskValue(manifest, skipStandardWorkload, cli)
		if err != nil {
			return failedStepStatusWithApplied(err, "MakeTaskValue", appliedResources), nil, taskValue
		}

		return common.WorkflowStepStatus{Phase: common.WorkflowStepPhaseSucceeded, AppliedResources: appliedResources}, nil, taskValue
	})
	var tasks []wfTypes.TaskRunner
	for _, step := range af.WorkflowSteps {
		genTask, err := taskDiscover.GetTaskGenerator(ctx, step.Type)
		if err != nil {
			return nil, err
		}
		task, err := genTask(step, &wfTypes.GeneratorOptions{ID: generateStepID(step.Name, app.Status.Workflow)})
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

func failedStepStatusWithApplied(err error, reason string, applied []corev1.ObjectReference) common.WorkflowStepStatus {
	return common.WorkflowStepStatus{Phase: common.WorkflowStepPhaseFailed, Reason: reason, Message: err.Error(), AppliedResources: applied}
}

func skipApplyWorkload(wl *appfile.Workload) bool {
	for _, trait := range wl.Traits {
		if trait.FullTemplate.TraitDefinition.Spec.ManageWorkload {
			return true
		}
	}
	return false
}

func makeTaskValue(manifest *types.ComponentManifest, skipStandardWorkload bool, cli client.Client) (*value.Value, error) {
	taskValue, err := value.NewValue("{}", nil)
	if err != nil {
		return nil, err
	}
	if !skipStandardWorkload {
		v := manifest.StandardWorkload.DeepCopy()
		if err := cli.Get(context.Background(), client.ObjectKeyFromObject(manifest.StandardWorkload), v); err != nil {
			return taskValue, err
		}
		if err := taskValue.FillObject(v.Object, "output"); err != nil {
			return taskValue, err
		}
	}

	for _, trait := range manifest.Traits {
		v := trait.DeepCopy()
		if err := cli.Get(context.Background(), client.ObjectKeyFromObject(trait), v); err != nil {
			return taskValue, err
		}
		if err := taskValue.FillObject(trait.Object, "outputs", trait.GetLabels()[oam.TraitResource]); err != nil {
			return taskValue, err
		}
	}
	return taskValue, nil
}

func genObjectReferences(objs ...*unstructured.Unstructured) []corev1.ObjectReference {
	refers := []corev1.ObjectReference{}
	for _, o := range objs {
		refers = append(refers, corev1.ObjectReference{
			Kind:       o.GetKind(),
			Namespace:  o.GetNamespace(),
			Name:       o.GetName(),
			APIVersion: o.GetAPIVersion(),
		})
	}
	return refers
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
