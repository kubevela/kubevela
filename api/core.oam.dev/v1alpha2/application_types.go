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
	"encoding/json"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ApplicationSpec defines the desired state of Application
type ApplicationSpec map[string]anyone

// Maps generate maps
func (spec ApplicationSpec) Maps() (map[string]interface{}, error) {
	ret := map[string]interface{}{}
	for k, v := range spec {
		var _v interface{}
		if err := json.Unmarshal(v.Raw, &_v); err != nil {
			return nil, err
		}
		ret[k] = _v
	}
	return ret, nil
}

type anyone struct {
	// Raw will hold the complete serialized object
	Raw []byte `json:"-"`
}

// UnmarshalJSON implement jsonUnmarshal
func (any *anyone) UnmarshalJSON(in []byte) error {
	any.Raw = in
	return nil
}

// MarshalJSON implement jsonMarshal
func (any *anyone) MarshalJSON() ([]byte, error) {
	return any.Raw, nil
}

// DeepCopy method
func (spec *ApplicationSpec) DeepCopy() *ApplicationSpec {
	ret := *spec
	return &ret
}

// RenderStatus is Application Render Status
type RenderStatus string

// ApplicationStatus defines the observed state of Application
type ApplicationStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	runtimev1alpha1.ConditionedStatus `json:",inline"`

	Phase RenderStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// Application is the Schema for the applications API
// +kubebuilder:subresource:status
type Application struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationSpec   `json:"spec,omitempty"`
	Status ApplicationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ApplicationList contains a list of Application
type ApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Application `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Application{}, &ApplicationList{})
}
