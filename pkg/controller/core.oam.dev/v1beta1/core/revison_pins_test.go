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
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velacache "github.com/oam-dev/kubevela/pkg/cache"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func TestCleanUpDefinitionRevisionKeepsPinnedRevisions(t *testing.T) {
	for _, indexed := range []bool{true, false} {
		t.Run(fmt.Sprintf("indexed=%v", indexed), func(t *testing.T) {
			prev := velacache.DefinitionPinIndexed
			velacache.DefinitionPinIndexed = indexed
			t.Cleanup(func() { velacache.DefinitionPinIndexed = prev })

			scheme := runtime.NewScheme()
			require.NoError(t, v1beta1.AddToScheme(scheme))
			objs := []client.Object{
				// Pins v1 from another namespace; the definition is in vela-system, so any may.
				&v1beta1.Application{
					ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "team-a"},
					Spec:       v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{{Name: "web", Type: "webservice@v1"}}},
				},
				// A trait of the same name pinned at v2 is not the component's v2.
				&v1beta1.Application{
					ObjectMeta: metav1.ObjectMeta{Name: "blog", Namespace: "team-b"},
					Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{{
						Name: "web", Type: "worker", Traits: []common.ApplicationTrait{{Type: "webservice@v2"}},
					}}},
				},
			}
			for i := 1; i <= 5; i++ {
				objs = append(objs, &v1beta1.DefinitionRevision{
					ObjectMeta: metav1.ObjectMeta{
						Name: fmt.Sprintf("webservice-v%d", i), Namespace: oam.SystemDefinitionNamespace,
						Labels: map[string]string{oam.LabelComponentDefinitionName: "webservice"},
					},
					Spec: v1beta1.DefinitionRevisionSpec{DefinitionType: common.ComponentType, Revision: int64(i)},
				})
			}
			builder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...)
			if indexed {
				builder = builder.WithIndex(&v1beta1.Application{}, velacache.DefinitionPinIndex, func(obj client.Object) []string {
					return velacache.DefinitionPinsOf(obj.(*v1beta1.Application))
				})
			}
			cli := builder.Build()
			def := &v1beta1.ComponentDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "webservice", Namespace: oam.SystemDefinitionNamespace},
				Status:     v1beta1.ComponentDefinitionStatus{LatestRevision: &common.Revision{Name: "webservice-v5", Revision: 5}},
			}

			// A limit of 2 keeps v5 (in use) and the two newest besides it, v3 and v4. v1 is pinned.
			require.NoError(t, CleanUpDefinitionRevision(context.Background(), cli, def, 2))

			list := &v1beta1.DefinitionRevisionList{}
			require.NoError(t, cli.List(context.Background(), list))
			var names []string
			for _, r := range list.Items {
				names = append(names, r.Name)
			}
			require.ElementsMatch(t, []string{"webservice-v1", "webservice-v3", "webservice-v4", "webservice-v5"}, names)
		})
	}
}

// A negative limit keeps no extra revisions, as zero does, rather than slicing past the end.
func TestCleanUpDefinitionRevisionWithANegativeLimit(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	var objs []client.Object
	for i := 1; i <= 3; i++ {
		objs = append(objs, &v1beta1.DefinitionRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("webservice-v%d", i), Namespace: oam.SystemDefinitionNamespace,
				Labels: map[string]string{oam.LabelComponentDefinitionName: "webservice"},
			},
			Spec: v1beta1.DefinitionRevisionSpec{DefinitionType: common.ComponentType, Revision: int64(i)},
		})
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	def := &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "webservice", Namespace: oam.SystemDefinitionNamespace},
		Status:     v1beta1.ComponentDefinitionStatus{LatestRevision: &common.Revision{Name: "webservice-v3", Revision: 3}},
	}
	require.NoError(t, CleanUpDefinitionRevision(context.Background(), cli, def, -2))
	list := &v1beta1.DefinitionRevisionList{}
	require.NoError(t, cli.List(context.Background(), list))
	require.Len(t, list.Items, 1, "only the revision in use")
	require.Equal(t, "webservice-v3", list.Items[0].Name)
}
