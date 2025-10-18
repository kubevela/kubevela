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
	// These flags directly modify the cuex package variables
	fs.BoolVar(&cuex.EnableExternalPackageForDefaultCompiler,
		"enable-external-package-for-default-compiler",
		cuex.EnableExternalPackageForDefaultCompiler,
		"Enable external package for default compiler")
	fs.BoolVar(&cuex.EnableExternalPackageWatchForDefaultCompiler,
		"enable-external-package-watch-for-default-compiler",
		cuex.EnableExternalPackageWatchForDefaultCompiler,
		"Enable external package watch for default compiler")

	// Also bind to our config struct
	c.EnableExternalPackage = cuex.EnableExternalPackageForDefaultCompiler
	c.EnableExternalPackageWatch = cuex.EnableExternalPackageWatchForDefaultCompiler
}