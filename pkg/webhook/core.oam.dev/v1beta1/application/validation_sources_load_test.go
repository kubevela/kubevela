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
	"context"
	"strings"
	"testing"

	"cuelang.org/go/cue"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// Reducing a template to its parameter block is what keeps the type check
// independent of which CUE providers happen to be registered with the compiler
// in hand. A template it cannot reduce falls back to the full compile, so
// "cannot reduce" has to mean cannot, not "did not bother".
func TestExtractParameterBlock(t *testing.T) {
	for _, tc := range []struct {
		name     string
		tmpl     string
		ok       bool
		contains []string
		excludes []string
	}{
		{
			name: "keeps the parameter block and drops the body",
			tmpl: `
import "vela/kube"

parameter: {
	host: string
	port: *8080 | int
}
output: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	data: host: parameter.host
}`,
			ok:       true,
			contains: []string{"parameter:", "host:", "port:"},
			excludes: []string{"ConfigMap", "vela/kube"},
		},
		{
			name: "keeps definitions the parameter block may reference",
			tmpl: `
#Port: int & >0

parameter: {
	port: #Port
}
output: {}`,
			ok:       true,
			contains: []string{"#Port", "parameter:"},
			excludes: []string{"output"},
		},
		{
			name: "a template with no parameter block cannot be reduced",
			tmpl: `output: {kind: "ConfigMap"}`,
			ok:   false,
		},
		{
			name: "nor can one that will not parse",
			tmpl: `parameter: {this is not cue`,
			ok:   false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src, ok := extractParameterBlock(tc.tmpl)
			require.Equal(t, tc.ok, ok)
			if !tc.ok {
				return
			}
			for _, want := range tc.contains {
				require.Contains(t, src, want)
			}
			for _, unwanted := range tc.excludes {
				require.NotContains(t, src, unwanted,
					"the body is dropped so the reduction needs no providers")
			}
		})
	}
}

// The reduction is memoised on the template text. A second call must answer the
// same, including when the answer was "no".
func TestParameterBlockSourceIsMemoised(t *testing.T) {
	tmpl := `parameter: {host: string}` + "\noutput: {}"
	first, ok1 := parameterBlockSource(tmpl)
	second, ok2 := parameterBlockSource(tmpl)
	require.True(t, ok1)
	require.Equal(t, ok1, ok2)
	require.Equal(t, first, second)

	none := `output: {}`
	_, nok1 := parameterBlockSource(none)
	_, nok2 := parameterBlockSource(none)
	require.False(t, nok1)
	require.Equal(t, nok1, nok2, "a negative answer is cached as faithfully as a positive one")
}

// The refusals, which decide whether the caller falls back to a full compile.
func TestParameterBlockOnlyRefusals(t *testing.T) {
	_, ok := parameterBlockOnly(`output: {}`)
	require.False(t, ok, "no parameter block, nothing to check against")

	_, ok = parameterBlockOnly("parameter: {host: string & int}\noutput: {}")
	require.False(t, ok, "a block that cannot compile yields nothing rather than a broken validator")

	param, ok := parameterBlockOnly("parameter: {host: string, port: *8080 | int}\noutput: {}")
	require.True(t, ok)
	require.True(t, param.requiredAt(segs("host")))
	require.False(t, param.requiredAt(segs("port")), "a default means the value is not required")
}

// A definition whose parameter block references the file around it cannot be
// reduced usefully, and the caller falls back to a full compile.
func TestExtractParameterBlockKeepsOnlyWhatItCanCompileAlone(t *testing.T) {
	src, ok := extractParameterBlock(`
parameter: {
	image: _imageDefault
}
_imageDefault: "nginx"
output: {}`)
	require.True(t, ok, "the parameter block is present, so the reduction is attempted")
	require.NotContains(t, src, "_imageDefault: \"nginx\"",
		"a non-definition helper is not carried, which is why the compile below can fail")
	require.True(t, strings.Contains(src, "parameter:"))

	_, compiled := parameterBlockOnly(`
parameter: {
	image: _imageDefault
}
_imageDefault: "nginx"
output: {}`)
	require.False(t, compiled, "so the caller falls back to compiling the whole template")
}

// getDefinitionTemplate is how the type check reaches the parameter block it
// validates against. A kind it cannot fetch, or one with no CUE, has to report
// that rather than an empty template that would type-check anything.
func TestGetDefinitionTemplate(t *testing.T) {
	sc := runtime.NewScheme()
	require.NoError(t, v1beta1.SchemeBuilder.AddToScheme(sc))

	cue := func(tmpl string) *oamcommon.Schematic {
		return &oamcommon.Schematic{CUE: &oamcommon.CUE{Template: tmpl}}
	}
	objs := []client.Object{
		&v1beta1.ComponentDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "webservice", Namespace: "vela-system"},
			Spec:       v1beta1.ComponentDefinitionSpec{Schematic: cue("parameter: {image: string}")},
		},
		&v1beta1.TraitDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "scaler", Namespace: "vela-system"},
			Spec:       v1beta1.TraitDefinitionSpec{Schematic: cue("parameter: {replicas: int}")},
		},
		&v1beta1.WorkflowStepDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "notify", Namespace: "vela-system"},
			Spec:       v1beta1.WorkflowStepDefinitionSpec{Schematic: cue("parameter: {message: string}")},
		},
		&v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "override", Namespace: "vela-system"},
			Spec:       v1beta1.PolicyDefinitionSpec{Schematic: cue("parameter: {owner: string}")},
		},
		// Declared, but with no CUE to check against.
		&v1beta1.ComponentDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "bare", Namespace: "vela-system"},
		},
	}
	h := &ValidatingHandler{Client: fake.NewClientBuilder().WithScheme(sc).WithObjects(objs...).Build()}
	ctx := context.Background()

	for _, tc := range []struct {
		kind, name string
		want       string
		ok         bool
	}{
		{"component", "webservice", "parameter: {image: string}", true},
		{"trait", "scaler", "parameter: {replicas: int}", true},
		{"workflowstep", "notify", "parameter: {message: string}", true},
		{"policy", "override", "parameter: {owner: string}", true},
		{"component", "bare", "", false},
		{"component", "absent", "", false},
		{"trait", "absent", "", false},
		{"workflowstep", "absent", "", false},
		{"policy", "absent", "", false},
		{"sourcedefinition", "anything", "", false},
		{"", "anything", "", false},
	} {
		t.Run(tc.kind+"/"+tc.name, func(t *testing.T) {
			got, ok := h.getDefinitionTemplate(ctx, "default", tc.kind, tc.name, nil)
			require.Equal(t, tc.ok, ok)
			require.Equal(t, tc.want, got)
		})
	}
}

