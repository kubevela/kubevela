/*
Copyright 2023 The KubeVela Authors.

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
	// ResourceUpdatePolicyType refers to the type of resource-update policy
	ResourceUpdatePolicyType = "resource-update"
)

// ResourceUpdatePolicySpec defines the spec of resource-update policy
type ResourceUpdatePolicySpec struct {
	Rules []ResourceUpdatePolicyRule `json:"rules"`
}

// Type the type name of the policy
func (in *ResourceUpdatePolicySpec) Type() string {
	return ResourceUpdatePolicyType
}

// ResourceUpdatePolicyRule defines the rule for resource-update resources
type ResourceUpdatePolicyRule struct {
	// Selector picks which resources should be affected
	Selector ResourcePolicyRuleSelector `json:"selector"`
	// Strategy the strategy for updating resources
	Strategy ResourceUpdateStrategy `json:"strategy,omitempty"`
}

// ResourceUpdateStrategy the update strategy for resource
type ResourceUpdateStrategy struct {
	// Op the update op for selected resources
	Op ResourceUpdateOp `json:"op,omitempty"`
	// RecreateFields the field path which will trigger recreate if changed
	RecreateFields []string `json:"recreateFields,omitempty"`
}

// ResourceUpdateOp update op for resource
type ResourceUpdateOp string

const (
	// ResourceUpdateStrategyPatch patch the target resource (three-way patch)
	ResourceUpdateStrategyPatch ResourceUpdateOp = "patch"
	// ResourceUpdateStrategyReplace update the target resource
	ResourceUpdateStrategyReplace ResourceUpdateOp = "replace"
)

// FindStrategy return if the target resource is read-only
func (in *ResourceUpdatePolicySpec) FindStrategy(manifest *unstructured.Unstructured) *ResourceUpdateStrategy {
	for _, rule := range in.Rules {
		if rule.Selector.Match(manifest) {
			return &rule.Strategy
		}
	}
	return nil
}
