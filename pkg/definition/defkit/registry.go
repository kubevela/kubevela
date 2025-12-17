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
	"encoding/json"
	"sync"

	"github.com/oam-dev/kubevela/pkg/definition/defkit/placement"
)

// DefinitionType represents the type of a KubeVela definition.
type DefinitionType string

const (
	// DefinitionTypeComponent is a ComponentDefinition
	DefinitionTypeComponent DefinitionType = "component"
	// DefinitionTypeTrait is a TraitDefinition
	DefinitionTypeTrait DefinitionType = "trait"
	// DefinitionTypePolicy is a PolicyDefinition
	DefinitionTypePolicy DefinitionType = "policy"
	// DefinitionTypeWorkflowStep is a WorkflowStepDefinition
	DefinitionTypeWorkflowStep DefinitionType = "workflow-step"
)

// Definition is the interface that all KubeVela X-Definitions implement.
// This interface enables runtime discovery and registry-based loading.
type Definition interface {
	// DefName returns the definition's name (e.g., "webservice", "scaler").
	DefName() string
	// DefType returns the definition type (component, trait, policy, workflow-step).
	DefType() DefinitionType
	// ToCue generates the complete CUE template for this definition.
	ToCue() string
	// ToYAML generates the Kubernetes CR YAML for this definition.
	ToYAML() ([]byte, error)
	// GetPlacement returns the placement constraints for this definition.
	GetPlacement() placement.PlacementSpec
	// HasPlacement returns true if the definition has placement constraints.
	HasPlacement() bool
}

// registry is the global definition registry.
// It is populated by init() functions in definition packages.
var (
	registry     []Definition
	registryLock sync.Mutex
)

// Register adds a definition to the global registry.
// This is typically called from init() functions in definition packages.
//
// Example usage:
//
//	func init() {
//	    defkit.Register(Webservice())
//	}
func Register(def Definition) {
	registryLock.Lock()
	defer registryLock.Unlock()
	registry = append(registry, def)
}

// All returns all registered definitions.
func All() []Definition {
	registryLock.Lock()
	defer registryLock.Unlock()
	// Return a copy to prevent modification
	result := make([]Definition, len(registry))
	copy(result, registry)
	return result
}

// Components returns all registered ComponentDefinitions.
func Components() []*ComponentDefinition {
	registryLock.Lock()
	defer registryLock.Unlock()
	var result []*ComponentDefinition
	for _, def := range registry {
		if comp, ok := def.(*ComponentDefinition); ok {
			result = append(result, comp)
		}
	}
	return result
}

// Traits returns all registered TraitDefinitions.
func Traits() []*TraitDefinition {
	registryLock.Lock()
	defer registryLock.Unlock()
	var result []*TraitDefinition
	for _, def := range registry {
		if trait, ok := def.(*TraitDefinition); ok {
			result = append(result, trait)
		}
	}
	return result
}

// Policies returns all registered PolicyDefinitions.
func Policies() []*PolicyDefinition {
	registryLock.Lock()
	defer registryLock.Unlock()
	var result []*PolicyDefinition
	for _, def := range registry {
		if policy, ok := def.(*PolicyDefinition); ok {
			result = append(result, policy)
		}
	}
	return result
}

// WorkflowSteps returns all registered WorkflowStepDefinitions.
func WorkflowSteps() []*WorkflowStepDefinition {
	registryLock.Lock()
	defer registryLock.Unlock()
	var result []*WorkflowStepDefinition
	for _, def := range registry {
		if step, ok := def.(*WorkflowStepDefinition); ok {
			result = append(result, step)
		}
	}
	return result
}

// Clear resets the registry. This is primarily useful for testing.
func Clear() {
	registryLock.Lock()
	defer registryLock.Unlock()
	registry = nil
}

// Count returns the number of registered definitions.
func Count() int {
	registryLock.Lock()
	defer registryLock.Unlock()
	return len(registry)
}

// RegistryOutput is the JSON structure output by the generated main program.
// It contains all registered definitions in a format the CLI can parse.
type RegistryOutput struct {
	Definitions []DefinitionOutput `json:"definitions"`
}

// DefinitionOutput represents a single definition in the registry output.
type DefinitionOutput struct {
	Name      string                   `json:"name"`
	Type      DefinitionType           `json:"type"`
	CUE       string                   `json:"cue"`
	Placement *PlacementOutput         `json:"placement,omitempty"`
}

// PlacementOutput represents placement constraints in the registry output.
type PlacementOutput struct {
	RunOn    []PlacementConditionOutput `json:"runOn,omitempty"`
	NotRunOn []PlacementConditionOutput `json:"notRunOn,omitempty"`
}

// PlacementConditionOutput represents a single placement condition in the output.
type PlacementConditionOutput struct {
	Key      string   `json:"key"`
	Operator string   `json:"operator"`
	Values   []string `json:"values,omitempty"`
}

// ToJSON serializes all registered definitions to JSON.
// This is used by the generated main program to output definitions.
func ToJSON() ([]byte, error) {
	registryLock.Lock()
	defer registryLock.Unlock()

	output := RegistryOutput{
		Definitions: make([]DefinitionOutput, 0, len(registry)),
	}

	for _, def := range registry {
		defOutput := DefinitionOutput{
			Name: def.DefName(),
			Type: def.DefType(),
			CUE:  def.ToCue(),
		}

		// Include placement if the definition has any
		if def.HasPlacement() {
			spec := def.GetPlacement()
			defOutput.Placement = &PlacementOutput{}

			for _, cond := range spec.RunOn {
				if labelCond, ok := cond.(*placement.LabelCondition); ok {
					defOutput.Placement.RunOn = append(defOutput.Placement.RunOn, PlacementConditionOutput{
						Key:      labelCond.Key,
						Operator: string(labelCond.Operator),
						Values:   labelCond.Values,
					})
				}
			}

			for _, cond := range spec.NotRunOn {
				if labelCond, ok := cond.(*placement.LabelCondition); ok {
					defOutput.Placement.NotRunOn = append(defOutput.Placement.NotRunOn, PlacementConditionOutput{
						Key:      labelCond.Key,
						Operator: string(labelCond.Operator),
						Values:   labelCond.Values,
					})
				}
			}
		}

		output.Definitions = append(output.Definitions, defOutput)
	}

	return json.Marshal(output)
}
