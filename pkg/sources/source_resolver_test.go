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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"cuelang.org/go/cue"
	upstreamcuex "github.com/kubevela/pkg/cue/cuex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	velacuex "github.com/oam-dev/kubevela/pkg/cue/cuex"
	"github.com/oam-dev/kubevela/pkg/cue/process"
)

func TestResolveSourceNode(t *testing.T) {
	sources := map[string]map[string]interface{}{
		"cluster-info": {
			"region": "us-east-1",
			"nested": map[string]interface{}{"tier": "prod"},
		},
	}
	// A hyphenated binding needs bracket form: source.cluster-info.region parses
	// as subtraction, which the grammar rejects with that explanation.
	in := map[string]interface{}{
		"region": `$(source["cluster-info"].region)`,
		"tier":   `$(source["cluster-info"].nested.tier)`,
	}
	resolver := newSourceResolver(context.Background(), map[string]interface{}{}, SurfaceComponent, sourceInputs{})
	resolver.resolved = sources
	resolver.sourceTypes = map[string]string{"cluster-info": "cluster"}
	got, err := resolveSourceNode(in, resolver)
	require.NoError(t, err)
	out, ok := got.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "us-east-1", out["region"])
	assert.Equal(t, "prod", out["tier"])
	statuses := resolver.statuses
	require.NotNil(t, statuses)
	assert.Equal(t, "us-east-1", statuses["cluster-info"].ConsumedFields["region"])
	assert.Equal(t, "prod", statuses["cluster-info"].ConsumedFields["nested.tier"])
}

func TestResolveChainedSourceProperties(t *testing.T) {
	ctx := process.NewContext(process.ContextData{})
	resolver := newSourceResolver(ctx.GetCtx(), contextValuesFor(ctx), SurfaceComponent, sourceInputsFromContext(ctx))
	resolver.sourceTypes = map[string]string{
		"sourceA": "typeA",
		"sourceB": "typeB",
	}
	resolver.sourceTemplates = map[string]string{
		"typeA": `
$internal: {key: "test-cache-key-1", keyInputs: []}
output: {
  nested: {
    image: {
      repo: parameter.repo
      tag:  parameter.tag
    }
  }
}
parameter: {
  repo: string
  tag:  string
}
`,
		"typeB": `
$internal: {key: "test-cache-key-2", keyInputs: []}
output: {
  resolved: {
    image: "\(parameter.repo):\(parameter.tag)"
  }
}
parameter: {
  repo: string
  tag:  string
}
`,
	}
	resolver.sourceProps = map[string]map[string]interface{}{
		"sourceA": {
			"repo": "nginx",
			"tag":  "1.25.2",
		},
		"sourceB": {
			"repo": "$(source.sourceA.nested.image.repo)",
			"tag":  "$(source.sourceA.nested.image.tag)",
		},
	}

	out, err := resolver.resolve("sourceB")
	require.NoError(t, err)
	resolved, ok := out["resolved"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "nginx:1.25.2", resolved["image"])
}

// Seeded cache entries have to be created under the identity the resolver will
// compute, which now covers the template as well as the properties - so both
// sides use the same text.
const resolver_stale_cache_use_template = `
$internal: {
	key: "stale-cache-use"
	keyInputs: []
}
storage: {
	storageTTL: "1ms"
	onStaleFailure: "use-stale"
}
output: {
  value: parameter.value
}
parameter: {
  value: string
}
`

const resolver_stale_cache_fail_template = `
$internal: {
	key: "stale-cache-fail"
	keyInputs: []
}
storage: {
	storageTTL: "1ms"
	onStaleFailure: "fail"
}
output: {
  value: parameter.value
}
parameter: {
  value: string
}
`

