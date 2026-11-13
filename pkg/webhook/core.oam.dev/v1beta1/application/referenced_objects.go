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
	"net/http"
	"reflect"

	wfTypesv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/workflow/step"
)

// ReferencedObjectHandler validates Workflow and Policy objects, which Applications reach by
// reference (spec.workflow.ref, a deploy step's policies). Whoever writes one must be able to use
// the definitions it names, and their namespace restrictions apply, as for an Application naming
// them inline; otherwise an Application admitted against a harmless object could have the object
// changed under it.
type ReferencedObjectHandler struct {
	app     *ValidatingHandler
	Decoder admission.Decoder
}

var _ admission.Handler = &ReferencedObjectHandler{}

// Handle checks a created or updated Workflow or Policy. The controller's own writes, made for
// Applications that were checked themselves, are let through.
func (h *ReferencedObjectHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation != admissionv1.Create && req.Operation != admissionv1.Update {
		return admission.Allowed("")
	}
	if h.app.controllerUsername != "" && req.UserInfo.Username == h.app.controllerUsername {
		return admission.Allowed("")
	}
	var uses []indirectUse
	switch req.Kind.Kind {
	case "Workflow":
		wf := &wfTypesv1alpha1.Workflow{}
		if err := h.Decoder.Decode(req, wf); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		steps := field.NewPath("steps")
		for i, s := range wf.Steps {
			if !step.IsBuiltinWorkflowStepType(s.Type) {
				uses = append(uses, indirectUse{s.Type, newWorkflowStepDefinition, steps.Index(i).Child("type"), ""})
			}
			for j, sub := range s.SubSteps {
				if !step.IsBuiltinWorkflowStepType(sub.Type) {
					uses = append(uses, indirectUse{sub.Type, newWorkflowStepDefinition, steps.Index(i).Child("subSteps").Index(j).Child("type"), ""})
				}
			}
		}
	case "Policy":
		po := &v1alpha1.Policy{}
		if err := h.Decoder.Decode(req, po); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		uses = append(uses, indirectUse{po.Type, newPolicyDefinition, field.NewPath("type"), ""})
		if po.Type == v1alpha1.OverridePolicyType {
			var r reach
			r.addOverride(po.Properties, field.NewPath("properties", "components"))
			uses = append(uses, r.uses...)
		}
	default:
		return admission.Allowed("")
	}

	var errs field.ErrorList
	if utilfeature.DefaultMutableFeatureGate.Enabled(features.ValidateDefinitionPermissions) {
		for _, use := range uses {
			errs = append(errs, h.app.permitDefinition(ctx, req, req.Namespace, reflect.TypeOf(use.newDef()).Elem(), use.defType, []*field.Path{use.path})...)
		}
	}
	if utilfeature.DefaultMutableFeatureGate.Enabled(features.RestrictDefinitionNamespaces) {
		c := &restrictionCheck{
			h:      h.app,
			ctx:    ctx,
			defCtx: oamutil.SetNamespaceInCtx(ctx, req.Namespace),
			app:    &v1beta1.Application{},
		}
		c.app.Name, c.app.Namespace = req.Name, req.Namespace
		for _, use := range uses {
			c.checkDefinition(use.defType, use.newDef, []*field.Path{use.path})
		}
		errs = append(errs, c.errs...)
	}
	if len(errs) > 0 {
		return admission.Denied(errs.ToAggregate().Error())
	}
	return admission.Allowed("")
}
