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

	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

func (h *ValidatingHandler) validateExpressionTargetTypes(ctx context.Context, app *v1beta1.Application, blobs []blobPlan,
	t *typing, reported map[string]bool) field.ErrorList {
	var errs field.ErrorList
	targetParams := map[string]*cueStruct{}
	loadTarget := func(kind, defType string) *cueStruct {
		key := kind + "/" + defType
		if pv, ok := targetParams[key]; ok {
			return pv
		}
		pv := h.loadTargetParameter(ctx, app.Namespace, kind, defType, app.GetAnnotations())
		targetParams[key] = pv
		return pv
	}
	for _, bp := range blobs {
		// A source's properties are typed against its SourceDefinition's
		// parameters, in validateSourceInputs.
		if bp.plan == nil || bp.sourceIndex >= 0 {
			continue
		}
		faults, err := t.faults(bp, bp.ctxSchema, loadTarget(bp.targetKind, bp.targetType))
		if err != nil {
			errs = append(errs, field.Invalid(bp.base, string(bp.raw), err.Error()))
			continue
		}
		w := parameterWording(bp.targetDesc)
		for _, lf := range bp.leaves {
			f, faulted := faults[lf.property]
			// A property the source rules refused is not restated in the type
			// check's words.
			if !faulted || lf.path == "" || reported[lf.fieldPath.String()] {
				continue
			}
			errs = append(errs, w.fieldError(lf, f))
		}
	}
	return errs
}

// sourceSchemaTexts maps binding names to their SourceDefinition schema text,
// which is what sentinel typing needs.
func (h *ValidatingHandler) sourceSchemaTexts(ctx context.Context, annotations map[string]string, appNamespace string,
	sourceNameToType map[string]string, schemaValidators map[string]*sourceSchemaValidator) map[string]string {
	out := map[string]string{}
	for name, sourceType := range sourceNameToType {
		if sourceType == "" {
			continue
		}
		sv, ok := schemaValidators[sourceType]
		if !ok {
			var err error
			sv, err = h.loadSourceSchemaValidator(ctx, appNamespace, sourceType, annotations)
			if err != nil {
				continue
			}
			schemaValidators[sourceType] = sv
		}
		if sv == nil || sv.schemaExpr == "" {
			continue
		}
		out[name] = sv.schemaExpr
	}
	return out
}

// policyIsAppScoped reports whether a policy type is an Application-scoped
// PolicyDefinition, which decides both the context it is typed against and
// whether it may read a source at all.
//
// A built-in type never has a definition, so it is answered without a lookup. A
// missing or unreadable definition answers false - the same fail-open every other
// surface check uses, and the definition's own absence is reported elsewhere.
func (h *ValidatingHandler) policyIsAppScoped(ctx context.Context, app *v1beta1.Application, policyType string) bool {
	if appfile.IsBuiltinPolicyType(policyType) {
		return false
	}
	def := &v1beta1.PolicyDefinition{}
	if err := oamutil.GetCapabilityDefinition(ctx, h.Client, def, policyType, app.Annotations); err != nil {
		return false
	}
	return def.Spec.Scope != v1beta1.DefaultScope
}

// policyScopeLookup returns a memoised classifier, so one Application does not
// fetch the same PolicyDefinition once per policy.
func (h *ValidatingHandler) policyScopeLookup(ctx context.Context, app *v1beta1.Application) func(string) bool {
	seen := map[string]bool{}
	return func(policyType string) bool {
		if v, ok := seen[policyType]; ok {
			return v
		}
		v := h.policyIsAppScoped(ctx, app, policyType)
		seen[policyType] = v
		return v
	}
}

// defaultHint tells the author how to defend a possibly-absent read, in the
// syntax the selected engine actually accepts.
//
// Getting this wrong is worse than saying nothing: the CUE disjunction is a parse
// error under CEL, so an author following the message would be told to write
// something the very next admission refuses.
func defaultHint(read string) string {
	return fmt.Sprintf("guard it with has(%s) ? %s : <fallback>", read, read)
}
