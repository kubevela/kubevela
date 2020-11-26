package mock

import (
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/discoverymapper"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var _ discoverymapper.DiscoveryMapper = &DiscoveryMapper{}

// nolint
type GetMapper func() (meta.RESTMapper, error)

// nolint
type Refresh func() (meta.RESTMapper, error)

// nolint
type RESTMapping func(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error)

// nolint
type KindsFor func(input schema.GroupVersionResource) ([]schema.GroupVersionKind, error)

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
	MockGetMapper   GetMapper
	MockRefresh     Refresh
	MockRESTMapping RESTMapping
	MockKindsFor    KindsFor
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
