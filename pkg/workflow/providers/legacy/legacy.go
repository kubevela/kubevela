package legacy

import (
	"strings"

	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"

	wflegacy "github.com/kubevela/workflow/pkg/providers/legacy"

	"github.com/oam-dev/kubevela/pkg/workflow/providers/legacy/multicluster"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/legacy/oam"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/legacy/terraform"
)

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
	registerProviders(providers, wflegacy.GetLegacyProviders())

	return providers
}

// GetLegacyTemplate get legacy template
func GetLegacyTemplate() string {
	return strings.Join([]string{
		multicluster.GetTemplate(),
		oam.GetTemplate(),
		terraform.GetTemplate(),
		wflegacy.GetLegacyTemplate(),
	},
		"\n")
}
