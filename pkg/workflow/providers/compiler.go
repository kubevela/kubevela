/*
Copyright 2022 The KubeVela Authors.

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

package providers

import (
	"github.com/kubevela/pkg/cue/cuex"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/runtime"
	"github.com/kubevela/pkg/util/singleton"

	"github.com/oam-dev/kubevela/pkg/workflow/providers/legacy"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/legacy/config"
)

const (
	// LegacyProviderName is the name of legacy provider
	LegacyProviderName = "op"
	ConfigProviderName = "config"
)

// Compiler is the workflow default compiler
var Compiler = singleton.NewSingletonE[*cuex.Compiler](func() (*cuex.Compiler, error) {
	return cuex.NewCompilerWithInternalPackages(
		// legacy packages
		runtime.Must(cuexruntime.NewInternalPackage(LegacyProviderName, legacy.GetLegacyTemplate(), legacy.GetLegacyProviders())),
		runtime.Must(cuexruntime.NewInternalPackage(ConfigProviderName, config.GetTemplate(), config.GetProviders())),
	), nil
})
