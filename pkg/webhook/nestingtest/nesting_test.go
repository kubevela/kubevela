/*
 Copyright 2026. The KubeVela Authors.

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

// Package nestingtest checks that every webhook taking CUE refuses a template nested too deeply
// before parsing it.
package nestingtest

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/pkg/cue/nesting"
	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1beta1/componentdefinition"
	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1beta1/policydefinition"
	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1beta1/traitdefinition"
	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1beta1/workflowstepdefinition"
)

func TestEveryWebhookTakingCUERefusesTooDeepATemplate(t *testing.T) {
	deep := "output: " + strings.Repeat("[", nesting.MaxDepth+1)
	for kind, h := range map[string]admission.Handler{
		"ComponentDefinition":    &componentdefinition.ValidatingHandler{},
		"TraitDefinition":        &traitdefinition.ValidatingHandler{},
		"PolicyDefinition":       &policydefinition.ValidatingHandler{},
		"WorkflowStepDefinition": &workflowstepdefinition.ValidatingHandler{},
	} {
		raw, err := json.Marshal(map[string]interface{}{
			"apiVersion": "core.oam.dev/v1beta1", "kind": kind,
			"metadata": map[string]interface{}{"name": "deep", "namespace": "vela-system"},
			"spec":     map[string]interface{}{"schematic": map[string]interface{}{"cue": map[string]interface{}{"template": deep}}, "template": deep},
		})
		require.NoError(t, err)
		resp := h.Handle(context.Background(), admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create, Object: runtime.RawExtension{Raw: raw}}})
		require.False(t, resp.Allowed, kind)
		require.Contains(t, resp.Result.Message, "nested deeper than 1000 levels", kind)
	}
}
