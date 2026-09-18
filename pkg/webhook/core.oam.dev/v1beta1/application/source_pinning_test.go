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

package application

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// A binding may pin a definition revision - `type: atlas@v1` - the same way a
// component type can. Admission has to resolve that the way the render does; a
// direct Get looks for an object literally named "atlas@v1", finds nothing, and
// rejects an Application that would have rendered perfectly well.
func TestGetSourceDefinitionResolvesAPinnedRevision(t *testing.T) {
	r := require.New(t)
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)

	live := &v1beta1.SourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "atlas", Namespace: "vela-system"},
		Spec: v1beta1.SourceDefinitionSpec{
			Schematic: &common.Schematic{CUE: &common.CUE{Template: `schema: {clusterName: string, region: string}`}},
		},
	}
	// v1 is the older, narrower shape. Pinning must get this one, not the live one.
	pinned := &v1beta1.DefinitionRevision{
		ObjectMeta: metav1.ObjectMeta{Name: "atlas-v1", Namespace: "vela-system"},
		Spec: v1beta1.DefinitionRevisionSpec{
			DefinitionType: common.SourceType,
			Revision:       1,
			SourceDefinition: v1beta1.SourceDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "atlas", Namespace: "vela-system"},
				Spec: v1beta1.SourceDefinitionSpec{
					Schematic: &common.Schematic{CUE: &common.CUE{Template: `schema: {clusterName: string}`}},
				},
			},
		},
	}

	h := &ValidatingHandler{Client: fake.NewClientBuilder().
		WithScheme(scheme).WithObjects(live, pinned).Build()}
	ctx := context.Background()

	unpinned, err := h.getSourceDefinition(ctx, "default", "atlas", nil)
	r.NoError(err)
	r.Contains(unpinned.Spec.Schematic.CUE.Template, "region", "an unpinned type follows the live definition")

	got, err := h.getSourceDefinition(ctx, "default", "atlas@v1", nil)
	r.NoError(err, "a pinned type must resolve, not report the source as undeclared")
	r.NotContains(got.Spec.Schematic.CUE.Template, "region",
		"admission must type-check against the pinned revision, not whatever is newest")

	_, err = h.getSourceDefinition(ctx, "default", "atlas@v9", nil)
	r.Error(err, "a revision that does not exist is still an error")
}
