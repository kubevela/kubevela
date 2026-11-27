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
	"strings"

	wfTypesv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/workflow/step"
)

// Base names of the policies and the step pinCrateComponentsToHub adds. A
// name the addon already uses gets a numeric suffix.
const (
	addonComponentsOverride = "addon-components"
	addonCratesTopology     = "addon-crates-hub"
	addonCratesOverride     = "addon-crates"
	addonCratesStep         = "deploy-addon-crates"
)

// pinCrateComponentsToHub keeps an addon's generated crate components on the
// hub when the addon places itself with topology policies. These are the
// type: crate components of its imports and the components that hold an
// inline crate's owned Application. Either one renders the crate's owned
// Application, which has to live where KubeVela runs; the crate splits its
// own objects between the hub and the clusters its placement names. Without
// this, the addon's topology would send the crate component itself to those
// clusters.
//
// It writes the workflow KubeVela would generate for the addon (one deploy
// step per topology policy, with every override policy) restricted to the
// addon's other components, then one step that deploys the crate components
// to the local cluster with the same override policies. A deploy step only
// waits for components it deploys itself, so the crate components lose their
// dependsOn; the step order keeps them after the addon's own components are
// healthy.
//
// An addon with no topology policy already installs on the hub, and an addon
// that writes its own workflow decides placement itself, so both are left as
// they are.
func pinCrateComponentsToHub(app map[string]interface{}, crateNames []string) {
	spec, _ := app["spec"].(map[string]interface{})
	if spec == nil || len(crateNames) == 0 {
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
		if slices.Contains(crateNames, name) {
			delete(comp, "dependsOn")
			continue
		}
		others = append(others, name)
	}

	componentsOverride := claimName(addonComponentsOverride, used)
	cratesTopology := claimName(addonCratesTopology, used)
	cratesOverride := claimName(addonCratesOverride, used)
	spec["policies"] = append(policies,
		policyMap(componentsOverride, v1alpha1.OverridePolicyType, map[string]interface{}{"selector": others}),
		policyMap(cratesTopology, v1alpha1.TopologyPolicyType, map[string]interface{}{"clusters": []interface{}{pkgmulticluster.Local}}),
		policyMap(cratesOverride, v1alpha1.OverridePolicyType, map[string]interface{}{"selector": stringsToList(crateNames)}),
	)

	stepNames := map[string]bool{}
	steps := make([]interface{}, 0, len(topologies)+1)
	for _, topology := range topologies {
		stepPolicies := append(append(stringsToList(overrides), componentsOverride), topology)
		steps = append(steps, deployStepMap(claimName("deploy-"+topology, stepNames), stepPolicies))
	}
	cratePolicies := append(append(stringsToList(overrides), cratesTopology), cratesOverride)
	steps = append(steps, deployStepMap(claimName(addonCratesStep, stepNames), cratePolicies))
	spec["workflow"] = map[string]interface{}{"mode": workflowMode(workflowv1alpha1.WorkflowModeStep), "steps": steps}
}

// workflowMode is an explicit workflow mode. Without one, the controller runs
// steps written in spec one by one and generated steps as a DAG, with sub-steps
// as a DAG in both cases. See writeExplicitWorkflow.
func workflowMode(steps wfTypesv1alpha1.WorkflowMode) map[string]interface{} {
	return map[string]interface{}{"steps": string(steps), "subSteps": string(workflowv1alpha1.WorkflowModeDAG)}
}

// completeMode returns mode with every key set, taking a key the addon left
// out from the mode the controller uses for steps written in spec. A key left
// out would otherwise keep the value an earlier render wrote.
func completeMode(mode interface{}) map[string]interface{} {
	out := workflowMode(workflowv1alpha1.WorkflowModeStep)
	set, _ := mode.(map[string]interface{})
	for key, value := range set {
		if s, ok := value.(string); ok && s != "" {
			out[key] = s
		}
	}
	return out
}

