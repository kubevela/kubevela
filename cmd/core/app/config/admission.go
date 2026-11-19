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

	standardcontroller "github.com/oam-dev/kubevela/pkg/controller"
)

// AdmissionConfig contains admission control configuration.
type AdmissionConfig struct {
	// Fields will be populated based on what standardcontroller.AddAdmissionFlags sets
}

// NewAdmissionConfig creates a new AdmissionConfig with defaults.
func NewAdmissionConfig() *AdmissionConfig {
	return &AdmissionConfig{}
}

// AddFlags registers admission configuration flags.
func (c *AdmissionConfig) AddFlags(fs *pflag.FlagSet) {
	standardcontroller.AddAdmissionFlags(fs)
}
