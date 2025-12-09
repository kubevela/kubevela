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

	"github.com/oam-dev/kubevela/pkg/oam"
)

// OAMConfig contains OAM-specific configuration.
type OAMConfig struct {
	SystemDefinitionNamespace string
}

// NewOAMConfig creates a new OAMConfig with defaults.
func NewOAMConfig() *OAMConfig {
	return &OAMConfig{
		SystemDefinitionNamespace: "vela-system",
	}
}

// AddFlags registers OAM configuration flags.
func (c *OAMConfig) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.SystemDefinitionNamespace,
		"system-definition-namespace",
		c.SystemDefinitionNamespace,
		"Define the namespace of the system-level definition")
}

// SyncToOAMGlobals syncs the parsed configuration values to OAM package global variables.
// This should be called after flag parsing to ensure the OAM runtime uses the configured values.
//
// NOTE: This method exists for backward compatibility with legacy code that depends on global
// variables in the oam package. Ideally, configuration should be injected rather than using globals.
//
// The flow is: CLI flags -> OAMConfig struct fields -> oam globals (via this method)
func (c *OAMConfig) SyncToOAMGlobals() {
	oam.SystemDefinitionNamespace = c.SystemDefinitionNamespace
}
