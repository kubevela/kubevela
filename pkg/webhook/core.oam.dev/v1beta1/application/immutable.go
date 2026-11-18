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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	wfTypesv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	upstreamcuex "github.com/kubevela/pkg/cue/cuex"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	velacuex "github.com/oam-dev/kubevela/pkg/cue/cuex"
	"github.com/oam-dev/kubevela/pkg/definition/inherit"
	"github.com/oam-dev/kubevela/pkg/oam"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/schema"
)

// ValidateImmutableFields validates that immutable parameter fields (`// +immutable`) have not changed
// All checks are bypassed if the app carries the app.oam.dev/force-mutate: "true" annotation.
func (h *ValidatingHandler) ValidateImmutableFields(ctx context.Context, newApp, oldApp *v1beta1.Application) field.ErrorList {
	if newApp.Annotations[oam.AnnotationForceParamMutations] == "true" {
		return nil
	}
	defCtx := oamutil.SetNamespaceInCtx(ctx, newApp.Namespace)

	var errs field.ErrorList
	errs = append(errs, h.validateComponentImmutableFields(defCtx, newApp, oldApp)...)
	errs = append(errs, h.validatePolicyImmutableFields(defCtx, newApp, oldApp)...)
	errs = append(errs, h.validateWorkflowStepImmutableFields(defCtx, newApp, oldApp)...)
	return errs
}

// validateComponentImmutableFields checks immutability for components and their traits.
func (h *ValidatingHandler) validateComponentImmutableFields(ctx context.Context, newApp, oldApp *v1beta1.Application) field.ErrorList {
	oldByName := make(map[string]common.ApplicationComponent, len(oldApp.Spec.Components))
	for _, c := range oldApp.Spec.Components {
		oldByName[c.Name] = c
	}

	var errs field.ErrorList
	for i, newComp := range newApp.Spec.Components {
		oldComp, exists := oldByName[newComp.Name]
		if !exists || oldComp.Type != newComp.Type {
			continue
		}

		tmpl, err := appfile.LoadTemplate(ctx, h.Client, newComp.Type, types.TypeComponentDefinition, newApp.Annotations)
		if err != nil {
			klog.V(4).Infof("immutable check: skipping component %q (type %q): %v", newComp.Name, newComp.Type, err)
			continue
		}
		fp := field.NewPath("spec", "components").Index(i).Child("properties")
		errs = append(errs, checkImmutableParams(tmpl.TemplateStr, fp, oldComp.Properties, newComp.Properties)...)
		errs = append(errs, checkAncestorImmutableParams(ctx, newComp.Type, tmpl, fp, oldComp.Properties, newComp.Properties)...)
		errs = append(errs, h.validateTraitImmutableFields(ctx, newApp, i, newComp.Traits, oldComp.Traits)...)
	}
	return errs
}

// validateTraitImmutableFields checks immutability for the traits on a single component.
func (h *ValidatingHandler) validateTraitImmutableFields(ctx context.Context, app *v1beta1.Application, compIdx int, newTraits, oldTraits []common.ApplicationTrait) field.ErrorList {
	var errs field.ErrorList
	for j, newTrait := range newTraits {
		if j >= len(oldTraits) || oldTraits[j].Type != newTrait.Type {
			continue
		}
		oldTrait := oldTraits[j]
		tmpl, err := appfile.LoadTemplate(ctx, h.Client, newTrait.Type, types.TypeTrait, app.Annotations)
		if err != nil {
			klog.V(4).Infof("immutable check: skipping trait %q on component[%d]: %v", newTrait.Type, compIdx, err)
			continue
		}
		fp := field.NewPath("spec", "components").Index(compIdx).Child("traits").Index(j).Child("properties")
		errs = append(errs, checkImmutableParams(tmpl.TemplateStr, fp, oldTrait.Properties, newTrait.Properties)...)
		errs = append(errs, checkAncestorImmutableParams(ctx, newTrait.Type, tmpl, fp, oldTrait.Properties, newTrait.Properties)...)
	}
	return errs
}

