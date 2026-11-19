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

// A restriction is always read from the live definition, so it must not affect
// the revision hash: at the chart's definitionRevisionLimit of 2, restricting the
// builtins would roll all of them and collect revisions Applications pin.
func TestDefinitionRevisionHashIgnoresRestrictions(t *testing.T) {
	restrictions := &common.DefinitionRestrictions{
		Namespaces:        []string{"tenant-*"},
		NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"tenant": "true"}},
	}

	hashOf := func(defRev *v1beta1.DefinitionRevision) string {
		t.Helper()
		h, err := computeDefinitionRevisionHash(defRev)
		require.NoError(t, err)
		require.NotEmpty(t, h)
		return h
	}

	t.Run("ComponentDefinition", func(t *testing.T) {
		plain := &v1beta1.DefinitionRevision{Spec: v1beta1.DefinitionRevisionSpec{
			DefinitionType: common.ComponentType,
			ComponentDefinition: v1beta1.ComponentDefinition{Spec: v1beta1.ComponentDefinitionSpec{
				Workload: common.WorkloadTypeDescriptor{Type: "deployments.apps"},
			}},
		}}
		restricted := plain.DeepCopy()
		restricted.Spec.ComponentDefinition.Spec.Restrictions = restrictions
		require.Equal(t, hashOf(plain), hashOf(restricted))

		// A real spec change still mints a new revision.
		changed := plain.DeepCopy()
		changed.Spec.ComponentDefinition.Spec.Workload.Type = "statefulsets.apps"
		require.NotEqual(t, hashOf(plain), hashOf(changed))
	})

	t.Run("TraitDefinition", func(t *testing.T) {
		plain := &v1beta1.DefinitionRevision{Spec: v1beta1.DefinitionRevisionSpec{
			DefinitionType:  common.TraitType,
			TraitDefinition: v1beta1.TraitDefinition{Spec: v1beta1.TraitDefinitionSpec{PodDisruptive: true}},
		}}
		restricted := plain.DeepCopy()
		restricted.Spec.TraitDefinition.Spec.Restrictions = restrictions
		require.Equal(t, hashOf(plain), hashOf(restricted))
	})

	t.Run("PolicyDefinition", func(t *testing.T) {
		plain := &v1beta1.DefinitionRevision{Spec: v1beta1.DefinitionRevisionSpec{
			DefinitionType:   common.PolicyType,
			PolicyDefinition: v1beta1.PolicyDefinition{Spec: v1beta1.PolicyDefinitionSpec{Version: "1.0.0"}},
		}}
		restricted := plain.DeepCopy()
		restricted.Spec.PolicyDefinition.Spec.Restrictions = restrictions
		require.Equal(t, hashOf(plain), hashOf(restricted))
	})

	t.Run("WorkflowStepDefinition", func(t *testing.T) {
		plain := &v1beta1.DefinitionRevision{Spec: v1beta1.DefinitionRevisionSpec{
			DefinitionType:         common.WorkflowStepType,
			WorkflowStepDefinition: v1beta1.WorkflowStepDefinition{Spec: v1beta1.WorkflowStepDefinitionSpec{Version: "1.0.0"}},
		}}
		restricted := plain.DeepCopy()
		restricted.Spec.WorkflowStepDefinition.Spec.Restrictions = restrictions
		require.Equal(t, hashOf(plain), hashOf(restricted))
	})

	t.Run("SourceDefinition", func(t *testing.T) {
		plain := &v1beta1.DefinitionRevision{Spec: v1beta1.DefinitionRevisionSpec{
			DefinitionType:   common.SourceType,
			SourceDefinition: *sourceDef("probe", "output: {}"),
		}}
		restricted := plain.DeepCopy()
		restricted.Spec.SourceDefinition.Spec.Restrictions = restrictions
		require.Equal(t, hashOf(plain), hashOf(restricted))
	})

	// Hashing a copy must not mutate the revision it was handed.
	t.Run("the definition is not modified by hashing", func(t *testing.T) {
		defRev := &v1beta1.DefinitionRevision{Spec: v1beta1.DefinitionRevisionSpec{
			DefinitionType: common.ComponentType,
			ComponentDefinition: v1beta1.ComponentDefinition{Spec: v1beta1.ComponentDefinitionSpec{
				Workload:     common.WorkloadTypeDescriptor{Type: "deployments.apps"},
				Restrictions: restrictions,
			}},
		}}
		_ = hashOf(defRev)
		require.Equal(t, restrictions, defRev.Spec.ComponentDefinition.Spec.Restrictions)
	})
}
