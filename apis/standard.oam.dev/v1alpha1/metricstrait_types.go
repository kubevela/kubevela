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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// MetricsTraitSpec defines the desired state of MetricsTrait
type MetricsTraitSpec struct {
	// An endpoint to be monitored by a ServiceMonitor.
	ScrapeService ScapeServiceEndPoint `json:"scrapeService"`
	// WorkloadReference to the workload whose metrics needs to be exposed
	WorkloadReference runtimev1alpha1.TypedReference `json:"workloadRef,omitempty"`
}

// ScapeServiceEndPoint defines a scrapeable endpoint serving Prometheus metrics.
type ScapeServiceEndPoint struct {
	// The format of the metrics data,
	// The default and only supported format is "prometheus" for now
	Format string `json:"format,omitempty"`
	// Number or name of the port to access on the pods targeted by the service.
	// The default is discovered automatically from podTemplate, metricTrait will create a service for the workload
	TargetPort intstr.IntOrString `json:"port,omitempty"`
	// Route service traffic to pods with label keys and values matching this
	// The default is discovered automatically from podTemplate.
	// If no podTemplate, use the labels specified here, or use the labels of the workload
	TargetSelector map[string]string `json:"selector,omitempty"`
	// HTTP path to scrape for metrics.
	// default is /metrics
	// +optional
	Path string `json:"path,omitempty"`
	// Scheme at which metrics should be scraped
	// The default and only supported scheme is "http"
	// +optional
	Scheme string `json:"scheme,omitempty"`
	// The default is true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// MetricsTraitStatus defines the observed state of MetricsTrait
type MetricsTraitStatus struct {
	runtimev1alpha1.ConditionedStatus `json:",inline"`

	// ServiceMonitorName managed by this trait
	ServiceMonitorName string `json:"serviceMonitorName,omitempty"`

	// Port is the real port monitoring
	Port intstr.IntOrString `json:"port,omitempty"`
	// SelectorLabels is the real labels selected
	SelectorLabels map[string]string `json:"selectorLabels,omitempty"`
}

// +kubebuilder:object:root=true

// MetricsTrait is the Schema for the metricstraits API
// +kubebuilder:resource:categories={oam}
// +kubebuilder:subresource:status
type MetricsTrait struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MetricsTraitSpec   `json:"spec"`
	Status MetricsTraitStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MetricsTraitList contains a list of MetricsTrait
type MetricsTraitList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MetricsTrait `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MetricsTrait{}, &MetricsTraitList{})
}

var _ oam.Trait = &MetricsTrait{}

// SetConditions for set CR condition
func (tr *MetricsTrait) SetConditions(c ...runtimev1alpha1.Condition) {
	tr.Status.SetConditions(c...)
}

// GetCondition for get CR condition
func (tr *MetricsTrait) GetCondition(c runtimev1alpha1.ConditionType) runtimev1alpha1.Condition {
	return tr.Status.GetCondition(c)
}

// GetWorkloadReference of this MetricsTrait.
func (tr *MetricsTrait) GetWorkloadReference() runtimev1alpha1.TypedReference {
	return tr.Spec.WorkloadReference
}

// SetWorkloadReference of this MetricsTrait.
func (tr *MetricsTrait) SetWorkloadReference(r runtimev1alpha1.TypedReference) {
	tr.Spec.WorkloadReference = r
}
