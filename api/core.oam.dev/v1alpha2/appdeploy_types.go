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

package v1alpha2

import (
	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ApplicationDeploymentSpec defines the desired state of ApplicationDeployment
type ApplicationDeploymentSpec struct {
	//TODO add spec here
}

// ApplicationDeploymentStatus defines the observed state of ApplicationDeployment
type ApplicationDeploymentStatus struct {
	//TODO add status field here
	runtimev1alpha1.ConditionedStatus `json:",inline"`
}

// ApplicationDeployment is the Schema for the ApplicationDeployment API
// +kubebuilder:object:root=true
// +kubebuilder:resource:categories={oam}
// +kubebuilder:subresource:status
type ApplicationDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationDeploymentSpec   `json:"spec,omitempty"`
	Status ApplicationDeploymentStatus `json:"status,omitempty"`
}

// ApplicationDeploymentList contains a list of ApplicationDeployment
// +kubebuilder:object:root=true
type ApplicationDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApplicationDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ApplicationDeployment{}, &ApplicationDeploymentList{})
}
