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

	clustercommon "github.com/oam-dev/cluster-gateway/pkg/common"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestDeclaredOutputs(t *testing.T) {
	names := declaredOutputs(`
parameter: {port: int}
outputs: {
  svc: {apiVersion: "v1", kind: "Service"}
  "ingress-main": {apiVersion: "networking.k8s.io/v1", kind: "Ingress"}
}
outputs: extra: {}
`)
	require.ElementsMatch(t, []string{"svc", "ingress-main", "extra"}, names)

	// The usual idiom: an output declared only when a parameter asks for it.
	require.ElementsMatch(t, []string{"service", "route", "cert"}, declaredOutputs(`
parameter: {expose: bool, tls: bool}
if parameter.expose {
  outputs: service: {apiVersion: "v1", kind: "Service"}
}
outputs: {
  if parameter.expose { route: {} }
  for k, v in parameter { "gen-\(k)": {} }
}
if parameter.tls {
  outputs: {cert: {}}
}
`))
	require.Empty(t, declaredOutputs(`this is { not cue`))
}

// A post-dispatch trait's resources are applied only once the workflow has
// finished, which a reader waiting on one of them would stop it doing.
func TestReadOfAPostDispatchOutputIsRefused(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	trait := func(name string, stage v1beta1.StageType) *v1beta1.TraitDefinition {
		return &v1beta1.TraitDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "vela-system"},
			Spec: v1beta1.TraitDefinitionSpec{Stage: stage, Schematic: &common.Schematic{CUE: &common.CUE{
				Template: `outputs: svc: {apiVersion: "v1", kind: "Service"}`}}},
		}
	}
	h := &ValidatingHandler{Client: fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(trait("late-svc", v1beta1.PostDispatch), trait("early-svc", v1beta1.DefaultDispatch)).Build()}

	app := func(traitType string) *v1beta1.Application {
		return &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
			Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{
				{Name: "db", Type: "webservice", Properties: rawJSON(`{"image":"postgres"}`),
					Traits: []common.ApplicationTrait{{Type: traitType}}},
				{Name: "api", Type: "webservice", Properties: rawJSON(`{"image":"$(component.db.outputs.svc.spec.clusterIP)"}`)},
			}},
		}
	}

	errs := h.validatePostDispatchReads(context.Background(), app("late-svc"))
	require.Len(t, errs, 1)
	require.Equal(t, "spec.components[1].properties", errs[0].Field)
	require.True(t, strings.Contains(errs[0].Error(), `post-dispatch trait "late-svc"`), errs[0].Error())

	require.Empty(t, h.validatePostDispatchReads(context.Background(), app("early-svc")))

	// Even from the reader's own post-dispatch trait: post-dispatch traits are
	// not applied producer first, and one can render during the workflow.
	lateReader := app("late-svc")
	lateReader.Spec.Components[1].Properties = rawJSON(`{"image":"api"}`)
	lateReader.Spec.Components[1].Traits = []common.ApplicationTrait{{Type: "late-svc",
		Properties: rawJSON(`{"ip":"$(component.db.outputs.svc.spec.clusterIP)"}`)}}
	lateErrs := h.validatePostDispatchReads(context.Background(), lateReader)
	require.Len(t, lateErrs, 1)
	require.Equal(t, "spec.components[1].traits[0].properties", lateErrs[0].Field, "pointing at where the read is written")
}

// A cluster named in a read must be registered: a typo would otherwise leave the
// reader waiting for a placement that can never happen.
func TestReadOfAnUnregisteredClusterIsRefused(t *testing.T) {
	registered := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "east", Namespace: "vela-system",
		Labels: map[string]string{clustercommon.LabelKeyClusterCredentialType: "X509Certificate"}}}
	h := &ValidatingHandler{Client: fake.NewClientBuilder().WithScheme(common2.Scheme).WithObjects(registered).Build()}
	app := func(expr string) *v1beta1.Application {
		return &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
			Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{
				{Name: "db", Type: "webservice", Properties: rawJSON(`{"image":"postgres"}`)},
				{Name: "api", Type: "webservice", Properties: rawJSON(`{"image":"` + expr + `"}`)},
			}},
		}
	}

	require.Empty(t, h.validateReadClusters(context.Background(), app(`$(component.db.cluster(\"east\").output.status.x)`)))
	require.Empty(t, h.validateReadClusters(context.Background(), app(`$(component.db.cluster(\"local\").output.status.x)`)))

	errs := h.validateReadClusters(context.Background(), app(`$(component.db.cluster(\"eats\").namespace(\"orders\").output.status.x)`))
	require.Len(t, errs, 1)
	require.Contains(t, errs[0].Error(), `no cluster "eats" is registered`)
	require.Equal(t, "spec.components[1].properties", errs[0].Field)
}
