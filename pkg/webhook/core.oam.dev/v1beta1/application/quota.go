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
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/pkg/controller/sharding"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velacache "github.com/oam-dev/kubevela/pkg/cache"
)

// quotaReader returns the reader to count with, and whether it carries the index.
//
// A sharded cache holds only this shard's Applications, and the webhook runs on the
// master, so counting from it would miss most of the namespace.
//
// Unsharded it reads the cache. A live read would narrow the window in which a
// just-admitted Application is not yet visible, but not close it, because the count
// is not transactional either way. It would cost a whole namespace list per
// admission, which no field selector narrows on a CRD.
func (h *ValidatingHandler) quotaReader() (client.Reader, bool) {
	if sharding.EnableSharding && h.APIReader != nil {
		return h.APIReader, false
	}
	return h.Client, velacache.DefinitionUsageIndexed
}

// countUsage counts one definition's uses across the namespace, skipping the
// Application named excluding.
//
// Excluding the incoming Application is what lets it be edited at the limit: its
// stored uses would otherwise be counted alongside the ones replacing them. Names
// are unique within a namespace, so a name is enough to find it.
func countUsage(ctx context.Context, c client.Reader, indexed bool, namespace, kind, name, excluding string) (int, error) {
	apps := &v1beta1.ApplicationList{}

	// Read-only, so the cache need not deep copy what it returns.
	opts := []client.ListOption{client.InNamespace(namespace), client.UnsafeDisableDeepCopy}
	if indexed {
		// Narrows the list to Applications using this definition.
		opts = append(opts, client.MatchingFields{velacache.DefinitionUsageIndex: velacache.UsageKey(kind, name)})
	}
	if err := c.List(ctx, apps, opts...); err != nil {
		return 0, err
	}

	total := 0
	for i := range apps.Items {
		app := &apps.Items[i]
		if app.Name == excluding {
			continue
		}
		total += usageInApp(app, kind, name)
	}
	return total, nil
}

// usageInApp counts a definition's occurrences in one Application, so two
// components of the same type, or one trait on two components, count twice.
func usageInApp(app *v1beta1.Application, kind, name string) int {
	n := 0
	for _, comp := range app.Spec.Components {
		if kind == velacache.UsageComponent {
			if velacache.BaseTypeName(comp.Type) == name {
				n++
			}
			continue
		}
		for _, tr := range comp.Traits {
			if velacache.BaseTypeName(tr.Type) == name {
				n++
			}
		}
	}
	return n
}