func TestResolveSourceUsesStaleCacheOnRefreshFailure(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	// The resolver appends a hash of the binding's properties to the declared
	// key, so the entry has to be seeded under that identity rather than the
	// key alone.
	staleProps := map[string]interface{}{"value": 1}
	cacheKey, err := cacheIdentity("stale-cache-use", identityInputs{
		Template:   templateFingerprint(resolver_stale_cache_use_template),
		Properties: staleProps,
	})
	require.NoError(t, err)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cacheKey,
			Namespace: CacheNamespace(),
			Annotations: map[string]string{
				sourceCacheSyncAtKey: time.Now().Add(-time.Hour).Format(time.RFC3339),
			},
		},
		Data: map[string][]byte{
			sourceCacheDataKey: []byte(`{"value":"cached"}`),
		},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	ctx := process.NewContext(process.ContextData{})
	ctx.PushData(process.ContextAppSourceCacheStore, NewSecretSourceCacheStore(cli))
	resolver := newSourceResolver(ctx.GetCtx(), contextValuesFor(ctx), SurfaceComponent, sourceInputsFromContext(ctx))
	resolver.sourceTypes = map[string]string{"s": "t"}
	resolver.sourceTemplates = map[string]string{
		"t": resolver_stale_cache_use_template,
	}
	// Invalid parameter type triggers refresh compile failure.
	resolver.sourceProps = map[string]map[string]interface{}{"s": staleProps}

	out, err := resolver.resolve("s")
	require.NoError(t, err)
	require.Equal(t, "cached", out["value"])
	statuses := resolver.statuses
	require.NotNil(t, statuses)
	assert.Equal(t, cacheKey, statuses["s"].Config)
	assert.NotEmpty(t, statuses["s"].ExpiresAt)
}

func TestResolveSourceFailsOnStaleRefreshFailureWhenPolicyFail(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	// The resolver appends a hash of the binding's properties to the declared
	// key, so the entry has to be seeded under that identity rather than the
	// key alone.
	staleProps := map[string]interface{}{"value": 1}
	cacheKey, err := cacheIdentity("stale-cache-fail", identityInputs{
		Template:   templateFingerprint(resolver_stale_cache_fail_template),
		Properties: staleProps,
	})
	require.NoError(t, err)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cacheKey,
			Namespace: CacheNamespace(),
			Annotations: map[string]string{
				sourceCacheSyncAtKey: time.Now().Add(-time.Hour).Format(time.RFC3339),
			},
		},
		Data: map[string][]byte{
			sourceCacheDataKey: []byte(`{"value":"cached"}`),
		},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	ctx := process.NewContext(process.ContextData{})
	ctx.PushData(process.ContextAppSourceCacheStore, NewSecretSourceCacheStore(cli))
	resolver := newSourceResolver(ctx.GetCtx(), contextValuesFor(ctx), SurfaceComponent, sourceInputsFromContext(ctx))
	resolver.sourceTypes = map[string]string{"s": "t"}
	resolver.sourceTemplates = map[string]string{
		"t": resolver_stale_cache_fail_template,
	}
	resolver.sourceProps = map[string]map[string]interface{}{"s": staleProps}

	_, err = resolver.resolve("s")
	require.Error(t, err)
	statuses := resolver.statuses
	require.NotNil(t, statuses)
	assert.Equal(t, cacheKey, statuses["s"].Config)
}

func TestResolveSourceSchemaMismatchFails(t *testing.T) {
	ctx := process.NewContext(process.ContextData{})
	resolver := newSourceResolver(ctx.GetCtx(), contextValuesFor(ctx), SurfaceComponent, sourceInputsFromContext(ctx))
	resolver.sourceTypes = map[string]string{"s": "t"}
	resolver.sourceTemplates = map[string]string{
		"t": `
$internal: {key: "test-cache-key-3", keyInputs: []}
schema: {
  image: string
}
output: {
  image: parameter.image
}
parameter: {
  image: _
}
`,
	}
	resolver.sourceProps = map[string]map[string]interface{}{
		"s": {"image": 123},
	}

	_, err := resolver.resolve("s")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validate output against schema")
}

func TestResolveSourceErrsFieldFails(t *testing.T) {
	ctx := process.NewContext(process.ContextData{})
	resolver := newSourceResolver(ctx.GetCtx(), contextValuesFor(ctx), SurfaceComponent, sourceInputsFromContext(ctx))
	resolver.sourceTypes = map[string]string{"s": "t"}
	resolver.sourceTemplates = map[string]string{
		"t": `
$internal: {key: "test-cache-key-4", keyInputs: []}
output: {
  value: parameter.value
}
errs: [
  if parameter.value < 0 {
    "value must be non-negative, got \(parameter.value)"
  },
]
parameter: {
  value: int
}
`,
	}
	resolver.sourceProps = map[string]map[string]interface{}{
		"s": {"value": -1},
	}

	_, err := resolver.resolve("s")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reported errors")
	assert.Contains(t, err.Error(), "value must be non-negative, got -1")

	// The authored error is surfaced on the per-source status too. Read off the
	// resolver: only the bridge pushes these onto a render context, and this test
	// drives the resolver directly.
	statuses := resolver.statuses
	require.Contains(t, statuses, "s")
	assert.Equal(t, "Failed", statuses["s"].Phase)
	assert.Contains(t, statuses["s"].Message, "value must be non-negative")
}

