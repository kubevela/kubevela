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

	// Rules defines list of rules to control gc strategy at resource level
	// if one resource is controlled by multiple rules, first rule will be used
	Rules []GarbageCollectPolicyRule `json:"rules,omitempty"`
}

// GarbageCollectPolicyRule defines a single garbage-collect policy rule
type GarbageCollectPolicyRule struct {
	Selector GarbageCollectPolicyRuleSelector `json:"selector"`
	Strategy GarbageCollectStrategy           `json:"strategy"`
}

// GarbageCollectPolicyRuleSelector select the targets of the rule
// if both traitTypes, resourceTypes and componentTypes are specified, combination logic is OR
// if one resource is specified with conflict strategies, strategy as component go first.
type GarbageCollectPolicyRuleSelector struct {
	CompNames        []string `json:"componentNames"`
	CompTypes        []string `json:"componentTypes"`
	OAMResourceTypes []string `json:"OAMResourceTypes"`
	TraitTypes       []string `json:"traitTypes"`
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
		var compName, compType, OAMResourceType, traitType string
		if labels := manifest.GetLabels(); labels != nil {
			compName = labels[oam.LabelAppComponent]
			compType = labels[oam.WorkloadTypeLabel]
			OAMResourceType = labels[oam.LabelOAMResourceType]
			traitType = labels[oam.TraitTypeLabel]
		}
		match := func(src []string, val string) (found bool) {
			for _, _val := range src {
				found = found || _val == val
			}
			return val != "" && found
		}
		if match(rule.Selector.CompNames, compName) ||
			match(rule.Selector.CompTypes, compType) ||
			match(rule.Selector.OAMResourceTypes, OAMResourceType) ||
			match(rule.Selector.TraitTypes, traitType) {
			return &rule.Strategy
		}
	}
	return nil
}
