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

func TestWorkflowStepDefinition_Basic(t *testing.T) {
	step := NewWorkflowStep("deploy").
		Description("A powerful and unified deploy step for components multi-cluster delivery.").
		Category("Application Delivery").
		Scope("Application").
		Params(
			Bool("auto").Default(true).Description("If set to false, the workflow will suspend automatically."),
			StringList("policies").Description("Declare the policies for deployment."),
			Int("parallelism").Default(5).Description("Maximum concurrent components."),
		)

	// Test basic properties
	if step.DefName() != "deploy" {
		t.Errorf("expected name 'deploy', got %q", step.DefName())
	}
	if step.DefType() != DefinitionTypeWorkflowStep {
		t.Errorf("expected type workflow-step, got %v", step.DefType())
	}
	if step.GetCategory() != "Application Delivery" {
		t.Errorf("expected category 'Application Delivery', got %q", step.GetCategory())
	}
	if step.GetScope() != "Application" {
		t.Errorf("expected scope 'Application', got %q", step.GetScope())
	}
	if len(step.GetParams()) != 3 {
		t.Errorf("expected 3 params, got %d", len(step.GetParams()))
	}
}

func TestWorkflowStepDefinition_ToCue(t *testing.T) {
	step := NewWorkflowStep("deploy").
		Description("Deploy step.").
		Category("Application Delivery").
		Scope("Application").
		Params(
			Bool("auto").Default(true),
			Int("parallelism").Default(5),
		)

	cue := step.ToCue()

	// Should contain workflow-step header
	if !strings.Contains(cue, `type: "workflow-step"`) {
		t.Error("expected CUE to contain type: \"workflow-step\"")
	}

	// Should contain annotations with category
	if !strings.Contains(cue, "annotations:") {
		t.Error("expected CUE to contain annotations:")
	}
	if !strings.Contains(cue, `"category": "Application Delivery"`) {
		t.Error("expected CUE to contain category annotation")
	}

	// Should contain labels with scope
	if !strings.Contains(cue, "labels:") {
		t.Error("expected CUE to contain labels:")
	}
	if !strings.Contains(cue, `"scope": "Application"`) {
		t.Error("expected CUE to contain scope label")
	}

	// Should contain template block
	if !strings.Contains(cue, "template:") {
		t.Error("expected CUE to contain template:")
	}

	// Should contain parameters
	if !strings.Contains(cue, "parameter:") {
		t.Error("expected CUE to contain parameter:")
	}
	if !strings.Contains(cue, "auto:") {
		t.Error("expected CUE to contain auto parameter")
	}
}

func TestWorkflowStepDefinition_WithImports(t *testing.T) {
	step := NewWorkflowStep("deploy").
		Description("Deploy step.").
		WithImports("vela/multicluster", "vela/builtin")

	cue := step.ToCue()

	if !strings.Contains(cue, `import (`) {
		t.Error("expected CUE to contain import block")
	}
	if !strings.Contains(cue, `"vela/multicluster"`) {
		t.Error("expected CUE to contain vela/multicluster import")
	}
	if !strings.Contains(cue, `"vela/builtin"`) {
		t.Error("expected CUE to contain vela/builtin import")
	}
}

func TestWorkflowStepDefinition_RawCUE(t *testing.T) {
	rawCUE := `import (
	"vela/multicluster"
)

"deploy": {
	type: "workflow-step"
	description: "Raw CUE step"
}
template: {
	deploy: multicluster.#Deploy
	parameter: auto: *true | bool
}`

	step := NewWorkflowStep("deploy").RawCUE(rawCUE)

	if step.ToCue() != rawCUE {
		t.Error("expected RawCUE to return exact raw CUE string")
	}
}

func TestWorkflowStepDefinition_ToYAML(t *testing.T) {
	step := NewWorkflowStep("deploy").
		Description("Deploy components.").
		Category("Application Delivery").
		Scope("Application").
		Params(Bool("auto").Default(true))

	yamlBytes, err := step.ToYAML()
	if err != nil {
		t.Fatalf("ToYAML failed: %v", err)
	}

	yaml := string(yamlBytes)

	if !strings.Contains(yaml, "kind: WorkflowStepDefinition") {
		t.Error("expected YAML to contain kind: WorkflowStepDefinition")
	}
	if !strings.Contains(yaml, "name: deploy") {
		t.Error("expected YAML to contain name: deploy")
	}
}

func TestWorkflowStepDefinition_Registry(t *testing.T) {
	Clear() // Reset registry

	step1 := NewWorkflowStep("deploy").Description("Deploy").Category("Application Delivery")
	step2 := NewWorkflowStep("suspend").Description("Suspend").Category("Workflow Control")
	comp := NewComponent("webservice").Description("Component")

	Register(step1)
	Register(step2)
	Register(comp)

	// Should have 3 definitions total
	if Count() != 3 {
		t.Errorf("expected 3 registered definitions, got %d", Count())
	}

	// Should have 2 workflow steps
	steps := WorkflowSteps()
	if len(steps) != 2 {
		t.Errorf("expected 2 workflow steps, got %d", len(steps))
	}

	Clear() // Clean up
}

func TestWorkflowStepDefinition_Template(t *testing.T) {
	auto := Bool("auto").Default(true)
	policies := StringList("policies")
	parallelism := Int("parallelism").Default(5)

	step := NewWorkflowStep("deploy").
		Description("Deploy step.").
		WithImports("vela/multicluster", "vela/builtin").
		Params(auto, policies, parallelism).
		Template(func(tpl *WorkflowStepTemplate) {
			// Add conditional suspend (when auto is false)
			tpl.SuspendIf(Not(auto.IsTrue()), `Waiting approval to the deploy step`)

			// Add deploy action
			tpl.Builtin("deploy", "multicluster.#Deploy").
				WithParams(map[string]Value{
					"policies":    policies,
					"parallelism": parallelism,
				}).Build()
		})

	cue := step.ToCue()

	// Should contain template content
	if !strings.Contains(cue, "template:") {
		t.Error("expected CUE to contain template:")
	}
}
