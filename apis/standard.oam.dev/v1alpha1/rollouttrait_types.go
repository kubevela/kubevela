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
	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/pkg/oam"
)

// RolloutTraitSpec defines the desired state of RolloutTrait
type RolloutTraitSpec struct {
	// TargetRef references a target resource that contains the newer version
	// of the software. We assumed that new resource already exists.
	// This is the only resource we work on if the resource is a stateful resource (cloneset/statefulset)
	TargetRef runtimev1alpha1.TypedReference `json:"targetRef"`

	// SourceRef references the list of resources that contains the older version
	// of the software. We assume that it's the first time to deploy when we cannot find any source.
	// +optional
	SourceRef []runtimev1alpha1.TypedReference `json:"sourceRef,omitempty"`

	// RolloutPlan is the details on how to rollout the resources
	RolloutPlan RolloutPlan `json:"rolloutPlan"`
}

// RolloutTrait is the Schema for the RolloutTrait API
// +kubebuilder:object:root=true
// +genclient
// +kubebuilder:resource:categories={oam}
// +kubebuilder:subresource:status
type RolloutTrait struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RolloutTraitSpec `json:"spec,omitempty"`
	Status RolloutStatus    `json:"status,omitempty"`
}

// RolloutTraitList contains a list of RolloutTrait
// +kubebuilder:object:root=true
// +genclient
type RolloutTraitList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RolloutTrait `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RolloutTrait{}, &RolloutTraitList{})
}

var _ oam.Trait = &RolloutTrait{}

// SetConditions for set CR condition
func (tr *RolloutTrait) SetConditions(c ...runtimev1alpha1.Condition) {
	tr.Status.SetConditions(c...)
}

// GetCondition for get CR condition
func (tr *RolloutTrait) GetCondition(c runtimev1alpha1.ConditionType) runtimev1alpha1.Condition {
	return tr.Status.GetCondition(c)
}

// GetWorkloadReference of this MetricsTrait.
func (tr *RolloutTrait) GetWorkloadReference() runtimev1alpha1.TypedReference {
	return tr.Spec.TargetRef
}

// SetWorkloadReference of this MetricsTrait.
func (tr *RolloutTrait) SetWorkloadReference(r runtimev1alpha1.TypedReference) {
	tr.Spec.TargetRef = r
}
