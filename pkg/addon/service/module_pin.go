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
	"slices"

	pkgmulticluster "github.com/kubevela/pkg/multicluster"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
)

// Base names of the policies and the step pinModuleComponentsToHub adds. A
// name the addon already uses gets a numeric suffix.
const (
	addonComponentsOverride = "addon-components"
	addonModulesTopology    = "addon-modules-hub"
	addonModulesOverride    = "addon-modules"
	addonModulesStep        = "deploy-addon-modules"
)

// pinModuleComponentsToHub keeps an addon's generated module components on the
// hub when the addon places itself with topology policies. These are the
// type: module components of its imports and the components that hold an
// inline module's owned Application. Either one renders the module's owned
// Application, which has to live where KubeVela runs; the module splits its
// own objects between the hub and the clusters its placement names. Without
// this, the addon's topology would send the module component itself to those
// clusters.
//
// It writes the workflow KubeVela would generate for the addon (one deploy
// step per topology policy, with every override policy) restricted to the
// addon's other components, then one step that deploys the module components
// to the local cluster with the same override policies. A deploy step only
// waits for components it deploys itself, so the module components lose their
// dependsOn; the step order keeps them after the addon's own components are
// healthy.
//
// An addon with no topology policy already installs on the hub, and an addon
// that writes its own workflow decides placement itself, so both are left as
// they are.
func pinModuleComponentsToHub(app map[string]interface{}, moduleNames []string) {
	spec, _ := app["spec"].(map[string]interface{})
	if spec == nil || len(moduleNames) == 0 {
		return
	}
	if _, hasWorkflow := spec["workflow"]; hasWorkflow {
		return
	}
	policies, _ := spec["policies"].([]interface{})
	used := map[string]bool{}
	var topologies, overrides []string
	for _, raw := range policies {
		policy, _ := raw.(map[string]interface{})
		name, _ := policy["name"].(string)
		used[name] = true
		switch policy["type"] {
		case v1alpha1.TopologyPolicyType:
			topologies = append(topologies, name)
		case v1alpha1.OverridePolicyType:
			overrides = append(overrides, name)
		}
	}
	if len(topologies) == 0 {
		return
	}

	components, _ := spec["components"].([]interface{})
	others := []interface{}{}
	for _, raw := range components {
		comp, _ := raw.(map[string]interface{})
		name, _ := comp["name"].(string)
		if slices.Contains(moduleNames, name) {
			delete(comp, "dependsOn")
			continue
		}
		others = append(others, name)
	}

	componentsOverride := claimName(addonComponentsOverride, used)
	modulesTopology := claimName(addonModulesTopology, used)
	modulesOverride := claimName(addonModulesOverride, used)
	spec["policies"] = append(policies,
		policyMap(componentsOverride, v1alpha1.OverridePolicyType, map[string]interface{}{"selector": others}),
		policyMap(modulesTopology, v1alpha1.TopologyPolicyType, map[string]interface{}{"clusters": []interface{}{pkgmulticluster.Local}}),
		policyMap(modulesOverride, v1alpha1.OverridePolicyType, map[string]interface{}{"selector": stringsToList(moduleNames)}),
	)

	stepNames := map[string]bool{}
	steps := make([]interface{}, 0, len(topologies)+1)
	for _, topology := range topologies {
		stepPolicies := append(append(stringsToList(overrides), componentsOverride), topology)
		steps = append(steps, deployStepMap(claimName("deploy-"+topology, stepNames), stepPolicies))
	}
	modulePolicies := append(append(stringsToList(overrides), modulesTopology), modulesOverride)
	steps = append(steps, deployStepMap(claimName(addonModulesStep, stepNames), modulePolicies))
	spec["workflow"] = map[string]interface{}{"steps": steps}
}

// claimName returns a name from base that used does not hold yet, and marks it
// used.
func claimName(base string, used map[string]bool) string {
	name := uniqueComponentName(base, used)
	used[name] = true
	return name
}

func policyMap(name, typ string, properties map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"name": name, "type": typ, "properties": properties}
}

func deployStepMap(name string, policies []interface{}) map[string]interface{} {
	return map[string]interface{}{"name": name, "type": "deploy", "properties": map[string]interface{}{"policies": policies}}
}

func stringsToList(in []string) []interface{} {
	out := make([]interface{}, 0, len(in))
	for _, s := range in {
		out = append(out, s)
	}
	return out
}
