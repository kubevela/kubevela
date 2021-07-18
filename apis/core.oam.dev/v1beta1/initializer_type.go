/*
 Copyright 2021. The KubeVela Authors.

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

package v1beta1

import (
	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InitializerPhase is a label for the condition of a initializer at the current time
type InitializerPhase string

const (
	// InitializerCheckingDependsOn means the initializer is checking the status of dependent Initializer
	InitializerCheckingDependsOn InitializerPhase = "checkingDependsOn"
	// InitializerInitializing means the initializer is initializing
	InitializerInitializing InitializerPhase = "initializing"
	// InitializerSuccess means the initializer successfully initialized the environment
	InitializerSuccess InitializerPhase = "success"
)

// DependsOn refer to an object which Initializer depends on
type DependsOn struct {
	Ref corev1.ObjectReference `json:"ref"`
}

// A InitializerSpec defines the desired state of a Initializer.
type InitializerSpec struct {
	// AppTemplate indicates the application template to render and deploy an system application.
	AppTemplate Application `json:"appTemplate"`

	// DependsOn indicates the other initializers that this depends on.
	// It will not apply its components until all dependencies exist.
	DependsOn []DependsOn `json:"dependsOn,omitempty"`
}

// InitializerStatus is the status of Initializer
type InitializerStatus struct {
	// ConditionedStatus reflects the observed status of a resource
	runtimev1alpha1.ConditionedStatus `json:",inline"`

	Phase InitializerPhase `json:"status,omitempty"`

	// The generation observed by the Initializer controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration"`
}

// +kubebuilder:object:root=true

// Initializer is the Schema for the Initializer API
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={oam},shortName=init
// +kubebuilder:printcolumn:name="PHASE",type=string,JSONPath=`.status.status`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=".metadata.creationTimestamp"
type Initializer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InitializerSpec   `json:"spec,omitempty"`
	Status InitializerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// InitializerList contains a list of Initializer.
type InitializerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Initializer `json:"items"`
}

// SetConditions set condition for Initializer
func (i *Initializer) SetConditions(c ...runtimev1alpha1.Condition) {
	i.Status.SetConditions(c...)
}

// GetCondition gets condition from Initializer
func (i *Initializer) GetCondition(conditionType runtimev1alpha1.ConditionType) runtimev1alpha1.Condition {
	return i.Status.GetCondition(conditionType)
}
