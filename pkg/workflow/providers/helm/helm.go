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

package helm

import (
	"github.com/kubevela/pkg/cue/cuex"
	"github.com/kubevela/pkg/cue/cuex/providers"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"

	helmProvider "github.com/oam-dev/kubevela/pkg/cue/cuex/providers/helm"
)

const (
	// ProviderName is provider name for helm
	ProviderName = "helm"
)

func init() {
	// Register the helm package into the upstream cuex.DefaultCompiler so that
	// any code path using it (e.g. the webhook's ValidateCuexTemplate) can
	// resolve "vela/helm" imports without needing providers.DefaultCompiler.
	cuex.DefaultCompiler.Get().LoadInternalPackages(helmProvider.Package)
}

// GetTemplate returns the cue template.
func GetTemplate() string {
	return helmProvider.Template
}

// GetProviders returns the cue providers.
func GetProviders() map[string]cuexruntime.ProviderFn {
	return map[string]cuexruntime.ProviderFn{
		"render":    cuexruntime.GenericProviderFn[providers.Params[helmProvider.RenderParams], providers.Returns[helmProvider.RenderReturns]](helmProvider.Render),
		"uninstall": cuexruntime.GenericProviderFn[providers.Params[helmProvider.UninstallParams], providers.Returns[helmProvider.UninstallReturns]](helmProvider.Uninstall),
	}
}