// validatePolicyImmutableFields checks immutability for policies.
func (h *ValidatingHandler) validatePolicyImmutableFields(ctx context.Context, newApp, oldApp *v1beta1.Application) field.ErrorList {
	oldByName := make(map[string]v1beta1.AppPolicy, len(oldApp.Spec.Policies))
	for _, p := range oldApp.Spec.Policies {
		oldByName[p.Name] = p
	}

	var errs field.ErrorList
	for i, newPolicy := range newApp.Spec.Policies {
		oldPolicy, exists := oldByName[newPolicy.Name]
		if !exists || oldPolicy.Type != newPolicy.Type {
			continue
		}
		tmpl, err := appfile.LoadTemplate(ctx, h.Client, newPolicy.Type, types.TypePolicy, newApp.Annotations)
		if err != nil {
			klog.V(4).Infof("immutable check: skipping policy %q (type %q): %v", newPolicy.Name, newPolicy.Type, err)
			continue
		}
		fp := field.NewPath("spec", "policies").Index(i).Child("properties")
		errs = append(errs, checkImmutableParams(tmpl.TemplateStr, fp, oldPolicy.Properties, newPolicy.Properties)...)
	}
	return errs
}

// validateWorkflowStepImmutableFields checks immutability for workflow steps.
func (h *ValidatingHandler) validateWorkflowStepImmutableFields(ctx context.Context, newApp, oldApp *v1beta1.Application) field.ErrorList {
	if newApp.Spec.Workflow == nil || oldApp.Spec.Workflow == nil {
		return nil
	}

	oldByName := indexWorkflowStepsByName(oldApp.Spec.Workflow.Steps)

	var errs field.ErrorList
	for i, newStep := range newApp.Spec.Workflow.Steps {
		oldStep, exists := oldByName[newStep.Name]
		if !exists || oldStep.Type != newStep.Type {
			continue
		}
		tmpl, err := appfile.LoadTemplate(ctx, h.Client, newStep.Type, types.TypeWorkflowStep, newApp.Annotations)
		if err != nil {
			klog.V(4).Infof("immutable check: skipping workflow step %q (type %q): %v", newStep.Name, newStep.Type, err)
			continue
		}
		fp := field.NewPath("spec", "workflow", "steps").Index(i).Child("properties")
		errs = append(errs, checkImmutableParams(tmpl.TemplateStr, fp, oldStep.Properties, newStep.Properties)...)
	}
	return errs
}

// checkImmutableParams extracts immutable field paths from templateStr and verifies
// that those fields have not changed between oldProps and newProps
func checkImmutableParams(templateStr string, fp *field.Path, oldProps, newProps *runtime.RawExtension) field.ErrorList {
	var errs field.ErrorList
	for _, v := range immutableViolations(templateStr, rawExtensionToMap(oldProps), rawExtensionToMap(newProps)) {
		if v.removed {
			errs = append(errs, field.Forbidden(fp.Key(v.path),
				fmt.Sprintf("immutable field cannot be removed (current: %s)", jsonString(v.old))))
			continue
		}
		errs = append(errs, field.Forbidden(fp.Key(v.path),
			fmt.Sprintf("immutable field cannot be changed (current: %s, new: %s)", jsonString(v.old), jsonString(v.new))))
	}
	return errs
}

// checkAncestorImmutableParams applies the +immutable parameters of each
// definition the type extends to what that definition receives through
// `$super: properties`, for the old properties and the new. A child forwards or
// renames its parent's parameters, so the field the Application changes is not
// always the one the parent marked. Best effort, as the definition's own check
// is: a chain that cannot be evaluated here is left to the render.
func checkAncestorImmutableParams(ctx context.Context, name string, tmpl *appfile.Template, fp *field.Path, oldProps, newProps *runtime.RawExtension) field.ErrorList {
	if tmpl == nil || len(tmpl.Ancestors) == 0 {
		return nil
	}
	chain := append([]inherit.Level{{Name: name, Template: tmpl.TemplateStr}}, tmpl.Ancestors...)
	const contextFile = "context: _"
	oldReceived, err := inherit.AncestorProperties(ctx, chain, paramFileOf(oldProps), contextFile, compileForImmutable)
	if err != nil {
		klog.V(4).Infof("immutable check: skipping the definitions %q extends: %v", name, err)
		return nil
	}
	newReceived, err := inherit.AncestorProperties(ctx, chain, paramFileOf(newProps), contextFile, compileForImmutable)
	if err != nil {
		klog.V(4).Infof("immutable check: skipping the definitions %q extends: %v", name, err)
		return nil
	}
	var errs field.ErrorList
	for i, anc := range tmpl.Ancestors {
		for _, v := range immutableViolations(anc.Template, jsonToMap(oldReceived[i]), jsonToMap(newReceived[i])) {
			if v.removed {
				errs = append(errs, field.Forbidden(fp, fmt.Sprintf(
					"%s's immutable parameter %s cannot be removed (current: %s)", anc.Name, v.path, jsonString(v.old))))
				continue
			}
			errs = append(errs, field.Forbidden(fp, fmt.Sprintf(
				"%s's immutable parameter %s cannot be changed (current: %s, new: %s)",
				anc.Name, v.path, jsonString(v.old), jsonString(v.new))))
		}
	}
	return errs
}

