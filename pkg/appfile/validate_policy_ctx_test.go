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

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue/definition"
	"github.com/oam-dev/kubevela/pkg/definition/inherit"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/policy/ctxschema"
)

// identity is a global supplying a team, a cost centre and, for some tenants,
// a mesh domain.
const identity = `
import "strings"
schema: ctx: close({
	team:        =~"^[a-z]+$"
	costCentre:  strings.MinRunes(3)
	meshDomain?: string
	org: close({unit: string, zone?: string})
})
output: ctx: {team: "payments", costCentre: "cc1", org: unit: "u"}
`

// defaulted is a global whose schema defaults the team: the controller hands
// components the default when the output leaves it out.
const defaulted = `
schema: ctx: close({team: *"platform" | string})
output: ctx: {}
`

func view(templates ...string) *ctxschema.View {
	var policies []ctxschema.Policy
	for i, tmpl := range templates {
		policies = append(policies, ctxschema.Policy{Name: []string{"identity", "second", "third"}[i], Template: tmpl})
	}
	return ctxschema.Build(policies)
}

func configMapOutput(fields string) string {
	return "output: {apiVersion: \"v1\", kind: \"ConfigMap\", metadata: {name: context.name" + fields + "}}"
}

type ctxCase struct {
	component string
	params    map[string]any
	traits    []string
	ancestors []inherit.Level
	view      *ctxschema.View
	wantErr   []string
}

func validateWithView(t *testing.T, tc ctxCase) error {
	t.Helper()
	if len(tc.ancestors) > 0 {
		withInheritance(t)
	}
	params := tc.params
	if params == nil {
		params = map[string]any{}
	}
	comp := &Component{
		Name:               "api",
		Type:               "worker",
		CapabilityCategory: types.CUECategory,
		Params:             params,
		FullTemplate:       &Template{TemplateStr: tc.component, Ancestors: tc.ancestors},
		engine:             definition.NewWorkloadAbstractEngine("api", tc.ancestors...),
	}
	for i, tmpl := range tc.traits {
		name := []string{"mesh", "tags"}[i]
		comp.Traits = append(comp.Traits, &Trait{
			Name:               name,
			CapabilityCategory: types.CUECategory,
			Template:           tmpl,
			Params:             map[string]any{},
			engine:             definition.NewTraitAbstractEngine(name),
		})
	}
	af := &Appfile{
		Name: "app", Namespace: "ns", app: &v1beta1.Application{},
		ParsedComponents: []*Component{comp},
		PolicyContext:    tc.view,
	}
	templates := []string{comp.FullTemplate.TemplateStr}
	for _, tr := range comp.Traits {
		templates = append(templates, tr.Template)
	}
	err := (&Parser{}).ValidateCUESchematicAppfile(af)
	// The view is admission's alone: nothing it validated keeps it.
	require.Equal(t, templates[0], comp.FullTemplate.TemplateStr)
	for i, tr := range comp.Traits {
		require.Equal(t, templates[i+1], tr.Template)
	}
	return err
}

func runCtxCases(t *testing.T, cases map[string]ctxCase) {
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := validateWithView(t, tc)
			if len(tc.wantErr) == 0 {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			for _, want := range tc.wantErr {
				require.Contains(t, err.Error(), want)
			}
		})
	}
}

