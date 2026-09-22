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

// Compilers are the two ways a chain's templates are compiled.
type Compilers struct {
	// Render compiles a level to render it, provider functions and all.
	Render CompileFunc
	// Schema compiles a level only to read its parameter declaration, and must
	// leave provider functions unresolved: that pass supplies no parameters for
	// them to run on.
	Schema CompileFunc
}

// SameCompiler uses one compiler for both passes, for a caller with no side
// effects to avoid.
func SameCompiler(f CompileFunc) Compilers { return Compilers{Render: f, Schema: f} }

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
	if len(chain) == 1 {
		// Nothing is extended, so there is no schema to resolve.
		return renderFrom(ctx, chain, nil, paramFile, contextFile, s, c.Render)
	}
	schemas, err := schemaChain(ctx, chain, contextFile, s, c.Schema)
	if err != nil {
		return nil, err
	}
	return renderFrom(ctx, chain, schemas, paramFile, contextFile, s, c.Render)
}

// schemaChain computes every level's parameter schema once, from the root down,
// since a level may state its own as `$super.parameter & {...}`. Resolving them
// here rather than inside the render keeps it to one compile per level.
//
// schemas[i] is the schema of chain[i:], so a level renders against schemas[i+1].
func schemaChain(ctx context.Context, chain []Level, contextFile string, s Surface, compile CompileFunc) ([]cue.Value, error) {
	// Index 0 is never read: a level renders against schemas[i+1], and the
	// recursion drops its own index each time.
	schemas := make([]cue.Value, len(chain))
	for i := len(chain) - 1; i >= 1; i-- {
		val, compileErr := compile(ctx, join(chain[i].Template, contextFile))
		if !val.Exists() {
			return nil, fmt.Errorf(
				"read the parameter schema of %s %s: %w", s.Kind, chain[i].Name, compileErr)
		}
		if i+1 < len(chain) {
			moved, err := adopt(val, schemas[i+1])
			if err != nil {
				return nil, fmt.Errorf("%s %s: %w", s.Kind, chain[i].Name, err)
			}
			val = val.FillPath(superPath(ParameterField), moved)
		}
		schema := val.LookupPath(cue.ParsePath(ParameterField))
		if schema.Err() != nil {
			return nil, fmt.Errorf(
				"read the parameter schema of %s %s: %w", s.Kind, chain[i].Name, schema.Err())
		}
		if !schema.Exists() {
			// Declaring no parameters is legitimate; an empty struct keeps
			// `$super.parameter` resolvable for whatever extends it.
			schema = val.Context().CompileString("{}")
		}
		schemas[i] = schema
	}
	return schemas, nil
}

// renderFrom renders chain[0] against the rest of the chain. schemas is aligned
// with chain, so chain[0] renders against schemas[1].
func renderFrom(ctx context.Context, chain []Level, schemas []cue.Value, paramFile, contextFile string, s Surface, compile CompileFunc) (*Result, error) {
	self := chain[0]
	parents := chain[1:]

	// Extending nothing: compile as an ordinary template.
	if len(parents) == 0 {
		val, err := compile(ctx, join(self.Template, paramFile, contextFile))
		if err != nil {
			return nil, fmt.Errorf("compile %s %s: %w", s.Kind, self.Name, err)
		}
		return &Result{Value: val, Errs: authoredErrs(val), Levels: []cue.Value{val}}, nil
	}

	parentSchema := schemas[1]

	// Compile with the parent's results absent, to read what the child supplies.
	staged, err := compile(ctx, join(self.Template, paramFile, contextFile))
	if err != nil {
		return nil, fmt.Errorf("compile %s %s: %w", s.Kind, self.Name, err)
	}
	// Asked before the schema is filled in, since filling creates `$super` itself.
	if !staged.LookupPath(cue.ParsePath(SuperField)).Exists() {
		return nil, fmt.Errorf(
			"%s %s extends %s but declares no `$super` block; "+
				"add `$super: {...}` naming the parameters %s should receive",
			s.Kind, self.Name, parents[0].Name, parents[0].Name)
	}

	// Only what the template reads is handed to it: moving values between
	// cue.Contexts is most of what a chain costs.
	reads := superReads(self.Template)
	if wants(reads, ParameterField) {
		adoptedSchema, err := adopt(staged, parentSchema)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", s.Kind, self.Name, err)
		}
		staged = staged.FillPath(superPath(ParameterField), adoptedSchema)
		if err := staged.Err(); err != nil {
			return nil, fmt.Errorf("compile %s %s: %w", s.Kind, self.Name, err)
		}
	}

	superParams := staged.LookupPath(propertiesPath())
	if err := checkNoResultDependency(superParams, self, parents[0], s); err != nil {
		return nil, err
	}

	parentResult, err := renderFrom(ctx, parents, schemas[1:], paramsFile(superParams), contextFile, s, compile)
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
			return nil, fmt.Errorf("%s %s: %w", s.Kind, self.Name, err)
		}
		// The merge below always needs this; `$super` only gets it if the template
		// reads it.
		adoptedParent[field] = moved
		if wants(reads, field) {
			val = val.FillPath(superPath(field), moved)
		}
	}
	if err := val.Err(); err != nil {
		return nil, fmt.Errorf("%s %s: handing %s's result back: %w", s.Kind, self.Name, parents[0].Name, err)
	}

	directives, err := parseDirectives(self)
	if err != nil {
		return nil, err
	}
	val, err = mergeSurfaces(val, adoptedParent, s, directives, self, parents[0])
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

// schemaOf reads parents[0]'s parameter schema: its template compiled with none
// supplied, so the declaration stands as the schema. It recurses, since a level
// may inherit its own with `$super.parameter & {...}`.
func schemaOf(ctx context.Context, parents []Level, contextFile string, s Surface, compile CompileFunc) (cue.Value, error) {
	// Compiling with no parameters errors on every field that wanted one, so the
	// value is judged at `parameter` rather than as a whole.
	val, compileErr := compile(ctx, join(parents[0].Template, contextFile))
	if !val.Exists() {
		return cue.Value{}, fmt.Errorf(
			"read the parameter schema of %s %s: %w", s.Kind, parents[0].Name, compileErr)
	}
	if len(parents) > 1 {
		inherited, err := schemaOf(ctx, parents[1:], contextFile, s, compile)
		if err != nil {
			return cue.Value{}, err
		}
		moved, err := adopt(val, inherited)
		if err != nil {
			return cue.Value{}, fmt.Errorf("%s %s: %w", s.Kind, parents[0].Name, err)
		}
		val = val.FillPath(superPath(ParameterField), moved)
	}
	schema := val.LookupPath(cue.ParsePath(ParameterField))
	if !schema.Exists() || schema.Err() != nil {
		if schema.Err() != nil {
			return cue.Value{}, fmt.Errorf(
				"read the parameter schema of %s %s: %w", s.Kind, parents[0].Name, schema.Err())
		}
		// Declaring no parameters is legitimate; an empty struct keeps
		// `$super.parameter` resolvable.
		return val.Context().CompileString("{}"), nil
	}
	return schema, nil
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
		if mentionsResultField(paramsFile(superParams)) {
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
func paramsFile(params cue.Value) string {
	if !params.Exists() {
		return ParameterField + ": {}"
	}
	return ParameterField + ": " + fmt.Sprintf("%v", params)
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
