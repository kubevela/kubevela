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
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
)

// TestCacheNeverCrossesEnvironments is the regression this cache could plausibly
// cause, and the reason the identity check exists.
//
// The permissive environment types every source read as dyn, so anything
// compiles there. A typed environment declares each binding's real shape, so the
// same text may not compile at all. Handing a caller who asked for the typed
// environment a program compiled against the permissive one would silently
// disable the target-type check - a string reaching an int parameter would stop
// being an admission error and become a render failure, which is the exact
// regression the type check was added to prevent.
func TestCacheNeverCrossesEnvironments(t *testing.T) {
	// port is a string here, so `+ 1000` is a type error. Against the permissive
	// environment the same text compiles happily, because everything is dyn.
	const expr = `source.cfg.port + 1000`

	dyn, err := DynEnv()
	require.NoError(t, err)
	typed, err := EnvForContext(map[string]string{"cfg": `{port: string}`}, propexpr.ContextSchema{})
	require.NoError(t, err)

	in := map[string]interface{}{
		// An int, so the permissive env both compiles and evaluates it. The
		// typed env below declares port a string, and it is that declaration -
		// not the data - that has to keep rejecting the expression.
		"source":  map[string]interface{}{"cfg": map[string]interface{}{"port": float64(8080)}},
		"context": map[string]interface{}{},
	}

	// Warm the cache through the permissive environment.
	_, err = Eval(dyn, expr, in)
	require.NoError(t, err, "precondition: the permissive env accepts this")
	_, err = References(dyn, expr)
	require.NoError(t, err)

	// Both cached entry points must still refuse it against the typed
	// environment. Serving the dyn compilation here would silently disable the
	// target-type check that stops a string reaching an int parameter.
	_, err = Eval(typed, expr, in)
	assert.Error(t, err, "Eval must recompile for a typed env, not serve the dyn program")

	_, err = References(typed, expr)
	assert.Error(t, err, "References must recompile for a typed env, not serve the dyn AST")

	// ...and a typed env that does accept it still reports the right type.
	typedOK, err := EnvForContext(map[string]string{"cfg": `{port: int}`}, propexpr.ContextSchema{})
	require.NoError(t, err)
	got, err := OutputType(typedOK, expr)
	require.NoError(t, err)
	assert.Equal(t, "int", got.String())
}

// TestCacheKeepsExpressionsApart guards the obvious way a keyed cache goes
// wrong: returning one expression's program for another.
func TestCacheKeepsExpressionsApart(t *testing.T) {
	in := map[string]interface{}{
		"source": map[string]interface{}{"cfg": map[string]interface{}{
			"host": "example.com", "port": float64(8080), "tier": "prod",
		}},
		"context": map[string]interface{}{},
	}
	env, err := DynEnv()
	require.NoError(t, err)

	cases := map[string]interface{}{
		`source.cfg.host`:                                 "example.com",
		`source.cfg.port`:                                 int64(8080),
		`source.cfg.port + 1`:                             int64(8081),
		`source.cfg.tier == "prod" ? "a" : "b"`:           "a",
		`source.cfg.host + ":" + string(source.cfg.port)`: "example.com:8080",
	}
	// Twice through, so the second pass is served from the cache.
	for pass := 0; pass < 2; pass++ {
		for expr, want := range cases {
			got, err := Eval(env, expr, in)
			require.NoErrorf(t, err, "pass %d: %s", pass, expr)
			assert.Equalf(t, want, got, "pass %d: %s", pass, expr)
		}
	}
}

// TestCacheDoesNotHideErrors pins that a bad expression stays bad, and that
// evaluating it does not poison the cache for a good one.
func TestCacheDoesNotHideErrors(t *testing.T) {
	env, err := DynEnv()
	require.NoError(t, err)
	in := map[string]interface{}{
		"source":  map[string]interface{}{"cfg": map[string]interface{}{"host": "a"}},
		"context": map[string]interface{}{},
	}

	for i := 0; i < 3; i++ {
		_, err := Eval(env, `source.cfg.host +`, in)
		require.Error(t, err, "a syntax error must be reported every time, not cached away")
	}
	for i := 0; i < 3; i++ {
		_, err := Eval(env, `parameter.nope`, in)
		require.Error(t, err, "an out-of-sandbox identifier must stay an error")
	}

	got, err := Eval(env, `source.cfg.host`, in)
	require.NoError(t, err)
	assert.Equal(t, "a", got)
}

// TestCacheIsConcurrencySafe exercises the shape the render path will use once
// sources resolve in parallel. Run under -race this is the real assertion.
func TestCacheIsConcurrencySafe(t *testing.T) {
	env, err := DynEnv()
	require.NoError(t, err)

	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			in := map[string]interface{}{
				"source":  map[string]interface{}{"cfg": map[string]interface{}{"n": float64(n)}},
				"context": map[string]interface{}{},
			}
			// A mix of shared and distinct expressions, so entries are both
			// re-read and inserted concurrently.
			for _, expr := range []string{
				`source.cfg.n`,
				`source.cfg.n + 1`,
				fmt.Sprintf(`source.cfg.n + %d`, n),
			} {
				if _, err := Eval(env, expr, in); err != nil {
					errs <- err
					return
				}
				if _, err := References(env, expr); err != nil {
					errs <- err
					return
				}
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent use failed: %v", err)
	}
}

// TestDynEnvIsShared pins the memoisation, which is what makes the cache key
// sound: if DynEnv handed back a new environment each call, the identity check
// in compiledFor would never match and the cache would be dead weight.
func TestDynEnvIsShared(t *testing.T) {
	a, err := DynEnv()
	require.NoError(t, err)
	b, err := DynEnv()
	require.NoError(t, err)
	assert.Same(t, a, b, "DynEnv must return one shared environment")
}
