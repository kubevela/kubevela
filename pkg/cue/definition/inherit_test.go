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

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	wfprocess "github.com/kubevela/workflow/pkg/cue/process"
	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/definition/inherit"
)

func renderCtx() wfprocess.Context {
	return process.NewContext(process.ContextData{
		AppName:         "acme-billing",
		CompName:        "billing-api",
		Namespace:       "acme",
		AppRevisionName: "acme-billing-v1",
		ClusterVersion:  types.ClusterVersion{Minor: "19+"},
	})
}

const parentComponent = `
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: name: context.name
	spec: {
		replicas: parameter.replicas
		template: spec: containers: [{
			name:  context.name
			image: parameter.image
		}]
	}
}

outputs: svc: {
	apiVersion: "v1"
	kind:       "Service"
	metadata: name: context.name
}

parameter: {
	image:    string
	replicas: *1 | int
}
`

func TestComponentEngineRendersAnInheritedChain(t *testing.T) {
	child := `
$super: properties: {image: parameter.image}

output: metadata: labels: "tenant.oam.dev/name": parameter.tenant

outputs: quota: {
	apiVersion: "v1"
	kind:       "ResourceQuota"
	metadata: name: context.name
}

parameter: $super.parameter & {
	tenant: string
}
`
	ctx := renderCtx()
	engine := NewWorkloadAbstractEngine("tenant-webservice",
		inherit.Level{Name: "webservice", Template: parentComponent})

	require.NoError(t, engine.Complete(ctx, child, map[string]interface{}{
		"image":  "acme/billing:1.4.2",
		"tenant": "acme",
	}))

	base, assists := ctx.Output()
	require.NotNil(t, base)

	workload, err := base.Unstructured()
	require.NoError(t, err)
	require.Equal(t, "Deployment", workload.GetKind(), "the parent's workload reached the engine")
	require.Equal(t, "acme", workload.GetLabels()["tenant.oam.dev/name"], "the child's overlay came with it")

	// The parent's Service and the child's quota both became auxiliaries.
	names := map[string]bool{}
	for _, a := range assists {
		names[a.Name] = true
	}
	require.True(t, names["svc"], "the parent's outputs survived, got %v", names)
	require.True(t, names["quota"], "the child's outputs were added, got %v", names)
}

// A parent's authored `errs:` must reach the engine that rendered its child,
// or a refusal is reported as an unexplained conflict further down.
func TestParentRefusalSurfacesThroughTheEngine(t *testing.T) {
	parent := parentComponent + `
if parameter.image == "banned" {
	errs: ["image 'banned' is not allowed by webservice"]
}
`
	engine := NewWorkloadAbstractEngine("tenant-webservice",
		inherit.Level{Name: "webservice", Template: parent})

	err := engine.Complete(renderCtx(), "$super: properties: {image: parameter.image}\nparameter: {tenant: string}",
		map[string]interface{}{"image": "banned", "tenant": "acme"})
	require.ErrorContains(t, err, "image 'banned' is not allowed by webservice")
}

// The strategy tag lives on the `patch` field's doc comment, which a merged
// value does not carry. Without recovering it from the level that declared it,
// the parent's patch degrades to plain unification and conflicts with the
// workload it was meant to replace.
func TestTraitEngineRecoversAParentJSONMergePatchStrategy(t *testing.T) {
	workloadTemplate := `
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: name: context.name
	spec: template: spec: containers: [{name: "main", image: "nginx"}]
}
parameter: {}
`
	parentTrait := `
parameter: {...}

// +patchStrategy=jsonMergePatch
patch: {
	spec: template: spec: containers: [{name: "only-me"}]
}
`
	childTrait := `
$super: properties: {}

patch: {
	metadata: annotations: "tenant.oam.dev/name": "acme"
}

parameter: {}
`
	ctx := renderCtx()
	require.NoError(t, NewWorkloadAbstractEngine("base").Complete(ctx, workloadTemplate, map[string]interface{}{}))

	engine := NewTraitAbstractEngine("tenant-override",
		inherit.Level{Name: "json-merge-patch", Template: parentTrait})
	require.NoError(t, engine.Complete(ctx, childTrait, map[string]interface{}{}))

	base, _ := ctx.Output()
	workload, err := base.Unstructured()
	require.NoError(t, err)

	containers, found := unstructuredSlice(workload.Object, "spec", "template", "spec", "containers")
	require.True(t, found)
	require.Len(t, containers, 1)
	require.Equal(t, "only-me", containers[0].(map[string]interface{})["name"],
		"a json merge patch replaces the list; plain unification would have conflicted")

	annotations := workload.GetAnnotations()
	require.Equal(t, "acme", annotations["tenant.oam.dev/name"], "the child's patch applied too")
}

// unstructuredSlice walks a path and reports whether it found a list there. It
// returns no error: every way of missing is the same "not found", and an error
// nobody can return only invites a check that proves nothing.
func unstructuredSlice(obj map[string]interface{}, fields ...string) ([]interface{}, bool) {
	cur := interface{}(obj)
	for _, f := range fields {
		m, ok := cur.(map[string]interface{})
		if !ok {
			return nil, false
		}
		cur, ok = m[f]
		if !ok {
			return nil, false
		}
	}
	out, ok := cur.([]interface{})
	return out, ok
}

// A patch strategy is a whole-patch mode, and the modes are mutually exclusive.
// Levels run root first, so the nearest declaration wins and a level that
// declares none defers outward.
func compiledPatch(t *testing.T, src string) cue.Value {
	t.Helper()
	v := cuecontext.New().CompileString(src)
	require.NoError(t, v.Err())
	return v
}

const mergePatchLevel = `
// +patchStrategy=jsonMergePatch
patch: {
	spec: replicas: 3
}
`

const plainPatchLevel = `
patch: {
	spec: template: spec: containers: [{name: "sidecar"}]
}
`

func TestAParentStrategyAppliesWhenTheChildDeclaresNone(t *testing.T) {
	levels := []cue.Value{
		compiledPatch(t, mergePatchLevel), // root
		compiledPatch(t, plainPatchLevel), // child
	}
	merged := compiledPatch(t, plainPatchLevel)

	opts := patchOptionsFromLevels(levels, merged)
	require.Len(t, opts, 1, "the root's strategy still applies")
}

func TestExactlyOneStrategyIsChosen(t *testing.T) {
	// Both levels declare one. Collecting from every level would hand the
	// merge two contradictory modes.
	levels := []cue.Value{
		compiledPatch(t, mergePatchLevel),
		compiledPatch(t, mergePatchLevel),
	}
	merged := compiledPatch(t, plainPatchLevel)

	require.Len(t, patchOptionsFromLevels(levels, merged), 1)
}

func TestNoStrategyAnywhereMeansOrdinaryUnification(t *testing.T) {
	levels := []cue.Value{compiledPatch(t, plainPatchLevel), compiledPatch(t, plainPatchLevel)}

	require.Empty(t, patchOptionsFromLevels(levels, compiledPatch(t, plainPatchLevel)))
}
