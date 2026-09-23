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

package celexpr

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
)

// TestTypedEnvCacheDistinguishesSchemas is the risk this cache carries. A typed
// environment is what makes an expression's result checkable against the
// parameter it feeds, so serving one Application the environment built for
// another's definitions would type its reads against the wrong contract - which
// is precisely the mix-up the typed check exists to catch.
func TestTypedEnvCacheDistinguishesSchemas(t *testing.T) {
	const expr = "source.cfg.port"

	stringPort, err := EnvForContext(map[string]string{"cfg": `{port: string}`}, propexpr.ComponentContext)
	require.NoError(t, err)
	intPort, err := EnvForContext(map[string]string{"cfg": `{port: int}`}, propexpr.ComponentContext)
	require.NoError(t, err)

	got, err := OutputType(stringPort, expr)
	require.NoError(t, err)
	assert.Equal(t, "string", got.String())

	got, err = OutputType(intPort, expr)
	require.NoError(t, err)
	assert.Equal(t, "int", got.String(), "the same binding name against a different schema must type differently")

	// The same inputs again come from the cache and must not have drifted.
	again, err := EnvForContext(map[string]string{"cfg": `{port: string}`}, propexpr.ComponentContext)
	require.NoError(t, err)
	got, err = OutputType(again, expr)
	require.NoError(t, err)
	assert.Equal(t, "string", got.String())
}

// TestTypedEnvCacheDistinguishesSurfaces pins the other half of the key. A
// surface decides which context fields exist, so two surfaces must not share an
// environment even with identical source schemas.
func TestTypedEnvCacheDistinguishesSurfaces(t *testing.T) {
	schemas := map[string]string{"cfg": `{region: string}`}

	comp, err := EnvForContext(schemas, propexpr.ComponentContext)
	require.NoError(t, err)
	step, err := EnvForContext(schemas, propexpr.WorkflowStepContext)
	require.NoError(t, err)

	// componentType is a component-and-trait field; a workflow step has no such
	// thing, so the same expression types on one surface and not the other.
	_, compErr := OutputType(comp, "context.componentType")
	_, stepErr := OutputType(step, "context.componentType")
	assert.NoError(t, compErr, "a component reads componentType")
	assert.Error(t, stepErr, "a workflow step must not be served the component environment")
}

// TestTypedEnvCacheIsConcurrencySafe exercises the shape admission uses.
func TestTypedEnvCacheIsConcurrencySafe(t *testing.T) {
	schemas := map[string]string{"cfg": `{region: string}`}
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			env, err := EnvForContext(schemas, propexpr.ComponentContext)
			if err != nil {
				t.Error(err)
				return
			}
			if _, err := OutputType(env, "source.cfg.region"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}
