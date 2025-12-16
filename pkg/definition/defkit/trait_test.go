/*
Copyright 2025 The KubeVela Authors.

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

package defkit

import (
	"strings"
	"testing"
)

func TestTraitDefinition_Basic(t *testing.T) {
	trait := NewTrait("scaler").
		Description("Manually scale K8s pod for your workload.").
		AppliesTo("deployments.apps", "statefulsets.apps").
		PodDisruptive(false).
		Params(
			Int("replicas").Default(1).Description("Specify the number of workload"),
		)

	// Test basic properties
	if trait.DefName() != "scaler" {
		t.Errorf("expected name 'scaler', got %q", trait.DefName())
	}
	if trait.DefType() != DefinitionTypeTrait {
		t.Errorf("expected type trait, got %v", trait.DefType())
	}
	if len(trait.GetAppliesToWorkloads()) != 2 {
		t.Errorf("expected 2 workloads, got %d", len(trait.GetAppliesToWorkloads()))
	}
	if trait.IsPodDisruptive() {
		t.Error("expected podDisruptive to be false")
	}
}

func TestTraitDefinition_PatchTemplate(t *testing.T) {
	replicas := Int("replicas").Default(1)

	trait := NewTrait("scaler").
		Description("Manually scale K8s pod.").
		AppliesTo("deployments.apps", "statefulsets.apps").
		PodDisruptive(false).
		Params(replicas).
		Template(func(tpl *Template) {
			tpl.PatchStrategy("retainKeys").
				Patch().Set("spec.replicas", replicas)
		})

	cue := trait.ToCue()

	// Should contain trait header
	if !strings.Contains(cue, `type: "trait"`) {
		t.Error("expected CUE to contain type: \"trait\"")
	}

	// Should contain attributes
	if !strings.Contains(cue, "podDisruptive: false") {
		t.Error("expected CUE to contain podDisruptive: false")
	}
	if !strings.Contains(cue, `appliesToWorkloads:`) {
		t.Error("expected CUE to contain appliesToWorkloads")
	}

	// Should contain patch
	if !strings.Contains(cue, "patch:") {
		t.Error("expected CUE to contain patch:")
	}

	// Should contain parameter
	if !strings.Contains(cue, "replicas:") {
		t.Error("expected CUE to contain replicas parameter")
	}
}

func TestTraitDefinition_ConflictsWith(t *testing.T) {
	trait := NewTrait("hpa").
		Description("HPA scaler trait.").
		AppliesTo("deployments.apps").
		ConflictsWith("scaler", "cpuscaler")

	if len(trait.GetConflictsWith()) != 2 {
		t.Errorf("expected 2 conflicts, got %d", len(trait.GetConflictsWith()))
	}

	cue := trait.ToCue()
	if !strings.Contains(cue, "conflictsWith:") {
		t.Error("expected CUE to contain conflictsWith")
	}
}

func TestTraitDefinition_Stage(t *testing.T) {
	trait := NewTrait("expose").
		Description("Expose service.").
		Stage("PostDispatch").
		AppliesTo("deployments.apps")

	if trait.GetStage() != "PostDispatch" {
		t.Errorf("expected stage 'PostDispatch', got %q", trait.GetStage())
	}

	cue := trait.ToCue()
	// Check for stage field with value - CUE formatter may add alignment spaces
	if !strings.Contains(cue, `stage:`) || !strings.Contains(cue, `"PostDispatch"`) {
		t.Errorf("expected CUE to contain stage annotation with PostDispatch, got:\n%s", cue)
	}
}

func TestTraitDefinition_CustomStatus(t *testing.T) {
	trait := NewTrait("expose").
		Description("Expose service.").
		AppliesTo("deployments.apps").
		CustomStatus(`message: "Service exposed"`).
		HealthPolicy(`isHealth: true`)

	cue := trait.ToCue()

	if !strings.Contains(cue, "customStatus:") {
		t.Error("expected CUE to contain customStatus")
	}
	if !strings.Contains(cue, "healthPolicy:") {
		t.Error("expected CUE to contain healthPolicy")
	}
}

func TestTraitDefinition_RawCUE(t *testing.T) {
	rawCUE := `scaler: {
	type: "trait"
	description: "Raw CUE trait"
}
template: {
	patch: spec: replicas: parameter.replicas
	parameter: replicas: *1 | int
}`

	trait := NewTrait("scaler").RawCUE(rawCUE)

	// RawCUE with complete definition (containing "template:") is returned with CUE formatting
	// The content should be functionally equivalent (same CUE structure)
	result := trait.ToCue()

	// Check that key parts are present (formatter may adjust spacing)
	if !strings.Contains(result, `scaler:`) {
		t.Error("expected CUE to contain scaler block")
	}
	if !strings.Contains(result, `type:`) || !strings.Contains(result, `"trait"`) {
		t.Error("expected CUE to contain type: trait")
	}
	if !strings.Contains(result, `template:`) {
		t.Error("expected CUE to contain template block")
	}
	if !strings.Contains(result, `patch: spec: replicas: parameter.replicas`) {
		t.Error("expected CUE to contain patch definition")
	}
	if !strings.Contains(result, `parameter: replicas:`) {
		t.Error("expected CUE to contain parameter definition")
	}
}

func TestTraitDefinition_ToYAML(t *testing.T) {
	trait := NewTrait("scaler").
		Description("Scale workload.").
		AppliesTo("deployments.apps").
		PodDisruptive(false).
		Params(Int("replicas").Default(1))

	yamlBytes, err := trait.ToYAML()
	if err != nil {
		t.Fatalf("ToYAML failed: %v", err)
	}

	yaml := string(yamlBytes)

	if !strings.Contains(yaml, "kind: TraitDefinition") {
		t.Error("expected YAML to contain kind: TraitDefinition")
	}
	if !strings.Contains(yaml, "name: scaler") {
		t.Error("expected YAML to contain name: scaler")
	}
	if !strings.Contains(yaml, "appliesToWorkloads:") {
		t.Error("expected YAML to contain appliesToWorkloads")
	}
}

func TestTraitDefinition_WithImports(t *testing.T) {
	trait := NewTrait("expose").
		Description("Expose service.").
		WithImports("strconv", "strings").
		AppliesTo("deployments.apps")

	cue := trait.ToCue()

	if !strings.Contains(cue, `import (`) {
		t.Error("expected CUE to contain import block")
	}
	if !strings.Contains(cue, `"strconv"`) {
		t.Error("expected CUE to contain strconv import")
	}
	if !strings.Contains(cue, `"strings"`) {
		t.Error("expected CUE to contain strings import")
	}
}

func TestTraitDefinition_Registry(t *testing.T) {
	Clear() // Reset registry

	trait1 := NewTrait("scaler").Description("Scale").AppliesTo("deployments.apps")
	trait2 := NewTrait("expose").Description("Expose").AppliesTo("deployments.apps")
	comp := NewComponent("webservice").Description("Component")

	Register(trait1)
	Register(trait2)
	Register(comp)

	// Should have 3 definitions total
	if Count() != 3 {
		t.Errorf("expected 3 registered definitions, got %d", Count())
	}

	// Should have 2 traits
	traits := Traits()
	if len(traits) != 2 {
		t.Errorf("expected 2 traits, got %d", len(traits))
	}

	// Should have 1 component
	comps := Components()
	if len(comps) != 1 {
		t.Errorf("expected 1 component, got %d", len(comps))
	}

	Clear() // Clean up
}
