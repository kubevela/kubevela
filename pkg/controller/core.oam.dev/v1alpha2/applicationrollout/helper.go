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

	"k8s.io/utils/pointer"

	"github.com/openkruise/kruise-api/apps/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/assemble"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func rolloutWorkloadName() assemble.WorkloadOption {
	return assemble.WorkloadOptionFn(func(w *unstructured.Unstructured, component *v1alpha2.Component, definition *v1beta1.ComponentDefinition) error {
		// we hard code the behavior depends on the workload group/kind for now. The only in-place upgradable resources
		// we support is cloneset/statefulset for now. We can easily add more later.
		if w.GroupVersionKind().Group == v1alpha1.GroupVersion.Group {
			if w.GetKind() == reflect.TypeOf(v1alpha1.CloneSet{}).Name() ||
				w.GetKind() == reflect.TypeOf(v1alpha1.StatefulSet{}).Name() {
				// we use the component name alone for those resources that do support in-place upgrade
				klog.InfoS("we reuse the component name for resources that support in-place upgrade",
					"GVK", w.GroupVersionKind(), "instance name", component.Name)
				w.SetName(component.Name)
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
