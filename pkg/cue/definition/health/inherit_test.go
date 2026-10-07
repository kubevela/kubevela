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

package health

import (
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"
)

// A workload the parent's policy calls unhealthy and the child's calls healthy,
// so the four forms are distinguishable by their verdict alone.
func chainContext() map[string]interface{} {
	return map[string]interface{}{
		"output": map[string]interface{}{
			"spec":   map[string]interface{}{"replicas": 1},
			"status": map[string]interface{}{"readyReplicas": 0},
		},
	}
}

const parentPolicy = `isHealth: context.output.status.readyReplicas == context.output.spec.replicas`
const childPolicy = `isHealth: context.output.spec.replicas > 0`

func healthOf(t *testing.T, child, parent Snippets) bool {
	t.Helper()
	got, err := checkHealthChain(chainContext(), []Snippets{child, parent}, nil)
	require.NoError(t, err)
	return got
}

// Silence takes the parent's verdict whole.
func TestHealthSilenceInherits(t *testing.T) {
	require.False(t, healthOf(t, Snippets{}, Snippets{Health: parentPolicy}))
}

// Stating isHealth adds to the parent's: both must hold.
func TestHealthAppends(t *testing.T) {
	require.False(t, healthOf(t,
		Snippets{Health: childPolicy},
		Snippets{Health: parentPolicy}),
		"the child's check passes, the parent's does not, so the pair does not")
}

// `$inherit: false` decides alone, and may still build on the parent's verdict.
func TestHealthRelaxesWithSuper(t *testing.T) {
	require.True(t, healthOf(t,
		Snippets{Health: "$inherit: false\nisHealth: $super.isHealth || context.output.spec.replicas > 0"},
		Snippets{Health: parentPolicy}))
}

// `$inherit: false` without reading `$super` replaces the parent's outright.
func TestHealthReplaces(t *testing.T) {
	require.True(t, healthOf(t,
		Snippets{Health: "$inherit: false\n" + childPolicy},
		Snippets{Health: parentPolicy}))
}

// A chain of one composes with nobody.
func TestHealthAloneIsItsOwnVerdict(t *testing.T) {
	got, err := checkHealthChain(chainContext(), []Snippets{{Health: childPolicy}}, nil)
	require.NoError(t, err)
	require.True(t, got)
}

// A three-level chain ands all the way down.
func TestHealthAcrossThreeLevels(t *testing.T) {
	got, err := checkHealthChain(chainContext(), []Snippets{
		{Health: childPolicy},
		{Health: childPolicy},
		{Health: parentPolicy},
	}, nil)
	require.NoError(t, err)
	require.False(t, got, "the root's check fails, so the chain does")
}

// A message replaces its parent's, since a status line is one line.
func TestMessageReplaces(t *testing.T) {
	got, err := statusMessageChain(chainContext(), []Snippets{
		{Custom: `message: "from the child"`},
		{Custom: `message: "from the parent"`},
	}, nil)
	require.NoError(t, err)
	require.Equal(t, "from the child", got)
}

// `$super.message` extends it instead.
func TestMessageExtendsWithSuper(t *testing.T) {
	got, err := statusMessageChain(chainContext(), []Snippets{
		{Custom: `message: "\($super.message), and the child"`},
		{Custom: `message: "from the parent"`},
	}, nil)
	require.NoError(t, err)
	require.Equal(t, "from the parent, and the child", got)
}

// Silence keeps the parent's message.
func TestMessageSilenceInherits(t *testing.T) {
	got, err := statusMessageChain(chainContext(), []Snippets{
		{},
		{Custom: `message: "from the parent"`},
	}, nil)
	require.NoError(t, err)
	require.Equal(t, "from the parent", got)
}

// Details merge, and `$inherit` never reaches the published map.
func TestDetailsMerge(t *testing.T) {
	joined := detailsChain([]Snippets{
		{Details: "childKey: \"c\""},
		{Details: "parentKey: \"p\""},
	})
	require.Contains(t, joined, "parentKey")
	require.Contains(t, joined, "childKey")

	replaced := detailsChain([]Snippets{
		{Details: "$inherit: false\nchildKey: \"c\""},
		{Details: "parentKey: \"p\""},
	})
	require.NotContains(t, replaced, "parentKey")
	require.NotContains(t, replaced, InheritField, "a directive must not be published as data")
}

// `$inherit` is a field, not a line. A snippet that puts it beside another
// declaration keeps the other one: removing the whole line silently drops
// whatever shared it.
func TestOnlyTheDirectiveIsRemoved(t *testing.T) {
	got := stripDirectives(`$inherit: false, isHealth: true`)

	require.NotContains(t, got, "$inherit")
	require.Contains(t, got, "isHealth")
}

func TestStrippingLeavesAnOrdinarySnippetAlone(t *testing.T) {
	snippet := "isHealth: context.output.status.ready\nmessage: \"fine\"\n"

	require.Contains(t, stripDirectives(snippet), "isHealth")
	require.Contains(t, stripDirectives(snippet), "message")
}

// A snippet that cannot be parsed is returned as it stands: reporting the
// syntax error is the compiler's job, and mangling it first only obscures that.
func TestUnparseableSnippetsSurviveStripping(t *testing.T) {
	require.Contains(t, stripDirectives("isHealth: ((("), "isHealth")
}

// Details from two levels are concatenated, so a child whose snippet opens with
// an import would otherwise land below its parent's fields and fail to parse.
func TestDetailsFromTwoLevelsWithAnImport(t *testing.T) {
	parent := "readyReplicas: 1\n"
	child := "import \"list\"\n\ntags: list.Concat([[\"a\"], [\"b\"]])\n"

	joined := detailsChain([]Snippets{{Details: child}, {Details: parent}})

	val := cuecontext.New().CompileString(joined)
	require.NoError(t, val.Err(), "joined details must parse: %s", joined)
}
