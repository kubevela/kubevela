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

package application

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// A global may read context: admission compiles its template without one, and
// the required-parameter check must not take that for a broken template.

func TestValidateGlobalPolicyReadingContext(t *testing.T) {
	global := func(template string) *v1beta1.PolicyDefinition {
		return &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "identity", Namespace: "vela-system"},
			Spec: v1beta1.PolicyDefinitionSpec{
				Global: true, Scope: v1beta1.ApplicationScope, Priority: 10,
				Schematic: &common.Schematic{CUE: &common.CUE{Template: template}},
			},
		}
	}
	cases := map[string]struct {
		template string
		wantErr  string
	}{
		"context in the output": {
			template: "parameter: {}\noutput: labels: ns: context.namespace",
		},
		"context in a provider call": {
			template: "import \"vela/kube\"\nparameter: {}\n_ns: kube.#Get & {$params: resource: {apiVersion: \"v1\", kind: \"Namespace\", metadata: name: context.namespace}}\noutput: ctx: team: _ns.$returns.metadata.labels[\"t\"]",
		},
		"context, and a defaulted parameter": {
			template: "parameter: env: *\"prod\" | string\noutput: labels: {ns: context.namespace, env: parameter.env}",
		},
		"context, and a required parameter": {
			template: "parameter: env: string\noutput: labels: {ns: context.namespace, env: parameter.env}",
			wantErr:  "cannot have parameters without default values",
		},
		"a real compile error is still refused": {
			template: "parameter: {}\noutput: labels: ns: undefinedThing",
			wantErr:  "undefinedThing",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			result := validatePolicy(global(tc.template))
			if tc.wantErr == "" {
				require.Empty(t, result.Errors)
				return
			}
			require.NotEmpty(t, result.Errors)
			require.Contains(t, strings.Join(result.Errors, "\n"), tc.wantErr)
		})
	}
}
