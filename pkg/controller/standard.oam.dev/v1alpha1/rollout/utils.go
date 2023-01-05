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
	"reflect"

	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/assemble"
	"github.com/oam-dev/kubevela/pkg/oam"

	"github.com/openkruise/kruise-api/apps/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// GetWorkloadReplicasPath get replicas path of workload
func GetWorkloadReplicasPath(u unstructured.Unstructured) (string, error) {
	switch u.GetKind() {
	case reflect.TypeOf(v1alpha1.CloneSet{}).Name(), reflect.TypeOf(appsv1.Deployment{}).Name(), reflect.TypeOf(appsv1.StatefulSet{}).Name():
		return "spec.replicas", nil
	default:
		return "", fmt.Errorf("rollout meet a workload we cannot support yet Kind  %s name %s", u.GetKind(), u.GetName())
	}
}

// WorkloadName generate workload name for when rollout
func WorkloadName(rolloutComp string) assemble.WorkloadOption {
	return assemble.WorkloadOptionFn(func(w *unstructured.Unstructured, _ *v1beta1.ComponentDefinition, _ []*unstructured.Unstructured) error {

		compName := w.GetLabels()[oam.LabelAppComponent]
		if compName != rolloutComp {
			return nil
		}

		// we hard code the behavior depends on the workload group/kind for now. The only in-place upgradable resources
		// we support is cloneset/statefulset for now. We can easily add more later.
		supportInplaceUpgrade := false
		if w.GroupVersionKind().Group == v1alpha1.GroupVersion.Group {
			if w.GetKind() == reflect.TypeOf(v1alpha1.CloneSet{}).Name() {
				supportInplaceUpgrade = true
			}
		} else if w.GroupVersionKind().Group == appsv1.GroupName {
			if w.GetKind() == reflect.TypeOf(appsv1.StatefulSet{}).Name() {
				supportInplaceUpgrade = true
			}
		}

		if supportInplaceUpgrade {
			// we use the component name alone for those resources that do support in-place upgrade
			klog.InfoS("we reuse the component name for resources that support in-place upgrade",
				"GVK", w.GroupVersionKind(), "instance name", w.GetName())
			// assemble use component name as workload name by default
			// so no need to re-set name
			return nil
		}

		// we assume that the rest of the resources do not support in-place upgrade
		compRevName := w.GetLabels()[oam.LabelAppComponentRevision]
		w.SetName(compRevName)
		klog.InfoS("we encountered an unknown resources, assume that it does not support in-place upgrade",
			"GVK", w.GroupVersionKind(), "instance name", compRevName)
		return nil
	})
}

// HandleReplicas override initial replicas of workload
func HandleReplicas(ctx context.Context, rolloutComp string, c client.Client) assemble.WorkloadOption {
	return assemble.WorkloadOptionFn(func(u *unstructured.Unstructured, _ *v1beta1.ComponentDefinition, _ []*unstructured.Unstructured) error {
		compName := u.GetLabels()[oam.LabelAppComponent]
		if compName != rolloutComp {
			return nil
		}

		pv := fieldpath.Pave(u.UnstructuredContent())

		// we hard code here, but we can easily support more types of workload by add more cases logic in switch
		replicasFieldPath, err := GetWorkloadReplicasPath(*u)
		if err != nil {
			klog.Errorf("rollout meet a workload we cannot support yet", "Kind", u.GetKind(), "name", u.GetName())
			return err
		}

		workload := u.DeepCopy()
		if err := c.Get(ctx, types.NamespacedName{Namespace: u.GetNamespace(), Name: u.GetName()}, workload); err != nil {
			if apierrors.IsNotFound(err) {
				//  workload not exist(eg. first scale operation) we force set the replicas to zero
				err = pv.SetNumber(replicasFieldPath, 0)
				if err != nil {
					return err
				}
				klog.InfoS("assemble force set workload replicas to 0", "Kind", u.GetKind(), "name", u.GetName())
				return nil
			}
			klog.Errorf("fail to get workload %s: %v", u.GetName(), err)
			return err
		}
		// the workload already exist, we cannot reset the replicas with manifest
		// eg. if workload type is cloneset. the source worklaod and target worklaod is same one
		// so dispatch shouldn't modify current replica number.
		wlpv := fieldpath.Pave(workload.UnstructuredContent())
		replicas, err := wlpv.GetInteger(replicasFieldPath)
		if err != nil {
			klog.Errorf("fail to get `spec.replicas` field from workload %s: %v", u.GetName(), err)
			return err
		}
		if err = pv.SetNumber(replicasFieldPath, float64(replicas)); err != nil {
			return err
		}
		klog.InfoS("assemble set existing workload replicas", "Kind", u.GetKind(), "name", u.GetName(), "replicas", replicas)
		return nil
	})
}