func TestValidateAgainstPolicyContext(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableCueValidation, true)
	runCtxCases(t, map[string]ctxCase{
		"no view: as before": {
			component: "parameter: {}\n" + configMapOutput(", labels: mesh: context.custom.anything"),
		},
		"a required key read directly": {
			component: "parameter: {}\n" + configMapOutput(", labels: team: context.custom.team"),
			view:      view(identity),
		},
		"a key used against its type": {
			component: "parameter: {}\noutput: {apiVersion: \"v1\", kind: \"ConfigMap\", metadata: name: context.name, data: replicas: int & context.custom.team}",
			view:      view(identity),
			wantErr:   []string{`component "api"`, "context.custom.team"},
		},
		"a key used against its constraint": {
			component: "parameter: {}\n" + configMapOutput(", labels: team: context.custom.team & \"PAYMENTS\""),
			view:      view(identity),
			wantErr:   []string{`component "api"`, "context.custom.team"},
		},
		"a standard-library constraint applies": {
			component: "parameter: {}\n" + configMapOutput(", labels: cc: context.custom.costCentre & \"x\""),
			view:      view(identity),
			wantErr:   []string{`component "api"`, "context.custom.costCentre"},
		},
		"a template with its own strings import": {
			component: "import \"strings\"\nparameter: {}\n" + configMapOutput(", labels: team: strings.ToLower(context.custom.team)"),
			view:      view(identity),
		},
		"a key the schema defaults, read directly": {
			component: "parameter: {}\n" + configMapOutput(", labels: team: context.custom.team"),
			view:      view(defaulted),
		},
		"a key the schema defaults, interpolated": {
			component: "parameter: {}\n" + configMapOutput(", labels: team: \"t-\\(context.custom.team)\""),
			view:      view(defaulted),
		},
		"a key the schema defaults, used against its type": {
			component: "parameter: {}\noutput: {apiVersion: \"v1\", kind: \"ConfigMap\", metadata: name: context.name, data: replicas: int & context.custom.team}",
			view:      view(defaulted),
			wantErr:   []string{`component "api"`, "context.custom.team"},
		},
		"an optional key read without a guard": {
			component: "parameter: {}\n" + configMapOutput(", labels: mesh: context.custom.meshDomain"),
			view:      view(identity),
			wantErr: []string{
				`component "api"`, "context.custom.meshDomain", `optional in policy "identity"`,
				"guard it", "default",
			},
		},
		"an optional key interpolated without a guard": {
			component: "parameter: {}\n" + configMapOutput(", annotations: url: \"https://\\(context.custom.meshDomain)/\""),
			view:      view(identity),
			wantErr:   []string{"context.custom.meshDomain", `optional in policy "identity"`},
		},
		"an optional key read with a guard": {
			component: "parameter: {}\n" + configMapOutput(", labels: {if context.custom.meshDomain != _|_ {mesh: context.custom.meshDomain}}"),
			view:      view(identity),
		},
		"an optional key with a default": {
			component: "parameter: {}\n_mesh: *\"none\" | string\nif context.custom.meshDomain != _|_ {_mesh: context.custom.meshDomain}\n" + configMapOutput(", labels: mesh: _mesh"),
			view:      view(identity),
		},
		"a nested optional key read without a guard": {
			component: "parameter: {}\n" + configMapOutput(", labels: zone: context.custom.org.zone"),
			view:      view(identity),
			wantErr:   []string{"context.custom.org.zone", `optional in policy "identity"`},
		},
		"an optional parameter read beside a guarded key of the same name": {
			component: "parameter: zone?: string\n" + configMapOutput(", labels: {z: parameter.zone, if context.custom.org.zone != _|_ {oz: context.custom.org.zone}}"),
			view:      view(identity),
		},
		"a read through an optional key": {
			component: "parameter: {}\n" + configMapOutput(", labels: owner: context.custom.owner.name"),
			view:      view("schema: ctx: close({owner?: close({name: string})})\noutput: ctx: {}\n"),
			wantErr:   []string{"context.custom.owner ", `optional in policy "identity"`},
		},
		"a parameter defaulted from an optional key": {
			component: "parameter: mesh: *context.custom.meshDomain | string\n" + configMapOutput(", labels: mesh: parameter.mesh"),
			view:      view(identity),
		},
		"a key no policy declares, under a complete view": {
			component: "parameter: {}\n" + configMapOutput(", labels: region: context.custom.region"),
			view:      view(identity),
			wantErr:   []string{`component "api"`, "context.custom.region", "no applicable policy declares"},
		},
		"a key no policy declares, guarded, under a complete view": {
			component: "parameter: {}\n" + configMapOutput(", labels: {if context.custom.region != _|_ {region: context.custom.region}}"),
			view:      view(identity),
		},
		"a key no policy declares, under an incomplete view": {
			component: "parameter: {}\n" + configMapOutput(", labels: region: context.custom.region"),
			view:      view(identity, `output: ctx: region: "eu"`),
		},
		"no policy sets ctx at all": {
			component: "parameter: {}\n" + configMapOutput(", labels: team: context.custom.team"),
			view:      view(`output: labels: a: "b"`),
			wantErr:   []string{"context.custom.team", "no applicable policy declares"},
		},
		"a key only a conditional policy declares": {
			component: "parameter: {}\n" + configMapOutput(", labels: tier: context.custom.tier"),
			view:      view(identity, "config: enabled: context.namespace == \"prod\"\nschema: ctx: close({tier: string})\noutput: ctx: tier: \"gold\""),
			wantErr:   []string{"context.custom.tier", `optional in policy "second"`},
		},
		"an auxiliary output reading an optional key": {
			component: "parameter: {}\n" + configMapOutput("") + "\noutputs: mesh: {apiVersion: \"v1\", kind: \"ConfigMap\", metadata: {name: \"m\", labels: mesh: context.custom.meshDomain}}",
			view:      view(identity),
			wantErr:   []string{"context.custom.meshDomain", `optional in policy "identity"`},
		},
		"a trait reading an optional key without a guard": {
			component: "parameter: {}\n" + configMapOutput(""),
			traits:    []string{"parameter: {}\npatch: metadata: labels: mesh: context.custom.meshDomain"},
			view:      view(identity),
			wantErr:   []string{`trait "mesh"`, "context.custom.meshDomain", `optional in policy "identity"`},
		},
		"a trait reading an optional key with a guard": {
			component: "parameter: {}\n" + configMapOutput(""),
			traits:    []string{"parameter: {}\npatch: metadata: labels: {if context.custom.meshDomain != _|_ {mesh: context.custom.meshDomain}}"},
			view:      view(identity),
		},
		"a trait using a key against its type": {
			component: "parameter: {}\n" + configMapOutput(""),
			traits:    []string{"parameter: {}\npatch: data: n: int & context.custom.team"},
			view:      view(identity),
			wantErr:   []string{`trait "mesh"`, "context.custom.team"},
		},
		"a trait using one of two keys against its type": {
			component: "parameter: {}\n" + configMapOutput(""),
			traits:    []string{"parameter: {}\npatch: data: {cc: context.custom.costCentre, n: int & context.custom.team}"},
			view:      view(identity),
			wantErr:   []string{`trait "mesh": uses context.custom.team against its type`},
		},
		"a trait reading a key no policy declares, under a complete view": {
			component: "parameter: {}\n" + configMapOutput(""),
			traits:    []string{"parameter: {}\npatch: metadata: labels: region: context.custom.region"},
			view:      view(identity),
			wantErr:   []string{`trait "mesh": reads context.custom.region, but no applicable policy declares it`},
		},
		"the second trait is the one blamed": {
			component: "parameter: {}\n" + configMapOutput(""),
			traits: []string{
				"parameter: {}\npatch: metadata: labels: team: context.custom.team",
				"parameter: {}\npatch: data: n: int & context.custom.team",
			},
			view:    view(identity),
			wantErr: []string{`trait "tags": uses context.custom.team against its type`},
		},
		"a trait with its own strings import": {
			component: "parameter: {}\n" + configMapOutput(""),
			traits:    []string{"import \"strings\"\nparameter: {}\npatch: metadata: labels: team: strings.ToLower(context.custom.team)"},
			view:      view(identity),
		},
		"a component extending another": {
			component: "parameter: image: string\n$super: properties: {}",
			ancestors: []inherit.Level{{Name: "base", Template: "parameter: {}\n" + configMapOutput(", labels: {team: context.custom.team, cc: context.custom.costCentre}")}},
			params:    map[string]any{"image": "nginx"},
			view:      view(identity),
		},
		"a parent using a key against its type": {
			component: "parameter: image: string\n$super: properties: {}",
			ancestors: []inherit.Level{{Name: "base", Template: "parameter: {}\noutput: {apiVersion: \"v1\", kind: \"ConfigMap\", metadata: name: context.name, data: replicas: int & context.custom.team}"}},
			params:    map[string]any{"image": "nginx"},
			view:      view(identity),
			wantErr:   []string{`component "api"`, "context.custom.team"},
		},
	})
}

