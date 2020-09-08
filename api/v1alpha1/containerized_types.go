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
	cpv1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ContainerizedSpec defines the desired state of Containerized
type ContainerizedSpec struct {
	// Replicas is the desired number of replicas of the given podSpec.
	// These are replicas in the sense that they are instantiations of the same podSpec.
	// If unspecified, defaults to 1.
	Replicas *int32 `json:"replicas"`

	// PodSpec describes the pods that will be created,
	// we omit the meta part as it will be exactly the same as the containerized
	PodSpec v1.PodSpec `json:"podSpec"`
}

// ContainerizedStatus defines the observed state of Containerized
type ContainerizedStatus struct {
	cpv1alpha1.ConditionedStatus `json:",inline"`

	// Resources managed by this workload.
	Resources []cpv1alpha1.TypedReference `json:"resources,omitempty"`
}

// +kubebuilder:object:root=true

// Containerized is the Schema for the containerizeds API
// +kubebuilder:resource:categories={oam}
// +kubebuilder:subresource:status
type Containerized struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ContainerizedSpec   `json:"spec,omitempty"`
	Status ContainerizedStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ContainerizedList contains a list of Containerized
type ContainerizedList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Containerized `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Containerized{}, &ContainerizedList{})
}

var _ oam.Workload = &Containerized{}

func (in *Containerized) SetConditions(c ...cpv1alpha1.Condition) {
	in.Status.SetConditions(c...)
}

func (in *Containerized) GetCondition(c cpv1alpha1.ConditionType) cpv1alpha1.Condition {
	return in.Status.GetCondition(c)
}
