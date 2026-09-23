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

package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	cueformat "cuelang.org/go/cue/format"
	"github.com/kubevela/workflow/pkg/cue/process"
	"k8s.io/utils/lru"

	"github.com/oam-dev/kubevela/pkg/definition/celexpr"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
)

// typeOnlyKey marks a render as a validation rather than a real one.
type typeOnlyKey struct{}

// WithTypeOnly marks a context as validating, so an expression is typed from its
// source schema rather than resolved.
func WithTypeOnly(ctx context.Context) context.Context {
	return context.WithValue(ctx, typeOnlyKey{}, true)
}

// TypeOnly reports whether this render is a validation.
func TypeOnly(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	on, _ := ctx.Value(typeOnlyKey{}).(bool)
	return on
}

// CUEType is a CUE type expression standing in for a value that is not knowable
// until render: "int", "string", "[...string]".
//
// A type and not a placeholder value: `replicas: int` unifies with a `>0 & int`
// constraint, while a sentinel 0 is refused as out of bound.
type CUEType string

// TypedParams replaces every $( ) expression in a component's properties with
// the type its source schema declares, leaving every other value alone.
//
// A struct read keeps its shape: the required-field check flattens what was
// provided and looks for "meta.region".
func TypedParams(ctx process.Context, params map[string]any) (map[string]any, error) {
	if len(params) == 0 {
		return params, nil
	}
	// The schemas and the CUE context are built on first use: most components
	// carry no expression at all, and for those this is a walk and nothing more.
	t := &paramTyper{ctx: ctx, compiled: map[string]cue.Value{}}
	out, changed, err := t.walk(params)
	if err != nil {
		return nil, err
	}
	typed, ok := out.(map[string]any)
	if !changed || !ok {
		return params, nil
	}
	return typed, nil
}

type paramTyper struct {
	ctx      process.Context
	schemas  map[string]string
	compiled map[string]cue.Value
	cuectx   *cue.Context
	ready    bool
}

// prepare builds what typing needs, once, on the first expression seen.
func (t *paramTyper) prepare() {
	if t.ready {
		return
	}
	t.ready = true
	if t.cuectx == nil {
		t.cuectx = cuecontext.New()
	}
	if t.schemas == nil && t.ctx != nil {
		t.schemas = SchemasForContext(t.ctx)
	}
}

func (t *paramTyper) walk(node any) (any, bool, error) {
	// A branch with nothing to type is returned as it came in. Most components
	// carry no expression at all, so copying the tree would be the whole cost.
	switch v := node.(type) {
	case map[string]any:
		var out map[string]any
		for key, child := range v {
			typed, changed, err := t.walk(child)
			if err != nil {
				return nil, false, err
			}
			if !changed {
				continue
			}
			if out == nil {
				out = make(map[string]any, len(v))
				for k, c := range v {
					out[k] = c
				}
			}
			out[key] = typed
		}
		if out == nil {
			return v, false, nil
		}
		return out, true, nil
	case []any:
		var out []any
		for i, child := range v {
			typed, changed, err := t.walk(child)
			if err != nil {
				return nil, false, err
			}
			if !changed {
				continue
			}
			if out == nil {
				out = make([]any, len(v))
				copy(out, v)
			}
			out[i] = typed
		}
		if out == nil {
			return v, false, nil
		}
		return out, true, nil
	case string:
		typed, err := t.typeOfLeaf(v)
		if err != nil {
			return nil, false, err
		}
		return typed, typed != node, nil
	default:
		return node, false, nil
	}
}

// typeOfLeaf returns what a string leaf should contribute: itself when it holds
// no expression, otherwise the type the expression produces.
func (t *paramTyper) typeOfLeaf(raw string) (any, error) {
	if !propexpr.MayContainExpr(raw) {
		return raw, nil
	}
	parsed, err := propexpr.Parse(raw)
	if err != nil || !parsed.HasExpr() {
		//nolint:nilerr // a malformed expression is reported by the expression validator
		return raw, nil
	}
	t.prepare()
	expr, whole := parsed.SoleExpr()
	if !whole {
		// Interpolated into text, so the result is a string whatever the parts
		// are. celexpr refuses a struct or list in that position separately.
		return CUEType("string"), nil
	}
	// A plain read of a declared field, the common case, is typed from the
	// schema subtree, which carries the field's shape and not just its kind.
	if shaped, ok := t.fromSchema(expr); ok {
		return shaped, nil
	}
	// Anything computed: a ternary, arithmetic, a function call. CEL knows the
	// result type without knowing the values.
	return t.fromCEL(expr)
}

