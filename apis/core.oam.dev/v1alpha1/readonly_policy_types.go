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
	// ReadOnlyPolicyType refers to the type of read-only policy
	ReadOnlyPolicyType = "read-only"
)

// ReadOnlyPolicySpec defines the spec of read-only policy
type ReadOnlyPolicySpec struct {
	Rules []ReadOnlyPolicyRule `json:"rules"`
}

// Type the type name of the policy
func (in *ReadOnlyPolicySpec) Type() string {
	return ReadOnlyPolicyType
}

// ReadOnlyPolicyRule defines the rule for read-only resources
type ReadOnlyPolicyRule struct {
	Selector ResourcePolicyRuleSelector `json:"selector"`
}

// FindStrategy return if the target resource is read-only
func (in *ReadOnlyPolicySpec) FindStrategy(manifest *unstructured.Unstructured) bool {
	for _, rule := range in.Rules {
		if rule.Selector.Match(manifest) {
			return true
		}
	}
	return false
}
