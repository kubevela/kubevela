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
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func componentResource(name, component string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(v1.SchemeGroupVersion.WithKind("ConfigMap"))
	u.SetName(name)
	u.SetNamespace("default")
	u.SetLabels(map[string]string{oam.LabelAppComponent: component})
	return u
}

func exists(t *testing.T, cli client.Client, name string) bool {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(v1.SchemeGroupVersion.WithKind("ConfigMap"))
	err := cli.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: name}, u)
	if apierrors.IsNotFound(err) {
		return false
	}
	require.NoError(t, err)
	return true
}

// A source-driven rename re-dispatches a component under a new resource name
// without bumping the Application revision, because that is the whole point of
// refreshing from a source. GC is ResourceTracker-level, so nothing marks the
// resource the component no longer renders, and it lingers.
//
// This is not specific to names: any shrink in the set a component renders leaks
// the same way. Renames just make it visible.
func TestRenamedResourceIsNotLeaked(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()

	app := &v1beta1.Application{
		ObjectMeta: v12.ObjectMeta{Name: "app", Namespace: "default", Generation: 1},
	}
	_rk, err := NewResourceKeeper(ctx, cli, app)
	r.NoError(err)
	rk := _rk.(*resourceKeeper)

	// The workflow dispatches the component, rendering "web-clusterA".
	r.NoError(rk.Dispatch(ctx, []*unstructured.Unstructured{componentResource("web-clusterA", "web")}, nil))
	r.True(exists(t, cli, "web-clusterA"))

	// A source value changes, so the refresh re-renders the same component under
	// a different name and dispatches it. No new revision, by design.
	r.NoError(rk.Dispatch(ctx, []*unstructured.Unstructured{componentResource("web-clusterB", "web")}, nil))
	r.True(exists(t, cli, "web-clusterB"))

	// GC alone does not help: it works on whole ResourceTrackers, and the current
	// tracker is only collected when the Application is deleted.
	_, _, err = rk.GarbageCollect(ctx)
	r.NoError(err)
	r.True(exists(t, cli, "web-clusterA"), "GC is tracker-level, so it cannot see this")

	// Pruning against the component's complete rendered set is what reaps it.
	pruned, err := rk.PruneComponentResources(ctx, "web",
		[]*unstructured.Unstructured{componentResource("web-clusterB", "web")})
	r.NoError(err)
	r.Len(pruned, 1)
	r.Equal("web-clusterA", pruned[0].Name)

	rt, err := rk.getCurrentRT(ctx)
	r.NoError(err)
	var tracked []string
	for _, mr := range rt.Spec.ManagedResources {
		if !mr.Deleted {
			tracked = append(tracked, mr.Name)
		}
	}
	r.ElementsMatch([]string{"web-clusterB"}, tracked,
		"the component no longer renders web-clusterA, so it must not stay tracked")
	r.False(exists(t, cli, "web-clusterA"), "web-clusterA must be reaped")
}

// Pruning is attributed by component, so it must not touch a sibling's
// resources - the refresh path walks components one at a time.
func TestPruneLeavesOtherComponentsAlone(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	_rk, err := NewResourceKeeper(ctx, cli, &v1beta1.Application{
		ObjectMeta: v12.ObjectMeta{Name: "app", Namespace: "default", Generation: 1},
	})
	r.NoError(err)
	rk := _rk.(*resourceKeeper)

	r.NoError(rk.Dispatch(ctx, []*unstructured.Unstructured{
		componentResource("web-old", "web"),
		componentResource("db-keep", "db"),
	}, nil))

	pruned, err := rk.PruneComponentResources(ctx, "web", nil)
	r.NoError(err)
	r.Len(pruned, 1)
	r.Equal("web-old", pruned[0].Name)
	r.False(exists(t, cli, "web-old"))
	r.True(exists(t, cli, "db-keep"), "another component's resource must survive")
}

// A resource the user asked to keep must survive pruning, or the prune becomes
// a way to bypass the garbage-collect policy.
func TestPruneRespectsGarbageCollectPolicy(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	_rk, err := NewResourceKeeper(ctx, cli, &v1beta1.Application{
		ObjectMeta: v12.ObjectMeta{Name: "app", Namespace: "default", Generation: 1},
	})
	r.NoError(err)
	rk := _rk.(*resourceKeeper)
	rk.garbageCollectPolicy = &v1alpha1.GarbageCollectPolicySpec{
		Rules: []v1alpha1.GarbageCollectPolicyRule{{
			Selector: v1alpha1.ResourcePolicyRuleSelector{ResourceNames: []string{"web-eternal"}},
			Strategy: v1alpha1.GarbageCollectStrategyNever,
		}},
	}

	r.NoError(rk.Dispatch(ctx, []*unstructured.Unstructured{componentResource("web-eternal", "web")}, nil))
	_, err = rk.PruneComponentResources(ctx, "web", nil)
	r.NoError(err)
	r.True(exists(t, cli, "web-eternal"), "a never-collect resource must survive a prune")
}

// A pruned resource has to be deleted in the cluster it actually lives in, and
// the tracker entry has to be marked there too.
//
// Both hang off the same fact: ManagedResource records a Cluster, and
// ClusterObjectReference.Equal compares it. A manifest synthesised without it
// carries Cluster "", so multicluster routing sends the DELETE to the hub and
// findMangedResourceIndex never matches the entry it was meant to retire.
//
// The hub-routing half is the dangerous one. A component placed on both the hub
// and a member, whose member variant goes stale, would have its live hub object
// deleted instead.
func TestPrunedResourceKeepsItsCluster(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()

	app := &v1beta1.Application{
		ObjectMeta: v12.ObjectMeta{Name: "app", Namespace: "default", Generation: 1},
	}
	_rk, err := NewResourceKeeper(ctx, cli, app)
	r.NoError(err)
	rk := _rk.(*resourceKeeper)

	// The component renders one resource into a member cluster.
	remote := componentResource("web-remote", "web")
	oam.SetCluster(remote, "cluster-1")
	r.NoError(rk.Dispatch(ctx, []*unstructured.Unstructured{remote}, nil))

	rt, err := rk.getCurrentRT(ctx)
	r.NoError(err)
	r.Len(rt.Spec.ManagedResources, 1)
	r.Equal("cluster-1", rt.Spec.ManagedResources[0].Cluster,
		"the tracker records where the resource was placed")

	// A source change re-renders the component with nothing at all, so the
	// remote resource is stale and must be pruned.
	pruned, err := rk.PruneComponentResources(ctx, "web", nil)
	r.NoError(err)
	r.Len(pruned, 1)
	r.Equal("cluster-1", pruned[0].Cluster)

	// The tracker entry must be marked deleted. It only can be if the manifest
	// carried the cluster, because Equal compares it.
	rt, err = rk.getCurrentRT(ctx)
	r.NoError(err)
	for _, mr := range rt.Spec.ManagedResources {
		if mr.Name == "web-remote" {
			r.True(mr.Deleted,
				"the entry is still tracked, so the manifest did not match it: "+
					"the synthesised manifest lost mr.Cluster")
		}
	}
}
