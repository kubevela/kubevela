/*
Copyright 2022 The KubeVela Authors.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/apiserver-runtime/pkg/builder/resource"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// ApplicationResourceTracker is an extension model for ResourceTracker
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ApplicationResourceTracker v1beta1.ResourceTracker

// ApplicationResourceTrackerList list for ApplicationResourceTracker
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ApplicationResourceTrackerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ApplicationResourceTracker `json:"items"`
}

var _ resource.Object = &ApplicationResourceTracker{}

// GetObjectMeta returns the object meta reference.
func (in *ApplicationResourceTracker) GetObjectMeta() *metav1.ObjectMeta {
	return &in.ObjectMeta
}

// NamespaceScoped returns if the object must be in a namespace.
func (in *ApplicationResourceTracker) NamespaceScoped() bool {
	return true
}

// New returns a new instance of the resource
func (in *ApplicationResourceTracker) New() runtime.Object {
	return &ApplicationResourceTracker{}
}

// NewList return a new list instance of the resource
func (in *ApplicationResourceTracker) NewList() runtime.Object {
	return &ApplicationResourceTrackerList{}
}

// GetGroupVersionResource returns the GroupVersionResource for this resource.
func (in *ApplicationResourceTracker) GetGroupVersionResource() schema.GroupVersionResource {
	return GroupVersion.WithResource(ApplicationResourceTrackerResource)
}

// IsStorageVersion returns true if the object is also the internal version
func (in *ApplicationResourceTracker) IsStorageVersion() bool {
	return true
}
