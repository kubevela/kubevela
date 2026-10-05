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

package resourcekeeper

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apicommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func managed(component, name string, deleted bool) v1beta1.ManagedResource {
	return v1beta1.ManagedResource{
		ClusterObjectReference: apicommon.ClusterObjectReference{
			ObjectReference: corev1.ObjectReference{APIVersion: "v1", Kind: "ConfigMap", Name: name, Namespace: "ns"},
		},
		OAMObjectReference: apicommon.OAMObjectReference{Component: component},
		Deleted:            deleted,
	}
}

// A component's applied resources, from both trackers that hold live ones, and
// never an entry already marked for deletion.
func TestComponentResources(t *testing.T) {
	h := &resourceKeeper{
		_currentRT: &v1beta1.ResourceTracker{Spec: v1beta1.ResourceTrackerSpec{ManagedResources: []v1beta1.ManagedResource{
			managed("db", "db-workload", false), managed("api", "api-workload", false), managed("db", "db-old", true),
		}}},
		_rootRT: &v1beta1.ResourceTracker{Spec: v1beta1.ResourceTrackerSpec{ManagedResources: []v1beta1.ManagedResource{
			managed("db", "db-shared", false),
		}}},
	}
	var names []string
	for _, mr := range h.ComponentResources("db") {
		names = append(names, mr.Name)
	}
	require.ElementsMatch(t, []string{"db-workload", "db-shared"}, names)

	require.Empty(t, (&resourceKeeper{}).ComponentResources("db"), "no tracker yet is nothing applied")
}

// A deploy step records into the tracker from parallel tasks while readers ask
// for a producer's resources; run with -race.
func TestComponentResourcesIsSafeAlongsideRecording(t *testing.T) {
	h := &resourceKeeper{_currentRT: &v1beta1.ResourceTracker{}}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			h.mu.Lock()
			h._currentRT.Spec.ManagedResources = append(h._currentRT.Spec.ManagedResources, managed("db", "db", false))
			h.mu.Unlock()
		}
	}()
	for i := 0; i < 200; i++ {
		_ = h.ComponentResources("db")
	}
	<-done
	require.Len(t, h.ComponentResources("db"), 200)
}

// Component reads find a producer's objects through ComponentResources, so what
// the keeper records when it dispatches has to carry what a read looks them up
// by: component, trait, placement and object reference. Recorded here through
// Dispatch, as the controller records it, rather than built by hand.
func TestComponentResourcesRecordsWhatReadsNeed(t *testing.T) {
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "shop", Generation: 1}}
	keeper, err := NewResourceKeeper(ctx, cli, app)
	require.NoError(t, err)

	object := func(kind, name string, labels map[string]string) *unstructured.Unstructured {
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind(kind))
		u.SetName(name)
		u.SetNamespace("shop")
		u.SetLabels(labels)
		return u
	}
	workload := object("ConfigMap", "db", map[string]string{oam.LabelAppComponent: "db"})
	svc := object("Service", "db-svc", map[string]string{
		oam.LabelAppComponent: "db", oam.TraitTypeLabel: "expose", oam.TraitResource: "svc"})
	require.NoError(t, keeper.Dispatch(ctx, []*unstructured.Unstructured{workload, svc}, nil))

	got := map[string]v1beta1.ManagedResource{}
	for _, mr := range keeper.(*resourceKeeper).ComponentResources("db") {
		got[mr.Name] = mr
	}
	require.Len(t, got, 2)
	for name, want := range map[string]struct{ kind, trait string }{"db": {"ConfigMap", ""}, "db-svc": {"Service", "expose"}} {
		mr := got[name]
		require.Equal(t, "db", mr.Component, name)
		require.Equal(t, want.trait, mr.Trait, name)
		require.Equal(t, "v1", mr.APIVersion, name)
		require.Equal(t, want.kind, mr.Kind, name)
		require.Equal(t, "shop", mr.Namespace, name)
		require.Contains(t, []string{"", "local"}, mr.Cluster, name)
		require.False(t, mr.Deleted, name)
	}

	// A read tells a trait's resource from the workload by this label, on the live object.
	live := &unstructured.Unstructured{}
	live.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Service"))
	require.NoError(t, cli.Get(ctx, client.ObjectKey{Namespace: "shop", Name: "db-svc"}, live))
	require.Equal(t, "svc", live.GetLabels()[oam.TraitResource])
}
