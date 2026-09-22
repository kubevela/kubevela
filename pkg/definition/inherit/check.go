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
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
)

// CheckCall validates a child's `$super` block against its parent's schema.
//
// It runs at admission, where properties are expressions over `parameter` and
// cannot be evaluated, so only the shape of the call is checked. Fatal problems
// come back as an error, advisory ones as warnings.
//
// alsoRead are the definition's healthPolicy, customStatus and details, so a
// parameter read only there is not reported as unused.
func CheckCall(ctx context.Context, chain []Level, s Surface, compile CompileFunc, alsoRead ...string) ([]string, error) {
	if len(chain) < 2 {
		return nil, nil
	}
	self, parent := chain[0], chain[1]

	schema, err := schemaOf(ctx, chain[1:], "", s, compile)
	if err != nil {
		return nil, err
	}

	// A compile can hand back a value that exists and still carries an error, and
	// judging a contract against one reports failures that are really just the
	// error, or passes a call that never compiled.
	child, compileErr := compile(ctx, self.Template)
	if !child.Exists() {
		return nil, fmt.Errorf("%s %s: %w", s.Kind, self.Name, compileErr)
	}
	if compileErr != nil {
		return nil, fmt.Errorf("%s %s: %w", s.Kind, self.Name, compileErr)
	}
	if err := child.Err(); err != nil {
		return nil, fmt.Errorf("%s %s: %w", s.Kind, self.Name, err)
	}

	// Asked before the schema is filled in, since filling creates `$super`.
	if !child.LookupPath(cue.ParsePath(SuperField)).Exists() {
		return nil, fmt.Errorf(
			"%s %s extends %s but its template never calls it; "+
				"add a `$super: properties: {...}` block naming what %s should receive",
			s.Kind, self.Name, parent.Name, parent.Name)
	}

	// The schema must be in place first, or `$super: properties: parameter`
	// supplies almost nothing and every required parameter looks unsupplied.
	moved, err := adopt(child, schema)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", s.Kind, self.Name, err)
	}
	child = child.FillPath(superPath(ParameterField), moved)
	call := child.LookupPath(propertiesPath())

	passed, err := fieldNames(call)
	if err != nil {
		return nil, fmt.Errorf("%s %s: reading its `$super` block: %w", s.Kind, self.Name, err)
	}
	declared, required, open, err := describeSchema(schema)
	if err != nil {
		return nil, fmt.Errorf("%s %s: reading the parameters of %s: %w", s.Kind, self.Name, parent.Name, err)
	}

	var missing []string
	for _, field := range required {
		if !passed[field] {
			missing = append(missing, field)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf(
			"%s %s calls %s without %s, which %s requires and gives no default; "+
				"pass %s in its `$super` block",
			s.Kind, self.Name, parent.Name, quoteList(missing), parent.Name,
			map[bool]string{true: "them", false: "it"}[len(missing) > 1])
	}

	// Usually a typo, but CUE accepts it on an open struct and a parent may read
	// parameters it never named, so it is said rather than refused.
	var warnings []string
	if !open && !forwardsWholesale(self.Template) {
		var unknown []string
		for field := range passed {
			if !declared[field] {
				unknown = append(unknown, field)
			}
		}
		if len(unknown) > 0 {
			sort.Strings(unknown)
			warnings = append(warnings, fmt.Sprintf(
				"%s %s passes %s to %s, which does not declare %s",
				s.Kind, self.Name, quoteList(unknown), parent.Name,
				map[bool]string{true: "them", false: "it"}[len(unknown) > 1]))
		}
	}

	// Catches a value of the wrong type, and a field a closed parent refuses.
	if err := unifiable(call, schema); err != nil {
		return warnings, fmt.Errorf(
			"%s %s calls %s with parameters it will not accept: %w",
			s.Kind, self.Name, parent.Name, err)
	}

	if inert := inertParameters(ctx, chain, s, compile, passed, alsoRead); len(inert) > 0 {
		plural := map[bool]string{true: "are", false: "is"}[len(inert) > 1]
		them := map[bool]string{true: "them", false: "it"}[len(inert) > 1]
		warnings = append(warnings, fmt.Sprintf(
			"%s %s publishes %s, which %s neither passed to %s nor used by %s itself, so setting %s "+
				"on a component will do nothing. Inheriting a parent's whole parameter set with "+
				"`$super.parameter` suits a definition that forwards all of it, as in `$super: properties: parameter`; "+
				"one that decides some of them on its users' behalf should declare only what it exposes",
			s.Kind, self.Name, quoteList(inert), plural, parent.Name, self.Name, them))
	}
	return warnings, nil
}

