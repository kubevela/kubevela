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

package core

import (
	"context"
	"fmt"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/event"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func numbersClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

// newWebservice is a definition as it is first created: no revision in its status.
func newWebservice(workload string) *v1beta1.ComponentDefinition {
	return &v1beta1.ComponentDefinition{
		TypeMeta:   metav1.TypeMeta{APIVersion: "core.oam.dev/v1beta1", Kind: "ComponentDefinition"},
		ObjectMeta: metav1.ObjectMeta{Name: "webservice", Namespace: "vela-system", UID: "second-incarnation"},
		Spec:       v1beta1.ComponentDefinitionSpec{Workload: common.WorkloadTypeDescriptor{Type: workload}},
	}
}

// A definition deleted and created again must not reuse its numbers: a pin on an old number
// would otherwise bind to the new content.
func TestARecreatedDefinitionContinuesItsNumbering(t *testing.T) {
	ctx := context.Background()
	numbers := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: DefinitionRevisionNumbersName, Namespace: "vela-system"},
		Data:       map[string]string{revisionNumberKey(common.ComponentType, "webservice"): `{"latest":3}`},
	}
	cli := numbersClient(t, numbers)

	defRev, isNew, err := GenerateDefinitionRevision(ctx, cli, newWebservice("deployments.apps"))
	require.NoError(t, err)
	require.True(t, isNew)
	require.Equal(t, "webservice-v4", defRev.Name)
	require.EqualValues(t, 4, defRev.Spec.Revision)
}

// Revisions left behind by an orphaning delete hold the old content under the old numbers.
func TestOrphanedRevisionsAreNumberedPast(t *testing.T) {
	ctx := context.Background()
	orphan := &v1beta1.DefinitionRevision{
		ObjectMeta: metav1.ObjectMeta{Name: "webservice-v2", Namespace: "vela-system",
			Labels: map[string]string{oam.LabelComponentDefinitionName: "webservice"}},
		Spec: v1beta1.DefinitionRevisionSpec{DefinitionType: common.ComponentType, Revision: 2},
	}
	otherKind := &v1beta1.DefinitionRevision{
		ObjectMeta: metav1.ObjectMeta{Name: "webservice-trait-v9", Namespace: "vela-system",
			Labels: map[string]string{oam.LabelTraitDefinitionName: "webservice"}},
		Spec: v1beta1.DefinitionRevisionSpec{DefinitionType: common.TraitType, Revision: 9},
	}
	cli := numbersClient(t, orphan, otherKind)

	defRev, isNew, err := GenerateDefinitionRevision(ctx, cli, newWebservice("deployments.apps"))
	require.NoError(t, err)
	require.True(t, isNew)
	require.Equal(t, "webservice-v3", defRev.Name, "past the orphan, and a trait of the same name does not count")
}

func TestReconcilingADefinitionRecordsItsNumber(t *testing.T) {
	ctx := context.Background()
	cli := numbersClient(t)
	def := newWebservice("deployments.apps")

	require.NoError(t, cli.Create(ctx, def))
	defRev, result, err := ReconcileDefinitionRevision(ctx, cli, event.NewNopRecorder(), def, 10, func(*common.Revision) error { return nil })
	require.NoError(t, err)
	require.Nil(t, result)
	require.Equal(t, "webservice-v1", defRev.Name, "a definition never seen starts at v1")

	numbers := &corev1.ConfigMap{}
	require.NoError(t, cli.Get(ctx, client.ObjectKey{Namespace: "vela-system", Name: DefinitionRevisionNumbersName}, numbers))
	require.JSONEq(t, `{"latest":1,"hashes":{"1":"`+defRev.Spec.RevisionHash+`"}}`, numbers.Data[revisionNumberKey(common.ComponentType, "webservice")])

	// the highest number never goes down, and a late write still records its hash
	numbers.Data[revisionNumberKey(common.ComponentType, "webservice")] = `{"latest":5}`
	require.NoError(t, cli.Update(ctx, numbers))
	require.NoError(t, recordRevisionNumber(ctx, cli, "vela-system", common.ComponentType, "webservice", 2, "h2"))
	require.NoError(t, cli.Get(ctx, client.ObjectKeyFromObject(numbers), numbers))
	require.JSONEq(t, `{"latest":5,"hashes":{"2":"h2"}}`, numbers.Data[revisionNumberKey(common.ComponentType, "webservice")])
}

func TestTheRecordKeepsTheNewestHashes(t *testing.T) {
	ctx := context.Background()
	cli := numbersClient(t)
	for n := int64(1); n <= recordedHashes+5; n++ {
		require.NoError(t, recordRevisionNumber(ctx, cli, "vela-system", common.ComponentType, "webservice", n, fmt.Sprintf("h%d", n)))
	}
	recorded, err := readRevisionNumbers(ctx, cli, "vela-system", common.ComponentType, "webservice")
	require.NoError(t, err)
	require.EqualValues(t, recordedHashes+5, recorded.Latest)
	require.Len(t, recorded.Hashes, recordedHashes)
	require.NotContains(t, recorded.Hashes, "5", "the oldest go first")
	require.Contains(t, recorded.Hashes, "6")
}