// fromSchema types a plain `source.<binding>.<path>` read from the binding's
// declared schema, or the whole binding when the path stops at its name.
// Reports false for anything else, including a read of a binding with no schema
// to judge by.
func (t *paramTyper) fromSchema(expr string) (any, bool) {
	refs, err := celexpr.PropertyReferences(expr)
	if err != nil || len(refs) != 1 {
		return nil, false
	}
	ref := refs[0]
	if !ref.IsSource() || len(ref.Path) == 0 || ref.String() != expr {
		// Not a bare read: the expression does something with the value.
		return nil, false
	}
	schema, ok := t.schemaFor(ref.Path[0])
	if !ok {
		return nil, false
	}
	field := schema
	for _, segment := range ref.Path[1:] {
		field = field.LookupPath(cue.MakePath(cue.Str(segment)))
		if !field.Exists() {
			// The read is refused by the schema check; nothing to type here.
			return nil, false
		}
	}
	return t.shapeOf(field), true
}

// shapeOf converts a schema field into what TypedParams emits: a nested map for
// a struct, so its fields survive flattening, and a type expression otherwise.
//
// Optional fields are left out. The required-field check treats every key it
// finds here as provided, and a field the schema marks optional is one the
// source may never supply.
func (t *paramTyper) shapeOf(field cue.Value) any {
	if field.IncompleteKind() == cue.StructKind {
		iter, err := field.Fields(cue.Optional(false), cue.Definitions(false))
		if err != nil {
			return CUEType("{...}")
		}
		out := map[string]any{}
		for iter.Next() {
			sel := iter.Selector()
			if !sel.IsString() {
				continue
			}
			out[sel.Unquoted()] = t.shapeOf(iter.Value())
		}
		if len(out) == 0 {
			// Nothing guaranteed: an open map declaring a value type and no
			// keys, or a struct whose fields are all optional.
			return CUEType("{...}")
		}
		return out
	}
	return CUEType(t.typeExprFor(field))
}

// typeExprFor renders what a schema field guarantees, constraints and all.
//
// Two constraints unify to bottom exactly when no value satisfies both, so
// carrying the source's guarantee lets admission refuse a consumer it can never
// satisfy. Only when the constraint stands alone: one expressed in terms of
// another field means nothing lifted out of its schema, and falls back to the
// kind.
func (t *paramTyper) typeExprFor(field cue.Value) string {
	kind := kindExpr(field.IncompleteKind(), field)
	syntax := field.Syntax(cue.Raw())
	if syntax == nil {
		return kind
	}
	rendered, err := cueformat.Node(syntax)
	if err != nil {
		return kind
	}
	expr := strings.TrimSpace(string(rendered))
	if expr == "" || strings.Contains(expr, "\n") {
		return kind
	}
	if !standaloneConstraint(t.cuectx, expr) {
		return kind
	}
	return expr
}

// standaloneConstraintCacheSize bounds the distinct constraint expressions kept.
// They come from SourceDefinition schemas, so the set is small and stable.
const standaloneConstraintCacheSize = 512

// standaloneConstraintCache answers whether a constraint means the same thing
// away from its schema. A pure function of the text, and the check costs a CUE
// compile, which admission would otherwise repeat for every component reading
// the same field.
var standaloneConstraintCache = lru.New(standaloneConstraintCacheSize)

// standaloneConstraint reports whether expr compiles on its own and still admits
// something. A constraint that narrowed to nothing would refuse every
// Application reading the field.
func standaloneConstraint(cuectx *cue.Context, expr string) bool {
	if hit, ok := standaloneConstraintCache.Get(expr); ok {
		if usable, ok := hit.(bool); ok {
			return usable
		}
	}
	usable := false
	if v := cuectx.CompileString("v: " + expr); v.Err() == nil {
		usable = v.LookupPath(cue.MakePath(cue.Str("v"))).Validate(cue.Concrete(false)) == nil
	}
	standaloneConstraintCache.Add(expr, usable)
	return usable
}

// fromCEL types a computed expression from its output type alone.
func (t *paramTyper) fromCEL(expr string) (any, error) {
	env, err := celexpr.EnvForContext(t.schemas, propexpr.ComponentContext)
	if err != nil {
		return nil, err
	}
	out, err := celexpr.OutputType(env, expr)
	if err != nil {
		// The expression validator reports this properly. Leaving it untyped
		// here keeps one failure to one message.
		//nolint:nilerr // reported elsewhere
		return CUEType("_"), nil
	}
	return CUEType(celTypeExpr(out.String())), nil
}

func (t *paramTyper) schemaFor(binding string) (cue.Value, bool) {
	if v, ok := t.compiled[binding]; ok {
		return v, v.Exists()
	}
	text, ok := t.schemas[binding]
	if !ok {
		t.compiled[binding] = cue.Value{}
		return cue.Value{}, false
	}
	v := t.cuectx.CompileString(text)
	if v.Err() != nil {
		v = cue.Value{}
	}
	t.compiled[binding] = v
	return v, v.Exists()
}

