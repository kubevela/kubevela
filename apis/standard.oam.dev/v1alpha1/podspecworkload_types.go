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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/pkg/oam"
)

// PodSpecWorkloadSpec defines the desired state of PodSpecWorkload
type PodSpecWorkloadSpec struct {
	// Replicas is the desired number of replicas of the given podSpec.
	// These are replicas in the sense that they are instantiations of the same podSpec.
	// If unspecified, defaults to 1.
	Replicas *int32 `json:"replicas,omitempty"`

	// PodSpec describes the pods that will be created,
	// we omit the meta part as it will be exactly the same as the PodSpecWorkload
	PodSpec v1.PodSpec `json:"podSpec"`
}

// PodSpecWorkloadStatus defines the observed state of PodSpecWorkload
type PodSpecWorkloadStatus struct {
	cpv1alpha1.ConditionedStatus `json:",inline"`

	// Resources managed by this workload.
	Resources []cpv1alpha1.TypedReference `json:"resources,omitempty"`
}

// +kubebuilder:object:root=true

// PodSpecWorkload is the Schema for the PodSpec API
// +genclient:method=GetScale,verb=get,subresource=scale,result=k8s.io/api/autoscaling/v1.Scale
// +genclient:method=UpdateScale,verb=update,subresource=scale,input=k8s.io/api/autoscaling/v1.Scale,result=k8s.io/api/autoscaling/v1.Scale
// +kubebuilder:resource:categories={oam}
// +kubebuilder:subresource:status
type PodSpecWorkload struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PodSpecWorkloadSpec   `json:"spec,omitempty"`
	Status PodSpecWorkloadStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PodSpecWorkloadList contains a list of PodSpecWorkload
type PodSpecWorkloadList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PodSpecWorkload `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PodSpecWorkload{}, &PodSpecWorkloadList{})
}

var _ oam.Workload = &PodSpecWorkload{}

// SetConditions set condition for this CR
func (in *PodSpecWorkload) SetConditions(c ...cpv1alpha1.Condition) {
	in.Status.SetConditions(c...)
}

// GetCondition set condition for this CR
func (in *PodSpecWorkload) GetCondition(c cpv1alpha1.ConditionType) cpv1alpha1.Condition {
	return in.Status.GetCondition(c)
}
