/*
Copyright 2022 The KubeVela Authors.

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

package cmd

import (
	"sync"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/flowcontrol"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/pkg/multicluster"

	cmdutil "github.com/oam-dev/kubevela/pkg/cmd/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// Factory client factory for running command
type Factory interface {
	Client() client.Client
	Config() *rest.Config
}

// ClientGetter function for getting client
type ClientGetter func() (client.Client, error)

// ConfigGetter function for getting config
type ConfigGetter func() (*rest.Config, error)

type delegateFactory struct {
	ClientGetter
	ConfigGetter
}

// Client return the client for command line use, interrupt if error encountered
func (f *delegateFactory) Client() client.Client {
	cli, err := f.ClientGetter()
	cmdutil.CheckErr(err)
	return cli
}

// Config return the kubeConfig for command line use
func (f *delegateFactory) Config() *rest.Config {
	cfg, err := f.ConfigGetter()
	cmdutil.CheckErr(err)
	return cfg
}

// NewDelegateFactory create a factory based on getter function
func NewDelegateFactory(clientGetter ClientGetter, configGetter ConfigGetter) Factory {
	return &delegateFactory{ClientGetter: clientGetter, ConfigGetter: configGetter}
}

var (
	// DefaultRateLimiter default rate limiter for cmd client
	DefaultRateLimiter = flowcontrol.NewTokenBucketRateLimiter(100, 200)
)

type defaultFactory struct {
	sync.Mutex
	cfg *rest.Config
	cli client.Client
}

// Client return the client for command line use, interrupt if error encountered
func (f *defaultFactory) Client() client.Client {
	f.Lock()
	defer f.Unlock()
	if f.cli == nil {
		var err error
		f.cli, err = client.New(f.cfg, client.Options{Scheme: common.Scheme})
		cmdutil.CheckErr(err)
	}
	return f.cli
}

// Config return the kubeConfig for command line use
func (f *defaultFactory) Config() *rest.Config {
	return f.cfg
}

// NewDefaultFactory create a factory based on client getter function
func NewDefaultFactory(cfg *rest.Config) Factory {
	copiedCfg := *cfg
	copiedCfg.RateLimiter = DefaultRateLimiter
	copiedCfg.Wrap(multicluster.NewTransportWrapper())
	return &defaultFactory{cfg: &copiedCfg}
}

type deferredFactory struct {
	sync.Mutex
	Factory
	ConfigGetter
}

// NewDeferredFactory create a factory that will only get KubeConfig until it is needed for the first time
func NewDeferredFactory(getter ConfigGetter) Factory {
	return &deferredFactory{ConfigGetter: getter}
}

func (f *deferredFactory) init() {
	cfg, err := f.ConfigGetter()
	cmdutil.CheckErr(err)
	f.Factory = NewDefaultFactory(cfg)
}

// Config return the kubeConfig
func (f *deferredFactory) Config() *rest.Config {
	f.Lock()
	defer f.Unlock()
	if f.Factory == nil {
		f.init()
	}
	return f.Factory.Config()
}

// Client return the kubeClient
func (f *deferredFactory) Client() client.Client {
	f.Lock()
	defer f.Unlock()
	if f.Factory == nil {
		f.init()
	}
	return f.Factory.Client()
}

type testFactory struct {
	cfg *rest.Config
	cli client.Client
}

// NewTestFactory new a factory for the testing
func NewTestFactory(cfg *rest.Config,
	cli client.Client) Factory {
	return &testFactory{cli: cli, cfg: cfg}
}

func (t *testFactory) Client() client.Client {
	return t.cli
}
func (t *testFactory) Config() *rest.Config {
	return t.cfg
}
