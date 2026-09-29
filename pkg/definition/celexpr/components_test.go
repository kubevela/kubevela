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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
)

func TestComponentReferences(t *testing.T) {
	for _, tc := range []struct {
		expr string
		want []string
	}{
		{`component.db.output.status.endpoint`, []string{"component.db.output.status.endpoint"}},
		{`component["db"].outputs.svc.spec.clusterIP`, []string{"component.db.outputs.svc.spec.clusterIP"}},
		{`"pg://" + string(component.db.output.status.endpoint) + ":" + source.cfg.port`,
			[]string{"component.db.output.status.endpoint", "source.cfg.port"}},
		// An index below the read is kept out of the path: the value is exported
		// whole and the index applied to it.
		{`component.db.output.status.addresses[0]`, []string{"component.db.output.status.addresses"}},
		// A bracketed key is still below its parent, so the parent is not a read
		// of its own.
		{`component.db.output.metadata.labels["app.oam.dev/team"]`,
			[]string{`component.db.output.metadata.labels["app.oam.dev/team"]`}},
		{`source.cfg.data["app.properties"]`, []string{`source.cfg.data["app.properties"]`}},
	} {
		t.Run(tc.expr, func(t *testing.T) {
			refs, err := PropertyReferences(tc.expr)
			require.NoError(t, err)
			var got []string
			for _, r := range refs {
				got = append(got, r.String())
			}
			require.Equal(t, tc.want, got)
		})
	}
}

func TestComponentsRootIsPerSurface(t *testing.T) {
	props := map[string]interface{}{"url": `$(component.db.output.status.endpoint)`}

	require.NoError(t, ValidateTree(props, propexpr.SourceIdent, propexpr.ContextIdent, propexpr.ComponentIdent))

	err := ValidateTree(props, propexpr.SourceIdent, propexpr.ContextIdent)
	require.ErrorContains(t, err, `"component" cannot be read here`)
}

func TestEvalReadsComponents(t *testing.T) {
	env, err := DynEnv()
	require.NoError(t, err)
	in := map[string]interface{}{
		"source":  map[string]interface{}{},
		"context": map[string]interface{}{},
		propexpr.ComponentIdent: map[string]interface{}{
			"db": map[string]interface{}{
				"output": map[string]interface{}{
					"status": map[string]interface{}{"endpoint": "db.internal", "port": float64(5432)},
				},
			},
		},
	}

	got, err := EvalProperty(env, `pg://$(component.db.output.status.endpoint):$(component.db.output.status.port)`, in)
	require.NoError(t, err)
	require.Equal(t, "pg://db.internal:5432", got)

	// A lone read keeps its type, as a source read does.
	got, err = EvalProperty(env, `$(component.db.output.status.port)`, in)
	require.NoError(t, err)
	require.Equal(t, int64(5432), got)
}

// The typed environment admission uses must accept the root too, or every
// component read fails there as an undeclared reference.
func TestTypedEnvDeclaresComponents(t *testing.T) {
	env, err := EnvForContext(nil, ComponentCtx())
	require.NoError(t, err)
	typ, err := OutputType(env, `component.db.output.status.endpoint`)
	require.NoError(t, err)
	require.Equal(t, "dyn", typ.String())
}

func TestPlacementQualifiedReferences(t *testing.T) {
	for _, tc := range []struct {
		expr string
		want []string
	}{
		{`component.db.cluster("data").output.status.endpoint`, []string{`component.db.cluster("data").output.status.endpoint`}},
		{`component.db.cluster("data").namespace("orders").output.status.endpoint`, []string{`component.db.cluster("data").namespace("orders").output.status.endpoint`}},
		// An index ends the path like a lambda does: the read is the list.
		{`component.db.namespace("team-a").output.status.endpoint`, []string{`component.db.namespace("team-a").output.status.endpoint`}},
		// What a lambda reads off each item is the lambda's business; the read
		// recorded is the list itself.
		// The unqualified read and a qualified one are different reads.
		{`component.cache.output.status.host + component.cache.cluster("data").output.status.host`,
			[]string{`component.cache.cluster("data").output.status.host`, `component.cache.output.status.host`}},
	} {
		t.Run(tc.expr, func(t *testing.T) {
			refs, err := PropertyReferences(tc.expr)
			require.NoError(t, err)
			var got []string
			for _, r := range refs {
				got = append(got, r.String())
			}
			require.Equal(t, tc.want, got)
		})
	}
}

