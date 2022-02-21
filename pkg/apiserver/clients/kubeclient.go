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

package clients

import (
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
)

var kubeClient client.Client
var kubeConfig *rest.Config

// SetKubeClient for test
func SetKubeClient(c client.Client) {
	kubeClient = c
}

// GetKubeClient create and return kube runtime client
func GetKubeClient() (client.Client, error) {
	if kubeClient != nil {
		return kubeClient, nil
	}
	var err error
	kubeClient, kubeConfig, err = multicluster.GetMulticlusterKubernetesClient()
	if err != nil {
		return nil, err
	}
	return kubeClient, nil
}

// GetKubeConfig create/get kube runtime config
func GetKubeConfig() (*rest.Config, error) {
	var err error
	if kubeConfig == nil {
		kubeConfig, err = config.GetConfig()
		return kubeConfig, err
	}
	return kubeConfig, nil
}

// GetDiscoverMapper get discover mapper
func GetDiscoverMapper() (discoverymapper.DiscoveryMapper, error) {
	conf, err := GetKubeConfig()
	if err != nil {
		return nil, err
	}
	dm, err := discoverymapper.New(conf)
	if err != nil {
		return nil, err
	}
	return dm, nil
}

// GetPackageDiscover get package discover
func GetPackageDiscover() (*packages.PackageDiscover, error) {
	conf, err := GetKubeConfig()
	if err != nil {
		return nil, err
	}
	pd, err := packages.NewPackageDiscover(conf)
	if err != nil {
		if !packages.IsCUEParseErr(err) {
			return nil, err
		}
	}
	return pd, nil
}

// GetDiscoveryClient return a discovery client
func GetDiscoveryClient() (*discovery.DiscoveryClient, error) {
	conf, err := GetKubeConfig()
	if err != nil {
		return nil, err
	}
	dc, err := discovery.NewDiscoveryClientForConfig(conf)
	if err != nil {
		return nil, err
	}
	return dc, nil
}
