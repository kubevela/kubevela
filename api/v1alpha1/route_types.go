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
	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RouteSpec defines the desired state of Route
type RouteSpec struct {
	// WorkloadReference to the workload whose metrics needs to be exposed
	WorkloadReference runtimev1alpha1.TypedReference `json:"workloadRef,omitempty"`

	// Host is the host of the route
	Host string `json:"host"`

	// Path is location Path, default for "/"
	Path string `json:"path,omitempty"`

	// TLS indicate route trait will create SSL secret using cert-manager with specified issuer
	// If this is nil, route trait will use a selfsigned issuer
	TLS *TLS `json:"tls,omitempty"`

	// DefaultBackend uses serviceName
	DefaultBackend *v1beta1.IngressBackend `json:"defaultBackend,omitempty"`

	// RewriteTarget will rewrite request from Path to RewriteTarget path.
	RewriteTarget string `json:"rewriteTarget,omitempty"`

	// CustomHeaders pass a custom list of headers to the backend service.
	CustomHeaders map[string]string `json:"customHeaders,omitempty"`

	// Backend indicate how to connect backend service
	// If it's nil, will auto discovery
	Backend *Backend `json:"backend,omitempty"`
}

type TLS struct {
	IssuerName string `json:"issuerName,omitempty"`

	// Type indicate the issuer is ClusterIssuer or NamespaceIssuer
	Type IssuerType `json:"type,omitempty"`
}

type IssuerType string

const (
	ClusterIssuer   IssuerType = "ClusterIssuer"
	NamespaceIssuer IssuerType = "Issuer"
)

// Route will automatically discover podTemplate for Port and SelectLabels if they are not set.
// If Port and SelectLabels are already set, discovery won't work.
// If Port is not set, the first port discovered will be set.
// If SelectLabels are not set, all selectorLabels discovered will be set.
type Backend struct {
	// Protocol means backend-protocol, HTTP, HTTPS, GRPC, GRPCS, AJP and FCGI, By default uses HTTP
	Protocol string `json:"protocol,omitempty"`
	// ReadTimeout used for setting read timeout duration for backend service, the unit is second.
	ReadTimeout int `json:"readTimeout,omitempty"`
	// SendTimeout used for setting send timeout duration for backend service, the unit is second.
	SendTimeout int `json:"sendTimeout,omitempty"`
	// Port points to backend service port.
	Port intstr.IntOrString `json:"port,omitempty"`
	// SelectLabels for backend service.
	SelectLabels map[string]string `json:"selectLabels,omitempty"`
}

// RouteStatus defines the observed state of Route
type RouteStatus struct {
	Ingress                           *runtimev1alpha1.TypedReference `json:"ingress,omitempty"`
	Service                           *runtimev1alpha1.TypedReference `json:"service,omitempty"`
	runtimev1alpha1.ConditionedStatus `json:",inline"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories={oam}
// +kubebuilder:subresource:status
// Route is the Schema for the routes API
type Route struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RouteSpec   `json:"spec,omitempty"`
	Status RouteStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// RouteList contains a list of Route
type RouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Route `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Route{}, &RouteList{})
}

var _ oam.Trait = &Route{}

func (r *Route) SetConditions(c ...runtimev1alpha1.Condition) {
	r.Status.SetConditions(c...)
}

func (r *Route) GetCondition(c runtimev1alpha1.ConditionType) runtimev1alpha1.Condition {
	return r.Status.GetCondition(c)
}

// GetWorkloadReference of this ManualScalerTrait.
func (r *Route) GetWorkloadReference() runtimev1alpha1.TypedReference {
	return r.Spec.WorkloadReference
}

// SetWorkloadReference of this ManualScalerTrait.
func (r *Route) SetWorkloadReference(rt runtimev1alpha1.TypedReference) {
	r.Spec.WorkloadReference = rt
}
