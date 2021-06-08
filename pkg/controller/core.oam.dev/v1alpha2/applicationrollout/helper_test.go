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
	"testing"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/pointer"
)

func TestDisableControllerOwner(t *testing.T) {
	w := &unstructured.Unstructured{}
	owners := []metav1.OwnerReference{
		{Name: "test-1", Controller: pointer.BoolPtr(false)},
		{Name: "test-2", Controller: pointer.BoolPtr(true)},
	}
	w.SetOwnerReferences(owners)
	disableControllerOwner(w)
	assert.Equal(t, 2, len(w.GetOwnerReferences()))
	for _, reference := range w.GetOwnerReferences() {
		assert.Equal(t, false, *reference.Controller)
	}
}

func TestEnableControllerOwner(t *testing.T) {
	w := &unstructured.Unstructured{}
	owners := []metav1.OwnerReference{
		{Name: "test-1", Controller: pointer.BoolPtr(false), Kind: v1beta1.ResourceTrackerKind},
		{Name: "test-2", Controller: pointer.BoolPtr(false), Kind: v1alpha2.ApplicationKind},
	}
	w.SetOwnerReferences(owners)
	enableControllerOwner(w)
	assert.Equal(t, 2, len(w.GetOwnerReferences()))
	for _, reference := range w.GetOwnerReferences() {
		if reference.Kind == v1beta1.ResourceTrackerKind {
			assert.Equal(t, true, *reference.Controller)
		} else {
			assert.Equal(t, false, *reference.Controller)
		}
	}
}