// writeExplicitWorkflow makes the addon Application's workflow explicit, so
// nothing from an earlier render can stay in it.
//
// The addon Application is patched with no last-applied configuration (see
// suppressLastAppliedConfig), so the patch can add and change fields but
// deletes only a field the render sets to null. A workflow field left out of a
// render would outlive it: the workflow pinCrateComponentsToHub writes for a
// placement, after the placement is removed. Default steps for a crate the
// next addon version drops. A ref the next version replaces with steps. Its
// steps would name policies or components that are gone, or sit next to a ref
// that cannot have steps. So every render states the whole workflow:
//
//   - With no workflow, it writes the one the controller would generate: the
//     same steps from the same generators, run as a DAG, and no ref. When the
//     generators give no step, it removes the workflow.
//   - Steps the addon wrote get no ref and a complete mode. A mode key the
//     addon leaves out is the one the controller uses for steps written in
//     spec, not one an earlier render wrote. Its steps never name the
//     components the render adds, so each of those gets an apply-component
//     step of its own: before is applied ahead of the addon's steps, after
//     behind them. Run as a DAG, the steps carry the same order as dependsOn.
//   - A ref gets no steps. A mode the addon wrote next to it stays. With none,
//     the mode is removed, because it comes from the Workflow the ref names.
//     A ref cannot apply the components the render adds, so an addon that
//     adds any is refused.
//
// A null removes its field from the patched Application and reads as no field
// in the typed Application, so writing it again changes nothing. Every other
// value written here survives a round trip through the typed Application
// unchanged; one that does not (an empty step list, for one) would flip on
// every pass.
func writeExplicitWorkflow(app map[string]interface{}, before, after []string) error {
	spec, _ := app["spec"].(map[string]interface{})
	if spec == nil {
		return nil
	}
	if wf, hasWorkflow := spec["workflow"].(map[string]interface{}); hasWorkflow {
		steps, _ := wf["steps"].([]interface{})
		ref, _ := wf["ref"].(string)
		switch {
		case ref != "":
			if added := append(append([]string{}, before...), after...); len(added) > 0 {
				return fmt.Errorf("the workflow is the ref %q, so it cannot apply the components the addon adds (%s); write the workflow as steps instead", ref, strings.Join(added, ", "))
			}
			wf["steps"] = nil
			if _, hasMode := wf["mode"]; !hasMode {
				wf["mode"] = nil
			}
		case len(steps) > 0:
			wf["ref"] = nil
			mode := completeMode(wf["mode"])
			wf["mode"] = mode
			wf["steps"] = applyAddedComponents(steps, before, after, mode["steps"] == string(workflowv1alpha1.WorkflowModeDAG))
		}
		return nil
	}
	steps, err := step.NewChainWorkflowStepGenerator(
		&step.DeployWorkflowStepGenerator{},
		&step.Deploy2EnvWorkflowStepGenerator{},
		&step.ApplyComponentWorkflowStepGenerator{},
	).Generate(workflowInputs(spec), nil)
	if err != nil {
		return fmt.Errorf("generate the default workflow: %w", err)
	}
	if len(steps) == 0 {
		spec["workflow"] = nil
		return nil
	}
	list := make([]interface{}, 0, len(steps))
	for i := range steps {
		item, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&steps[i])
		if err != nil {
			return fmt.Errorf("encode the default workflow: %w", err)
		}
		list = append(list, item)
	}
	spec["workflow"] = map[string]interface{}{"mode": workflowMode(workflowv1alpha1.WorkflowModeDAG), "steps": list, "ref": nil}
	return nil
}

// workflowInputs returns an Application with only what the default step
// generators read: each component's name, type and dependsOn, and each
// policy's name and type, with the properties of an env-binding policy. It
// leaves out component properties, so a render does not decode the manifests
// of a large k8s-objects component only to name its steps.
func workflowInputs(spec map[string]interface{}) *v1beta1.Application {
	app := &v1beta1.Application{}
	components, _ := spec["components"].([]interface{})
	for _, raw := range components {
		comp, _ := raw.(map[string]interface{})
		name, _ := comp["name"].(string)
		typ, _ := comp["type"].(string)
		dependsOn, _ := comp["dependsOn"].([]interface{})
		item := common.ApplicationComponent{Name: name, Type: typ}
		for _, dep := range dependsOn {
			if s, ok := dep.(string); ok {
				item.DependsOn = append(item.DependsOn, s)
			}
		}
		app.Spec.Components = append(app.Spec.Components, item)
	}
	policies, _ := spec["policies"].([]interface{})
	for _, raw := range policies {
		policy, _ := raw.(map[string]interface{})
		name, _ := policy["name"].(string)
		typ, _ := policy["type"].(string)
		item := v1beta1.AppPolicy{Name: name, Type: typ}
		if properties, ok := policy["properties"]; ok && typ == v1alpha1.EnvBindingPolicyType {
			if encoded, err := json.Marshal(properties); err == nil {
				item.Properties = &runtime.RawExtension{Raw: encoded}
			}
		}
		app.Spec.Policies = append(app.Spec.Policies, item)
	}
	return app
}

// applyAddedComponents returns the addon's own steps with an apply-component
// step for each component in before ahead of them and each in after behind
// them, named apart from the addon's steps. In a DAG the addon's steps depend
// on the steps for before, and the steps for after on every other step.
func applyAddedComponents(steps []interface{}, before, after []string, dag bool) []interface{} {
	used := map[string]bool{}
	for _, raw := range steps {
		if s, ok := raw.(map[string]interface{}); ok {
			name, _ := s["name"].(string)
			used[name] = true
		}
	}
	out := make([]interface{}, 0, len(before)+len(steps)+len(after))
	var earlier []string
	for _, comp := range before {
		name := claimName(comp, used)
		out = append(out, applyComponentStepMap(name, comp))
		earlier = append(earlier, name)
	}
	firstAdded := slices.Clone(earlier)
	for _, raw := range steps {
		s, ok := raw.(map[string]interface{})
		if ok {
			name, _ := s["name"].(string)
			earlier = append(earlier, name)
			if dag && len(firstAdded) > 0 {
				s["dependsOn"] = appendMissing(s["dependsOn"], firstAdded)
			}
		}
		out = append(out, raw)
	}
	for _, comp := range after {
		s := applyComponentStepMap(claimName(comp, used), comp)
		if dag {
			s["dependsOn"] = stringsToList(earlier)
		}
		out = append(out, s)
	}
	return out
}

// appendMissing returns list with each of names it lacks appended.
func appendMissing(list interface{}, names []string) []interface{} {
	out, _ := list.([]interface{})
	for _, n := range names {
		if !slices.Contains(out, interface{}(n)) {
			out = append(out, n)
		}
	}
	return out
}

// applyComponentStepMap is a step applying component, named name.
func applyComponentStepMap(name, component string) map[string]interface{} {
	return map[string]interface{}{"name": name, "type": "apply-component", "properties": map[string]interface{}{"component": component}}
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