func TestValidateAgainstPolicyContextWithoutCueValidation(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableCueValidation, false)
	runCtxCases(t, map[string]ctxCase{
		"an optional key read without a guard is still refused": {
			component: "parameter: {}\n" + configMapOutput(", labels: mesh: context.custom.meshDomain"),
			view:      view(identity),
			wantErr:   []string{"context.custom.meshDomain"},
		},
		"a required key read directly": {
			component: "parameter: {}\n" + configMapOutput(", labels: team: context.custom.team"),
			view:      view(identity),
		},
	})
}

// Pins the CUE messages the read checks depend on, so a CUE upgrade that
// rewords them fails here.
func TestReadErrorLabels(t *testing.T) {
	cases := map[string]struct {
		src  string
		want []readLabel
	}{
		"optional": {
			src:  "c: close({m?: string})\no: x: c.m",
			want: []readLabel{{name: "m", optional: true}},
		},
		"optional, interpolated": {
			src:  "c: close({m?: string})\no: x: \"a-\\(c.m)\"",
			want: []readLabel{{name: "m", optional: true}},
		},
		"undefined": {
			src:  "c: close({m?: string})\no: x: c.u",
			want: []readLabel{{name: "u"}},
		},
		"a quoted label": {
			src:  "c: close({\"cost-centre\"?: string})\no: x: c[\"cost-centre\"]",
			want: []readLabel{{name: "cost-centre", optional: true}},
		},
		"neither": {
			src: "o: x: int",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			v := cuecontext.New().CompileString(tc.src)
			err := v.Err()
			if err == nil {
				err = v.LookupPath(cue.ParsePath("o")).Validate(cue.Concrete(true))
			}
			require.Equal(t, tc.want, readErrorLabels(err))
		})
	}
}

