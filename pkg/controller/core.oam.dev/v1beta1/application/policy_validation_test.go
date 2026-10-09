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

package application

import (
	"context"
	"fmt"

	"cuelang.org/go/cue"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	utilfeature "k8s.io/apiserver/pkg/util/feature"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	webhookutils "github.com/oam-dev/kubevela/pkg/webhook/utils"
)

// validatePolicy mirrors what the webhook handler does: compile the template
// once and hand the result to ValidatePolicyDefinition, rather than each spec
// re-deriving that plumbing. A compile failure short-circuits here exactly as
// it does in the handler, which never reaches ValidatePolicyDefinition either.
func validatePolicy(policy *v1beta1.PolicyDefinition) *PolicyValidationResult {
	ctx := context.TODO()
	if policy.Spec.Schematic == nil || policy.Spec.Schematic.CUE == nil {
		return ValidatePolicyDefinition(ctx, policy, "", cue.Value{})
	}
	cueTemplate := policy.Spec.Schematic.CUE.Template
	val, err := webhookutils.CompilePolicyTemplate(ctx, policy.Spec.Scope, cueTemplate)
	if err != nil {
		return &PolicyValidationResult{Errors: []string{err.Error()}}
	}
	return ValidatePolicyDefinition(ctx, policy, cueTemplate, val)
}

