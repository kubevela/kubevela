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

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apicommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestResourceKeeperGarbageCollect(t *testing.T) {
	MarkWithProbability = 1.0
	r := require.New(t)
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	ctx := context.Background()

	rtMaps := map[int64]*v1beta1.ResourceTracker{}
	cmMaps := map[int]*unstructured.Unstructured{}
	crMaps := map[int]*appsv1.ControllerRevision{}

	crRT := &v1beta1.ResourceTracker{
		ObjectMeta: metav1.ObjectMeta{Name: "app-comp-rev", Labels: map[string]string{
			oam.LabelAppName:      "app",
			oam.LabelAppNamespace: "default",
			oam.LabelAppUID:       "uid",
		}, Finalizers: []string{resourcetracker.Finalizer}},
		Spec: v1beta1.ResourceTrackerSpec{
			Type: v1beta1.ResourceTrackerTypeComponentRevision,
		},
	}
	r.NoError(cli.Create(ctx, crRT))

	createRT := func(gen int64) {
		_rt := &v1beta1.ResourceTracker{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("app-v%d", gen), Labels: map[string]string{
				oam.LabelAppName:      "app",
				oam.LabelAppNamespace: "default",
				oam.LabelAppUID:       "uid",
			}, Finalizers: []string{resourcetracker.Finalizer}},
			Spec: v1beta1.ResourceTrackerSpec{
				Type:                  v1beta1.ResourceTrackerTypeVersioned,
				ApplicationGeneration: gen,
			},
		}
		r.NoError(cli.Create(ctx, _rt))
		rtMaps[gen] = _rt
	}

	addConfigMapToRT := func(i int, gen int64, compID int) {
		_rt := rtMaps[gen]
		if _, exists := cmMaps[i]; !exists {
			cm := &unstructured.Unstructured{}
			cm.SetName(fmt.Sprintf("cm-%d", i))
			cm.SetNamespace("default")
			cm.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
			cm.SetLabels(map[string]string{
				oam.LabelAppComponent: fmt.Sprintf("comp-%d", compID),
				oam.LabelAppNamespace: "default",
				oam.LabelAppName:      "app",
			})
			r.NoError(cli.Create(ctx, cm))
			cmMaps[i] = cm
		}
		if _, exists := crMaps[compID]; !exists {
			cr := &appsv1.ControllerRevision{Data: runtime.RawExtension{Raw: []byte(`{}`)}}
			cr.SetName(fmt.Sprintf("cr-comp-%d", compID))
			cr.SetNamespace("default")
			cr.SetLabels(map[string]string{
				oam.LabelAppComponent: fmt.Sprintf("comp-%d", compID),
			})
			r.NoError(cli.Create(ctx, cr))
			crMaps[compID] = cr
			obj := &unstructured.Unstructured{}
			obj.SetName(cr.GetName())
			obj.SetNamespace(cr.GetNamespace())
			obj.SetLabels(cr.GetLabels())
			r.NoError(resourcetracker.RecordManifestsInResourceTracker(ctx, cli, crRT, []*unstructured.Unstructured{obj}, true, false, ""))
		}
		r.NoError(resourcetracker.RecordManifestsInResourceTracker(ctx, cli, _rt, []*unstructured.Unstructured{cmMaps[i]}, true, false, ""))
	}

	checkCount := func(cmCount, rtCount int, crCount int) {
		n := 0
		for _, v := range cmMaps {
			o := &unstructured.Unstructured{}
			o.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
			err := cli.Get(ctx, client.ObjectKeyFromObject(v), o)
			if err == nil {
				n += 1
			}
		}
		r.Equal(cmCount, n)
		_rts := &v1beta1.ResourceTrackerList{}
		r.NoError(cli.List(ctx, _rts))
		r.Equal(rtCount, len(_rts.Items))
		_crs := &appsv1.ControllerRevisionList{}
		r.NoError(cli.List(ctx, _crs))
		r.Equal(crCount, len(_crs.Items))
	}

	createRK := func(gen int64, keepLegacy bool, order v1alpha1.GarbageCollectOrder, components ...apicommon.ApplicationComponent) *resourceKeeper {
		_rk, err := NewResourceKeeper(ctx, cli, &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default", UID: "uid", Generation: gen},
			Spec:       v1beta1.ApplicationSpec{Components: components},
		})
		r.NoError(err)
		rk := _rk.(*resourceKeeper)
		rk.garbageCollectPolicy = &v1alpha1.GarbageCollectPolicySpec{
			Order:              order,
			KeepLegacyResource: keepLegacy,
		}
		return rk
	}

	createRT(1)
	addConfigMapToRT(1, 1, 1)
	addConfigMapToRT(2, 1, 2)
	createRT(2)
	addConfigMapToRT(1, 2, 1)
	addConfigMapToRT(3, 2, 3)
	createRT(3)
	addConfigMapToRT(4, 3, 3)
	createRT(4)
	addConfigMapToRT(5, 4, 4)
	addConfigMapToRT(6, 4, 5)
	addConfigMapToRT(7, 4, 6)
	checkCount(7, 5, 6)

	opts := []GCOption{DisableLegacyGCOption{}}
	// no need to gc
	rk := createRK(4, true, "")
	finished, _, err := rk.GarbageCollect(ctx, opts...)
	r.NoError(err)
	r.True(finished)
	checkCount(7, 5, 6)

	// delete rt2, trigger gc for cm3
	dt := metav1.Now()
	rtMaps[2].SetDeletionTimestamp(&dt)
	r.NoError(cli.Update(ctx, rtMaps[2]))
	rk = createRK(4, true, "")
	finished, _, err = rk.GarbageCollect(ctx, opts...)
	r.NoError(err)
	r.False(finished)
	rk = createRK(4, true, "")
	finished, _, err = rk.GarbageCollect(ctx, opts...)
	r.NoError(err)
	r.True(finished)
	checkCount(6, 4, 6)

	// delete cm4, trigger gc for rt3, comp-3 no use
	r.NoError(cli.Delete(ctx, cmMaps[4]))
	rk = createRK(5, true, "")
	finished, _, err = rk.GarbageCollect(ctx, opts...)
	r.NoError(err)
	r.True(finished)
	checkCount(5, 3, 5)

	// upgrade and gc legacy rt1
	rk = createRK(4, false, "")
	finished, _, err = rk.GarbageCollect(ctx, opts...)
	r.NoError(err)
	r.False(finished)
	rk = createRK(4, false, "")
	finished, _, err = rk.GarbageCollect(ctx, opts...)
	r.NoError(err)
	r.True(finished)
	checkCount(3, 2, 3)

	// delete with sequential
	comps := []apicommon.ApplicationComponent{
		{
			Name: "comp-5",
			DependsOn: []string{
				"comp-6",
			},
		},
		{
			Name: "comp-6",
			DependsOn: []string{
				"comp-7",
			},
		},
		{
			Name: "comp-7",
		},
	}
	rk = createRK(5, false, v1alpha1.OrderDependency, comps...)
	rtMaps[3].SetDeletionTimestamp(&dt)
	finished, _, err = rk.GarbageCollect(ctx, opts...)
	r.NoError(err)
	r.False(finished)
	rk = createRK(5, false, v1alpha1.OrderDependency, comps...)
	finished, _, err = rk.GarbageCollect(ctx, opts...)
	r.NoError(err)
	r.False(finished)
	rk = createRK(5, false, v1alpha1.OrderDependency, comps...)
	finished, _, err = rk.GarbageCollect(ctx, opts...)
	r.NoError(err)
	r.True(finished)

	r.NoError(cli.Get(ctx, client.ObjectKeyFromObject(crRT), crRT))
	// recreate rt, delete app, gc all
	createRT(5)
	addConfigMapToRT(8, 5, 8)
	addConfigMapToRT(9, 5, 8)
	createRT(6)
	addConfigMapToRT(9, 6, 8)
	addConfigMapToRT(10, 6, 8)
	checkCount(3, 3, 1)

	rk = createRK(6, false, "")
	rk.app.SetDeletionTimestamp(&dt)
	finished, _, err = rk.GarbageCollect(ctx, opts...)
	r.NoError(err)
	r.False(finished)
	rk = createRK(6, false, "")
	finished, _, err = rk.GarbageCollect(ctx, opts...)
	r.NoError(err)
	r.True(finished)
	checkCount(0, 0, 0)

	rk = createRK(7, false, "")
	finished, _, err = rk.GarbageCollect(ctx, opts...)
	r.NoError(err)
	r.True(finished)
}

