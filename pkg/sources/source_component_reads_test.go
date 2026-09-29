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

package sources

import (
	"context"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"

	wfprocess "github.com/kubevela/workflow/pkg/cue/process"

	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func componentReadsContext(t *testing.T) wfprocess.Context {
	t.Helper()
	ctx := typingContext(t)
	ctx.PushData(process.ContextAppAnnotations, map[string]string{oam.AnnotationCelExpressions: "true"})
	return ctx
}

func view(fields map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"output": fields}
}

// withScope is a render context carrying delivered reads, as the controller
// hands them over.
func withScope(t *testing.T, scope map[string]interface{}) wfprocess.Context {
	ctx := componentReadsContext(t)
	ctx.SetCtx(WithComponentScope(context.Background(), scope))
	return ctx
}

func TestComponentReadsResolve(t *testing.T) {
	db := view(map[string]interface{}{"status": map[string]interface{}{"endpoint": "db.internal", "port": float64(5432)}})
	db[propexpr.QualifiedKey] = map[string]interface{}{
		propexpr.PlacementCall(propexpr.PlaceCluster, "data"): view(map[string]interface{}{"status": map[string]interface{}{"endpoint": "db.data"}}),
	}
	out, err := ResolveSourceExpressions(withScope(t, map[string]interface{}{"db": db}), map[string]interface{}{
		"url":    "pg://$(component.db.output.status.endpoint):5432",
		"port":   "$(component.db.output.status.port)",
		"remote": `$(component.db.cluster("data").output.status.endpoint)`,
		"image":  "nginx",
	}, SurfaceComponent)
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{
		"url":    "pg://db.internal:5432",
		"port":   int64(5432),
		"remote": "db.data",
		"image":  "nginx",
	}, out)
}

// A trait renders on its component's context, so it reads the same delivery.
func TestComponentReadsReachTheComponentsTraits(t *testing.T) {
	ctx := withScope(t, map[string]interface{}{
		"db": view(map[string]interface{}{"metadata": map[string]interface{}{"name": "db-0"}}),
	})
	out, err := ResolveSourceExpressions(ctx, map[string]interface{}{
		"labels": map[string]interface{}{"db": "$(component.db.output.metadata.name)"},
	}, SurfaceTrait)
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{"labels": map[string]interface{}{"db": "db-0"}}, out)
}

func TestComponentReadsWithoutAValue(t *testing.T) {
	delivered := map[string]interface{}{"db": view(map[string]interface{}{"status": map[string]interface{}{}})}

	t.Run("nothing delivered is an error", func(t *testing.T) {
		_, err := ResolveSourceExpressions(componentReadsContext(t),
			map[string]interface{}{"url": "$(component.db.output.status.endpoint)"}, SurfaceComponent)
		require.ErrorContains(t, err, `component.db.output.status.endpoint has no value in this render`)
		require.False(t, IsComponentReadNotReady(err))
	})

	t.Run("a field not there yet is not ready", func(t *testing.T) {
		_, err := ResolveSourceExpressions(withScope(t, delivered), map[string]interface{}{
			"url": "$(component.db.output.status.endpoint)",
		}, SurfaceComponent)
		require.True(t, IsComponentReadNotReady(err), "%v", err)
	})

	t.Run("a guard sees it missing", func(t *testing.T) {
		out, err := ResolveSourceExpressions(withScope(t, delivered), map[string]interface{}{
			"url": `$(has(component.db.output.status.endpoint) ? component.db.output.status.endpoint : "pending")`,
		}, SurfaceComponent)
		require.NoError(t, err)
		require.Equal(t, map[string]interface{}{"url": "pending"}, out)
	})

	t.Run("a surface that cannot read components does not see the delivery", func(t *testing.T) {
		_, err := ResolveSourceExpressions(withScope(t, delivered), map[string]interface{}{
			"url": "$(component.db.output.status.endpoint)",
		}, SurfaceWorkflowStep)
		require.ErrorContains(t, err, `has no value in this render`)
	})
}

// A dry-run has no live producers, so a component read renders as its own text
// while everything else in the value still evaluates.
func TestComponentReadsRenderAsPlaceholders(t *testing.T) {
	ctx := componentReadsContext(t)
	ctx.SetCtx(WithComponentPlaceholders(context.Background()))
	out, err := ResolveSourceExpressions(ctx, map[string]interface{}{
		"url":   "postgres://$(component.db.output.status.endpoint):5432/$(context.appName)",
		"whole": `$(component.cache.cluster("east").output.spec.replicas)`,
		"plain": "$(context.appName)",
	}, SurfaceComponent)
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{
		"url": "postgres://<component.db.output.status.endpoint>:5432/app",
		// A whole value's type is unknowable: the placeholder where text fits,
		// otherwise unchecked.
		"whole": CUEType(`*"<component.cache.cluster(\"east\").output.spec.replicas>" | _`),
		"plain": "app",
	}, out)
}

// A placeholder dry-run takes the placeholder a text field defaults to; a
// validation keeps treating any open leaf as unknowable.
func TestOpenRenderPruning(t *testing.T) {
	v := cuecontext.New().CompileString(`{text: (*"<ph>" | _) & string, num: (*"<ph>" | _) & int, env: [{v: (*"<ph>" | _) & string}]}`)

	dry, _ := ConcreteForOpenRender(WithComponentPlaceholders(context.Background()), v)
	var got map[string]interface{}
	require.NoError(t, dry.Decode(&got))
	require.Equal(t, map[string]interface{}{"text": "<ph>", "env": []interface{}{map[string]interface{}{"v": "<ph>"}}}, got)

	validation, _ := ConcreteForOpenRender(WithTypeOnly(context.Background()), v)
	got = nil
	require.NoError(t, validation.Decode(&got))
	require.Equal(t, map[string]interface{}{"env": []interface{}{map[string]interface{}{}}}, got)
}

// A field that is there but null is there: only an absent one is waited for.
func TestComponentReadOfANullFieldIsNotWaitedOn(t *testing.T) {
	delivered := map[string]interface{}{"db": view(map[string]interface{}{"status": map[string]interface{}{"endpoint": nil}})}
	out, err := ResolveSourceExpressions(withScope(t, delivered), map[string]interface{}{
		"url": `$(component.db.output.status.endpoint == null ? "none" : "some")`,
	}, SurfaceComponent)
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{"url": "none"}, out)
}

// An element not there yet is a wait, like a field not there yet: a list in a
// status is often filled in after the resource is first healthy.
func TestComponentReadOfAMissingElementWaits(t *testing.T) {
	delivered := map[string]interface{}{"db": view(map[string]interface{}{"status": map[string]interface{}{"addresses": []interface{}{}}})}
	_, err := ResolveSourceExpressions(withScope(t, delivered), map[string]interface{}{
		"url": `$(component.db.output.status.addresses[0])`,
	}, SurfaceComponent)
	require.True(t, IsComponentReadNotReady(err), "%v", err)
}
