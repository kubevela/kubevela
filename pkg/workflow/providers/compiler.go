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
	"context"

	"github.com/kubevela/pkg/cue/cuex"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/runtime"
	"github.com/kubevela/pkg/util/singleton"
	"k8s.io/klog/v2"

	"github.com/kubevela/workflow/pkg/providers/builtin"
	"github.com/kubevela/workflow/pkg/providers/email"
	"github.com/kubevela/workflow/pkg/providers/http"
	"github.com/kubevela/workflow/pkg/providers/kube"
	"github.com/kubevela/workflow/pkg/providers/metrics"
	"github.com/kubevela/workflow/pkg/providers/time"
	"github.com/kubevela/workflow/pkg/providers/util"

	"github.com/oam-dev/kubevela/pkg/workflow/providers/config"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/helm"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/legacy"
	legacyquery "github.com/oam-dev/kubevela/pkg/workflow/providers/legacy/query"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/multicluster"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/oam"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/query"
	"github.com/oam-dev/kubevela/pkg/workflow/providers/terraform"
)

const (
	// LegacyProviderName is the name of legacy provider
	LegacyProviderName = "op"
	// QLProviderName is the name of ql provider
	QLProviderName = "ql"
)

// compiler is the workflow default compiler
var compiler = singleton.NewSingletonE[*cuex.Compiler](func() (*cuex.Compiler, error) {
	return cuex.NewCompilerWithInternalPackages(
		// legacy packages
		runtime.Must(cuexruntime.NewInternalPackage(LegacyProviderName, legacy.GetLegacyTemplate(), legacy.GetLegacyProviders())),
		runtime.Must(cuexruntime.NewInternalPackage(QLProviderName, legacyquery.GetTemplate(), legacyquery.GetProviders())),

		// workflow internal packages
		runtime.Must(cuexruntime.NewInternalPackage("email", email.GetTemplate(), email.GetProviders())),
		runtime.Must(cuexruntime.NewInternalPackage("http", http.GetTemplate(), http.GetProviders())),
		runtime.Must(cuexruntime.NewInternalPackage("kube", kube.GetTemplate(), kube.GetProviders())),
		runtime.Must(cuexruntime.NewInternalPackage("metrics", metrics.GetTemplate(), metrics.GetProviders())),
		runtime.Must(cuexruntime.NewInternalPackage("time", time.GetTemplate(), time.GetProviders())),
		runtime.Must(cuexruntime.NewInternalPackage("util", util.GetTemplate(), util.GetProviders())),
		runtime.Must(cuexruntime.NewInternalPackage("builtin", builtin.GetTemplate(), builtin.GetProviders())),

		// kubevela internal packages
		runtime.Must(cuexruntime.NewInternalPackage("multicluster", multicluster.GetTemplate(), multicluster.GetProviders())),
		runtime.Must(cuexruntime.NewInternalPackage("config", config.GetTemplate(), config.GetProviders())),
		runtime.Must(cuexruntime.NewInternalPackage("helm", helm.GetTemplate(), helm.GetProviders())),
		runtime.Must(cuexruntime.NewInternalPackage("oam", oam.GetTemplate(), oam.GetProviders())),
		runtime.Must(cuexruntime.NewInternalPackage("query", query.GetTemplate(), query.GetProviders())),
		runtime.Must(cuexruntime.NewInternalPackage("terraform", terraform.GetTemplate(), terraform.GetProviders())),
	), nil
})

// DefaultCompiler compiler for cuex to compile
var DefaultCompiler = singleton.NewSingleton[*cuex.Compiler](func() *cuex.Compiler {
	c := compiler.Get()
	if cuex.EnableExternalPackageForDefaultCompiler {
		if err := c.LoadExternalPackages(context.Background()); err != nil {
			klog.Errorf("failed to load external packages for cuex default compiler: %v", err.Error())
		}
	}
	if cuex.EnableExternalPackageWatchForDefaultCompiler {
		go c.ListenExternalPackages(nil)
	}
	return c
})