// kindExpr names a CUE kind as a type expression, for a field whose own
// constraint cannot be carried.
func kindExpr(k cue.Kind, field cue.Value) string {
	switch k {
	case cue.StringKind:
		return "string"
	case cue.IntKind:
		return "int"
	case cue.FloatKind:
		return "float"
	case cue.NumberKind:
		return "number"
	case cue.BoolKind:
		return "bool"
	case cue.ListKind:
		if elem := field.LookupPath(cue.MakePath(cue.AnyIndex)); elem.Exists() {
			return "[..." + kindExpr(elem.IncompleteKind(), elem) + "]"
		}
		return "[...]"
	case cue.NullKind:
		return "null"
	}
	return "_"
}

// celTypeExpr names a CEL type as a CUE type expression.
func celTypeExpr(t string) string {
	switch t {
	case "string":
		return "string"
	case "int", "uint":
		return "int"
	case "double":
		return "float"
	case "bool":
		return "bool"
	case "bytes":
		return "bytes"
	case "null_type":
		return "null"
	}
	if strings.HasPrefix(t, "list(") {
		return "[..." + celTypeExpr(strings.TrimSuffix(strings.TrimPrefix(t, "list("), ")")) + "]"
	}
	if strings.HasPrefix(t, "map(") {
		return "{...}"
	}
	// dyn, any, a message type: nothing precise to say, so constrain nothing.
	return "_"
}

// ParamsAsCUE renders typed properties as CUE. Values without a type in them
// render as the JSON they would have anyway; a type cannot survive JSON.
func ParamsAsCUE(params map[string]any) (string, error) {
	return renderCUE(params)
}

// renderCUE writes a value as CUE. JSON is valid CUE, so only the type markers
// need special handling: they are written bare rather than quoted.
func renderCUE(node any) (string, error) {
	switch v := node.(type) {
	case CUEType:
		return string(v), nil
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			rendered, err := renderCUE(v[key])
			if err != nil {
				return "", err
			}
			label, err := json.Marshal(key)
			if err != nil {
				return "", err
			}
			parts = append(parts, fmt.Sprintf("%s: %s", label, rendered))
		}
		return "{" + strings.Join(parts, ", ") + "}", nil
	case []any:
		parts := make([]string, 0, len(v))
		for _, child := range v {
			rendered, err := renderCUE(child)
			if err != nil {
				return "", err
			}
			parts = append(parts, rendered)
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	default:
		raw, err := json.Marshal(node)
		if err != nil {
			return "", err
		}
		return string(raw), nil
	}
}

// ConcreteForValidation makes a typed render marshalable: a component's output
// becomes the base its traits render against, and the engine passes that as
// JSON, which an unknowable leaf cannot survive.
//
// Those leaves are left out rather than zeroed. A trait patches the base by
// unification, and a zero value conflicts with every literal it might patch in,
// which would refuse an Application that renders fine. An absent field unifies
// with anything.
//
// The output and not the parameters. The parameter block is checked against the
// definition's constraints, where a missing field would fail the required-field
// check; the output is checked against nothing.
func ConcreteForValidation(v cue.Value) (cue.Value, bool) {
	pruned, changed := pruneIncomplete(v)
	if !changed || pruned == any(dropped) {
		return v, false
	}
	out := v.Context().Encode(pruned)
	if out.Err() != nil {
		return v, false
	}
	return out, true
}

// dropped marks a leaf that carries no value worth passing on. A sentinel and
// not nil, because nil is what a CUE null decodes to.
type droppedLeaf struct{}

var dropped = droppedLeaf{}

func pruneIncomplete(v cue.Value) (any, bool) {
	switch v.IncompleteKind() {
	case cue.StructKind:
		iter, err := v.Fields()
		if err != nil {
			return map[string]any{}, false
		}
		out := map[string]any{}
		changed := false
		for iter.Next() {
			sel := iter.Selector()
			if !sel.IsString() {
				continue
			}
			child, childChanged := pruneIncomplete(iter.Value())
			if child == any(dropped) {
				changed = true
				continue
			}
			out[sel.Unquoted()] = child
			changed = changed || childChanged
		}
		return out, changed
	case cue.ListKind:
		iter, err := v.List()
		if err != nil {
			return []any{}, false
		}
		out := []any{}
		changed := false
		for iter.Next() {
			child, childChanged := pruneIncomplete(iter.Value())
			if child == any(dropped) {
				// A list unifies by position, so an element cannot be left out
				// without moving the ones after it.
				child = zeroOf(iter.Value().IncompleteKind())
				childChanged = true
			}
			out = append(out, child)
			changed = changed || childChanged
		}
		return out, changed
	}
	if v.IsConcrete() {
		var decoded any
		if err := v.Decode(&decoded); err == nil {
			return decoded, false
		}
	}
	return dropped, true
}

// zeroOf is a stand-in of the right shape for a list element that is not
// knowable, which cannot be left out without moving its neighbours.
func zeroOf(k cue.Kind) any {
	switch k {
	case cue.StringKind:
		return ""
	case cue.IntKind:
		return 0
	case cue.FloatKind, cue.NumberKind:
		return 0.0
	case cue.BoolKind:
		return false
	case cue.ListKind:
		return []any{}
	case cue.StructKind:
		return map[string]any{}
	}
	return ""
}
