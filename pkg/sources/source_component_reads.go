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

	celengine "github.com/kubevela/pkg/cel"
	"github.com/kubevela/workflow/pkg/cue/process"

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

// componentReadsFor answers the component reads of one render from the scope the
// controller delivered, checking each before anything is evaluated.
//
// A producer missing altogether means nothing was delivered: this render is not
// one the controller answers reads for, such as a CLI dry-run. A field missing
// from a delivered output, read without a guard, is not ready yet: a status field
// is commonly filled in after the resource is first healthy. In a dry-run with
// placeholders, nothing is checked and every read renders as its own text.
func (r *sourceResolver) componentReadsFor(ctx context.Context, reads []celengine.Read) (interface{}, error) {
	if ComponentPlaceholders(ctx) {
		return celengine.Unknown, nil
	}
	delivered := r.componentReads
	for _, read := range reads {
		ref := read.Reference
		if len(ref.Path) == 0 {
			continue
		}
		calls, path := propexpr.Placement(ref)
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
			qualified, _ := m[celengine.QualifiedKey].(map[string]interface{})
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
// null is there. A segment indexes a list when the value is one, since a read
// carries indices as decimal text.
func lookupPath(v interface{}, path []string) (interface{}, bool) {
	if v == nil {
		return nil, false
	}
	for _, seg := range path {
		switch t := v.(type) {
		case map[string]interface{}:
			next, ok := t[seg]
			if !ok {
				return nil, false
			}
			v = next
		case []interface{}:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(t) {
				return nil, false
			}
			v = t[i]
		default:
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

// Is makes a ComponentReadNotReady match celengine.ErrNotReady.
func (e ComponentReadNotReady) Is(target error) bool { return target == celengine.ErrNotReady }

// IsComponentReadNotReady reports whether err, or anything it wraps, means a
// read is waiting: a ComponentReadNotReady, or any resolver's
// celengine.ErrNotReady.
func IsComponentReadNotReady(err error) bool {
	return errors.Is(err, celengine.ErrNotReady)
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

// componentPlaceholder renders a component read in a dry-run as its own text in
// angle brackets.
//
// A value that is only a component read has no knowable type, so it is not
// checked: it defaults to the placeholder where the parameter takes text and is
// otherwise left open, and a render in this mode prunes what stays open.
func componentPlaceholder(expr string, whole bool) (interface{}, error) {
	if whole {
		return CUEType("*" + strconv.Quote("<"+expr+">") + " | _"), nil
	}
	return "<" + expr + ">", nil
}
