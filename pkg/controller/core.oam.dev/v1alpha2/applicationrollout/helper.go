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

package applicationrollout

import (
	"reflect"

	"github.com/openkruise/kruise-api/apps/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	oamstd "github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/assemble"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func rolloutWorkloadName(rolloutComp string) assemble.WorkloadOption {
	return assemble.WorkloadOptionFn(func(w *unstructured.Unstructured, _ *v1beta1.ComponentDefinition, _ []*unstructured.Unstructured) error {

		compName := w.GetLabels()[oam.LabelAppComponent]
		if compName != rolloutComp {
			return nil
		}

		// we hard code the behavior depends on the workload group/kind for now. The only in-place upgradable resources
		// we support is cloneset/statefulset for now. We can easily add more later.
		if w.GroupVersionKind().Group == v1alpha1.GroupVersion.Group {
			if w.GetKind() == reflect.TypeOf(v1alpha1.CloneSet{}).Name() ||
				w.GetKind() == reflect.TypeOf(v1alpha1.StatefulSet{}).Name() {
				// we use the component name alone for those resources that do support in-place upgrade
				klog.InfoS("we reuse the component name for resources that support in-place upgrade",
					"GVK", w.GroupVersionKind(), "instance name", w.GetName())
				// assemble use component name as workload name by default
				// so no need to re-set name
				return nil
			}
		}
		// we assume that the rest of the resources do not support in-place upgrade
		compRevName := w.GetLabels()[oam.LabelAppComponentRevision]
		w.SetName(compRevName)
		klog.InfoS("we encountered an unknown resources, assume that it does not support in-place upgrade",
			"GVK", w.GroupVersionKind(), "instance name", compRevName)
		return nil
	})
}

// appRollout should take over updating workload, so disable previous controller owner(resourceTracker)
func disableControllerOwner(workload *unstructured.Unstructured) {
	if workload == nil {
		return
	}
	ownerRefs := workload.GetOwnerReferences()
	for i, ref := range ownerRefs {
		if ref.Controller != nil && *ref.Controller {
			ownerRefs[i].Controller = pointer.BoolPtr(false)
		}
	}
	workload.SetOwnerReferences(ownerRefs)
}

// enableControllerOwner yield controller owner back to resourceTracker
func enableControllerOwner(workload *unstructured.Unstructured) {
	owners := workload.GetOwnerReferences()
	for i, owner := range owners {
		if owner.Kind == v1beta1.ResourceTrackerKind && owner.Controller != nil && !*owner.Controller {
			owners[i].Controller = pointer.BoolPtr(true)
		}
	}
	workload.SetOwnerReferences(owners)
}

func handleRollingTerminated(appRollout v1beta1.AppRollout) bool {
	// handle rollout completed
	if appRollout.Status.RollingState == oamstd.RolloutSucceedState ||
		appRollout.Status.RollingState == oamstd.RolloutFailedState {
		if appRollout.Status.LastUpgradedTargetAppRevision == appRollout.Spec.TargetAppRevisionName &&
			appRollout.Status.LastSourceAppRevision == appRollout.Spec.SourceAppRevisionName {
			// spec.targetSize could be nil, If targetSize isn't nil and not equal to status.RolloutTargetSize it's
			// means user have modified targetSize to restart an scale operation
			if appRollout.Spec.RolloutPlan.TargetSize != nil {
				if appRollout.Status.RolloutTargetSize == *appRollout.Spec.RolloutPlan.TargetSize {
					klog.InfoS("rollout completed, no need to reconcile", "source", appRollout.Spec.SourceAppRevisionName,
						"target", appRollout.Spec.TargetAppRevisionName)
					return true
				}
				return false
			}
			klog.InfoS("rollout completed, no need to reconcile", "source", appRollout.Spec.SourceAppRevisionName,
				"target", appRollout.Spec.TargetAppRevisionName)
			return true
		}
	}
	return false
}

// check if either the source or the target of the appRollout has changed.
// when reset the state machine, the controller will set the status.RolloutTargetSize as -1 in AppLocating phase
// so we should ignore this case.
// if status.RolloutTargetSize isn't equal to Spec.RolloutPlan.TargetSize, it's means user want trigger another scale operation.
func isRolloutModified(appRollout v1beta1.AppRollout) bool {
	return appRollout.Status.RollingState != oamstd.RolloutDeletingState &&
		((appRollout.Status.LastUpgradedTargetAppRevision != "" &&
			appRollout.Status.LastUpgradedTargetAppRevision != appRollout.Spec.TargetAppRevisionName) ||
			(appRollout.Status.LastSourceAppRevision != "" &&
				appRollout.Status.LastSourceAppRevision != appRollout.Spec.SourceAppRevisionName) ||
			(appRollout.Spec.RolloutPlan.TargetSize != nil && appRollout.Status.RolloutTargetSize != -1 &&
				appRollout.Status.RolloutTargetSize != *appRollout.Spec.RolloutPlan.TargetSize))
}
