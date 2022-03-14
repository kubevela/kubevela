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

package policy

import (
	utilfeature "k8s.io/apiserver/pkg/util/feature"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/features"
)

// GetClusterLabelSelectorInTopology get cluster label selector in topology policy spec
func GetClusterLabelSelectorInTopology(topology *v1alpha1.TopologyPolicySpec) map[string]string {
	if topology.ClusterLabelSelector != nil {
		return topology.ClusterLabelSelector
	}
	if utilfeature.DefaultMutableFeatureGate.Enabled(features.DeprecatedPolicySpecCompatible) {
		return topology.DeprecatedClusterSelector
	}
	return nil
}
