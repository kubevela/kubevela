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
	"strings"

	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"

	"cuelang.org/go/cue"
	cueast "cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	cueformat "cuelang.org/go/cue/format"
	cueparser "cuelang.org/go/cue/parser"
	upstreamcuex "github.com/kubevela/pkg/cue/cuex"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velacue "github.com/oam-dev/kubevela/pkg/cue"
	velacuex "github.com/oam-dev/kubevela/pkg/cue/cuex"
	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1beta1/sourcedefinition"
)

// loadConsumableFrom returns the surfaces a SourceDefinition may be consumed
// from, memoised per source type. Nil means unrestricted.
func (h *ValidatingHandler) loadConsumableFrom(ctx context.Context, appNamespace, sourceType string, cache map[string][]string, annotations map[string]string) ([]string, error) {
	if surfaces, ok := cache[sourceType]; ok {
		return surfaces, nil
	}
	def, err := h.getSourceDefinition(ctx, appNamespace, sourceType, annotations)
	if err != nil {
		return nil, err
	}
	var surfaces []string
	if def.Spec.Schematic != nil && def.Spec.Schematic.CUE != nil {
		surfaces, err = sourcedefinition.ParseConsumableFrom(def.Spec.Schematic.CUE.Template)
		if err != nil {
			return nil, err
		}
	}
	cache[sourceType] = surfaces
	return surfaces, nil
}

func (h *ValidatingHandler) loadSourceSchemaValidator(ctx context.Context, appNamespace, sourceType string, annotations map[string]string) (*sourceSchemaValidator, error) {
	def, err := h.getSourceDefinition(ctx, appNamespace, sourceType, annotations)
	if err != nil {
		return nil, err
	}
	if def.Spec.Schematic == nil || def.Spec.Schematic.CUE == nil {
		return nil, nil
	}
	schemaExpr, err := extractSourceSchemaExprForAdmission(def.Spec.Schematic.CUE.Template)
	if err != nil {
		return nil, err
	}
	if schemaExpr == "" {
		return nil, nil
	}
	v := cuecontext.New().CompileString("schema: " + schemaExpr)
	if v.Err() != nil {
		return nil, v.Err()
	}
	schema := v.LookupPath(cue.ParsePath("schema"))
	if !schema.Exists() {
		return nil, nil
	}
	return &sourceSchemaValidator{schema: schema, schemaExpr: schemaExpr}, nil
}

// loadSourceParameter returns a validator over the SourceDefinition's top-level
// parameter: block, or nil if the definition declares no parameter block.
func (h *ValidatingHandler) loadSourceParameter(ctx context.Context, appNamespace, sourceType string, annotations map[string]string) (*cueStruct, error) {
	def, err := h.getSourceDefinition(ctx, appNamespace, sourceType, annotations)
	if err != nil {
		return nil, err
	}
	if def.Spec.Schematic == nil || def.Spec.Schematic.CUE == nil {
		return nil, nil
	}
	// The whole reduction, not just the parameter field: a definition may write
	// `parameter: #Params` against a top-level #Params, and extracting the field
	// alone left a reference to something that was no longer there. The template
	// then would not compile and every binding of the source was rejected.
	src, ok := extractParameterBlock(def.Spec.Schematic.CUE.Template)
	if !ok || strings.TrimSpace(src) == "" {
		return nil, nil
	}
	v := cuecontext.New().CompileString(src)
	if v.Err() != nil {
		return nil, v.Err()
	}
	param := v.LookupPath(cue.ParsePath("parameter"))
	if !param.Exists() {
		return nil, nil
	}
	return &cueStruct{root: param}, nil
}

// loadTargetParameter returns a validator over the parameter: block of a
// ComponentDefinition (kind "component") or TraitDefinition (kind "trait"),
// used to type-check expression-fed values against the consuming parameter.
// Best-effort by design: any failure - definition not found, template that does
// not compile statically, no parameter block - yields nil so validation fails
// open rather than blocking a legitimate apply. There is no error to return.
func (h *ValidatingHandler) loadTargetParameter(ctx context.Context, appNamespace, kind, defName string,
	annotations map[string]string) *cueStruct {
	tmpl, ok := h.getDefinitionTemplate(ctx, appNamespace, kind, defName, annotations)
	if !ok || strings.TrimSpace(tmpl) == "" {
		return nil
	}
	// Only the `parameter:` block is wanted, so only that is compiled.
	//
	// Compiling the whole template needs every package it imports registered with
	// the compiler in hand, and SourceCompiler carries the packages a source may
	// but not vela/multicluster or vela/builtin. Reducing to the parameter block
	// keeps the check independent of which providers happen to be loaded.
	if param, ok := parameterBlockOnly(tmpl); ok {
		return param
	}

	// Fall back to compiling the whole template: a `parameter` that references
	// something else in the file needs the file.
	val, err := velacuex.SourceCompiler.Get().CompileStringWithOptions(
		ctx, tmpl+velacue.BaseTemplate, upstreamcuex.DisableResolveProviderFunctions{})
	if err != nil || val.Err() != nil {
		klog.V(4).Infof("skip target parameter type check for %s %q: template did not compile statically", kind, defName)
		return nil
	}
	param := val.LookupPath(cue.ParsePath("parameter"))
	if !param.Exists() {
		return nil
	}
	return &cueStruct{root: param}
}

