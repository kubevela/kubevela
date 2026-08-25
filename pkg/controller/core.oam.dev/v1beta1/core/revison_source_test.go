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
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func sourceDef(name, template string) *v1beta1.SourceDefinition {
	return &v1beta1.SourceDefinition{
		TypeMeta:   metav1.TypeMeta{APIVersion: "core.oam.dev/v1beta1", Kind: "SourceDefinition"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "vela-system"},
		Spec: v1beta1.SourceDefinitionSpec{
			Schematic: &common.Schematic{CUE: &common.CUE{Template: template}},
		},
	}
}

// Every switch in the revision machinery discriminates on kind, and a missing
// case fails silently rather than loudly: the revision is still created, but
// with an empty hash and a name of "-v1", so two different definitions collide
// and a changed one never mints a new revision.
func TestGatherRevisionInfoForSourceDefinition(t *testing.T) {
	r := require.New(t)

	defRev, lastRevision, err := GatherRevisionInfo(sourceDef("atlas", `schema: {clusterName: string}`))
	r.NoError(err)
	r.Nil(lastRevision, "no prior revision on a first apply")
	r.Equal(common.SourceType, defRev.Spec.DefinitionType)
	r.Equal("atlas", defRev.Spec.SourceDefinition.Name)
	r.NotEmpty(defRev.Spec.RevisionHash, "an unhandled kind hashes to empty, which would collide across definitions")
	r.Len(defRev.OwnerReferences, 1)
	r.Equal("SourceDefinition", defRev.OwnerReferences[0].Kind)

	// A different template must hash differently, or a changed definition would
	// never produce a new revision.
	other, _, err := GatherRevisionInfo(sourceDef("atlas", `schema: {clusterName: string, region: string}`))
	r.NoError(err)
	r.NotEqual(defRev.Spec.RevisionHash, other.Spec.RevisionHash)

	// ...and an identical one must hash the same, or every reconcile would mint
	// a revision.
	same, _, err := GatherRevisionInfo(sourceDef("atlas", `schema: {clusterName: string}`))
	r.NoError(err)
	r.Equal(defRev.Spec.RevisionHash, same.Spec.RevisionHash)
}

func TestSourceDefinitionRevisionNamingAndEquality(t *testing.T) {
	r := require.New(t)

	defRev, _, err := GatherRevisionInfo(sourceDef("atlas", `schema: {clusterName: string}`))
	r.NoError(err)

	name, num := getDefNextRevision(defRev, nil)
	r.Equal("atlas-v1", name, "an unhandled kind would name this \"-v1\"")
	r.EqualValues(1, num)

	name, num = getDefNextRevision(defRev, &common.Revision{Name: "atlas-v1", Revision: 1})
	r.Equal("atlas-v2", name)
	r.EqualValues(2, num)

	changed, _, err := GatherRevisionInfo(sourceDef("atlas", `schema: {clusterName: string, region: string}`))
	r.NoError(err)
	r.True(DeepEqualDefRevision(defRev, defRev))
	r.False(DeepEqualDefRevision(defRev, changed), "a changed spec must not compare equal")
}