var _ = Describe("Test PolicyDefinition Validation", func() {

	It("Test valid global policy passes validation", func() {
		policy := &v1beta1.PolicyDefinition{
			Spec: v1beta1.PolicyDefinitionSpec{
				Global:   true,
				Priority: 100,
				Scope:    v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {}

enabled: true

output: {
	labels: {
		"test": "value"
	}
}
`,
					},
				},
			},
		}

		result := validatePolicy(policy)
		Expect(result.IsValid()).Should(BeTrue())
		Expect(result.Errors).Should(BeEmpty())
	})

	It("Test global policy with required parameter fails validation", func() {
		policy := &v1beta1.PolicyDefinition{
			Spec: v1beta1.PolicyDefinitionSpec{
				Global:   true,
				Priority: 100,
				Scope:    v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {
	envName: string  // Required field - no default!
}

output: {
	labels: {
		"env": parameter.envName
	}
}
`,
					},
				},
			},
		}

		result := validatePolicy(policy)
		Expect(result.IsValid()).Should(BeFalse())
		Expect(result.Errors).Should(HaveLen(1))
		Expect(result.Errors[0]).Should(ContainSubstring("without default values"))
		Expect(result.Errors[0]).Should(ContainSubstring("envName"))
	})

	It("Test global policy with optional parameter without default fails validation", func() {
		policy := &v1beta1.PolicyDefinition{
			Spec: v1beta1.PolicyDefinitionSpec{
				Global:   true,
				Priority: 100,
				Scope:    v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {
	envName?: string  // Optional but no default - can't compile!
}

output: {
	labels: {
		"env": parameter.envName
	}
}
`,
					},
				},
			},
		}

		result := validatePolicy(policy)
		Expect(result.IsValid()).Should(BeFalse())
		Expect(result.Errors).Should(HaveLen(1))
		Expect(result.Errors[0]).Should(ContainSubstring("without default values"))
		Expect(result.Errors[0]).Should(ContainSubstring("envName"))
	})

	It("Test global policy with default parameters passes validation", func() {
		policy := &v1beta1.PolicyDefinition{
			Spec: v1beta1.PolicyDefinitionSpec{
				Global:   true,
				Priority: 100,
				Scope:    v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {
	envName: *"production" | string  // Has default
	replicas: *3 | int  // Has default
}

output: {
	labels: {
		"env": parameter.envName
		"replicas": "\(parameter.replicas)"
	}
}
`,
					},
				},
			},
		}

		result := validatePolicy(policy)
		Expect(result.IsValid()).Should(BeTrue())
		Expect(result.Errors).Should(BeEmpty())
	})

	It("Test global policy with empty parameter block passes validation", func() {
		policy := &v1beta1.PolicyDefinition{
			Spec: v1beta1.PolicyDefinitionSpec{
				Global:   true,
				Priority: 100,
				Scope:    v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {}  // Empty is fine

output: {
	labels: {
		"static": "value"
	}
}
`,
					},
				},
			},
		}

		result := validatePolicy(policy)
		Expect(result.IsValid()).Should(BeTrue())
		Expect(result.Errors).Should(BeEmpty())
	})

	It("Test global policy with wrong scope fails validation", func() {
		policy := &v1beta1.PolicyDefinition{
			Spec: v1beta1.PolicyDefinitionSpec{
				Global:   true,
				Priority: 100,
				Scope:    "WorkflowStep", // Wrong scope!
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {}
`,
					},
				},
			},
		}

		result := validatePolicy(policy)
		Expect(result.IsValid()).Should(BeFalse())
		Expect(result.Errors).Should(ContainElement(ContainSubstring("scope='Application'")))
	})

	It("Test global policy without explicit priority gets warning", func() {
		origASP := utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableApplicationScopedPolicies)
		Expect(utilfeature.DefaultMutableFeatureGate.Set("EnableApplicationScopedPolicies=true")).Should(Succeed())
		defer utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("EnableApplicationScopedPolicies=%v", origASP))

		policy := &v1beta1.PolicyDefinition{
			Spec: v1beta1.PolicyDefinitionSpec{
				Global: true,
				// Priority not set (defaults to 0)
				Scope: v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `parameter: {}`,
					},
				},
			},
		}

		result := validatePolicy(policy)
		Expect(result.IsValid()).Should(BeTrue())
		Expect(result.Warnings).Should(HaveLen(1))
		Expect(result.Warnings[0]).Should(ContainSubstring("explicit priority"))
	})

	It("Test global policy with very high priority gets warning", func() {
		policy := &v1beta1.PolicyDefinition{
			Spec: v1beta1.PolicyDefinitionSpec{
				Global:   true,
				Priority: 10000, // Unusually high (> 9999 threshold)
				Scope:    v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `parameter: {}`,
					},
				},
			},
		}

		result := validatePolicy(policy)
		Expect(result.IsValid()).Should(BeTrue())
		Expect(result.Warnings).Should(ContainElement(ContainSubstring("unusually high")))
	})

	It("Test policy with invalid CUE syntax fails validation", func() {
		policy := &v1beta1.PolicyDefinition{
			Spec: v1beta1.PolicyDefinitionSpec{
				Global:   true,
				Priority: 100,
				Scope:    v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {
	this is not valid CUE syntax!!!
}
`,
					},
				},
			},
		}

		result := validatePolicy(policy)
		Expect(result.IsValid()).Should(BeFalse())
		Expect(result.Errors).Should(ContainElement(ContainSubstring("expected label")))
	})

	It("Test enabled field must be bool", func() {
		policy := &v1beta1.PolicyDefinition{
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {}

enabled: "true"  // Invalid! Should be bool, not string
`,
					},
				},
			},
		}

		result := validatePolicy(policy)
		Expect(result.IsValid()).Should(BeFalse())
		Expect(result.Errors).Should(ContainElement(ContainSubstring("'enabled' field must be of type bool")))
	})

	It("Test non-global policy with required parameters is allowed", func() {
		policy := &v1beta1.PolicyDefinition{
			Spec: v1beta1.PolicyDefinitionSpec{
				Global:   false, // Not global
				Priority: 100,
				Scope:    v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {
	envName: string  // Required is OK for non-global policies
}

output: {
	labels: {
		"env": parameter.envName
	}
}
`,
					},
				},
			},
		}

		result := validatePolicy(policy)
		Expect(result.IsValid()).Should(BeTrue())
		Expect(result.Errors).Should(BeEmpty())
	})

	It("Test Application-scoped policy warns when EnableApplicationScopedPolicies is disabled", func() {
		origASP := utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableApplicationScopedPolicies)
		utilfeature.DefaultMutableFeatureGate.Set("EnableApplicationScopedPolicies=false")
		defer utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("EnableApplicationScopedPolicies=%v", origASP))

		policy := &v1beta1.PolicyDefinition{
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{Template: "enabled: true\noutput: {}"},
				},
			},
		}

		result := validatePolicy(policy)
		Expect(result.IsValid()).Should(BeTrue())
		Expect(result.Warnings).Should(ContainElement(ContainSubstring("EnableApplicationScopedPolicies feature gate is disabled")))
	})

	It("Test global policy warns when EnableGlobalPolicies is disabled", func() {
		origGP := utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableGlobalPolicies)
		utilfeature.DefaultMutableFeatureGate.Set("EnableGlobalPolicies=false")
		defer utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("EnableGlobalPolicies=%v", origGP))

		policy := &v1beta1.PolicyDefinition{
			Spec: v1beta1.PolicyDefinitionSpec{
				Global:   true,
				Priority: 100,
				Scope:    v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{Template: "enabled: true\noutput: {}"},
				},
			},
		}

		result := validatePolicy(policy)
		Expect(result.IsValid()).Should(BeTrue())
		Expect(result.Warnings).Should(ContainElement(ContainSubstring("EnableGlobalPolicies feature gate is disabled")))
	})

	It("Test policy without schematic fails validation", func() {
		policy := &v1beta1.PolicyDefinition{
			Spec: v1beta1.PolicyDefinitionSpec{
				Global:   true,
				Priority: 100,
				Scope:    v1beta1.ApplicationScope,
				// No schematic!
			},
		}

		result := validatePolicy(policy)
		Expect(result.IsValid()).Should(BeFalse())
		Expect(result.Errors).Should(ContainElement(ContainSubstring("must have a CUE schematic")))
	})

	It("Test policy importing a cuex provider package passes validation", func() {
		// Plain CUE has no "vela/base64", so this only passes when the template
		// is checked with the cuex compiler that renders it.
		policy := &v1beta1.PolicyDefinition{
			Spec: v1beta1.PolicyDefinitionSpec{
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
import "vela/base64"

encoded: base64.#Encode & {
	$params: "hello"
}
`,
					},
				},
			},
		}

		result := validatePolicy(policy)
		Expect(result.IsValid()).Should(BeTrue())
		Expect(result.Errors).Should(BeEmpty())
	})

	It("Test global policy with required parameter and a context reference fails validation", func() {
		// The reference to context leaves the plain compile in error. The
		// parameters are read with context opened instead, so the missing
		// default is still found rather than the policy being waved through.
		policy := &v1beta1.PolicyDefinition{
			Spec: v1beta1.PolicyDefinitionSpec{
				Global:   true,
				Priority: 100,
				Scope:    v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {
	envName: string  // Required field - no default!
}

output: {
	labels: {
		"env": parameter.envName
		"app": context.appName
	}
}
`,
					},
				},
			},
		}

		result := validatePolicy(policy)
		Expect(result.IsValid()).Should(BeFalse())
		Expect(result.Errors).Should(HaveLen(1))
		Expect(result.Errors[0]).Should(ContainSubstring("without default values"))
		Expect(result.Errors[0]).Should(ContainSubstring("envName"))
	})

	It("Test global policy with defaulted parameters and a context reference passes validation", func() {
		// context only exists at render, so a template reading it does not
		// compile on its own. That must not stop its parameters being checked.
		policy := &v1beta1.PolicyDefinition{
			Spec: v1beta1.PolicyDefinitionSpec{
				Global:   true,
				Priority: 100,
				Scope:    v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {
	envName: *"production" | string
}

output: {
	labels: {
		"env": parameter.envName
		"app": context.appName
	}
}
`,
					},
				},
			},
		}

		result := validatePolicy(policy)
		Expect(result.IsValid()).Should(BeTrue())
		Expect(result.Errors).Should(BeEmpty())
	})

	It("Test global policy with an unresolvable reference fails validation", func() {
		// Opening context does not excuse any other unresolved reference.
		policy := &v1beta1.PolicyDefinition{
			Spec: v1beta1.PolicyDefinitionSpec{
				Global:   true,
				Priority: 100,
				Scope:    v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
parameter: {
	envName: *"production" | string
}

output: {
	labels: {
		"env": parameterr.envName
	}
}
`,
					},
				},
			},
		}

		result := validatePolicy(policy)
		Expect(result.IsValid()).Should(BeFalse())
		Expect(result.Errors).Should(ContainElement(ContainSubstring("failed to compile CUE template")))
	})

	It("Test Application-scoped policy importing a package of its render compiler passes validation", func() {
		// Application-scoped policies render with the upstream default compiler,
		// which has "vela/util" and the workload compiler does not.
		policy := &v1beta1.PolicyDefinition{
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: utilImportTemplate,
					},
				},
			},
		}

		result := validatePolicy(policy)
		Expect(result.IsValid()).Should(BeTrue())
		Expect(result.Errors).Should(BeEmpty())
	})

	It("Test Application-scoped policy importing a package its render compiler lacks fails validation", func() {
		// "vela/helm" is only in the workload compiler, so this would pass
		// admission and then fail every render.
		policy := &v1beta1.PolicyDefinition{
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope: v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: helmImportTemplate,
					},
				},
			},
		}

		result := validatePolicy(policy)
		Expect(result.IsValid()).Should(BeFalse())
		Expect(result.Errors).Should(ContainElement(ContainSubstring(`builtin package "vela/helm" undefined`)))
	})

	It("Test default-scope policy importing a workload compiler package passes validation", func() {
		// A default-scope policy renders as a component, with the workload
		// compiler, so "vela/helm" is available to it.
		policy := &v1beta1.PolicyDefinition{
			Spec: v1beta1.PolicyDefinitionSpec{
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: helmImportTemplate,
					},
				},
			},
		}

		result := validatePolicy(policy)
		Expect(result.IsValid()).Should(BeTrue())
		Expect(result.Errors).Should(BeEmpty())
	})

})

// utilImportTemplate calls a provider only the upstream default compiler,
// which renders Application-scoped policies, registers.
const utilImportTemplate = `
import "vela/util"

parameter: {
	name: *"my-app" | string
}

shortName: util.#Truncate & {
	$params: {
		value:     parameter.name
		maxLength: 20
	}
}
`

// helmImportTemplate calls a provider only the workload compiler, which
// renders default-scope policies, registers.
const helmImportTemplate = `
import "vela/helm"

chart: helm.#Render & {
	$params: chart: source: "nginx"
}
`
