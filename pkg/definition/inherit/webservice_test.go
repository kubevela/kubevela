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

package inherit

import (
	"context"
	"os"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/require"
)

// The shipped webservice template, verbatim. It is the definition people will
// actually extend, and it is the one that exercises the parts a smaller fixture
// cannot: file-level imports, a `parameter` block declared in terms of
// `#HealthProbe`, comprehensions over optional parameters, and a conditional
// `outputs`.
func webserviceTemplate(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/webservice.cue")
	require.NoError(t, err)
	return string(b)
}

const webserviceContext = `
context: {
	name:        "billing-api"
	appName:     "acme-billing"
	namespace:   "acme"
	appRevision: "acme-billing-v1"
}
`

// A child of webservice that stamps a tenant label on the workload and its pods,
// pins a pod security baseline, and adds a quota alongside whatever webservice
// already produces. It never mentions the parent's output.
const tenantWebservice = `
$super: properties: {
	image: parameter.image
	ports: parameter.ports
}

output: {
	metadata: labels: "tenant.oam.dev/name": parameter.tenant
	spec: template: {
		metadata: labels: "tenant.oam.dev/name": parameter.tenant
		spec: securityContext: {
			runAsNonRoot: true
			seccompProfile: type: "RuntimeDefault"
		}
	}
}

outputs: quota: {
	apiVersion: "v1"
	kind:       "ResourceQuota"
	metadata: {
		name:      context.name
		namespace: context.namespace
	}
	spec: hard: "count/pods": parameter.maxPods
}

parameter: $super.parameter & {
	// +usage=Owning tenant, stamped on every pod
	tenant: string
	// +usage=Pod ceiling for this component
	maxPods: *10 | int
}
`

func TestExtendingTheRealWebservice(t *testing.T) {
	res, err := Render(
		context.Background(),
		[]Level{
			{Name: "tenant-webservice", Template: tenantWebservice},
			{Name: "webservice", Template: webserviceTemplate(t)},
		},
		`parameter: {image: "acme/billing:1.4.2", ports: [{port: 8080, expose: true}], tenant: "acme"}`,
		webserviceContext, ComponentSurface, SameCompiler(testCompile()),
	)
	require.NoError(t, err)

	out := res.Value.LookupPath(cue.ParsePath("output"))

	kind, err := out.LookupPath(cue.ParsePath("kind")).String()
	require.NoError(t, err)
	require.Equal(t, "Deployment", kind)

	// webservice's own labelling still happens.
	comp, err := out.LookupPath(cue.ParsePath(`spec.template.metadata.labels."app.oam.dev/component"`)).String()
	require.NoError(t, err)
	require.Equal(t, "billing-api", comp)

	// and the child's lands beside it, on both the workload and the pod template.
	for _, path := range []string{
		`metadata.labels."tenant.oam.dev/name"`,
		`spec.template.metadata.labels."tenant.oam.dev/name"`,
	} {
		got, err := out.LookupPath(cue.ParsePath(path)).String()
		require.NoError(t, err, path)
		require.Equal(t, "acme", got, path)
	}

	// The child's pod security baseline is merged into webservice's pod spec,
	// which it says nothing about, without disturbing the container webservice built.
	nonRoot, err := out.LookupPath(cue.ParsePath("spec.template.spec.securityContext.runAsNonRoot")).Bool()
	require.NoError(t, err)
	require.True(t, nonRoot)

	image, err := out.LookupPath(cue.ParsePath("spec.template.spec.containers[0].image")).String()
	require.NoError(t, err)
	require.Equal(t, "acme/billing:1.4.2", image)

	port, err := out.LookupPath(cue.ParsePath("spec.template.spec.containers[0].ports[0].containerPort")).Int64()
	require.NoError(t, err)
	require.EqualValues(t, 8080, port)

	// webservice's Service is kept, and the child's quota is added to it.
	outputs := res.Value.LookupPath(cue.ParsePath("outputs"))
	require.True(t, outputs.LookupPath(cue.ParsePath("webserviceExpose")).Exists(), "the parent's Service survives")

	quotaPods, err := outputs.LookupPath(cue.ParsePath(`quota.spec.hard."count/pods"`)).Int64()
	require.NoError(t, err)
	require.EqualValues(t, 10, quotaPods, "the child's default reached its own output")
}

// The schema lookup is the thing textual composition could not do: webservice
// declares `parameter` in terms of `#HealthProbe`, so lifting the block out of
// the AST broke the reference. Compiling the template whole does not.
func TestInheritedSchemaCarriesWebserviceParameters(t *testing.T) {
	res, err := Render(
		context.Background(),
		[]Level{
			{Name: "tenant-webservice", Template: tenantWebservice},
			{Name: "webservice", Template: webserviceTemplate(t)},
		},
		`parameter: {image: "acme/billing:1.4.2", ports: [{port: 8080, expose: true}], tenant: "acme"}`,
		webserviceContext, ComponentSurface, SameCompiler(testCompile()),
	)
	require.NoError(t, err)

	param := res.Value.LookupPath(cue.ParsePath("parameter"))

	// The child's own parameters.
	tenant, err := param.LookupPath(cue.ParsePath("tenant")).String()
	require.NoError(t, err)
	require.Equal(t, "acme", tenant)

	// webservice's, inherited. Most of them are optional, and an optional field
	// that nobody set does not answer to LookupPath, so the declared set is read
	// off the schema rather than probed.
	declared := map[string]bool{}
	iter, err := param.Fields(cue.Optional(true), cue.All())
	require.NoError(t, err)
	for iter.Next() {
		// An optional field's selector prints with its "?".
		declared[strings.TrimSuffix(iter.Selector().String(), "?")] = true
	}

	for _, field := range []string{
		"image", "ports", "cpu", "memory", "volumeMounts",
		"imagePullPolicy", "imagePullSecrets",
		// declared through #HealthProbe, which is what defeated lifting the
		// parameter block out of the AST.
		"livenessProbe", "readinessProbe",
	} {
		require.True(t, declared[field], "expected webservice's %q in the inherited schema, got %v", field, declared)
	}
}
