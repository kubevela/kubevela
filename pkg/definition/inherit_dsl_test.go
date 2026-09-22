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

package definition

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// `vela def` authors a definition in its own CUE dialect, where `attributes`
// becomes the spec. These pin how `extends` is written there, because a key the
// converter does not recognise is dropped in silence: a definition would apply
// cleanly and simply not inherit anything.
func TestExtendsThroughTheDefinitionDSL(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{
			name: "top level, beside type",
			src: `
"tenant-webservice": {
	type:    "component"
	extends: "webservice"
	attributes: {}
}
template: {
	$super: properties: {image: parameter.image}
	parameter: {tenant: string}
}
`,
		},
		{
			name: "top level, after attributes",
			src: `
"tenant-webservice": {
	type: "component"
	attributes: {
		workload: definition: {apiVersion: "apps/v1", kind: "Deployment"}
	}
	extends: "webservice"
}
template: {
	$super: properties: {image: parameter.image}
	parameter: {tenant: string}
}
`,
		},
		{
			name: "inside attributes, which is the spec",
			src: `
"tenant-webservice": {
	type: "component"
	attributes: {
		extends: "webservice"
	}
}
template: {
	$super: properties: {image: parameter.image}
	parameter: {tenant: string}
}
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			def := Definition{Unstructured: unstructured.Unstructured{}}
			require.NoError(t, def.FromCUEString(tc.src, nil))

			extends, found, err := unstructured.NestedString(def.Object, "spec", "extends")
			require.NoError(t, err)
			require.True(t, found, "spec.extends was dropped, so the definition would apply and inherit nothing")
			require.Equal(t, "webservice", extends)
		})
	}
}

// Declaring the same thing at the top level and under attributes is a mistake
// worth naming: whichever the loop reads last wins and the other is discarded
// without a word.
func TestDeclaringExtendsTwiceIsRefused(t *testing.T) {
	def := &Definition{Unstructured: unstructured.Unstructured{}}
	err := def.FromCUEString(`
"conflicted": {
	type:    "component"
	extends: "webservice"
	attributes: extends: "something-else"
}
template: {
	$super: properties: {}
	parameter: {}
}
`, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "declared twice")
}

func TestDeclaringAbstractTwiceIsRefused(t *testing.T) {
	def := &Definition{Unstructured: unstructured.Unstructured{}}
	err := def.FromCUEString(`
"conflicted": {
	type:     "component"
	abstract: true
	attributes: abstract: false
}
template: {
	output: {}
	parameter: {}
}
`, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "declared twice")
}

// The refusal must not depend on which was written first. CUE hands fields back
// in the order they were declared, so a guard that only remembers what it has
// already seen misses the case where attributes leads and lets the top-level
// value overwrite it in silence.
func TestDeclaringExtendsTwiceIsRefusedWhateverTheOrder(t *testing.T) {
	def := &Definition{Unstructured: unstructured.Unstructured{}}
	err := def.FromCUEString(`
"conflicted": {
	type: "component"
	attributes: extends: "something-else"
	extends: "webservice"
}
template: {
	$super: properties: {}
	parameter: {}
}
`, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "declared twice")
}

func TestDeclaringAbstractTwiceIsRefusedWhateverTheOrder(t *testing.T) {
	def := &Definition{Unstructured: unstructured.Unstructured{}}
	err := def.FromCUEString(`
"conflicted": {
	type: "component"
	attributes: abstract: false
	abstract: true
}
template: {
	output: {}
	parameter: {}
}
`, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "declared twice")
}

// Declaring it once under attributes, which is where it lives in the spec, is
// the ordinary way and stays fine.
func TestExtendsUnderAttributesAloneIsFine(t *testing.T) {
	def := &Definition{Unstructured: unstructured.Unstructured{}}
	require.NoError(t, def.FromCUEString(`
"attributed": {
	type: "component"
	attributes: extends: "webservice"
}
template: {
	$super: properties: {}
	parameter: {}
}
`, nil))
}
