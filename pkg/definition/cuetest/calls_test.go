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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCallAssertions(t *testing.T) {
	suites, err := Load("testdata/calls")
	require.NoError(t, err)
	require.NoError(t, suites[0].Err)
	got := map[string][]string{}
	for _, c := range suites[0].Cases {
		got[c.Name] = c.Run()
	}
	require.Equal(t, map[string][]string{
		"exactly these calls":              nil,
		"what a mock returned is recorded": nil,
		"in any order":                     nil,
		"a call that was not made":         nil,
		"fails on purpose":                 {`calls."vela/http"."#Do"[0].$params.method: expected "GET", got "POST"`},
	}, got)
}

func TestCallsRecordedInOrder(t *testing.T) {
	suites, err := Load("testdata/calls/calls_test.cue")
	require.NoError(t, err)
	require.NoError(t, suites[0].Err)
	r, err := Render(suites[0].Cases[0].Subject, suites[0].Cases[0].Input)
	require.NoError(t, err)
	require.Len(t, r.Calls, 2)
	require.Equal(t, "vela/kube.#Get", r.Calls[0].Path+"."+r.Calls[0].Def, "the Get runs first; the Do needs what it returned")
	// A POST through #Do is a #Post, as the package defines one.
	require.Equal(t, "vela/http.#Post", r.Calls[1].Path+"."+r.Calls[1].Def)
	require.Equal(t, []string{"#Do"}, r.Calls[1].Also)
}
