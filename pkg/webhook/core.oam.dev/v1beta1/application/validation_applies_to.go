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
	"github.com/oam-dev/kubevela/pkg/oam"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	webhookutils "github.com/oam-dev/kubevela/pkg/webhook/utils"
)

const errFmtTraitNotAppliable = "trait %q cannot apply to component %q of type %q (workload %q); appliesToWorkloads: %q"

// appliesToTarget is what a trait's appliesToWorkloads is matched against.
type appliesToTarget struct {
	// definition is the ComponentDefinition name.
	definition string
	// workload is <plural>.<group>, or empty when unknown (autodetect, unmapped).
	workload string
}

// traitAttachment identifies a trait on a component, to tell if an update added it.
type traitAttachment struct {
	component, componentType, trait string
}

// ValidateTraitAppliesTo checks each trait against its TraitDefinition's
// appliesToWorkloads. Like ValidateTraitConflicts, it sits outside
// ValidateComponents so it also runs under sharding. oldApp is nil on create;
// on update, a trait the Application already had on the same component only
// warns, so existing Applications stay editable.
func (h *ValidatingHandler) ValidateTraitAppliesTo(ctx context.Context, app, oldApp *v1beta1.Application) (field.ErrorList, []string) {
	if !utilfeature.DefaultMutableFeatureGate.Enabled(features.ValidateTraitAppliesToWorkloads) {
		return nil, nil
	}
	var errs field.ErrorList
	var warnings []string
	defCtx := oamutil.SetNamespaceInCtx(ctx, app.Namespace)
	existing := traitAttachments(oldApp)

	// Resolve each type once, and report a failure where it first appears.
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
				// Fail closed: a lookup error must not let the trait through.
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

// memoize caches f per key, errors included, and reports a key's first call.
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

// traitAttachments lists the traits on app's components. app may be nil.
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

// resolveAppliesToTarget resolves a component type. nil means nothing to check.
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
			return nil, chainFailure(compType, err)
		}
	}
	workload, err := h.workloadOf(ctx, cd)
	if err != nil {
		return nil, err
	}
	return &appliesToTarget{definition: cd.Name, workload: workload}, nil
}

// resolveAppliesToTrait resolves a trait type with its inherited fields. nil
// means nothing to check.
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
			return nil, chainFailure(traitType, err)
		}
	}
	return td, nil
}

// chainFailure returns err only when the API server failed to answer. Other
// extends errors (missing or cross-namespace parent, cycle) are definition
// problems that rendering reports, so the check skips them instead of failing
// every write.
func chainFailure(typ string, err error) error {
	var status apierrors.APIStatus
	if (errors.As(err, &status) && !apierrors.IsNotFound(err)) ||
		errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return err
	}
	klog.V(4).InfoS("Skipping appliesToWorkloads validation for a type whose extends chain does not resolve", "type", typ, "err", err)
	return nil
}

// readDefinition reads the definition a type names into def, resolved as at
// render and retried against the API server on a cache miss. false means it
// doesn't exist; a read error is returned. A type that doesn't resolve is
// skipped, since ValidateAbstractTypes already refuses it in the same phase.
func (h *ValidatingHandler) readDefinition(ctx context.Context, app *v1beta1.Application, typ string, capType types.CapType, def client.Object) (bool, error) {
	resolved, err := appfile.ResolveModuleType(ctx, h.Client, typ, capType)
	if err != nil {
		klog.V(4).InfoS("Skipping appliesToWorkloads validation for a type that does not resolve", "type", typ, "err", err)
		return false, nil
	}
	err = webhookutils.ReadWithLiveRetry(h.Client, h.Live, func(cli client.Client) error {
		return oamutil.GetCapabilityDefinition(ctx, cli, def, resolved, app.GetAnnotations())
	})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

// inheritanceEnabled reports whether spec.extends is on. When it's off,
// rendering refuses extending definitions, so they aren't checked here.
func inheritanceEnabled() bool {
	return utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableDefinitionInheritance)
}

// workloadOf returns cd's workload as <plural>.<group>, or "" if it has none.
// With a GVK, spec.workload.type is the mapped name (mapped here if missing).
// Without one, spec.workload.type names a WorkloadDefinition.
func (h *ValidatingHandler) workloadOf(ctx context.Context, cd *v1beta1.ComponentDefinition) (string, error) {
	w := cd.Spec.Workload
	if w.Type == types.AutoDetectWorkloadDefinition {
		return "", nil
	}
	if w.Definition == (common.WorkloadGVK{}) {
		// Use cd's namespace, as its mutating webhook did, so a same-named
		// WorkloadDefinition in the Application's namespace can't change the result.
		ns := cd.Namespace
		if ns == "" {
			ns = oam.SystemDefinitionNamespace
		}
		return h.workloadDefinitionRef(oamutil.SetNamespaceInCtx(ctx, ns), w.Type)
	}
	if w.Type != "" {
		return w.Type, nil
	}
	ref, err := oamutil.ConvertWorkloadGVK2Definition(h.Client.RESTMapper(), w.Definition)
	if err != nil {
		// An unserved kind (say, a CRD not installed yet) has no workload type.
		// Any other error fails closed.
		if !meta.IsNoMatchError(err) {
			return "", err
		}
		klog.V(2).InfoS("Workload kind is not served, so it has no workload type", "componentDefinition", cd.Name,
			"apiVersion", w.Definition.APIVersion, "kind", w.Definition.Kind, "err", err)
		return "", nil
	}
	return ref.Name, nil
}

// workloadDefinitionRef returns the workload a WorkloadDefinition refers to.
// WorkloadDefinitions are named after their workload, so the name is the fallback.
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

// traitAppliesTo reports whether rules allow a trait on target. An empty list
// allows anything.
func traitAppliesTo(rules []string, target appliesToTarget) bool {
	return len(rules) == 0 || slices.ContainsFunc(rules, func(rule string) bool {
		return appliesToRuleMatches(rule, target)
	})
}

// appliesToRuleMatches checks one appliesToWorkloads entry:
//   - "*"                  any component
//   - "<definition-name>"  that ComponentDefinition
//   - "*.<group>"          any workload in that API group
//   - "<plural>.<group>"   that workload, e.g. "deployments.apps"
//
// A "/<version>" suffix is ignored. When the workload is unknown, any entry
// with a dot matches, so only "*" and definition names restrict.
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
