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
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// A release that adds a field to a definition's spec changes every definition's hash, content
// unchanged. The stored revision, hashed by the release before, must still be recognised.
func TestGenerateDefinitionRevisionKeepsARevisionHashedByAnEarlierRelease(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	spec := v1beta1.ComponentDefinitionSpec{Workload: common.WorkloadTypeDescriptor{Type: "deployments.apps"}}
	stored := &v1beta1.DefinitionRevision{
		ObjectMeta: metav1.ObjectMeta{Name: "webservice-v1", Namespace: "vela-system"},
		Spec: v1beta1.DefinitionRevisionSpec{
			DefinitionType: common.ComponentType,
			Revision:       1,
			RevisionHash:   "hashed-by-1.11",
			ComponentDefinition: v1beta1.ComponentDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "webservice", Namespace: "vela-system"},
				Spec:       spec,
			},
		},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(stored).Build()
	def := func(s v1beta1.ComponentDefinitionSpec) *v1beta1.ComponentDefinition {
		return &v1beta1.ComponentDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "webservice", Namespace: "vela-system"},
			Spec:       s,
			Status: v1beta1.ComponentDefinitionStatus{LatestRevision: &common.Revision{
				Name: "webservice-v1", Revision: 1, RevisionHash: "hashed-by-1.11",
			}},
		}
	}

	defRev, isNew, err := GenerateDefinitionRevision(context.Background(), cli, def(spec))
	require.NoError(t, err)
	require.False(t, isNew, "identical content must not mint a revision")
	require.Equal(t, "webservice-v1", defRev.Name)
	require.EqualValues(t, 1, defRev.Spec.Revision)

	changed := spec
	changed.Workload.Type = "statefulsets.apps"
	defRev, isNew, err = GenerateDefinitionRevision(context.Background(), cli, def(changed))
	require.NoError(t, err)
	require.True(t, isNew, "a content change mints a revision")
	require.Equal(t, "webservice-v2", defRev.Name)
}
