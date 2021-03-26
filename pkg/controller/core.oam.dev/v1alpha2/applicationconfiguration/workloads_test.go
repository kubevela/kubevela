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

package applicationconfiguration

import (
	"strconv"
	"strings"
	"testing"

	kruise "github.com/openkruise/kruise-api/apps/v1alpha1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/pkg/oam/util"
)

func TestSetAppWorkloadInstanceName(t *testing.T) {
	tests := map[string]struct {
		compName string
		w        *unstructured.Unstructured
		revision int
		expName  string
		inplace  string
		reason   string
	}{
		"two resources case": {
			compName: "webservice",
			revision: 5,
			w: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "extensions/v1beta1",
				"kind":       "deployment",
			}},
			expName: "webservice-v5",
			reason:  "workloadName should be the component with revision",
		},
		"one resources case": {
			compName: "mysql",
			revision: 2,
			w: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "apps.kruise.io/v1alpha1",
				"kind":       "CloneSet",
			}},
			expName: "mysql",
			reason:  "workloadName should be just the component name if we can do in-place upgrade",
		},
		"ignore any existing name": {
			compName: "mysql",
			revision: 2,
			w: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "apps.kruise.io/v1alpha1",
				"kind":       "CloneSet",
				"metadata": map[string]interface{}{
					"name": "mysql-v1",
				},
			}},
			expName: "mysql",
			reason:  "workloadName set in the template is ignored",
		},
		"one resources same name case": {
			compName: "mysql",
			revision: 2,
			w: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "oam.dev/v1alpha1",
				"kind":       "CloneSet",
			}},
			expName: "mysql-v2",
			reason:  "we compare not only the kind but also the group name",
		},
		"use inplaceUpgrade = true": {
			compName: "mysql",
			revision: 2,
			w: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "oam.dev/v1alpha1",
				"kind":       "CloneSet",
			}},
			expName: "mysql",
			inplace: strconv.FormatBool(true),
			reason:  "we compare not only the kind but also the group name",
		},
		"use inplaceUpgrade = other value won't work": {
			compName: "mysql",
			revision: 2,
			w: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "oam.dev/v1alpha1",
				"kind":       "CloneSet",
			}},
			expName: "mysql-v2",
			inplace: "t",
			reason:  "we compare not only the kind but also the group name",
		},
	}
	for name, ti := range tests {
		t.Run(name, func(t *testing.T) {
			SetAppWorkloadInstanceName(ti.compName, ti.w, ti.revision, ti.inplace)
			assert.Equal(t, ti.expName, ti.w.GetName(), ti.reason)
		})
	}
}

func TestPrepWorkloadInstanceForRollout(t *testing.T) {
	workload := kruise.CloneSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CloneSet",
			APIVersion: "apps.kruise.io/v1alpha1",
		},
		Spec: kruise.CloneSetSpec{
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{},
				},
			},
		},
	}
	w, _ := util.Object2Unstructured(workload)
	assert.True(t, prepWorkloadInstanceForRollout(w) == nil)
	value, exist, err := unstructured.NestedBool(w.Object, "spec", "updateStrategy", "paused")
	assert.True(t, exist)
	assert.True(t, err == nil)
	assert.True(t, value)
	// Test statefulset
	workload.Kind = "StatefulSet"
	w, _ = util.Object2Unstructured(workload)
	assert.True(t, prepWorkloadInstanceForRollout(w) == nil)
	value, exist, err = unstructured.NestedBool(w.Object, "spec", "updateStrategy", "rollingUpdate", "paused")
	assert.True(t, exist)
	assert.True(t, err == nil)
	assert.True(t, value)
	// Test deployment
	workload.Kind = "Deployment"
	workload.APIVersion = "apps/v1"
	w, _ = util.Object2Unstructured(workload)
	assert.True(t, prepWorkloadInstanceForRollout(w) == nil)
	value, exist, err = unstructured.NestedBool(w.Object, "spec", "paused")
	assert.True(t, exist)
	assert.True(t, err == nil)
	assert.True(t, value)
	// Test other
	workload.Kind = "StatefulSet"
	w, _ = util.Object2Unstructured(workload)
	assert.True(t, strings.Contains(prepWorkloadInstanceForRollout(w).Error(), "we do not know how to prepare"))
}
