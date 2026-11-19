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
	"testing"

	celengine "github.com/kubevela/pkg/cel"
	"github.com/kubevela/pkg/cel/template"
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
		{`component.db.output.status.addresses[0]`, []string{"component.db.output.status.addresses[0]"}},
		// A bracketed key is still below its parent, so the parent is not a read
		// of its own.
		{`component.db.output.metadata.labels["app.oam.dev/team"]`,
			[]string{`component.db.output.metadata.labels["app.oam.dev/team"]`}},
		{`source.cfg.data["app.properties"]`, []string{`source.cfg.data["app.properties"]`}},
	} {
		t.Run(tc.expr, func(t *testing.T) {
			refs, err := Vela.PropertyReferences(tc.expr)
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

	plan, err := Vela.Plan(props)
	require.NoError(t, err)
	require.Empty(t, plan.Check([]string{propexpr.SourceIdent, propexpr.ContextIdent, propexpr.ComponentIdent}, nil))

	faults := plan.Check([]string{propexpr.SourceIdent, propexpr.ContextIdent}, nil)
	require.Len(t, faults, 1)
	require.ErrorContains(t, faults[0], `"component" cannot be read here`)
}

func TestEvalReadsComponents(t *testing.T) {
	env := Vela.DynEnv()
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

	got, err := Vela.EvalProperty(env, `pg://$(component.db.output.status.endpoint):$(component.db.output.status.port)`, in)
	require.NoError(t, err)
	require.Equal(t, "pg://db.internal:5432", got)

	// A lone read keeps its type, as a source read does.
	got, err = Vela.EvalProperty(env, `$(component.db.output.status.port)`, in)
	require.NoError(t, err)
	require.Equal(t, int64(5432), got)
}

// The typed environment admission uses must accept the root too, or every
// component read fails there as an undeclared reference.
func TestTypedEnvDeclaresComponents(t *testing.T) {
	env, err := EnvForContext(nil, propexpr.ComponentContext)
	require.NoError(t, err)
	typ, err := Vela.OutputType(env, `component.db.output.status.endpoint`)
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
			refs, err := Vela.PropertyReferences(tc.expr)
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
	env := Vela.DynEnv()
	view := func(host string) map[string]interface{} {
		return map[string]interface{}{"output": map[string]interface{}{"status": map[string]interface{}{"endpoint": host}}}
	}
	call := template.Call
	data := view("db.data")
	data[celengine.QualifiedKey] = map[string]interface{}{call(propexpr.PlaceNamespace, "orders"): view("db.data.orders")}
	db := view("db.here")
	db[celengine.QualifiedKey] = map[string]interface{}{
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
			got, err := Vela.Eval(env, expr, in)
			require.NoError(t, err)
			require.Equal(t, want, got)
		})
	}

	_, err := Vela.Eval(env, `component.db.cluster("nowhere").output.status.endpoint`, in)
	require.ErrorContains(t, err, `.cluster("nowhere") was not delivered`)
}

func TestPlacementFunctionsType(t *testing.T) {
	env, err := EnvForContext(nil, propexpr.ComponentContext)
	require.NoError(t, err)
	for expr, want := range map[string]string{
		`component.db.cluster("data").output.status.endpoint`: "dyn",
	} {
		typ, err := Vela.OutputType(env, expr)
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
		_, err := Vela.PropertyReferences(expr)
		require.ErrorContains(t, err, "only on a component read", expr)
	}
	// A read names one placement at most; there is no reading every placement.
	_, err := Vela.PropertyReferences(`component.db.placements("hub")[0].output`)
	require.ErrorContains(t, err, "undeclared reference to 'placements'")

	_, err = Vela.PropertyReferences("component.db.output.data[\"a\\u0000b\"]")
	require.ErrorContains(t, err, "NUL")

	// A NUL in a value, not a key, is no business of the read path.
	_, err = Vela.PropertyReferences("component.db.output.data.v == \"a\\u0000b\"")
	require.NoError(t, err)

	// A placement call goes straight after the name, or after another one; after
	// an index or a field it would find nothing delivered at render.
	for _, expr := range []string{
		`component.db.output.namespace("x").status`,
		`component.db.output.cluster("data").status`,
	} {
		_, err := Vela.PropertyReferences(expr)
		require.ErrorContains(t, err, "go straight after component.<name>", expr)
	}
	_, err = Vela.PropertyReferences(`component[context.appName].cluster("a").output`)
	require.ErrorContains(t, err, "go straight after component.<name>", "a computed name cannot carry a placement")
	_, err = Vela.PropertyReferences(`component["db"].cluster("a").output`)
	require.NoError(t, err)

	_, err = Vela.PropertyReferences(`component.db.cluster("a\u0000b").output`)
	require.ErrorContains(t, err, "argument containing a NUL")

	// A non-literal placement argument is refused as that, even mid-chain.
	_, err = Vela.PropertyReferences(`component.db.cluster(context.cluster).namespace("west").output`)
	require.ErrorContains(t, err, "takes a literal string")
}

// A hyphenated component name read with a dot parses as subtraction; the error
// says to read it by index, and the index form reads it.
func TestHyphenatedComponentName(t *testing.T) {
	env := Vela.DynEnv()
	var err error

	_, err = OutputType(env, `component.my-db.output.data.host`)
	require.Error(t, err)
	require.Contains(t, err.Error(), `write component["my-db"].output.data.host`)

	_, err = PropertyReferences(`"pg://" + component.my-db.cluster("east").output.data.host`)
	require.Error(t, err)
	require.Contains(t, err.Error(), `write "pg://" + component["my-db"].cluster("east").output.data.host`,
		"the suggestion is the whole expression, ready to copy")

	refs, err := PropertyReferences(`component["my-db"].output.data.host`)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	require.Equal(t, `component["my-db"].output.data.host`, refs[0].String(), "a read renders as it must be written")

	_, err = OutputType(env, `component.db.output.replicas - 1`)
	require.NoError(t, err, "subtraction after a component read is still subtraction")
	_, err = OutputType(env, `undefined_thing`)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "by index", "only a dotted hyphenated component read gets the hint")
}

// A component name may start with a digit or hold a run of hyphens, and is
// still read by index.
func TestHyphenatedComponentNameShapes(t *testing.T) {
	env := Vela.DynEnv()
	var err error
	for name, expr := range map[string]string{
		"2-tier": `component.2-tier.output.x`,
		"my--db": `component.my--db.output.x`,
	} {
		_, err = OutputType(env, expr)
		require.Error(t, err)
		require.Contains(t, err.Error(), fmt.Sprintf(`component[%q]`, name), expr)
	}
}

// The hint is for an error the hyphenated read caused, not one elsewhere in an
// expression that happens to hold such text.
func TestHyphenatedComponentHintFollowsTheError(t *testing.T) {
	env := Vela.DynEnv()
	var err error
	_, err = OutputType(env, `'component.web-db' == missing`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "undeclared reference to 'missing'")
	require.NotContains(t, err.Error(), "by index")
}

// CEL reports an error's location in runes, so text with multi-byte runes
// before the read still places the error on it.
func TestHyphenatedComponentHintAfterMultiByteText(t *testing.T) {
	env := Vela.DynEnv()
	var err error
	_, err = OutputType(env, `"hé" == component.my-db`)
	require.Error(t, err)
	require.Contains(t, err.Error(), `write "hé" == component["my-db"]`)

	// Each é is two bytes, enough to move a byte window past the error.
	_, err = OutputType(env, `"éééééééééééééééééééé" == component.web-db`)
	require.Error(t, err)
	require.Contains(t, err.Error(), `component["web-db"]`)
}
