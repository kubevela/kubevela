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

package apis

// APIs copied from fluxcd/helmcontroller

/*
   Copyright 2021 The Flux CD contributors.
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

// CrossNamespaceObjectReference contains enough information to let you locate
// the typed referenced object at cluster level.
type CrossNamespaceObjectReference struct {
	// APIVersion of the referent.
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`

	// Kind of the referent.
	// +kubebuilder:validation:Enum=HelmRepository;GitRepository;Bucket
	// +required
	Kind string `json:"kind,omitempty"`

	// Name of the referent.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +required
	Name string `json:"name"`

	// Namespace of the referent.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Optional
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// ValuesReference contains a reference to a resource containing Helm values,
// and optionally the key they can be found at.
type ValuesReference struct {
	// Kind of the values referent, valid values are ('Secret', 'ConfigMap').
	// +kubebuilder:validation:Enum=Secret;ConfigMap
	// +required
	Kind string `json:"kind"`

	// Name of the values referent. Should reside in the same namespace as the
	// referring resource.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +required
	Name string `json:"name"`

	// ValuesKey is the data key where the values.yaml or a specific value can be
	// found at. Defaults to 'values.yaml'.
	// +optional
	ValuesKey string `json:"valuesKey,omitempty"`

	// TargetPath is the YAML dot notation path the value should be merged at. When
	// set, the ValuesKey is expected to be a single flat value. Defaults to 'None',
	// which results in the values getting merged at the root.
	// +optional
	TargetPath string `json:"targetPath,omitempty"`

	// Optional marks this ValuesReference as optional. When set, a not found error
	// for the values reference is ignored, but any ValuesKey, TargetPath or
	// transient error will still result in a reconciliation failure.
	// +optional
	Optional bool `json:"optional,omitempty"`
}
