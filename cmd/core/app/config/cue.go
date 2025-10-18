/*
Copyright 2025 The KubeVela Authors.

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

package config

import (
	"github.com/kubevela/pkg/cue/cuex"
	"github.com/spf13/pflag"
)

// CUEConfig contains CUE language configuration.
type CUEConfig struct {
	EnableExternalPackage      bool
	EnableExternalPackageWatch bool
}

// NewCUEConfig creates a new CUEConfig with defaults.
func NewCUEConfig() *CUEConfig {
	return &CUEConfig{
		EnableExternalPackage:      cuex.EnableExternalPackageForDefaultCompiler,
		EnableExternalPackageWatch: cuex.EnableExternalPackageWatchForDefaultCompiler,
	}
}

// AddFlags registers CUE configuration flags.
func (c *CUEConfig) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&c.EnableExternalPackage,
		"enable-external-package-for-default-compiler",
		c.EnableExternalPackage,
		"Enable external package for default compiler")
	fs.BoolVar(&c.EnableExternalPackageWatch,
		"enable-external-package-watch-for-default-compiler",
		c.EnableExternalPackageWatch,
		"Enable external package watch for default compiler")
}

// SyncToCUEGlobals syncs the parsed configuration values to CUE package global variables.
// This should be called after flag parsing to ensure the CUE compiler uses the configured values.
func (c *CUEConfig) SyncToCUEGlobals() {
	cuex.EnableExternalPackageForDefaultCompiler = c.EnableExternalPackage
	cuex.EnableExternalPackageWatchForDefaultCompiler = c.EnableExternalPackageWatch
}
