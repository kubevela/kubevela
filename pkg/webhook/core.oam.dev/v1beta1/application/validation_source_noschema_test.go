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

package application

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// A binding whose SourceDefinition declares no schema, or cannot be loaded, is
// declared like any other: its reads fit anything, and a definition that cannot
// be loaded is reported once, as that.
func TestABindingWithNoSchemaIsStillDeclared(t *testing.T) {
	defs := append(readKindsDefinitions(), &v1beta1.SourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "raw-http", Namespace: "vela-system"},
		Spec: v1beta1.SourceDefinitionSpec{Schematic: &common.Schematic{CUE: &common.CUE{Template: `
output: {body: "x"}
parameter: {}
`}}},
	})
	validate := func(t *testing.T, sourceType, props string) []string {
		t.Helper()
		cli := fake.NewClientBuilder().WithScheme(readKindsScheme()).WithRuntimeObjects(defs...).Build()
		app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
			Spec: v1beta1.ApplicationSpec{
				Sources:    []v1beta1.ApplicationSource{{Name: "raw", Type: sourceType, Properties: rawJSON(`{}`)}},
				Components: []common.ApplicationComponent{{Name: "api", Type: "server", Properties: rawJSON(props)}},
			}}
		var out []string
		for _, e := range (&ValidatingHandler{Client: cli}).ValidateSources(context.Background(), app) {
			out = append(out, e.Error())
		}
		return out
	}

	t.Run("a read of a binding with no schema fits anything", func(t *testing.T) {
		require.Empty(t, validate(t, "raw-http", `{"image":"$(source.raw.body)","replicas":"$(source.raw.count)"}`))
	})
	t.Run("the whole binding too", func(t *testing.T) {
		require.Empty(t, validate(t, "raw-http", `{"note":"$(source.raw.body)"}`))
	})
	t.Run("a definition that cannot be loaded is reported once", func(t *testing.T) {
		var atRead []string
		for _, e := range validate(t, "gone", `{"image":"$(source.raw.body)"}`) {
			if strings.HasPrefix(e, "spec.components[0].properties.image") {
				atRead = append(atRead, e)
			}
		}
		require.Len(t, atRead, 1, "errors at the read: %v", atRead)
		require.Contains(t, atRead[0], `failed to load SourceDefinition "gone"`)
	})
}
