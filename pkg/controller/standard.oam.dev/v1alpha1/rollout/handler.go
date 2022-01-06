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

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/pkg/errors"

	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/assemble"
	"github.com/oam-dev/kubevela/pkg/oam"
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
		WorkloadName(h.compName),
		assemble.PrepareWorkloadForRollout(h.compName),
		HandleReplicas(ctx, h.compName, h.Client)}

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
	targetWorkload, err := h.extractWorkload(ctx, h.rollout.Namespace, h.targetRevName)
	if err != nil {
		return fmt.Errorf("failed to extarct target revision workload of rollout %s %w", h.rollout.Name, err)
	}
	h.targetWorkload = targetWorkload
	if len(h.sourceRevName) != 0 {
		sourceWorkload, err := h.extractWorkload(ctx, h.rollout.Namespace, h.sourceRevName)
		if err != nil {
			return fmt.Errorf("failed to extarct source revision workload of rollout %s %w", h.rollout.Name, err)
		}
		h.sourceWorkload = sourceWorkload
	}
	return nil
}

// extractWorkload extract workload info from componentRevision by revisionName
func (h *handler) extractWorkload(ctx context.Context, namespace, revisionName string) (*unstructured.Unstructured, error) {
	var revision v1.ControllerRevision
	if err := h.Get(ctx, types.NamespacedName{Namespace: namespace, Name: revisionName}, &revision); err != nil {
		klog.Errorf("cannot get rollout related controllerRevision %v", err, "namespace", namespace, "name", revisionName)
		return nil, err
	}
	// extract component data from controllerRevision
	component, err := util.RawExtension2Component(revision.Data)
	if err != nil {
		klog.Errorf("failed to extract component info err: %v", err, "namespace", namespace, "name", revisionName)
		return nil, err
	}
	workload, err := util.RawExtension2Unstructured(&component.Spec.Workload)
	if err != nil {
		klog.Errorf("failed to get workload from component  %v", err, "namespace", namespace, "name", revisionName)
		return nil, err
	}
	return workload, nil
}

// applyTargetWorkload check the target workload whether exist. if not create it.
// and recode workload in resourceTracker
func (h *handler) applyTargetWorkload(ctx context.Context) error {
	if h.targetWorkload == nil {
		return fmt.Errorf("cannot find target workload to template")
	}
	meta.AddOwnerReference(h.targetWorkload, metav1.OwnerReference{
		APIVersion:         v1alpha1.SchemeGroupVersion.String(),
		Kind:               v1alpha1.RolloutKind,
		Name:               h.rollout.Name,
		UID:                h.rollout.UID,
		Controller:         pointer.Bool(false),
		BlockOwnerDeletion: pointer.Bool(true),
	})
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
	// patch target workload to pass rollout owner to workload
	if err := h.passResourceTrackerToWorkload(ctx, h.targetWorkload); err != nil {
		return errors.Wrap(err, "fail to pass resourceTracker to target workload")
	}

	if h.sourceWorkload != nil && (h.sourceWorkload.GetName() != h.targetWorkload.GetName()) {
		if err := h.Delete(ctx, h.sourceWorkload); err != nil {
			return err
		}
		klog.InfoS("gc rollout source workload", "namespace", h.rollout.Namespace,
			"rollout", h.rollout.Name, "sourceWorkload", h.sourceWorkload.GetName())
	}
	return nil
}

func (h *handler) handleFinalizeFailed(ctx context.Context) error {
	if err := h.passResourceTrackerToWorkload(ctx, h.targetWorkload); err != nil {
		return errors.Wrap(err, "fail to pass resourceTracker to target workload")
	}

	// patch target workload to pass rollout owner to source workload
	if h.sourceWorkload != nil && (h.sourceWorkload.GetName() != h.targetWorkload.GetName()) {

		if err := h.passResourceTrackerToWorkload(ctx, h.sourceWorkload); err != nil {
			return errors.Wrap(err, "fail to pass resourceTracker to source workload")
		}

		klog.Info("rollout failed set resourceTracker as source Workload's owner", "namespace", h.rollout.Namespace,
			"rollout", h.rollout.Name, "sourceWorkload", h.sourceWorkload.GetName())
	}

	return nil
}

func (h *handler) passResourceTrackerToWorkload(ctx context.Context, workload *unstructured.Unstructured) error {
	// patch target workload to pass rollout owner to workload
	wl := workload.DeepCopy()
	if err := h.Get(ctx, types.NamespacedName{Namespace: wl.GetNamespace(), Name: wl.GetName()}, wl); err != nil {
		return errors.Wrap(err, "fail to get targetWorkload")
	}
	wlPatch := client.MergeFrom(wl.DeepCopy())
	h.passOwnerToTargetWorkload(wl)
	if err := h.Patch(ctx, wl, wlPatch); err != nil {
		return errors.Wrap(err, "fail to patch workload to pass rollout owners to workload")
	}

	// recode targetWorkload
	if err := h.recordWorkloadInResourceTracker(ctx, workload); err != nil {
		return errors.Wrap(err, "fail to add resourceTracker as owner for workload")
	}

	return nil
}

