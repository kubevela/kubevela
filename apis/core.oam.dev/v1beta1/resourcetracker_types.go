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
	"encoding/json"
	"reflect"
	"strings"

	errors2 "github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/interfaces"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/errors"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// An ResourceTracker represents a tracker for track cross namespace resources
// +kubebuilder:printcolumn:name="TYPE",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="APP",type=string,JSONPath=`.metadata.labels['app\.oam\.dev\/name']`
// +kubebuilder:printcolumn:name="APP-NS",type=string,JSONPath=`.metadata.labels['app\.oam\.dev\/namespace']`
// +kubebuilder:printcolumn:name="APP-GEN",type=number,JSONPath=`.spec.applicationGeneration`
// +kubebuilder:resource:scope=Cluster,categories={oam},shortName=rt
type ResourceTracker struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResourceTrackerSpec   `json:"spec,omitempty"`
	Status ResourceTrackerStatus `json:"status,omitempty"`
}

// ResourceTrackerType defines the type of resourceTracker
type ResourceTrackerType string

const (
	// ResourceTrackerTypeRoot means resources in this resourceTracker will only be recycled when application is deleted
	ResourceTrackerTypeRoot = ResourceTrackerType("root")
	// ResourceTrackerTypeVersioned means resources in this resourceTracker will be recycled when this version is unused and this resource is not managed by latest RT
	ResourceTrackerTypeVersioned = ResourceTrackerType("versioned")
	// ResourceTrackerTypeComponentRevision stores all component revisions used
	ResourceTrackerTypeComponentRevision = ResourceTrackerType("component-revision")
)

// ResourceTrackerSpec define the spec of resourceTracker
type ResourceTrackerSpec struct {
	Type                  ResourceTrackerType `json:"type,omitempty"`
	ApplicationGeneration int64               `json:"applicationGeneration"`
	ManagedResources      []ManagedResource   `json:"managedResources,omitempty"`
}

// ManagedResource define the resource to be managed by ResourceTracker
type ManagedResource struct {
	common.ClusterObjectReference `json:",inline"`
	common.OAMObjectReference     `json:",inline"`
	// +kubebuilder:pruning:PreserveUnknownFields
	Data *runtime.RawExtension `json:"raw,omitempty"`
	// Deleted marks the resource to be deleted
	Deleted bool `json:"deleted,omitempty"`
	// DependsOn records the component that was dependent
	DependsOn []string `json:"dependsOn,omitempty"`
}

// Equal check if two managed resource equals
func (in ManagedResource) Equal(r ManagedResource) bool {
	if !in.ClusterObjectReference.Equal(r.ClusterObjectReference) {
		return false
	}
	if !in.OAMObjectReference.Equal(r.OAMObjectReference) {
		return false
	}
	return reflect.DeepEqual(in.Data, r.Data)
}

// DisplayName readable name for locating resource
func (in ManagedResource) DisplayName() string {
	s := in.Kind + " " + in.Name
	if in.Namespace != "" || in.Cluster != "" {
		s += " ("
		if in.Cluster != "" {
			s += "Cluster: " + in.Cluster
			if in.Namespace != "" {
				s += ", "
			}
		}
		if in.Namespace != "" {
			s += "Namespace: " + in.Namespace
		}
		s += ")"
	}
	return s
}

// NamespacedName namespacedName
func (in ManagedResource) NamespacedName() types.NamespacedName {
	return types.NamespacedName{Namespace: in.Namespace, Name: in.Name}
}

// ResourceKey computes the key for managed resource, resources with the same key points to the same resource
func (in ManagedResource) ResourceKey() string {
	gv, kind := in.GroupVersionKind().ToAPIVersionAndKind()
	return strings.Join([]string{gv, kind, in.Cluster, in.Namespace, in.Name}, "/")
}

// ComponentKey computes the key for the component which managed resource belongs to
func (in ManagedResource) ComponentKey() string {
	return strings.Join([]string{in.Env, in.Component}, "/")
}

