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
	"strconv"
	"strings"

	"github.com/oam-dev/kubevela/pkg/definition/celexpr"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
)

// resolveSourceNode walks a properties blob, tracking the path it is at so a
// recorded read can say which property received the value. Without that, status
// can report what was read but not where it went, which is the half that matters
// once a property is assembled from more than one source.
func resolveSourceNode(node interface{}, resolver *sourceResolver) (interface{}, error) {
	return propexpr.Map(node, "", func(at, raw string) (interface{}, error) {
		return evaluateSourceExpression(raw, resolver, at)
	})
}

// evaluateSourceExpression substitutes $(...) expressions in a property value.
// A value with no delimiter comes back byte-identical; one holding only `$$(`
// escapes comes back with them collapsed, which is the point of writing them.
//
// Resolution happens here rather than at admission so that reading a source
// through an expression drives the resolution and the consumed-value recording
// that status reports. Otherwise a binding read only by an expression would show
// as unresolved.
func evaluateSourceExpression(raw string, resolver *sourceResolver, property string) (interface{}, error) {
	parsed, err := propexpr.Parse(raw)
	if err != nil {
		return nil, err
	}
	if !parsed.HasExpr() {
		return parsed.Literal(), nil
	}

	resolved := map[string]map[string]interface{}{}
	for _, fragment := range parsed.Fragments {
		if !fragment.IsExpr() {
			continue
		}
		refs, rerr := celexpr.PropertyReferences(fragment.Expr)
		if rerr != nil {
			return nil, rerr
		}
		for _, ref := range refs {
			// A bare `source` names no binding to resolve. Admission refuses it,
			// and reaching here means it came from somewhere admission does not
			// cover.
			if !ref.IsSource() || len(ref.Path) == 0 {
				continue
			}
			name := ref.Path[0]
			values, verr := resolver.resolve(name)
			if verr != nil {
				return nil, verr
			}
			resolved[name] = values

			// Record what the expression read, so status reports it exactly as a
			// status reports it - including +sensitive redaction, which
			// matches on the recorded path.
			// Looked up by segments, reported as text. A key may contain a dot
			// - a ConfigMap entry called app.properties, a domain-prefixed label
			// - and splitting the rendered path would read one key as two, so
			// the read went unrecorded and with it the hash that drives
			// auto-update for that binding.
			segments := ref.Path[1:]
			if value, ok := lookupMapSegments(values, segments); ok {
				resolver.recordConsumedValue(name, resolver.sourceTypes[name],
					strings.Join(segments, "."), value, property)
			}
		}
	}

	return celEvalProperty(raw, resolved, resolver.expressionContext())
}

// celEvalProperty evaluates a whole property value with CEL, interpolation
// included. The $( ) splitting is shared, so only the contents differ.
func celEvalProperty(raw string, resolved map[string]map[string]interface{},
	ctx map[string]interface{}) (interface{}, error) {
	env, err := celexpr.DynEnv()
	if err != nil {
		return nil, err
	}
	in := map[string]interface{}{"context": ctx}
	sources := map[string]interface{}{}
	for name, values := range resolved {
		sources[name] = values
	}
	in["source"] = sources
	// Typed: resolved values were retyped against their source's schema, so
	// guessing an int from a float64 with no fractional part would undo it.
	return celexpr.EvalPropertyTyped(env, raw, in)
}

func lookupMapSegments(data map[string]interface{}, segments []string) (interface{}, bool) {
	// No segments is a read of the binding entire - `$(source.cfg)` rather than
	// `$(source.cfg.host)`, whose reference is Path=["cfg"] and so leaves
	// nothing after the name.
	//
	// The value still substituted without this, so it looked fine; what was lost
	// is the hash that resolvedSourceHashes stamps, and with it auto-update for
	// that binding.
	//
	// Recording it under the empty path is what redaction already expects:
	// RedactValue descends from the read path and joinMaskPath treats an empty
	// prefix as the root, so a +sensitive field one level down is still masked.
	if len(segments) == 0 {
		return data, true
	}
	cur := interface{}(data)
	for _, p := range segments {
		// A segment is an index when what it is being applied to is a list. The
		// reference carries indices as decimal text, and only the value decides
		// how to read them - the same rule the schema walk uses.
		if list, ok := cur.([]interface{}); ok {
			index, err := strconv.Atoi(p)
			if err != nil || index < 0 || index >= len(list) {
				return nil, false
			}
			cur = list[index]
			continue
		}
		m, ok := cur.(map[string]interface{})
		if !ok {
			return nil, false
		}
		next, ok := m[p]
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}
