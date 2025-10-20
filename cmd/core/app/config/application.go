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
	"time"

	"github.com/spf13/pflag"

	commonconfig "github.com/oam-dev/kubevela/pkg/controller/common"
)

// ApplicationConfig contains application-specific configuration.
type ApplicationConfig struct {
	ReSyncPeriod time.Duration
}

// NewApplicationConfig creates a new ApplicationConfig with defaults.
func NewApplicationConfig() *ApplicationConfig {
	return &ApplicationConfig{
		ReSyncPeriod: commonconfig.ApplicationReSyncPeriod,
	}
}

// AddFlags registers application configuration flags.
func (c *ApplicationConfig) AddFlags(fs *pflag.FlagSet) {
	fs.DurationVar(&c.ReSyncPeriod,
		"application-re-sync-period",
		c.ReSyncPeriod,
		"Re-sync period for application to re-sync, also known as the state-keep interval.")
}

// SyncToApplicationGlobals syncs the parsed configuration values to application package global variables.
// This should be called after flag parsing to ensure the application controller uses the configured values.
//
// NOTE: This method exists for backward compatibility with legacy code that depends on global
// variables in the commonconfig package. Ideally, configuration should be injected rather than using globals.
//
// The flow is: CLI flags -> ApplicationConfig struct fields -> commonconfig globals (via this method)
func (c *ApplicationConfig) SyncToApplicationGlobals() {
	commonconfig.ApplicationReSyncPeriod = c.ReSyncPeriod
}
