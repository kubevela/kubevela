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

	"k8s.io/apimachinery/pkg/util/validation/field"

	celengine "github.com/kubevela/pkg/cel"

	"github.com/oam-dev/kubevela/pkg/definition/cachekey"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
)

// requiredContext returns the context fields a SourceDefinition's template reads,
// memoised per source type.
func (h *ValidatingHandler) requiredContext(ctx context.Context, appNamespace, sourceType string, annotations map[string]string,
	cache map[string][]string) ([]string, error) {
	if fields, ok := cache[sourceType]; ok {
		return fields, nil
	}
	def, err := h.getSourceDefinition(ctx, appNamespace, sourceType, annotations)
	if err != nil {
		return nil, err
	}
	if def.Spec.Schematic == nil || def.Spec.Schematic.CUE == nil {
		cache[sourceType] = nil
		return nil, nil
	}
	fields, err := cachekey.RequiredContext(def.Spec.Schematic.CUE.Template)
	if err != nil {
		return nil, err
	}
	cache[sourceType] = fields
	return fields, nil
}

// validateSourceContextReads checks the context an Application reads inside
// spec.sources[].properties against the surfaces that consume each binding.
//
// A SourceDefinition's own template may only read universally-available
// context, but an Application can feed a source from context, which is how a
// per-component source is written:
//
//	sources:
//	  - name: own
//	    type: percomp
//	    properties: {component: '$(context.componentName)'}
//
// Such a binding works only where every surface consuming it offers the field.
func validateSourceContextReads(blobs []blobPlan, effective map[string][]string, reported map[string]bool) field.ErrorList {
	var errs field.ErrorList
	for _, bp := range blobs {
		if bp.sourceIndex < 0 || bp.binding == "" || bp.plan == nil {
			continue
		}
		faults := bp.plan.Check(everyRoot, map[string]celengine.Checker{propexpr.ContextIdent: contextChecker(bp, effective)})
		errs = append(errs, bp.fieldErrors(faults, reported)...)
	}
	return errs
}

// contextUnavailableMessage states the field, the surface that lacks it, why that
// surface is being mentioned, and where the field would work.
//
// The last clause is the one that decides what the author does next: move the
// consumption, or stop reading the field. Omitted when nothing offers it, since
// "available in" with an empty list reads as a bug.
func contextUnavailableMessage(field, surface, binding string) string {
	msg := fmt.Sprintf("context.%s is unavailable in %s, where source %q is consumed",
		field, propexpr.SurfacePlural(surface), binding)
	if available := propexpr.SurfacesOffering(field); len(available) > 0 {
		msg += "; it is available in " + strings.Join(available, ", ")
	}
	return msg
}
