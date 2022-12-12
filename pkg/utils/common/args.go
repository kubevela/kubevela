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

	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/flowcontrol"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kubevela/workflow/pkg/cue/packages"

	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
)

// Args is args for controller-runtime client
type Args struct {
	config    *rest.Config
	rawConfig *api.Config
	Schema    *runtime.Scheme
	client    client.Client
	dm        discoverymapper.DiscoveryMapper
	pd        *packages.PackageDiscover
	dc        *discovery.DiscoveryClient
}

// SetConfig insert kubeconfig into Args
func (a *Args) SetConfig(c *rest.Config) error {
	if c != nil {
		a.config = c
		return nil
	}
	restConf, err := config.GetConfig()
	if err != nil {
		return err
	}
	restConf.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(100, 200)
	a.config = restConf
	return nil
}

// GetConfig get config, if not exist, will create
func (a *Args) GetConfig() (*rest.Config, error) {
	if a.config != nil {
		return a.config, nil
	}
	if err := a.SetConfig(nil); err != nil {
		return nil, err
	}
	return a.config, nil
}

// GetRawConfig get raw kubeconfig, if not exist, will create
func (a *Args) GetRawConfig() (*api.Config, error) {
	if a.rawConfig != nil {
		return a.rawConfig, nil
	}
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	raw, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules, nil).RawConfig()
	if err != nil {
		return nil, err
	}
	return &raw, nil
}

// GetNamespaceFromConfig will get namespace from kube config
func (a *Args) GetNamespaceFromConfig() string {
	conf, err := a.GetRawConfig()
	if err != nil || conf == nil || conf.Contexts == nil {
		return ""
	}
	ctx, ok := conf.Contexts[conf.CurrentContext]
	if !ok {
		return ""
	}
	return ctx.Namespace
}

// SetClient set custom client
func (a *Args) SetClient(c client.Client) {
	a.client = c
}

// GetClient get client if exist
func (a *Args) GetClient() (client.Client, error) {
	if a.client != nil {
		return a.client, nil
	}
	if a.config == nil {
		if err := a.SetConfig(nil); err != nil {
			return nil, err
		}
	}
	newClient, err := pkgmulticluster.NewClient(a.config,
		pkgmulticluster.ClientOptions{
			Options: client.Options{Scheme: a.Schema}})
	if err != nil {
		return nil, err
	}
	a.client = newClient
	return a.client, nil
}

// GetFakeClient returns a fake client with the definition objects preloaded
func (a *Args) GetFakeClient(defs []oam.Object) (client.Client, error) {
	if a.client != nil {
		return a.client, nil
	}
	if a.config == nil {
		if err := a.SetConfig(nil); err != nil {
			return nil, err
		}
	}
	objs := make([]client.Object, 0, len(defs))
	for _, def := range defs {
		if unstructDef, ok := def.(*unstructured.Unstructured); ok {
			objs = append(objs, unstructDef)
		}
	}
	return fake.NewClientBuilder().WithObjects(objs...).WithScheme(a.Schema).Build(), nil
}

// GetDiscoveryMapper get discoveryMapper client if exist, create if not exist.
func (a *Args) GetDiscoveryMapper() (discoverymapper.DiscoveryMapper, error) {
	if a.config == nil {
		if err := a.SetConfig(nil); err != nil {
			return nil, err
		}
	}
	if a.dm != nil {
		return a.dm, nil
	}
	dm, err := discoverymapper.New(a.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create CRD discovery client %w", err)
	}
	a.dm = dm
	return dm, nil
}

// GetPackageDiscover get PackageDiscover client if exist, create if not exist.
func (a *Args) GetPackageDiscover() (*packages.PackageDiscover, error) {
	if a.config == nil {
		if err := a.SetConfig(nil); err != nil {
			return nil, err
		}
	}
	if a.pd != nil {
		return a.pd, nil
	}
	pd, err := packages.NewPackageDiscover(a.config)
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
	cfg, err := a.GetConfig()
	if err != nil {
		return nil, err
	}

	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return dc, nil
}
