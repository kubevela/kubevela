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

package v1alpha1

const (
	// RefObjectsComponentType refers to the type of ref-objects
	RefObjectsComponentType = "ref-objects"
)

// RefObjectsComponentSpec defines the spec of ref-objects component
type RefObjectsComponentSpec struct {
	// Objects the referrers to the Kubernetes objects
	Objects []ObjectReferrer `json:"objects,omitempty"`
	// URLs are the links that stores the referred objects
	URLs []string `json:"urls,omitempty"`
}

// ObjectReferrer selects Kubernetes objects
type ObjectReferrer struct {
	// ObjectTypeIdentifier identifies the type of referred objects
	ObjectTypeIdentifier `json:",inline"`
	// ObjectSelector select object by name or labelSelector
	ObjectSelector `json:",inline"`
}

// ObjectTypeIdentifier identifies the scheme of Kubernetes object
type ObjectTypeIdentifier struct {
	// Resource is the resource name of the Kubernetes object.
	Resource string `json:"resource"`
	// Group is the API Group of the Kubernetes object.
	Group string `json:"group"`
	// LegacyObjectTypeIdentifier is the legacy identifier
	// Deprecated: use resource/group instead
	LegacyObjectTypeIdentifier `json:",inline"`
}

// LegacyObjectTypeIdentifier legacy object type identifier
type LegacyObjectTypeIdentifier struct {
	// APIVersion is the APIVersion of the Kubernetes object.
	APIVersion string `json:"apiVersion"`
	// APIVersion is the Kind of the Kubernetes object.
	Kind string `json:"kind"`
}

// ObjectSelector selector for Kubernetes object
type ObjectSelector struct {
	// Name is the name of the Kubernetes object.
	// If empty, it will inherit the application component's name.
	Name string `json:"name,omitempty"`
	// Namespace is the namespace for selecting Kubernetes objects.
	// If empty, it will inherit the application's namespace.
	Namespace string `json:"namespace,omitempty"`
	// Cluster is the cluster for selecting Kubernetes objects.
	// If empty, it will use the local cluster
	Cluster string `json:"cluster,omitempty"`
	// LabelSelector selects Kubernetes objects by labels
	// Exclusive to "name"
	LabelSelector map[string]string `json:"labelSelector,omitempty"`
	// DeprecatedLabelSelector a deprecated alias to LabelSelector
	// Deprecated: use labelSelector instead.
	DeprecatedLabelSelector map[string]string `json:"selector,omitempty"`
}
