/*
Copyright 2024 The KubeVela Authors.

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

package legacy

import (
	"strings"

	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"

	wflegacy "github.com/kubevela/workflow/pkg/providers/legacy"

	"github.com/oam-dev/kubevela/pkg/workflow/providers/legacy/config"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/legacy/multicluster"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/legacy/oam"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/legacy/terraform"
)

// nolint:unparam
func registerProviders(providers map[string]cuexruntime.ProviderFn, new map[string]cuexruntime.ProviderFn) map[string]cuexruntime.ProviderFn {
	for k, v := range new {
		providers[k] = v
	}
	return providers
}

// GetLegacyProviders get legacy providers
func GetLegacyProviders() map[string]cuexruntime.ProviderFn {
	providers := make(map[string]cuexruntime.ProviderFn, 0)
	registerProviders(providers, multicluster.GetProviders())
	registerProviders(providers, oam.GetProviders())
	registerProviders(providers, terraform.GetProviders())
	registerProviders(providers, config.GetProviders())
	registerProviders(providers, wflegacy.GetLegacyProviders())

	return providers
}

// GetLegacyTemplate get legacy template
func GetLegacyTemplate() string {
	return strings.Join([]string{
		multicluster.GetTemplate(),
		oam.GetTemplate(),
		terraform.GetTemplate(),
		config.GetTemplate(),
		wflegacy.GetLegacyTemplate(),
	},
		"\n")
}
