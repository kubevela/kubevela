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
	"context"
	"fmt"

	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	"github.com/pkg/errors"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils"
)

// GetClusterLabelSelectorInTopology get cluster label selector in topology policy spec
func GetClusterLabelSelectorInTopology(topology *v1alpha1.TopologyPolicySpec) map[string]string {
	if topology.ClusterLabelSelector != nil {
		return topology.ClusterLabelSelector
	}
	if utilfeature.DefaultMutableFeatureGate.Enabled(features.DeprecatedPolicySpec) {
		return topology.DeprecatedClusterSelector
	}
	return nil
}

// GetPlacementsFromTopologyPolicies get placements from topology policies with provided client
func GetPlacementsFromTopologyPolicies(ctx context.Context, cli client.Client, appNs string, policies []v1beta1.AppPolicy, allowCrossNamespace bool) ([]v1alpha1.PlacementDecision, error) {
	placements := make([]v1alpha1.PlacementDecision, 0)
	placementMap := map[string]struct{}{}
	addCluster := func(cluster string, ns string, validateCluster bool) error {
		if validateCluster {
			if _, e := multicluster.NewClusterClient(cli).Get(ctx, cluster); e != nil {
				return errors.Wrapf(e, "failed to get cluster %s", cluster)
			}
		}
		if !allowCrossNamespace && (ns != appNs && ns != "") {
			return errors.Errorf("cannot cross namespace")
		}
		placement := v1alpha1.PlacementDecision{Cluster: cluster, Namespace: ns}
		name := placement.String()
		if _, found := placementMap[name]; !found {
			placementMap[name] = struct{}{}
			placements = append(placements, placement)
		}
		return nil
	}
	hasTopologyPolicy := false
	for _, policy := range policies {
		if policy.Type == v1alpha1.TopologyPolicyType {
			if policy.Properties == nil {
				return nil, fmt.Errorf("topology policy %s must not have empty properties", policy.Name)
			}
			hasTopologyPolicy = true
			topologySpec := &v1alpha1.TopologyPolicySpec{}
			if err := utils.StrictUnmarshal(policy.Properties.Raw, topologySpec); err != nil {
				return nil, errors.Wrapf(err, "failed to parse topology policy %s", policy.Name)
			}
			clusterLabelSelector := GetClusterLabelSelectorInTopology(topologySpec)
			switch {
			case topologySpec.Clusters != nil:
				for _, cluster := range topologySpec.Clusters {
					if err := addCluster(cluster, topologySpec.Namespace, true); err != nil {
						return nil, err
					}
				}
			case clusterLabelSelector != nil:
				clusterList, err := multicluster.NewClusterClient(cli).List(ctx, client.MatchingLabels(clusterLabelSelector))
				if err != nil {
					return nil, errors.Wrapf(err, "failed to find clusters in topology %s", policy.Name)
				}
				if len(clusterList.Items) == 0 && !topologySpec.AllowEmpty {
					return nil, errors.New("failed to find any cluster matches given labels")
				}
				for _, cluster := range clusterList.Items {
					if err = addCluster(cluster.Name, topologySpec.Namespace, false); err != nil {
						return nil, err
					}
				}
			default:
				if err := addCluster(pkgmulticluster.Local, topologySpec.Namespace, false); err != nil {
					return nil, err
				}
			}
		}
	}
	if !hasTopologyPolicy {
		placements = []v1alpha1.PlacementDecision{{Cluster: multicluster.ClusterLocalName}}
	}
	return placements, nil
}
