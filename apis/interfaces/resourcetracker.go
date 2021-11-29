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

package interfaces

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ObjectOwner is the interface for get and set ownerReference
type ObjectOwner interface {
	GetOwnerReferences() []metav1.OwnerReference
	SetOwnerReferences([]metav1.OwnerReference)
}

// TrackableResource is the interface for resources to be tracked by resourcetracker
type TrackableResource interface {
	client.Object
	metav1.Type
	ObjectOwner
}
