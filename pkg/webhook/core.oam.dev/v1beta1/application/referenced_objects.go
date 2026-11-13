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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// Handle checks a created or updated Workflow or Policy. One the controller writes is checked for
// the Application that dispatched it: restrictions against that Application's namespace, and
// permissions as its author where that identity can be trusted.
func (h *ReferencedObjectHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation != admissionv1.Create && req.Operation != admissionv1.Update {
		return admission.Allowed("")
	}
	var obj metav1.Object
	var uses []indirectUse
	var wf *wfTypesv1alpha1.Workflow
	switch req.Kind.Kind {
	case "Workflow":
		wf = &wfTypesv1alpha1.Workflow{}
		if err := h.Decoder.Decode(req, wf); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		obj = wf
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
		obj = po
		uses = append(uses, indirectUse{po.Type, newPolicyDefinition, field.NewPath("type"), ""})
		if po.Type == v1alpha1.OverridePolicyType {
			var r reach
			r.addOverride(po.Properties, field.NewPath("properties", "components"))
			uses = append(uses, r.uses...)
		}
	default:
		return admission.Allowed("")
	}

	asker, namespace, checkPermissions := req, req.Namespace, true
	if h.app.fromController(req) {
		if dispatcher := dispatchingNamespace(obj); dispatcher != "" {
			namespace = dispatcher
		}
		author, trusted, err := h.app.dispatcherAuthor(ctx, obj)
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}
		asker.UserInfo, checkPermissions = author, trusted
	}

	var errs field.ErrorList
	if wf != nil {
		// A deploy step loads Policy objects by name from the namespace it runs in; what they set is
		// used as if the Workflow named it.
		var r reach
		steps := field.NewPath("steps")
		for i, s := range wf.Steps {
			r.addDeployPolicies(ctx, h.app.Client, namespace, nil, s, stepPolicyPath(steps.Index(i)))
		}
		uses = append(uses, r.uses...)
		for _, u := range r.unreadable {
			errs = append(errs, field.InternalError(u.path, u.err))
		}
	}
	if checkPermissions && utilfeature.DefaultMutableFeatureGate.Enabled(features.ValidateDefinitionPermissions) {
		for _, use := range uses {
			errs = append(errs, h.app.permitDefinition(ctx, asker, namespace, reflect.TypeOf(use.newDef()).Elem(), use.defType, []*field.Path{use.path})...)
		}
	}
	if utilfeature.DefaultMutableFeatureGate.Enabled(features.RestrictDefinitionNamespaces) {
		c := &restrictionCheck{
			h:      h.app,
			ctx:    ctx,
			defCtx: oamutil.SetNamespaceInCtx(ctx, namespace),
			app:    &v1beta1.Application{},
		}
		c.app.Name, c.app.Namespace = req.Name, namespace
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
