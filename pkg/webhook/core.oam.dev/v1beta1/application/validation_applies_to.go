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
	"errors"
	"fmt"
	"slices"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/features"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	webhookutils "github.com/oam-dev/kubevela/pkg/webhook/utils"
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

// traitAttachment identifies a trait on a component, for telling whether an
// update introduced it.
type traitAttachment struct {
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
	existing := traitAttachments(oldApp)

	// Each type is resolved once per request, failures included, so a failing
	// type is reported once, where it is first used.
	resolveTarget := memoize(func(compType string) (*appliesToTarget, error) {
		return h.resolveAppliesToTarget(defCtx, app, compType)
	})
	resolveTrait := memoize(func(traitType string) (*v1beta1.TraitDefinition, error) {
		return h.resolveAppliesToTrait(defCtx, app, traitType)
	})

	for compIdx, comp := range app.Spec.Components {
		if len(comp.Traits) == 0 {
			continue
		}
		compPath := field.NewPath("spec", "components").Index(compIdx)
		target, first, err := resolveTarget(comp.Type)
		if err != nil {
			if first {
				klog.Errorf("Failed to resolve ComponentDefinition %q for appliesToWorkloads validation: %v", comp.Type, err)
				errs = append(errs, field.InternalError(compPath.Child("type"), err))
			}
			continue
		}
		if target == nil {
			continue
		}

		for i, trait := range comp.Traits {
			traitPath := compPath.Child("traits").Index(i).Child("type")
			def, first, err := resolveTrait(trait.Type)
			if err != nil {
				// Fail closed, so a lookup error cannot be used to get past the check.
				if first {
					klog.Errorf("Failed to resolve TraitDefinition %q for appliesToWorkloads validation: %v", trait.Type, err)
					errs = append(errs, field.InternalError(traitPath, err))
				}
				continue
			}
			if def == nil || traitAppliesTo(def.Spec.AppliesToWorkloads, *target) {
				continue
			}
			msg := fmt.Sprintf(errFmtTraitNotAppliable, def.Name, comp.Name, target.definition, target.workload, def.Spec.AppliesToWorkloads)
			if existing[traitAttachment{component: comp.Name, componentType: comp.Type, trait: trait.Type}] {
				klog.V(2).Infof("Admitting Application %s/%s with a trait it already had: %s", app.Namespace, app.Name, msg)
				warnings = append(warnings, msg+"; allowed because the Application already had it")
				continue
			}
			errs = append(errs, field.Invalid(traitPath, trait.Type, msg))
		}
	}
	return errs, warnings
}

// memoize caches f's result, error included, per key. The returned func also
// reports whether this call was the first for its key.
func memoize[T any](f func(string) (T, error)) func(string) (T, bool, error) {
	type result struct {
		value T
		err   error
	}
	cache := make(map[string]result)
	return func(key string) (T, bool, error) {
		if r, ok := cache[key]; ok {
			return r.value, false, r.err
		}
		value, err := f(key)
		cache[key] = result{value: value, err: err}
		return value, true, err
	}
}

// traitAttachments lists the traits on each component of app, which may be nil.
func traitAttachments(app *v1beta1.Application) map[traitAttachment]bool {
	if app == nil {
		return nil
	}
	attached := make(map[traitAttachment]bool)
	for _, comp := range app.Spec.Components {
		for _, trait := range comp.Traits {
			attached[traitAttachment{component: comp.Name, componentType: comp.Type, trait: trait.Type}] = true
		}
	}
	return attached
}

// resolveAppliesToTarget looks up the ComponentDefinition behind a component
// type. It returns nil when there is nothing to check against.
func (h *ValidatingHandler) resolveAppliesToTarget(ctx context.Context, app *v1beta1.Application, compType string) (*appliesToTarget, error) {
	cd := &v1beta1.ComponentDefinition{}
	found, err := h.readDefinition(ctx, app, compType, types.TypeComponentDefinition, cd)
	if !found || err != nil {
		return nil, err
	}
	if cd.Spec.Extends != "" {
		if !inheritanceEnabled() {
			return nil, nil
		}
		if cd, err = appfile.InheritedComponentDefinition(ctx, h.Client, cd, app.GetAnnotations()); err != nil {
			return nil, ignoreNotFound(err)
		}
	}
	workload, err := h.workloadOf(ctx, cd)
	if err != nil {
		return nil, err
	}
	return &appliesToTarget{definition: cd.Name, workload: workload}, nil
}

// resolveAppliesToTrait looks up a TraitDefinition with what it inherits
// through spec.extends filled in. It returns nil when there is nothing to check
// against.
func (h *ValidatingHandler) resolveAppliesToTrait(ctx context.Context, app *v1beta1.Application, traitType string) (*v1beta1.TraitDefinition, error) {
	td := &v1beta1.TraitDefinition{}
	found, err := h.readDefinition(ctx, app, traitType, types.TypeTrait, td)
	if !found || err != nil {
		return nil, err
	}
	if td.Spec.Extends != "" {
		if !inheritanceEnabled() {
			return nil, nil
		}
		if td, err = appfile.InheritedTraitDefinition(ctx, h.Client, td, app.GetAnnotations()); err != nil {
			return nil, ignoreNotFound(err)
		}
	}
	return td, nil
}

