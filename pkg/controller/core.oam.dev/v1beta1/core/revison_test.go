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
	"errors"
	"fmt"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	workflowv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

func revisionClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).
		WithStatusSubresource(&v1beta1.ComponentDefinition{}, &v1beta1.TraitDefinition{},
			&v1beta1.PolicyDefinition{}, &v1beta1.WorkflowStepDefinition{}).Build()
}

// definitionOf builds one definition of each kind, with a spec that varies by template.
type definitionOf struct {
	kind     common.DefinitionType
	label    string
	build    func(name, template string) util.ConditionedObject
	setLast  func(def util.ConditionedObject, rev *common.Revision)
	setField func(def util.ConditionedObject, version string)
}

var definitionKinds = []definitionOf{
	{
		kind: common.ComponentType, label: oam.LabelComponentDefinitionName,
		build: func(name, template string) util.ConditionedObject {
			return &v1beta1.ComponentDefinition{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "team-a"},
				Spec: v1beta1.ComponentDefinitionSpec{Schematic: &common.Schematic{CUE: &common.CUE{Template: template}}}}
		},
		setLast: func(d util.ConditionedObject, r *common.Revision) {
			d.(*v1beta1.ComponentDefinition).Status.LatestRevision = r
		},
		setField: func(d util.ConditionedObject, v string) { d.(*v1beta1.ComponentDefinition).Spec.Version = v },
	},
	{
		kind: common.TraitType, label: oam.LabelTraitDefinitionName,
		build: func(name, template string) util.ConditionedObject {
			return &v1beta1.TraitDefinition{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "team-a"},
				Spec: v1beta1.TraitDefinitionSpec{Schematic: &common.Schematic{CUE: &common.CUE{Template: template}}}}
		},
		setLast: func(d util.ConditionedObject, r *common.Revision) {
			d.(*v1beta1.TraitDefinition).Status.LatestRevision = r
		},
		setField: func(d util.ConditionedObject, v string) { d.(*v1beta1.TraitDefinition).Spec.Version = v },
	},
	{
		kind: common.PolicyType, label: oam.LabelPolicyDefinitionName,
		build: func(name, template string) util.ConditionedObject {
			return &v1beta1.PolicyDefinition{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "team-a"},
				Spec: v1beta1.PolicyDefinitionSpec{Schematic: &common.Schematic{CUE: &common.CUE{Template: template}}}}
		},
		setLast: func(d util.ConditionedObject, r *common.Revision) {
			d.(*v1beta1.PolicyDefinition).Status.LatestRevision = r
		},
		setField: func(d util.ConditionedObject, v string) { d.(*v1beta1.PolicyDefinition).Spec.Version = v },
	},
	{
		kind: common.WorkflowStepType, label: oam.LabelWorkflowStepDefinitionName,
		build: func(name, template string) util.ConditionedObject {
			return &v1beta1.WorkflowStepDefinition{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "team-a"},
				Spec: v1beta1.WorkflowStepDefinitionSpec{Schematic: &common.Schematic{CUE: &common.CUE{Template: template}}}}
		},
		setLast: func(d util.ConditionedObject, r *common.Revision) {
			d.(*v1beta1.WorkflowStepDefinition).Status.LatestRevision = r
		},
		setField: func(d util.ConditionedObject, v string) { d.(*v1beta1.WorkflowStepDefinition).Spec.Version = v },
	},
}

