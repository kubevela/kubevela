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

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/definition/cachekey"
	"github.com/oam-dev/kubevela/pkg/definition/celexpr"
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
// This is the other half of surface compatibility, and the half that is reachable
// today. A SourceDefinition's own template may only read universally-available
// context, so it can be consumed anywhere - but the *Application* can feed a
// source from context, which is how a per-component source is written:
//
//	sources:
//	  - name: own
//	    type: percomp
//	    properties: {component: '$(context.componentName)'}
//
// That binding now only works where componentName exists. Consumed from a
// workflow step, the read has nothing to resolve against - and the failure was
// silent: the step's expressions were left unsubstituted and the literal
// "$(source.own.label)" was written into the rendered resource.
func validateSourceContextReads(app *v1beta1.Application, effective map[string][]string) field.ErrorList {
	var errs field.ErrorList
	for i, src := range app.Spec.Sources {
		if src.Properties == nil || len(src.Properties.Raw) == 0 || src.Name == "" {
			continue
		}
		base := field.NewPath("spec", "sources").Index(i).Child("properties")
		for _, lf := range flattenLeafPaths(src.Properties.Raw, base) {
			text, ok := lf.literal.(string)
			if !ok {
				continue
			}
			parsed, perr := propexpr.Parse(text)
			if perr != nil || !parsed.HasExpr() {
				continue
			}
			for _, fragment := range parsed.Fragments {
				if !fragment.IsExpr() {
					continue
				}
				reads, rerr := celexpr.PropertyReferences(fragment.Expr)
				if rerr != nil {
					continue // reported by validateExpressions
				}
				for _, read := range reads {
					if read.IsSource() || len(read.Path) == 0 {
						continue
					}
					for _, surface := range effective[src.Name] {
						if propexpr.ContextFor(surface).Offers(read.Path[0]) {
							continue
						}
						errs = append(errs, field.Invalid(lf.fieldPath, text,
							contextUnavailableMessage(read.Path[0], surface, src.Name)))
					}
				}
			}
		}
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
