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

package inherit

import (
	"context"
	"fmt"
	"strings"

	"cuelang.org/go/cue"
)

const (
	// SuperField is the call site.
	SuperField = "$super"
	// PropertiesField is what a child supplies to its parent, under `$super`,
	// named for the Application's own `properties`.
	PropertiesField = "properties"
	// ParameterField is what a template declares its parameters under.
	ParameterField = "parameter"
	// ErrsField is where a template states why it refused.
	ErrsField = "errs"
)

// superPath is where the chain fills `$super`: the parent's schema and results.
func superPath(field string) cue.Path {
	return cue.MakePath(cue.Str(SuperField), cue.Str(field))
}

// propertiesPath is where a child states what its parent receives.
func propertiesPath() cue.Path {
	return cue.MakePath(cue.Str(SuperField), cue.Str(PropertiesField))
}

// Surface names the fields a kind's render engine reads, in merge order. It is
// all that differs between a component and a trait here.
type Surface struct {
	// Kind names the sort of definition, for error messages.
	Kind string
	// Fields are the surface fields, merged parent-first.
	Fields []string
}

var (
	// ComponentSurface is what a ComponentDefinition's render reads.
	ComponentSurface = Surface{Kind: "component", Fields: []string{"output", "outputs"}}
	// TraitSurface is what a TraitDefinition's render reads, `processing`
	// included, since traitDef.Complete acts on it.
	TraitSurface = Surface{Kind: "trait", Fields: []string{"patch", "patchOutputs", "outputs", "processing"}}
)

// Level is one definition in an inheritance chain.
type Level struct {
	// Name is the definition's name, used to attribute errors.
	Name string
	// Template is the CUE the definition was authored with.
	Template string
}

// CompileFunc compiles one CUE file. Callers pass their render path's own
// compiler, so an inherited template sees the same provider packages.
type CompileFunc func(ctx context.Context, src string) (cue.Value, error)

// Compilers is how a chain's templates are compiled.
type Compilers struct {
	// Render compiles a level to render it, provider functions and all.
	Render CompileFunc
}

// SameCompiler is the plain case, kept so callers read the same as before.
func SameCompiler(f CompileFunc) Compilers { return Compilers{Render: f} }

// Result is what a chain renders to.
type Result struct {
	// Value is the child's template with `$super` populated and the surfaces
	// merged, read exactly as one that extended nothing.
	Value cue.Value
	// Errs are the authored `errs:` from every level, root first.
	Errs []string
	// Levels is each level's own compiled value, root first. A merged value
	// drops the doc comments its fields came with, so a caller needing one, such
	// as a trait's `+patchStrategy`, asks the level that declared it.
	Levels []cue.Value
}

// Render renders a chain, child first and root last. A chain of one renders as
// an ordinary template, so callers need not branch.
func Render(ctx context.Context, chain []Level, paramFile, contextFile string, s Surface, c Compilers) (*Result, error) {
	if len(chain) == 0 {
		return nil, fmt.Errorf("render %s: empty inheritance chain", s.Kind)
	}
	return renderFrom(ctx, chain, paramFile, contextFile, s, c.Render, nil)
}

// chainPath names a level by the route taken to reach it, leaf first, as in
// `tenant-webservice:webservice`.
//
// A level is compiled with what the level below supplied, so its errors are
// often the lower one's doing. Naming it alone sends the author to a file they
// did not write; the route says which definition failed and which one called it.
func chainPath(trail []string, name string) string {
	if len(trail) == 0 {
		return name
	}
	return strings.Join(append(append([]string{}, trail...), name), ":")
}

