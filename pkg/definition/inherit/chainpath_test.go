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
	"testing"

	"github.com/stretchr/testify/require"
)

// A level is compiled with what the level below supplied, so a failure inside a
// parent is often the child's doing. Naming the parent alone sends the author
// to a file they did not write, so the route is reported instead.
func TestAnErrorInsideAParentNamesTheRouteToIt(t *testing.T) {
	parent := `
output: {
	kind: "Deployment"
	spec: template: spec: containers: [{image: parameter.image}]
}

parameter: image: string
`
	// Hands its parent an int where a string is wanted.
	child := `
$super: properties: {image: parameter.tenant}

output: metadata: labels: t: "x"

parameter: tenant: int
`
	_, err := Render(context.Background(),
		[]Level{{Name: "tenant-webservice", Template: child}, {Name: "webservice", Template: parent}},
		`parameter: {tenant: 7}`, contextFile, ComponentSurface, SameCompiler(testCompile()))

	require.Error(t, err)
	require.Contains(t, err.Error(), "tenant-webservice:webservice",
		"the failure is in webservice, reached from tenant-webservice")
}

// The route grows with the chain, so a middle level is placed rather than just
// named.
func TestTheRouteGrowsWithTheChain(t *testing.T) {
	require.Equal(t, "webservice", chainPath(nil, "webservice"))
	require.Equal(t, "tenant:webservice", chainPath([]string{"tenant"}, "webservice"))
	require.Equal(t, "tenant:regional:webservice",
		chainPath([]string{"tenant", "regional"}, "webservice"))
}

// Building one route must not disturb another: the trail is shared down the
// recursion, and appending to it in place would rewrite a sibling's.
func TestBuildingARouteDoesNotDisturbTheTrail(t *testing.T) {
	trail := make([]string, 1, 4)
	trail[0] = "tenant"

	require.Equal(t, "tenant:a", chainPath(trail, "a"))
	require.Equal(t, "tenant:b", chainPath(trail, "b"))
	require.Equal(t, []string{"tenant"}, trail)
}