func TestReadsPolicyContext(t *testing.T) {
	comp := func(tmpl string, ancestors []string, traits ...string) *Component {
		c := &Component{FullTemplate: &Template{TemplateStr: tmpl}}
		for _, a := range ancestors {
			c.FullTemplate.Ancestors = append(c.FullTemplate.Ancestors, inherit.Level{Name: "a", Template: a})
		}
		for _, tr := range traits {
			c.Traits = append(c.Traits, &Trait{Template: tr})
		}
		return c
	}
	cases := map[string]struct {
		comps []*Component
		want  bool
	}{
		"none":                    {[]*Component{comp(`output: name: context.name`, nil)}, false},
		"a component reads a key": {[]*Component{comp(`output: x: context.custom.team`, nil)}, true},
		"an ancestor does":        {[]*Component{comp(`output: {}`, []string{`output: x: context.custom.team`})}, true},
		"a trait does":            {[]*Component{comp(`output: {}`, nil, `patch: x: context.custom.team`)}, true},
		"the second component":    {[]*Component{comp(`output: {}`, nil), comp(`output: x: context.custom.team`, nil)}, true},
		"only custom as a whole":  {[]*Component{comp(`if context.custom != _|_ {output: {}}`, nil)}, false},
		"no template":             {[]*Component{{}}, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, ReadsPolicyContext(&Appfile{ParsedComponents: tc.comps}))
		})
	}
}
