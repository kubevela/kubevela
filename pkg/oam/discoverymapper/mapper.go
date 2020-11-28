package discoverymapper

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
)

// DiscoveryMapper is a interface for refresh and discovery resources from GVK.
type DiscoveryMapper interface {
	GetMapper() (meta.RESTMapper, error)
	Refresh() (meta.RESTMapper, error)
	RESTMapping(gk schema.GroupKind, version ...string) (*meta.RESTMapping, error)
	KindsFor(input schema.GroupVersionResource) ([]schema.GroupVersionKind, error)
}

var _ DiscoveryMapper = &DefaultDiscoveryMapper{}

// DefaultDiscoveryMapper is a K8s resource mapper for discovery, it will cache the result
type DefaultDiscoveryMapper struct {
	dc     *discovery.DiscoveryClient
	mapper meta.RESTMapper
}

// New will create a new DefaultDiscoveryMapper by giving a K8s rest config
func New(c *rest.Config) (DiscoveryMapper, error) {
	dc, err := discovery.NewDiscoveryClientForConfig(c)
	if err != nil {
		return nil, err
	}
	return &DefaultDiscoveryMapper{
		dc: dc,
	}, nil
}

// GetMapper will get the cached restmapper, if nil, it will create one by refresh
// Prefer lazy discovery, because resources created after refresh can not be found
func (d *DefaultDiscoveryMapper) GetMapper() (meta.RESTMapper, error) {
	if d.mapper == nil {
		return d.Refresh()
	}
	return d.mapper, nil
}

// Refresh will re-create the mapper by getting the new resource from K8s API by using discovery client
func (d *DefaultDiscoveryMapper) Refresh() (meta.RESTMapper, error) {
	gr, err := restmapper.GetAPIGroupResources(d.dc)
	if err != nil {
		return nil, err
	}
	d.mapper = restmapper.NewDiscoveryRESTMapper(gr)
	return d.mapper, nil
}

// RESTMapping will mapping resources from GVK, if not found, it will refresh from APIServer and try once again
func (d *DefaultDiscoveryMapper) RESTMapping(gk schema.GroupKind, version ...string) (*meta.RESTMapping, error) {
	mapper, err := d.GetMapper()
	if err != nil {
		return nil, err
	}
	mapping, err := mapper.RESTMapping(gk, version...)
	if meta.IsNoMatchError(err) {
		// if no kind match err, refresh and try once more.
		mapper, err = d.Refresh()
		if err != nil {
			return nil, err
		}
		mapping, err = mapper.RESTMapping(gk, version...)
	}
	return mapping, err
}

// KindsFor will get kinds from GroupVersionResource, if version not set, all resources matched will be returned.
func (d *DefaultDiscoveryMapper) KindsFor(input schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	mapper, err := d.GetMapper()
	if err != nil {
		return nil, err
	}
	mapping, err := mapper.KindsFor(input)
	if meta.IsNoMatchError(err) {
		// if no kind match err, refresh and try once more.
		mapper, err = d.Refresh()
		if err != nil {
			return nil, err
		}
		mapping, err = mapper.KindsFor(input)
	}
	return mapping, err
}