// Each kind's definition gets v1 first, keeps it while its spec is unchanged, and gets v2 when
// the spec changes.
func TestGenerateDefinitionRevisionNumbersEachChange(t *testing.T) {
	ctx := context.Background()
	for _, k := range definitionKinds {
		t.Run(string(k.kind), func(t *testing.T) {
			cli := revisionClient(t)
			def := k.build("db", "output: {}")

			first, isNew, err := GenerateDefinitionRevision(ctx, cli, def)
			require.NoError(t, err)
			require.True(t, isNew)
			require.Equal(t, "db-v1", first.Name)
			require.EqualValues(t, 1, first.Spec.Revision)
			require.Equal(t, k.kind, first.Spec.DefinitionType)
			require.NotEmpty(t, first.Spec.RevisionHash)
			require.Equal(t, "db", first.OwnerReferences[0].Name)
			require.NoError(t, CreateDefinitionRevision(ctx, cli, def, first.DeepCopy()))

			last := &common.Revision{Name: first.Name, Revision: 1, RevisionHash: first.Spec.RevisionHash}
			k.setLast(def, last)
			same, isNew, err := GenerateDefinitionRevision(ctx, cli, def)
			require.NoError(t, err)
			require.False(t, isNew, "an unchanged spec keeps its revision")
			require.Equal(t, "db-v1", same.Name)

			changed := k.build("db", "output: {kind: \"x\"}")
			k.setLast(changed, last)
			next, isNew, err := GenerateDefinitionRevision(ctx, cli, changed)
			require.NoError(t, err)
			require.True(t, isNew)
			require.Equal(t, "db-v2", next.Name)
			require.EqualValues(t, 2, next.Spec.Revision)
		})
	}
}

// A stored revision whose hash matches but whose spec differs is a new revision, and a latest
// revision that cannot be read is an error.
func TestGenerateDefinitionRevisionChecksTheStoredSpec(t *testing.T) {
	ctx := context.Background()
	def := definitionKinds[0].build("db", "output: {}")
	rev, _, err := GatherRevisionInfo(def)
	require.NoError(t, err)
	last := &common.Revision{Name: "db-v1", Revision: 1, RevisionHash: rev.Spec.RevisionHash}
	definitionKinds[0].setLast(def, last)

	_, _, err = GenerateDefinitionRevision(ctx, revisionClient(t), def)
	require.ErrorContains(t, err, "get the definitionRevision db-v1")

	stored := definitionKinds[0].build("db", "output: {stale: true}").(*v1beta1.ComponentDefinition)
	cli := revisionClient(t, &v1beta1.DefinitionRevision{
		ObjectMeta: metav1.ObjectMeta{Name: "db-v1", Namespace: "team-a"},
		Spec:       v1beta1.DefinitionRevisionSpec{DefinitionType: common.ComponentType, Revision: 1, ComponentDefinition: *stored},
	})
	next, isNew, err := GenerateDefinitionRevision(ctx, cli, def)
	require.NoError(t, err)
	require.True(t, isNew)
	require.Equal(t, "db-v2", next.Name)
}

// spec.version names the revision after the version, and the name annotation after its value;
// either one that already exists is returned as it is.
func TestGenerateDefinitionRevisionNamedRevisions(t *testing.T) {
	ctx := context.Background()
	for _, k := range definitionKinds {
		t.Run(string(k.kind), func(t *testing.T) {
			def := k.build("db", "output: {}")
			k.setField(def, "1.2.0")
			rev, isNew, err := GenerateDefinitionRevision(ctx, revisionClient(t), def)
			require.NoError(t, err)
			require.True(t, isNew)
			require.Equal(t, "db-v1.2.0", rev.Name)

			existing := &v1beta1.DefinitionRevision{ObjectMeta: metav1.ObjectMeta{Name: "db-v1.2.0", Namespace: "team-a"},
				Spec: v1beta1.DefinitionRevisionSpec{Revision: 7}}
			got, isNew, err := GenerateDefinitionRevision(ctx, revisionClient(t, existing), def)
			require.NoError(t, err)
			require.False(t, isNew, "a revision is immutable")
			require.EqualValues(t, 7, got.Spec.Revision)

			k.setField(def, "not-a-version")
			_, _, err = GenerateDefinitionRevision(ctx, revisionClient(t), def)
			require.Error(t, err)
		})
	}

	def := definitionKinds[1].build("db", "output: {}")
	def.SetAnnotations(map[string]string{oam.AnnotationDefinitionRevisionName: "beta"})
	rev, isNew, err := GenerateDefinitionRevision(ctx, revisionClient(t), def)
	require.NoError(t, err)
	require.True(t, isNew)
	require.Equal(t, "db-vbeta", rev.Name)
}

