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

package query

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/oam-dev/kubevela/pkg/utils/types"
)

// A reader confined to one namespace, as VelaUX's project group is, must still see a Service
// and its children: owner references never cross namespaces, so neither need the list.
func TestServiceChildrenAreListedInTheServiceNamespace(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "team-a", UID: "svc-uid"}}
	slice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{Name: "web-abcde", Namespace: "team-a",
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "v1", Kind: "Service", Name: "web", UID: "svc-uid"}}},
		AddressType: discoveryv1.AddressTypeIPv4,
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc, slice).WithInterceptorFuncs(interceptor.Funcs{
		List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			o := &client.ListOptions{}
			o.ApplyOptions(opts)
			if o.Namespace == "" {
				return kerrors.NewForbidden(schema.GroupResource{Resource: "list"}, "", nil)
			}
			return c.List(ctx, list, opts...)
		},
	}).Build()

	children, err := iterateListSubResources(context.Background(), "", cli, types.ResourceTreeNode{
		APIVersion: "v1", Kind: "Service", Namespace: "team-a", Name: "web",
	}, 1, func(types.ResourceTreeNode) bool { return true })
	require.NoError(t, err)
	var kinds []string
	for _, c := range children {
		kinds = append(kinds, c.Kind+"/"+c.Name)
	}
	require.Contains(t, kinds, "EndpointSlice/web-abcde")
}

// A child kind the reader may not list is left out rather than failing the parent, which is
// still reported.
func TestAChildKindTheReaderMayNotListIsLeftOut(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "team-a", UID: "svc-uid"}}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).WithInterceptorFuncs(interceptor.Funcs{
		List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
			return kerrors.NewForbidden(schema.GroupResource{Resource: "endpointslices"}, "", nil)
		},
	}).Build()

	children, err := iterateListSubResources(context.Background(), "", cli, types.ResourceTreeNode{
		APIVersion: "v1", Kind: "Service", Namespace: "team-a", Name: "web",
	}, 1, func(types.ResourceTreeNode) bool { return true })
	require.NoError(t, err)
	require.Empty(t, children)
}
