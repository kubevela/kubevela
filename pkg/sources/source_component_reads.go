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
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/kubevela/workflow/pkg/cue/process"

	"github.com/oam-dev/kubevela/pkg/definition/celexpr"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
)

// componentScopeKey carries a render's delivered component reads. A Go context
// value, so that the reads reach the render without being written into the
// component: its properties feed its revision hash, and a producer's live
// output changes on every status update.
type componentScopeKey struct{}

// WithComponentScope attaches the `component` scope the controller delivered for
// a render: producer name -> its output view.
func WithComponentScope(ctx context.Context, scope map[string]interface{}) context.Context {
	return context.WithValue(ctx, componentScopeKey{}, scope)
}

// componentScopeFor is the delivered scope a surface may read, or nil.
func componentScopeFor(ctx process.Context, surface string) map[string]interface{} {
	if !SurfaceReadsComponents(surface) || ctx.GetCtx() == nil {
		return nil
	}
	scope, _ := ctx.GetCtx().Value(componentScopeKey{}).(map[string]interface{})
	return scope
}

// componentScope checks the delivered `component` value against the reads an
// expression makes, before it is evaluated.
//
// A producer missing altogether means nothing was delivered: this render is not
// one the controller answers reads for, such as a CLI dry-run. A field missing
// from a delivered output, read without a guard, is not ready yet: a status field
// is commonly filled in after the resource is first healthy.
func componentScope(refs []propexpr.Reference, delivered map[string]interface{}) (map[string]interface{}, error) {
	for _, ref := range refs {
		if !ref.IsComponent() || len(ref.Path) == 0 {
			continue
		}
		calls, path := ref.Placement()
		entry, ok := delivered[path[0]].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%s has no value in this render: component reads are answered by the "+
				"controller when it renders the component", ref)
		}
		if ref.Defaulted {
			continue
		}
		view := interface{}(entry)
		for _, call := range calls {
			m, _ := view.(map[string]interface{})
			qualified, _ := m[propexpr.QualifiedKey].(map[string]interface{})
			view = qualified[call]
		}
		if _, found := lookupPath(view, path[1:]); !found {
			return nil, ComponentReadNotReady{Reason: fmt.Sprintf("waiting for %s: component %q has no such field yet",
				ref, path[0])}
		}
	}
	if delivered == nil {
		return map[string]interface{}{}, nil
	}
	return delivered, nil
}

// lookupPath follows path through a delivered view. A field that is there but
// null is there.
func lookupPath(v interface{}, path []string) (interface{}, bool) {
	if v == nil {
		return nil, false
	}
	for _, seg := range path {
		m, ok := v.(map[string]interface{})
		if !ok {
			return nil, false
		}
		if v, ok = m[seg]; !ok {
			return nil, false
		}
	}
	return v, true
}

// ComponentReadNotReady is a component read that cannot be answered yet: its
// producer is not healthy, not placed where the read names, or its output lacks
// the field. A reader that hits one is not healthy, and waits, as it would for
// any dependency; nothing about it is an error in the Application.
type ComponentReadNotReady struct{ Reason string }

func (e ComponentReadNotReady) Error() string { return e.Reason }

// IsComponentReadNotReady reports whether err, or anything it wraps, is a
// ComponentReadNotReady.
func IsComponentReadNotReady(err error) bool {
	var nr ComponentReadNotReady
	return errors.As(err, &nr)
}

type componentPlaceholdersKey struct{}

// WithComponentPlaceholders marks a render as one with no live producers, a
// dry-run: each component read renders as its own text, `<component.db...>`,
// while source and context reads still evaluate.
func WithComponentPlaceholders(ctx context.Context) context.Context {
	return context.WithValue(ctx, componentPlaceholdersKey{}, true)
}

// ComponentPlaceholders reports whether component reads render as placeholders.
func ComponentPlaceholders(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	on, _ := ctx.Value(componentPlaceholdersKey{}).(bool)
	return on
}

func readsComponent(refs []propexpr.Reference) bool {
	for _, r := range refs {
		if r.IsComponent() {
			return true
		}
	}
	return false
}

// placeholderProperty renders a property value whose expressions read a
// component: each such expression becomes its own text in angle brackets, and
// the rest evaluate as usual.
//
// A value that is only a component read has no knowable type, so it is not
// checked: it defaults to the placeholder where the parameter takes text and is
// otherwise left open, and a render in this mode prunes what stays open.
func placeholderProperty(parsed propexpr.Parsed, resolved map[string]map[string]interface{},
	ctx map[string]interface{}) (interface{}, error) {
	if expr, whole := parsed.SoleExpr(); whole {
		return CUEType("*" + strconv.Quote("<"+expr+">") + " | _"), nil
	}
	var b strings.Builder
	for _, f := range parsed.Fragments {
		if !f.IsExpr() {
			b.WriteString(f.Text)
			continue
		}
		refs, err := celexpr.PropertyReferences(f.Expr)
		if err != nil {
			return nil, err
		}
		if readsComponent(refs) {
			b.WriteString("<" + f.Expr + ">")
			continue
		}
		v, err := celEvalProperty("$("+f.Expr+")", resolved, ctx, nil)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&b, "%v", v)
	}
	return b.String(), nil
}
