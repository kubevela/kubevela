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
	"errors"
	"fmt"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	apiConfig "github.com/oam-dev/kubevela/pkg/apiserver/config"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var kubeClient client.Client
var kubeConfig *rest.Config

// SetKubeClient for test
func SetKubeClient(c client.Client) {
	kubeClient = c
}

func setKubeConfig(conf *rest.Config) (err error) {
	if conf == nil {
		conf, err = config.GetConfig()
		if err != nil {
			return err
		}
	}
	kubeConfig = conf
	kubeConfig.Wrap(auth.NewImpersonatingRoundTripper)
	return nil
}

// SetKubeConfig generate the kube config from the config of apiserver
func SetKubeConfig(c apiConfig.Config) error {
	conf, err := config.GetConfig()
	if err != nil {
		return err
	}
	kubeConfig = conf
	kubeConfig.Burst = c.KubeBurst
	kubeConfig.QPS = float32(c.KubeQPS)
	return setKubeConfig(kubeConfig)
}

// GetKubeClient create and return kube runtime client
func GetKubeClient() (client.Client, error) {
	if kubeClient != nil {
		return kubeClient, nil
	}
	if kubeConfig == nil {
		return nil, fmt.Errorf("please call SetKubeConfig first")
	}
	var err error
	kubeClient, err = multicluster.Initialize(kubeConfig, false)
	if err == nil {
		return kubeClient, nil
	}
	if !errors.Is(err, multicluster.ErrDetectClusterGateway) {
		return nil, err
	}
	// create single cluster client
	kubeClient, err = client.New(kubeConfig, client.Options{Scheme: common.Scheme})
	if err != nil {
		return nil, err
	}
	return kubeClient, nil
}

// GetKubeConfig create/get kube runtime config
func GetKubeConfig() (*rest.Config, error) {
	if kubeConfig == nil {
		return nil, fmt.Errorf("please call SetKubeConfig first")
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
