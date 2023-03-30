package v1alpha1

import (
	"testing"

	
	"cuelang.org/go/cue/cuecontext"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	
)

func TestReadOnlyPolicySpec(t *testing.T) {
	const cueCode = `
		readOnly: {
			annotations: {}
			description: "Configure the resources to be read-only in the application (no update / state-keep)."
			labels: {}
			attributes: {}
			type: "policy"
		}

		template: {
			#PolicyRule: {
				selector: #RuleSelector
			}

			#RuleSelector: {
				componentNames?: [...string]
				componentTypes?: [...string]
				oamTypes?: [...string]
				traitTypes?: [...string]
				resourceTypes?: [...string]
				resourceNames?: [...string]
			}

			parameter: {
				rules?: [...#PolicyRule]
			}
		}
	`

	// Create a new context and compile the CUE code
    ctx := cuecontext.New()
    r := cue.Runtime{}
    inst, err := r.Compile("test", cueCode)
    if err != nil {
        t.Fatalf("Failed to compile CUE code: %v", err)
    }

    // Check the consistency of the type name
    typeName := inst.Lookup("readOnly").Lookup("type").String()
    if typeName != ReadOnlyPolicyType {
        t.Errorf("Inconsistent type name: got %q, want %q", typeName, ReadOnlyPolicyType)
    }

    // Check the consistency of the FindStrategy method
    specValue := inst.Lookup("template").Lookup("parameter").Lookup("rules").Index(0)
    spec := &ReadOnlyPolicySpec{}
    err = specValue.Decode(spec)
    if err != nil {
        t.Fatalf("Failed to decode CUE value: %v", err)
    }

    manifest := &unstructured.Unstructured{}
    manifest.SetAPIVersion("apps/v1")
    manifest.SetKind("Deployment")
    manifest.SetName("example-deployment")

    isReadOnly := spec.FindStrategy(manifest)
    if isReadOnly {
        t.Errorf("Unexpected read-only strategy: got true, want false")
    }

    // Add a read-only rule and check again
    spec.Rules = append(spec.Rules, ReadOnlyPolicyRule{
        Selector: ResourcePolicyRuleSelector{
            componentTypes: []string{"my-component"},
        },
    })

    isReadOnly = spec.FindStrategy(manifest)
    if !isReadOnly {
        t.Errorf("Unexpected read-only strategy: got false, want true")
    }
}