// UnmarshalTo unmarshal ManagedResource into target object
func (in ManagedResource) UnmarshalTo(obj interface{}) error {
	if in.Data == nil || in.Data.Raw == nil {
		return errors.ManagedResourceHasNoDataError{}
	}
	return json.Unmarshal(in.Data.Raw, obj)
}

// ToUnstructured converts managed resource into unstructured
func (in ManagedResource) ToUnstructured() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(in.GroupVersionKind())
	obj.SetName(in.Name)
	if in.Namespace != "" {
		obj.SetNamespace(in.Namespace)
	}
	oam.SetCluster(obj, in.Cluster)
	return obj
}

// ToUnstructuredWithData converts managed resource into unstructured and unmarshal data
func (in ManagedResource) ToUnstructuredWithData() (*unstructured.Unstructured, error) {
	obj := in.ToUnstructured()
	if err := in.UnmarshalTo(obj); err != nil {
		if errors2.Is(err, errors.ManagedResourceHasNoDataError{}) {
			return nil, err
		}
	}
	return obj, nil
}

// ResourceTrackerStatus define the status of resourceTracker
// For backward-compatibility
type ResourceTrackerStatus struct {
	// Deprecated
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

func (in *ResourceTracker) findMangedResourceIndex(mr ManagedResource) int {
	for i, _mr := range in.Spec.ManagedResources {
		if mr.ClusterObjectReference.Equal(_mr.ClusterObjectReference) {
			return i
		}
	}
	return -1
}

// AddManagedResource add object to managed resources, if exists, update
func (in *ResourceTracker) AddManagedResource(rsc client.Object, dependsOn []string, metaOnly bool) (updated bool) {
	gvk := rsc.GetObjectKind().GroupVersionKind()
	mr := ManagedResource{
		ClusterObjectReference: common.ClusterObjectReference{
			ObjectReference: v1.ObjectReference{
				APIVersion: gvk.GroupVersion().String(),
				Kind:       gvk.Kind,
				Name:       rsc.GetName(),
				Namespace:  rsc.GetNamespace(),
			},
			Cluster: oam.GetCluster(rsc),
		},
		OAMObjectReference: common.NewOAMObjectReferenceFromObject(rsc),
		Deleted:            false,
		DependsOn:          dependsOn,
	}
	if !metaOnly {
		mr.Data = &runtime.RawExtension{Object: rsc}
	}
	if idx := in.findMangedResourceIndex(mr); idx >= 0 {
		if reflect.DeepEqual(in.Spec.ManagedResources[idx], mr) {
			return false
		}
		in.Spec.ManagedResources[idx] = mr
	} else {
		in.Spec.ManagedResources = append(in.Spec.ManagedResources, mr)
	}
	return true
}

// DeleteManagedResource if remove flag is on, it will remove the object from recorded resources.
// otherwise, it will mark the object as deleted instead of removing it
// workflow   stage: resources are marked as deleted (and execute the deletion action)
// state-keep stage: resources marked as deleted and successfully deleted will be removed from resourcetracker
func (in *ResourceTracker) DeleteManagedResource(rsc client.Object, remove bool) (updated bool) {
	gvk := rsc.GetObjectKind().GroupVersionKind()
	mr := ManagedResource{
		ClusterObjectReference: common.ClusterObjectReference{
			ObjectReference: v1.ObjectReference{
				APIVersion: gvk.GroupVersion().String(),
				Kind:       gvk.Kind,
				Name:       rsc.GetName(),
				Namespace:  rsc.GetNamespace(),
			},
			Cluster: oam.GetCluster(rsc),
		},
		Deleted: true,
	}
	if idx := in.findMangedResourceIndex(mr); idx >= 0 {
		if remove {
			in.Spec.ManagedResources = append(in.Spec.ManagedResources[:idx], in.Spec.ManagedResources[idx+1:]...)
		} else {
			if reflect.DeepEqual(in.Spec.ManagedResources[idx], mr) {
				return false
			}
			in.Spec.ManagedResources[idx] = mr
		}
	} else {
		if !remove {
			in.Spec.ManagedResources = append(in.Spec.ManagedResources, mr)
		}
	}
	return true
}

// addClusterObjectReference
// Deprecated
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
// Deprecated
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