// readDefinition reads the definition a component or trait type names into def,
// the way isAbstract does: resolved as the render path resolves it, and retried
// against the API server when the cache has not caught up. It reports false for
// a definition that does not exist, which fails elsewhere with a message that
// says so. Any other failure is returned, so it cannot be used to get past the
// check.
func (h *ValidatingHandler) readDefinition(ctx context.Context, app *v1beta1.Application, typ string, capType types.CapType, def client.Object) (bool, error) {
	resolved, err := appfile.ResolveModuleType(ctx, h.Client, typ, capType)
	if err != nil {
		if errors.Is(err, appfile.ErrNoDefinition) {
			return false, nil
		}
		return false, err
	}
	err = webhookutils.ReadWithLiveRetry(h.Client, h.Live, func(cli client.Client) error {
		return oamutil.GetCapabilityDefinition(ctx, cli, def, resolved, app.GetAnnotations())
	})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

// inheritanceEnabled reports whether spec.extends is read at all. While it is
// off, a definition that sets it is refused at render, so it is not checked here.
func inheritanceEnabled() bool {
	return utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableDefinitionInheritance)
}

// workloadOf returns a ComponentDefinition's workload as <plural>.<group>, or
// empty when it has no single workload type.
//
// With a GVK, spec.workload.type is the mapped name, filled in by the
// ComponentDefinition mutating webhook, or mapped here when it is missing.
// Without one, spec.workload.type names a WorkloadDefinition, whose
// definitionRef holds the workload.
func (h *ValidatingHandler) workloadOf(ctx context.Context, cd *v1beta1.ComponentDefinition) (string, error) {
	w := cd.Spec.Workload
	if w.Type == types.AutoDetectWorkloadDefinition {
		return "", nil
	}
	if w.Definition == (common.WorkloadGVK{}) {
		return h.workloadDefinitionRef(ctx, w.Type)
	}
	if w.Type != "" {
		return w.Type, nil
	}
	ref, err := oamutil.ConvertWorkloadGVK2Definition(h.Client.RESTMapper(), w.Definition)
	if err != nil {
		// A kind the cluster does not serve, such as a CRD not installed yet, has
		// no workload to compare against. Any other failure, such as discovery
		// being unavailable, leaves the answer unknown and fails closed.
		if !meta.IsNoMatchError(err) {
			return "", err
		}
		klog.Warningf("Cannot map workload %s/%s of ComponentDefinition %q: %v", w.Definition.APIVersion, w.Definition.Kind, cd.Name, err)
		return "", nil
	}
	return ref.Name, nil
}

// workloadDefinitionRef returns the workload a WorkloadDefinition refers to.
// WorkloadDefinitions are conventionally named after their workload, so the name
// itself is used when there is none or it does not say.
func (h *ValidatingHandler) workloadDefinitionRef(ctx context.Context, name string) (string, error) {
	if name == "" {
		return "", nil
	}
	wd := &v1beta1.WorkloadDefinition{}
	if err := oamutil.GetDefinition(ctx, h.Client, wd, name); err != nil {
		if apierrors.IsNotFound(err) {
			return name, nil
		}
		return "", err
	}
	if wd.Spec.Reference.Name == "" {
		return name, nil
	}
	return wd.Spec.Reference.Name, nil
}

// traitAppliesTo reports whether a trait with the given appliesToWorkloads may
// be attached to target. An empty list applies to everything.
func traitAppliesTo(rules []string, target appliesToTarget) bool {
	return len(rules) == 0 || slices.ContainsFunc(rules, func(rule string) bool {
		return appliesToRuleMatches(rule, target)
	})
}

// appliesToRuleMatches checks a single appliesToWorkloads entry against a component.
// Supported forms:
//   - "*"                  matches any component
//   - "<definition-name>"  matches the component's ComponentDefinition name
//   - "*.<group>"          matches a workload in exactly that API group
//   - "<plural>.<group>"   matches that workload, e.g. "deployments.apps"
//
// A "/<version>" suffix, which the API field's documentation shows, is
// ignored: a workload is matched by resource and group only.
//
// A component with no workload type cannot be checked against the workload
// forms, so any entry containing a dot is taken to match it; only "*" and the
// definition name can restrict it.
func appliesToRuleMatches(rule string, target appliesToTarget) bool {
	rule, _, _ = strings.Cut(rule, "/")
	switch {
	case rule == "*" || rule == target.definition:
		return true
	case target.workload == "":
		return strings.Contains(rule, ".")
	case strings.HasPrefix(rule, "*."):
		return schema.ParseGroupResource(target.workload).Group == strings.TrimPrefix(rule, "*.")
	default:
		return rule == target.workload
	}
}
