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
	"github.com/kubevela/pkg/multicluster"
	"github.com/kubevela/workflow/pkg/cue/packages"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
)

var (
	singletonConfig          *rest.Config
	singletonDynamicClient   client.Client
	singletonClient          *kubernetes.Clientset
	singletonDiscoveryMapper discoverymapper.DiscoveryMapper
	singletonPackageDiscover *packages.PackageDiscover
	singletonRawConfig       *api.Config
	singletonDiscoveryClient *discovery.DiscoveryClient
)

var (
	err         error
	rateLimiter = flowcontrol.NewTokenBucketRateLimiter(100, 200)
)

// Config returns a Kubernetes config
func Config() *rest.Config {
	if singletonConfig != nil {
		return singletonConfig
	}
	err = loadConfig()
	if err != nil {
		panic(err)
	}
	return singletonConfig
}

// ConfigOrNil -
func ConfigOrNil() *rest.Config {
	if singletonConfig != nil {
		return singletonConfig
	}
	err = loadConfig()
	if err != nil {
		return nil
	}
	return singletonConfig
}

// DynamicClient will return Kubernetes client from controller-runtime package
func DynamicClient() client.Client {
	if singletonDynamicClient != nil {
		return singletonDynamicClient
	}
	err = loadDynamicClient()
	if err != nil {
		panic(err)
	}
	return singletonDynamicClient
}

// DynamicClientOrNil -
func DynamicClientOrNil() client.Client {
	if singletonDynamicClient != nil {
		return singletonDynamicClient
	}
	err = loadDynamicClient()
	if err != nil {
		return nil
	}
	return singletonDynamicClient
}

// GetFakeClient will return a fake client contains some pre-defined objects
func GetFakeClient(defs []oam.Object) client.Client {
	objs := make([]client.Object, 0, len(defs))
	for _, def := range defs {
		if unstructDef, ok := def.(*unstructured.Unstructured); ok {
			objs = append(objs, unstructDef)
		}
	}
	return fake.NewClientBuilder().WithObjects(objs...).WithScheme(Scheme).Build()
}

// Client returns a Kubernetes client from client-go package
func Client() *kubernetes.Clientset {
	if singletonClient != nil {
		return singletonClient
	}
	err = loadClient()
	if err != nil {
		panic(err)
	}
	return singletonClient
}

// DiscoveryMapper returns a discovery mapper
func DiscoveryMapper() discoverymapper.DiscoveryMapper {
	if singletonDiscoveryMapper != nil {
		return singletonDiscoveryMapper
	}
	err = loadDiscoveryMapper()
	if err != nil {
		panic(err)
	}
	return singletonDiscoveryMapper
}

// PackageDiscover returns a package discover
func PackageDiscover() *packages.PackageDiscover {
	if singletonPackageDiscover != nil {
		return singletonPackageDiscover
	}
	err = loadPackageDiscover()
	if err != nil {
		panic(err)
	}
	return singletonPackageDiscover
}

// DiscoveryClient returns a discovery client
func DiscoveryClient() *discovery.DiscoveryClient {
	if singletonDiscoveryClient != nil {
		return singletonDiscoveryClient
	}
	err = loadDynamicClient()
	if err != nil {
		panic(err)
	}
	return singletonDiscoveryClient
}

// PackageDiscoverOrNil returns a package discover or nil if failed to create
func PackageDiscoverOrNil() *packages.PackageDiscover {
	if singletonPackageDiscover != nil {
		return singletonPackageDiscover
	}
	err = loadPackageDiscover()
	if err != nil {
		return nil
	}
	return singletonPackageDiscover
}

// DiscoveryMapperOrNil -
func DiscoveryMapperOrNil() discoverymapper.DiscoveryMapper {
	if singletonDiscoveryMapper != nil {
		return singletonDiscoveryMapper
	}
	err = loadDiscoveryMapper()
	if err != nil {
		return nil
	}
	return singletonDiscoveryMapper
}

// RawConfigOrNil returns a raw config
func RawConfigOrNil() *api.Config {
	if singletonRawConfig != nil {
		return singletonRawConfig
	}
	err = loadRawConfig()
	if err != nil {
		return nil
	}
	return singletonRawConfig
}

// SetConfig will set the given config to singleton config
func SetConfig(c *rest.Config) {
	if c != nil {
		singletonConfig = c
		reloadClient()
	}
}

// SetClient will set the given client to singleton client
func SetClient(c client.Client) {
	singletonDynamicClient = c
}

func reloadClient() {
	_ = loadDynamicClient()
	_ = loadClient()
	_ = loadDiscoveryMapper()
	_ = loadPackageDiscover()
	_ = loadDiscoveryClient()
}

func loadConfig() error {
	singletonConfig, err = config.GetConfig()
	singletonConfig.Wrap(multicluster.NewTransportWrapper())
	singletonConfig.RateLimiter = rateLimiter
	if err != nil {
		klog.V(3).InfoS(err.Error(), "Fail to get Kubernetes config")
		return err
	}
	return nil
}

func loadClient() error {
	singletonClient, err = kubernetes.NewForConfig(Config())
	if err != nil {
		klog.V(3).InfoS(err.Error(), "Fail to create Kubernetes client")
		return err
	}
	return nil
}

func loadDynamicClient() error {
	singletonDynamicClient, err = client.New(Config(), client.Options{Scheme: Scheme})
	if err != nil {
		klog.V(3).InfoS(err.Error(), "Fail to create Kubernetes dynamic client")
		return err
	}
	return nil
}

func loadDiscoveryMapper() error {
	singletonDiscoveryMapper, err = discoverymapper.New(Config())
	if err != nil {
		klog.V(3).InfoS(err.Error(), "Fail to create discovery mapper")
		return err
	}
	return nil
}

func loadPackageDiscover() error {
	singletonPackageDiscover, err = packages.NewPackageDiscover(Config())
	if err != nil {
		klog.V(3).InfoS(err.Error(), "Fail to create package discover")
		return err
	}
	return nil
}

func loadRawConfig() error {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	raw, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules, nil).RawConfig()
	if err != nil {
		klog.V(3).InfoS(err.Error(), "Fail to create raw config")
		return err
	}
	singletonRawConfig = &raw
	return nil
}

func loadDiscoveryClient() error {
	singletonDiscoveryClient, err = discovery.NewDiscoveryClientForConfig(Config())
	if err != nil {
		klog.V(3).InfoS(err.Error(), "Fail to create discovery client")
		return err
	}
	return nil
}
