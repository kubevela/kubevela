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
)

// ObservabilityConfig contains metrics and logging configuration.
type ObservabilityConfig struct {
	MetricsAddr    string
	LogFilePath    string
	LogFileMaxSize uint64
	LogDebug       bool
	DevLogs        bool
}

// NewObservabilityConfig creates a new ObservabilityConfig with defaults.
func NewObservabilityConfig() *ObservabilityConfig {
	return &ObservabilityConfig{
		MetricsAddr:    ":8080",
		LogFilePath:    "",
		LogFileMaxSize: 1024,
		LogDebug:       false,
		DevLogs:        false,
	}
}

// AddFlags registers observability configuration flags.
func (c *ObservabilityConfig) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.MetricsAddr, "metrics-addr", c.MetricsAddr,
		"The address the metric endpoint binds to.")
	fs.StringVar(&c.LogFilePath, "log-file-path", c.LogFilePath,
		"The file to write logs to.")
	fs.Uint64Var(&c.LogFileMaxSize, "log-file-max-size", c.LogFileMaxSize,
		"Defines the maximum size a log file can grow to, Unit is megabytes.")
	fs.BoolVar(&c.LogDebug, "log-debug", c.LogDebug,
		"Enable debug logs for development purpose")
	fs.BoolVar(&c.DevLogs, "dev-logs", c.DevLogs,
		"Enable ANSI color formatting for console logs (ignored when log-file-path is set)")
}
