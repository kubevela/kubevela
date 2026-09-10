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

package propexpr

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Literal is what a value with nothing to evaluate should render as. Callers
// used to return the raw string in that case, which is byte-identical only when
// the value contains no escape: "$$(FOO)" carries a delimiter the author asked
// to have collapsed, and returning it untouched leaves $$( in the object.
//
// That mattered because $$( is the whole migration path. An operator escaping
// $(SERVICE_HOST) before enabling expressions would ship $$(SERVICE_HOST), and
// Kubernetes renders that as the literal text $(SERVICE_HOST) rather than
// expanding it - so the escape silently broke the thing it was protecting.
func TestLiteralCollapsesEscapes(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want string
	}{
		{"escape alone", "$$(SERVICE_HOST)", "$(SERVICE_HOST)"},
		{"two escapes", "$$(HOST):$$(PORT)", "$(HOST):$(PORT)"},
		{"escape in text", "echo $$(hostname)", "echo $(hostname)"},
		{"no delimiter is untouched", "nginx:1.25.0", "nginx:1.25.0"},
		{"empty", "", ""},
		{"bare dollar is not an escape", "cost is $5", "cost is $5"},
		{"doubled dollar without paren", "$$HOME", "$$HOME"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := Parse(tc.raw)
			require.NoError(t, err)
			require.False(t, parsed.HasExpr(), "fixture should have nothing to evaluate")
			require.Equal(t, tc.want, parsed.Literal())
		})
	}
}

// A value that mixes an escape with a real expression already collapsed
// correctly, because it went through the fragment-joining path. Literal covers
// the other half, so both halves now agree.
func TestLiteralOnAMixedValueIsTheTextOnly(t *testing.T) {
	parsed, err := Parse("$$(HOST) in $(context.appName)")
	require.NoError(t, err)
	require.True(t, parsed.HasExpr())
	require.Equal(t, "$(HOST) in ", parsed.Literal())
}
