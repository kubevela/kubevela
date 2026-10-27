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
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/features"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

const errFmtTraitNotAppliable = "trait %q cannot apply to component %q of type %q (workload %q); appliesToWorkloads: %q"

// appliesToTarget is what a trait's appliesToWorkloads is matched against.
type appliesToTarget struct {
	// definition is the ComponentDefinition name.
	definition string
	// workload is the component's workload as <plural>.<group>, or empty when
	// it has none (autodetects.core.oam.dev) or it could not be resolved.
	workload string
}

// attachedTrait identifies a trait on a component, for telling whether an
// update introduced it.
type attachedTrait struct {
	component, componentType, trait string
}

// ValidateTraitAppliesTo validates TraitDefinition.spec.appliesToWorkloads for
// every trait attached to a component. Like ValidateTraitConflicts it is kept
// outside ValidateComponents so it still runs when sharding skips component
// schematic validation.
//
// oldApp is nil on create. On update, a trait the Application already had on
// the same component is admitted with a warning rather than refused: the field
// went unenforced for a long time, and refusing would leave such an
// Application impossible to edit at all, the controller's own writes included,
// until the trait was removed.
func (h *ValidatingHandler) ValidateTraitAppliesTo(ctx context.Context, app, oldApp *v1beta1.Application) (field.ErrorList, []string) {
	if !utilfeature.DefaultMutableFeatureGate.Enabled(features.ValidateTraitAppliesToWorkloads) {
		return nil, nil
	}
	var errs field.ErrorList
	var warnings []string
	defCtx := oamutil.SetNamespaceInCtx(ctx, app.Namespace)
	existing := attachedTraits(oldApp)

	// Cache resolved definitions across components so each type is fetched once.
	targets := make(map[string]*appliesToTarget)
	traitDefs := make(map[string]*v1beta1.TraitDefinition)

	for compIdx, comp := range app.Spec.Components {
		if len(comp.Traits) == 0 {
			continue
		}
		compPath := field.NewPath("spec", "components").Index(compIdx)
		target, cached := targets[comp.Type]
		if !cached {
			var err error
			if target, err = h.resolveAppliesToTarget(defCtx, app, comp.Type); err != nil {
				klog.Errorf("Failed to resolve ComponentDefinition %q for appliesToWorkloads validation: %v", comp.Type, err)
				errs = append(errs, field.InternalError(compPath.Child("type"), err))
				continue
			}
			targets[comp.Type] = target
		}
		if target == nil {
			continue
		}

		for i, trait := range comp.Traits {
			traitPath := compPath.Child("traits").Index(i).Child("type")
			def, cached := traitDefs[trait.Type]
			if !cached {
				var err error
				if def, err = h.resolveAppliesToTrait(defCtx, app, trait.Type); err != nil {
					// Fail closed, as ValidateTraitConflicts does, so a lookup error
					// cannot be used to get past the check.
					klog.Errorf("Failed to resolve TraitDefinition %q for appliesToWorkloads validation: %v", trait.Type, err)
					errs = append(errs, field.InternalError(traitPath, err))
					continue
				}
				traitDefs[trait.Type] = def
			}
			if def == nil || traitAppliesTo(def.Spec.AppliesToWorkloads, *target) {
				continue
			}
			msg := fmt.Sprintf(errFmtTraitNotAppliable, def.Name, comp.Name, target.definition, target.workload, def.Spec.AppliesToWorkloads)
			if existing[attachedTrait{component: comp.Name, componentType: comp.Type, trait: trait.Type}] {
				klog.Infof("Admitting Application %s/%s with a trait it already had: %s", app.Namespace, app.Name, msg)
				warnings = append(warnings, msg+"; allowed because the Application already had it")
				continue
			}
			errs = append(errs, field.Invalid(traitPath, trait.Type, msg))
		}
	}
	return errs, warnings
}

// attachedTraits lists the traits on each component of app, which may be nil.
func attachedTraits(app *v1beta1.Application) map[attachedTrait]bool {
	if app == nil {
		return nil
	}
	attached := make(map[attachedTrait]bool)
	for _, comp := range app.Spec.Components {
		for _, trait := range comp.Traits {
			attached[attachedTrait{component: comp.Name, componentType: comp.Type, trait: trait.Type}] = true
		}
	}
	return attached
}

