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

package celexpr

import (
	"fmt"
	"math"
	"strings"

	"github.com/google/cel-go/cel"

	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
)

// A property blob is arbitrary JSON, and an expression can sit at any depth
// inside it - in a nested object, in a list entry, in a k8s-objects manifest.
// These walk it.
//
// The walking is not specific to the expression language; only what happens at a
// string leaf is. propexpr.Parse still does the `$( )` splitting, because that
// is text handling rather than evaluation.

// ValidateTree checks every expression in a property blob, refusing any that does
// not compile or that reads a root this surface does not permit.
//
// The permissive environment is deliberate: this is the grammar-level pass, run
// before schemas are loaded. Types are checked separately, against the declared
// shapes.
func ValidateTree(v interface{}, roots ...string) error {
	env, err := DynEnv()
	if err != nil {
		return err
	}
	return validateNode(env, v, roots)
}

func validateNode(env *cel.Env, v interface{}, roots []string) error {
	return propexpr.Walk(v, "", func(_, raw string) error {
		return validateLeaf(env, raw, roots)
	})
}

func validateLeaf(env *cel.Env, t string, roots []string) error {
	parsed, err := propexpr.Parse(t)
	if err != nil || !parsed.HasExpr() {
		return err
	}
	for _, f := range parsed.Fragments {
		if !f.IsExpr() {
			continue
		}
		refs, rerr := References(env, f.Expr)
		if rerr != nil {
			return rerr
		}
		for _, r := range refs {
			if !contains(roots, r.Root) {
				return fmt.Errorf("%q cannot be read here; this surface permits %q",
					r.Root, strings.Join(roots, `", "`))
			}
		}
	}
	return nil
}

// EvalTree substitutes every expression in a property blob.
//
// A leaf that is a single expression keeps its type - an int stays an int - and
// one embedded in text becomes a string. A leaf with no expression is returned
// byte-identical, so a plain value is never rewritten.
func EvalTree(v interface{}, resolved map[string]map[string]interface{},
	ctx map[string]interface{}) (interface{}, error) {
	env, err := DynEnv()
	if err != nil {
		return nil, err
	}
	sources := map[string]interface{}{}
	for name, values := range resolved {
		sources[name] = values
	}
	in := map[string]interface{}{"source": sources, "context": ctx}
	return evalNode(env, v, in)
}

func evalNode(env *cel.Env, v interface{}, in map[string]interface{}) (interface{}, error) {
	return propexpr.Map(v, "", func(_, raw string) (interface{}, error) {
		return EvalProperty(env, raw, in)
	})
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// normaliseNumbers turns an integral float64 into an int64 throughout a value,
// on top of the width conversions normaliseWidths makes.
//
// It is the reading for a value with no schema to consult. Such a value has been
// through JSON, where every number decodes as float64 and the int/float
// distinction is lost. CEL has no mixed numeric overloads, so
// `source.cfg.port + 1000` on a JSON-decoded 8080 fails at evaluation with "no
// such overload" - an error that names neither the field nor the cause, on an
// expression that type-checked cleanly at admission because the schema said
// `port: int`.
//
// A JSON number with no fractional part was an integer in the schema that
// produced it, more often than not. Where the schema is actually available the
// guess is unnecessary and wrong: see normaliseWidths.
func normaliseNumbers(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, child := range t {
			out[k] = normaliseNumbers(child)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, child := range t {
			out[i] = normaliseNumbers(child)
		}
		return out
	case alreadyTyped:
		return t.value
	case float64:
		if t == math.Trunc(t) && !math.IsInf(t, 0) {
			return int64(t)
		}
		return t
	default:
		return normaliseWidths(v)
	}
}

// alreadyTyped wraps a value whose numeric types are the ones its schema
// declares, so normaliseNumbers unwraps it rather than guessing over it.
type alreadyTyped struct{ value interface{} }

// normaliseWidths converts Go's numeric widths to the two CEL has overloads for,
// and nothing else.
//
// For a value already typed by a schema, that is the whole job: an int is an
// int and a float is a float, and guessing from the value would only undo the
// answer. Guessing turned a field declared `float` holding exactly 2.0 into an
// int, so `ratio * 2.0` type-checked as double at admission and then failed at
// render with "no such overload".
func normaliseWidths(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, child := range t {
			out[k] = normaliseWidths(child)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, child := range t {
			out[i] = normaliseWidths(child)
		}
		return out
	case float32:
		return float64(t)
	case int:
		return int64(t)
	case int32:
		return int64(t)
	default:
		return v
	}
}

// normaliseInput applies normaliseNumbers to an activation map, keeping the type
// CEL's Eval expects.
func normaliseInput(in map[string]interface{}) map[string]interface{} {
	out, _ := normaliseNumbers(in).(map[string]interface{})
	if out == nil {
		return in
	}
	return out
}