func TestGatherRevisionInfoRefusesOtherTypes(t *testing.T) {
	_, _, err := GatherRevisionInfo(&v1beta1.Application{})
	require.ErrorContains(t, err, "unsupported type")
}

func TestDeepEqualDefRevisionComparesEveryKind(t *testing.T) {
	for _, k := range definitionKinds {
		a, _, err := GatherRevisionInfo(k.build("db", "output: {}"))
		require.NoError(t, err)
		b, _, err := GatherRevisionInfo(k.build("db", "output: {}"))
		require.NoError(t, err)
		c, _, err := GatherRevisionInfo(k.build("db", "output: {x: 1}"))
		require.NoError(t, err)
		require.True(t, DeepEqualDefRevision(a, b), k.kind)
		require.False(t, DeepEqualDefRevision(a, c), k.kind)
	}
}

// A revision is created with the definition's labels and its kind's name label; one that
// already exists is left as it is.
func TestCreateDefinitionRevisionLabelsTheRevision(t *testing.T) {
	ctx := context.Background()
	for _, k := range definitionKinds {
		cli := revisionClient(t)
		def := k.build("db", "output: {}")
		def.SetLabels(map[string]string{"team": "a"})
		rev, _, err := GenerateDefinitionRevision(ctx, cli, def)
		require.NoError(t, err)
		require.NoError(t, CreateDefinitionRevision(ctx, cli, def, rev.DeepCopy()))
		require.NoError(t, CreateDefinitionRevision(ctx, cli, def, rev.DeepCopy()), "already there")

		got := &v1beta1.DefinitionRevision{}
		require.NoError(t, cli.Get(ctx, client.ObjectKey{Namespace: "team-a", Name: "db-v1"}, got))
		require.Equal(t, map[string]string{"team": "a", k.label: "db"}, got.Labels, k.kind)
	}
}