// The delivered scope nests each placement call's view under the call before it,
// and the functions only look it up: where a placement is, is decided before
// evaluation.
func TestPlacementFunctionsEvaluate(t *testing.T) {
	env, err := DynEnv()
	require.NoError(t, err)
	view := func(host string) map[string]interface{} {
		return map[string]interface{}{"output": map[string]interface{}{"status": map[string]interface{}{"endpoint": host}}}
	}
	call := propexpr.PlacementCall
	data := view("db.data")
	data[propexpr.QualifiedKey] = map[string]interface{}{call(propexpr.PlaceNamespace, "orders"): view("db.data.orders")}
	db := view("db.here")
	db[propexpr.QualifiedKey] = map[string]interface{}{
		call(propexpr.PlaceCluster, "data"):     data,
		call(propexpr.PlaceNamespace, "team-a"): view("db.team-a"),
	}
	in := map[string]interface{}{"source": map[string]interface{}{}, "context": map[string]interface{}{},
		propexpr.ComponentIdent: map[string]interface{}{"db": db}}

	for expr, want := range map[string]interface{}{
		`component.db.output.status.endpoint`:                                     "db.here",
		`component.db.cluster("data").output.status.endpoint`:                     "db.data",
		`component.db.cluster("data").namespace("orders").output.status.endpoint`: "db.data.orders",
		`component.db.namespace("team-a").output.status.endpoint`:                 "db.team-a",
	} {
		t.Run(expr, func(t *testing.T) {
			got, err := Eval(env, expr, in)
			require.NoError(t, err)
			require.Equal(t, want, got)
		})
	}

	_, err = Eval(env, `component.db.cluster("nowhere").output.status.endpoint`, in)
	require.ErrorContains(t, err, `.cluster("nowhere") was not delivered`)
}

func TestPlacementFunctionsType(t *testing.T) {
	env, err := EnvForContext(nil, ComponentCtx())
	require.NoError(t, err)
	for expr, want := range map[string]string{
		`component.db.cluster("data").output.status.endpoint`: "dyn",
	} {
		typ, err := OutputType(env, expr)
		require.NoError(t, err, expr)
		require.Equal(t, want, typ.String(), expr)
	}
}

// cluster, namespace and placements name where a component is; on anything else
// they could only fail at render, so they are refused where they are written.
func TestPlacementCallsOnlyOnAComponent(t *testing.T) {
	for _, expr := range []string{
		`source.cfg.cluster("east").host`,
		`context.namespace.namespace("x")`,
	} {
		_, err := PropertyReferences(expr)
		require.ErrorContains(t, err, "only on a component read", expr)
	}
	// A read names one placement at most; there is no reading every placement.
	_, err := PropertyReferences(`component.db.placements("hub")[0].output`)
	require.ErrorContains(t, err, "undeclared reference to 'placements'")

	_, err = PropertyReferences("component.db.output.data[\"a\\u0000b\"]")
	require.ErrorContains(t, err, "NUL")

	// A NUL in a value, not a key, is no business of the read path.
	_, err = PropertyReferences("component.db.output.data.v == \"a\\u0000b\"")
	require.NoError(t, err)

	// A placement call goes straight after the name, or after another one; after
	// an index or a field it would find nothing delivered at render.
	for _, expr := range []string{
		`component.db.output.namespace("x").status`,
		`component.db.output.cluster("data").status`,
	} {
		_, err := PropertyReferences(expr)
		require.ErrorContains(t, err, "go straight after the component", expr)
	}
	_, err = PropertyReferences(`component[context.appName].cluster("a").output`)
	require.ErrorContains(t, err, "go straight after the component", "a computed name cannot carry a placement")
	_, err = PropertyReferences(`component["db"].cluster("a").output`)
	require.NoError(t, err)

	_, err = PropertyReferences(`component.db.cluster("a\u0000b").output`)
	require.ErrorContains(t, err, "a placement argument containing a NUL")

	// A non-literal placement argument is reported as that, even mid-chain.
	_, err = PropertyReferences(`component.db.cluster(context.cluster).namespace("west").output`)
	require.NoError(t, err, "left to the read's own validation, which names the literal-argument rule")
}