// if in middle of previous rollout, continue use previous source and target appRevision as this round rollout
func (h *handler) handleRolloutModified() {
	klog.InfoS("rollout target changed, restart the rollout", "new target", h.rollout.Spec.TargetRevisionName)
	h.record.Event(h.rollout, event.Normal("Rollout Restarted",
		"rollout target changed, restart the rollout", "new target", h.rollout.Spec.TargetRevisionName))
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
		// previous rollout have finished, using last target as source and real target as targetRevision
		h.targetRevName = h.rollout.Spec.TargetRevisionName
		h.sourceRevName = h.rollout.Status.LastUpgradedTargetRevision
		// mark so that we don't think we are modified again
		h.rollout.Status.LastUpgradedTargetRevision = h.targetRevName
		h.rollout.Status.LastSourceRevision = h.sourceRevName
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

	var appRev string
	if len(h.rollout.GetLabels()) > 0 {
		appRev = h.rollout.GetLabels()[oam.LabelAppRevision]
	}

	h.targetWorkload.SetName(h.compName)
	util.AddLabels(h.targetWorkload, map[string]string{
		oam.LabelAppComponentRevision: h.targetRevName,
		oam.LabelAppRevision:          appRev,
	})
	util.AddAnnotations(h.targetWorkload, map[string]string{oam.AnnotationSkipGC: "true"})

	if h.sourceWorkload != nil {
		h.sourceWorkload.SetName(h.compName)
		util.AddLabels(h.sourceWorkload, map[string]string{
			oam.LabelAppComponentRevision: h.sourceRevName,
			oam.LabelAppRevision:          appRev,
		})
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

func getWorkloadReplicasNum(u unstructured.Unstructured) (int32, error) {
	replicaPath, err := GetWorkloadReplicasPath(u)
	if err != nil {
		return 0, fmt.Errorf("get workload replicas path err %w", err)
	}
	wlpv := fieldpath.Pave(u.UnstructuredContent())
	replicas, err := wlpv.GetInteger(replicaPath)
	if err != nil {
		return 0, fmt.Errorf("get workload replicas err %w", err)
	}
	return int32(replicas), nil
}

// checkRollingTerminated check the rollout if have finished
func checkRollingTerminated(rollout v1alpha1.Rollout) bool {
	// handle rollout completed
	if rollout.Status.RollingState == v1alpha1.RolloutSucceedState ||
		rollout.Status.RollingState == v1alpha1.RolloutFailedState {
		if rollout.Status.LastUpgradedTargetRevision == rollout.Spec.TargetRevisionName {
			// spec.targetSize could be nil, If targetSize isn't nil and not equal to status.RolloutTargetSize it's
			// means user have modified targetSize to restart an scale operation
			if rollout.Spec.RolloutPlan.TargetSize != nil {
				if rollout.Status.RolloutTargetSize == *rollout.Spec.RolloutPlan.TargetSize {
					klog.InfoS("rollout completed, no need to reconcile", "source", rollout.Spec.SourceRevisionName,
						"target", rollout.Spec.TargetRevisionName)
					return true
				}
				return false
			}
			klog.InfoS("rollout completed, no need to reconcile", "source", rollout.Spec.SourceRevisionName,
				"target", rollout.Spec.TargetRevisionName)
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
			(rollout.Spec.RolloutPlan.TargetSize != nil && rollout.Status.RolloutTargetSize != -1 &&
				rollout.Status.RolloutTargetSize != *rollout.Spec.RolloutPlan.TargetSize))
}

func (h *handler) recordWorkloadInResourceTracker(ctx context.Context, workload *unstructured.Unstructured) error {
	var resourceTrackerName string
	for _, reference := range h.rollout.OwnerReferences {
		if reference.Kind == v1beta1.ResourceTrackerKind && reference.APIVersion == v1beta1.SchemeGroupVersion.String() {
			resourceTrackerName = reference.Name
		}
	}
	if len(resourceTrackerName) == 0 {
		// rollout isn't created by application
		return nil
	}
	rt := v1beta1.ResourceTracker{}
	if err := h.Get(ctx, types.NamespacedName{Name: resourceTrackerName}, &rt); err != nil {
		klog.Errorf("fail to get resourceTracker to record workload rollout: namespace:%s, name: %s", h.rollout.Namespace, h.rollout.Name)
		return err
	}
	rt.AddTrackedResource(workload)
	if err := h.Status().Update(ctx, &rt); err != nil {
		klog.Errorf("fail to update resourceTracker for rollout record workload namespace:%s, name: %s", h.rollout.Namespace, h.rollout.Name)
		return err
	}
	klog.InfoS("succeed to record workload in resourceTracker rollout: namespace:%s, name: %s", h.rollout.Namespace, h.rollout.Name)
	return nil
}

func (h *handler) passOwnerToTargetWorkload(wl *unstructured.Unstructured) {
	var owners []metav1.OwnerReference
	for _, reference := range h.rollout.OwnerReferences {
		reference.Controller = pointer.Bool(false)
		owners = append(owners, reference)
	}
	for _, owner := range owners {
		meta.AddOwnerReference(wl, owner)
	}
}
