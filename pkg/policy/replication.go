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
	"k8s.io/kubectl/pkg/util/slice"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	pkgutils "github.com/oam-dev/kubevela/pkg/utils"
)

// selectReplicateComponents will replicate the components
func selectReplicateComponents(components []common.ApplicationComponent, selectors []string) ([]common.ApplicationComponent, error) {
	var compToReplicate = make([]common.ApplicationComponent, 0)
	for _, comp := range components {
		if slice.ContainsString(selectors, comp.Name, nil) {
			compToReplicate = append(compToReplicate, comp)
		}
	}
	if len(compToReplicate) == 0 {
		return nil, errors.New("no component selected to replicate")
	}
	return compToReplicate, nil
}

// ReplicateComponents will filter the components to replicate, return the replication decisions
func ReplicateComponents(policies []v1beta1.AppPolicy, components []common.ApplicationComponent) ([]common.ApplicationComponent, error) {
	var (
		compToRemove = make(map[string]bool)
		compToAdd    = make([]common.ApplicationComponent, 0)
	)
	existReplicationPolicy := false
	for _, policy := range policies {
		if policy.Type == v1alpha1.ReplicationPolicyType {
			existReplicationPolicy = true
			replicateSpec := &v1alpha1.ReplicationPolicySpec{}
			if policy.Properties == nil {
				continue
			}
			if err := pkgutils.StrictUnmarshal(policy.Properties.Raw, replicateSpec); err != nil {
				return nil, errors.Wrapf(err, "failed to parse replicate policy %s", policy.Name)
			}
			compToRep, err := selectReplicateComponents(components, replicateSpec.Selector)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to apply replicate policy %s", policy.Name)
			}
			compToAdd = append(compToAdd, replicateComponents(compToRep, replicateSpec.Keys)...)
			for _, comp := range compToRep {
				compToRemove[comp.Name] = true
			}

		}
	}

	if !existReplicationPolicy {
		return components, nil
	}
	compsAfterReplicate := make([]common.ApplicationComponent, 0, len(components))
	for _, comp := range components {
		if !compToRemove[comp.Name] {
			compsAfterReplicate = append(compsAfterReplicate, comp)
		}
	}
	compsAfterReplicate = append(compsAfterReplicate, compToAdd...)
	return compsAfterReplicate, nil
}

func replicateComponents(comps []common.ApplicationComponent, keys []string) []common.ApplicationComponent {
	compsAfterReplicate := make([]common.ApplicationComponent, 0, len(comps)*len(keys))
	for _, comp := range comps {
		for _, key := range keys {
			compAfterReplicate := comp.DeepCopy()
			compAfterReplicate.ReplicaKey = key
			compsAfterReplicate = append(compsAfterReplicate, *compAfterReplicate)
		}
	}
	return compsAfterReplicate
}
