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
	pkgclient "github.com/kubevela/pkg/controller/client"
	"github.com/spf13/pflag"
)

// ClientConfig contains controller client configuration.
// This wraps the external package's client configuration flags.
type ClientConfig struct {
	// Note: The actual configuration is managed by the pkgclient package
	// This is a wrapper to maintain consistency with our config pattern
}

// NewClientConfig creates a new ClientConfig with defaults.
func NewClientConfig() *ClientConfig {
	return &ClientConfig{}
}

// AddFlags registers client configuration flags.
// Delegates to the external package's flag registration.
func (c *ClientConfig) AddFlags(fs *pflag.FlagSet) {
	pkgclient.AddTimeoutControllerClientFlags(fs)
}
