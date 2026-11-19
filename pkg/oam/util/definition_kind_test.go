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

package util_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// DefinitionRevisions are named "<definition>-v<n>" with no kind in the name, so
// two definitions of different kinds that share a name share a revision name.
// That is not hypothetical: KubeVela ships a "configmap" trait, and naming a
// SourceDefinition after what it reads makes "configmap" the obvious choice.
//
// Without a kind check the collision is silent. GetCapabilityDefinition assigns
// defRev.Spec.SourceDefinition, which on a Trait revision is a zero struct, so
// the caller receives an empty definition and no error - surfacing later as
// "declares no parameters" and "undefined field", neither of which points at the
// real problem.
func TestGetCapabilityDefinitionRejectsARevisionOfAnotherKind(t *testing.T) {
	r := require.New(t)
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)

	traitRev := &v1beta1.DefinitionRevision{
		ObjectMeta: metav1.ObjectMeta{Name: "configmap-v1", Namespace: "vela-system"},
		Spec: v1beta1.DefinitionRevisionSpec{
			DefinitionType: common.TraitType,
			Revision:       1,
			TraitDefinition: v1beta1.TraitDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "configmap", Namespace: "vela-system"},
			},
		},
	}
	sourceRev := &v1beta1.DefinitionRevision{
		ObjectMeta: metav1.ObjectMeta{Name: "configmap-v2", Namespace: "vela-system"},
		Spec: v1beta1.DefinitionRevisionSpec{
			DefinitionType: common.SourceType,
			Revision:       2,
			SourceDefinition: v1beta1.SourceDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "configmap", Namespace: "vela-system"},
				Spec: v1beta1.SourceDefinitionSpec{
					Schematic: &common.Schematic{CUE: &common.CUE{Template: `schema: {a: string}`}},
				},
			},
		},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(traitRev, sourceRev).Build()
	ctx := util.SetNamespaceInCtx(context.Background(), "vela-system")

	// The collision: a Source asking for @v1 lands on the Trait's revision.
	def := &v1beta1.SourceDefinition{}
	err := util.GetCapabilityDefinition(ctx, cli, def, "configmap@v1", nil)
	r.Error(err, "a revision holding another kind must be an error, not an empty definition")
	r.Contains(err.Error(), "Trait", "the message should name what the revision actually holds")
	r.Contains(err.Error(), "configmap-v1")

	// The matching revision still resolves.
	def = &v1beta1.SourceDefinition{}
	r.NoError(util.GetCapabilityDefinition(ctx, cli, def, "configmap@v2", nil))
	r.NotNil(def.Spec.Schematic)
}