func TestResolveSourceErrsFieldEmptyIsIgnored(t *testing.T) {
	ctx := process.NewContext(process.ContextData{})
	resolver := newSourceResolver(ctx.GetCtx(), contextValuesFor(ctx), SurfaceComponent, sourceInputsFromContext(ctx))
	resolver.sourceTypes = map[string]string{"s": "t"}
	resolver.sourceTemplates = map[string]string{
		"t": `
$internal: {key: "test-cache-key-5", keyInputs: []}
output: {
  value: parameter.value
}
errs: [
  if parameter.value < 0 {
    "value must be non-negative"
  },
]
parameter: {
  value: int
}
`,
	}
	resolver.sourceProps = map[string]map[string]interface{}{
		"s": {"value": 5},
	}

	out, err := resolver.resolve("s")
	require.NoError(t, err)
	assert.EqualValues(t, 5, out["value"])
}

// A source whose definition sets no storageTTL has no expiry, and formatting
// the zero time put "0001-01-01T00:00:00Z" into Application status - a date,
// where the honest answer is silence.
// A source whose definition sets no storageTTL has no expiry, and formatting
// the zero time put "0001-01-01T00:00:00Z" into Application status - a date,
// where the honest answer is silence.
func TestFormatExpiryOmitsTheZeroTime(t *testing.T) {
	if got := formatExpiry(time.Time{}); got != "" {
		t.Fatalf("zero time should render as empty, got %q", got)
	}
	at := time.Date(2026, 8, 19, 15, 4, 5, 0, time.UTC)
	if got := formatExpiry(at); got != "2026-08-19T15:04:05Z" {
		t.Fatalf("real expiry should render RFC3339, got %q", got)
	}
}

// recordingCompiler stands in for the workload compiler so a test can prove the
// resolver used what it was given rather than the package singleton.
// recordingCompiler stands in for the workload compiler so a test can prove the
// resolver used what it was given rather than the package singleton.
type recordingCompiler struct {
	inner  SourceCompiler
	called int
}

func (c *recordingCompiler) CompileString(ctx context.Context, src string) (cue.Value, error) {
	c.called++
	return c.inner.CompileString(ctx, src)
}

func (c *recordingCompiler) CompileStringWithOptions(ctx context.Context, src string,
	opts ...upstreamcuex.CompileOption) (cue.Value, error) {
	c.called++
	return c.inner.CompileStringWithOptions(ctx, src, opts...)
}

