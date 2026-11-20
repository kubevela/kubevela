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

package application

import (
	"context"
	"net/http"

	wfTypesv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/workflow/step"
)

// ReferencedObjectHandler validates Workflow and Policy objects, which Applications reach by
// reference (spec.workflow.ref, a deploy step's policies). Whoever writes one must be able to use
// the definitions it names, as for an Application naming them inline; otherwise an Application
// admitted against a harmless object could have the object changed under it.
type ReferencedObjectHandler struct {
	app     *ValidatingHandler
	Decoder admission.Decoder
}

var _ admission.Handler = &ReferencedObjectHandler{}

// Handle checks a created or updated Workflow or Policy.
func (h *ReferencedObjectHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation != admissionv1.Create && req.Operation != admissionv1.Update {
		return admission.Allowed("")
	}
	// The controller compiles a Workflow's step conditions and data passing for every Application
	// that references it, so they are checked whatever the feature gates.
	if req.Kind.Kind == "Workflow" {
		wf := &wfTypesv1alpha1.Workflow{}
		if err := h.Decoder.Decode(req, wf); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if errs := StepsCUEDepth(field.NewPath("steps"), wf.Steps); len(errs) > 0 {
			return admission.Denied(errs.ToAggregate().Error())
		}
	}
	if !utilfeature.DefaultMutableFeatureGate.Enabled(features.ValidateDefinitionPermissions) {
		return admission.Allowed("")
	}
	var uses []indirectUse
	var unreadable []unreadableReference
	switch req.Kind.Kind {
	case "Workflow":
		wf := &wfTypesv1alpha1.Workflow{}
		if err := h.Decoder.Decode(req, wf); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		steps := field.NewPath("steps")
		for i, s := range wf.Steps {
			if !step.IsBuiltinWorkflowStepType(s.Type) {
				uses = append(uses, indirectUse{s.Type, workflowStepDefinitionType, steps.Index(i).Child("type")})
			}
			for j, sub := range s.SubSteps {
				if !step.IsBuiltinWorkflowStepType(sub.Type) {
					uses = append(uses, indirectUse{sub.Type, workflowStepDefinitionType, steps.Index(i).Child("subSteps").Index(j).Child("type")})
				}
			}
		}
		// A deploy step loads Policy objects by name from the Workflow's namespace; what they set is
		// used as if the Workflow named it.
		var r reach
		for i, s := range wf.Steps {
			r.addDeployPolicies(ctx, h.app.Client, req.Namespace, nil, s, stepPolicyPath(steps.Index(i)))
		}
		uses = append(uses, r.uses...)
		unreadable = r.unreadable
	case "Policy":
		po := &v1alpha1.Policy{}
		if err := h.Decoder.Decode(req, po); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		uses = append(uses, indirectUse{po.Type, policyDefinitionType, field.NewPath("type")})
		if po.Type == v1alpha1.OverridePolicyType {
			var r reach
			r.addOverride(po.Properties, field.NewPath("properties", "components"))
			uses = append(uses, r.uses...)
		}
	default:
		return admission.Allowed("")
	}

	var errs field.ErrorList
	for _, u := range unreadable {
		errs = append(errs, field.InternalError(u.path, u.err))
	}
	errs = append(errs, h.app.permitUses(ctx, req, req.Namespace, uses)...)
	if len(errs) > 0 {
		return admission.Denied(errs.ToAggregate().Error())
	}
	return admission.Allowed("")
}
