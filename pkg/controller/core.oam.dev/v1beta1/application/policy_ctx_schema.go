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
	"fmt"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	cueerrors "cuelang.org/go/cue/errors"
	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/pkg/policy/ctxschema"
)

// checkCtxSchema checks a rendered policy's output.ctx against the schema.ctx
// it declares: every value matching, every required key present, and nothing
// undeclared when the schema is closed. A policy declaring no schema is not
// checked.
//
// The rendered template is used whole, so a schema may refer to the rest of
// the template here; admission, which reads the schema on its own, may not.
func checkCtxSchema(rendered cue.Value) error {
	schema := rendered.LookupPath(cue.ParsePath("schema.ctx"))
	if !schema.Exists() {
		return nil
	}
	out := rendered.LookupPath(cue.ParsePath("output.ctx"))
	c := &ctxChecker{}
	c.check(schema, out, []string{"output", "ctx"})
	if len(c.lines) == 0 {
		return nil
	}
	sort.Strings(c.lines)
	return errors.New("output.ctx does not match schema.ctx: " + strings.Join(c.lines, "; "))
}

// ctxWithSchemaDefaults is the context a policy hands components: its
// output.ctx unified with its schema.ctx, so a key the schema defaults
// (team: *"x" | string) and the output leaves out is filled, as both checks
// already judged it. An optional key is never filled. A policy declaring no
// schema hands over decoded, its output.ctx as it is. Call it only once
// checkCtxSchema has passed.
func ctxWithSchemaDefaults(rendered cue.Value, decoded map[string]interface{}) (map[string]interface{}, error) {
	schema := rendered.LookupPath(cue.ParsePath("schema.ctx"))
	if !schema.Exists() {
		return decoded, nil
	}
	v := schema
	if out := rendered.LookupPath(cue.ParsePath("output.ctx")); out.Exists() {
		v = schema.Unify(out)
	}
	var ctx map[string]interface{}
	if err := v.Decode(&ctx); err != nil {
		return nil, errors.Wrap(err, "failed to decode output.ctx with schema.ctx defaults")
	}
	return ctx, nil
}

// ctxChecker collects every way an output departs from its schema, so an author
// sees them all at once. CUE validating the whole reports one error per field,
// and an undeclared key only when nothing else is wrong, so this walks both and
// validates each leaf on its own.
type ctxChecker struct {
	lines []string
}

func (c *ctxChecker) check(schema, out cue.Value, path []string) {
	if schema.IncompleteKind() != cue.StructKind {
		v := schema
		if out.Exists() {
			v = schema.Unify(out)
		}
		c.addErrors(path, v.Validate(cue.Concrete(true)))
		return
	}
	v := schema
	if out.Exists() {
		if it, err := out.Fields(); err == nil {
			for it.Next() {
				if !schema.Allows(it.Selector()) {
					c.add(subPath(path, it.Selector()), "not declared by schema.ctx, which is closed")
				}
			}
		}
		v = schema.Unify(out)
	}
	// The unified struct is an error as a whole once any field is, so whether
	// to descend is judged from the schema, never from it. An output that is
	// not a struct leaves no fields to walk, and the error says why.
	it, err := v.Fields()
	if err != nil {
		c.addErrors(path, err)
		return
	}
	for it.Next() {
		sel := it.Selector()
		child := lookupField(schema, sel)
		if !child.Exists() {
			// Undeclared under an open schema, or matched by a pattern
			// constraint: the unified field carries whatever applies.
			child = it.Value()
		}
		c.check(child, out.LookupPath(cue.MakePath(sel)), subPath(path, sel))
	}
}

func subPath(path []string, sel cue.Selector) []string {
	return append(append(make([]string, 0, len(path)+1), path...), sel.String())
}

// lookupField finds a field of the schema whether it declares it required or
// optional: an output setting an optional key makes it a regular field of the
// unified value.
func lookupField(schema cue.Value, sel cue.Selector) cue.Value {
	if f := schema.LookupPath(cue.MakePath(sel)); f.Exists() {
		return f
	}
	return schema.LookupPath(cue.MakePath(sel.Optional()))
}

// addErrors adds CUE's errors at a path, in the author's terms: an incomplete
// value is a required key the output did not set. An error naming a deeper
// path, a list element, keeps it.
func (c *ctxChecker) addErrors(path []string, err error) {
	for _, e := range cueerrors.Errors(err) {
		p := path
		if ep := e.Path(); len(ep) > len(path) {
			p = append(append(make([]string, 0, len(ep)), path...), ep[len(path):]...)
		}
		format, args := e.Msg()
		switch {
		case strings.HasPrefix(format, "incomplete value"):
			c.add(p, fmt.Sprintf("required by schema.ctx (%v), not set", args...))
		case format == "field not allowed":
			c.add(p, "not declared by schema.ctx, which is closed")
		default:
			c.add(p, fmt.Sprintf(format, args...))
		}
	}
}

func (c *ctxChecker) add(path []string, msg string) {
	c.lines = append(c.lines, strings.Join(path, ".")+": "+msg)
}

// validateCtxSchema checks what an Application-scoped policy declares about
// its context, for the PolicyDefinition webhook. A schema admission cannot read
// on its own is refused, where it is written rather than on every Application
// it would have judged. Setting ctx with no schema is allowed, and warned of.
func validateCtxSchema(template string) (errs, warnings []string) {
	info, err := ctxschema.Read(template)
	if err != nil {
		// Reported by the template's own compilation.
		return nil, nil
	}
	if info.Schema != "" {
		if _, err := ctxschema.Compile(cuecontext.New(), info.Schema); err != nil {
			return []string{fmt.Sprintf("schema.ctx must stand on its own, using at most the CUE standard library and nothing else of the template, so that admission can read it: %v", err)}, nil
		}
		return nil, nil
	}
	if info.SetsCtx {
		warnings = append(warnings, "output.ctx is set without a schema.ctx declaring it, so Applications cannot be checked against it at admission; declare schema: ctx (and close() it to make it the whole contract)")
	}
	return nil, warnings
}
