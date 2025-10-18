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
	fs.StringVar(&oam.SystemDefinitionNamespace,
		"system-definition-namespace",
		"vela-system",
		"Define the namespace of the system-level definition")

	// Keep our config in sync
	c.SystemDefinitionNamespace = oam.SystemDefinitionNamespace
}
