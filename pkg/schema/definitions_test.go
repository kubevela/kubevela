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

package schema

import (
	"context"
	"testing"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
)

func definitionsClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func cueSchematic(template string) *common.Schematic {
	return &common.Schematic{CUE: &common.CUE{Template: template}}
}

func TestSourceSchemaText(t *testing.T) {
	source := func(ns, schema string) *v1beta1.SourceDefinition {
		return &v1beta1.SourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "atlas", Namespace: ns},
			Spec: v1beta1.SourceDefinitionSpec{Schematic: cueSchematic(schema + "\nparameter: {}\n")}}
	}
	pinned := &v1beta1.DefinitionRevision{
		ObjectMeta: metav1.ObjectMeta{Name: "atlas-v1", Namespace: "vela-system"},
		Spec: v1beta1.DefinitionRevisionSpec{DefinitionType: common.SourceType, Revision: 1,
			SourceDefinition: *source("vela-system", `schema: {name: string}`)},
	}
	ctx := context.Background()

	t.Run("from vela-system", func(t *testing.T) {
		text, err := SourceSchemaText(ctx, definitionsClient(t, source("vela-system", `schema: {name: string, region: string}`)),
			"default", "atlas", nil)
		require.NoError(t, err)
		require.Contains(t, text, "region")
		require.NotContains(t, text, "parameter", "only the schema block")
	})
	t.Run("the Application's namespace first", func(t *testing.T) {
		text, err := SourceSchemaText(ctx, definitionsClient(t,
			source("vela-system", `schema: {name: string}`), source("team", `schema: {name: string, team: string}`)),
			"team", "atlas", nil)
		require.NoError(t, err)
		require.Contains(t, text, "team")
	})
	t.Run("the revision a type names", func(t *testing.T) {
		text, err := SourceSchemaText(ctx, definitionsClient(t, source("vela-system", `schema: {name: string, region: string}`), pinned),
			"default", "atlas@v1", nil)
		require.NoError(t, err)
		require.Contains(t, text, "name")
		require.NotContains(t, text, "region")
	})
	t.Run("a definition with no schema", func(t *testing.T) {
		text, err := SourceSchemaText(ctx, definitionsClient(t, source("vela-system", `output: {}`)), "default", "atlas", nil)
		require.NoError(t, err)
		require.Empty(t, text)
	})
	t.Run("a missing definition is not found, by its type", func(t *testing.T) {
		_, err := SourceSchemaText(ctx, definitionsClient(t), "default", "atlas", nil)
		require.True(t, apierrors.IsNotFound(err))
		require.Contains(t, err.Error(), `sourcedefinitions.core.oam.dev "atlas" not found`)
	})
}

func TestTopLevelBlock(t *testing.T) {
	tmpl := `
schema: {
	name: string
	nested: parameter: int
}
parameter: {count: int}
`
	got, err := TopLevelBlock(tmpl, "parameter")
	require.NoError(t, err)
	require.Contains(t, got, "count")
	require.NotContains(t, got, "name")

	got, err = TopLevelBlock(tmpl, "schema")
	require.NoError(t, err)
	require.Contains(t, got, "nested")

	got, err = TopLevelBlock(tmpl, "output")
	require.NoError(t, err)
	require.Empty(t, got)

	_, err = TopLevelBlock(`schema: {`, "schema")
	require.Error(t, err)
}

