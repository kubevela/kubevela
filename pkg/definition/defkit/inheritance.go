/*
Copyright 2025 The KubeVela Authors.

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

package defkit

import (
	"context"
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"

	"github.com/oam-dev/kubevela/pkg/definition/inherit"
)

// inheritance is what a component or trait declares about the definition it
// extends.
type inheritance struct {
	extends  string
	abstract bool
	// reference marks a placeholder for a definition installed elsewhere, made
	// by ComponentRef or TraitRef. It is extended, never registered.
	reference bool
}

// GetExtends returns the name, and pinned revision if any, of the definition
// this one extends.
func (i *inheritance) GetExtends() string { return i.extends }

// IsAbstract reports whether the definition may only be extended, not used.
func (i *inheritance) IsAbstract() bool { return i.abstract }

// writeInheritance writes extends and abstract as attributes.
func writeInheritance(sb *strings.Builder, i inheritance, indent string) {
	if i.extends != "" {
		sb.WriteString(fmt.Sprintf("%sextends: %q\n", indent, i.extends))
	}
	if i.abstract {
		sb.WriteString(fmt.Sprintf("%sabstract: true\n", indent))
	}
}

// setInheritanceSpec copies extends and abstract into a ToYAML spec.
func setInheritanceSpec(spec map[string]any, i inheritance) {
	if i.extends != "" {
		spec["extends"] = i.extends
	}
	if i.abstract {
		spec["abstract"] = true
	}
}

// --- ComponentDefinition ---

// Extends makes the component render on top of parent. The template calls the
// parent through Super, and its output and outputs merge onto the parent's.
// Parameters are not inherited: declare the ones this component takes.
func (c *ComponentDefinition) Extends(parent *ComponentDefinition) *ComponentDefinition {
	c.parent = parent
	c.inheritance.extends = parent.GetName()
	return c
}

// ComponentRef refers to a component installed elsewhere, such as a built-in,
// for Extends. The name may pin a revision, as "webservice@v3", and is written
// into extends as given. Supply the parent's CUE with WithCUE to have Register
// check the call and EvaluateCUE render the chain; without it, both are left to
// the cluster.
func ComponentRef(name string) *ComponentDefinition {
	c := NewComponent(name)
	c.inheritance.reference = true
	return c
}

// WithCUE gives a ComponentRef the CUE of the definition it refers to. It is a
// copy for checking and testing; the cluster resolves the installed one.
func (c *ComponentDefinition) WithCUE(src string) *ComponentDefinition {
	if !c.reference {
		panic(fmt.Sprintf("defkit: WithCUE is for a ComponentRef; component %s is a definition of its own, so use RawCUE", c.GetName()))
	}
	c.setRawCUE(src)
	return c
}

// Abstract marks the component as one to extend, which an Application may not
// name directly.
func (c *ComponentDefinition) Abstract() *ComponentDefinition {
	c.inheritance.abstract = true
	return c
}

// GetParent returns the component this one extends, or nil when it extends
// nothing or only a name.
func (c *ComponentDefinition) GetParent() *ComponentDefinition { return c.parent }

// hasDeclaredWorkload reports whether the component names its own workload,
// rather than taking its parent's.
func (c *ComponentDefinition) hasDeclaredWorkload() bool {
	w := c.workload
	return w.autodetect || w.apiVersion != "" || w.kind != ""
}

// validateDefinition refuses to register a reference, and checks the call to a
// parent with a template the way admission does.
func (c *ComponentDefinition) validateDefinition() error {
	if c.reference {
		return errReference("ComponentRef", c.GetName())
	}
	if c.parent == nil || !hasTemplate(c.parent) {
		return nil
	}
	return checkCall(c, c.parent, inherit.ComponentSurface)
}

// componentChain returns the component and every typed ancestor, child first.
func componentChain(c *ComponentDefinition) ([]inherit.Level, error) {
	var chain []inherit.Level
	for d := c; d != nil; d = d.parent {
		if !hasTemplate(d) {
			return nil, fmt.Errorf("%s extends %s, which has no template here to render it on; supply it with ComponentRef(%q).WithCUE(...)",
				c.GetName(), d.GetName(), d.GetName())
		}
		level, err := levelOf(d)
		if err != nil {
			return nil, err
		}
		chain = append(chain, level)
	}
	return chain, nil
}

// --- TraitDefinition ---

// Extends makes the trait render on top of parent. The template calls the
// parent through Super, and its patch, patchOutputs, outputs and processing
// merge onto the parent's. Parameters are not inherited.
func (t *TraitDefinition) Extends(parent *TraitDefinition) *TraitDefinition {
	t.parent = parent
	t.inheritance.extends = parent.GetName()
	return t
}

// TraitRef refers to a trait installed elsewhere, for Extends. The name may pin
// a revision, as "labels@v2". Supply the parent's CUE with WithCUE to have
// Register check the call.
func TraitRef(name string) *TraitDefinition {
	t := NewTrait(name)
	t.inheritance.reference = true
	return t
}

// WithCUE gives a TraitRef the CUE of the definition it refers to. It is a copy
// for checking and testing; the cluster resolves the installed one.
func (t *TraitDefinition) WithCUE(src string) *TraitDefinition {
	if !t.reference {
		panic(fmt.Sprintf("defkit: WithCUE is for a TraitRef; trait %s is a definition of its own, so use RawCUE", t.GetName()))
	}
	t.setRawCUE(src)
	return t
}

// Abstract marks the trait as one to extend, which an Application may not
// name directly.
func (t *TraitDefinition) Abstract() *TraitDefinition {
	t.inheritance.abstract = true
	return t
}

// GetParent returns the trait this one extends, or nil when it extends
// nothing or only a name.
func (t *TraitDefinition) GetParent() *TraitDefinition { return t.parent }

// validateDefinition refuses to register a reference, and checks the call to a
// parent with a template the way admission does.
func (t *TraitDefinition) validateDefinition() error {
	if t.reference {
		return errReference("TraitRef", t.GetName())
	}
	if t.parent == nil || !hasTemplate(t.parent) {
		return nil
	}
	return checkCall(t, t.parent, inherit.TraitSurface)
}

// --- Template ---

// SuperCall is what a child passes its parent, written as `$super: properties`
// and checked against the parent's parameters.
type SuperCall struct {
	res *Resource
}

// Set passes a value for one of the parent's parameters. Dotted paths set a
// field of a struct parameter.
func (s *SuperCall) Set(path string, v Value) *SuperCall {
	s.res.Set(path, v)
	return s
}

// SetIf passes a value only when the condition holds.
func (s *SuperCall) SetIf(cond Condition, path string, v Value) *SuperCall {
	s.res.SetIf(cond, path, v)
	return s
}

// Super calls the parent of a definition that extends one. Every child calls
// it, even with nothing to pass.
func (t *Template) Super() *SuperCall {
	if t.super == nil {
		t.super = &SuperCall{res: &Resource{}}
	}
	return t.super
}

// Inherit turns merging onto the parent's output wholly on or off. Off, the
// child produces only what it writes, and reads the parent through SuperOutput.
func (t *Template) Inherit(enabled bool) *Template {
	t.inheritAll = &enabled
	return t
}

// InheritField turns merging on or off for one surface field, such as
// "outputs" or "patch".
func (t *Template) InheritField(field string, enabled bool) *Template {
	if t.inheritFields == nil {
		t.inheritFields = map[string]bool{}
	}
	t.inheritFields[field] = enabled
	return t
}

// SuperOutput reads the parent's rendered output at a dotted path, or the
// whole output when path is empty.
func SuperOutput(path string) *Ref { return superRef("output", path) }

// SuperOutputs reads one of the parent's rendered outputs at a dotted path.
func SuperOutputs(name, path string) *Ref { return superRef("outputs."+cueLabel(name), path) }

// SuperPatch reads the parent trait's patch at a dotted path.
func SuperPatch(path string) *Ref { return superRef("patch", path) }

func superRef(field, path string) *Ref {
	ref := inherit.SuperField + "." + field
	if path != "" {
		ref += "." + path
	}
	return Reference(ref)
}

// writeInheritanceBlock writes the template's `$super` call and `$inherit`.
func (g *CUEGenerator) writeInheritanceBlock(sb *strings.Builder, tpl *Template, depth int) {
	indent := strings.Repeat(g.indent, depth)
	if s := tpl.super; s != nil {
		if len(s.res.Ops()) == 0 {
			sb.WriteString(fmt.Sprintf("%s%s: %s: {}\n", indent, inherit.SuperField, inherit.PropertiesField))
		} else {
			sb.WriteString(fmt.Sprintf("%s%s: %s: {\n", indent, inherit.SuperField, inherit.PropertiesField))
			g.writeFieldTree(sb, g.buildFieldTree(s.res.Ops()), depth+1)
			sb.WriteString(fmt.Sprintf("%s}\n", indent))
		}
	}
	if tpl.inheritAll != nil {
		sb.WriteString(fmt.Sprintf("%s$inherit: %v\n", indent, *tpl.inheritAll))
	}
	for _, field := range sortedKeys(tpl.inheritFields) {
		sb.WriteString(fmt.Sprintf("%s$inherit: %s: %v\n", indent, cueLabel(field), tpl.inheritFields[field]))
	}
}

// --- Rendering and checking a chain ---

// errReference refuses to register a placeholder for an installed definition.
func errReference(constructor, name string) error {
	return fmt.Errorf("%s(%q) refers to a definition installed elsewhere, so it is extended, not registered; register the definition that extends it", constructor, name)
}

// hasTemplate reports whether a parent can be rendered here: a definition built
// with defkit always can, a reference only once given its CUE.
func hasTemplate(def interface {
	isReference() bool
	HasRawCUE() bool
}) bool {
	return !def.isReference() || def.HasRawCUE()
}

func (i *inheritance) isReference() bool { return i.reference }

// cueOf returns the CUE a chain level is taken from: a reference's as supplied,
// any other definition's as generated.
func cueOf(def Definition) string {
	if r, ok := def.(interface {
		isReference() bool
		GetRawCUE() string
	}); ok && r.isReference() {
		return r.GetRawCUE()
	}
	return def.ToCue()
}

// levelOf returns a definition as one level of a chain: its name and the
// template the controller stores for it.
func levelOf(def Definition) (inherit.Level, error) {
	body, err := templateBody(cueOf(def))
	if err != nil {
		return inherit.Level{}, fmt.Errorf("%s %s: %w", def.DefType(), def.DefName(), err)
	}
	return inherit.Level{Name: def.DefName(), Template: body}, nil
}

// templateBody returns the imports and the contents of a generated
// definition's template block, which is what a render compiles.
func templateBody(src string) (string, error) {
	f, err := parser.ParseFile("definition.cue", src)
	if err != nil {
		return "", err
	}
	var imports, body []ast.Decl
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.ImportDecl:
			imports = append(imports, d)
		case *ast.Field:
			if name, _, _ := ast.LabelName(d.Label); name != "template" {
				continue
			}
			st, ok := d.Value.(*ast.StructLit)
			if !ok {
				return "", fmt.Errorf("template is not a struct")
			}
			body = append(body, st.Elts...)
		}
	}
	bs, err := format.Node(&ast.File{Decls: append(imports, body...)})
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

// compileOpen compiles a template as admission does, with `context` and
// `parameter` opened so a template reading them compiles on its own.
func compileOpen(_ context.Context, src string) (cue.Value, error) {
	v := cuecontext.New().CompileString(src + "\ncontext: _\nparameter: _\n")
	return v, v.Err()
}

// compileLevel compiles one level of a render, whose parameter and context
// files are already joined to it.
func compileLevel(_ context.Context, src string) (cue.Value, error) {
	v := cuecontext.New().CompileString(src)
	return v, v.Err()
}

// checkCall runs the admission check on a child's call to its parent.
func checkCall(child, parent Definition, s inherit.Surface) error {
	self, err := levelOf(child)
	if err != nil {
		return err
	}
	up, err := levelOf(parent)
	if err != nil {
		return err
	}
	_, err = inherit.CheckCall(context.Background(), []inherit.Level{self, up}, s, compileOpen)
	return err
}
