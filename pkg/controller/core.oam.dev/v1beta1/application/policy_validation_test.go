/*
Copyright 2021 The KubeVela Authors.

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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

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

		result := ValidatePolicyDefinition(policy)
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

		result := ValidatePolicyDefinition(policy)
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

		result := ValidatePolicyDefinition(policy)
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

		result := ValidatePolicyDefinition(policy)
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

		result := ValidatePolicyDefinition(policy)
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

		result := ValidatePolicyDefinition(policy)
		Expect(result.IsValid()).Should(BeFalse())
		Expect(result.Errors).Should(ContainElement(ContainSubstring("scope='Application'")))
	})

	It("Test global policy without explicit priority gets warning", func() {
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

		result := ValidatePolicyDefinition(policy)
		Expect(result.IsValid()).Should(BeTrue())
		Expect(result.Warnings).Should(HaveLen(1))
		Expect(result.Warnings[0]).Should(ContainSubstring("explicit priority"))
	})

	It("Test global policy with very high priority gets warning", func() {
		policy := &v1beta1.PolicyDefinition{
			Spec: v1beta1.PolicyDefinitionSpec{
				Global:   true,
				Priority: 5000, // Unusually high
				Scope:    v1beta1.ApplicationScope,
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `parameter: {}`,
					},
				},
			},
		}

		result := ValidatePolicyDefinition(policy)
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

		result := ValidatePolicyDefinition(policy)
		Expect(result.IsValid()).Should(BeFalse())
		Expect(result.Errors).Should(ContainElement(ContainSubstring("syntax error")))
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

		result := ValidatePolicyDefinition(policy)
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

		result := ValidatePolicyDefinition(policy)
		Expect(result.IsValid()).Should(BeTrue())
		Expect(result.Errors).Should(BeEmpty())
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

		result := ValidatePolicyDefinition(policy)
		Expect(result.IsValid()).Should(BeFalse())
		Expect(result.Errors).Should(ContainElement(ContainSubstring("must have a CUE schematic")))
	})

})
