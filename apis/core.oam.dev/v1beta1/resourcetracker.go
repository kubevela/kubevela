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

package v1beta1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/interfaces"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// An ResourceTracker represents a tracker for track cross namespace resources
// +kubebuilder:resource:scope=Cluster,categories={oam},shortName=tracker
type ResourceTracker struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Status ResourceTrackerStatus `json:"status,omitempty"`
}

// ResourceTrackerStatus define the status of resourceTracker
type ResourceTrackerStatus struct {
	TrackedResources []common.ClusterObjectReference `json:"trackedResources,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ResourceTrackerList contains a list of ResourceTracker
type ResourceTrackerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ResourceTracker `json:"items"`
}

// ToOwnerReference convert ResourceTracker into owner reference for other resource to refer
func (in *ResourceTracker) ToOwnerReference() *metav1.OwnerReference {
	return &metav1.OwnerReference{
		APIVersion:         SchemeGroupVersion.String(),
		Kind:               ResourceTrackerKind,
		Name:               in.Name,
		UID:                in.UID,
		Controller:         pointer.BoolPtr(true),
		BlockOwnerDeletion: pointer.BoolPtr(true),
	}
}

// AddOwnerReferenceToTrackerResource add resourcetracker as owner reference to target object, return true if already exists (outdated)
func (in *ResourceTracker) AddOwnerReferenceToTrackerResource(rsc interfaces.ObjectOwner) bool {
	ownerRefs := []metav1.OwnerReference{*in.ToOwnerReference()}
	exists := false
	for _, owner := range rsc.GetOwnerReferences() {
		// delete the old resourceTracker owner
		if owner.Kind == ResourceTrackerKind && owner.APIVersion == SchemeGroupVersion.String() {
			exists = true
			continue
		}
		if owner.Controller != nil && *owner.Controller && owner.UID != in.UID {
			owner.Controller = pointer.BoolPtr(false)
		}
		ownerRefs = append(ownerRefs, owner)
	}
	rsc.SetOwnerReferences(ownerRefs)
	return exists
}

func (in *ResourceTracker) addClusterObjectReference(ref common.ClusterObjectReference) bool {
	for _, _rsc := range in.Status.TrackedResources {
		if _rsc.Equal(ref) {
			return true
		}
	}
	in.Status.TrackedResources = append(in.Status.TrackedResources, ref)
	return false
}

// AddTrackedResource add new object reference into tracked resources, return if already exists
func (in *ResourceTracker) AddTrackedResource(rsc interfaces.TrackableResource) bool {
	return in.addClusterObjectReference(common.ClusterObjectReference{
		ObjectReference: v1.ObjectReference{
			APIVersion: rsc.GetAPIVersion(),
			Kind:       rsc.GetKind(),
			Name:       rsc.GetName(),
			Namespace:  rsc.GetNamespace(),
			UID:        rsc.GetUID(),
		},
	})
}

// AddTrackedCluster add resourcetracker in remote cluster into tracked resources, return if already exists
func (in *ResourceTracker) AddTrackedCluster(clusterName string) bool {
	if clusterName == "" {
		return true
	}
	return in.addClusterObjectReference(common.ClusterObjectReference{
		Cluster: clusterName,
		ObjectReference: v1.ObjectReference{
			APIVersion: SchemeGroupVersion.String(),
			Kind:       ResourceTrackerKind,
			Name:       in.GetName(),
		},
	})
}

// GetTrackedClusters return remote clusters recorded in the resource tracker
func (in *ResourceTracker) GetTrackedClusters() (clusters []string) {
	for _, ref := range in.Status.TrackedResources {
		if ref.APIVersion == SchemeGroupVersion.String() && ref.Kind == ResourceTrackerKind && ref.Name == in.Name && ref.Cluster != "" {
			clusters = append(clusters, ref.Cluster)
		}
	}
	return
}

// IsLifeLong check if resourcetracker shares the same whole life with the entire application
func (in *ResourceTracker) IsLifeLong() bool {
	_, ok := in.GetAnnotations()[oam.AnnotationResourceTrackerLifeLong]
	return ok
}

// SetLifeLong set life long to resource tracker
func (in *ResourceTracker) SetLifeLong() {
	in.SetAnnotations(map[string]string{oam.AnnotationResourceTrackerLifeLong: "true"})
}
