/*
Copyright 2023 The KubeVela Authors.

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

package cuex

import (
	"context"

	"github.com/kubevela/pkg/cue/cuex"
	"github.com/kubevela/pkg/cue/cuex/providers/base64"
	cueext "github.com/kubevela/pkg/cue/cuex/providers/cue"
	"github.com/kubevela/pkg/cue/cuex/providers/http"
	"github.com/kubevela/pkg/cue/cuex/providers/kube"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/singleton"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/pkg/cue/cuex/providers/addon"
	"github.com/oam-dev/kubevela/pkg/cue/cuex/providers/config"
	"github.com/oam-dev/kubevela/pkg/cue/cuex/providers/helm"
	"github.com/oam-dev/kubevela/pkg/cue/cuex/providers/kuberead"
	"github.com/oam-dev/kubevela/pkg/cue/cuex/providers/registry"
	"github.com/oam-dev/kubevela/pkg/cue/cuex/providers/velaconfig"
)

// ConfigCompiler ...
var ConfigCompiler = singleton.NewSingleton[*cuex.Compiler](func() *cuex.Compiler {
	compiler := cuex.NewCompilerWithInternalPackages(
		config.Package,
	)
	return compiler
})

// sourcePackages are the CueX packages a SourceDefinition may import.
//
// A source is a cached, shared read: one resolution serves every Application
// with the same key, and the value outlives the render in a Config. So the set
// is what fetches, not what acts.
//
// helm and addon are excluded for that reason rather than for tidiness.
// helm.#Render calls installOrUpgradeChart outside dry-run, and the render path
// sets no dry-run, so a source importing vela/helm would install a real release
// on every cache miss. config is excluded too: it validates registry
// credentials, which is not a fetch.
//
// Adding one here widens what every SourceDefinition on the cluster can do.
// TestSourceCompilerPackageSetIsDeliberate fails when this drifts from
// WorkloadCompiler, so the choice has to be made rather than inherited.
func sourcePackages() []cuexruntime.Package {
	return []cuexruntime.Package{
		// vela/kube with #Apply and #Patch removed. The package set alone is not
		// enough: kube carries reads and writes under one name, so a source could
		// otherwise apply arbitrary resources on every cache miss.
		kuberead.Package,
		http.Package,
		base64.Package,
		registry.Package,
		velaconfig.Package,
		cueext.Package,
	}
}

func sourceCompilerPackageNames() []string {
	names := make([]string, 0, len(sourcePackages()))
	for _, p := range sourcePackages() {
		names = append(names, p.GetName())
	}
	return names
}

// SourceCompiler is the compiler a SourceDefinition's template is resolved with.
var SourceCompiler = singleton.NewSingleton[*cuex.Compiler](func() *cuex.Compiler {
	compiler := cuex.NewCompilerWithInternalPackages(sourcePackages()...)
	if cuex.EnableExternalPackageForDefaultCompiler {
		if err := compiler.LoadExternalPackages(context.Background()); err != nil {
			klog.Errorf("failed to load external packages for source compiler: %v", err.Error())
		}
	}
	return compiler
})

// WorkloadCompiler is the compiler for workload/component definitions
var WorkloadCompiler = singleton.NewSingleton[*cuex.Compiler](func() *cuex.Compiler {
	compiler := cuex.NewCompilerWithInternalPackages(
		config.Package,
		helm.Package,
		base64.Package,
		http.Package,
		kube.Package,
		cueext.Package,
		addon.Package,
		// SourceDefinitions compile against this compiler, so a source can read
		// a file from a registry the platform has configured, or the properties
		// of a Config the platform has created.
		registry.Package,
		velaconfig.Package,
	)
	if cuex.EnableExternalPackageForDefaultCompiler {
		if err := compiler.LoadExternalPackages(context.Background()); err != nil {
			klog.Errorf("failed to load external packages for workload compiler: %v", err.Error())
		}
	}
	return compiler
})
