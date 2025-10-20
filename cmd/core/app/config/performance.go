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
	"github.com/spf13/pflag"

	standardcontroller "github.com/oam-dev/kubevela/pkg/controller"
	commonconfig "github.com/oam-dev/kubevela/pkg/controller/common"
)

// PerformanceConfig contains performance and optimization configuration.
type PerformanceConfig struct {
	PerfEnabled bool
}

// NewPerformanceConfig creates a new PerformanceConfig with defaults.
func NewPerformanceConfig() *PerformanceConfig {
	return &PerformanceConfig{
		PerfEnabled: commonconfig.PerfEnabled,
	}
}

// AddFlags registers performance configuration flags.
func (c *PerformanceConfig) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&c.PerfEnabled,
		"perf-enabled",
		c.PerfEnabled,
		"Enable performance logging for controllers, disabled by default.")

	// Add optimization flags from the standard controller
	standardcontroller.AddOptimizeFlags(fs)
}

// SyncToPerformanceGlobals syncs the parsed configuration values to performance package global variables.
// This should be called after flag parsing to ensure the performance monitoring uses the configured values.
//
// NOTE: This method exists for backward compatibility with legacy code that depends on global
// variables in the commonconfig package. Ideally, configuration should be injected rather than using globals.
//
// The flow is: CLI flags -> PerformanceConfig struct fields -> commonconfig globals (via this method)
func (c *PerformanceConfig) SyncToPerformanceGlobals() {
	commonconfig.PerfEnabled = c.PerfEnabled
}
