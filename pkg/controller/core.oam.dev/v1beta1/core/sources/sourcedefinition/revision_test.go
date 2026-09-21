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

package sourcedefinition

import (
	"context"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	apitypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	velacommon "github.com/oam-dev/kubevela/pkg/utils/common"
)

func newSourceDef(template string) *v1beta1.SourceDefinition {
	return &v1beta1.SourceDefinition{
		TypeMeta:   metav1.TypeMeta{APIVersion: "core.oam.dev/v1beta1", Kind: "SourceDefinition"},
		ObjectMeta: metav1.ObjectMeta{Name: "atlas", Namespace: "vela-system"},
		Spec: v1beta1.SourceDefinitionSpec{
			Schematic: &common.Schematic{CUE: &common.CUE{Template: template}},
		},
	}
}

func newReconciler(objs ...client.Object) *Reconciler {
	cli := fake.NewClientBuilder().
		WithScheme(velacommon.Scheme).
		WithObjects(objs...).
		WithStatusSubresource(&v1beta1.SourceDefinition{}).
		Build()
	return &Reconciler{
		Client: cli,
		Scheme: velacommon.Scheme,
		record: event.NewNopRecorder(),
		options: options{
			defRevLimit:    5,
			cacheGCEnabled: false,
		},
	}
}

func listRevisions(t *testing.T, cli client.Client) []v1beta1.DefinitionRevision {
	revs := &v1beta1.DefinitionRevisionList{}
	require.NoError(t, cli.List(context.Background(), revs,
		client.InNamespace("vela-system"),
		client.MatchingLabels{oam.LabelSourceDefinitionName: "atlas"}))
	return revs.Items
}

// The reported symptom: applying a SourceDefinition produced no
// DefinitionRevision at all, so there was no history and nothing to pin to.
func TestReconcileCreatesDefinitionRevision(t *testing.T) {
	r := require.New(t)
	def := newSourceDef(`schema: {clusterName: string}`)
	rec := newReconciler(def)
	ctx := context.Background()
	key := types.NamespacedName{Name: "atlas", Namespace: "vela-system"}

	_, err := rec.Reconcile(ctx, ctrl.Request{NamespacedName: key})
	r.NoError(err)

	revs := listRevisions(t, rec.Client)
	r.Len(revs, 1, "reconciling a SourceDefinition must produce a DefinitionRevision")
	r.Equal("atlas-v1", revs[0].Name)
	r.Equal(common.SourceType, revs[0].Spec.DefinitionType)
	r.NotEmpty(revs[0].Spec.RevisionHash)
	r.Equal("atlas", revs[0].Spec.SourceDefinition.Name,
		"the revision must carry the definition, or pinning resolves to nothing")

	// status.latestRevision is what CleanUpDefinitionRevision and the next
	// reconcile both read; without it every reconcile starts from scratch.
	live := &v1beta1.SourceDefinition{}
	r.NoError(rec.Client.Get(ctx, key, live))
	r.NotNil(live.Status.LatestRevision)
	r.Equal("atlas-v1", live.Status.LatestRevision.Name)
}

// Reconciling is not a one-shot: it runs on every resync, and an unchanged
// definition must not mint a revision each time.
func TestReconcileIsIdempotent(t *testing.T) {
	r := require.New(t)
	rec := newReconciler(newSourceDef(`schema: {clusterName: string}`))
	ctx := context.Background()
	key := types.NamespacedName{Name: "atlas", Namespace: "vela-system"}

	for i := 0; i < 3; i++ {
		_, err := rec.Reconcile(ctx, ctrl.Request{NamespacedName: key})
		r.NoError(err)
	}
	r.Len(listRevisions(t, rec.Client), 1, "an unchanged definition must stay at one revision")
}

// A changed template is a new revision, or pinning has nothing to distinguish.
func TestReconcileCreatesNewRevisionOnChange(t *testing.T) {
	r := require.New(t)
	rec := newReconciler(newSourceDef(`schema: {clusterName: string}`))
	ctx := context.Background()
	key := types.NamespacedName{Name: "atlas", Namespace: "vela-system"}

	_, err := rec.Reconcile(ctx, ctrl.Request{NamespacedName: key})
	r.NoError(err)

	live := &v1beta1.SourceDefinition{}
	r.NoError(rec.Client.Get(ctx, key, live))
	live.Spec.Schematic.CUE.Template = `schema: {clusterName: string, region: string}`
	r.NoError(rec.Client.Update(ctx, live))

	_, err = rec.Reconcile(ctx, ctrl.Request{NamespacedName: key})
	r.NoError(err)

	revs := listRevisions(t, rec.Client)
	r.Len(revs, 2, "a changed template must mint a second revision")
	names := []string{revs[0].Name, revs[1].Name}
	r.ElementsMatch([]string{"atlas-v1", "atlas-v2"}, names)

	r.NoError(rec.Client.Get(ctx, key, live))
	r.Equal("atlas-v2", live.Status.LatestRevision.Name)
}

// The generated ConfigTemplate carries the owning SourceDefinition, so the cache
// GC sweep can tell a live template from an orphan without parsing its CUE.
//
// Nothing asserted this before: the sweep's own test hand-builds a ConfigMap
// with the labels already on it, so the controller could have stopped setting
// them and every test would still have passed.
func TestReconcileLabelsTheGeneratedConfigTemplate(t *testing.T) {
	r := require.New(t)
	def := newSourceDef(`
schema: {host: string}
$internal: {key: "atlas", keyInputs: []}
output: {host: "example.com"}
`)
	rec := newReconciler(def)
	_, err := rec.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "vela-system", Name: "atlas"}})
	r.NoError(err)

	var templates configv1alpha1.ConfigTemplateList
	r.NoError(rec.Client.List(context.Background(), &templates,
		client.InNamespace("vela-system"),
		client.HasLabels{oam.LabelSourceDefinitionName}))
	r.Len(templates.Items, 1, "the reconcile must write exactly one labelled ConfigTemplate")

	ct := templates.Items[0]
	r.Equal("atlas", ct.Labels[oam.LabelSourceDefinitionName])
	r.Equal("vela-system", ct.Labels[apitypes.LabelSourceDefinitionNamespace])
	r.Contains(ct.Spec.Template, "parameter:", "the schema CUE is carried onto the CR")
}
