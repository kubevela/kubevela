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

package service

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/policy/placement"
)

// addonPlacementPolicy names the topology policy a type: addon placement adds.
const addonPlacementPolicy = "addon-placement"

// applyAddonPlacement turns the installer's placement on a type: addon
// component into the addon's placement. The addon gets a topology policy built
// from it, and every imported module that sets no placement of its own gets
// the same placement, so the module's runtime objects follow the addon.
//
// An addon that defines its own topology policy decides where it goes, usually
// from its own parameters, so a placement request is refused rather than
// silently replacing or ignoring the author's topology. A nil place changes
// nothing.
func applyAddonPlacement(app *v1beta1.Application, addonName string, place *v1alpha1.Placement, moduleComps []common2.ApplicationComponent) error {
	if place == nil {
		return nil
	}
	for _, p := range app.Spec.Policies {
		if p.Type == v1alpha1.TopologyPolicyType {
			return fmt.Errorf("addon %q places itself with its topology policy %q, so placement cannot be set; use the addon's own parameters instead", addonName, p.Name)
		}
	}
	props := placement.TopologyProperties(*place)
	raw, err := json.Marshal(props)
	if err != nil {
		return fmt.Errorf("encode placement for addon %q: %w", addonName, err)
	}
	app.Spec.Policies = append(app.Spec.Policies, v1beta1.AppPolicy{
		Name:       addonPlacementPolicy,
		Type:       v1alpha1.TopologyPolicyType,
		Properties: &runtime.RawExtension{Raw: raw},
	})

	for i := range moduleComps {
		var compProps map[string]interface{}
		if moduleComps[i].Properties != nil && len(moduleComps[i].Properties.Raw) > 0 {
			if err := json.Unmarshal(moduleComps[i].Properties.Raw, &compProps); err != nil {
				return fmt.Errorf("read module component %q of addon %q: %w", moduleComps[i].Name, addonName, err)
			}
		}
		if compProps == nil {
			compProps = map[string]interface{}{}
		}
		if _, own := compProps["placement"]; own {
			continue
		}
		compProps["placement"] = props
		b, err := json.Marshal(compProps)
		if err != nil {
			return fmt.Errorf("encode module component %q of addon %q: %w", moduleComps[i].Name, addonName, err)
		}
		moduleComps[i].Properties = &runtime.RawExtension{Raw: b}
	}
	return nil
}
