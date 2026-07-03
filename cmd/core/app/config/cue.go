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
	"context"

	"github.com/kubevela/pkg/cue/cuex"
	"github.com/spf13/pflag"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/pkg/cue/upgrade"
)

// CUEConfig contains CUE language configuration.
type CUEConfig struct {
	EnableExternalPackage         bool
	EnableExternalPackageWatch    bool
	EnableCUEVersionCompatibility bool
	CUECompatibilityCacheSize     int
}

// NewCUEConfig creates a new CUEConfig with defaults.
func NewCUEConfig() *CUEConfig {
	return &CUEConfig{
		EnableExternalPackage:         cuex.EnableExternalPackageForDefaultCompiler,
		EnableExternalPackageWatch:    cuex.EnableExternalPackageWatchForDefaultCompiler,
		EnableCUEVersionCompatibility: *upgrade.EnableCUEVersionCompatibility,
		CUECompatibilityCacheSize:     upgrade.CompatibilityCacheSize,
	}
}

// AddFlags registers CUE configuration flags.
func (c *CUEConfig) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&c.EnableExternalPackage,
		"enable-external-package-for-default-compiler",
		c.EnableExternalPackage,
		"Enable loading third-party CUE packages into the default CUE compiler. When enabled, external CUE packages can be imported and used in CUE templates.")
	fs.BoolVar(&c.EnableExternalPackageWatch,
		"enable-external-package-watch-for-default-compiler",
		c.EnableExternalPackageWatch,
		"Enable watching for changes in external CUE packages and automatically reload them when modified. Requires enable-external-package-for-default-compiler to be enabled.")
	fs.BoolVar(&c.EnableCUEVersionCompatibility,
		"enable-cue-version-compatibility",
		c.EnableCUEVersionCompatibility,
		"Automatically rewrite legacy CUE syntax in stored definitions at render time.")
	fs.IntVar(&c.CUECompatibilityCacheSize,
		"cue-compatibility-cache-size",
		c.CUECompatibilityCacheSize,
		"Maximum number of CUE templates to cache after version compatibility rewriting. Set to 0 to disable caching.")
}

// SyncToCUEGlobals syncs the parsed configuration values to CUE package global variables.
// This should be called after flag parsing to ensure the CUE compiler uses the configured values.
//
// NOTE: This method exists for backward compatibility with legacy code that depends on global
// variables in the cuex package. Ideally, the CUE compiler configuration should be injected
// rather than relying on globals.
//
// The flow is: CLI flags -> CUEConfig struct fields -> cuex/upgrade globals (via this method)
// ctx should be the controller's root context so cache eviction goroutines are tied to its lifetime.
func (c *CUEConfig) SyncToCUEGlobals(ctx context.Context) {
	cuex.EnableExternalPackageForDefaultCompiler = c.EnableExternalPackage
	cuex.EnableExternalPackageWatchForDefaultCompiler = c.EnableExternalPackageWatch
	*upgrade.EnableCUEVersionCompatibility = c.EnableCUEVersionCompatibility
	if c.CUECompatibilityCacheSize < 0 {
		klog.Warningf("cue-compatibility-cache-size %d is invalid (must be >= 0); caching disabled", c.CUECompatibilityCacheSize)
		c.CUECompatibilityCacheSize = 0
	}
	upgrade.InitCompatibilityCache(ctx, c.CUECompatibilityCacheSize)
}
