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
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apicommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestResourceCache(t *testing.T) {
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	cache := newResourceCache(cli, nil)
	r := require.New(t)
	createMR := func(name string) v1beta1.ManagedResource {
		return v1beta1.ManagedResource{
			ClusterObjectReference: apicommon.ClusterObjectReference{
				ObjectReference: corev1.ObjectReference{
					Name:       name,
					Kind:       "Deployment",
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
			},
		}
	}
	r.NoError(cli.Create(context.Background(), &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "resource-1"}}))
	r.NoError(cli.Create(context.Background(), &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "resource-2"}}))
	r.NoError(cli.Create(context.Background(), &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "resource-3"}}))
	r.NoError(cli.Create(context.Background(), &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "resource-4"}}))
	r.NoError(cli.Create(context.Background(), &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "resource-6"}}))
	ti := metav1.Now()
	rt1 := &v1beta1.ResourceTracker{
		Spec: v1beta1.ResourceTrackerSpec{
			ManagedResources: []v1beta1.ManagedResource{
				createMR("resource-1"),
				createMR("resource-3"),
			},
		},
	}
	rt2 := &v1beta1.ResourceTracker{
		ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &ti},
		Spec: v1beta1.ResourceTrackerSpec{
			ManagedResources: []v1beta1.ManagedResource{
				createMR("resource-1"),
				createMR("resource-2"),
				createMR("resource-3"),
				createMR("resource-4"),
			},
		},
	}
	rt3 := &v1beta1.ResourceTracker{
		Spec: v1beta1.ResourceTrackerSpec{
			ManagedResources: []v1beta1.ManagedResource{
				createMR("resource-1"),
				createMR("resource-4"),
				createMR("resource-5"),
			},
		},
	}
	rts := []*v1beta1.ResourceTracker{nil, rt1, rt2, rt3}
	cache.registerResourceTrackers(rts...)
	o, ok := cache.m.Get(createMR("resource-1").ResourceKey())
	r.True(ok)
	r.False(o.loaded)
	for _, check := range []struct {
		name           string
		usedBy         []*v1beta1.ResourceTracker
		latestActiveRT *v1beta1.ResourceTracker
		gcExecutorRT   *v1beta1.ResourceTracker
		notFound       bool
	}{{
		name:           "resource-1",
		usedBy:         []*v1beta1.ResourceTracker{rt1, rt2, rt3},
		latestActiveRT: rt3,
		gcExecutorRT:   rt3,
	}, {
		name:           "resource-2",
		usedBy:         []*v1beta1.ResourceTracker{rt2},
		latestActiveRT: nil,
		gcExecutorRT:   rt2,
	}, {
		name:           "resource-3",
		usedBy:         []*v1beta1.ResourceTracker{rt1, rt2},
		latestActiveRT: rt1,
		gcExecutorRT:   rt1,
	}, {
		name:           "resource-4",
		usedBy:         []*v1beta1.ResourceTracker{rt2, rt3},
		latestActiveRT: rt3,
		gcExecutorRT:   rt3,
	}, {
		name:           "resource-5",
		usedBy:         []*v1beta1.ResourceTracker{rt3},
		latestActiveRT: rt3,
		gcExecutorRT:   rt3,
		notFound:       true,
	}, {
		name: "resource-6",
	}} {
		entry := cache.get(context.Background(), createMR(check.name))
		r.Equal(check.usedBy, entry.usedBy)
		r.Equal(check.latestActiveRT, entry.latestActiveRT)
		r.Equal(check.gcExecutorRT, entry.gcExecutorRT)
		r.Equal(check.notFound, !entry.exists)
		r.True(entry.loaded)
	}
}

func TestResourceCacheExistenceCheck(t *testing.T) {
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	cache := newResourceCache(cli, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{
		Name:      "app",
		Namespace: "test",
	}})
	r := require.New(t)
	createResource := func(appName, appNs, sharedBy string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"labels":          map[string]interface{}{oam.LabelAppName: appName, oam.LabelAppNamespace: appNs},
				"annotations":     map[string]interface{}{oam.AnnotationAppSharedBy: sharedBy},
				"resourceVersion": "-",
			},
		}}
	}
	r.True(cache.exists(createResource("app", "test", "")))
	r.False(cache.exists(createResource("app-no-shared-by", "test", "")))
	r.True(cache.exists(createResource("app-shared-by", "ex", "x/y,test/app,ex/app-shared-by")))
}
