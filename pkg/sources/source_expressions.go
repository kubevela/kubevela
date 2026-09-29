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
	"fmt"
	"strconv"
	"strings"

	celengine "github.com/kubevela/pkg/cel"

	"github.com/oam-dev/kubevela/pkg/definition/celexpr"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
)

// resolveSourceNode substitutes every expression in a properties blob, reading
// sources through this resolver.
//
// Resolution happens here rather than at admission so that reading a source
// through an expression drives the resolution and the consumed-value recording
// that status reports. Each read carries the property it feeds, so status can
// report where a value went as well as what was read, which is the half that
// matters once a property is assembled from more than one source.
func resolveSourceNode(node interface{}, resolver *sourceResolver) (interface{}, error) {
	ctx := resolver.goCtx
	if ctx == nil {
		ctx = context.Background()
	}
	return celexpr.Vela.EvalTree(ctx, node, map[string]celengine.Resolver{
		propexpr.SourceIdent: celengine.ResolverFunc(resolver.resolveReads),
		propexpr.ContextIdent: celengine.ResolverFunc(func(context.Context, []celengine.Read) (interface{}, error) {
			return resolver.expressionContext(), nil
		}),
		propexpr.ComponentIdent: celengine.ResolverFunc(resolver.componentReadsFor),
	}, celengine.TreeOptions{Unknown: componentPlaceholder, OnEvalError: waitOnMissingOutput})
}

// waitOnMissingOutput treats an element or key missing below a component read,
// such as the first of a list a status fills in later, as a wait: only
// evaluation finds it missing.
func waitOnMissingOutput(expr string, roots []string, err error) error {
	for _, root := range roots {
		if root == propexpr.ComponentIdent && missingFromOutput(err) {
			return ComponentReadNotReady{Reason: fmt.Sprintf("waiting for %s: %v", expr, err)}
		}
	}
	return err
}

// missingFromOutput reports an evaluation that failed on an element or key not
// there, as opposed to one that failed on how the expression is written.
func missingFromOutput(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "out of bounds") || strings.Contains(msg, "out of range") || strings.Contains(msg, "no such key")
}

// resolveReads resolves every binding the reads name and records what each read
// consumed.
//
// Resolved values were retyped against their source's schema, so they are
// marked typed: guessing an int from a float64 with no fractional part would
// undo that.
func (r *sourceResolver) resolveReads(_ context.Context, reads []celengine.Read) (interface{}, error) {
	resolved := map[string]interface{}{}
	for _, read := range reads {
		// A bare `source` names no binding to resolve. Admission refuses it, and
		// reaching here means it came from somewhere admission does not cover.
		if len(read.Path) == 0 {
			continue
		}
		name := read.Path[0]
		values, err := r.resolve(name)
		if err != nil {
			return nil, err
		}
		resolved[name] = values

		// Recorded as status reports it, including +sensitive redaction, which
		// matches on the recorded path. Looked up by segments, reported as text:
		// a key may contain a dot (a ConfigMap entry called app.properties, a
		// domain-prefixed label), and splitting the rendered path would read one
		// key as two, losing the read and the hash that drives auto-update.
		segments := read.Path[1:]
		if value, ok := lookupMapSegments(values, segments); ok {
			r.recordConsumedValue(name, r.sourceTypes[name], strings.Join(segments, "."), value, read.Property)
		}
	}
	return celengine.Typed(resolved), nil
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
