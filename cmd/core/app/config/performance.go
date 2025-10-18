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
	fs.BoolVar(&commonconfig.PerfEnabled,
		"perf-enabled",
		commonconfig.PerfEnabled,
		"Enable performance logging for controllers, disabled by default.")

	// Add optimization flags from the standard controller
	standardcontroller.AddOptimizeFlags(fs)

	// Keep our config in sync
	c.PerfEnabled = commonconfig.PerfEnabled
}