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
	"strconv"

	utillog "github.com/kubevela/pkg/util/log"
	"github.com/spf13/pflag"

	commonconfig "github.com/oam-dev/kubevela/pkg/controller/common"
)

// KLogConfig contains klog configuration.
// This wraps the Kubernetes logging configuration.
type KLogConfig struct {
	// Reference to observability config for log settings
	observability *ObservabilityConfig
}

// NewKLogConfig creates a new KLogConfig.
func NewKLogConfig(observability *ObservabilityConfig) *KLogConfig {
	return &KLogConfig{
		observability: observability,
	}
}

// AddFlags registers klog configuration flags.
func (c *KLogConfig) AddFlags(fs *pflag.FlagSet) {
	// Add base klog flags
	utillog.AddFlags(fs)
}

// ConfigureKLog applies klog configuration based on parsed observability settings.
// This should be called after flag parsing to configure klog verbosity and log file settings.
func (c *KLogConfig) ConfigureKLog(fs *pflag.FlagSet) {
	if c.observability != nil {
		if c.observability.LogDebug {
			_ = fs.Set("v", strconv.Itoa(int(commonconfig.LogDebug)))
		}

		if c.observability.LogFilePath != "" {
			_ = fs.Set("logtostderr", "false")
			_ = fs.Set("log_file", c.observability.LogFilePath)
			_ = fs.Set("log_file_max_size", strconv.FormatUint(c.observability.LogFileMaxSize, 10))
		}
	}
}
