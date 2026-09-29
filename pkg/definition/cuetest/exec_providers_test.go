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
	"context"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"
)

func TestExecProviders(t *testing.T) {
	mocks, err := parseMocksFor(cuecontext.New().CompileString(`{
		"vela/http": "#HTTPDo": {$params: url: =~"^https://hooks", $returns: {statusCode: 202, body: "ok"}}
	}`), execProviders)
	require.NoError(t, err)
	compiler, rec, err := useExecProviders(mocks)
	require.NoError(t, err)

	cases := map[string]struct {
		src, err string
	}{
		"a mocked call is answered": {src: `import "vela/http"
post: http.#HTTPDo & {$params: {method: "POST", url: "https://hooks.example.com"}}`},
		"an unmocked call that stays inside runs for real": {src: `import "vela/time"
ts: time.#DateToTimestamp & {$params: {date: "2026-09-27T00:00:00Z", layout: ""}}`},
		"an unmocked call that reaches outside fails": {
			src: `import "vela/http"
get: http.#HTTPDo & {$params: url: "https://example.com"}`,
			err: "unmocked call vela/http.#HTTPDo with $params",
		},
		"placement decisions are written into the Application": {
			src: `import "vela/op"
decide: op.#MakePlacementDecisions & {inputs: {policyName: "env", envName: "prod", placement: clusterSelector: name: "local"}}`,
			err: "vela/op.#MakePlacementDecisions needs an Application",
		},
		"a connection's status is read from the Application": {
			src: `import "vela/op"
status: op.#GetConnectionStatus & {inputs: componentName: "db"}`,
			err: "vela/op.#GetConnectionStatus needs an Application",
		},
		"an unmocked call that needs an Application fails": {
			src: `import "vela/oam"
apply: oam.#ApplyComponent & {$params: value: {}}`,
			err: "vela/oam.#ApplyComponent needs an Application, which a step test does not have: mock it",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := compiler.CompileString(context.Background(), tc.src)
			if tc.err == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.err)
			}
		})
	}
	require.Contains(t, callsResult(rec.calls), "vela/http")
	require.Contains(t, callsResult(rec.calls), "vela/time", "real calls are recorded too")
}

func TestExecProvidersRefusedReason(t *testing.T) {
	compiler, _, err := useExecProviders(nil)
	require.NoError(t, err)
	_, err = compiler.CompileString(context.Background(), `import "vela/http"
get: http.#HTTPDo & {$params: url: "https://example.com"}`)
	require.ErrorContains(t, err, "it reaches outside the cluster, so a step test must mock it")
}

func TestExecMockValidation(t *testing.T) {
	_, err := parseMocksFor(cuecontext.New().CompileString(`{"vela/builtin": "#Suspend": {}}`), execProviders)
	require.ErrorContains(t, err, `mocks."vela/builtin": vela/builtin drives the step's phase, so it runs for real`)
}
