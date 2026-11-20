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
	"strconv"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velamulticluster "github.com/oam-dev/kubevela/pkg/multicluster"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/sources"
)

// validatePostDispatchReads refuses a read of a resource that a post-dispatch
// trait outputs. Those are applied only once the workflow has finished, and a
// reader waiting on one stops the workflow finishing. That holds for a read in
// the reader's own post-dispatch trait too: post-dispatch traits are not applied
// producer first, and one renders during the workflow once its component is
// healthy.
//
// Only a trait resource named in the template's own outputs blocks can be
// judged; a name built by interpolation is not knowable here and is let through.
func (h *ValidatingHandler) validatePostDispatchReads(ctx context.Context, app *v1beta1.Application) field.ErrorList {
	var errs field.ErrorList
	lookupCtx := oamutil.SetNamespaceInCtx(ctx, app.Namespace)
	defs := map[string]*v1beta1.TraitDefinition{}
	definition := func(traitType string) *v1beta1.TraitDefinition {
		if def, seen := defs[traitType]; seen {
			return def
		}
		def := &v1beta1.TraitDefinition{}
		if err := oamutil.GetCapabilityDefinition(lookupCtx, h.Client, def, traitType, app.GetAnnotations()); err != nil {
			def = nil
		}
		defs[traitType] = def
		return def
	}
	lateOutputs := map[string]map[string]string{} // producer -> output name -> trait type
	late := func(producer string) map[string]string {
		if outputs, ok := lateOutputs[producer]; ok {
			return outputs
		}
		outputs := map[string]string{}
		for _, c := range app.Spec.Components {
			if c.Name != producer {
				continue
			}
			for _, tr := range c.Traits {
				def := definition(tr.Type)
				if def == nil || def.Spec.Stage != v1beta1.PostDispatch || def.Spec.Schematic == nil || def.Spec.Schematic.CUE == nil {
					continue
				}
				for _, name := range declaredOutputs(def.Spec.Schematic.CUE.Template) {
					outputs[name] = tr.Type
				}
			}
		}
		lateOutputs[producer] = outputs
		return outputs
	}

	for i, c := range app.Spec.Components {
		reads, err := sources.ComponentReads(c)
		if err != nil {
			continue
		}
		for _, r := range reads {
			if len(r.Path) < 2 || r.Path[0] != "outputs" {
				continue
			}
			if traitType, ok := late(r.Producer)[r.Path[1]]; ok {
				errs = append(errs, field.Invalid(readPath(i, r), r.String(),
					fmt.Sprintf("component %q reads %s, which post-dispatch trait %q outputs; it is applied only after "+
						"the workflow finishes, so the read would wait for ever", c.Name, r, traitType)))
			}
		}
	}
	return errs
}

// declaredOutputs is the resource names a template's outputs blocks declare,
// read from its syntax: a template depends on parameters that do not exist at
// admission, so it cannot be evaluated here. A block or name under an `if` counts,
// since that is how a trait declares an optional output; a name built by
// interpolation cannot be known and is left out.
func declaredOutputs(template string) []string {
	file, err := parser.ParseFile("template", template)
	if err != nil {
		return nil
	}
	var names []string
	walkOutputs(file.Decls, &names)
	return names
}

// walkOutputs finds the outputs fields among declarations, descending into
// comprehensions.
func walkOutputs(decls []ast.Decl, names *[]string) {
	for _, decl := range decls {
		switch x := decl.(type) {
		case *ast.Field:
			if labelName(x.Label) == "outputs" {
				collectLabels(x.Value, names)
			}
		case *ast.Comprehension:
			if lit, ok := x.Value.(*ast.StructLit); ok {
				walkOutputs(lit.Elts, names)
			}
		}
	}
}

// collectLabels gathers the static labels of a struct literal, descending into
// comprehensions. The shorthand `outputs: svc: {...}` parses as the same
// literal.
func collectLabels(v ast.Expr, names *[]string) {
	lit, ok := v.(*ast.StructLit)
	if !ok {
		return
	}
	for _, elt := range lit.Elts {
		switch x := elt.(type) {
		case *ast.Field:
			if name := labelName(x.Label); name != "" {
				*names = append(*names, name)
			}
		case *ast.Comprehension:
			collectLabels(x.Value, names)
		}
	}
}

func labelName(l ast.Label) string {
	switch x := l.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.BasicLit:
		if s, err := strconv.Unquote(x.Value); err == nil {
			return s
		}
	}
	return ""
}

// validateReadClusters refuses a read naming a cluster that is not registered:
// the reader would wait for a placement that can never happen, and a typo is
// the usual cause. A lookup that fails for any other reason is let through, as
// a registered cluster may simply be unreachable from here.
func (h *ValidatingHandler) validateReadClusters(ctx context.Context, app *v1beta1.Application) field.ErrorList {
	var errs field.ErrorList
	known := map[string]bool{}
	registered := func(cluster string) bool {
		if ok, seen := known[cluster]; seen {
			return ok
		}
		_, err := velamulticluster.GetVirtualCluster(velamulticluster.ContextInLocalCluster(ctx), h.Client, cluster)
		// IsClusterNotExists cannot take a nil error.
		known[cluster] = err == nil || !velamulticluster.IsClusterNotExists(err)
		return known[cluster]
	}
	for i, c := range app.Spec.Components {
		reads, err := sources.ComponentReads(c)
		if err != nil {
			continue
		}
		for _, r := range reads {
			target, err := r.Target()
			if err != nil || target.Cluster == "" || target.Cluster == velamulticluster.ClusterLocalName {
				continue
			}
			if !registered(target.Cluster) {
				errs = append(errs, field.Invalid(readPath(i, r), r.String(),
					fmt.Sprintf("component %q reads %s, but no cluster %q is registered", c.Name, r, target.Cluster)))
			}
		}
	}
	return errs
}

// readPath is the properties a read was written in: the component's own, or one
// of its traits'.
func readPath(component int, r sources.ComponentRead) *field.Path {
	p := field.NewPath("spec", "components").Index(component)
	if r.Trait >= 0 {
		return p.Child("traits").Index(r.Trait).Child("properties")
	}
	return p.Child("properties")
}
