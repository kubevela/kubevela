/*
Copyright 2026 The KubeVela Authors.

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

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func def(name string, spec map[string]interface{}) unstructured.Unstructured {
	obj := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "core.oam.dev/v1beta1",
		"kind":       "ComponentDefinition",
		"metadata":   map[string]interface{}{"name": name},
	}}
	if spec != nil {
		obj.Object["spec"] = spec
	}
	return obj
}

func TestByAbstractHidesThemUnlessAsked(t *testing.T) {
	plain := def("webservice", map[string]interface{}{"extends": ""})
	abstract := def("base", map[string]interface{}{"abstract": true})

	list := unstructured.UnstructuredList{Items: []unstructured.Unstructured{plain, abstract}}

	hidden := ApplyToList(list, ByAbstract(false))
	require.Len(t, hidden.Items, 1)
	require.Equal(t, "webservice", hidden.Items[0].GetName())

	shown := ApplyToList(list, ByAbstract(true))
	require.Len(t, shown.Items, 2)
}

func TestIsAbstractReadsTheSpec(t *testing.T) {
	require.False(t, IsAbstract(def("a", nil)))
	require.False(t, IsAbstract(def("b", map[string]interface{}{"abstract": false})))
	require.True(t, IsAbstract(def("c", map[string]interface{}{"abstract": true})))

	// A spec.abstract that is not a bool at all, which a hand-written manifest
	// can produce.
	require.False(t, IsAbstract(def("d", map[string]interface{}{"abstract": "true"})))
}
