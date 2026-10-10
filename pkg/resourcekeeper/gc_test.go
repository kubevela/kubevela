/*
Copyright 2021 The KubeVela Authors.

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
	"fmt"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestEnableMarkStageGCOnWorkflowFailure(t *testing.T) {
	h := &resourceKeeper{policies: Policies{GarbageCollect: &v1alpha1.GarbageCollectPolicySpec{ContinueOnFailure: true}}}
	options := []GCOption{DisableMarkStageGCOption{}}
	cfg := h.buildGCConfig(context.Background(), options...)
	require.True(t, cfg.disableMark)
	cfg = h.buildGCConfig(WithFailedRun(context.Background(), true), options...)
	require.False(t, cfg.disableMark)
}

func TestResourceKeeperGarbageCollectWithoutCurrentRT(t *testing.T) {
	MarkWithProbability = 1.0
	r := require.New(t)
	ctx := context.Background()

	// app reconciled spec at generation 1, then a rapid A -> B -> A spec flip
	// advanced the generation to 3 without a workflow restart, so no RT was
	// created for generation 3 and the generation-1 RT became history while
	// still tracking the live resources of the current revision.
	setup := func() client.Client {
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
		rt := &v1beta1.ResourceTracker{
			ObjectMeta: metav1.ObjectMeta{Name: "app-v1", Labels: map[string]string{
				oam.LabelAppName:      "app",
				oam.LabelAppNamespace: "default",
				oam.LabelAppUID:       "uid",
				oam.LabelAppRevision:  "app-v1",
			}, Finalizers: []string{resourcetracker.Finalizer}},
			Spec: v1beta1.ResourceTrackerSpec{
				Type:                  v1beta1.ResourceTrackerTypeVersioned,
				ApplicationGeneration: 1,
			},
		}
		r.NoError(cli.Create(ctx, rt))
		cm := &unstructured.Unstructured{}
		cm.SetName("cm-1")
		cm.SetNamespace("default")
		cm.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
		cm.SetLabels(map[string]string{
			oam.LabelAppName:      "app",
			oam.LabelAppNamespace: "default",
		})
		r.NoError(cli.Create(ctx, cm))
		r.NoError(resourcetracker.RecordManifestsInResourceTracker(ctx, cli, rt, []*unstructured.Unstructured{cm}, true, false, ""))
		return cli
	}

	gc := func(cli client.Client, latestRevision string) (bool, error) {
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default", UID: "uid", Generation: 3},
		}
		if latestRevision != "" {
			app.Status.LatestRevision = &apicommon.Revision{Name: latestRevision}
		}
		_rk, err := NewResourceKeeper(ctx, cli, app)
		r.NoError(err)
		finished, _, err := _rk.(*resourceKeeper).GarbageCollect(ctx)
		return finished, err
	}
	rtExists := func(cli client.Client) (*v1beta1.ResourceTracker, bool) {
		rt := &v1beta1.ResourceTracker{}
		err := cli.Get(ctx, client.ObjectKey{Namespace: "", Name: "app-v1"}, rt)
		return rt, err == nil
	}
	cmExists := func(cli client.Client) bool {
		cm := &unstructured.Unstructured{}
		cm.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
		return cli.Get(ctx, client.ObjectKey{Namespace: "default", Name: "cm-1"}, cm) == nil
	}

	// the history RT of the current revision is kept along with its resources
	cli := setup()
	finished, err := gc(cli, "app-v1")
	r.NoError(err)
	r.True(finished)
	rt, ok := rtExists(cli)
	r.True(ok)
	r.Nil(rt.GetDeletionTimestamp())
	r.True(cmExists(cli))
	finished, err = gc(cli, "app-v1")
	r.NoError(err)
	r.True(finished)
	_, ok = rtExists(cli)
	r.True(ok)
	r.True(cmExists(cli))

	// a history RT of an outdated revision is still collected when no
	// current RT exists (e.g. all components removed from the spec)
	cli = setup()
	finished, err = gc(cli, "app-v2")
	r.NoError(err)
	r.False(finished)
	r.False(cmExists(cli))
	finished, err = gc(cli, "app-v2")
	r.NoError(err)
	r.True(finished)
	_, ok = rtExists(cli)
	r.False(ok)

	// no revision recorded at all: keep the previous behavior
	cli = setup()
	finished, err = gc(cli, "")
	r.NoError(err)
	r.False(finished)
	r.False(cmExists(cli))
}

func TestResourceKeeperGarbageCollectKeepsNewestMatchingHistoryRT(t *testing.T) {
	MarkWithProbability = 1.0
	r := require.New(t)
	ctx := context.Background()

	// history RTs carry the same revision label after repeated spec flips.
	// The newest live one tracks the live resources and must be protected,
	// while an older RT and an RT that is already being deleted must be
	// recycled. historyRTs come back sorted by application generation
	// ascending (SortResourceTrackersByVersion), so slice order always agrees
	// with generation order here: the deleting RT is what discriminates the
	// fix from the old keep-the-last-matching logic, which would protect
	// app-v1-gen3 and recycle the live cm-new instead.
	setup := func() client.Client {
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
		for _, tt := range []struct {
			name     string
			gen      int64
			cm       string
			deleting bool
		}{
			{"app-v1-gen1", 1, "cm-old", false},
			{"app-v1-gen2", 2, "cm-new", false},
			{"app-v1-gen3", 3, "cm-dying", true},
		} {
			rt := &v1beta1.ResourceTracker{
				ObjectMeta: metav1.ObjectMeta{Name: tt.name, Labels: map[string]string{
					oam.LabelAppName:      "app",
					oam.LabelAppNamespace: "default",
					oam.LabelAppUID:       "uid",
					oam.LabelAppRevision:  "app-v1",
				}, Finalizers: []string{resourcetracker.Finalizer}},
				Spec: v1beta1.ResourceTrackerSpec{
					Type:                  v1beta1.ResourceTrackerTypeVersioned,
					ApplicationGeneration: tt.gen,
				},
			}
			r.NoError(cli.Create(ctx, rt))
			cm := &unstructured.Unstructured{}
			cm.SetName(tt.cm)
			cm.SetNamespace("default")
			cm.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
			cm.SetLabels(map[string]string{
				oam.LabelAppName:      "app",
				oam.LabelAppNamespace: "default",
			})
			r.NoError(cli.Create(ctx, cm))
			r.NoError(resourcetracker.RecordManifestsInResourceTracker(ctx, cli, rt, []*unstructured.Unstructured{cm}, true, false, ""))
			if tt.deleting {
				// the finalizer keeps the RT around with a deletion timestamp,
				// as after a previous GC round already marked it
				r.NoError(cli.Delete(ctx, rt))
			}
		}
		return cli
	}

	gc := func(cli client.Client) (bool, error) {
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default", UID: "uid", Generation: 4},
		}
		app.Status.LatestRevision = &apicommon.Revision{Name: "app-v1"}
		_rk, err := NewResourceKeeper(ctx, cli, app)
		r.NoError(err)
		finished, _, err := _rk.(*resourceKeeper).GarbageCollect(ctx)
		return finished, err
	}
	exists := func(cli client.Client, name string) bool {
		rt := &v1beta1.ResourceTracker{}
		return cli.Get(ctx, client.ObjectKey{Name: name}, rt) == nil
	}
	cmExists := func(cli client.Client, name string) bool {
		cm := &unstructured.Unstructured{}
		cm.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
		return cli.Get(ctx, client.ObjectKey{Namespace: "default", Name: name}, cm) == nil
	}

	cli := setup()
	finished, err := gc(cli)
	r.NoError(err)
	r.False(finished)
	// the newest live matching RT and its resources are protected
	r.True(exists(cli, "app-v1-gen2"))
	r.True(cmExists(cli, "cm-new"))
	// the older RT sharing the same revision label is recycled
	r.False(cmExists(cli, "cm-old"))
	// the RT already being deleted is not protected either
	r.False(cmExists(cli, "cm-dying"))
	finished, err = gc(cli)
	r.NoError(err)
	r.True(finished)
	r.False(exists(cli, "app-v1-gen1"))
	r.False(exists(cli, "app-v1-gen3"))
	r.True(exists(cli, "app-v1-gen2"))
	r.True(cmExists(cli, "cm-new"))
}

func TestUpdateSharedManagedResourceOwner(t *testing.T) {
	ctx := context.Background()

	baseCM := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "shared-cm",
				"namespace": "test-ns",
				"labels": map[string]interface{}{
					oam.LabelAppName:      "old-app",
					oam.LabelAppNamespace: "old-ns",
				},
			},
		},
	}

	mockUpdateErr := fmt.Errorf("mock update error")

	testCases := []struct {
		name        string
		setup       func(t *testing.T) (client.Client, *unstructured.Unstructured)
		newSharedBy string
		wantErr     error
		verify      func(t *testing.T, cli client.Client, cm *unstructured.Unstructured)
	}{
		{
			name: "update with multi-tenant sharer",
			setup: func(t *testing.T) (client.Client, *unstructured.Unstructured) {
				r := require.New(t)
				cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
				cm := baseCM.DeepCopy()
				r.NoError(cli.Create(ctx, cm))
				return cli, cm
			},
			newSharedBy: "new-ns/new-app,other-ns/other-app",
			verify: func(t *testing.T, cli client.Client, cm *unstructured.Unstructured) {
				r := require.New(t)
				updatedCM := &unstructured.Unstructured{}
				updatedCM.SetGroupVersionKind(cm.GroupVersionKind())
				r.NoError(cli.Get(ctx, client.ObjectKeyFromObject(cm), updatedCM))
				r.Equal("new-ns/new-app,other-ns/other-app", updatedCM.GetAnnotations()[oam.AnnotationAppSharedBy])
				r.Equal("new-app", updatedCM.GetLabels()[oam.LabelAppName])
				r.Equal("new-ns", updatedCM.GetLabels()[oam.LabelAppNamespace])
			},
		},
		{
			name: "update with single-tenant sharer",
			setup: func(t *testing.T) (client.Client, *unstructured.Unstructured) {
				r := require.New(t)
				cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
				cm := baseCM.DeepCopy()
				r.NoError(cli.Create(ctx, cm))
				return cli, cm
			},
			newSharedBy: "just-an-app",
			verify: func(t *testing.T, cli client.Client, cm *unstructured.Unstructured) {
				r := require.New(t)
				updatedCM := &unstructured.Unstructured{}
				updatedCM.SetGroupVersionKind(cm.GroupVersionKind())
				r.NoError(cli.Get(ctx, client.ObjectKeyFromObject(cm), updatedCM))
				r.Equal("just-an-app", updatedCM.GetAnnotations()[oam.AnnotationAppSharedBy])
				r.Equal("just-an-app", updatedCM.GetLabels()[oam.LabelAppName])
				r.Equal("default", updatedCM.GetLabels()[oam.LabelAppNamespace])
			},
		},
		{
			name: "client update fails",
			setup: func(t *testing.T) (client.Client, *unstructured.Unstructured) {
				cli := &test.MockClient{
					MockUpdate: test.NewMockUpdateFn(mockUpdateErr),
				}
				cm := baseCM.DeepCopy()
				return cli, cm
			},
			newSharedBy: "any/sharer",
			wantErr:     mockUpdateErr,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)
			cli, cm := tc.setup(t)

			err := UpdateSharedManagedResourceOwner(ctx, cli, cm, tc.newSharedBy)

			if tc.wantErr != nil {
				r.Error(err)
				r.Equal(tc.wantErr, err)
			} else {
				r.NoError(err)
			}

			if tc.verify != nil {
				tc.verify(t, cli, cm)
			}
		})
	}
}

// A component read beside the reader orders deletion as its dependsOn would: the
// producer's resources wait for the reader's. A read naming a placement does not.