// immutableViolation is one +immutable field that was removed or changed.
type immutableViolation struct {
	path     string
	removed  bool
	old, new any
}

// immutableViolations compares old and new parameters on the fields a template
// marks +immutable. A field not set before may be set now.
func immutableViolations(templateStr string, oldMap, newMap map[string]any) []immutableViolation {
	immutableFields := schema.ImmutableFieldsFromTemplate(templateStr)
	if len(immutableFields) == 0 {
		return nil
	}
	var out []immutableViolation
	for fieldPath := range immutableFields {
		oldVal, oldOK := getNestedValue(oldMap, fieldPath)
		newVal, newOK := getNestedValue(newMap, fieldPath)
		if !oldOK {
			// Field was not set before — first-time population is allowed
			continue
		}
		if !newOK {
			out = append(out, immutableViolation{path: fieldPath, removed: true, old: oldVal})
			continue
		}
		if !jsonEqual(oldVal, newVal) {
			out = append(out, immutableViolation{path: fieldPath, old: oldVal, new: newVal})
		}
	}
	return out
}

// paramFileOf is an Application's properties as the parameter file a chain is
// evaluated with.
func paramFileOf(props *runtime.RawExtension) string {
	if props == nil || len(props.Raw) == 0 {
		return inherit.ParameterField + ": {}"
	}
	return inherit.ParameterField + ": " + string(props.Raw)
}

func jsonToMap(raw []byte) map[string]any {
	m := map[string]any{}
	_ = json.Unmarshal(raw, &m)
	return m
}

// compileForImmutable compiles a level of a chain with provider functions off,
// so the check has no side effects.
func compileForImmutable(ctx context.Context, src string) (cue.Value, error) {
	val, err := velacuex.WorkloadCompiler.Get().CompileStringWithOptions(ctx, src, upstreamcuex.DisableResolveProviderFunctions{})
	if err != nil {
		return cue.Value{}, err
	}
	return val, val.Err()
}

// getNestedValue retrieves a value from a nested map using a dotted path.
// Returns (value, true) if found, (nil, false) if any segment is missing.
func getNestedValue(m map[string]any, path string) (any, bool) {
	parts := strings.SplitN(path, ".", 2)
	val, ok := m[parts[0]]
	if !ok {
		return nil, false
	}
	if len(parts) == 1 {
		return val, true
	}
	nested, ok := val.(map[string]any)
	if !ok {
		return nil, false
	}
	return getNestedValue(nested, parts[1])
}

// rawExtensionToMap decodes a *runtime.RawExtension into a map[string]any.
// Returns nil on nil input or parse error.
func rawExtensionToMap(ext *runtime.RawExtension) map[string]any {
	if ext == nil || len(ext.Raw) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(ext.Raw, &m); err != nil {
		return nil
	}
	return m
}

// jsonString returns a compact JSON representation of v, or "<unknown>" on error.
func jsonString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "<unknown>"
	}
	return string(b)
}

// jsonEqual reports whether two values are equal by comparing their JSON encodings.
func jsonEqual(a, b any) bool {
	aJSON, err := json.Marshal(a)
	if err != nil {
		return false
	}
	bJSON, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return bytes.Equal(aJSON, bJSON)
}

// indexWorkflowStepsByName returns a map of workflow steps indexed by name.
func indexWorkflowStepsByName(steps []wfTypesv1alpha1.WorkflowStep) map[string]wfTypesv1alpha1.WorkflowStep {
	byName := make(map[string]wfTypesv1alpha1.WorkflowStep, len(steps))
	for _, step := range steps {
		byName[step.Name] = step
	}
	return byName
}
