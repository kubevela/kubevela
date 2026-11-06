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

package appfile

import (
	"testing"

	"github.com/stretchr/testify/require"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/features"
)

func TestContextCustomPaths(t *testing.T) {
	cases := map[string]struct {
		templates []string
		want      [][]string
	}{
		"none": {
			templates: []string{`output: name: context.name`},
		},
		"a key": {
			templates: []string{`parameter: team: *context.custom.team | string`},
			want:      [][]string{{"team"}},
		},
		"a nested key keeps its full path": {
			templates: []string{`x: context.custom.org.team`},
			want:      [][]string{{"org", "team"}},
		},
		"a quoted key": {
			templates: []string{`x: context.custom["cost-centre"]`},
			want:      [][]string{{"cost-centre"}},
		},
		"a dynamic index stops the path": {
			templates: []string{`k: "a"
x: context.custom.org[k]`},
			want: [][]string{{"org"}},
		},
		"a selector after a dynamic index does not extend the path": {
			templates: []string{`k: "a"
x: context.custom.org[k].team`},
			want: [][]string{{"org"}},
		},
		"a selector after an integer index does not extend the path": {
			templates: []string{`x: context.custom.items[0].name`},
			want:      [][]string{{"items"}},
		},
		"a read inside a dynamic index is recorded": {
			templates: []string{`x: context.custom.items[context.custom.key]`},
			want:      [][]string{{"items"}, {"key"}},
		},
		"custom alone has no key": {
			templates: []string{`if context.custom != _|_ {x: 1}`},
		},
		"another context field is ignored": {
			templates: []string{`x: context.customer.team`},
		},
		"duplicates and several templates are merged and sorted": {
			templates: []string{
				`a: context.custom.zone
b: context.custom.team`,
				`c: context.custom.team`,
			},
			want: [][]string{{"team"}, {"zone"}},
		},
		"a template that does not parse contributes nothing": {
			templates: []string{`x: {`, `y: context.custom.team`},
			want:      [][]string{{"team"}},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, contextCustomPaths(tc.templates...))
		})
	}
}

func TestContextDefaultedParams(t *testing.T) {
	cases := map[string]struct {
		template string
		want     map[string]bool
	}{
		"none": {
			template: `parameter: {team: string, replicas: *1 | int}`,
		},
		"a default from context": {
			template: `parameter: team: *context.custom.team | string`,
			want:     map[string]bool{"team": true},
		},
		"nested, in either struct form": {
			template: `parameter: {
	meta: team: *context.custom.org.team | string
	spec: {zone: *context.custom.zone | string}
}`,
			want: map[string]bool{"meta.team": true, "spec.zone": true},
		},
		"a default computed from context": {
			template: `import "strings"
parameter: team: *strings.ToLower(context.custom.team) | string`,
			want: map[string]bool{"team": true},
		},
		"a parameter given as a definition": {
			template: `parameter: #P
#P: team: *context.custom.team | string`,
			want: map[string]bool{"team": true},
		},
		"a field given as a definition holding the default": {
			template: `parameter: team: #Team
#Team: *context.custom.team | string`,
			want: map[string]bool{"team": true},
		},
		"a nested field given as a definition holding the default": {
			template: `parameter: meta: owner: #Owner
#Owner: *context.custom.owner | string`,
			want: map[string]bool{"meta.owner": true},
		},
		"a definition unified into the parameter": {
			template: `parameter: #P & {replicas: *1 | int}
#P: team: *context.custom.team | string`,
			want: map[string]bool{"team": true},
		},
		"a definition embedded in the parameter": {
			template: `parameter: {
	#P
	replicas: *1 | int
}
#P: meta: team: *context.custom.team | string`,
			want: map[string]bool{"meta.team": true},
		},
		"a definition that refers to itself": {
			template: `parameter: #P
#P: {next?: #P, team: *context.custom.team | string}`,
			want: map[string]bool{"team": true},
		},
		"context outside the default does not count": {
			template: `parameter: team: *"x" | context.custom.team`,
		},
		"another context field does not count": {
			template: `parameter: name: *context.name | string`,
		},
		"outside parameter does not count": {
			template: `output: team: *context.custom.team | string`,
		},
		"a template that does not parse": {
			template: `parameter: {`,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := contextDefaultedParams(tc.template)
			if tc.want == nil {
				require.Empty(t, got)
				return
			}
			require.Equal(t, tc.want, got)
		})
	}
}

// A parameter defaulted from policy context is optional: an Application-scoped
// policy supplies context.custom when the controller renders. Admission runs
// before any policy, where the key does not exist yet and the default does not
// resolve, so the parameter must not be reported missing.
func TestValidateComponentParamsWithContextCustomDefaults(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate,
		features.EnableCueValidation, true)

	const output = `
output: {apiVersion: "v1", kind: "ConfigMap", metadata: name: context.name}
`
	cases := map[string]struct {
		template string
		params   map[string]any
		wantErr  string
	}{
		"a default read from context": {
			template: `parameter: team: *context.custom.team | string`,
		},
		"a nested default": {
			template: `parameter: meta: team: *context.custom.org.team | string`,
		},
		"a typed default": {
			template: `parameter: replicas: *context.custom.replicas | int`,
		},
		// The default is unresolved at admission, so a value only the other
		// branch would refuse gets through; the render, where context.custom is
		// concrete, refuses it.
		"a value of the wrong type for a context default is left to the render": {
			template: `parameter: replicas: *context.custom.replicas | int`,
			params:   map[string]any{"replicas": "three"},
		},
		"a value of the wrong type is still refused without one": {
			template: `parameter: replicas: *3 | int`,
			params:   map[string]any{"replicas": "three"},
			wantErr:  "parameter constraint violation",
		},
		"a required parameter is still required": {
			template: `parameter: team: string`,
			wantErr:  "missing parameters: team",
		},
		// A type on the key collapses `*string | string` to string, so the
		// evaluated schema has no default; the declaration still says policy
		// context supplies it.
		"a key the template types itself": {
			template: `context: custom: team: string
parameter: team: *context.custom.team | string`,
		},
		"a nested key the template types itself": {
			template: `context: custom: org: team: =~"^[a-z]+$"
parameter: meta: team: *context.custom.org.team | string`,
		},
		"a required sibling of a context default is still required": {
			template: `context: custom: team: string
parameter: {
	team: *context.custom.team | string
	image: string
}`,
			wantErr: "missing parameters: image",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tmpl := tc.template + output
			wl := &Component{
				Name:               "c",
				Type:               "t",
				Params:             tc.params,
				FullTemplate:       &Template{TemplateStr: tmpl},
				CapabilityCategory: "CUE",
			}
			if wl.Params == nil {
				wl.Params = map[string]any{}
			}
			app := &Appfile{Name: "app", Namespace: "ns", app: &v1beta1.Application{}}
			ctxData := velaprocess.ContextData{CompName: "c", AppName: "app", Namespace: "ns"}

			err := (&Parser{}).ValidateComponentParams(ctxData, wl, app)
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantErr)
			}
			// The placeholders are admission's alone: the template the
			// controller renders is untouched.
			require.Equal(t, tmpl, wl.FullTemplate.TemplateStr)
		})
	}
}
