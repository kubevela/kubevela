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

package discoverymapper

import (
	"sync"

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
	ResourcesFor(input schema.GroupVersionKind) (schema.GroupVersionResource, error)
}

var _ DiscoveryMapper = &DefaultDiscoveryMapper{}

// DefaultDiscoveryMapper is a K8s resource mapper for discovery, it will cache the result
type DefaultDiscoveryMapper struct {
	dc     *discovery.DiscoveryClient
	mapper meta.RESTMapper
	mutex  sync.RWMutex
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
	d.mutex.RLock()
	mapper := d.mapper
	d.mutex.RUnlock()

	if mapper == nil {
		return d.Refresh()
	}
	return mapper, nil
}

// Refresh will re-create the mapper by getting the new resource from K8s API by using discovery client
func (d *DefaultDiscoveryMapper) Refresh() (meta.RESTMapper, error) {
	gr, err := restmapper.GetAPIGroupResources(d.dc)
	if err != nil {
		return nil, err
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
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

// ResourcesFor will get a resource from GroupVersionKind
func (d *DefaultDiscoveryMapper) ResourcesFor(input schema.GroupVersionKind) (schema.GroupVersionResource, error) {
	var gvr schema.GroupVersionResource
	mapping, err := d.RESTMapping(input.GroupKind(), input.Version)
	if err != nil {
		return gvr, err
	}
	gvr = mapping.Resource
	return gvr, nil
}
