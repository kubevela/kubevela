/*
Copyright 2023 The KubeVela Authors.

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

package cache

import (
	"context"
	"strings"

	"github.com/kubevela/pkg/controller/sharding"
	"github.com/kubevela/pkg/util/k8s"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// AppIndex identify the index for resourcetracker to accelerate cache retrieval
const AppIndex = "app"

// DefinitionUsageIndex indexes Applications by the definitions they use, so a quota
// can be counted without walking the namespace. Values are kind-prefixed, since a
// component type and a trait type may share a name and carry separate quotas. An
// Application appears under each key it uses, pin stripped, so webservice@v1 counts
// against webservice.
const DefinitionUsageIndex = "definitionUsage"

// The kinds a quota can count. Only these two: a policy, workflow step or source is
// not a thing a namespace accumulates.
const (
	UsageComponent = "component"
	UsageTrait     = "trait"
)

// UsageKey is the index value for one definition of one kind.
func UsageKey(kind, definitionType string) string {
	return kind + "/" + BaseTypeName(definitionType)
}

var (
	// OptimizeListOp optimize ResourceTracker & ApplicationRevision list op by adding index
	// used in controller optimization (informer index). Client side should not use it.
	OptimizeListOp = false

	// DefinitionUsageIndexed reports whether DefinitionUsageIndex was registered.
	// Querying an index that was not fails the request, so callers read this rather
	// than work the condition out again.
	DefinitionUsageIndexed = false
)

// BuildCache if optimize-list-op enabled, ResourceTracker and ApplicationRevision will be cached by
// application namespace & name
func BuildCache(ctx context.Context, opts cache.Options, shardingObjects ...client.Object) cache.NewCacheFunc {
	AddInformerTransformFuncToCacheOption(&opts)
	fn := sharding.BuildCacheWithOptions(shardingObjects...)
	return func(config *rest.Config, opts cache.Options) (cache.Cache, error) {
		c, err := fn(config, opts)
		if err != nil {
			return nil, err
		}
		if err = registerIndexes(ctx, c); err != nil {
			return nil, err
		}
		if utilfeature.DefaultMutableFeatureGate.Enabled(features.SharedDefinitionStorageForApplicationRevision) {
			go DefaultDefinitionCache.Get().Start(ctx, c, ApplicationRevisionDefinitionCachePruneDuration)
		}
		return c, nil
	}
}

// registerIndexes adds the informer indexes the controller lists by. It takes a
// FieldIndexer rather than the cache so it can be tested: IndexField needs an API
// server for discovery, and a cache cannot be built without one.
func registerIndexes(ctx context.Context, idx client.FieldIndexer) error {
	// Cleared first, so a run that skips the index cannot leave the flag set from
	// an earlier one and send a MatchingFields list at an index that is not there.
	DefinitionUsageIndexed = false
	if !OptimizeListOp {
		return nil
	}
	if err := idx.IndexField(ctx, &v1beta1.ResourceTracker{}, AppIndex, func(obj client.Object) []string {
		return []string{k8s.GetLabel(obj, oam.LabelAppNamespace) + "/" + k8s.GetLabel(obj, oam.LabelAppName)}
	}); err != nil {
		return err
	}
	if err := idx.IndexField(ctx, &v1beta1.ApplicationRevision{}, AppIndex, func(obj client.Object) []string {
		return []string{obj.GetNamespace() + "/" + k8s.GetLabel(obj, oam.LabelAppName)}
	}); err != nil {
		return err
	}
	if !indexDefinitionUsage() {
		return nil
	}
	if err := idx.IndexField(ctx, &v1beta1.Application{}, DefinitionUsageIndex, func(obj client.Object) []string {
		app, ok := obj.(*v1beta1.Application)
		if !ok {
			return nil
		}
		return DefinitionUsageOf(app)
	}); err != nil {
		return err
	}
	DefinitionUsageIndexed = true
	return nil
}

// indexDefinitionUsage reports whether to register DefinitionUsageIndex. It costs
// memory per Application and time on every watch event, and only the quota check
// reads it, so it follows that feature.
func indexDefinitionUsage() bool {
	return OptimizeListOp && utilfeature.DefaultMutableFeatureGate.Enabled(features.RestrictDefinitionNamespaces)
}

// DefinitionUsageOf lists the distinct definitions an Application uses, as kind
// prefixed keys. It backs DefinitionUsageIndex.
func DefinitionUsageOf(app *v1beta1.Application) []string {
	seen := map[string]struct{}{}
	keys := make([]string, 0, len(app.Spec.Components))
	add := func(kind, definitionType string) {
		k := UsageKey(kind, definitionType)
		if _, dup := seen[k]; dup {
			return
		}
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	for _, comp := range app.Spec.Components {
		add(UsageComponent, comp.Type)
		for _, tr := range comp.Traits {
			add(UsageTrait, tr.Type)
		}
	}
	return keys
}

// BaseTypeName strips a version pin: webservice@v1 names the definition webservice.
func BaseTypeName(componentType string) string {
	if base, _, found := strings.Cut(componentType, "@"); found {
		return base
	}
	return componentType
}
