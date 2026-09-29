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
