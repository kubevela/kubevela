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

package v1alpha2

import (
	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/pkg/oam"
)

// HealthStatus represents health status strings.
type HealthStatus string

const (
	// StatusHealthy represents healthy status.
	StatusHealthy HealthStatus = "HEALTHY"
	// StatusUnhealthy represents unhealthy status.
	StatusUnhealthy = "UNHEALTHY"
	// StatusUnknown represents unknown status.
	StatusUnknown = "UNKNOWN"
)

var _ oam.Scope = &HealthScope{}

// A HealthScopeSpec defines the desired state of a HealthScope.
type HealthScopeSpec struct {
	// ProbeTimeout is the amount of time in seconds to wait when receiving a response before marked failure.
	ProbeTimeout *int32 `json:"probe-timeout,omitempty"`

	// ProbeInterval is the amount of time in seconds between probing tries.
	ProbeInterval *int32 `json:"probe-interval,omitempty"`

	// WorkloadReferences to the workloads that are in this scope.
	WorkloadReferences []runtimev1alpha1.TypedReference `json:"workloadRefs"`
}

// A HealthScopeStatus represents the observed state of a HealthScope.
type HealthScopeStatus struct {
	runtimev1alpha1.ConditionedStatus `json:",inline"`

	// ScopeHealthCondition represents health condition summary of the scope
	ScopeHealthCondition ScopeHealthCondition `json:"scopeHealthCondition"`

	// WorkloadHealthConditions represents health condition of workloads in the scope
	WorkloadHealthConditions []*WorkloadHealthCondition `json:"healthConditions,omitempty"`
}

// ScopeHealthCondition represents health condition summary of a scope.
type ScopeHealthCondition struct {
	HealthStatus       HealthStatus `json:"healthStatus"`
	Total              int64        `json:"total,omitempty"`
	HealthyWorkloads   int64        `json:"healthyWorkloads,omitempty"`
	UnhealthyWorkloads int64        `json:"unhealthyWorkloads,omitempty"`
	UnknownWorkloads   int64        `json:"unknownWorkloads,omitempty"`
}

// WorkloadHealthCondition represents informative health condition.
type WorkloadHealthCondition struct {
	// ComponentName represents the component name if target is a workload
	ComponentName  string                         `json:"componentName,omitempty"`
	TargetWorkload runtimev1alpha1.TypedReference `json:"targetWorkload,omitempty"`
	HealthStatus   HealthStatus                   `json:"healthStatus"`
	Diagnosis      string                         `json:"diagnosis,omitempty"`
	// WorkloadStatus represents status of workloads whose HealthStatus is UNKNOWN.
	WorkloadStatus string `json:"workloadStatus,omitempty"`
}

// +kubebuilder:object:root=true

// A HealthScope determines an aggregate health status based of the health of components.
// +kubebuilder:resource:categories={oam}
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=".status.health",name=HEALTH,type=string
type HealthScope struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HealthScopeSpec   `json:"spec,omitempty"`
	Status HealthScopeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// HealthScopeList contains a list of HealthScope.
type HealthScopeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HealthScope `json:"items"`
}
