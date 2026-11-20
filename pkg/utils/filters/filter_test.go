/*
Copyright 2022 The KubeVela Authors.

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

package filters

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	addonutil "github.com/oam-dev/kubevela/pkg/utils/addon"
)

func TestApply(t *testing.T) {
	// Any of the filters rejected
	assert.Equal(t, false, Apply(unstructured.Unstructured{},
		KeepAll(),
		KeepNone(),
	))

	// All filters kept
	assert.Equal(t, true, Apply(unstructured.Unstructured{},
		KeepAll(),
		KeepAll(),
	))
}

func TestApplyToList(t *testing.T) {
	list := unstructured.UnstructuredList{Items: []unstructured.Unstructured{
		{},
		{},
	}}

	list.Items[0].SetName("name")

	filtered := ApplyToList(list, KeepAll(), ByName("name"))
	assert.Equal(t, len(filtered.Items), 1)
}

func TestKeepAll(t *testing.T) {
	f := KeepAll()
	assert.Equal(t, true, f(unstructured.Unstructured{}))
}

func TestKeepNone(t *testing.T) {
	f := KeepNone()
	assert.Equal(t, false, f(unstructured.Unstructured{}))
}

func TestByOwnerAddon(t *testing.T) {
	// Test with empty addon name
	f := ByOwnerAddon("")
	assert.Equal(t, true, f(unstructured.Unstructured{}))

	f = ByOwnerAddon("addon-name")

	// Test with empty owner refs
	u := unstructured.Unstructured{}
	assert.Equal(t, false, f(u))

	// Test with right owner refs
	u.SetOwnerReferences([]v1.OwnerReference{{
		Name: addonutil.Addon2AppName("addon-name"),
	}})
	assert.Equal(t, true, f(u))

	// Test with wrong owner refs
	u.SetOwnerReferences([]v1.OwnerReference{{
		Name: "addon-name-2",
	}})
	assert.Equal(t, false, f(u))
}

func TestByName(t *testing.T) {
	// Test with empty name
	f := ByName("")
	assert.Equal(t, true, f(unstructured.Unstructured{}))

	f = ByName("name")

	// Test with empty name
	u := unstructured.Unstructured{}
	assert.Equal(t, false, f(u))

	// Test with right name
	u.SetName("name")
	assert.Equal(t, true, f(u))

	// Test with wrong name
	u.SetName("name-2")
	assert.Equal(t, false, f(u))
}

func TestByAppliedWorkload(t *testing.T) {
	// Test with empty workload
	f := ByAppliedWorkload("")
	assert.Equal(t, true, f(unstructured.Unstructured{}))

	f = ByAppliedWorkload("workload")

	// Test with AppliesToWorkloads=*
	trait := v1beta1.TraitDefinition{}
	trait.Spec.AppliesToWorkloads = []string{"*"}
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&trait)
	assert.NoError(t, err)
	assert.Equal(t, true, f(unstructured.Unstructured{Object: u}))

	// Test with AppliesToWorkloads=workload
	trait = v1beta1.TraitDefinition{}
	trait.Spec.AppliesToWorkloads = []string{"workload"}
	u, err = runtime.DefaultUnstructuredConverter.ToUnstructured(&trait)
	assert.NoError(t, err)
	assert.Equal(t, true, f(unstructured.Unstructured{Object: u}))

	// Test with AppliesToWorkloads=wrong
	trait = v1beta1.TraitDefinition{}
	trait.Spec.AppliesToWorkloads = []string{"wrong"}
	u, err = runtime.DefaultUnstructuredConverter.ToUnstructured(&trait)
	assert.NoError(t, err)
	assert.Equal(t, false, f(unstructured.Unstructured{Object: u}))

	// Test not a definition
	assert.Equal(t, false, f(unstructured.Unstructured{}))
}

func TestByUsableFrom(t *testing.T) {
	def := func(patterns []string, selector map[string]string) unstructured.Unstructured {
		spec := map[string]interface{}{}
		restrictions := map[string]interface{}{}
		if patterns != nil {
			ns := make([]interface{}, 0, len(patterns))
			for _, p := range patterns {
				ns = append(ns, p)
			}
			restrictions["namespaces"] = ns
		}
		if selector != nil {
			labels := map[string]interface{}{}
			for k, v := range selector {
				labels[k] = v
			}
			restrictions["namespaceSelector"] = map[string]interface{}{"matchLabels": labels}
		}
		if len(restrictions) > 0 {
			spec["restrictions"] = restrictions
		}
		return unstructured.Unstructured{Object: map[string]interface{}{"spec": spec}}
	}

	testCases := map[string]struct {
		obj      unstructured.Unstructured
		ns       string
		nsLabels map[string]string
		want     bool
	}{
		"an empty namespace keeps everything": {def([]string{"tenant-*"}, nil), "", nil, true},
		"unrestricted is kept":                {def(nil, nil), "default", nil, true},
		"a matching glob is kept":             {def([]string{"tenant-*"}, nil), "tenant-a", nil, true},
		"a non-matching glob is dropped":      {def([]string{"tenant-*"}, nil), "default", nil, false},
		"a matching selector is kept": {
			def(nil, map[string]string{"tenant": "true"}), "acme",
			map[string]string{"tenant": "true"}, true,
		},
		"a non-matching selector is dropped": {
			def(nil, map[string]string{"tenant": "true"}), "acme", map[string]string{}, false,
		},
		// nil labels mean they could not be read, so a selector keeps nothing.
		"unreadable labels drop a selector": {
			def(nil, map[string]string{"tenant": "true"}), "acme", nil, false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, ByUsableFrom(tc.ns, tc.nsLabels)(tc.obj))
		})
	}
}
