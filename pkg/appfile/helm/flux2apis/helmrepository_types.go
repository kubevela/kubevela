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

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// APIs copied from fluxcd/source-controller/api/v1beta1 @ api/v0.7.4

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

// HelmRepositorySpec defines the reference to a Helm repository.
type HelmRepositorySpec struct {
	// The Helm repository URL, a valid URL contains at least a protocol and host.
	// +required
	URL string `json:"url"`

	// The name of the secret containing authentication credentials for the Helm
	// repository.
	// For HTTP/S basic auth the secret must contain username and
	// password fields.
	// For TLS the secret must contain a certFile and keyFile, and/or
	// caCert fields.
	// +optional
	// SecretRef *meta.LocalObjectReference `json:"secretRef,omitempty"`

	// The interval at which to check the upstream for updates.
	// make it optional in KubeVela
	// +optional
	Interval *metav1.Duration `json:"interval,omitempty"`

	// The timeout of index downloading, defaults to 60s.
	// +kubebuilder:default:="60s"
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// This flag tells the controller to suspend the reconciliation of this source.
	// +optional
	Suspend bool `json:"suspend,omitempty"`
}
