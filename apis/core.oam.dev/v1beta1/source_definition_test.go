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

package v1beta1

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
)

// The controller reports readiness through these, so a SourceDefinition that
// cannot carry a condition reports nothing at all.
func TestSourceDefinitionConditions(t *testing.T) {
	def := &SourceDefinition{}
	absent := def.GetCondition(condition.TypeReady)
	require.Equal(t, condition.TypeReady, absent.Type)
	require.Equal(t, corev1.ConditionUnknown, absent.Status,
		"a definition with no conditions reports Unknown for the type asked about, "+
			"which is a different answer from not ready")

	def.SetConditions(condition.Available())
	got := def.GetCondition(condition.TypeReady)
	require.Equal(t, condition.TypeReady, got.Type)
	require.Equal(t, corev1.ConditionTrue, got.Status)

	def.SetConditions(condition.Unavailable())
	require.Equal(t, corev1.ConditionFalse, def.GetCondition(condition.TypeReady).Status,
		"a later condition of the same type replaces the earlier one")
}
