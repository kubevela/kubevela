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
)

// A bare root identifier was not reported as a reference, so root validation had
// nothing to refuse: `$(source)` passed on a surface that offers no source at
// all and was then evaluated against an empty map.
func TestReferencesReportsBareRoots(t *testing.T) {
	env, err := DynEnv()
	require.NoError(t, err)

	for expr, want := range map[string]string{
		"source":          "source",
		"context":         "context",
		"source.cfg":      "source.cfg",
		"source.cfg.host": "source.cfg.host",
	} {
		refs, err := References(env, expr)
		require.NoError(t, err)
		require.Len(t, refs, 1, "expr %q", expr)
		require.Equal(t, want, refs[0].String())
	}

	require.Error(t, ValidateTree("$(source)", "context"),
		"a bare source must be refused where the surface offers none")
	require.NoError(t, ValidateTree("$(source)", "context", "source"))
}

// A sibling has() was treated as a guard under || as well as &&, but the two are
// duals and only && absorbs the error:
//
//	has(x) && read(x)    has() false -> false, read never evaluated
//	has(x) || read(x)    has() false -> read evaluated -> "no such key"
//	!has(x) || read(x)   has() false -> true, read never evaluated
//
// So an unsafe read was reported as defaulted, and admission asked for no
// fallback on an expression that fails at render.
func TestGuardedDistinguishesAndFromOr(t *testing.T) {
	env, err := DynEnv()
	require.NoError(t, err)

	defaulted := func(expr string) bool {
		refs, err := References(env, expr)
		require.NoError(t, err)
		for _, r := range refs {
			if r.String() == "source.cfg.note" {
				return r.Defaulted
			}
		}
		t.Fatalf("expression %q made no read of source.cfg.note", expr)
		return false
	}

	require.True(t, defaulted(`has(source.cfg.note) && source.cfg.note == "y"`))
	require.True(t, defaulted(`!has(source.cfg.note) || source.cfg.note == "y"`))
	require.True(t, defaulted(`has(source.cfg.note) ? source.cfg.note : "x"`))

	require.False(t, defaulted(`has(source.cfg.note) || source.cfg.note == "y"`),
		"|| with a plain has() does not defend the read")
	require.False(t, defaulted(`source.cfg.note == "y" || has(source.cfg.note)`))
	require.False(t, defaulted(`!has(source.cfg.note) && source.cfg.note == "y"`),
		"&& with a negated has() is the wrong way round too")
}
