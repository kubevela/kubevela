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

package traitdefinition

import (
	"context"
	"strings"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// extendsIndex indexes a TraitDefinition by the definition it extends, so
// the children of a definition can be found without listing everything.
const extendsIndex = "spec.extends"

// indexExtends registers the index. The indexed value drops any pinned
// revision, so a child on `gateway@v2` still wakes when `gateway`
// changes.
func indexExtends(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &v1beta1.TraitDefinition{}, extendsIndex,
		func(obj client.Object) []string {
			cd, ok := obj.(*v1beta1.TraitDefinition)
			if !ok || cd.Spec.Extends == "" {
				return nil
			}
			name := cd.Spec.Extends
			if i := strings.LastIndex(name, "@"); i > 0 {
				name = name[:i]
			}
			return []string{name}
		})
}

// descendantsOf wakes every definition built on the one that just changed, the
// whole way down, so a published schema does not go on describing a parent that
// has moved.
//
// Direct children are not enough: a middle level whose own spec and revision are
// unchanged emits no update of its own, so anything under it would never hear.
// Rendering is unaffected either way, since it resolves the chain afresh.
//
// The walk is breadth-first and remembers what it has seen, so a chain that
// loops terminates. It stays in one namespace, which is where a parent has to
// live.
func descendantsOf(cli client.Client) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		var reqs []reconcile.Request
		seen := map[string]bool{obj.GetName(): true}
		frontier := []string{obj.GetName()}

		for len(frontier) > 0 {
			var next []string
			for _, name := range frontier {
				children := &v1beta1.TraitDefinitionList{}
				if err := cli.List(ctx, children,
					client.InNamespace(obj.GetNamespace()),
					client.MatchingFields{extendsIndex: name}); err != nil {
					// Nothing useful to do from a map function but say so: the
					// alternative is a silently stale schema.
					klog.ErrorS(err, "listing what extends a trait definition, so its descendants may not refresh",
						"namespace", obj.GetNamespace(), "name", name)
					continue
				}
				for _, child := range children.Items {
					if seen[child.Name] {
						continue
					}
					seen[child.Name] = true
					next = append(next, child.Name)
					reqs = append(reqs, reconcile.Request{
						NamespacedName: types.NamespacedName{Namespace: child.Namespace, Name: child.Name},
					})
				}
			}
			frontier = next
		}
		return reqs
	}
}
