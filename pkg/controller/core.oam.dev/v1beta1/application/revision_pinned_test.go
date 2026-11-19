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
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
)

// Policies pinning different revisions of one PolicyDefinition each get an
// entry under their reference, beside the entry under the definition's name.
func TestRevisionRecordsEachPinnedPolicyRevision(t *testing.T) {
	policyDef := func(data string) *v1beta1.PolicyDefinition {
		return &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "configmap-policy"},
			Spec: v1beta1.PolicyDefinitionSpec{
				Schematic: &common.Schematic{CUE: &common.CUE{Template: data}},
			},
		}
	}
	policy := func(ref, data string) *appfile.Component {
		return &appfile.Component{
			Name:         ref,
			Reference:    ref,
			FullTemplate: &appfile.Template{PolicyDefinition: policyDef(data)},
		}
	}

	h := &AppHandler{app: &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"}}}
	af := &appfile.Appfile{
		ParsedPolicies: []*appfile.Component{
			policy("configmap-policy@v1", "output: revision: 1"),
			policy("configmap-policy@v2", "output: revision: 2"),
		},
		RelatedComponentDefinitions:    map[string]*v1beta1.ComponentDefinition{},
		RelatedTraitDefinitions:        map[string]*v1beta1.TraitDefinition{},
		RelatedWorkflowStepDefinitions: map[string]*v1beta1.WorkflowStepDefinition{},
	}

	rev, _, err := h.gatherRevisionSpec(af)
	require.NoError(t, err)

	defs := rev.Spec.PolicyDefinitions
	require.Contains(t, defs, "configmap-policy@v1")
	require.Contains(t, defs, "configmap-policy@v2")
	require.Equal(t, "output: revision: 1", defs["configmap-policy@v1"].Spec.Schematic.CUE.Template)
	require.Equal(t, "output: revision: 2", defs["configmap-policy@v2"].Spec.Schematic.CUE.Template)
	require.Contains(t, defs, "configmap-policy", "the bare entry stays for readers that look up by name")
}
