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

	"github.com/kubevela/pkg/controller/sharding"
	"github.com/kubevela/pkg/util/k8s"
	"k8s.io/apimachinery/pkg/runtime"
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

var (
	// OptimizeListOp optimize ResourceTracker & ApplicationRevision list op by adding index
	// used in controller optimization (informer index). Client side should not use it.
	OptimizeListOp = false
)

// BuildCache if optimize-list-op enabled, ResourceTracker and ApplicationRevision will be cached by
// application namespace & name
func BuildCache(ctx context.Context, scheme *runtime.Scheme, shardingObjects ...client.Object) cache.NewCacheFunc {
	opts := cache.Options{Scheme: scheme}
	AddInformerTransformFuncToCacheOption(&opts)
	fn := sharding.BuildCacheWithOptions(opts, shardingObjects...)
	return func(config *rest.Config, opts cache.Options) (cache.Cache, error) {
		c, err := fn(config, opts)
		if err != nil {
			return nil, err
		}
		if OptimizeListOp {
			if err = c.IndexField(ctx, &v1beta1.ResourceTracker{}, AppIndex, func(obj client.Object) []string {
				return []string{k8s.GetLabel(obj, oam.LabelAppNamespace) + "/" + k8s.GetLabel(obj, oam.LabelAppName)}
			}); err != nil {
				return nil, err
			}
			if err = c.IndexField(ctx, &v1beta1.ApplicationRevision{}, AppIndex, func(obj client.Object) []string {
				return []string{obj.GetNamespace() + "/" + k8s.GetLabel(obj, oam.LabelAppName)}
			}); err != nil {
				return nil, err
			}
		}
		if utilfeature.DefaultMutableFeatureGate.Enabled(features.SharedDefinitionStorageForApplicationRevision) {
			go DefaultDefinitionCache.Get().Start(ctx, c, ApplicationRevisionDefinitionCachePruneDuration)
		}
		return c, nil
	}
}
