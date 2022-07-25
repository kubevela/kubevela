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
	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/policy/utils"
	pkgutils "github.com/oam-dev/kubevela/pkg/utils"
)

// selectReplicateComponents will replicate the components
func selectReplicateComponents(components []common.ApplicationComponent, selectors []string) ([]string, error) {
	var compToReplicate []string
	for _, comp := range components {
		compToReplicate = append(compToReplicate, comp.Name)
	}

	// Select the components to replicate
	compToReplicate = utils.FilterComponents(compToReplicate, selectors)

	if len(compToReplicate) == 0 {
		return nil, errors.New("no component selected to replicate")
	}

	return compToReplicate, nil
}

// GetReplicationComponents will filter the components to replicate, return the replication decisions
func GetReplicationComponents(policies []v1beta1.AppPolicy, components []common.ApplicationComponent) ([]v1alpha1.ReplicationDecision, []common.ApplicationComponent, error) {
	var (
		err                  error
		replicationDecisions []v1alpha1.ReplicationDecision
		compToRep            []string
		compToKeep           = make(map[string]bool)
	)
	existReplicationPolicy := false
	for _, policy := range policies {
		if policy.Type == v1alpha1.ReplicationPolicyType {
			existReplicationPolicy = true
			replicateSpec := &v1alpha1.ReplicationPolicySpec{}
			if err := pkgutils.StrictUnmarshal(policy.Properties.Raw, replicateSpec); err != nil {
				return nil, nil, errors.Wrapf(err, "failed to parse replicate policy %s", policy.Name)
			}
			compToRep, err = selectReplicateComponents(components, replicateSpec.Selector)
			if err != nil {
				return nil, nil, errors.Wrapf(err, "failed to apply replicate policy %s", policy.Name)
			}
			replicationDecisions = append(replicationDecisions, v1alpha1.ReplicationDecision{
				Keys:       replicateSpec.Keys,
				Components: compToRep,
			})
			for _, comp := range compToRep {
				compToKeep[comp] = true
			}
		}
	}

	if !existReplicationPolicy {
		return nil, components, nil
	}
	filteredComps := make([]common.ApplicationComponent, 0, len(components))
	for _, comp := range components {
		if _, found := compToKeep[comp.Name]; found {
			filteredComps = append(filteredComps, comp)
		}
	}
	return replicationDecisions, filteredComps, nil
}
