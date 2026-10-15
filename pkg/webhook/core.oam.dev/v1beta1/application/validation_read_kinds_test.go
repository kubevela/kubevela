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
	"fmt"
	"testing"

	wfv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// The producer's template, in two revisions: v1 wrote replicas as text.
const serverTemplate = `
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: name: context.name
	spec: replicas: parameter.replicas
}
outputs: svc: {
	apiVersion: "v1"
	kind:       "Service"
	spec: ports: [{port: parameter.port}]
}
parameter: {
	image:    *"nginx" | string
	replicas: *1 | int
	port:     *80 | int
	note?:    string
}
`

const serverV1Template = `
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: name: context.name
	spec: replicas: "\(parameter.replicas)"
}
parameter: {
	image:    *"nginx" | string
	replicas: *1 | int
	port:     *80 | int
}
`

func readKindsDefinitions() []runtime.Object {
	cue := func(template string) *common.Schematic {
		return &common.Schematic{CUE: &common.CUE{Template: template}}
	}
	server := &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "server", Namespace: "vela-system"},
		Spec:       v1beta1.ComponentDefinitionSpec{Schematic: cue(serverTemplate)},
	}
	v1 := &v1beta1.DefinitionRevision{
		ObjectMeta: metav1.ObjectMeta{Name: "server-v1", Namespace: "vela-system"},
		Spec: v1beta1.DefinitionRevisionSpec{Revision: 1, DefinitionType: common.ComponentType,
			ComponentDefinition: v1beta1.ComponentDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "server", Namespace: "vela-system"},
				Spec:       v1beta1.ComponentDefinitionSpec{Schematic: cue(serverV1Template)},
			}},
	}
	trait := func(name, template string) *v1beta1.TraitDefinition {
		return &v1beta1.TraitDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "vela-system"},
			Spec:       v1beta1.TraitDefinitionSpec{Schematic: cue(template)},
		}
	}
	return []runtime.Object{server, v1,
		trait("scaler", `
patch: spec: replicas: parameter.replicas
parameter: replicas: int
`),
		trait("ingress", `
outputs: ingress: {
	apiVersion: "networking.k8s.io/v1"
	kind:       "Ingress"
	spec: rules: [{host: parameter.host}]
}
parameter: host: string
`),
		trait("svc-type", `
patchOutputs: svc: spec: type: parameter.type
parameter: type: string
`),
		&v1beta1.WorkflowStepDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "notify", Namespace: "vela-system"},
			Spec: v1beta1.WorkflowStepDefinitionSpec{Schematic: cue(`
parameter: {
	text:   string
	count?: int
}
`)},
		},
		&v1beta1.SourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "regions", Namespace: "vela-system"},
			Spec: v1beta1.SourceDefinitionSpec{Schematic: cue(`
schema: list: [...{name: string, wave: int}]
output: list: [{name: "eu", wave: 1}]
parameter: {}
`)},
		},
	}
}

