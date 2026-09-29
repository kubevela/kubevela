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
	"testing"

	"github.com/kubevela/workflow/pkg/cue/model"
	wfprocess "github.com/kubevela/workflow/pkg/cue/process"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/definition/inherit"
	"github.com/oam-dev/kubevela/pkg/sources"
)

// A dry-run standing in for a component read, and a validation typing a source
// expression, both render a parameter whose value is not known yet as its CUE
// type. Every rendered resource has to come out marshallable, with the
// unknowable field left out and everything concrete kept. These pin that for
// each shape a definition can render.

var openRenders = map[string]func(context.Context) context.Context{
	"placeholder dry-run":  sources.WithComponentPlaceholders,
	"type-only validation": sources.WithTypeOnly,
}

func openCtx(open func(context.Context) context.Context) wfprocess.Context {
	return process.NewContext(process.ContextData{
		AppName: "acme-billing", CompName: "billing-api", Namespace: "acme", AppRevisionName: "acme-billing-v1",
		ClusterVersion: types.ClusterVersion{Minor: "19+"},
		Ctx:            open(context.Background()),
	})
}

func unknown() sources.CUEType { return sources.CUEType("string") }

func marshalled(t *testing.T, ins model.Instance) *unstructured.Unstructured {
	t.Helper()
	u, err := ins.Unstructured()
	require.NoError(t, err, "the rendered resource marshals")
	return u
}

const openWorkload = `
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: {
		name: context.name
		annotations: image: parameter.image
	}
	spec: replicas: parameter.replicas
}
parameter: {
	image:    string
	replicas: *1 | int
}
`

func TestOpenRenderOfAComponent(t *testing.T) {
	for name, open := range openRenders {
		t.Run(name, func(t *testing.T) {
			ctx := openCtx(open)
			require.NoError(t, NewWorkloadAbstractEngine("webservice").Complete(ctx, openWorkload,
				map[string]interface{}{"image": unknown(), "replicas": 2}))
			base, _ := ctx.Output()
			w := marshalled(t, base)
			require.NotContains(t, w.GetAnnotations(), "image", "the unknowable field is left out")
			replicas, _, _ := unstructured.NestedInt64(w.Object, "spec", "replicas")
			require.Equal(t, int64(2), replicas, "a concrete field is kept")
		})
	}
}

func TestOpenRenderOfAPatchingTrait(t *testing.T) {
	const trait = `
patch: metadata: labels: tier: parameter.tier
parameter: tier: string
`
	for name, open := range openRenders {
		t.Run(name, func(t *testing.T) {
			ctx := openCtx(open)
			require.NoError(t, NewWorkloadAbstractEngine("webservice").Complete(ctx, openWorkload,
				map[string]interface{}{"image": "acme/billing:1.4.2"}))
			require.NoError(t, NewTraitAbstractEngine("tier").Complete(ctx, trait,
				map[string]interface{}{"tier": unknown()}))
			base, _ := ctx.Output()
			w := marshalled(t, base)
			require.NotContains(t, w.GetLabels(), "tier", "the patched-in unknowable field is left out")
			require.Equal(t, "acme/billing:1.4.2", w.GetAnnotations()["image"], "the workload's own fields are kept")
		})
	}
}

func TestOpenRenderOfAnAuxiliary(t *testing.T) {
	const trait = `
outputs: svc: {
	apiVersion: "v1"
	kind:       "Service"
	metadata: name: context.name
	spec: clusterIP: parameter.ip
}
parameter: ip: string
`
	for name, open := range openRenders {
		t.Run(name, func(t *testing.T) {
			ctx := openCtx(open)
			require.NoError(t, NewWorkloadAbstractEngine("webservice").Complete(ctx, openWorkload,
				map[string]interface{}{"image": "acme/billing:1.4.2"}))
			require.NoError(t, NewTraitAbstractEngine("expose").Complete(ctx, trait,
				map[string]interface{}{"ip": unknown()}))
			_, assists := ctx.Output()
			require.Len(t, assists, 1)
			svc := marshalled(t, assists[0].Ins)
			_, found, _ := unstructured.NestedString(svc.Object, "spec", "clusterIP")
			require.False(t, found, "the unknowable field is left out")
			require.Equal(t, "Service", svc.GetKind(), "a concrete field is kept")
		})
	}
}

func TestOpenRenderOfAnInheritedChain(t *testing.T) {
	const child = `
$super: properties: {image: parameter.image}

output: metadata: labels: "tenant.oam.dev/name": parameter.tenant

outputs: quota: {
	apiVersion: "v1"
	kind:       "ResourceQuota"
	metadata: name: context.name
}

parameter: {
	image:  string
	tenant: string
}
`
	for name, open := range openRenders {
		t.Run(name, func(t *testing.T) {
			ctx := openCtx(open)
			engine := NewWorkloadAbstractEngine("tenant-webservice", inherit.Level{Name: "webservice", Template: parentComponent})
			require.NoError(t, engine.Complete(ctx, child, map[string]interface{}{"image": unknown(), "tenant": "acme"}))
			base, assists := ctx.Output()
			w := marshalled(t, base)
			require.Equal(t, "Deployment", w.GetKind(), "the parent's workload is rendered")
			require.Equal(t, "acme", w.GetLabels()["tenant.oam.dev/name"], "the child's concrete overlay is kept")
			for _, a := range assists {
				marshalled(t, a.Ins)
			}
		})
	}
}
