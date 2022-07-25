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

package docgen

import (
	"context"
	"fmt"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// LoadCapabilityByName will load capability from local by name
func LoadCapabilityByName(name string, userNamespace string, c common.Args) (types.Capability, error) {
	caps, err := LoadAllInstalledCapability(userNamespace, c)
	if err != nil {
		return types.Capability{}, err
	}
	for _, c := range caps {
		if c.Name == name {
			return c, nil
		}
	}
	return types.Capability{}, fmt.Errorf("%s not found", name)
}

// LoadAllInstalledCapability will list all capability
func LoadAllInstalledCapability(userNamespace string, c common.Args) ([]types.Capability, error) {
	caps, err := GetCapabilitiesFromCluster(context.TODO(), userNamespace, c, nil)
	if err != nil {
		return nil, err
	}
	systemCaps, err := GetCapabilitiesFromCluster(context.TODO(), types.DefaultKubeVelaNS, c, nil)
	if err != nil {
		return nil, err
	}
	caps = append(caps, systemCaps...)
	return caps, nil
}

// LoadInstalledCapabilityWithType will load cap list by type
func LoadInstalledCapabilityWithType(userNamespace string, c common.Args, capT types.CapType) ([]types.Capability, error) {
	switch capT {
	case types.TypeComponentDefinition:
		caps, _, err := GetComponentsFromCluster(context.TODO(), userNamespace, c, nil)
		if err != nil {
			return nil, err
		}
		if userNamespace != types.DefaultKubeVelaNS {
			systemCaps, _, err := GetComponentsFromCluster(context.TODO(), types.DefaultKubeVelaNS, c, nil)
			if err != nil {
				return nil, err
			}
			caps = append(caps, systemCaps...)
		}
		return caps, nil
	case types.TypeTrait:
		caps, _, err := GetTraitsFromCluster(context.TODO(), userNamespace, c, nil)
		if err != nil {
			return nil, err
		}
		if userNamespace != types.DefaultKubeVelaNS {
			systemCaps, _, err := GetTraitsFromCluster(context.TODO(), types.DefaultKubeVelaNS, c, nil)
			if err != nil {
				return nil, err
			}
			caps = append(caps, systemCaps...)
		}
		return caps, nil
	case types.TypeScope:
	case types.TypeWorkload:
	default:
	}
	return nil, nil
}
