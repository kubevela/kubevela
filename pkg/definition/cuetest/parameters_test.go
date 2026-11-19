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

// A parameter that cannot be encoded fails the render rather than render
// as no parameters.
func TestUnencodableParameterFails(t *testing.T) {
	bad := map[string]any{"replicas": make(chan int)}
	component := Subject{Kind: KindComponent, Name: "web", Template: "output: {apiVersion: \"v1\", kind: \"ConfigMap\"}\nparameter: {...}"}
	trait := Subject{Kind: KindTrait, Name: "scaler", Template: "patch: {}\nparameter: {...}"}
	policy := Subject{Kind: KindPolicy, Name: "guard", Template: "output: {apiVersion: \"v1\", kind: \"ConfigMap\"}\nparameter: {...}"}

	_, err := Render(component, Input{Parameter: bad})
	require.ErrorContains(t, err, "encoding the parameters")
	_, err = Render(trait, Input{Parameter: bad, Workload: map[string]any{"kind": "Deployment"}})
	require.ErrorContains(t, err, "encoding the parameters")
	_, err = Render(component, Input{Traits: []TraitInput{{Subject: trait, Parameter: bad}}})
	require.ErrorContains(t, err, "encoding the parameters")
	_, err = RenderPolicy(policy, Input{Parameter: bad})
	require.ErrorContains(t, err, "encoding the parameters")
	scoped := Subject{Kind: KindApplicationPolicy, Name: "tagger", Template: "config: enabled: true\nparameter: {...}"}
	_, err = RenderApplicationPolicy(scoped, Input{Parameter: bad})
	require.ErrorContains(t, err, "encoding the parameters")
}
