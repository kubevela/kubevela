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

package cuetest

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Every field the registry offers a component or trait must be one a case can
// set, or one the harness derives, so a new field upstream forces a decision.
func TestContextSurfaceCoverage(t *testing.T) {
	settable := map[string]bool{}
	ct := reflect.TypeOf(Context{})
	for i := 0; i < ct.NumField(); i++ {
		settable[strings.Split(ct.Field(i).Tag.Get("json"), ",")[0]] = true
	}
	for kind, surface := range contextSurfaces {
		for _, field := range surface.ReadableFields() {
			_, derived := derivedContextFields[kind][field]
			require.Truef(t, settable[field] || derived,
				"context.%s is offered to %s but is neither settable by cuetest nor declared derived", field, surface.Surface)
		}
		for field := range derivedContextFields[kind] {
			require.Truef(t, surface.Offers(field), "context.%s is declared derived for %s, which does not offer it", field, kind)
		}
	}
}

func TestContextReachesTheRender(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "d.cue", "d: type: \"component\"\ntemplate: {output: {apiVersion: \"v1\", kind: \"ConfigMap\"}, parameter: {}}")
	write(t, dir, "t.cue", "t: type: \"trait\"\ntemplate: {parameter: {}}")
	const ctx = `{
		name: "api", namespace: "prod", appName: "shop", appRevision: "shop-v3"
		appLabels: team: "payments", appAnnotations: note: "hi"
		cluster: "eu-1", clusterVersion: {major: "1", minor: 30, gitVersion: "v1.30.2", platform: "linux/amd64"}
		publishVersion: "v2", workflowName: "rollout", revision: "api-v7", replicaKey: "eu", custom: tier: "gold"
	}`
	write(t, dir, "a_test.cue", `import "vela/test"

"component": test.#ComponentRender & {
	definition: "d"
	context: `+ctx+`
	expect: context: `+ctx+` & {componentName: "api", componentType: "d", appRevisionNum: 3}
}
"trait": test.#TraitRender & {
	definition: "t"
	workload: {apiVersion: "v1", kind: "ConfigMap"}
	context: `+ctx+`
	expect: context: `+ctx+` & {traitType: "t", appRevisionNum: 3}
}
`)
	suites, err := Load(dir)
	require.NoError(t, err)
	require.NoError(t, suites[0].Err)
	for _, c := range suites[0].Cases {
		require.Empty(t, c.Run(), c.Name)
	}
}

func TestContextErrors(t *testing.T) {
	cases := map[string]struct {
		test, context, err string
	}{
		"another surface's field": {"#ComponentRender", `stepName: "x"`, "context.stepName is not offered to components; it is offered to workflow steps"},
		"offered nowhere":         {"#ComponentRender", `nope: "x"`, "context.nope is not offered to components"},
		"derived: componentType":  {"#ComponentRender", `componentType: "x"`, "context.componentType is derived: it is the definition's type"},
		"derived: appRevisionNum": {"#ComponentRender", `appRevisionNum: 3`, "context.appRevisionNum is derived: set context.appRevision, e.g. \"shop-v3\""},
		"derived: traitType":      {"#TraitRender", `traitType: "x"`, "context.traitType is derived: it is the definition's type"},
		"wrong type":              {"#ComponentRender", `appRevision: 3`, "context.appRevision: "},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, "d.cue", "d: type: \"component\"\ntemplate: {output: {apiVersion: \"v1\", kind: \"X\"}, parameter: {}}")
			write(t, dir, "t.cue", "t: type: \"trait\"\ntemplate: {parameter: {}}")
			def := "d"
			extra := ""
			if tc.test == "#TraitRender" {
				def, extra = "t", ", workload: {}"
			}
			write(t, dir, "a_test.cue", "import \"vela/test\"\nx: test."+tc.test+" & {definition: \""+def+"\""+extra+", context: {"+tc.context+"}, expect: {}}")
			suites, err := Load(dir)
			require.NoError(t, err)
			require.ErrorContains(t, suites[0].Err, tc.err)
		})
	}
}
