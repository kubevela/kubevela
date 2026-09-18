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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	wfprocess "github.com/kubevela/workflow/pkg/cue/process"

	velacuex "github.com/oam-dev/kubevela/pkg/cue/cuex"

	"github.com/oam-dev/kubevela/pkg/cue/process"
)

const policyTemplate = `
schema: {v: string}
$internal: {
  key: "policy-\(context.namespace)-\(parameter.name)"
  keyInputs: ["namespace"]
}
storage: {storageTTL: "7m", onStaleFailure: "use-stale"}
parameter: {name: string}
output: {v: parameter.name}
`

func policyCtx(t *testing.T, namespace string) wfprocess.Context {
	t.Helper()
	pCtx := process.NewContext(process.ContextData{
		Namespace: namespace, CompName: "web", AppName: "app",
	})
	pCtx.PushData(process.ContextAppSources, map[string]map[string]interface{}{
		"a": {"name": "alpha"},
		"b": {"name": "beta"},
	})
	pCtx.PushData(process.ContextAppSourceTypes, map[string]string{"a": "pol", "b": "pol"})
	pCtx.PushData(process.ContextAppSourceTemplates, map[string]string{"pol": policyTemplate})
	return pCtx
}

func policyResolver(t *testing.T, ns string) *sourceResolver {
	t.Helper()
	pCtx := policyCtx(t, ns)
	return newSourceResolver(pCtx.GetCtx(), contextValuesFor(pCtx), SurfaceComponent, sourceInputsFromContext(pCtx))
}

// TestPolicyCacheDistinguishesItsInputs is the risk this cache carries. The
// policy names which stored entry to read, so returning one binding's policy for
// another would not be slow, it would be wrong - a source served another
// source's value.
func TestPolicyCacheDistinguishesItsInputs(t *testing.T) {
	r := policyResolver(t, "team-a")

	alpha, err := r.resolveCachePolicy("a", "pol", policyTemplate, map[string]interface{}{"name": "alpha"})
	require.NoError(t, err)
	beta, err := r.resolveCachePolicy("b", "pol", policyTemplate, map[string]interface{}{"name": "beta"})
	require.NoError(t, err)

	assert.Equal(t, "policy-team-a-alpha", alpha.Key)
	assert.Equal(t, "policy-team-a-beta", beta.Key,
		"different properties must not be served the first binding's policy")

	// A different namespace is different context, so a different key again.
	other := policyResolver(t, "team-b")
	elsewhere, err := other.resolveCachePolicy("a", "pol", policyTemplate, map[string]interface{}{"name": "alpha"})
	require.NoError(t, err)
	assert.Equal(t, "policy-team-b-alpha", elsewhere.Key,
		"a different render context must not be served the first context's policy")

	// ...and asking again gives the same answers, from the cache this time.
	again, err := r.resolveCachePolicy("a", "pol", policyTemplate, map[string]interface{}{"name": "alpha"})
	require.NoError(t, err)
	assert.Equal(t, alpha.Key, again.Key)
	assert.Equal(t, 7*time.Minute, again.TTL)
	assert.Equal(t, "use-stale", again.OnStaleFailure)
	assert.Equal(t, []string{"namespace"}, again.KeyInputs)
}

// TestPolicyCacheHandsOutIsolatedCopies pins that a caller cannot reach back
// into the cache through the slice it was given.
func TestPolicyCacheHandsOutIsolatedCopies(t *testing.T) {
	r := policyResolver(t, "team-a")
	props := map[string]interface{}{"name": "alpha"}

	first, err := r.resolveCachePolicy("a", "pol", policyTemplate, props)
	require.NoError(t, err)
	require.Equal(t, []string{"namespace"}, first.KeyInputs)

	first.KeyInputs[0] = "clobbered"
	first.Key = "clobbered"

	second, err := r.resolveCachePolicy("a", "pol", policyTemplate, props)
	require.NoError(t, err)
	assert.Equal(t, []string{"namespace"}, second.KeyInputs,
		"mutating a returned policy must not corrupt the cached one")
	assert.Equal(t, "policy-team-a-alpha", second.Key)
}

// TestPolicyCacheDoesNotCacheFailures keeps an error reportable. Caching one
// would outlive whatever caused it.
func TestPolicyCacheDoesNotCacheFailures(t *testing.T) {
	r := policyResolver(t, "team-a")
	const broken = `
schema: {v: string}
$internal: {key: "broken", keyInputs: []}
storage: {storageTTL: "not-a-duration"}
parameter: {name: string}
output: {v: "x"}
`
	for i := 0; i < 3; i++ {
		_, err := r.resolveCachePolicy("a", "pol", broken, map[string]interface{}{"name": "alpha"})
		require.Error(t, err, "an invalid storageTTL must be reported every time")
		assert.Contains(t, err.Error(), "storageTTL")
	}
}

// TestPolicyCacheIsUsed proves the cache is actually consulted, by counting
// compilations. Without it this whole file would pass against no cache at all.
func TestPolicyCacheIsUsed(t *testing.T) {
	pCtx := policyCtx(t, "team-count")
	in := sourceInputsFromContext(pCtx)
	// in.Compiler is nil here - the default is applied inside newSourceResolver -
	// so the spy wraps the singleton directly.
	spy := &recordingCompiler{inner: velacuex.WorkloadCompiler.Get()}
	in.Compiler = spy
	r := newSourceResolver(pCtx.GetCtx(), contextValuesFor(pCtx), SurfaceComponent, in)

	props := map[string]interface{}{"name": "counted"}
	_, err := r.resolveCachePolicy("a", "pol", policyTemplate, props)
	require.NoError(t, err)
	afterFirst := spy.called
	require.Positive(t, afterFirst, "the first resolution must compile")

	for i := 0; i < 5; i++ {
		_, err := r.resolveCachePolicy("a", "pol", policyTemplate, props)
		require.NoError(t, err)
	}
	assert.Equal(t, afterFirst, spy.called,
		"repeated resolutions of the same inputs must not compile again")
}

// A malformed generated block, or a storage field of the wrong type, used to be
// swallowed and defaulted. Dropping keyInputs is the one that matters: the
// identity hash loses every context dimension, so two Applications reading the
// same source in different namespaces share one cache entry.
func TestResolveCachePolicyRefusesMalformedBlocks(t *testing.T) {
	for _, tc := range []struct {
		name, template, wantErr string
	}{
		{
			name: "keyInputs missing",
			template: `
schema: {v: string}
$internal: {key: "policy-\(context.namespace)"}
parameter: {name: string}
output: {v: parameter.name}
`,
			wantErr: "keyInputs",
		},
		{
			name: "keyInputs of the wrong type",
			template: `
schema: {v: string}
$internal: {key: "policy-x", keyInputs: "namespace"}
parameter: {name: string}
output: {v: parameter.name}
`,
			wantErr: "keyInputs",
		},
		{
			name: "storageTTL of the wrong type",
			template: `
schema: {v: string}
$internal: {key: "policy-x", keyInputs: []}
storage: {storageTTL: 7}
parameter: {name: string}
output: {v: parameter.name}
`,
			wantErr: "storageTTL",
		},
		{
			name: "onStaleFailure of the wrong type",
			template: `
schema: {v: string}
$internal: {key: "policy-x", keyInputs: []}
storage: {onStaleFailure: true}
parameter: {name: string}
output: {v: parameter.name}
`,
			wantErr: "onStaleFailure",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := policyResolver(t, "team-a")
			_, err := r.resolveCachePolicy("a", "pol", tc.template, map[string]interface{}{"name": "alpha"})
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}
