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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue/definition"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func traitTemplateNamed(name string, labels map[string]string, cue string) *Template {
	return &Template{
		TemplateStr: cue,
		TraitDefinition: &v1beta1.TraitDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		},
	}
}

var moduleLabels = map[string]string{types.LabelDefinitionModule: "widget-kit"}

func TestTraitTypeLabel(t *testing.T) {
	cases := map[string]struct {
		traitName string
		templ     *Template
		want      string
	}{
		"module trait written as Form 1": {
			traitName: "note",
			templ:     traitTemplateNamed("widget-kit-v1-note", moduleLabels, ""),
			want:      "widget-kit-v1-note",
		},
		"module trait written as Form 3": {
			traitName: "widget-kit/v1/note",
			templ:     traitTemplateNamed("widget-kit-v1-note", moduleLabels, ""),
			want:      "widget-kit-v1-note",
		},
		"module trait written as the installed name": {
			traitName: "widget-kit-v1-note",
			templ:     traitTemplateNamed("widget-kit-v1-note", moduleLabels, ""),
			want:      "widget-kit-v1-note",
		},
		"legacy trait": {
			traitName: "scaler",
			templ:     traitTemplateNamed("scaler", nil, ""),
			want:      "scaler",
		},
		"revisioned legacy trait keeps the revisioned name": {
			traitName: "scaler-v1",
			templ:     traitTemplateNamed("scaler", nil, ""),
			want:      "scaler-v1",
		},
		"definition with no metadata.name, as loaded from a file": {
			traitName: "note",
			templ:     traitTemplateNamed("", moduleLabels, ""),
			want:      "note",
		},
		"template without a TraitDefinition": {
			traitName: "note",
			templ:     &Template{},
			want:      "note",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, traitTypeLabel(tc.traitName, tc.templ))
		})
	}
}

// A Form 1 module trait keeps its written name as the trait type, but the
// objects it emits through outputs are labelled with the installed name.
func TestModuleTraitOutputsAreLabelledWithInstalledName(t *testing.T) {
	r := require.New(t)
	p := &Parser{}
	trait, err := p.convertTemplate2Trait("note", map[string]interface{}{"text": "hi"}, traitTemplateNamed("widget-kit-v1-note", moduleLabels, `
outputs: note: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: name: context.name + "-note"
	data: text: parameter.text
}
parameter: text: string
`))
	r.NoError(err)
	r.Equal("note", trait.Name)
	r.Equal("widget-kit-v1-note", trait.TypeLabel)

	pCtx := velaprocess.NewContext(velaprocess.ContextData{AppName: "app", CompName: "stamped-widget", Namespace: "default", AppRevisionName: "app-v1"})
	r.NoError(definition.NewWorkloadAbstractEngine("widget").Complete(pCtx, `
output: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: name: context.name
}
parameter: {}
`, map[string]interface{}{}))
	r.NoError(trait.EvalContext(pCtx))

	comp := &Component{Name: "stamped-widget", Type: "widget", Traits: []*Trait{trait}}
	manifest, err := evalWorkloadWithContext(pCtx, comp, "default", "app")
	r.NoError(err)
	r.Len(manifest.ComponentOutputsAndTraits, 1)
	r.Equal("widget-kit-v1-note", manifest.ComponentOutputsAndTraits[0].GetLabels()[oam.TraitTypeLabel])
}