// renderFrom renders chain[0] against the rest of the chain. trail is the route
// from the leaf to chain[0], for attributing errors.
func renderFrom(ctx context.Context, chain []Level, paramFile, contextFile string, s Surface, compile CompileFunc, trail []string) (*Result, error) {
	self := chain[0]
	parents := chain[1:]
	at := chainPath(trail, self.Name)
	below := append(append([]string{}, trail...), self.Name)

	// Extending nothing: compile as an ordinary template.
	if len(parents) == 0 {
		val, err := compile(ctx, join(self.Template, paramFile, contextFile))
		if err != nil {
			return nil, fmt.Errorf("compile %s %s: %w", s.Kind, at, err)
		}
		return &Result{Value: val, Errs: authoredErrs(val), Levels: []cue.Value{val}}, nil
	}

	// Compile with the parent's results absent, to read what the child supplies.
	staged, err := compile(ctx, join(self.Template, paramFile, contextFile))
	if err != nil {
		return nil, fmt.Errorf("compile %s %s: %w", s.Kind, at, err)
	}
	// Asked before the schema is filled in, since filling creates `$super` itself.
	if !staged.LookupPath(cue.ParsePath(SuperField)).Exists() {
		return nil, fmt.Errorf(
			"%s %s extends %s but declares no `$super` block; "+
				"add `$super: {...}` naming the parameters %s should receive",
			s.Kind, at, parents[0].Name, parents[0].Name)
	}

	reads := superReads(self.Template)

	superParams := staged.LookupPath(propertiesPath())
	if err := checkNoResultDependency(superParams, self, parents[0], s); err != nil {
		return nil, err
	}

	ownParams := staged.LookupPath(cue.ParsePath(ParameterField))
	parentResult, err := renderFrom(ctx, parents,
		paramsFile(superParams, ownParams), contextFile, s, compile, below)
	if err != nil {
		return nil, err
	}

	// Hand the parent's results back down, then merge the surfaces.
	val := staged
	adoptedParent := map[string]cue.Value{}
	for _, field := range s.Fields {
		got := parentResult.Value.LookupPath(cue.ParsePath(field))
		if !got.Exists() {
			continue
		}
		moved, err := adopt(val, got)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", s.Kind, at, err)
		}
		// The merge below always needs this; `$super` only gets it if the template
		// reads it.
		adoptedParent[field] = moved
		if wants(reads, field) {
			val = val.FillPath(superPath(field), moved)
		}
	}
	if err := val.Err(); err != nil {
		return nil, fmt.Errorf("%s %s: handing %s's result back: %w", s.Kind, at, parents[0].Name, err)
	}

	directives, err := parseDirectives(self)
	if err != nil {
		return nil, err
	}
	val, err = mergeSurfaces(val, adoptedParent, s, directives, parents[0], at)
	if err != nil {
		return nil, err
	}

	return &Result{
		Value: val,
		Errs:  append(parentResult.Errs, authoredErrs(val)...),
		// This level's own value, before its surfaces were merged onto the
		// parent's, so a caller reading a field's doc comments sees the ones it
		// was declared with.
		Levels: append(parentResult.Levels, staged),
	}, nil
}

// checkNoResultDependency refuses a `$super` block that depends on what the
// parent produced: properties travel up before results come back, so it cannot
// be evaluated yet and would surface far from its cause.
func checkNoResultDependency(superParams cue.Value, self, parent Level, s Surface) error {
	err := superParams.Err()
	if err == nil {
		// CUE resolves lazily, so a block referring to something not filled yet
		// is not an error on the value: it survives into the text handed to the
		// parent, whose own compile then fails on an unresolved `$super` and says
		// nothing about where it came from. What travels up is what to inspect.
		if mentionsResultField(wholeParamsText(superParams)) {
			return fmt.Errorf(
				"%s %s: its `$super` block depends on what %s produced, which is not "+
					"available until %s has rendered. Properties travel up the chain before "+
					"results travel back down, so `$super` may only reference `parameter` and `context`",
				s.Kind, self.Name, parent.Name, parent.Name)
		}
		return nil
	}
	if mentionsResultField(err.Error()) {
		return fmt.Errorf(
			"%s %s: its `$super` block depends on what %s produced, which is not "+
				"available until %s has rendered. Properties travel up the chain before "+
				"results travel back down, so `$super` may only reference `parameter` and `context`: %w",
			s.Kind, self.Name, parent.Name, parent.Name, err)
	}
	// Any other error is fatal too, and reported here where it can be attributed
	// to the block that holds it.
	return fmt.Errorf("%s %s: what it supplies to %s: %w", s.Kind, self.Name, parent.Name, err)
}

func allResultFields() []string {
	fields := append([]string{}, ComponentSurface.Fields...)
	fields = append(fields, TraitSurface.Fields...)
	return fields
}