func TestComponentReadsAreTypedFromTheProducer(t *testing.T) {
	db := func(props string, traits ...common.ApplicationTrait) common.ApplicationComponent {
		return common.ApplicationComponent{Name: "db", Type: "server", Properties: rawJSON(props), Traits: traits}
	}
	reader := func(props string) common.ApplicationComponent {
		return common.ApplicationComponent{Name: "api", Type: "server", Properties: rawJSON(props)}
	}
	traitOn := func(typ, props string) common.ApplicationTrait {
		return common.ApplicationTrait{Type: typ, Properties: rawJSON(props)}
	}
	for _, tc := range []struct {
		name       string
		components []common.ApplicationComponent
		refused    string // a fragment of the one refusal, or "" for none
	}{
		{name: "a string field into an int parameter is refused",
			components: []common.ApplicationComponent{db(`{}`), reader(`{"replicas":"$(component.db.output.metadata.name)"}`)},
			refused:    "expression $(component.db.output.metadata.name) is string but component \"server\" parameter expects int"},
		{name: "a string field into a string parameter",
			components: []common.ApplicationComponent{db(`{}`), reader(`{"image":"$(component.db.output.metadata.name)"}`)}},
		{name: "an int field into an int parameter",
			components: []common.ApplicationComponent{db(`{}`), reader(`{"port":"$(component.db.output.spec.replicas)"}`)}},
		{name: "an int field into a string parameter is refused",
			components: []common.ApplicationComponent{db(`{}`), reader(`{"image":"$(component.db.output.spec.replicas)"}`)},
			refused:    "is int but component \"server\" parameter expects string"},
		{name: "an int field in text is text",
			components: []common.ApplicationComponent{db(`{}`), reader(`{"image":"r$(component.db.output.spec.replicas)"}`)}},
		{name: "the whole workload into a string parameter is refused",
			components: []common.ApplicationComponent{db(`{}`), reader(`{"image":"$(component.db.output)"}`)},
			refused:    "parameter expects string"},
		{name: "a live field the template does not write fits anything",
			components: []common.ApplicationComponent{db(`{}`), reader(`{"replicas":"$(component.db.output.metadata.uid)"}`)}},
		{name: "status fits anything",
			components: []common.ApplicationComponent{db(`{}`), reader(`{"replicas":"$(component.db.output.status.readyReplicas)"}`)}},
		{name: "a resource's status fits anything",
			components: []common.ApplicationComponent{db(`{}`), reader(`{"replicas":"$(component.db.outputs.svc.status.loadBalancer.ingress[0].ip)"}`)}},
		{name: "a resource's whole status fits anything",
			components: []common.ApplicationComponent{db(`{}`), reader(`{"image":"$(component.db.outputs.svc.status)"}`)}},
		{name: "a resource the component applies is typed",
			components: []common.ApplicationComponent{db(`{}`), reader(`{"image":"$(component.db.outputs.svc.spec.ports[0].port)"}`)},
			refused:    "is int but component \"server\" parameter expects string"},
		{name: "a read at a cluster is typed",
			components: []common.ApplicationComponent{db(`{}`), reader(`{"replicas":"$(component.db.cluster(\"local\").output.metadata.name)"}`)},
			refused:    "parameter expects int"},
		{name: "a hyphenated producer, read by index, is typed",
			components: []common.ApplicationComponent{
				{Name: "my-db", Type: "server", Properties: rawJSON(`{}`)},
				reader(`{"replicas":"$(component[\"my-db\"].output.metadata.name)"}`)},
			refused: "parameter expects int"},
		{name: "a read in a trait's properties is typed",
			components: []common.ApplicationComponent{db(`{}`), {Name: "api", Type: "server", Properties: rawJSON(`{}`),
				Traits: []common.ApplicationTrait{traitOn("scaler", `{"replicas":"$(component.db.output.metadata.name)"}`)}}},
			refused: "trait \"scaler\" parameter expects int"},
		{name: "the producer's revision decides: v1 writes replicas as text",
			components: []common.ApplicationComponent{
				{Name: "db", Type: "server@v1", Properties: rawJSON(`{}`)},
				reader(`{"port":"$(component.db.output.spec.replicas)"}`)},
			refused: "is string but component \"server\" parameter expects int"},
		{name: "a patch trait on the producer leaves its workload untyped",
			components: []common.ApplicationComponent{db(`{}`, traitOn("scaler", `{"replicas":2}`)),
				reader(`{"replicas":"$(component.db.output.metadata.name)"}`)}},
		{name: "a patch trait leaves the producer's resources typed",
			components: []common.ApplicationComponent{db(`{}`, traitOn("scaler", `{"replicas":2}`)),
				reader(`{"image":"$(component.db.outputs.svc.spec.ports[0].port)"}`)},
			refused: "is int but"},
		{name: "a resource a trait adds is typed",
			components: []common.ApplicationComponent{db(`{}`, traitOn("ingress", `{"host":"db.local"}`)),
				reader(`{"replicas":"$(component.db.outputs.ingress.spec.rules[0].host)"}`)},
			refused: "is string but component \"server\" parameter expects int"},
		{name: "patchOutputs leaves the producer's resources untyped",
			components: []common.ApplicationComponent{db(`{}`, traitOn("svc-type", `{"type":"NodePort"}`)),
				reader(`{"image":"$(component.db.outputs.svc.spec.ports[0].port)"}`)}},
		{name: "patchOutputs leaves the workload typed",
			components: []common.ApplicationComponent{db(`{}`, traitOn("svc-type", `{"type":"NodePort"}`)),
				reader(`{"replicas":"$(component.db.output.metadata.name)"}`)},
			refused: "is string but"},
		{name: "a producer trait that cannot be found may patch anything",
			components: []common.ApplicationComponent{db(`{}`, traitOn("missing", `{}`)),
				reader(`{"replicas":"$(component.db.output.metadata.name)"}`)}},
		{name: "a producer whose definition cannot be found declares nothing",
			components: []common.ApplicationComponent{{Name: "db", Type: "missing", Properties: rawJSON(`{}`)},
				reader(`{"replicas":"$(component.db.output.metadata.name)"}`)}},
		{name: "a guarded read is still typed by what it reads",
			components: []common.ApplicationComponent{db(`{}`),
				reader(`{"replicas":"$(has(component.db.output.metadata.name) ? component.db.output.metadata.name : 1)"}`)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cli := fake.NewClientBuilder().WithScheme(readKindsScheme()).WithRuntimeObjects(readKindsDefinitions()...).Build()
			app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
				Spec: v1beta1.ApplicationSpec{Components: tc.components}}
			errs := (&ValidatingHandler{Client: cli}).ValidateSources(context.Background(), app)
			if tc.refused == "" {
				require.Empty(t, errs)
				return
			}
			require.Len(t, errs, 1, "errors: %v", errs)
			require.Contains(t, errs[0].Error(), tc.refused)
		})
	}
}

