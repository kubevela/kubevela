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

func TestPolicyDefinition_Basic(t *testing.T) {
	policy := NewPolicy("topology").
		Description("Describe the destination where components should be deployed to.").
		Params(
			StringList("clusters").Description("Specify the names of the clusters to select."),
			StringKeyMap("clusterLabelSelector").Description("Specify the label selector for clusters"),
			Bool("allowEmpty").Description("Ignore empty cluster error"),
			String("namespace").Description("Specify the target namespace"),
		)

	// Test basic properties
	if policy.DefName() != "topology" {
		t.Errorf("expected name 'topology', got %q", policy.DefName())
	}
	if policy.DefType() != DefinitionTypePolicy {
		t.Errorf("expected type policy, got %v", policy.DefType())
	}
	if len(policy.GetParams()) != 4 {
		t.Errorf("expected 4 params, got %d", len(policy.GetParams()))
	}
}

func TestPolicyDefinition_ToCue(t *testing.T) {
	policy := NewPolicy("topology").
		Description("Describe the destination.").
		Params(
			StringList("clusters").Description("Cluster names"),
			Bool("allowEmpty"),
		)

	cue := policy.ToCue()

	// Should contain policy header
	if !strings.Contains(cue, `type: "policy"`) {
		t.Error("expected CUE to contain type: \"policy\"")
	}

	// Should contain description
	if !strings.Contains(cue, `description: "Describe the destination."`) {
		t.Error("expected CUE to contain description")
	}

	// Should contain template block
	if !strings.Contains(cue, "template:") {
		t.Error("expected CUE to contain template:")
	}

	// Should contain parameter block
	if !strings.Contains(cue, "parameter:") {
		t.Error("expected CUE to contain parameter:")
	}

	// Should contain parameters
	if !strings.Contains(cue, "clusters") {
		t.Error("expected CUE to contain clusters parameter")
	}
}

func TestPolicyDefinition_RawCUE(t *testing.T) {
	rawCUE := `"topology": {
	type: "policy"
	description: "Raw CUE policy"
}
template: {
	parameter: clusters?: [...string]
}`

	policy := NewPolicy("topology").RawCUE(rawCUE)

	if policy.ToCue() != rawCUE {
		t.Error("expected RawCUE to return exact raw CUE string")
	}
}

func TestPolicyDefinition_WithImports(t *testing.T) {
	policy := NewPolicy("custom").
		Description("Custom policy.").
		WithImports("strings")

	cue := policy.ToCue()

	if !strings.Contains(cue, `import (`) {
		t.Error("expected CUE to contain import block")
	}
	if !strings.Contains(cue, `"strings"`) {
		t.Error("expected CUE to contain strings import")
	}
}

func TestPolicyDefinition_ToYAML(t *testing.T) {
	policy := NewPolicy("topology").
		Description("Deployment topology.").
		Params(StringList("clusters"))

	yamlBytes, err := policy.ToYAML()
	if err != nil {
		t.Fatalf("ToYAML failed: %v", err)
	}

	yaml := string(yamlBytes)

	if !strings.Contains(yaml, "kind: PolicyDefinition") {
		t.Error("expected YAML to contain kind: PolicyDefinition")
	}
	if !strings.Contains(yaml, "name: topology") {
		t.Error("expected YAML to contain name: topology")
	}
}

func TestPolicyDefinition_Registry(t *testing.T) {
	Clear() // Reset registry

	policy1 := NewPolicy("topology").Description("Topology")
	policy2 := NewPolicy("override").Description("Override")
	comp := NewComponent("webservice").Description("Component")

	Register(policy1)
	Register(policy2)
	Register(comp)

	// Should have 3 definitions total
	if Count() != 3 {
		t.Errorf("expected 3 registered definitions, got %d", Count())
	}

	// Should have 2 policies
	policies := Policies()
	if len(policies) != 2 {
		t.Errorf("expected 2 policies, got %d", len(policies))
	}

	Clear() // Clean up
}