func mentionsResultField(msg string) bool {
	for _, f := range allResultFields() {
		// Qualified only. A bare "output" or "patch" appears in plenty of errors
		// that have nothing to do with reading the parent's result.
		if strings.Contains(msg, SuperField+"."+f) {
			return true
		}
	}
	return false
}

// paramsFile renders what a child supplies into its parent's file. Supplying
// nothing is not an error here; that is the parent's schema's business.
//
// A field with nothing behind it is left out rather than rendered. The text is
// compiled inside the parent's file, where `parameter` means the parent's own
// block, so a field still holding a reference such as `parameter.tag` would be
// re-read there and quietly pick up whatever the parent happens to call `tag`.
// Leaving it out says what the child meant: it has nothing to supply, so the
// parent's own declaration stands.
func paramsFile(params, ownParams cue.Value) string {
	if !params.Exists() {
		return ParameterField + ": {}"
	}
	kept, ok := resolvedFields(params, ownParams)
	if !ok {
		return wholeParamsText(params)
	}
	return ParameterField + ": " + fmt.Sprintf("%v", kept)
}

// wholeParamsText renders the block as the child wrote it, nothing left out,
// for the checks that judge what it says rather than what it supplies.
func wholeParamsText(params cue.Value) string {
	if !params.Exists() {
		return ParameterField + ": {}"
	}
	return ParameterField + ": " + fmt.Sprintf("%v", params)
}

// resolvedFields drops the fields of a struct that had nothing to supply. A
// field that is merely non-concrete is kept: a forwarded `*1 | int` is a real
// answer, and the parent is entitled to the default.
func resolvedFields(params, ownParams cue.Value) (cue.Value, bool) {
	it, err := params.Fields(cue.All())
	if err != nil {
		return cue.Value{}, false
	}
	kept := params.Context().CompileString("{}")
	for it.Next() {
		if it.Value().Err() != nil {
			if !suppliesNothing(it.Value(), ownParams) {
				// Not simply empty: reading the parent's results, or naming a
				// parameter that was never declared. Both are the child's
				// mistake and both are reported, so the field travels and the
				// complaint names it.
				return cue.Value{}, false
			}
			continue
		}
		kept = kept.FillPath(cue.MakePath(it.Selector()), it.Value())
		if kept.Err() != nil {
			return cue.Value{}, false
		}
	}
	return kept, true
}

// suppliesNothing reports whether a field came up empty because it forwards a
// parameter the child declares and nothing set.
//
// That is the one case worth passing over in silence: the child said to hand
// its `tag` up, it has no tag, so the parent's own declaration stands. Naming a
// parameter that was never declared reads the same way to CUE and is a typo, so
// it is left in to be complained about.
func suppliesNothing(field, ownParams cue.Value) bool {
	ref, ok := forwardedParameter(fmt.Sprintf("%v", field))
	if !ok {
		return false
	}
	// Walked rather than looked up: the case this exists for is an optional
	// parameter nobody set, and LookupPath does not return optional fields.
	it, err := ownParams.Fields(cue.All())
	if err != nil {
		return false
	}
	for it.Next() {
		if strings.TrimSuffix(it.Selector().String(), "?") == ref {
			return true
		}
	}
	return false
}

// forwardedParameter reads the name out of a field left holding `parameter.x`.
func forwardedParameter(text string) (string, bool) {
	const prefix = ParameterField + "."
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, prefix) {
		return "", false
	}
	name := text[len(prefix):]
	if name == "" || strings.ContainsAny(name, " \t\n.[]{}()&|") {
		return "", false
	}
	return name, true
}

func join(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, "\n")
}

// authoredErrs reads a template's `errs:` field, the way a definition says why
// it refused in its own words.
func authoredErrs(val cue.Value) []string {
	errs := val.LookupPath(cue.ParsePath(ErrsField))
	if !errs.Exists() {
		return nil
	}
	var out []string
	if err := errs.Decode(&out); err != nil {
		return nil
	}
	kept := out[:0]
	for _, e := range out {
		if strings.TrimSpace(e) != "" {
			kept = append(kept, e)
		}
	}
	return kept
}
