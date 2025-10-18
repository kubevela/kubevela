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
	fs.DurationVar(&commonconfig.ApplicationReSyncPeriod,
		"application-re-sync-period",
		commonconfig.ApplicationReSyncPeriod,
		"Re-sync period for application to re-sync, also known as the state-keep interval.")

	// Keep our config in sync
	c.ReSyncPeriod = commonconfig.ApplicationReSyncPeriod
}
