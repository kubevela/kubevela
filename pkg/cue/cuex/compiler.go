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
	"github.com/kubevela/pkg/util/singleton"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/pkg/cue/cuex/providers/config"
	"github.com/oam-dev/kubevela/pkg/cue/options"
)

// ConfigCompiler ...
var ConfigCompiler = singleton.NewSingleton[*cuex.Compiler](func() *cuex.Compiler {
	compiler := cuex.NewCompilerWithInternalPackages(
		config.Package,
	)
	return compiler
})

// WorkloadCompiler ...
var WorkloadCompiler = singleton.NewSingleton[*cuex.Compiler](func() *cuex.Compiler {
	compiler := cuex.NewCompilerWithInternalPackages(
		config.Package,
	)
	if options.EnableExternalPackageForDefaultCompiler && options.EnableExternalPackagesForWorkloadsAndTraits {
		if err := compiler.LoadExternalPackages(context.Background()); err != nil && !kerrors.IsNotFound(err) {
			klog.Errorf("failed to load external packages for cuex default compiler: %s", err.Error())
		}
	}
	if options.EnableExternalPackageWatchForDefaultCompiler && options.EnableExternalPackagesForWorkloadsAndTraits {
		go compiler.ListenExternalPackages(nil)
	}
	return compiler
})
