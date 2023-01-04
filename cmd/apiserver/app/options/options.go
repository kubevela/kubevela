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

package options

import (
	"flag"

	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/pkg/apiserver/config"
	"github.com/oam-dev/kubevela/pkg/features"
)

// ServerRunOptions contains everything necessary to create and run api server
type ServerRunOptions struct {
	GenericServerRunOptions *config.Config
}

// NewServerRunOptions creates a new ServerRunOptions object with default parameters
func NewServerRunOptions() *ServerRunOptions {
	s := &ServerRunOptions{
		GenericServerRunOptions: config.NewConfig(),
	}
	return s
}

// Flags returns the complete NamedFlagSets
func (s *ServerRunOptions) Flags() (fss cliflag.NamedFlagSets) {
	fs := fss.FlagSet("generic")
	s.GenericServerRunOptions.AddFlags(fs, s.GenericServerRunOptions)
	features.APIServerMutableFeatureGate.AddFlag(fss.FlagSet("featuregate"))
	local := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(local)
	fs.AddGoFlagSet(local)
	return fss
}
