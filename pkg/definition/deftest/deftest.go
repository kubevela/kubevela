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

// Package deftest holds test helpers for definitions written in CUE.
package deftest

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/pkg/definition"
)

// Template returns the CUE template of the definition source at path, the same
// one `vela def render` puts in the chart, so a test of it cannot drift from
// what runs in a cluster.
func Template(t *testing.T, path string) string {
	t.Helper()
	source, err := os.ReadFile(path)
	require.NoError(t, err)
	def := definition.Definition{Unstructured: unstructured.Unstructured{}}
	require.NoError(t, def.FromCUEString(string(source), nil))
	tpl, found, err := unstructured.NestedString(def.Object, definition.DefinitionTemplateKeys...)
	require.NoError(t, err)
	require.True(t, found, "definition at %s has no template", path)
	return tpl
}
