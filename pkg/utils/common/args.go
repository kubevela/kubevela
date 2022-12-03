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

package common

import (
	"fmt"

	"k8s.io/client-go/discovery"

	"github.com/kubevela/workflow/pkg/cue/packages"

	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
)

// Args is args for controller-runtime client
type Args struct {
	dm discoverymapper.DiscoveryMapper
	pd *packages.PackageDiscover
	dc *discovery.DiscoveryClient
}

// GetDiscoveryMapper get discoveryMapper client if exist, create if not exist.
func (a *Args) GetDiscoveryMapper() (discoverymapper.DiscoveryMapper, error) {
	if a.dm != nil {
		return a.dm, nil
	}
	dm, err := discoverymapper.New(Config())
	if err != nil {
		return nil, fmt.Errorf("failed to create CRD discovery client %w", err)
	}
	a.dm = dm
	return dm, nil
}

// GetPackageDiscover get PackageDiscover client if exist, create if not exist.
func (a *Args) GetPackageDiscover() (*packages.PackageDiscover, error) {
	if a.pd != nil {
		return a.pd, nil
	}
	pd, err := packages.NewPackageDiscover(Config())
	if err != nil {
		return nil, fmt.Errorf("failed to create CRD discovery for CUE package client %w", err)
	}
	a.pd = pd
	return pd, nil
}

// GetDiscoveryClient return a discovery client from cli args
func (a *Args) GetDiscoveryClient() (*discovery.DiscoveryClient, error) {
	if a.dc != nil {
		return a.dc, nil
	}
	dc, err := discovery.NewDiscoveryClientForConfig(Config())
	if err != nil {
		return nil, err
	}
	return dc, nil
}
