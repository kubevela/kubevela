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
	"slices"

	wfTypesv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/workflow/step"
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
	spec["workflow"] = map[string]interface{}{"mode": workflowMode(workflowv1alpha1.WorkflowModeStep), "steps": steps}
}

// workflowMode is an explicit workflow mode. Without one, the controller runs
// steps written in spec one by one and generated steps as a DAG, with sub-steps
// as a DAG in both cases. See writeExplicitWorkflow.
func workflowMode(steps wfTypesv1alpha1.WorkflowMode) map[string]interface{} {
	return map[string]interface{}{"steps": string(steps), "subSteps": string(workflowv1alpha1.WorkflowModeDAG)}
}

// writeExplicitWorkflow makes the addon Application's workflow explicit, so
// nothing from an earlier render can stay in it.
//
// The addon Application is patched with no last-applied configuration (see
// suppressLastAppliedConfig), so the patch can add and change fields but never
// delete one. A workflow left out of a render would outlive it: the one
// pinModuleComponentsToHub writes for a placement, after the placement is
// removed, or default steps for a module the next addon version drops. Their
// steps would name policies or components that are gone. A workflow with
// steps is replaced as a whole instead, so:
//
//   - With no workflow, it writes the one the controller would generate: the
//     same steps from the same generators, run as a DAG.
//   - Steps with no mode get the mode the controller uses for steps written in
//     spec, so a mode from an earlier render cannot change how they run.
//
// A workflow that names a Workflow by ref takes its mode from that Workflow
// when it sets none, so it is left as it is. Every value written here
// survives a round trip through the typed Application unchanged; anything
// that does not (an empty step list, for one) would flip on every pass.
func writeExplicitWorkflow(app map[string]interface{}) error {
	spec, _ := app["spec"].(map[string]interface{})
	if spec == nil {
		return nil
	}
	if wf, hasWorkflow := spec["workflow"].(map[string]interface{}); hasWorkflow {
		steps, _ := wf["steps"].([]interface{})
		ref, _ := wf["ref"].(string)
		if _, hasMode := wf["mode"]; !hasMode && ref == "" && len(steps) > 0 {
			wf["mode"] = workflowMode(workflowv1alpha1.WorkflowModeStep)
		}
		return nil
	}
	typed := &v1beta1.Application{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(app, typed); err != nil {
		return fmt.Errorf("read the addon Application: %w", err)
	}
	steps, err := step.NewChainWorkflowStepGenerator(
		&step.DeployWorkflowStepGenerator{},
		&step.Deploy2EnvWorkflowStepGenerator{},
		&step.ApplyComponentWorkflowStepGenerator{},
	).Generate(typed, nil)
	if err != nil {
		return fmt.Errorf("generate the default workflow: %w", err)
	}
	if len(steps) == 0 {
		return nil
	}
	raw, err := json.Marshal(steps)
	if err != nil {
		return fmt.Errorf("encode the default workflow: %w", err)
	}
	var list []interface{}
	if err := json.Unmarshal(raw, &list); err != nil {
		return fmt.Errorf("encode the default workflow: %w", err)
	}
	spec["workflow"] = map[string]interface{}{"mode": workflowMode(workflowv1alpha1.WorkflowModeDAG), "steps": list}
	return nil
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
