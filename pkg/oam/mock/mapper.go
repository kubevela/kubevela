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

package mock

import (
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var _ discoverymapper.DiscoveryMapper = &DiscoveryMapper{}

// GetMapper is func type for mock convenience
type GetMapper func() (meta.RESTMapper, error)

// Refresh is func type for mock convenience
type Refresh func() (meta.RESTMapper, error)

// RESTMapping is func type for mock convenience
type RESTMapping func(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error)

// KindsFor is func type for mock convenience
type KindsFor func(input schema.GroupVersionResource) ([]schema.GroupVersionKind, error)

// ResourcesFor is func type for mock convenience
type ResourcesFor func(input schema.GroupVersionKind) (schema.GroupVersionResource, error)

// NewMockDiscoveryMapper for unit test only
func NewMockDiscoveryMapper() *DiscoveryMapper {
	return &DiscoveryMapper{
		MockRESTMapping: NewMockRESTMapping(""),
		MockKindsFor:    NewMockKindsFor(""),
	}
}

// NewMockRESTMapping for unit test only
func NewMockRESTMapping(resource string) RESTMapping {
	return func(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
		return &meta.RESTMapping{Resource: schema.GroupVersionResource{Resource: resource, Version: versions[0], Group: gk.Group}}, nil
	}
}

// NewMockKindsFor for unit test only
func NewMockKindsFor(kind string, version ...string) KindsFor {
	return func(input schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
		if len(kind) <= 1 {
			return []schema.GroupVersionKind{{Version: input.Version, Group: input.Group, Kind: kind}}, nil
		}
		var ss []schema.GroupVersionKind
		for _, v := range version {
			gvk := schema.GroupVersionKind{Version: v, Group: input.Group, Kind: kind}
			if input.Version != "" && input.Version == v {
				return []schema.GroupVersionKind{gvk}, nil
			}
			ss = append(ss, gvk)
		}
		return ss, nil
	}
}

// DiscoveryMapper for unit test only, use GetMapper and refresh will panic
type DiscoveryMapper struct {
	MockGetMapper    GetMapper
	MockRefresh      Refresh
	MockRESTMapping  RESTMapping
	MockKindsFor     KindsFor
	MockResourcesFor ResourcesFor
}

// GetMapper for mock
func (m *DiscoveryMapper) GetMapper() (meta.RESTMapper, error) {
	return m.MockGetMapper()
}

// Refresh for mock
func (m *DiscoveryMapper) Refresh() (meta.RESTMapper, error) {
	return m.MockRefresh()
}

// RESTMapping for mock
func (m *DiscoveryMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	return m.MockRESTMapping(gk, versions...)
}

// KindsFor for mock
func (m *DiscoveryMapper) KindsFor(input schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	return m.MockKindsFor(input)
}

// ResourcesFor for mock
func (m *DiscoveryMapper) ResourcesFor(input schema.GroupVersionKind) (schema.GroupVersionResource, error) {
	var gvr schema.GroupVersionResource
	mapping, err := m.RESTMapping(input.GroupKind(), input.Version)
	if err != nil {
		return gvr, err
	}
	gvr = mapping.Resource
	return gvr, nil
}
