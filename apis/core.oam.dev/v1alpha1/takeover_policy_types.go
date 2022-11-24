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
	// TakeOverPolicyType refers to the type of take-over policy
	TakeOverPolicyType = "take-over"
)

// TakeOverPolicySpec defines the spec of take-over policy
type TakeOverPolicySpec struct {
	Rules []TakeOverPolicyRule `json:"rules"`
}

// Type the type name of the policy
func (in *TakeOverPolicySpec) Type() string {
	return TakeOverPolicyType
}

// TakeOverPolicyRule defines the rule for taking over resources
type TakeOverPolicyRule struct {
	Selector ResourcePolicyRuleSelector `json:"selector"`
}

// FindStrategy return if the target resource should be taken over
func (in *TakeOverPolicySpec) FindStrategy(manifest *unstructured.Unstructured) bool {
	for _, rule := range in.Rules {
		if rule.Selector.Match(manifest) {
			return true
		}
	}
	return false
}
