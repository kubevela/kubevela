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

package resourcetracker

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func TestBaseTrackerNamesAndLabels(t *testing.T) {
	base := NewBase(Owner{Kind: "Component", Namespace: "team-a", Name: "shop-webservice-backend", UID: "uid-1", Generation: 3, Deleting: false})

	root, err := base.NewTracker(v1beta1.ResourceTrackerTypeRoot)
	require.NoError(t, err)
	require.Equal(t, "team-a-shop-webservice-backend", root.Name)

	current, err := base.NewTracker(v1beta1.ResourceTrackerTypeVersioned)
	require.NoError(t, err)
	require.Equal(t, "team-a-shop-webservice-backend-v3", current.Name)
	require.Equal(t, int64(3), current.Spec.ApplicationGeneration)
	require.Empty(t, current.Labels, "NewTracker names and types a tracker; CreateTracker labels it")
	require.Equal(t, map[string]string{
		oam.LabelOwnerKind: "Component", oam.LabelOwnerName: "shop-webservice-backend",
		oam.LabelOwnerNamespace: "team-a", oam.LabelOwnerUID: "uid-1",
	}, base.TrackerLabels())

	_, err = base.NewTracker(v1beta1.ResourceTrackerTypeComponentRevision)
	require.Error(t, err, "component-revision trackers belong to the kind, not the base")
}

func TestBaseKeyAndOwnership(t *testing.T) {
	base := NewBase(Owner{Kind: "Component", Namespace: "team-a", Name: "backend", UID: "uid-1", Generation: 1, Deleting: false})
	require.Equal(t, "Component/team-a/backend", base.Key())

	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
	require.Equal(t, "", base.ControlledBy(obj), "an unlabelled object is controlled by nobody")
	base.Stamp(obj)
	require.Equal(t, base.Key(), base.ControlledBy(obj), "a stamped object is controlled by its owner")
}

// An owner recreated with the same name and namespace is a different owner: its trackers
// carry the dead incarnation's uid, and adopting them would hand it the old owner's
// resources to manage and collect.
func TestBaseRefusesTrackersOfAnotherIncarnation(t *testing.T) {
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(testScheme).Build()
	dead := NewBase(Owner{Kind: "Component", Namespace: "team-a", Name: "backend", UID: "uid-dead", Generation: 1, Deleting: false})
	rt, err := CreateTracker(ctx, cli, dead, v1beta1.ResourceTrackerTypeVersioned)
	require.NoError(t, err)

	recreated := NewBase(Owner{Kind: "Component", Namespace: "team-a", Name: "backend", UID: "uid-new", Generation: 1, Deleting: false})
	_, err = recreated.LoadTrackers(ctx, cli)
	require.ErrorContains(t, err, "controlled by another Component", "the same name is not the same owner")
	require.ErrorContains(t, err, "uid-dead")

	trackers, err := dead.LoadTrackers(ctx, cli)
	require.NoError(t, err, "its own owner still loads it")
	require.Equal(t, rt.Name, trackers.Current.Name)
}

// labelledKind is a kind that carries its own labels besides owner.oam.dev/*, as a Component
// does, by overriding TrackerLabels.
type labelledKind struct{ *Base }

func (k labelledKind) TrackerLabels() map[string]string {
	labels := k.Base.TrackerLabels()
	labels["component.oam.dev/type"] = "webservice"
	return labels
}

// A kind's own labels must reach the trackers it creates. Embedding Base gives no virtual
// dispatch, so whatever builds the tracker has to ask the kind, not the Base.
func TestATrackerCarriesItsKindsOwnLabels(t *testing.T) {
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(testScheme).Build()
	kind := labelledKind{Base: NewBase(Owner{Kind: "Component", Namespace: "team-a", Name: "backend", UID: "uid-1", Generation: 1, Deleting: false})}

	rt, err := CreateTracker(ctx, cli, kind, v1beta1.ResourceTrackerTypeVersioned)
	require.NoError(t, err)
	require.Equal(t, "webservice", rt.Labels["component.oam.dev/type"], "the kind's labels reach its tracker")
	require.Equal(t, "Component", rt.Labels[oam.LabelOwnerKind])

	live := &v1beta1.ResourceTracker{}
	require.NoError(t, cli.Get(ctx, client.ObjectKey{Name: rt.Name}, live))
	require.Equal(t, "webservice", live.Labels["component.oam.dev/type"], "and are stored, not only set in memory")
}
