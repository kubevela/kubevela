/*
Copyright 2022 The KubeVela Authors.

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
	"errors"
	"testing"

	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// newUnstructured is a helper to create a simple unstructured object for testing.
func newUnstructured(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name": name,
			},
		},
	}
}

func TestContainsResources(t *testing.T) {
	res1 := newUnstructured("res-1")
	res2 := newUnstructured("res-2")
	res3 := newUnstructured("res-3")
	res4Missing := newUnstructured("res-4-missing")

	baseKeeperSetup := func() *resourceKeeper {
		return &resourceKeeper{
			Client:     fake.NewClientBuilder().WithScheme(common.Scheme).Build(),
			_currentRT: &v1beta1.ResourceTracker{},
			_rootRT:    &v1beta1.ResourceTracker{},
		}
	}

	testCases := []struct {
		name     string
		keeper   *resourceKeeper
		input    []*unstructured.Unstructured
		expected bool
	}{
		{
			name: "all resources exist across both trackers",
			keeper: func() *resourceKeeper {
				k := baseKeeperSetup()
				k._currentRT.AddManagedResource(res1, true, false, "")
				k._rootRT.AddManagedResource(res2, true, false, "")
				k._rootRT.AddManagedResource(res3, true, false, "")
				return k
			}(),
			input:    []*unstructured.Unstructured{res1, res2, res3},
			expected: true,
		},
		{
			name: "one resource exists in currentRT",
			keeper: func() *resourceKeeper {
				k := baseKeeperSetup()
				k._currentRT.AddManagedResource(res1, true, false, "")
				return k
			}(),
			input:    []*unstructured.Unstructured{res1},
			expected: true,
		},
		{
			name: "one resource exists in rootRT",
			keeper: func() *resourceKeeper {
				k := baseKeeperSetup()
				k._rootRT.AddManagedResource(res2, true, false, "")
				return k
			}(),
			input:    []*unstructured.Unstructured{res2},
			expected: true,
		},
		{
			name: "one resource is missing",
			keeper: func() *resourceKeeper {
				k := baseKeeperSetup()
				k._currentRT.AddManagedResource(res1, true, false, "")
				return k
			}(),
			input:    []*unstructured.Unstructured{res1, res4Missing},
			expected: false,
		},
		{
			name: "empty input slice should return true",
			keeper: &resourceKeeper{
				Client: fake.NewClientBuilder().WithScheme(common.Scheme).Build(),
			},
			input:    []*unstructured.Unstructured{},
			expected: true,
		},
		{
			name: "trackers are nil",
			keeper: &resourceKeeper{
				Client: fake.NewClientBuilder().WithScheme(common.Scheme).Build(),
			},
			input:    []*unstructured.Unstructured{res1},
			expected: false,
		},
		{
			name: "only rootRT is nil, resource in currentRT",
			keeper: func() *resourceKeeper {
				k := &resourceKeeper{
					Client:     fake.NewClientBuilder().WithScheme(common.Scheme).Build(),
					_currentRT: &v1beta1.ResourceTracker{},
				}
				k._currentRT.AddManagedResource(res1, true, false, "")
				return k
			}(),
			input:    []*unstructured.Unstructured{res1},
			expected: true,
		},
		{
			name: "only rootRT is nil, resource not in currentRT",
			keeper: func() *resourceKeeper {
				k := &resourceKeeper{
					Client:     fake.NewClientBuilder().WithScheme(common.Scheme).Build(),
					_currentRT: &v1beta1.ResourceTracker{},
				}
				k._currentRT.AddManagedResource(res1, true, false, "")
				return k
			}(),
			input:    []*unstructured.Unstructured{res2},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)
			result := tc.keeper.ContainsResources(tc.input)
			r.Equal(tc.expected, result)
		})
	}
}

func TestManifestMetadataChanged(t *testing.T) {
	withMeta := func(labels, annotations map[string]string) *unstructured.Unstructured {
		u := newUnstructured("cm")
		u.SetLabels(labels)
		u.SetAnnotations(annotations)
		return u
	}
	// recordRaw records rsc the way a tracker read back from the API holds it.
	recordRaw := func(t *testing.T, rt *v1beta1.ResourceTracker, rsc *unstructured.Unstructured) {
		rt.AddManagedResource(rsc, false, false, "")
		raw, err := rsc.MarshalJSON()
		require.NoError(t, err)
		rt.Spec.ManagedResources[len(rt.Spec.ManagedResources)-1].Data = &runtime.RawExtension{Raw: raw}
	}
	v1 := map[string]string{"gwcp/tag": "v1"}
	v2 := map[string]string{"gwcp/tag": "v2"}

	testCases := []struct {
		name     string
		record   func(t *testing.T, rt *v1beta1.ResourceTracker)
		desired  *unstructured.Unstructured
		expected bool
	}{
		{
			name:     "resource not recorded",
			record:   func(*testing.T, *v1beta1.ResourceTracker) {},
			desired:  withMeta(v2, nil),
			expected: false,
		},
		{
			name: "recorded without manifest data (meta only)",
			record: func(_ *testing.T, rt *v1beta1.ResourceTracker) {
				rt.AddManagedResource(withMeta(v1, nil), true, false, "")
			},
			desired:  withMeta(v2, nil),
			expected: false,
		},
		{
			name:     "same labels and annotations",
			record:   func(t *testing.T, rt *v1beta1.ResourceTracker) { recordRaw(t, rt, withMeta(v1, v1)) },
			desired:  withMeta(v1, v1),
			expected: false,
		},
		{
			name:     "label changed",
			record:   func(t *testing.T, rt *v1beta1.ResourceTracker) { recordRaw(t, rt, withMeta(v1, nil)) },
			desired:  withMeta(v2, nil),
			expected: true,
		},
		{
			name:     "annotation changed",
			record:   func(t *testing.T, rt *v1beta1.ResourceTracker) { recordRaw(t, rt, withMeta(nil, v1)) },
			desired:  withMeta(nil, v2),
			expected: true,
		},
		{
			name: "keys stamped at dispatch are ignored",
			record: func(t *testing.T, rt *v1beta1.ResourceTracker) {
				recordRaw(t, rt, withMeta(map[string]string{"gwcp/tag": "v1", apply.LabelRenderHash: "abc", "owner.oam.dev/name": "app"},
					map[string]string{"stamped": "x"}))
			},
			desired:  withMeta(v1, nil),
			expected: false,
		},
		{
			name:     "rendered label missing from the recorded manifest",
			record:   func(t *testing.T, rt *v1beta1.ResourceTracker) { recordRaw(t, rt, withMeta(nil, nil)) },
			desired:  withMeta(v1, nil),
			expected: true,
		},
		{
			name: "label changed against a manifest recorded in this reconcile",
			record: func(_ *testing.T, rt *v1beta1.ResourceTracker) {
				rt.AddManagedResource(withMeta(v1, nil), false, false, "")
			},
			desired:  withMeta(v2, nil),
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			k := &resourceKeeper{
				Client:     fake.NewClientBuilder().WithScheme(common.Scheme).Build(),
				_currentRT: &v1beta1.ResourceTracker{},
				_rootRT:    &v1beta1.ResourceTracker{},
			}
			tc.record(t, k._currentRT)
			require.Equal(t, tc.expected, k.ManifestMetadataChanged([]*unstructured.Unstructured{tc.desired}))
		})
	}
}

func TestRefreshRecordedMetadata(t *testing.T) {
	ctx := context.Background()
	manifest := func(name string, labels, annotations map[string]string) *unstructured.Unstructured {
		u := newUnstructured(name)
		u.SetNamespace("ns")
		u.SetLabels(labels)
		u.SetAnnotations(annotations)
		return u
	}
	component := manifest("from-component", map[string]string{"gwcp/tag": "v1", "author": "x"}, map[string]string{"gwcp/note": "a"})
	component.SetLabels(map[string]string{"gwcp/tag": "v1", "author": "x", oam.LabelAppComponent: "comp"})
	notComponent := manifest("from-step", map[string]string{"gwcp/tag": "v1"}, nil)
	metaOnly := manifest("meta-only", map[string]string{"gwcp/tag": "v1", oam.LabelAppComponent: "comp"}, nil)

	rt := &v1beta1.ResourceTracker{ObjectMeta: metav1.ObjectMeta{Name: "app-v1-ns"}}
	rt.AddManagedResource(component, false, false, "")
	rt.AddManagedResource(notComponent, false, false, "")
	rt.AddManagedResource(metaOnly, true, false, "")
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(rt).Build()

	// The keeper holds the tracker as read back from the API: manifests are raw JSON.
	loaded := &v1beta1.ResourceTracker{}
	require.NoError(t, cli.Get(ctx, client.ObjectKeyFromObject(rt), loaded))
	k := &resourceKeeper{Client: cli, _currentRT: loaded}

	changed, err := k.RefreshRecordedMetadata(ctx,
		map[string]string{"gwcp/tag": "v2", "gwcp/new": "y"}, map[string]string{"gwcp/note": "b"}, MetadataRemoval{})
	require.NoError(t, err)
	require.True(t, changed)

	stored := &v1beta1.ResourceTracker{}
	require.NoError(t, cli.Get(ctx, client.ObjectKeyFromObject(rt), stored))
	recorded := map[string]*unstructured.Unstructured{}
	for i := range stored.Spec.ManagedResources {
		mr := stored.Spec.ManagedResources[i]
		obj, err := mr.ToUnstructuredWithData()
		if err != nil {
			obj = nil
		}
		recorded[mr.Name] = obj
	}
	require.Equal(t, map[string]string{"gwcp/tag": "v2", "author": "x", oam.LabelAppComponent: "comp"}, recorded["from-component"].GetLabels(),
		"only keys the manifest already carries are updated")
	require.Equal(t, map[string]string{"gwcp/note": "b"}, recorded["from-component"].GetAnnotations())
	require.Equal(t, "v1", recorded["from-step"].GetLabels()["gwcp/tag"], "resources not rendered from a component are left alone")
	require.Nil(t, recorded["meta-only"], "records without a manifest stay without one")

	changed, err = k.RefreshRecordedMetadata(ctx, map[string]string{"gwcp/tag": "v2"}, map[string]string{"gwcp/note": "b"}, MetadataRemoval{})
	require.NoError(t, err)
	require.False(t, changed, "nothing to change the second time")
}

func TestPolicyContextHash(t *testing.T) {
	ctx := context.Background()
	rt := &v1beta1.ResourceTracker{ObjectMeta: metav1.ObjectMeta{Name: "app-v1-ns"}}
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(rt).Build()
	loaded := &v1beta1.ResourceTracker{}
	require.NoError(t, cli.Get(ctx, client.ObjectKeyFromObject(rt), loaded))
	k := &resourceKeeper{Client: cli, _currentRT: loaded}

	_, recorded := k.PolicyContextHash()
	require.False(t, recorded, "a tracker without the annotation has no recorded hash")

	require.NoError(t, k.SetPolicyContextHash(ctx, "abc"))
	hash, recorded := k.PolicyContextHash()
	require.True(t, recorded)
	require.Equal(t, "abc", hash)

	stored := &v1beta1.ResourceTracker{}
	require.NoError(t, cli.Get(ctx, client.ObjectKeyFromObject(rt), stored))
	require.Equal(t, "abc", stored.GetAnnotations()[oam.AnnotationPolicyContextHash])

	empty := &resourceKeeper{Client: cli}
	require.NoError(t, empty.SetPolicyContextHash(ctx, "abc"), "no current tracker is a no-op")
}

func TestSetPolicyContextHashPatchesMetadataOnly(t *testing.T) {
	ctx := context.Background()
	rt := &v1beta1.ResourceTracker{ObjectMeta: metav1.ObjectMeta{Name: "app-v1-ns"}}
	var updates, patches int
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(rt).WithInterceptorFuncs(interceptor.Funcs{
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			updates++
			return c.Update(ctx, obj, opts...)
		},
		Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			patches++
			return c.Patch(ctx, obj, patch, opts...)
		},
	}).Build()
	loaded := &v1beta1.ResourceTracker{}
	require.NoError(t, cli.Get(ctx, client.ObjectKeyFromObject(rt), loaded))
	k := &resourceKeeper{Client: cli, _currentRT: loaded}

	require.NoError(t, k.SetPolicyContextHash(ctx, "abc"))
	require.Equal(t, 0, updates, "the tracker's manifests are not rewritten to set one annotation")
	require.Equal(t, 1, patches)

	stored := &v1beta1.ResourceTracker{}
	require.NoError(t, cli.Get(ctx, client.ObjectKeyFromObject(rt), stored))
	require.Equal(t, "abc", stored.GetAnnotations()[oam.AnnotationPolicyContextHash])

	require.NoError(t, k.SetPolicyContextHash(ctx, "abc"))
	require.Equal(t, 1, patches, "an unchanged hash writes nothing")
}

func TestMetadataRemoval(t *testing.T) {
	ctx := context.Background()
	component := newUnstructured("from-component")
	component.SetNamespace("ns")
	component.SetLabels(map[string]string{"gwcp/dropped": "v1", "gwcp/edited": "mine", oam.LabelAppComponent: "comp"})
	component.SetAnnotations(map[string]string{"gwcp/note": "a"})
	notComponent := newUnstructured("from-step")
	notComponent.SetNamespace("ns")
	notComponent.SetLabels(map[string]string{"gwcp/dropped": "v1"})

	rt := &v1beta1.ResourceTracker{ObjectMeta: metav1.ObjectMeta{Name: "app-v1-ns"}}
	rt.AddManagedResource(component, false, false, "")
	rt.AddManagedResource(notComponent, false, false, "")
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(rt).Build()
	loaded := &v1beta1.ResourceTracker{}
	require.NoError(t, cli.Get(ctx, client.ObjectKeyFromObject(rt), loaded))
	k := &resourceKeeper{Client: cli, _currentRT: loaded}

	removal := MetadataRemoval{
		// gwcp/edited is at another value than the one to remove, so it stays.
		Labels:      map[string]string{"gwcp/dropped": "v1", "gwcp/edited": "v1"},
		Annotations: map[string]string{"gwcp/note": "a"},
	}
	changed, err := k.RefreshRecordedMetadata(ctx, nil, nil, removal)
	require.NoError(t, err)
	require.True(t, changed)
	stored := &v1beta1.ResourceTracker{}
	require.NoError(t, cli.Get(ctx, client.ObjectKeyFromObject(rt), stored))
	recorded := map[string]*unstructured.Unstructured{}
	for i := range stored.Spec.ManagedResources {
		obj, err := stored.Spec.ManagedResources[i].ToUnstructuredWithData()
		require.NoError(t, err)
		recorded[obj.GetName()] = obj
	}
	require.Equal(t, map[string]string{"gwcp/edited": "mine", oam.LabelAppComponent: "comp"}, recorded["from-component"].GetLabels())
	require.Empty(t, recorded["from-component"].GetAnnotations())
	require.Equal(t, "v1", recorded["from-step"].GetLabels()["gwcp/dropped"], "resources not rendered from a component are left alone")
}

func TestTrackerWritesGoToTheHub(t *testing.T) {
	member := pkgmulticluster.WithCluster(context.Background(), "member")
	component := newUnstructured("cm")
	component.SetNamespace("ns")
	component.SetLabels(map[string]string{"gwcp/tag": "v1", oam.LabelAppComponent: "comp"})
	rt := &v1beta1.ResourceTracker{ObjectMeta: metav1.ObjectMeta{Name: "app-v1-ns"}}
	rt.AddManagedResource(component, false, false, "")
	var clusters []string
	onHub := func(ctx context.Context) {
		cluster, _ := pkgmulticluster.ClusterFrom(ctx)
		clusters = append(clusters, cluster)
	}
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(rt).WithInterceptorFuncs(interceptor.Funcs{
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			onHub(ctx)
			return c.Update(ctx, obj, opts...)
		},
		Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			onHub(ctx)
			return c.Patch(ctx, obj, patch, opts...)
		},
	}).Build()
	loaded := &v1beta1.ResourceTracker{}
	require.NoError(t, cli.Get(context.Background(), client.ObjectKeyFromObject(rt), loaded))
	k := &resourceKeeper{Client: cli, _currentRT: loaded}

	_, err := k.RefreshRecordedMetadata(member, map[string]string{"gwcp/tag": "v2"}, nil, MetadataRemoval{})
	require.NoError(t, err)
	require.NoError(t, k.SetPolicyContextHash(member, "abc"))
	require.Equal(t, []string{pkgmulticluster.Local, pkgmulticluster.Local}, clusters,
		"trackers live on the hub, whichever cluster the caller is working in")
}

func TestSetPolicyContextHashKeepsNothingAfterAFailedPatch(t *testing.T) {
	ctx := context.Background()
	rt := &v1beta1.ResourceTracker{ObjectMeta: metav1.ObjectMeta{Name: "app-v1-ns"}}
	fail := true
	patches := 0
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(rt).WithInterceptorFuncs(interceptor.Funcs{
		Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			patches++
			if fail {
				return errors.New("conflict")
			}
			return c.Patch(ctx, obj, patch, opts...)
		},
	}).Build()
	loaded := &v1beta1.ResourceTracker{}
	require.NoError(t, cli.Get(ctx, client.ObjectKeyFromObject(rt), loaded))
	k := &resourceKeeper{Client: cli, _currentRT: loaded}

	require.Error(t, k.SetPolicyContextHash(ctx, "abc"))
	_, recorded := k._currentRT.GetAnnotations()[oam.AnnotationPolicyContextHash]
	require.False(t, recorded, "a failed patch leaves the tracker as it was")
	fail = false
	require.NoError(t, k.SetPolicyContextHash(ctx, "abc"))
	require.Equal(t, 2, patches, "the retry writes the hash")
}