// inertParameters finds parameters a definition publishes that go nowhere: not
// supplied to the parent, not read by the definition itself. An application can
// set one and nothing happens.
func inertParameters(ctx context.Context, chain []Level, s Surface, compile CompileFunc, passed map[string]bool, alsoRead []string) []string {
	use := scanParameterUse(append([]string{chain[0].Template}, alsoRead...)...)
	if use.wholesale {
		// `parameter` is used as a whole somewhere, so no individual field of it
		// can be called unused.
		return nil
	}

	effective, err := SchemaValue(ctx, chain, "", s, compile)
	if err != nil {
		return nil
	}
	published, err := fieldNames(effective.LookupPath(cue.ParsePath(ParameterField)))
	if err != nil {
		return nil
	}

	var inert []string
	for name := range published {
		if passed[name] || use.fields[name] {
			continue
		}
		inert = append(inert, name)
	}
	sort.Strings(inert)
	return inert
}

// fieldNames lists the names in a struct, optional ones included.
func fieldNames(call cue.Value) (map[string]bool, error) {
	out := map[string]bool{}
	iter, err := call.Fields(cue.Optional(true))
	if err != nil {
		return nil, err
	}
	for iter.Next() {
		out[strings.TrimSuffix(iter.Selector().String(), "?")] = true
	}
	return out, nil
}

// describeSchema reads a parent's parameter declaration: what it names, what it
// requires, and whether it takes names it never mentioned.
//
// Required means not optional, no default, not already concrete. `*1 | int` is
// not required: leaving it out is what a default is for.
func describeSchema(schema cue.Value) (declared map[string]bool, required []string, open bool, err error) {
	declared = map[string]bool{}

	iter, err := schema.Fields(cue.Optional(true))
	if err != nil {
		return nil, nil, false, err
	}
	for iter.Next() {
		name := strings.TrimSuffix(iter.Selector().String(), "?")
		declared[name] = true
		if iter.IsOptional() {
			continue
		}
		v := iter.Value()
		if _, hasDefault := v.Default(); hasDefault {
			continue
		}
		if v.IsConcrete() {
			continue
		}
		required = append(required, name)
	}

	return declared, required, hasPatternConstraint(schema), nil
}

// hasPatternConstraint reports whether a declaration takes names nobody wrote
// down, as `parameter: [string]: string` does in the labels trait.
//
// Read from the syntax, because a plain CUE struct is open too: asking the value
// whether it allows an invented name says yes for both.
//
// A schema may arrive as an expression rather than a struct. A child that
// inherits its parent's declaration writes `$super.parameter & {...}`, whose
// syntax is a binary expression, so both sides are searched: reading only the
// top level would report it closed and warn about every name the pattern exists
// to allow.
func hasPatternConstraint(schema cue.Value) bool {
	return nodeHasPattern(schema.Syntax(cue.Raw()))
}

func nodeHasPattern(node ast.Node) bool {
	switch n := node.(type) {
	case *ast.StructLit:
		for _, elt := range n.Elts {
			f, ok := elt.(*ast.Field)
			if !ok {
				continue
			}
			if _, isPattern := f.Label.(*ast.ListLit); isPattern {
				return true
			}
		}
	case *ast.BinaryExpr:
		return nodeHasPattern(n.X) || nodeHasPattern(n.Y)
	case *ast.ParenExpr:
		return nodeHasPattern(n.X)
	case *ast.File:
		for _, decl := range n.Decls {
			if nodeHasPattern(decl) {
				return true
			}
		}
	case *ast.EmbedDecl:
		return nodeHasPattern(n.Expr)
	}
	return false
}

// unifiable reports whether a schema would accept a call, neither being
// concrete yet.
func unifiable(call, schema cue.Value) error {
	merged := call.Unify(schema)
	if err := merged.Err(); err != nil {
		return err
	}
	// Not Concrete: a call is expressions over `parameter`, and demanding values
	// would reject every correct definition.
	return merged.Validate()
}

func quoteList(names []string) string {
	quoted := make([]string, 0, len(names))
	for _, n := range names {
		quoted = append(quoted, fmt.Sprintf("%q", n))
	}
	if len(quoted) == 1 {
		return quoted[0]
	}
	return strings.Join(quoted[:len(quoted)-1], ", ") + " and " + quoted[len(quoted)-1]
}

// SchemaValue compiles a chain's child far enough for its parameters to be read,
// which a definition writing `parameter: $super.parameter & {...}` needs. The
// OpenAPI generator uses it. Nothing is rendered, so a parent that calls a
// provider is not made to call it.
func SchemaValue(ctx context.Context, chain []Level, contextFile string, s Surface, compile CompileFunc) (cue.Value, error) {
	if len(chain) == 0 {
		return cue.Value{}, fmt.Errorf("read parameters: empty inheritance chain")
	}
	self := chain[0]

	val, compileErr := compile(ctx, join(self.Template, contextFile))
	if !val.Exists() {
		return cue.Value{}, fmt.Errorf("read the parameters of %s %s: %w", s.Kind, self.Name, compileErr)
	}
	if len(chain) == 1 {
		return val, nil
	}

	parentSchema, err := schemaOf(ctx, chain[1:], contextFile, s, compile)
	if err != nil {
		return cue.Value{}, err
	}
	moved, err := adopt(val, parentSchema)
	if err != nil {
		return cue.Value{}, fmt.Errorf("%s %s: %w", s.Kind, self.Name, err)
	}
	return val.FillPath(superPath(ParameterField), moved), nil
}
