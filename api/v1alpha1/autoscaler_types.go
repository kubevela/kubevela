/*


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
	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// Protocol defines network protocols supported for things like container ports.
type Protocol string

// TriggerType defines the type of trigger
type TriggerType string

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories={oam}
// Autoscaler is the Schema for the autoscalers API
type Autoscaler struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AutoscalerSpec   `json:"spec"`
	Status AutoscalerStatus `json:"status,omitempty"`
}

func (as *Autoscaler) SetConditions(c ...v1alpha1.Condition) {
	as.Status.SetConditions(c...)
}

func (as *Autoscaler) GetCondition(conditionType v1alpha1.ConditionType) v1alpha1.Condition {
	return as.Status.GetCondition(conditionType)
}

func (as *Autoscaler) GetWorkloadReference() v1alpha1.TypedReference {
	return as.Spec.WorkloadReference
}

func (as *Autoscaler) SetWorkloadReference(reference v1alpha1.TypedReference) {
	as.Spec.WorkloadReference = reference
}

type DefaultCondition struct {
	// Target is the threshold value to the metric
	Target *int32 `json:"target,omitempty"`
}

type CronTypeCondition struct {
	// StartAt is the time when the scaler starts, in format `"HHMM"` for example, "08:00"
	StartAt string `json:"startAt,omitempty"`

	// Duration means how long the target scaling will keep, after the time of duration, the scaling will stop
	Duration string `json:"duration,omitempty"`

	// Days means in which days the condition will take effect
	Days []string `json:"days,omitempty"`

	// Replicas is the expected replicas
	Replicas int `json:"replicas,omitempty"`

	// Timezone defines the time zone, default to the timezone of the Kubernetes cluster
	Timezone string `json:"timezone,omitempty"`
}

// TriggerCondition set the condition when to trigger scaling
type TriggerCondition struct {
	// DefaultCondition is the condition for resource types, like `cpu/memory/storage/ephemeral-storage`
	*DefaultCondition `json:",inline,omitempty"`

	// CronTypeCondition is the condition for Cron type scaling, `cron`
	*CronTypeCondition `json:",inline,omitempty"`
}

// Trigger defines the trigger of Autoscaler
type Trigger struct {
	// Name is the trigger name, if not set, it will be automatically generated and make it globally unique
	Name string `json:"name,omitempty"`

	// Enabled marks whether the trigger immediately. Defaults to `true`
	Enabled bool `json:"enabled,omitempty"`

	// Type allows value in [cpu/memory/storage/ephemeral-storage、cron、pps、qps/rps、custom]
	Type TriggerType `json:"type"`

	// Condition set the condition when to trigger scaling
	Condition TriggerCondition `json:"condition"`
}

// AutoscalerSpec defines the desired state of Autoscaler
type AutoscalerSpec struct {
	// MinReplicas is the minimal replicas
	// +optional
	MinReplicas *int32 `json:"minReplicas,omitempty"`

	// MinReplicas is the maximal replicas
	// +optional
	MaxReplicas *int32 `json:"maxReplicas,omitempty"`

	// Triggers lists all triggers
	Triggers []Trigger `json:"triggers"`

	// TargetWorkload specify the workload which is about to be scaled
	TargetWorkload TargetWorkload `json:"targetWorkload,omitempty"`

	// WorkloadReference specifies the workload which is about to be scaled
	WorkloadReference runtimev1alpha1.TypedReference `json:"workloadRef,omitempty"`
}

// TargetWorkload holds the a reference to the scale target Object
type TargetWorkload struct {
	Name string `json:"name"`
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`
	// +optional
	Kind string `json:"kind,omitempty"`
}

// AutoscalerStatus defines the observed state of Autoscaler
type AutoscalerStatus struct {
	runtimev1alpha1.ConditionedStatus `json:",inline"`
}

// +kubebuilder:object:root=true

// AutoscalerList contains a list of Autoscaler
type AutoscalerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Autoscaler `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Autoscaler{}, &AutoscalerList{})
}
