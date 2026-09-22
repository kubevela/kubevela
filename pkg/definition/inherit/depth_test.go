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
	"fmt"
	"os"
	"testing"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/require"
)

const depthRoot = `
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: name: context.name
	spec: {
		replicas: parameter.replicas
		template: spec: containers: [{
			name:  context.name
			image: parameter.image
		}]
	}
}

parameter: {
	image:    string
	replicas: *1 | int
}
`

// Inherits its parent's schema, which means its own `$super` is filled from a
// context above it before the level below adopts the result.
func schemaInheritingLevel(i int) Level {
	return Level{
		Name: fmt.Sprintf("level-%d", i),
		Template: fmt.Sprintf(`
$super: properties: {image: parameter.image}

parameter: $super.parameter & {
	tier%d: *"standard" | string
}
`, i),
	}
}

// Forwards everything it was given rather than naming fields.
func forwardingLevel(i int) Level {
	return Level{
		Name: fmt.Sprintf("level-%d", i),
		Template: fmt.Sprintf(`
$super: properties: parameter

parameter: $super.parameter & {
	tier%d: *"standard" | string
}
`, i),
	}
}

// Declares its own parameters and reads nothing off `$super`.
func abstractingLevelForDepth(i int) Level {
	return Level{
		Name: fmt.Sprintf("level-%d", i),
		Template: fmt.Sprintf(`
$super: properties: {image: parameter.image}

parameter: {
	image:  string
	tier%d: *"standard" | string
}
`, i),
	}
}

func chainOf(level func(int) Level, depth int, root string) []Level {
	chain := make([]Level, 0, depth)
	for i := depth - 1; i >= 1; i-- {
		chain = append(chain, level(i))
	}
	return append(chain, Level{Name: "root", Template: root})
}

// A chain renders at any depth the cap allows, whatever shape its levels take.
//
// Depth is the point of the test: a level's own `$super` is filled from the
// context above it, and only a chain of three or more has a level below to
// adopt that result.
func TestChainsRenderAtEveryDepth(t *testing.T) {
	shapes := map[string]func(int) Level{
		"schema-inheriting": schemaInheritingLevel,
		"forwarding":        forwardingLevel,
		"abstracting":       abstractingLevelForDepth,
	}

	for name, level := range shapes {
		for _, depth := range []int{2, 3, 5, 9} {
			t.Run(fmt.Sprintf("%s/depth-%d", name, depth), func(t *testing.T) {
				res, err := Render(context.Background(), chainOf(level, depth, depthRoot),
					`parameter: {image: "nginx:1.27"}`,
					`context: {name: "app", appName: "acme", namespace: "acme"}`,
					ComponentSurface, SameCompiler(testCompile()))
				require.NoError(t, err)

				image, err := res.Value.LookupPath(
					cue.ParsePath("output.spec.template.spec.containers[0].image")).String()
				require.NoError(t, err)
				require.Equal(t, "nginx:1.27", image, "the root still rendered what it was passed")

				replicas, err := res.Value.LookupPath(cue.ParsePath("output.spec.replicas")).Int64()
				require.NoError(t, err)
				require.EqualValues(t, 1, replicas, "and its default survived the chain")
			})
		}
	}
}

// webserviceLevel forwards what the root actually takes, so a parameter supplied
// at the top reaches it. A level that forwards only `image` leaves everything
// else inert, and a test built on one proves less than it appears to.
func webserviceLevel(i int) Level {
	return Level{
		Name: fmt.Sprintf("level-%d", i),
		Template: fmt.Sprintf(`
$super: properties: {
	image: parameter.image
	ports: parameter.ports
}

parameter: $super.parameter & {
	tier%d: *"standard" | string
}
`, i),
	}
}

// The same, against the shipped webservice rather than a toy root.
func TestWebserviceChainRendersAtDepth(t *testing.T) {
	root, err := os.ReadFile("testdata/webservice.cue")
	require.NoError(t, err)

	for _, depth := range []int{3, 9} {
		t.Run(fmt.Sprintf("depth-%d", depth), func(t *testing.T) {
			res, err := Render(context.Background(), chainOf(webserviceLevel, depth, string(root)),
				`parameter: {image: "nginx:1.27", ports: [{port: 8080, expose: true}]}`,
				`context: {name: "billing-api", appName: "acme-billing", namespace: "acme"}`,
				ComponentSurface, SameCompiler(testCompile()))
			require.NoError(t, err)

			image, err := res.Value.LookupPath(
				cue.ParsePath("output.spec.template.spec.containers[0].image")).String()
			require.NoError(t, err)
			require.Equal(t, "nginx:1.27", image)

			// `expose: true` is what makes webservice render a Service, so this
			// is the evidence that `ports` reached the root rather than sitting
			// unused in the parameter file.
			port, err := res.Value.LookupPath(
				cue.ParsePath("outputs.webserviceExpose.spec.ports[0].port")).Int64()
			require.NoError(t, err)
			require.EqualValues(t, 8080, port)
		})
	}
}
