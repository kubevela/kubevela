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

	"github.com/oam-dev/kubevela/pkg/resourcekeeper"
)

// ResourceConfig contains resource management configuration.
type ResourceConfig struct {
	MaxDispatchConcurrent int
}

// NewResourceConfig creates a new ResourceConfig with defaults.
func NewResourceConfig() *ResourceConfig {
	return &ResourceConfig{
		MaxDispatchConcurrent: 10,
	}
}

// AddFlags registers resource configuration flags.
func (c *ResourceConfig) AddFlags(fs *pflag.FlagSet) {
	fs.IntVar(&c.MaxDispatchConcurrent,
		"max-dispatch-concurrent",
		c.MaxDispatchConcurrent,
		"Set the max dispatch concurrent number, default is 10")
}

// SyncToResourceGlobals syncs the parsed configuration values to resource package global variables.
// This should be called after flag parsing to ensure the resource keeper uses the configured values.
//
// NOTE: This method exists for backward compatibility with legacy code that depends on global
// variables in the resourcekeeper package. The long-term goal should be to refactor to use
// dependency injection rather than globals.
//
// The flow is: CLI flags -> ResourceConfig struct fields -> resourcekeeper globals (via this method)
func (c *ResourceConfig) SyncToResourceGlobals() {
	resourcekeeper.MaxDispatchConcurrent = c.MaxDispatchConcurrent
}