func TestProducedFor(t *testing.T) {
	component := func(name, ns, template string) *v1beta1.ComponentDefinition {
		return &v1beta1.ComponentDefinition{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: v1beta1.ComponentDefinitionSpec{Schematic: cueSchematic(template)}}
	}
	trait := func(name, ns, template string) *v1beta1.TraitDefinition {
		return &v1beta1.TraitDefinition{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: v1beta1.TraitDefinitionSpec{Schematic: cueSchematic(template)}}
	}
	const server = `
output: {apiVersion: "apps/v1", kind: "Deployment", metadata: name: context.name, spec: replicas: parameter.replicas}
outputs: svc: {apiVersion: "v1", kind: "Service", spec: ports: [{port: 80}]}
parameter: replicas: *1 | int
`
	const textServer = `
output: {apiVersion: "apps/v1", kind: "Deployment", spec: replicas: "\(parameter.replicas)"}
parameter: replicas: *1 | int
`
	child := component("child", "vela-system", `
parameter: replicas: *1 | int
$super: properties: replicas: parameter.replicas
output: metadata: labels: tier: "db"
`)
	child.Spec.Extends = "server"
	revision := &v1beta1.DefinitionRevision{
		ObjectMeta: metav1.ObjectMeta{Name: "server-v1", Namespace: "vela-system"},
		Spec: v1beta1.DefinitionRevisionSpec{DefinitionType: common.ComponentType, Revision: 1,
			ComponentDefinition: *component("server", "vela-system", textServer)},
	}
	cli := definitionsClient(t,
		component("server", "vela-system", server), revision, child,
		component("team-server", "team", textServer),
		trait("scaler", "vela-system", "patch: spec: replicas: parameter.replicas\nparameter: replicas: int"),
		trait("svc-type", "vela-system", "patchOutputs: svc: spec: type: \"NodePort\""),
		trait("ingress", "vela-system", "outputs: ingress: {apiVersion: \"networking.k8s.io/v1\", kind: \"Ingress\", spec: host: parameter.host}\nparameter: host: string"),
		trait("other-svc", "vela-system", "outputs: svc: {apiVersion: \"v1\", kind: \"Service\", spec: clusterIP: \"None\"}"),
		trait("team-ingress", "team", "outputs: ingress: {apiVersion: \"networking.k8s.io/v1\", kind: \"Ingress\", spec: host: parameter.host}\nparameter: host: string"),
	)
	withTraits := func(name, typ string, traits ...string) common.ApplicationComponent {
		c := common.ApplicationComponent{Name: name, Type: typ}
		for _, tr := range traits {
			c.Traits = append(c.Traits, common.ApplicationTrait{Type: tr})
		}
		return c
	}
	kind := func(t *testing.T, ns string, comp common.ApplicationComponent, expr string) (cue.Kind, bool) {
		t.Helper()
		produced := ProducedFor(context.Background(), cli, ns, []common.ApplicationComponent{comp}, nil)
		return ComponentReadKind(produced, readOf(t, expr))
	}

	for _, tc := range []struct {
		name string
		ns   string
		comp common.ApplicationComponent
		expr string
		kind cue.Kind
		ok   bool
	}{
		{"the workload", "default", withTraits("db", "server"), "component.db.output.spec.replicas", cue.IntKind, true},
		{"a resource", "default", withTraits("db", "server"), "component.db.outputs.svc.spec.ports[0].port", cue.IntKind, true},
		{"the revision a type names", "default", withTraits("db", "server@v1"), "component.db.output.spec.replicas", cue.StringKind, true},
		{"a definition in the Application's namespace", "team", withTraits("db", "team-server"), "component.db.output.spec.replicas", cue.StringKind, true},
		{"a definition in another namespace is not found", "default", withTraits("db", "team-server"), "component.db.output.spec.replicas", cue.BottomKind, false},
		{"a missing definition declares nothing", "default", withTraits("db", "missing"), "component.db.output.spec.replicas", cue.BottomKind, false},
		{"a patch trait leaves the workload untyped", "default", withTraits("db", "server", "scaler"), "component.db.output.spec.replicas", cue.BottomKind, false},
		{"and the resources typed", "default", withTraits("db", "server", "scaler"), "component.db.outputs.svc.spec.ports[0].port", cue.IntKind, true},
		{"patchOutputs leaves the resources untyped", "default", withTraits("db", "server", "svc-type"), "component.db.outputs.svc.spec.ports[0].port", cue.BottomKind, false},
		{"and the workload typed", "default", withTraits("db", "server", "svc-type"), "component.db.output.spec.replicas", cue.IntKind, true},
		{"a resource a trait adds", "default", withTraits("db", "server", "ingress"), "component.db.outputs.ingress.spec.host", cue.StringKind, true},
		{"a trait in the Application's namespace", "team", withTraits("db", "server", "team-ingress"), "component.db.outputs.ingress.spec.host", cue.StringKind, true},
		{"a resource declared twice is untyped", "default", withTraits("db", "server", "other-svc"), "component.db.outputs.svc.spec.ports[0].port", cue.BottomKind, false},
		{"a trait that cannot be found may patch the workload", "default", withTraits("db", "server", "missing"), "component.db.output.spec.replicas", cue.BottomKind, false},
		{"or the resources", "default", withTraits("db", "server", "missing"), "component.db.outputs.svc.spec.ports[0].port", cue.BottomKind, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			k, ok := kind(t, tc.ns, tc.comp, tc.expr)
			require.Equal(t, tc.ok, ok)
			if ok {
				require.Equal(t, tc.kind, k)
			}
		})
	}

	t.Run("what a definition extends, with inheritance on", func(t *testing.T) {
		featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableDefinitionInheritance, true)
		k, ok := kind(t, "default", withTraits("db", "child"), "component.db.output.spec.replicas")
		require.True(t, ok, "the parent's field")
		require.Equal(t, cue.IntKind, k)
		k, ok = kind(t, "default", withTraits("db", "child"), "component.db.output.metadata.labels.tier")
		require.True(t, ok, "and the child's own")
		require.Equal(t, cue.StringKind, k)
		k, ok = kind(t, "default", withTraits("db", "child"), "component.db.outputs.svc.spec.ports[0].port")
		require.True(t, ok, "and the parent's resources")
		require.Equal(t, cue.IntKind, k)
	})

	t.Run("a definition that extends another, with inheritance off, declares nothing", func(t *testing.T) {
		featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableDefinitionInheritance, false)
		_, ok := kind(t, "default", withTraits("db", "child"), "component.db.output.spec.replicas")
		require.False(t, ok)
	})

	t.Run("every component, by name", func(t *testing.T) {
		produced := ProducedFor(context.Background(), cli, "default",
			[]common.ApplicationComponent{withTraits("a", "server"), withTraits("b", "server@v1"), withTraits("c", "missing")}, nil)
		require.Contains(t, produced, "a")
		require.Contains(t, produced, "b")
		require.NotContains(t, produced, "c")
	})
}