// parameterBlockOnly extracts a template's `parameter:` declaration and compiles
// it on its own, without the imports or the body around it.
//
// A definition's parameter block is a plain type declaration - it is the
// contract an author writes against - so it almost never needs the rest of the
// file. Compiling it alone makes the type check independent of which providers
// the compiler happens to hold, which is what stopped it running for workflow
// steps at all.
func parameterBlockOnly(tmpl string) (*cueStruct, bool) {
	src, ok := parameterBlockSource(tmpl)
	if !ok {
		return nil, false
	}
	val := cuecontext.New().CompileBytes([]byte(src))
	if val.Err() != nil {
		return nil, false
	}
	param := val.LookupPath(cue.ParsePath("parameter"))
	if !param.Exists() {
		return nil, false
	}
	return &cueStruct{root: param}, true
}

// parameterBlockExtract is one template's reduced parameter source, or the fact
// that it has none.
type parameterBlockExtract struct {
	src string
	ok  bool
}

// parameterBlockSource reduces a template to its `parameter:` declaration plus
// any top-level definitions that might reference it.
func parameterBlockSource(tmpl string) (string, bool) {
	if hit, loaded := parameterBlockSources.Load(tmpl); loaded {
		got := hit.(parameterBlockExtract)
		return got.src, got.ok
	}
	src, ok := extractParameterBlock(tmpl)
	parameterBlockSources.Store(tmpl, parameterBlockExtract{src: src, ok: ok})
	return src, ok
}

func extractParameterBlock(tmpl string) (string, bool) {
	file, err := cueparser.ParseFile("-", tmpl, cueparser.ParseComments)
	if err != nil || file == nil {
		return "", false
	}

	// The parameter field, plus any top-level definitions it might reference.
	var keep []cueast.Decl
	found := false
	for _, decl := range file.Decls {
		field, ok := decl.(*cueast.Field)
		if !ok {
			continue
		}
		name, _, lerr := cueast.LabelName(field.Label)
		if lerr != nil {
			continue
		}
		switch {
		case name == "parameter":
			keep = append(keep, decl)
			found = true
		case strings.HasPrefix(name, "#"):
			keep = append(keep, decl)
		}
	}
	if !found {
		return "", false
	}

	src, ferr := cueformat.Node(&cueast.File{Decls: keep})
	if ferr != nil {
		return "", false
	}
	return string(src), true
}

// getDefinitionTemplate fetches a Component/Trait definition (app namespace with
// system-namespace fallback) and returns its CUE template string.
// A version-pinned type - webservice@v1 - names a DefinitionRevision rather than
// the live definition, so GetCapabilityDefinition is what resolves it, exactly as
// the render path does. GetDefinition looks for an object literally called
// "webservice@v1", finds none, and loadTargetParameter then fails open - so the
// type check was skipped and a mismatched expression reached render.
func (h *ValidatingHandler) getDefinitionTemplate(ctx context.Context, appNamespace, kind, defName string,
	annotations map[string]string) (string, bool) {
	lookupCtx := oamutil.SetNamespaceInCtx(ctx, appNamespace)
	switch kind {
	case "component":
		def := &v1beta1.ComponentDefinition{}
		if err := oamutil.GetCapabilityDefinition(lookupCtx, h.Client, def, defName, annotations); err != nil {
			return "", false
		}
		if def.Spec.Schematic == nil || def.Spec.Schematic.CUE == nil {
			return "", false
		}
		return def.Spec.Schematic.CUE.Template, true
	case "trait":
		def := &v1beta1.TraitDefinition{}
		if err := oamutil.GetCapabilityDefinition(lookupCtx, h.Client, def, defName, annotations); err != nil {
			return "", false
		}
		if def.Spec.Schematic == nil || def.Spec.Schematic.CUE == nil {
			return "", false
		}
		return def.Spec.Schematic.CUE.Template, true
	case "workflowstep":
		def := &v1beta1.WorkflowStepDefinition{}
		if err := oamutil.GetCapabilityDefinition(lookupCtx, h.Client, def, defName, annotations); err != nil {
			return "", false
		}
		if def.Spec.Schematic == nil || def.Spec.Schematic.CUE == nil {
			return "", false
		}
		return def.Spec.Schematic.CUE.Template, true
	case "policy":
		def := &v1beta1.PolicyDefinition{}
		if err := oamutil.GetCapabilityDefinition(lookupCtx, h.Client, def, defName, annotations); err != nil {
			return "", false
		}
		if def.Spec.Schematic == nil || def.Spec.Schematic.CUE == nil {
			return "", false
		}
		return def.Spec.Schematic.CUE.Template, true
	}
	return "", false
}

// getSourceDefinition resolves a binding's type to its definition, honouring a
// pinned revision - `type: atlas@v1` - exactly as the render path does.
//
// Through GetCapabilityDefinition rather than a direct Get, because a direct Get
// looks for an object literally named "atlas@v1", finds nothing, and reports the
// type as undeclared. Admission would then reject a pinned binding that renders
// perfectly well. It also brings the full namespace search - app, the configured
// x-definition namespace, vela-system, then cluster-scoped for old clusters -
// which the two-namespace lookup here only approximated.
//
// annotations carries the Application's, because autoUpdate widens `@v1` to the
// latest revision in that range. Passing them keeps admission checking the same
// revision the render will use.
func (h *ValidatingHandler) getSourceDefinition(ctx context.Context, appNamespace, sourceType string,
	annotations map[string]string) (*v1beta1.SourceDefinition, error) {
	def := &v1beta1.SourceDefinition{}
	lookupCtx := oamutil.SetNamespaceInCtx(ctx, appNamespace)
	if err := oamutil.GetCapabilityDefinition(lookupCtx, h.Client, def, sourceType, annotations); err != nil {
		if errors.IsNotFound(err) {
			return nil, errors.NewNotFound(schema.GroupVersionResource{
				Group: v1beta1.Group, Version: v1beta1.Version, Resource: "sourcedefinitions"}.GroupResource(), sourceType)
		}
		return nil, err
	}
	return def, nil
}
