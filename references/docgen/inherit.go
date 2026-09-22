/*
Copyright 2026 The KubeVela Authors.

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

package docgen

import (
	"context"
	"fmt"

	"cuelang.org/go/cue"
	"github.com/kubevela/pkg/cue/cuex"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	velacue "github.com/oam-dev/kubevela/pkg/cue"
	velacuex "github.com/oam-dev/kubevela/pkg/cue/cuex"
	"github.com/oam-dev/kubevela/pkg/definition/inherit"
)

// inheritedParameterValue resolves the `parameter` of a definition that extends
// another. A child may state its parameters as `$super.parameter & {...}`, so
// reading its template alone documents nothing. The chain resolves against the
// cluster, since a parent's template is the only place its parameters are
// written down.
func inheritedParameterValue(ctx context.Context, cli client.Client, capability *types.Capability, compilers ...*cuex.Compiler) (cue.Value, error) {
	ancestors, err := ancestorsOf(ctx, cli, capability)
	if err != nil {
		return cue.Value{}, err
	}

	chain := make([]inherit.Level, 0, len(ancestors)+1)
	chain = append(chain, inherit.Level{Name: capability.Name, Template: capability.CueTemplate})
	chain = append(chain, ancestors...)

	surface := inherit.ComponentSurface
	if capability.Type == types.TypeTrait {
		surface = inherit.TraitSurface
	}

	val, err := inherit.SchemaValue(ctx, chain, velacue.BaseTemplate, surface, docCompiler(compilers...))
	if err != nil {
		return cue.Value{}, err
	}
	param := val.LookupPath(cue.ParsePath(inherit.ParameterField))
	if !param.Exists() {
		return cue.Value{}, fmt.Errorf("%s declares no parameters, even through what it extends", capability.Name)
	}
	return param, nil
}

// ancestorsOf walks what a capability extends.
func ancestorsOf(ctx context.Context, cli client.Client, capability *types.Capability) ([]inherit.Level, error) {
	switch capability.Type {
	case types.TypeTrait:
		td := &v1beta1.TraitDefinition{}
		td.Name, td.Namespace = capability.Name, capability.Namespace
		td.Spec.Extends = capability.Extends
		td.Spec.Schematic = cueSchematic(capability.CueTemplate)
		return appfile.TraitAncestors(ctx, cli, td)
	default:
		cd := &v1beta1.ComponentDefinition{}
		cd.Name, cd.Namespace = capability.Name, capability.Namespace
		cd.Spec.Extends = capability.Extends
		cd.Spec.Schematic = cueSchematic(capability.CueTemplate)
		return appfile.ComponentAncestors(ctx, cli, cd)
	}
}

// docCompiler compiles a template for documentation, with provider functions off
// so printing a table fetches nothing, and `context` and `parameter` opened so a
// parent reading `context.name` resolves.
//
// It defaults to the render path's compiler: a chain whose parent imports
// `vela/helm` or another provider package documents nothing under a compiler
// that does not carry them. A caller may still pass its own.
func docCompiler(compilers ...*cuex.Compiler) inherit.CompileFunc {
	compiler := velacuex.WorkloadCompiler.Get()
	if len(compilers) > 0 && compilers[0] != nil {
		compiler = compilers[0]
	}
	return func(ctx context.Context, src string) (cue.Value, error) {
		val, err := compiler.CompileStringWithOptions(
			ctx, src+"\ncontext: _\nparameter: _\n", cuex.DisableResolveProviderFunctions{})
		if err != nil {
			return val, err
		}
		return val, val.Err()
	}
}

// cueSchematic wraps a template so a definition object can be reconstructed from
// a capability, which is all the chain walk needs.
func cueSchematic(template string) *common.Schematic {
	return &common.Schematic{CUE: &common.CUE{Template: template}}
}
