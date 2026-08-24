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
	"context"
	"os"
	"strings"
	"testing"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/addon/service/api"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/features"
)

// enableAddonComponent turns the gate on for tests that render the addon
// ComponentDefinition. The vela/addon provider refuses outright when the gate is
// off, and feature gates default to false.
func enableAddonComponent(t *testing.T) {
	t.Helper()
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate,
		features.EnableAddonComponent, true)
}

// addonTemplateDefPath is the addon ComponentDefinition under test.
const addonTemplateDefPath = "../../../vela-templates/definitions/internal/component/addon.cue"

type fakeAddonRenderer struct {
	req api.AddonRequest
	res *api.AddonResult
	err error
}

func (f *fakeAddonRenderer) RenderAddon(_ context.Context, req api.AddonRequest) (*api.AddonResult, error) {
	f.req = req
	return f.res, f.err
}

// extractAbstractTemplate mirrors how pkg/definition stores an internal
// definition: the file-level imports prepended to the body of the top-level
// "template" field. It parses the real .cue file so the test never drifts from
// the shipped definition.
func extractAbstractTemplate(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	f, err := parser.ParseFile(path, raw, parser.ParseComments)
	require.NoError(t, err)

	var importDecls, templateDecls []ast.Decl
	for _, decl := range f.Decls {
		if imp, ok := decl.(*ast.ImportDecl); ok {
			importDecls = append(importDecls, imp)
			continue
		}
		field, ok := decl.(*ast.Field)
		if !ok {
			continue
		}
		if lit, ok := field.Label.(*ast.Ident); ok && lit.Name == "template" {
			if st, ok := field.Value.(*ast.StructLit); ok {
				templateDecls = append(templateDecls, st.Elts...)
			}
		}
	}
	require.NotEmpty(t, templateDecls, "no template field found in %s", path)

	var sb strings.Builder
	for _, d := range importDecls {
		b, err := format.Node(d)
		require.NoError(t, err)
		sb.Write(b)
		sb.WriteString("\n")
	}
	for _, d := range templateDecls {
		b, err := format.Node(d)
		require.NoError(t, err)
		sb.Write(b)
		sb.WriteString("\n")
	}
	return sb.String()
}

// TestAddonComponentDefinitionRenders compiles the real addon ComponentDefinition
// through the WorkloadCompiler with a fake renderer installed, and asserts the
// component's output is the addon Application and each auxiliary lands in outputs.
func TestAddonComponentDefinitionRenders(t *testing.T) {
	enableAddonComponent(t)

	prev := api.DefaultRenderer()
	t.Cleanup(func() { api.SetDefaultRenderer(prev) })

	fake := &fakeAddonRenderer{
		res: &api.AddonResult{
			ResolvedVersion: "1.0.0",
			Registry:        "my-registry",
			Application: map[string]interface{}{
				"apiVersion": "core.oam.dev/v1beta1",
				"kind":       "Application",
				"metadata":   map[string]interface{}{"name": "addon-example", "namespace": "vela-system"},
			},
		},
	}
	api.SetDefaultRenderer(fake)

	abstractTemplate := extractAbstractTemplate(t, addonTemplateDefPath)

	ctx := process.NewContext(process.ContextData{
		AppName:         "myapp",
		CompName:        "example",
		Namespace:       "default",
		AppRevisionName: "myapp-v1",
		ClusterVersion:  types.ClusterVersion{Minor: "19+"},
	})

	wt := NewWorkloadAbstractEngine("addon")
	err := wt.Complete(ctx, abstractTemplate, map[string]interface{}{
		"addon":                 "example",
		"version":               "1.0.0",
		"properties":            map[string]interface{}{"foo": "bar"},
		"skipVersionValidation": true,
	})
	require.NoError(t, err)

	// The renderer received the component parameters.
	assert.Equal(t, "example", fake.req.Name)
	assert.Equal(t, "1.0.0", fake.req.Version)
	assert.Equal(t, map[string]interface{}{"foo": "bar"}, fake.req.Properties)
	assert.True(t, fake.req.SkipVersionValidate)

	base, assists := ctx.Output()
	require.NotNil(t, base)
	baseObj, err := base.Unstructured()
	require.NoError(t, err)
	assert.Equal(t, "Application", baseObj.GetKind())
	assert.Equal(t, "core.oam.dev/v1beta1", baseObj.GetAPIVersion())
	assert.Equal(t, "addon-example", baseObj.GetName())

	// The component now emits only output (the self-contained Application);
	// auxiliaries are folded into the Application by the render service, so the
	// definition produces no outputs.
	assert.Empty(t, assists, "addon component must emit no outputs")
}

// TestAddonComponentDefinitionDefaultsAddonToComponentName verifies the
// parameter default: when addon is omitted, it falls back to context.name.
func TestAddonComponentDefinitionDefaultsAddonToComponentName(t *testing.T) {
	enableAddonComponent(t)

	prev := api.DefaultRenderer()
	t.Cleanup(func() { api.SetDefaultRenderer(prev) })

	fake := &fakeAddonRenderer{
		res: &api.AddonResult{
			Application: map[string]interface{}{
				"apiVersion": "core.oam.dev/v1beta1",
				"kind":       "Application",
				"metadata":   map[string]interface{}{"name": "addon-fluxcd"},
			},
		},
	}
	api.SetDefaultRenderer(fake)

	abstractTemplate := extractAbstractTemplate(t, addonTemplateDefPath)

	ctx := process.NewContext(process.ContextData{
		AppName:         "myapp",
		CompName:        "fluxcd",
		Namespace:       "default",
		AppRevisionName: "myapp-v1",
		ClusterVersion:  types.ClusterVersion{Minor: "19+"},
	})

	wt := NewWorkloadAbstractEngine("addon")
	require.NoError(t, wt.Complete(ctx, abstractTemplate, map[string]interface{}{}))

	assert.Equal(t, "fluxcd", fake.req.Name)
	assert.False(t, fake.req.SkipVersionValidate)

	base, _ := ctx.Output()
	require.NotNil(t, base)
	baseObj, err := base.Unstructured()
	require.NoError(t, err)
	assert.Equal(t, "Application", baseObj.GetKind())
}
