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
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cmdutil "github.com/oam-dev/kubevela/pkg/cmd/util"
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

type defaultFactory struct {
	ClientGetter
	ConfigGetter
}

// Client return the client for command line use, interrupt if error encountered
func (f *defaultFactory) Client() client.Client {
	cli, err := f.ClientGetter()
	cmdutil.CheckErr(err)
	return cli
}

// Config return the kubeConfig for command line use
func (f *defaultFactory) Config() *rest.Config {
	cfg, err := f.ConfigGetter()
	cmdutil.CheckErr(err)
	return cfg
}

// NewDefaultFactory create a factory based on client getter function
func NewDefaultFactory(clientGetter ClientGetter, configGetter ConfigGetter) Factory {
	return &defaultFactory{ClientGetter: clientGetter, ConfigGetter: configGetter}
}
