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

package rollout

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/oam-dev/kubevela/pkg/oam"

	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/applicationrollout"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"

	v1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/assemble"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

type handler struct {
	*reconciler
	rollout       *v1alpha1.Rollout
	sourceRevName string
	targetRevName string
	compName      string

	sourceWorkload *unstructured.Unstructured
	targetWorkload *unstructured.Unstructured
}

// assembleWorkload help to assemble worklad info, because rollout don't know AppRevision we cannot use assemble func directly.
// but we do same thing with it in the func
func (h *handler) assembleWorkload(ctx context.Context) error {
	workloadOptions := []assemble.WorkloadOption{
		applicationrollout.RolloutWorkloadName(h.compName),
		assemble.PrepareWorkloadForRollout(h.compName),
		applicationrollout.HandleReplicas(ctx, h.compName, h)}

	for _, workloadOption := range workloadOptions {
		if err := workloadOption.ApplyToWorkload(h.targetWorkload, nil, nil); err != nil {
			return err
		}
		if len(h.sourceRevName) != 0 {
			if err := workloadOption.ApplyToWorkload(h.sourceWorkload, nil, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

// extractWorkloadFromCompRevision extract workload info from component revision
func (h *handler) extractWorkloadFromCompRevision(ctx context.Context) error {
	var err error
	var targetRevsion v1.ControllerRevision
	if err = h.Get(ctx, types.NamespacedName{Namespace: h.rollout.Namespace, Name: h.targetRevName}, &targetRevsion); err != nil {
		return err
	}
	// extract component data from controllerRevision
	targetComp, err := util.RawExtension2Component(targetRevsion.Data)
	if err != nil {
		return err
	}
	h.targetWorkload, err = util.RawExtension2Unstructured(&targetComp.Spec.Workload)
	if err != nil {
		return err
	}
	if len(h.sourceRevName) != 0 {
		var sourceRevision v1.ControllerRevision
		if err = h.Get(ctx, types.NamespacedName{Namespace: h.rollout.Namespace, Name: h.sourceRevName}, &sourceRevision); err != nil {
			return err
		}
		sourceComp, err := util.RawExtension2Component(sourceRevision.Data)
		if err != nil {
			return err
		}
		h.sourceWorkload, err = util.RawExtension2Unstructured(&sourceComp.Spec.Workload)
		if err != nil {
			return err
		}
	}
	return nil
}

// templateTargetWorkload check the target workload whether exist. if not create it.
func (h *handler) templateTargetWorkload(ctx context.Context) error {
	if h.targetWorkload == nil {
		return fmt.Errorf("cannot find target workload to template")
	}
	if err := h.applicator.Apply(ctx, h.targetWorkload); err != nil {
		klog.Errorf("cannot template rollout target workload", "namespace", h.rollout.Namespace,
			"rollout", h.rollout.Name, "targetWorkload", h.targetWorkload.GetName())
		return err
	}
	klog.InfoS("template rollout target workload", "namespace", h.rollout.Namespace,
		"rollout", h.rollout.Name, "targetWorkload", h.targetWorkload.GetName())
	return nil
}

// handleFinalizeSucceed gc source workload if target and source are not same one
func (h *handler) handleFinalizeSucceed(ctx context.Context) error {
	// this is a scale operation
	if h.sourceWorkload != nil && (h.sourceWorkload.GetName() != h.targetWorkload.GetName()) {
		if err := h.Delete(ctx, h.sourceWorkload); err != nil {
			return err
		}
		klog.InfoS("gc rollout source workload", "namespace", h.rollout.Namespace,
			"rollout", h.rollout.Name, "sourceWorkload", h.sourceWorkload.GetName())
	}
	return nil
}

// if in middle of previous rollout, continue use previous source and target appRevision as this round rollout
func (h *handler) handleRolloutModified() {
	klog.InfoS("rollout target changed, restart the rollout", "new source", h.rollout.Spec.SourceRevisionName,
		"new target", h.rollout.Spec.TargetRevisionName)
	h.record.Event(h.rollout, event.Normal("Rollout Restarted",
		"rollout target changed, restart the rollout", "new source", h.rollout.Spec.SourceRevisionName,
		"new target", h.rollout.Spec.TargetRevisionName))
	// we are okay to move directly to restart the rollout since we are at the terminal state
	// however, we need to make sure we properly finalizing the existing rollout before restart if it's
	// still in the middle of rolling out
	if h.rollout.Status.RollingState != v1alpha1.RolloutSucceedState &&
		h.rollout.Status.RollingState != v1alpha1.RolloutFailedState {
		// happen when roll forward or revert in middle of rollout, previous rollout haven't finished
		// continue to handle the previous resources until we are okay to move forward
		h.targetRevName = h.rollout.Status.LastUpgradedTargetRevision
		h.sourceRevName = h.rollout.Status.LastSourceRevision
	} else {
		// previous rollout have finished, go ahead using new source/target revision
		h.targetRevName = h.rollout.Spec.TargetRevisionName
		h.sourceRevName = h.rollout.Spec.SourceRevisionName
		// mark so that we don't think we are modified again
		h.rollout.Status.LastUpgradedTargetRevision = h.rollout.Spec.TargetRevisionName
		h.rollout.Status.LastSourceRevision = h.rollout.Spec.SourceRevisionName
	}
	h.rollout.Status.StateTransition(v1alpha1.RollingModifiedEvent)
}

// setWorkloadBaseInfo set base workload base info, which is such as workload name and component revision label
func (h *handler) setWorkloadBaseInfo() {
	if len(h.targetWorkload.GetNamespace()) == 0 {
		h.targetWorkload.SetNamespace(h.rollout.Namespace)
	}
	if h.sourceWorkload != nil && len(h.sourceWorkload.GetNamespace()) == 0 {
		h.sourceWorkload.SetNamespace(h.rollout.Namespace)
	}
	h.targetWorkload.SetName(h.compName)
	util.AddLabels(h.targetWorkload, map[string]string{oam.LabelAppComponentRevision: h.targetRevName})
	if h.sourceWorkload != nil {
		h.sourceWorkload.SetName(h.compName)
		util.AddLabels(h.sourceWorkload, map[string]string{oam.LabelAppComponentRevision: h.sourceRevName})
	}
}

func (h *handler) checkWorkloadNotExist(ctx context.Context) (bool, error) {
	if err := h.Get(ctx, types.NamespacedName{Namespace: h.targetWorkload.GetNamespace(), Name: h.targetWorkload.GetName()}, h.targetWorkload); err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

// checkRollingTerminated check the rollout if have finished
func checkRollingTerminated(appRollout v1alpha1.Rollout) bool {
	// handle rollout completed
	if appRollout.Status.RollingState == v1alpha1.RolloutSucceedState ||
		appRollout.Status.RollingState == v1alpha1.RolloutFailedState {
		if appRollout.Status.LastUpgradedTargetRevision == appRollout.Spec.TargetRevisionName &&
			appRollout.Status.LastSourceRevision == appRollout.Spec.SourceRevisionName {
			// spec.targetSize could be nil, If targetSize isn't nil and not equal to status.RolloutTargetSize it's
			// means user have modified targetSize to restart an scale operation
			if appRollout.Spec.RolloutPlan.TargetSize != nil {
				if appRollout.Status.RolloutTargetSize == *appRollout.Spec.RolloutPlan.TargetSize {
					klog.InfoS("rollout completed, no need to reconcile", "source", appRollout.Spec.SourceRevisionName,
						"target", appRollout.Spec.TargetRevisionName)
					return true
				}
				return false
			}
			klog.InfoS("rollout completed, no need to reconcile", "source", appRollout.Spec.SourceRevisionName,
				"target", appRollout.Spec.TargetRevisionName)
			return true
		}
	}
	return false
}

// check if either the source or the target of the appRollout has changed.
// when reset the state machine, the controller will set the status.RolloutTargetSize as -1 in AppLocating phase
// so we should ignore this case.
// if status.RolloutTargetSize isn't equal to Spec.RolloutPlan.TargetSize, it's means user want trigger another scale operation.
func (h *handler) isRolloutModified(rollout v1alpha1.Rollout) bool {
	return rollout.Status.RollingState != v1alpha1.RolloutDeletingState &&
		((rollout.Status.LastUpgradedTargetRevision != "" &&
			rollout.Status.LastUpgradedTargetRevision != rollout.Spec.TargetRevisionName) ||
			(rollout.Status.LastSourceRevision != "" &&
				rollout.Status.LastSourceRevision != rollout.Spec.SourceRevisionName) ||
			(rollout.Spec.RolloutPlan.TargetSize != nil && rollout.Status.RolloutTargetSize != -1 &&
				rollout.Status.RolloutTargetSize != *rollout.Spec.RolloutPlan.TargetSize))
}
