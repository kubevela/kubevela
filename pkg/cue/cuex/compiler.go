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
	"sync"

	"github.com/kubevela/pkg/cue/cuex"
	"github.com/kubevela/pkg/cue/cuex/providers/base64"
	cueext "github.com/kubevela/pkg/cue/cuex/providers/cue"
	"github.com/kubevela/pkg/cue/cuex/providers/kube"
	cuexutil "github.com/kubevela/pkg/cue/cuex/providers/util"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/singleton"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/pkg/cue/cuex/providers/config"
	"github.com/oam-dev/kubevela/pkg/cue/cuex/providers/guardedhttp"
	"github.com/oam-dev/kubevela/pkg/cue/cuex/providers/helm"
	"github.com/oam-dev/kubevela/pkg/utils/kubeconfig"
)

// ConfigCompiler ...
var ConfigCompiler = singleton.NewSingleton[*cuex.Compiler](func() *cuex.Compiler {
	compiler := cuex.NewCompilerWithInternalPackages(
		config.Package,
	)
	return compiler
})

// WorkloadCompiler is the compiler for workload/component definitions
var WorkloadCompiler = singleton.NewSingleton[*cuex.Compiler](func() *cuex.Compiler {
	compiler := cuex.NewCompilerWithInternalPackages(
		config.Package,
		helm.Package,
		base64.Package,
		guardedhttp.Package,
		kube.Package,
		cueext.Package,
	)
	// See the note in pkg/workflow/providers/compiler.go: LoadExternalPackages
	// and ListenExternalPackages reach config.GetConfigOrDie, which exits the
	// process instead of returning an error when no kubeconfig exists.
	if cuex.EnableExternalPackageForDefaultCompiler || cuex.EnableExternalPackageWatchForDefaultCompiler {
		if err := kubeconfig.CheckFor("external CUE packages for the workload compiler"); err != nil {
			return compiler
		}
	}
	if cuex.EnableExternalPackageForDefaultCompiler {
		if err := compiler.LoadExternalPackages(context.Background()); err != nil {
			klog.Errorf("failed to load external packages for workload compiler: %v", err.Error())
		}
	}
	if cuex.EnableExternalPackageWatchForDefaultCompiler {
		go compiler.ListenExternalPackages(nil)
	}
	return compiler
})

// DefaultPackages are the packages of the upstream cuex.DefaultCompiler, with
// vela/http replaced by the guarded variant.
func DefaultPackages() []cuexruntime.Package {
	return []cuexruntime.Package{
		base64.Package,
		guardedhttp.Package,
		kube.Package,
		cueext.Package,
		cuexutil.Package,
	}
}

// GuardDefaultCompiler replaces the upstream cuex.DefaultCompiler with one built
// from DefaultPackages, so a vela/http call in a template rendered by it goes
// through the outbound HTTP guard. It loads external packages the way the
// upstream compiler does. Call it once at startup.
func GuardDefaultCompiler() {
	guardDefaultCompilerOnce.Do(guardDefaultCompiler)
}

var guardDefaultCompilerOnce sync.Once

// guardDefaultCompiler builds and installs the guarded default compiler.
func guardDefaultCompiler() {
	compiler := cuex.NewCompilerWithInternalPackages(DefaultPackages()...)
	if cuex.EnableExternalPackageForDefaultCompiler {
		if err := compiler.LoadExternalPackages(context.Background()); err != nil && !kerrors.IsNotFound(err) {
			klog.Errorf("failed to load external packages for cuex default compiler: %s", err.Error())
		}
	}
	if cuex.EnableExternalPackageWatchForDefaultCompiler {
		go compiler.ListenExternalPackages(nil)
	}
	cuex.DefaultCompiler.Set(compiler)
}