func TestLoopReadsAreTypedFromTheItems(t *testing.T) {
	items := func(raw string) *wfv1alpha1.ForEach {
		return &wfv1alpha1.ForEach{Items: &apiextensionsv1.JSON{Raw: []byte(raw)}}
	}
	notify := func(name, props string) wfv1alpha1.WorkflowStep {
		return wfv1alpha1.WorkflowStep{WorkflowStepBase: wfv1alpha1.WorkflowStepBase{
			Name: name, Type: "notify", Properties: rawJSON(props)}}
	}
	loop := func(forEach *wfv1alpha1.ForEach, props string) wfv1alpha1.WorkflowStep {
		s := notify("loop", props)
		s.ForEach = forEach
		return s
	}
	group := func(forEach *wfv1alpha1.ForEach, props string) wfv1alpha1.WorkflowStep {
		return wfv1alpha1.WorkflowStep{
			WorkflowStepBase: wfv1alpha1.WorkflowStepBase{Name: "group", Type: "step-group"},
			ForEach:          forEach,
			SubSteps: []wfv1alpha1.WorkflowStepBase{
				{Name: "first", Type: "notify", Properties: rawJSON(`{"text":"hi"}`)},
				{Name: "second", Type: "notify", Properties: rawJSON(props)},
			},
		}
	}
	waves := items(`[{"region":"eu","wave":1},{"region":"us","wave":2}]`)
	for _, tc := range []struct {
		name    string
		steps   []wfv1alpha1.WorkflowStep
		refused string
	}{
		{name: "an int item into a string parameter is refused",
			steps:   []wfv1alpha1.WorkflowStep{loop(items(`[1, 2]`), `{"text":"$(each.item)"}`)},
			refused: "expression $(each.item) is int but workflow step \"notify\" parameter expects string"},
		{name: "a string item into a string parameter",
			steps: []wfv1alpha1.WorkflowStep{loop(items(`["eu", "us"]`), `{"text":"$(each.item)"}`)}},
		{name: "items of two kinds fit anything",
			steps: []wfv1alpha1.WorkflowStep{loop(items(`[1, "us"]`), `{"text":"$(each.item)"}`)}},
		{name: "an item's int field into a string parameter is refused",
			steps:   []wfv1alpha1.WorkflowStep{loop(waves, `{"text":"$(each.item.wave)"}`)},
			refused: "is int but"},
		{name: "an item's string field into a string parameter",
			steps: []wfv1alpha1.WorkflowStep{loop(waves, `{"text":"$(each.item.region)"}`)}},
		{name: "an item's int field into an int parameter",
			steps: []wfv1alpha1.WorkflowStep{loop(waves, `{"text":"hi","count":"$(each.item.wave)"}`)}},
		{name: "an item's field in text is text",
			steps: []wfv1alpha1.WorkflowStep{loop(waves, `{"text":"wave $(each.item.wave)"}`)}},
		{name: "a field no item has fits anything",
			steps: []wfv1alpha1.WorkflowStep{loop(waves, `{"text":"$(each.item.zone)"}`)}},
		{name: "the whole item into a string parameter is refused",
			steps:   []wfv1alpha1.WorkflowStep{loop(waves, `{"text":"$(each.item)"}`)},
			refused: "parameter expects string"},
		{name: "the index into a string parameter is refused",
			steps:   []wfv1alpha1.WorkflowStep{loop(waves, `{"text":"$(each.index)"}`)},
			refused: "expression $(each.index) is int but"},
		{name: "the index into an int parameter",
			steps: []wfv1alpha1.WorkflowStep{loop(waves, `{"text":"hi","count":"$(each.index)"}`)}},
		{name: "the index is typed when the items are not",
			steps:   []wfv1alpha1.WorkflowStep{loop(&wfv1alpha1.ForEach{From: "regions"}, `{"text":"$(each.index)"}`)},
			refused: "is int but"},
		{name: "items from a workflow variable fit anything",
			steps: []wfv1alpha1.WorkflowStep{loop(&wfv1alpha1.ForEach{From: "regions"}, `{"text":"$(each.item)"}`)}},
		{name: "items read from a source are typed by its schema",
			steps:   []wfv1alpha1.WorkflowStep{loop(items(`"$(source.r.list)"`), `{"text":"$(each.item.wave)"}`)},
			refused: "is int but"},
		{name: "a source item's string field",
			steps: []wfv1alpha1.WorkflowStep{loop(items(`"$(source.r.list)"`), `{"text":"$(each.item.name)"}`)}},
		{name: "items computed from a source fit anything",
			steps: []wfv1alpha1.WorkflowStep{loop(items(`"$(source.r.list.filter(x, x.wave > 1))"`), `{"text":"$(each.item.wave)"}`)}},
		{name: "a sub-step of a looping group is typed",
			steps:   []wfv1alpha1.WorkflowStep{group(waves, `{"text":"$(each.item.wave)"}`)},
			refused: "spec.workflow.steps[0].subSteps[1].properties.text"},
		{name: "a sub-step's string field",
			steps: []wfv1alpha1.WorkflowStep{group(waves, `{"text":"$(each.item.region)"}`)}},
		{name: "each step reads its own loop's items",
			steps: []wfv1alpha1.WorkflowStep{
				loop(items(`["eu"]`), `{"text":"$(each.item)"}`),
				func() wfv1alpha1.WorkflowStep {
					s := loop(items(`[1]`), `{"text":"hi","count":"$(each.item)"}`)
					s.Name = "other"
					return s
				}()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cli := fake.NewClientBuilder().WithScheme(readKindsScheme()).WithRuntimeObjects(readKindsDefinitions()...).Build()
			app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{Name: "db", Type: "server", Properties: rawJSON(`{}`)}},
					Sources:    []v1beta1.ApplicationSource{{Name: "r", Type: "regions", Properties: rawJSON(`{}`)}},
					Workflow:   &v1beta1.Workflow{Steps: tc.steps},
				}}
			errs := (&ValidatingHandler{Client: cli}).ValidateSources(context.Background(), app)
			if tc.refused == "" {
				require.Empty(t, errs)
				return
			}
			require.Len(t, errs, 1, "errors: %v", errs)
			require.Contains(t, errs[0].Error(), tc.refused)
		})
	}
}

func readKindsScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	if err := v1beta1.AddToScheme(scheme); err != nil {
		panic(fmt.Sprintf("scheme: %v", err))
	}
	return scheme
}