func TestCheckDependentComponent(t *testing.T) {
	rk := &resourceKeeper{
		app: &v1beta1.Application{
			Spec: v1beta1.ApplicationSpec{
				Components: []apicommon.ApplicationComponent{
					{
						Name: "comp-1",
						Outputs: workflowv1alpha1.StepOutputs{
							{
								Name: "output-1",
							},
						},
					},
					{
						Name: "comp-2",
						Outputs: workflowv1alpha1.StepOutputs{
							{
								Name: "output-2",
							},
						},
					},
					{
						Name: "comp-3",
						Inputs: workflowv1alpha1.StepInputs{
							{
								From: "output-1",
							},
							{
								From: "output-2",
							},
						},
					},
					{
						Name:      "comp-4",
						DependsOn: []string{"comp-3"},
					},
					{
						Name:      "comp-5",
						DependsOn: []string{"comp-4", "comp-3"},
					},
				},
			},
		},
	}
	testCases := []struct {
		comp   string
		result []string
	}{
		{
			comp:   "comp-1",
			result: []string{"comp-3"},
		},
		{
			comp:   "comp-2",
			result: []string{"comp-3"},
		},
		{
			comp:   "comp-3",
			result: []string{"comp-4", "comp-5"},
		},
		{
			comp:   "comp-4",
			result: []string{"comp-5"},
		},
		{
			comp:   "comp-5",
			result: []string{},
		},
	}
	gcHandler := &gcHandler{
		resourceKeeper: rk,
	}
	r := require.New(t)
	for _, tc := range testCases {
		mr := v1beta1.ManagedResource{
			OAMObjectReference: apicommon.OAMObjectReference{
				Component: tc.comp,
			},
		}
		r.Equal(gcHandler.checkDependentComponent(mr), tc.result)
	}
}
