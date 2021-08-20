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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"

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

	// AppRefs records references of applications' components
	AppRefs []AppReference `json:"appReferences,omitempty"`

	// WorkloadReferences to the workloads that are in this scope.
	// +deprecated
	WorkloadReferences []corev1.ObjectReference `json:"workloadRefs"`
}

// AppReference records references of an application's components
type AppReference struct {
	AppName        string          `json:"appName,omitempty"`
	CompReferences []CompReference `json:"compReferences,omitempty"`
}

// CompReference records references of a component's resources
type CompReference struct {
	CompName string                   `json:"compName,omitempty"`
	Workload corev1.ObjectReference   `json:"workload,omitempty"`
	Traits   []corev1.ObjectReference `json:"traits,omitempty"`
}

// A HealthScopeStatus represents the observed state of a HealthScope.
type HealthScopeStatus struct {
	condition.ConditionedStatus `json:",inline"`

	// ScopeHealthCondition represents health condition summary of the scope
	ScopeHealthCondition ScopeHealthCondition `json:"scopeHealthCondition"`

	// AppHealthConditions represents health condition of applications in the scope
	AppHealthConditions []*AppHealthCondition `json:"appHealthConditions,omitempty"`

	// WorkloadHealthConditions represents health condition of workloads in the scope
	// Use AppHealthConditions to provide app level status
	// +deprecated
	WorkloadHealthConditions []*WorkloadHealthCondition `json:"healthConditions,omitempty"`
}

// AppHealthCondition represents health condition of an application
type AppHealthCondition struct {
	AppName    string                     `json:"appName"`
	Components []*WorkloadHealthCondition `json:"components,omitempty"`
}

// ScopeHealthCondition represents health condition summary of a scope.
type ScopeHealthCondition struct {
	HealthStatus       HealthStatus `json:"healthStatus"`
	Total              int64        `json:"total,omitempty"`
	HealthyWorkloads   int64        `json:"healthyWorkloads,omitempty"`
	UnhealthyWorkloads int64        `json:"unhealthyWorkloads,omitempty"`
	UnknownWorkloads   int64        `json:"unknownWorkloads,omitempty"`
}

// WorkloadHealthCondition represents informative health condition of a workload.
type WorkloadHealthCondition struct {
	// ComponentName represents the component name if target is a workload
	ComponentName  string                 `json:"componentName,omitempty"`
	TargetWorkload corev1.ObjectReference `json:"targetWorkload,omitempty"`
	HealthStatus   HealthStatus           `json:"healthStatus"`
	Diagnosis      string                 `json:"diagnosis,omitempty"`
	// WorkloadStatus represents status of workloads whose HealthStatus is UNKNOWN.
	WorkloadStatus  string                  `json:"workloadStatus,omitempty"`
	CustomStatusMsg string                  `json:"customStatusMsg,omitempty"`
	Traits          []*TraitHealthCondition `json:"traits,omitempty"`
}

// TraitHealthCondition represents informative health condition of a trait.
type TraitHealthCondition struct {
	Type            string       `json:"type"`
	Resource        string       `json:"resource"`
	HealthStatus    HealthStatus `json:"healthStatus"`
	Diagnosis       string       `json:"diagnosis,omitempty"`
	CustomStatusMsg string       `json:"customStatusMsg,omitempty"`
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
