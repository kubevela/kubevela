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
	utilfeature "k8s.io/apiserver/pkg/util/feature"
)

// FeatureConfig contains feature gate configuration.
// This wraps the Kubernetes feature gate system.
type FeatureConfig struct {
	// Note: The actual configuration is managed by the utilfeature package
	// This is a wrapper to maintain consistency with our config pattern
}

// NewFeatureConfig creates a new FeatureConfig with defaults.
func NewFeatureConfig() *FeatureConfig {
	return &FeatureConfig{}
}

// AddFlags registers feature gate configuration flags.
// Delegates to the Kubernetes feature gate system.
func (c *FeatureConfig) AddFlags(fs *pflag.FlagSet) {
	utilfeature.DefaultMutableFeatureGate.AddFlag(fs)
}
