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
	"sync"

	"github.com/oam-dev/kubevela/pkg/definition/celexpr"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
)

// prefetchConcurrency bounds how many bindings resolve at once. The wait is on
// someone else's service, and several bindings may share a registry.
const prefetchConcurrency = 8

// prefetch resolves, concurrently, the bindings this render needs that do not
// themselves read another binding.
//
// Those are the leaves of the dependency graph, so they resolve in any order,
// and they are the ones that go to the network - a chained binding assembles
// values already in hand. Restricting it to them keeps the concurrent phase free
// of ordering concerns rather than making the resolver safe to share.
//
// Each binding resolves in its own resolver, so nothing mutable is shared while
// the goroutines run; results merge afterwards.
//
// A binding that fails here is left out, and the ordinary lazy path resolves it
// again and reports the failure with its proper context. Prefetching must never
// change an outcome.
func (r *sourceResolver) prefetch(properties interface{}) {
	names := r.independentBindings(properties)
	if len(names) < 2 {
		// One binding gains nothing from a goroutine, and zero gains less.
		return
	}

	type result struct {
		name     string
		values   map[string]interface{}
		statuses map[string]SourceResolutionStatus
	}

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		out  = make([]result, 0, len(names))
		gate = make(chan struct{}, prefetchConcurrency)
	)
	for _, name := range names {
		wg.Add(1)
		go func(binding string) {
			defer wg.Done()
			gate <- struct{}{}
			defer func() { <-gate }()

			sub := newSourceResolver(r.goCtx, r.ctxValues, r.surface, r.inputs())
			values, err := sub.resolve(binding)
			if err != nil {
				return
			}
			mu.Lock()
			out = append(out, result{name: binding, values: values, statuses: sub.statuses})
			mu.Unlock()
		}(name)
	}
	wg.Wait()

	for _, res := range out {
		r.resolved[res.name] = res.values
		r.statuses = mergeStatuses(r.statuses, res.statuses)
	}
}

// inputs reconstructs the resolver's inputs for a sub-resolver.
func (r *sourceResolver) inputs() sourceInputs {
	return sourceInputs{
		Bindings:  r.sourceProps,
		Types:     r.sourceTypes,
		Templates: r.sourceTemplates,
		Sensitive: r.sensitivePaths,
		Store:     r.cacheStore,
		Compiler:  r.compiler,
	}
}

// independentBindings returns the bindings reachable from properties whose own
// properties read nothing, in the order first encountered.
//
// The walk is transitive: a chained binding is excluded, but the bindings it
// reads are reached through its properties, which is where the waiting is.
func (r *sourceResolver) independentBindings(properties interface{}) []string {
	seen := map[string]bool{}
	var order []string

	queue := []interface{}{properties}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		//nolint:errcheck // a malformed expression is the lazy path's to report
		_ = propexpr.Walk(node, "", func(_, raw string) error {
			parsed, err := propexpr.Parse(raw)
			if err != nil || !parsed.HasExpr() {
				//nolint:nilerr // prefetching must not change an outcome; the lazy path reports this
				return nil
			}
			for _, fragment := range parsed.Fragments {
				if !fragment.IsExpr() {
					continue
				}
				refs, rerr := celexpr.PropertyReferences(fragment.Expr)
				if rerr != nil {
					continue
				}
				for _, ref := range refs {
					if ref.Root != "source" || len(ref.Path) == 0 {
						continue
					}
					name := ref.Path[0]
					if seen[name] {
						continue
					}
					seen[name] = true

					props, ok := r.sourceProps[name]
					if ok && props != nil && propexpr.HasExpression(props) {
						queue = append(queue, props)
						continue
					}
					order = append(order, name)
				}
			}
			return nil
		})
	}
	return order
}
