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
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	wfTypesv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue/definition"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// How a module type reference (Form 2 or 3) behaves once it has left type
// resolution: in the workload label, in an application revision, in the
// admission placeholder check and in the runtime-parameter helpers.

func TestTraitGetTypeLabelFallsBackToName(t *testing.T) {
	assert.Equal(t, "widget-kit-v1-note", (&Trait{Name: "note", TypeLabel: "widget-kit-v1-note"}).GetTypeLabel())
	assert.Equal(t, "note", (&Trait{Name: "note"}).GetTypeLabel(), "a Trait built without a TypeLabel is labelled by its name")
}

// renderedWorkload renders a one-ConfigMap component through the same path the
// controller uses and returns the workload type label it was stamped with.
func renderedWorkloadTypeLabel(t *testing.T, comp *Component) string {
	t.Helper()
	pCtx := velaprocess.NewContext(velaprocess.ContextData{AppName: "app", CompName: comp.Name, Namespace: "default", AppRevisionName: "app-v1"})
	require.NoError(t, definition.NewWorkloadAbstractEngine(comp.Name).Complete(pCtx, `
output: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: name: context.name
}
parameter: {}
`, map[string]interface{}{}))
	workload, err := makeWorkloadWithContext(pCtx, comp, "default", "app")
	require.NoError(t, err)
	return workload.GetLabels()[oam.WorkloadTypeLabel]
}

func TestWorkloadTypeLabelForModuleReferences(t *testing.T) {
	compDef := &v1beta1.ComponentDefinition{ObjectMeta: metav1.ObjectMeta{Name: "s3-v1-bucket"}}
	wlDef := &v1beta1.WorkloadDefinition{ObjectMeta: metav1.ObjectMeta{Name: "s3-v1-bucket-wl"}}

	t.Run("Form 3 takes the installed ComponentDefinition name", func(t *testing.T) {
		comp := &Component{Name: "my-bucket", Type: "s3/v1/bucket", FullTemplate: &Template{ComponentDefinition: compDef}}
		assert.Equal(t, "s3-v1-bucket", renderedWorkloadTypeLabel(t, comp), "a slash is not a valid label value")
	})

	t.Run("Form 2 falls back to the WorkloadDefinition name", func(t *testing.T) {
		comp := &Component{Name: "my-bucket", Type: "v1/bucket", FullTemplate: &Template{WorkloadDefinition: wlDef}}
		assert.Equal(t, "s3-v1-bucket-wl", renderedWorkloadTypeLabel(t, comp))
	})

	t.Run("a nameless definition does not blank the label", func(t *testing.T) {
		// A template loaded from a file or an application revision carries a
		// definition with no metadata.name; the raw type is kept rather than "".
		comp := &Component{Name: "my-bucket", Type: "s3/v1/bucket", FullTemplate: &Template{ComponentDefinition: &v1beta1.ComponentDefinition{}}}
		assert.Equal(t, "s3/v1/bucket", renderedWorkloadTypeLabel(t, comp))
	})

	t.Run("Form 1 keeps its short name", func(t *testing.T) {
		comp := &Component{Name: "my-bucket", Type: "bucket", FullTemplate: &Template{ComponentDefinition: compDef}}
		assert.Equal(t, "bucket", renderedWorkloadTypeLabel(t, comp), "revisioned names such as configmap-component-v1 must survive")
	})
}

func TestLoadTemplateRejectsAnUnparsableType(t *testing.T) {
	_, err := LoadTemplate(context.Background(), resolutionClient(t), "a/b/c/d", types.TypeComponentDefinition, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "3 segments")
}

// moduleRevision is an application revision that snapshotted one legacy
// definition and one module-installed definition of every kind, plus two
// modules that both provide a v1 "bucket".
func moduleRevision() *v1beta1.ApplicationRevision {
	labelled := func(module, apiVersion, name string) metav1.ObjectMeta {
		return metav1.ObjectMeta{Labels: stampedLabels(module, apiVersion, name)}
	}
	return &v1beta1.ApplicationRevision{
		ObjectMeta: metav1.ObjectMeta{Name: "app-v3"},
		Spec: v1beta1.ApplicationRevisionSpec{
			ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
				ComponentDefinitions: map[string]*v1beta1.ComponentDefinition{
					"legacy":        {},
					"s3-v1-bucket":  {ObjectMeta: labelled("s3", "v1", "bucket")},
					"gcs-v1-bucket": {ObjectMeta: labelled("gcs", "v1", "bucket")},
					"kit-v1-widget": {ObjectMeta: labelled("kit", "v1", "widget")},
				},
				WorkloadDefinitions: map[string]v1beta1.WorkloadDefinition{
					"wl-v1-thing": {ObjectMeta: labelled("wl", "v1", "thing")},
				},
				TraitDefinitions: map[string]*v1beta1.TraitDefinition{
					"kit-v1-note": {ObjectMeta: labelled("kit", "v1", "note")},
				},
				PolicyDefinitions: map[string]v1beta1.PolicyDefinition{
					"kit-v1-quota": {ObjectMeta: labelled("kit", "v1", "quota")},
				},
				WorkflowStepDefinitions: map[string]*v1beta1.WorkflowStepDefinition{
					"kit-v1-step": {ObjectMeta: labelled("kit", "v1", "step")},
				},
				SourceDefinitions: map[string]*v1beta1.SourceDefinition{
					"kit-v1-src": {ObjectMeta: labelled("kit", "v1", "src")},
				},
			},
		},
	}
}