// resolveAppliesToTarget looks up the ComponentDefinition behind a component
// type. It returns nil when the type cannot be resolved or does not exist:
// that is reported by other validations, and must not block admission here.
func (h *ValidatingHandler) resolveAppliesToTarget(ctx context.Context, app *v1beta1.Application, compType string) (*appliesToTarget, error) {
	resolved, err := appfile.ResolveModuleType(ctx, h.Client, compType, types.TypeComponentDefinition)
	if err != nil {
		// Invalid or ambiguous type strings are reported by other validations.
		klog.V(4).InfoS("Skipping appliesToWorkloads validation for unresolved component type", "type", compType, "err", err)
		return nil, nil
	}
	cd := &v1beta1.ComponentDefinition{}
	if err := oamutil.GetCapabilityDefinition(ctx, h.Client, cd, resolved, app.GetAnnotations()); err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	merged, err := appfile.InheritedComponentDefinition(ctx, h.Client, cd)
	if err != nil {
		// A broken extends chain is refused at render with a better message.
		klog.Warningf("Skipping appliesToWorkloads validation for component type %q: %v", compType, err)
		return nil, nil
	}
	return &appliesToTarget{definition: merged.Name, workload: h.workloadOf(merged)}, nil
}

// resolveAppliesToTrait looks up a TraitDefinition with what it inherits
// through spec.extends filled in. It returns nil when there is nothing to check
// against, for the same reasons as resolveAppliesToTarget.
func (h *ValidatingHandler) resolveAppliesToTrait(ctx context.Context, app *v1beta1.Application, traitType string) (*v1beta1.TraitDefinition, error) {
	resolved, err := appfile.ResolveModuleType(ctx, h.Client, traitType, types.TypeTrait)
	if err != nil {
		klog.V(4).InfoS("Skipping appliesToWorkloads validation for unresolved trait type", "type", traitType, "err", err)
		return nil, nil
	}
	td := &v1beta1.TraitDefinition{}
	if err := oamutil.GetCapabilityDefinition(ctx, h.Client, td, resolved, app.GetAnnotations()); err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	merged, err := appfile.InheritedTraitDefinition(ctx, h.Client, td)
	if err != nil {
		klog.Warningf("Skipping appliesToWorkloads validation for trait type %q: %v", traitType, err)
		return nil, nil
	}
	return merged, nil
}

// workloadOf returns a ComponentDefinition's workload as <plural>.<group>, or
// empty when it has no single workload type. spec.workload.type is normally
// filled in by the ComponentDefinition mutating webhook; when only the GVK is
// set it is mapped here instead.
func (h *ValidatingHandler) workloadOf(cd *v1beta1.ComponentDefinition) string {
	w := cd.Spec.Workload
	switch {
	case w.Type == types.AutoDetectWorkloadDefinition:
		return ""
	case w.Type != "":
		return w.Type
	case w.Definition == (common.WorkloadGVK{}):
		return ""
	}
	ref, err := oamutil.ConvertWorkloadGVK2Definition(h.Client.RESTMapper(), w.Definition)
	if err != nil {
		klog.Warningf("Cannot map workload %s/%s of ComponentDefinition %q: %v", w.Definition.APIVersion, w.Definition.Kind, cd.Name, err)
		return ""
	}
	return ref.Name
}

// traitAppliesTo reports whether a trait with the given appliesToWorkloads may
// be attached to target. An empty list applies to everything.
func traitAppliesTo(rules []string, target appliesToTarget) bool {
	if len(rules) == 0 {
		return true
	}
	for _, rule := range rules {
		if appliesToRuleMatches(rule, target) {
			return true
		}
	}
	return false
}

// appliesToRuleMatches checks a single appliesToWorkloads entry against a component.
// Supported forms:
//   - "*"                  matches any component
//   - "<definition-name>"  matches the component's ComponentDefinition name
//   - "*.<group>"          matches a workload in exactly that API group
//   - "<plural>.<group>"   matches that workload, e.g. "deployments.apps"
//
// A component with no workload type cannot be checked against the workload
// forms, so any entry containing a dot is taken to match it; only "*" and the
// definition name can restrict it.
func appliesToRuleMatches(rule string, target appliesToTarget) bool {
	switch {
	case rule == "*":
		return true
	case rule == target.definition:
		return true
	case target.workload == "":
		return strings.Contains(rule, ".")
	case strings.HasPrefix(rule, "*."):
		return schema.ParseGroupResource(target.workload).Group == strings.TrimPrefix(rule, "*.")
	default:
		return rule == target.workload
	}
}
