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
	"github.com/kubevela/pkg/controller/sharding"
	"github.com/spf13/pflag"
)

// ShardingConfig contains controller sharding configuration.
// This wraps the external package's sharding configuration flags.
type ShardingConfig struct {
	// Note: The actual configuration is managed by the sharding package
	// This is a wrapper to maintain consistency with our config pattern
}

// NewShardingConfig creates a new ShardingConfig with defaults.
func NewShardingConfig() *ShardingConfig {
	return &ShardingConfig{}
}

// AddFlags registers sharding configuration flags.
// Delegates to the external package's flag registration.
func (c *ShardingConfig) AddFlags(fs *pflag.FlagSet) {
	sharding.AddFlags(fs)
}