// ReconcileDefinitionRevision creates a new revision and records it on the definition, and a
// failed record is reported on the definition's conditions.
func TestReconcileDefinitionRevision(t *testing.T) {
	ctx := context.Background()
	def := definitionKinds[0].build("db", "output: {}")
	cli := revisionClient(t, def)

	var recorded *common.Revision
	rev, res, err := ReconcileDefinitionRevision(ctx, cli, event.NewNopRecorder(), def, 5, func(r *common.Revision) error {
		recorded = r
		return nil
	})
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, "db-v1", rev.Name)
	require.Equal(t, &common.Revision{Name: "db-v1", Revision: 1, RevisionHash: rev.Spec.RevisionHash}, recorded)

	other := definitionKinds[0].build("cache", "output: {}")
	cli = revisionClient(t, other)
	_, res, err = ReconcileDefinitionRevision(ctx, cli, event.NewNopRecorder(), other, 5, func(*common.Revision) error {
		return errors.New("status write refused")
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	got := &v1beta1.ComponentDefinition{}
	require.NoError(t, cli.Get(ctx, client.ObjectKeyFromObject(other), got))
	require.Contains(t, got.Status.Conditions[0].Message, "status write refused")

	bad := definitionKinds[0].build("broken", "output: {}")
	definitionKinds[0].setField(bad, "not-a-version")
	cli = revisionClient(t, bad)
	_, res, err = ReconcileDefinitionRevision(ctx, cli, event.NewNopRecorder(), bad, 5, func(*common.Revision) error { return nil })
	require.NoError(t, err)
	require.NotNil(t, res, "a revision that cannot be generated is reported, not raised")
}

// Cleanup does nothing before a definition has a revision in use, or while it is within its
// limit, and reads pins only from the definition's own namespace outside the system namespace.
func TestCleanUpDefinitionRevisionLimits(t *testing.T) {
	ctx := context.Background()
	revs := func(ns, label, name string, n int) []client.Object {
		var out []client.Object
		for i := 1; i <= n; i++ {
			out = append(out, &v1beta1.DefinitionRevision{
				ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("%s-v%d", name, i), Namespace: ns, Labels: map[string]string{label: name}},
				Spec:       v1beta1.DefinitionRevisionSpec{Revision: int64(i)},
			})
		}
		return out
	}
	count := func(cli client.Client) int {
		list := &v1beta1.DefinitionRevisionList{}
		require.NoError(t, cli.List(ctx, list))
		return len(list.Items)
	}

	for _, k := range definitionKinds {
		t.Run(string(k.kind), func(t *testing.T) {
			def := k.build("db", "output: {}")
			cli := revisionClient(t, revs("team-a", k.label, "db", 4)...)
			require.NoError(t, CleanUpDefinitionRevision(ctx, cli, def, 1))
			require.Equal(t, 4, count(cli), "no revision in use yet")

			k.setLast(def, &common.Revision{Name: "db-v4", Revision: 4})
			require.NoError(t, CleanUpDefinitionRevision(ctx, cli, def, 5))
			require.Equal(t, 4, count(cli), "within the limit")

			require.NoError(t, CleanUpDefinitionRevision(ctx, cli, def, 1))
			require.Equal(t, 2, count(cli), "the one in use and the newest other")
		})
	}

	// Outside the system namespace, an Application elsewhere cannot pin the definition.
	objs := append(revs("team-a", oam.LabelTraitDefinitionName, "db", 3),
		&v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "elsewhere", Namespace: "team-b"},
			Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{{Name: "c", Type: "x",
				Traits: []common.ApplicationTrait{{Type: "db@v1"}}}}}},
		&v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "here", Namespace: "team-a"},
			Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{{Name: "c", Type: "x",
				Traits: []common.ApplicationTrait{{Type: "db@v2"}}}}}})
	cli := revisionClient(t, objs...)
	def := definitionKinds[1].build("db", "output: {}")
	definitionKinds[1].setLast(def, &common.Revision{Name: "db-v3", Revision: 3})
	require.NoError(t, CleanUpDefinitionRevision(ctx, cli, def, 0))
	list := &v1beta1.DefinitionRevisionList{}
	require.NoError(t, cli.List(ctx, list))
	var names []string
	for _, r := range list.Items {
		names = append(names, r.Name)
	}
	require.ElementsMatch(t, []string{"db-v2", "db-v3"}, names)
}

// pinnedTypes reads pins from where each kind is named: traits, policies, and workflow steps
// including sub-steps.
func TestPinnedTypesReadsEveryKind(t *testing.T) {
	app := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{
		Components: []common.ApplicationComponent{{Name: "c", Type: "web@v1",
			Traits: []common.ApplicationTrait{{Type: "scaler@v2"}, {Type: "plain"}}}},
		Policies: []v1beta1.AppPolicy{{Name: "p", Type: "topology@v3"}},
		Workflow: &v1beta1.Workflow{Steps: []workflowv1alpha1.WorkflowStep{{
			WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{Name: "s", Type: "deploy@v4"},
			SubSteps:         []workflowv1alpha1.WorkflowStepBase{{Name: "sub", Type: "notify@v5"}},
		}}},
	}}
	require.Equal(t, []string{"web@v1"}, pinnedTypes(app, common.ComponentType))
	require.Equal(t, []string{"scaler@v2"}, pinnedTypes(app, common.TraitType))
	require.Equal(t, []string{"topology@v3"}, pinnedTypes(app, common.PolicyType))
	require.Equal(t, []string{"deploy@v4", "notify@v5"}, pinnedTypes(app, common.WorkflowStepType))
}
