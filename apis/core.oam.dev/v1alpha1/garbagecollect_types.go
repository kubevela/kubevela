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
	"k8s.io/utils/pointer"
	"k8s.io/utils/strings/slices"

	"github.com/oam-dev/kubevela/pkg/oam"
)

const (
	// GarbageCollectPolicyType refers to the type of garbage-collect
	GarbageCollectPolicyType = "garbage-collect"
)

// GarbageCollectPolicySpec defines the spec of configuration drift
type GarbageCollectPolicySpec struct {
	// KeepLegacyResource if is set, outdated versioned resourcetracker will not be recycled automatically
	// outdated resources will be kept until resourcetracker be deleted manually
	KeepLegacyResource bool `json:"keepLegacyResource,omitempty"`

	// Order defines the order of garbage collect
	Order GarbageCollectOrder `json:"order,omitempty"`

	// Rules defines list of rules to control gc strategy at resource level
	// if one resource is controlled by multiple rules, first rule will be used
	Rules []GarbageCollectPolicyRule `json:"rules,omitempty"`
}

// GarbageCollectOrder is the order of garbage collect
type GarbageCollectOrder string

const (
	// OrderDependency is the order of dependency
	OrderDependency GarbageCollectOrder = "dependency"
)

// GarbageCollectPolicyRule defines a single garbage-collect policy rule
type GarbageCollectPolicyRule struct {
	Selector ResourcePolicyRuleSelector `json:"selector"`
	Strategy GarbageCollectStrategy     `json:"strategy"`
}

// ResourcePolicyRuleSelector select the targets of the rule
// if multiple conditions are specified, combination logic is AND
type ResourcePolicyRuleSelector struct {
	CompNames        []string `json:"componentNames,omitempty"`
	CompTypes        []string `json:"componentTypes,omitempty"`
	OAMResourceTypes []string `json:"oamTypes,omitempty"`
	TraitTypes       []string `json:"traitTypes,omitempty"`
	ResourceTypes    []string `json:"resourceTypes,omitempty"`
	ResourceNames    []string `json:"resourceNames,omitempty"`
}

// Match check if current rule selector match the target resource
// If at least one condition is matched and no other condition failed (could be empty), return true
// Otherwise, return false
func (in *ResourcePolicyRuleSelector) Match(manifest *unstructured.Unstructured) bool {
	var compName, compType, oamType, traitType, resourceType, resourceName string
	if labels := manifest.GetLabels(); labels != nil {
		compName = labels[oam.LabelAppComponent]
		compType = labels[oam.WorkloadTypeLabel]
		oamType = labels[oam.LabelOAMResourceType]
		traitType = labels[oam.TraitTypeLabel]
	}
	resourceType = manifest.GetKind()
	resourceName = manifest.GetName()
	match := func(src []string, val string) (found *bool) {
		if len(src) == 0 {
			return nil
		}
		return pointer.Bool(val != "" && slices.Contains(src, val))
	}
	conditions := []*bool{
		match(in.CompNames, compName),
		match(in.CompTypes, compType),
		match(in.OAMResourceTypes, oamType),
		match(in.TraitTypes, traitType),
		match(in.ResourceTypes, resourceType),
		match(in.ResourceNames, resourceName),
	}
	hasMatched := false
	for _, cond := range conditions {
		// if any non-empty condition failed, return false
		if cond != nil && !*cond {
			return false
		}
		// if condition succeed, record it
		if cond != nil && *cond {
			hasMatched = true
		}
	}
	// if at least one condition is met, return true
	return hasMatched
}

// GarbageCollectStrategy the strategy for target resource to recycle
type GarbageCollectStrategy string

const (
	// GarbageCollectStrategyNever do not recycle target resource, leave it
	GarbageCollectStrategyNever GarbageCollectStrategy = "never"
	// GarbageCollectStrategyOnAppDelete do not recycle target resource until application is deleted
	// this means the resource will be kept even it is not used in the latest version
	GarbageCollectStrategyOnAppDelete GarbageCollectStrategy = "onAppDelete"
	// GarbageCollectStrategyOnAppUpdate recycle target resource when it is not inUse
	GarbageCollectStrategyOnAppUpdate GarbageCollectStrategy = "onAppUpdate"
)

// FindStrategy find gc strategy for target resource
func (in GarbageCollectPolicySpec) FindStrategy(manifest *unstructured.Unstructured) *GarbageCollectStrategy {
	for _, rule := range in.Rules {
		if rule.Selector.Match(manifest) {
			return &rule.Strategy
		}
	}
	return nil
}
