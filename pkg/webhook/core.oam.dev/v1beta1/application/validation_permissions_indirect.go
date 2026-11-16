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
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	wfv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	authv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/workflow/step"
)

// indirectUse is a definition an Application reaches other than by naming it inline.
type indirectUse struct {
	defType string
	def     reflect.Type
	path    *field.Path
}

// referencedObject is a Workflow or Policy object an Application reads through a reference.
type referencedObject struct {
	kind, resource, namespace, name string
	path                            *field.Path
}

// unreadableReference is a reference whose object could not be read, so what it brings in is
// unknown.
type unreadableReference struct {
	path *field.Path
	err  error
}

// reach is what an Application uses beyond what it names inline.
type reach struct {
	uses       []indirectUse
	objects    []referencedObject
	unreadable []unreadableReference
}

var (
	componentDefinitionType    = reflect.TypeOf(v1beta1.ComponentDefinition{})
	traitDefinitionType        = reflect.TypeOf(v1beta1.TraitDefinition{})
	policyDefinitionType       = reflect.TypeOf(v1beta1.PolicyDefinition{})
	workflowStepDefinitionType = reflect.TypeOf(v1beta1.WorkflowStepDefinition{})
)

// validateIndirectReach checks what the Application reaches through references as if it were
// named inline, and that the requester can read each object it reaches it through: the
// controller reads them with its own identity and runs what they name.
func (h *ValidatingHandler) validateIndirectReach(ctx context.Context, app *v1beta1.Application, req admission.Request) field.ErrorList {
	var errs field.ErrorList
	r := indirectReach(ctx, h.Client, app)
	for _, u := range r.unreadable {
		errs = append(errs, field.InternalError(u.path, u.err))
	}
	for _, obj := range r.objects {
		sar := requesterReview(req, authv1.ResourceAttributes{
			Verb: "get", Group: "core.oam.dev", Version: "v1alpha1", Resource: obj.resource, Namespace: obj.namespace, Name: obj.name,
		})
		switch err := h.Client.Create(ctx, sar); {
		case err != nil:
			errs = append(errs, field.InternalError(obj.path, fmt.Errorf("cannot check access to %s %s/%s: %w", obj.kind, obj.namespace, obj.name, err)))
		case !sar.Status.Allowed:
			errs = append(errs, field.Forbidden(obj.path, fmt.Sprintf("user %q cannot read %s %q in namespace %q, which this field references",
				req.UserInfo.Username, obj.kind, obj.name, obj.namespace)))
		}
	}
	return append(errs, h.permitUses(ctx, req, app.Namespace, r.uses)...)
}

// requesterReview asks whether the admission request's requester, as authenticated, may do attrs.
func requesterReview(req admission.Request, attrs authv1.ResourceAttributes) *authv1.SubjectAccessReview {
	extra := make(map[string]authv1.ExtraValue, len(req.UserInfo.Extra))
	for k, v := range req.UserInfo.Extra {
		extra[k] = authv1.ExtraValue(v)
	}
	return &authv1.SubjectAccessReview{Spec: authv1.SubjectAccessReviewSpec{
		User:               req.UserInfo.Username,
		Groups:             req.UserInfo.Groups,
		UID:                req.UserInfo.UID,
		Extra:              extra,
		ResourceAttributes: &attrs,
	}}
}

// permitUses checks the requester may use each definition, from namespace, reporting each at
// the field that brings it in.
func (h *ValidatingHandler) permitUses(ctx context.Context, req admission.Request, namespace string, uses []indirectUse) field.ErrorList {
	var errs field.ErrorList
	for _, use := range uses {
		defInfo, ok := v1beta1.DefinitionTypeMap[use.def]
		if !ok {
			klog.Errorf("Unknown definition type: %v", use.def)
			continue
		}
		allowed, err := h.checkDefinitionPermission(ctx, req, defInfo.GVR.Resource, use.defType, namespace)
		errs = append(errs, h.processDefinitionPermissionCheck(
			allowed, err, req, defInfo.Kind, use.defType, namespace, []*field.Path{use.path})...)
	}
	return errs
}

