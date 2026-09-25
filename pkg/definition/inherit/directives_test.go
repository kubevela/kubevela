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
	"testing"

	"github.com/stretchr/testify/require"
)

// A quoted label may carry escapes, and CUE decodes them: "output" is the
// field `output`. Reading the literal text instead leaves the directive naming a
// field that does not exist, so it silently does nothing and the surface it was
// meant to take over is merged anyway.
func TestAQuotedLabelWithAnEscapeIsDecoded(t *testing.T) {
	d, err := parseDirectives(Level{
		Name:     "child",
		Template: "$inherit: {\"out\\u0070ut\": false}\noutput: {}\n",
	})
	require.NoError(t, err)
	require.False(t, d.inherits("output"), "the directive names output, however it was spelled")
}

// The ordinary spellings keep working.
func TestPlainAndQuotedLabelsBothName(t *testing.T) {
	for _, tmpl := range []string{
		"$inherit: {output: false}\noutput: {}\n",
		"$inherit: {\"output\": false}\noutput: {}\n",
	} {
		d, err := parseDirectives(Level{Name: "child", Template: tmpl})
		require.NoError(t, err)
		require.False(t, d.inherits("output"))
		require.True(t, d.inherits("outputs"), "and says nothing about the others")
	}
}