// Best-effort by design: a definition it cannot reach yields nil so validation
// fails open rather than blocking a legitimate apply.
func TestLoadTargetParameterFailsOpen(t *testing.T) {
	sc := runtime.NewScheme()
	require.NoError(t, v1beta1.SchemeBuilder.AddToScheme(sc))
	h := &ValidatingHandler{Client: fake.NewClientBuilder().WithScheme(sc).WithObjects(
		&v1beta1.ComponentDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "webservice", Namespace: "vela-system"},
			Spec: v1beta1.ComponentDefinitionSpec{Schematic: &oamcommon.Schematic{
				CUE: &oamcommon.CUE{Template: "parameter: {image: string}\noutput: {}"}}},
		},
		&v1beta1.ComponentDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "no-parameter", Namespace: "vela-system"},
			Spec: v1beta1.ComponentDefinitionSpec{Schematic: &oamcommon.Schematic{
				CUE: &oamcommon.CUE{Template: `output: {kind: "ConfigMap"}`}}},
		},
	).Build()}
	ctx := context.Background()

	param := h.loadTargetParameter(ctx, "default", "component", "webservice", nil)
	require.NotNil(t, param)
	require.True(t, param.requiredAt(segs("image")))

	require.Nil(t, h.loadTargetParameter(ctx, "default", "component", "absent", nil),
		"a definition that is not there yet must not block the apply")
	require.Nil(t, h.loadTargetParameter(ctx, "default", "component", "no-parameter", nil),
		"a definition with no parameter block has nothing to check against")
}

// The source parameter block was extracted on its own, so a definition writing
// `parameter: #Params` lost the #Params it referenced and would not compile.
// loadSourceParameter then reported a failure and every binding of that source
// was rejected. The target-definition extractor already kept them.
func TestLoadSourceParameterKeepsReferencedDefinitions(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))

	def := &v1beta1.SourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-params", Namespace: "vela-system"},
		Spec: v1beta1.SourceDefinitionSpec{
			Schematic: &oamcommon.Schematic{CUE: &oamcommon.CUE{Template: `
#Params: {
	host: string
	port: *443 | int
}
schema: {url: string}
$internal: {key: "shared-params", keyInputs: []}
parameter: #Params
output: {url: "https://\(parameter.host)"}
`}},
		},
	}

	h := &ValidatingHandler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(def).Build()}
	param, err := h.loadSourceParameter(context.Background(), "default", "shared-params", nil)
	require.NoError(t, err)
	require.NotNil(t, param)

	kind, declared := param.kindAt(segs("host"))
	require.True(t, declared, "host must resolve through the referenced definition")
	require.Equal(t, cue.StringKind, kind)
	require.True(t, param.requiredAt(segs("host")))
	require.False(t, param.requiredAt(segs("port")), "port has a default")
}

// A pinned type names a DefinitionRevision, not the live definition, so looking
// for an object literally called "webservice@v1" found nothing and
// loadTargetParameter failed open: the type check was skipped and a mismatched
// expression reached render.
func TestGetDefinitionTemplateResolvesAPinnedRevision(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))

	const pinned = `parameter: {image: string, replicas: int}`
	live := &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "webservice", Namespace: "vela-system"},
		Spec: v1beta1.ComponentDefinitionSpec{
			Schematic: &oamcommon.Schematic{CUE: &oamcommon.CUE{Template: `parameter: {image: string}`}},
		},
	}
	rev := &v1beta1.DefinitionRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name: "webservice-v1", Namespace: "vela-system",
			Labels: map[string]string{oam.LabelComponentDefinitionName: "webservice"},
		},
		Spec: v1beta1.DefinitionRevisionSpec{
			Revision:       1,
			DefinitionType: oamcommon.ComponentType,
			ComponentDefinition: v1beta1.ComponentDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "webservice"},
				Spec: v1beta1.ComponentDefinitionSpec{
					Schematic: &oamcommon.Schematic{CUE: &oamcommon.CUE{Template: pinned}},
				},
			},
		},
	}

	h := &ValidatingHandler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(live, rev).Build()}

	tmpl, ok := h.getDefinitionTemplate(context.Background(), "default", "component", "webservice@v1", nil)
	require.True(t, ok, "the pinned revision must resolve")
	require.Equal(t, pinned, tmpl)

	param := h.loadTargetParameter(context.Background(), "default", "component", "webservice@v1", nil)
	require.NotNil(t, param, "a pinned target must still be type-checked")
	_, declared := param.kindAt(segs("replicas"))
	require.True(t, declared, "the revision's parameter block is the one that applies")
}