// indirectReach lists the definitions the renderer will use that the spec does not name inline,
// each at the field that brings it in: component types and traits an override policy sets, the
// steps of the Workflow spec.workflow.ref names, and the Policy objects a deploy step loads by
// name, with what those set if they are overrides. It reads the referenced objects from the
// Application's namespace, as the controller does, and lists them, so a caller can check the
// requester may read them too.
func indirectReach(ctx context.Context, cli client.Client, app *v1beta1.Application) reach {
	var r reach
	inline := map[string]bool{}
	for i, policy := range app.Spec.Policies {
		inline[policy.Name] = true
		if policy.Type == v1alpha1.OverridePolicyType {
			r.addOverride(policy.Properties, field.NewPath("spec", "policies").Index(i).Child("properties", "components"))
		}
	}
	if wf := app.Spec.Workflow; wf != nil {
		steps := field.NewPath("spec", "workflow", "steps")
		for i, s := range wf.Steps {
			r.addDeployPolicies(ctx, cli, app.Namespace, inline, s, stepPolicyPath(steps.Index(i)))
		}
		if wf.Ref != "" {
			ref := field.NewPath("spec", "workflow", "ref")
			refWorkflow := &wfv1alpha1.Workflow{}
			switch err := cli.Get(ctx, client.ObjectKey{Namespace: app.Namespace, Name: wf.Ref}, refWorkflow); {
			case errors.IsNotFound(err):
				// the render reports a Workflow that is not there
			case err != nil:
				r.unreadable = append(r.unreadable, unreadableReference{ref, fmt.Errorf("cannot read Workflow %q to check the definitions its steps use: %w", wf.Ref, err)})
			default:
				r.objects = append(r.objects, referencedObject{"Workflow", "workflows", app.Namespace, wf.Ref, ref})
				for _, s := range refWorkflow.Steps {
					for _, t := range append([]string{s.Type}, subStepTypes(s)...) {
						if !step.IsBuiltinWorkflowStepType(t) {
							r.uses = append(r.uses, indirectUse{t, workflowStepDefinitionType, ref})
						}
					}
					r.addDeployPolicies(ctx, cli, app.Namespace, inline, s, func(int, int) *field.Path { return ref })
				}
			}
		}
	}
	return r
}

// addOverride adds the component types and traits an override policy's properties set.
func (r *reach) addOverride(properties *runtime.RawExtension, components *field.Path) {
	if properties == nil {
		return
	}
	spec := &v1alpha1.OverridePolicySpec{}
	if err := json.Unmarshal(properties.Raw, spec); err != nil {
		return // the policy's own validation reports it
	}
	for j, comp := range spec.Components {
		if comp.Type != "" {
			r.uses = append(r.uses, indirectUse{comp.Type, componentDefinitionType, components.Index(j).Child("type")})
		}
		for k, tr := range comp.Traits {
			r.uses = append(r.uses, indirectUse{tr.Type, traitDefinitionType, components.Index(j).Child("traits").Index(k).Child("type")})
		}
	}
}

// addDeployPolicies adds the Policy objects a deploy step, or its sub-steps, loads by name: the
// names not declared inline, read from the Application's namespace as the controller reads them.
// pathOf gives the field the name at index j of sub-step sub (-1 for the step itself) is reported at.
func (r *reach) addDeployPolicies(ctx context.Context, cli client.Client, namespace string, inline map[string]bool,
	s wfv1alpha1.WorkflowStep, pathOf func(sub, j int) *field.Path) {
	steps := append([]wfv1alpha1.WorkflowStepBase{s.WorkflowStepBase}, s.SubSteps...)
	for k, st := range steps {
		if st.Type != step.DeployWorkflowStep || st.Properties == nil {
			continue
		}
		props := struct {
			Policies []interface{} `json:"policies"`
		}{}
		if err := json.Unmarshal(st.Properties.Raw, &props); err != nil {
			continue // the step's own validation reports it
		}
		for j, entry := range props.Policies {
			name, ok := entry.(string)
			if !ok || inline[name] || strings.Contains(name, "$(") {
				continue // an expression is resolved at render
			}
			path := pathOf(k-1, j)
			po := &v1alpha1.Policy{}
			switch err := cli.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, po); {
			case errors.IsNotFound(err):
				continue // the render reports a Policy that is not there
			case err != nil:
				r.unreadable = append(r.unreadable, unreadableReference{path, fmt.Errorf("cannot read Policy %q to check the definition it uses: %w", name, err)})
				continue
			}
			r.objects = append(r.objects, referencedObject{"Policy", "policies", namespace, name, path})
			r.uses = append(r.uses, indirectUse{po.Type, policyDefinitionType, path})
			if po.Type == v1alpha1.OverridePolicyType {
				before := len(r.uses)
				r.addOverride(po.Properties, path)
				// An external override's fields are reported at the reference that loads it.
				for k := before; k < len(r.uses); k++ {
					r.uses[k].path = path
				}
			}
		}
	}
}

func subStepTypes(s wfv1alpha1.WorkflowStep) []string {
	var types []string
	for _, sub := range s.SubSteps {
		types = append(types, sub.Type)
	}
	return types
}

// stepPolicyPath reports a deploy policy at the step, or sub-step, that names it.
func stepPolicyPath(step *field.Path) func(sub, j int) *field.Path {
	return func(sub, j int) *field.Path {
		at := step
		if sub >= 0 {
			at = at.Child("subSteps").Index(sub)
		}
		return at.Child("properties", "policies").Index(j)
	}
}