func TestResolveRevisionCapabilityName(t *testing.T) {
	apprev := moduleRevision()
	for _, tc := range []struct {
		name    string
		capName string
		capType types.CapType
		want    string
		wantErr string
	}{
		{"legacy name as is", "legacy", types.TypeComponentDefinition, "legacy", ""},
		{"Form 3 derives the snapshotted name", "s3/v1/bucket", types.TypeComponentDefinition, "s3-v1-bucket", ""},
		{"Form 3 not snapshotted is left for the caller", "nope/v1/bucket", types.TypeComponentDefinition, "nope/v1/bucket", ""},
		{"Form 2 with one match", "v1/widget", types.TypeComponentDefinition, "kit-v1-widget", ""},
		{"Form 2 with no match is left for the caller", "v1/missing", types.TypeComponentDefinition, "v1/missing", ""},
		{"Form 2 ambiguous", "v1/bucket", types.TypeComponentDefinition, "",
			`type "v1/bucket" is ambiguous in app revision app-v3: definitions [gcs-v1-bucket, s3-v1-bucket] all match`},
		{"Form 2 finds a WorkloadDefinition for a component", "v1/thing", types.TypeComponentDefinition, "wl-v1-thing", ""},
		{"Form 2 workload", "v1/thing", types.TypeWorkload, "wl-v1-thing", ""},
		{"Form 2 trait", "v1/note", types.TypeTrait, "kit-v1-note", ""},
		{"Form 2 policy", "v1/quota", types.TypePolicy, "kit-v1-quota", ""},
		{"Form 2 workflow step", "v1/step", types.TypeWorkflowStep, "kit-v1-step", ""},
		{"Form 2 source", "v1/src", types.TypeSource, "kit-v1-src", ""},
		{"Form 1 source exists", "kit-v1-src", types.TypeSource, "kit-v1-src", ""},
		{"unknown kind has no snapshot to search", "v1/src", types.CapType("scope"), "v1/src", ""},
		{"unparsable", "a/b/c/d", types.TypeComponentDefinition, "", "3 segments"},
		{"revisioned component found as a workload", "wl-v1-thing@v2", types.TypeComponentDefinition, "wl-v1-thing", ""},
		{"revisioned workload", "wl-v1-thing@v1", types.TypeWorkload, "wl-v1-thing", ""},
		{"revisioned name not snapshotted is parsed as written", "ghost@v1", types.TypeTrait, "ghost@v1", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveRevisionCapabilityName(tc.capName, tc.capType, apprev)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRendersPlaceholderDuringValidation(t *testing.T) {
	assert.True(t, rendersPlaceholderDuringValidation(&Component{Type: "module"}))
	assert.True(t, rendersPlaceholderDuringValidation(&Component{Type: "addon"}))
	assert.False(t, rendersPlaceholderDuringValidation(&Component{Type: "webservice"}))
	assert.False(t, rendersPlaceholderDuringValidation(&Component{Type: "s3/v1/bucket"}), "no template, nothing more to go on")

	moduleDef := &Template{ComponentDefinition: &v1beta1.ComponentDefinition{ObjectMeta: metav1.ObjectMeta{Name: "module"}}}
	assert.True(t, rendersPlaceholderDuringValidation(&Component{Type: "module@v2", FullTemplate: moduleDef}),
		"a revisioned reference is recognised through the definition it resolved to")
	other := &Template{ComponentDefinition: &v1beta1.ComponentDefinition{ObjectMeta: metav1.ObjectMeta{Name: "s3-v1-bucket"}}}
	assert.False(t, rendersPlaceholderDuringValidation(&Component{Type: "s3/v1/bucket", FullTemplate: other}))
}

func TestHasParamsSuppliedAtRuntime(t *testing.T) {
	assert.False(t, HasParamsSuppliedAtRuntime(&Appfile{Components: []common.ApplicationComponent{{Name: "web"}}}))

	viaStep := &Appfile{WorkflowSteps: []wfTypesv1alpha1.WorkflowStep{{WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
		Type:   "apply-component",
		Inputs: wfTypesv1alpha1.StepInputs{{From: "image", ParameterKey: "image"}},
	}}}}
	assert.True(t, HasParamsSuppliedAtRuntime(viaStep), "a workflow step input fills a parameter at runtime")

	viaComponent := &Appfile{Components: []common.ApplicationComponent{{
		Name:   "web",
		Inputs: wfTypesv1alpha1.StepInputs{{From: "image", ParameterKey: "image"}},
	}}}
	assert.True(t, HasParamsSuppliedAtRuntime(viaComponent), "a component input does too, with no explicit workflow")
}

func TestHasComponentParamsSuppliedAtRuntimeLooksAtTheComponentFirst(t *testing.T) {
	app := &Appfile{
		Components: []common.ApplicationComponent{
			{Name: "direct", Inputs: wfTypesv1alpha1.StepInputs{{From: "image", ParameterKey: "image"}}},
			{Name: "plain"},
		},
		WorkflowSteps: []wfTypesv1alpha1.WorkflowStep{
			// Not an apply-component step: its inputs do not count for anyone.
			{WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
				Type:       "notification",
				Properties: &runtime.RawExtension{Raw: []byte(`{"component":"plain"}`)},
				Inputs:     wfTypesv1alpha1.StepInputs{{From: "msg", ParameterKey: "message"}},
			}},
			// An apply-component step without properties names no component.
			{WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
				Type:   "apply-component",
				Inputs: wfTypesv1alpha1.StepInputs{{From: "image", ParameterKey: "image"}},
			}},
		},
	}
	assert.True(t, HasComponentParamsSuppliedAtRuntime(app, "direct"))
	assert.False(t, HasComponentParamsSuppliedAtRuntime(app, "plain"))
	assert.False(t, HasComponentParamsSuppliedAtRuntime(app, "absent"))
}
