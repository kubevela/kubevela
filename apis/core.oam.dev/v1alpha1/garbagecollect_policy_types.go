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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// GarbageCollectPolicyType refers to the type of garbage-collect
	GarbageCollectPolicyType = "garbage-collect"
)

// GarbageCollectPolicySpec defines the spec of configuration drift
type GarbageCollectPolicySpec struct {
	// ApplicationRevisionLimit if set, this application will use this number for application revision instead of
	// the global configuration
	ApplicationRevisionLimit *int `json:"applicationRevisionLimit,omitempty"`

	// KeepLegacyResource if is set, outdated versioned resourcetracker will not be recycled automatically
	// outdated resources will be kept until resourcetracker be deleted manually
	KeepLegacyResource bool `json:"keepLegacyResource,omitempty"`

	// ContinueOnFailure if is set, continue to execute gc when the workflow fails, by default gc will be executed only after the workflow succeeds
	ContinueOnFailure bool `json:"continueOnFailure,omitempty"`

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
	Selector    ResourcePolicyRuleSelector `json:"selector"`
	Strategy    GarbageCollectStrategy     `json:"strategy"`
	Propagation *GarbageCollectPropagation `json:"propagation"`
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

// GarbageCollectPropagation the deletion propagation setting similar to metav1.DeletionPropagation
type GarbageCollectPropagation string

const (
	// GarbageCollectPropagationOrphan orphan child resources while deleting target resources
	GarbageCollectPropagationOrphan = "orphan"
	// GarbageCollectPropagationCascading delete child resources in background while deleting target resources
	GarbageCollectPropagationCascading = "cascading"
)

// Type the type name of the policy
func (in *GarbageCollectPolicySpec) Type() string {
	return GarbageCollectPolicyType
}

// FindStrategy find gc strategy for target resource
func (in *GarbageCollectPolicySpec) FindStrategy(manifest *unstructured.Unstructured) *GarbageCollectStrategy {
	for _, rule := range in.Rules {
		if rule.Selector.Match(manifest) {
			return &rule.Strategy
		}
	}
	return nil
}

// FindDeleteOption find delete option for target resource
func (in *GarbageCollectPolicySpec) FindDeleteOption(manifest *unstructured.Unstructured) []client.DeleteOption {
	for _, rule := range in.Rules {
		if rule.Selector.Match(manifest) && rule.Propagation != nil {
			switch *rule.Propagation {
			case GarbageCollectPropagationOrphan:
				return []client.DeleteOption{client.PropagationPolicy(metav1.DeletePropagationOrphan)}
			case GarbageCollectPropagationCascading:
				return []client.DeleteOption{client.PropagationPolicy(metav1.DeletePropagationBackground)}
			}
		}
	}
	return nil
}
