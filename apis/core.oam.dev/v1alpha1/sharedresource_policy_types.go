/*
Copyright 2022 The KubeVela Authors.

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

package v1alpha1

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

const (
	// SharedResourcePolicyType refers to the type of shared resource policy
	SharedResourcePolicyType = "shared-resource"
)

// SharedResourcePolicySpec defines the spec of shared-resource policy
type SharedResourcePolicySpec struct {
	Rules []SharedResourcePolicyRule `json:"rules"`
}

// Type the type name of the policy
func (in *SharedResourcePolicySpec) Type() string {
	return SharedResourcePolicyType
}

// SharedResourcePolicyRule defines the rule for sharing resources
type SharedResourcePolicyRule struct {
	Selector ResourcePolicyRuleSelector `json:"selector"`
}

// FindStrategy return if the target resource should be shared
func (in *SharedResourcePolicySpec) FindStrategy(manifest *unstructured.Unstructured) bool {
	for _, rule := range in.Rules {
		if rule.Selector.Match(manifest) {
			return true
		}
	}
	return false
}
