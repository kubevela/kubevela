/*
Copyright 2021 The KubeVela Authors.

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

package resourcetracker

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// appIndex identify the index for resourcetracker to accelerate cache retrieval
const appIndex = "app"

var (
	// OptimizeListOp optimize ResourceTracker List Op by adding index
	OptimizeListOp = true
)

// ExtendResourceTrackerListOption wraps list rt options by adding indexing fields
func ExtendResourceTrackerListOption(list client.ObjectList, opts []client.ListOption) []client.ListOption {
	if !OptimizeListOp {
		return opts
	}
	if _, ok := list.(*v1beta1.ResourceTrackerList); ok {
		for _, opt := range opts {
			if ml, isML := opt.(client.MatchingLabels); isML {
				appName := ml[oam.LabelAppName]
				appNs := ml[oam.LabelAppNamespace]
				if appName != "" {
					opts = append(opts, client.MatchingFields(map[string]string{
						appIndex: appNs + "/" + appName,
					}))
				}
			}
		}
	}
	return opts
}

// AddResourceTrackerCacheIndex add indexing configuration for cache
func AddResourceTrackerCacheIndex(cache cache.Cache) error {
	if !OptimizeListOp {
		return nil
	}
	return cache.IndexField(context.Background(), &v1beta1.ResourceTracker{}, appIndex, func(obj client.Object) []string {
		if labels := obj.GetLabels(); labels != nil {
			return []string{labels[oam.LabelAppNamespace] + "/" + labels[oam.LabelAppName]}
		}
		return []string{}
	})
}