// hashOf is the revision hash a definition gets.
func hashOf(t *testing.T, def *v1beta1.ComponentDefinition) string {
	t.Helper()
	defRev, _, err := GatherRevisionInfo(def)
	require.NoError(t, err)
	return defRev.Spec.RevisionHash
}

// A definition deleted and created again with content it once had gets that content's number
// back, so pins on it keep working: a GitOps replace deletes and re-creates unchanged.
func TestARecreatedDefinitionRestoresTheNumberItsContentHad(t *testing.T) {
	ctx := context.Background()
	a, b := newWebservice("deployments.apps"), newWebservice("statefulsets.apps")
	numbers := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: DefinitionRevisionNumbersName, Namespace: "vela-system"},
		Data: map[string]string{revisionNumberKey(common.ComponentType, "webservice"): fmt.Sprintf(
			`{"latest":3,"hashes":{"1":%q,"2":%q,"3":"other"}}`, hashOf(t, a), hashOf(t, b))},
	}
	cli := numbersClient(t, numbers)

	defRev, isNew, err := GenerateDefinitionRevision(ctx, cli, newWebservice("statefulsets.apps"))
	require.NoError(t, err)
	require.True(t, isNew)
	require.Equal(t, "webservice-v2", defRev.Name)
	require.EqualValues(t, 2, defRev.Spec.Revision)

	defRev, _, err = GenerateDefinitionRevision(ctx, cli, newWebservice("daemonsets.apps"))
	require.NoError(t, err)
	require.Equal(t, "webservice-v4", defRev.Name, "content it never had takes a new number")
}

// Revisions an orphaning delete left behind are reused for the content they hold, whichever.
func TestASurvivingRevisionWithTheContentIsReused(t *testing.T) {
	ctx := context.Background()
	revOf := func(def *v1beta1.ComponentDefinition, n int64) *v1beta1.DefinitionRevision {
		rev, _, err := GatherRevisionInfo(def)
		require.NoError(t, err)
		rev.Name = fmt.Sprintf("webservice-v%d", n)
		rev.Namespace = "vela-system"
		rev.Labels = map[string]string{oam.LabelComponentDefinitionName: "webservice"}
		rev.OwnerReferences = nil
		rev.Spec.Revision = n
		return rev
	}
	cli := numbersClient(t, revOf(newWebservice("deployments.apps"), 1), revOf(newWebservice("statefulsets.apps"), 2))

	defRev, isNew, err := GenerateDefinitionRevision(ctx, cli, newWebservice("deployments.apps"))
	require.NoError(t, err)
	require.True(t, isNew, "the status still has to point at it")
	require.Equal(t, "webservice-v1", defRev.Name)
}

// A live definition that goes back to content it had is a new revision, as it always was: only
// a definition with no revision of its own yet takes an old number back.
func TestALiveDefinitionRevertingItsContentGetsANewNumber(t *testing.T) {
	ctx := context.Background()
	a := newWebservice("deployments.apps")
	numbers := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: DefinitionRevisionNumbersName, Namespace: "vela-system"},
		Data: map[string]string{revisionNumberKey(common.ComponentType, "webservice"): fmt.Sprintf(
			`{"latest":2,"hashes":{"1":%q,"2":"b"}}`, hashOf(t, a))},
	}
	cli := numbersClient(t, numbers)
	a.Status.LatestRevision = &common.Revision{Name: "webservice-v2", Revision: 2, RevisionHash: "b"}

	defRev, isNew, err := GenerateDefinitionRevision(ctx, cli, a)
	require.NoError(t, err)
	require.True(t, isNew)
	require.Equal(t, "webservice-v3", defRev.Name)
}

// A reconcile that reads the definition before its status records the revision it just made
// must find that revision, not mint a second one with the same content.
func TestAStaleStatusReusesTheRevisionItMade(t *testing.T) {
	ctx := context.Background()
	cli := numbersClient(t)
	def := newWebservice("deployments.apps")
	first, isNew, err := GenerateDefinitionRevision(ctx, cli, def)
	require.NoError(t, err)
	require.True(t, isNew)
	require.NoError(t, CreateDefinitionRevision(ctx, cli, def, first))

	again, isNew, err := GenerateDefinitionRevision(ctx, cli, newWebservice("deployments.apps"))
	require.NoError(t, err)
	require.True(t, isNew, "the status still has to point at it")
	require.Equal(t, "webservice-v1", again.Name)

	changed, _, err := GenerateDefinitionRevision(ctx, cli, newWebservice("statefulsets.apps"))
	require.NoError(t, err)
	require.Equal(t, "webservice-v2", changed.Name, "new content still gets a new number")
}