// A supplied compiler has to be the one that runs. A default that quietly fell
// back to the singleton would behave identically for the Application and leave a
// second caller's providers unused, with nothing to notice.
// A supplied compiler has to be the one that runs. A default that quietly fell
// back to the singleton would behave identically for the Application and leave a
// second caller's providers unused, with nothing to notice.
func TestResolverUsesTheSuppliedCompiler(t *testing.T) {
	pCtx := process.NewContext(process.ContextData{Namespace: "default", CompName: "web", AppName: "app"})
	pCtx.PushData(process.ContextAppSources, map[string]map[string]interface{}{"s": {}})
	pCtx.PushData(process.ContextAppSourceTypes, map[string]string{"s": "demo"})
	pCtx.PushData(process.ContextAppSourceTemplates, map[string]string{"demo": `
schema: {v: string}
$internal: {key: "demo", keyInputs: []}
output: {v: "hello"}
`})

	spy := &recordingCompiler{inner: velacuex.WorkloadCompiler.Get()}
	in := sourceInputsFromContext(pCtx)
	in.Compiler = spy

	r := newSourceResolver(pCtx.GetCtx(), contextValuesFor(pCtx), SurfaceComponent, in)
	if _, err := r.resolve("s"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if spy.called == 0 {
		t.Fatal("the resolver ignored the supplied compiler and used the singleton")
	}

	// ...and nil still means the workload compiler, so the Application path is
	// unchanged.
	plain := newSourceResolver(pCtx.GetCtx(), contextValuesFor(pCtx), SurfaceComponent, sourceInputsFromContext(pCtx))
	if plain.compiler == nil {
		t.Fatal("a nil compiler must default, not stay nil")
	}
}

// resolverFor builds a resolver over the given bindings, with no cache.
func resolverFor(t *testing.T, in sourceInputs) *sourceResolver {
	t.Helper()
	return newSourceResolver(context.Background(), map[string]interface{}{}, SurfaceComponent, in)
}

// The failure paths, which are where a resolver either says what is wrong or
// leaves an operator staring at an empty status.
func TestResolveReportsWhyItCannot(t *testing.T) {
	t.Run("a binding nothing declares", func(t *testing.T) {
		r := resolverFor(t, sourceInputs{})
		_, err := r.resolve("ghost")
		require.ErrorContains(t, err, `source "ghost" not found`)
		require.Equal(t, PhaseFailed, r.statuses["ghost"].Phase)
		require.Contains(t, r.statuses["ghost"].Message, "not found",
			"status has to carry the cause, not just the failure")
	})

	t.Run("a type with no template", func(t *testing.T) {
		r := resolverFor(t, sourceInputs{
			Types:     map[string]string{"cfg": "http-get"},
			Templates: map[string]string{},
		})
		_, err := r.resolve("cfg")
		require.ErrorContains(t, err, "missing cue template")
		require.Equal(t, PhaseFailed, r.statuses["cfg"].Phase)
	})

	t.Run("a template that is the empty string", func(t *testing.T) {
		r := resolverFor(t, sourceInputs{
			Types:     map[string]string{"cfg": "http-get"},
			Templates: map[string]string{"http-get": ""},
		})
		_, err := r.resolve("cfg")
		require.ErrorContains(t, err, "missing cue template")
	})

	t.Run("properties that read a binding that does not exist", func(t *testing.T) {
		r := resolverFor(t, sourceInputs{
			Types:     map[string]string{"cfg": "t"},
			Templates: map[string]string{"t": `schema: {}` + "\n" + `output: {}`},
			Bindings:  map[string]map[string]interface{}{"cfg": {"x": "$(source.absent.v)"}},
		})
		_, err := r.resolve("cfg")
		require.Error(t, err)
		require.Contains(t, err.Error(), "resolve source properties for cfg",
			"the error names the binding whose properties failed, not just the inner cause")
		require.Equal(t, PhaseFailed, r.statuses["cfg"].Phase)
	})
}

// A source whose properties read itself, directly or through another, must be
// refused rather than recursing until the stack gives out.
func TestResolveRefusesACircularChain(t *testing.T) {
	t.Run("direct", func(t *testing.T) {
		r := resolverFor(t, sourceInputs{
			Types:     map[string]string{"a": "t"},
			Templates: map[string]string{"t": "schema: {}\noutput: {}"},
			Bindings:  map[string]map[string]interface{}{"a": {"x": "$(source.a.v)"}},
		})
		_, err := r.resolve("a")
		require.ErrorContains(t, err, "circular source dependency")
	})

	t.Run("through another binding", func(t *testing.T) {
		r := resolverFor(t, sourceInputs{
			Types:     map[string]string{"a": "t", "b": "t"},
			Templates: map[string]string{"t": "schema: {}\noutput: {}"},
			Bindings: map[string]map[string]interface{}{
				"a": {"x": "$(source.b.v)"},
				"b": {"x": "$(source.a.v)"},
			},
		})
		_, err := r.resolve("a")
		require.ErrorContains(t, err, "circular source dependency")
	})
}

// Resolving twice returns the first answer rather than fetching again: a source
// read by a component and its trait must not hit the network twice.
func TestResolveIsMemoisedWithinOneRender(t *testing.T) {
	r := resolverFor(t, sourceInputs{})
	r.resolved["cfg"] = map[string]interface{}{"v": "cached"}
	got, err := r.resolve("cfg")
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{"v": "cached"}, got)
}

func TestResolveSourceExpressionsOnEmptyAndBadInput(t *testing.T) {
	pCtx := process.NewContext(process.ContextData{Namespace: "default", CompName: "web", AppName: "app"})

	out, err := ResolveSourceExpressions(pCtx, nil, SurfaceComponent)
	require.NoError(t, err)
	require.Nil(t, out, "no properties, no work")

	_, err = ResolveSourceExpressions(pCtx, map[string]interface{}{"ch": make(chan int)}, SurfaceComponent)
	require.Error(t, err, "properties that cannot be normalised are refused rather than half-resolved")
}
