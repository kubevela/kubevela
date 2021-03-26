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

package healthscope

import (
	"context"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kuberuntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// PodSpecWorkload is a generic PaaS workload which adopts full K8s pod spec.
	// More details refer to oam-dev/kubevela
	podSpecWorkloadGVK = schema.GroupVersionKind{
		Group:   "standard.oam.dev",
		Version: "v1alpha1",
		Kind:    "PodSpecWorkload",
	}
)

// CheckPodSpecWorkloadHealth check health condition of podspecworkloads.standard.oam.dev
func CheckPodSpecWorkloadHealth(ctx context.Context, c client.Client, ref runtimev1alpha1.TypedReference, namespace string) *WorkloadHealthCondition {
	if ref.GroupVersionKind() != podSpecWorkloadGVK {
		return nil
	}
	r := &WorkloadHealthCondition{
		HealthStatus:   StatusHealthy,
		TargetWorkload: ref,
	}
	workloadObj := unstructured.Unstructured{}
	workloadObj.SetGroupVersionKind(ref.GroupVersionKind())
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, &workloadObj); err != nil {
		r.HealthStatus = StatusUnhealthy
		r.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
		return r
	}
	r.ComponentName = getComponentNameFromLabel(&workloadObj)
	r.TargetWorkload.UID = workloadObj.GetUID()

	childRefsData, _, _ := unstructured.NestedSlice(workloadObj.Object, "status", "resources")
	childRefs := []runtimev1alpha1.TypedReference{}
	for _, v := range childRefsData {
		v := v.(map[string]interface{})
		tmpChildRef := &runtimev1alpha1.TypedReference{}
		if err := kuberuntime.DefaultUnstructuredConverter.FromUnstructured(v, tmpChildRef); err != nil {
			r.HealthStatus = StatusUnhealthy
			r.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
		}
		childRefs = append(childRefs, *tmpChildRef)
	}
	updateChildResourcesCondition(ctx, c, namespace, r, ref, childRefs)
	return r
}
