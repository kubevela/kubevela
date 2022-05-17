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

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	// ApplyOncePolicyType refers to the type of configuration drift policy
	ApplyOncePolicyType = "apply-once"
)

// ApplyOncePolicySpec defines the spec of preventing configuration drift
type ApplyOncePolicySpec struct {
	Enable bool `json:"enable"`
	// +optional
	Rules []ApplyOncePolicyRule `json:"rules,omitempty"`
}

// ApplyOncePolicyRule defines a single apply-once policy rule
type ApplyOncePolicyRule struct {
	// +optional
	Selector ResourcePolicyRuleSelector `json:"selector,omitempty"`
	// +optional
	Strategy *ApplyOnceStrategy `json:"strategy,omitempty"`
}

// ApplyOnceStrategy the strategy for resource path to allow configuration drift
type ApplyOnceStrategy struct {
	// Path the specified path that allow configuration drift
	// like 'spec.template.spec.containers[0].resources' and '*' means the whole target allow configuration drift
	Path []string `json:"path"`
}

// FindStrategy find apply-once strategy for target resource
func (in ApplyOncePolicySpec) FindStrategy(manifest *unstructured.Unstructured) *ApplyOnceStrategy {
	if !in.Enable {
		return nil
	}
	for _, rule := range in.Rules {
		match := func(src []string, val string) (found bool) {
			for _, _val := range src {
				found = found || _val == val
			}
			return val != "" && found
		}
		if (match(rule.Selector.CompNames, manifest.GetName()) && match(rule.Selector.ResourceTypes, manifest.GetKind())) ||
			(rule.Selector.CompNames == nil && match(rule.Selector.ResourceTypes, manifest.GetKind()) ||
				(rule.Selector.ResourceTypes == nil && match(rule.Selector.CompNames, manifest.GetName()))) {
			return rule.Strategy
		}
	}
	return nil
}
