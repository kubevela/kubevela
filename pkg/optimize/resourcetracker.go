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

package optimize

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

const ResourceTrackerAppIndex = "app"

type resourceTrackerOptimizer struct{
	OptimizeListOp      bool
	EnableDeleteOnlyTrigger bool
	MarkWithProbability float64
}

// ResourceTrackerOptimizer optimizer for ResourceTracker
var ResourceTrackerOptimizer = resourceTrackerOptimizer{}

func (o *resourceTrackerOptimizer) ExtendResourceTrackerListOption(list client.ObjectList, opts []client.ListOption) []client.ListOption {
	if !o.OptimizeListOp {
		return opts
	}
	if _, ok := list.(*v1beta1.ResourceTrackerList); ok {
		for _, opt := range opts {
			if ml, isML := opt.(client.MatchingLabels); isML {
				appName := ml[oam.LabelAppName]
				appNs := ml[oam.LabelAppNamespace]
				if appName != "" {
					opts = append(opts, client.MatchingFields(map[string]string{
						ResourceTrackerAppIndex: appNs + "/" + appName,
					}))
				}
			}
		}
	}
	return opts
}

func (o *resourceTrackerOptimizer) AddResourceTrackerCacheIndex(cache cache.Cache) error {
	if !o.OptimizeListOp {
		return nil
	}
	return cache.IndexField(context.Background(), &v1beta1.ResourceTracker{}, ResourceTrackerAppIndex, func(obj client.Object) []string {
		if labels := obj.GetLabels(); labels != nil {
			return []string{labels[oam.LabelAppNamespace] + "/" + labels[oam.LabelAppName]}
		}
		return []string{}
	})
}