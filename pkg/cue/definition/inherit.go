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

package definition

import (
	"context"

	"cuelang.org/go/cue"
	"github.com/kubevela/pkg/cue/cuex"

	"github.com/kubevela/workflow/pkg/cue/model/sets"
	"github.com/kubevela/workflow/pkg/cue/model/value"
	"github.com/kubevela/workflow/pkg/cue/process"

	velacuex "github.com/oam-dev/kubevela/pkg/cue/cuex"
	"github.com/oam-dev/kubevela/pkg/cue/render"
	"github.com/oam-dev/kubevela/pkg/definition/inherit"
)

// rendered is a definition's template, compiled and ready to be read.
type rendered struct {
	// value is what the engine reads output, outputs and patch off.
	value cue.Value
	// userErrors are the authored `errs:` from every level of the chain.
	userErrors []string
	// levels is each level's compiled value, root first, empty when nothing was
	// extended. The trait engine uses it to recover a patch strategy.
	levels []cue.Value
}

// extendsSomething reports whether this definition has a chain to render, so a
// definition that extends nothing keeps its existing path: the two engines
// assemble source differently, and a trait compiles without `context` opened.
func (d *def) extendsSomething() bool { return len(d.ancestors) > 0 }

// renderChain renders this definition on top of everything it extends.
func (d *def) renderChain(ctx process.Context, abstractTemplate, paramFile, contextFile string, surface inherit.Surface) (*rendered, error) {
	chain := make([]inherit.Level, 0, len(d.ancestors)+1)
	chain = append(chain, inherit.Level{Name: d.name, Template: abstractTemplate})
	chain = append(chain, d.ancestors...)

	res, err := inherit.Render(ctx.GetCtx(), chain, paramFile, contextFile, surface,
		inherit.Compilers{Render: compileLevel, Schema: compileLevelForSchema})
	if err != nil {
		return nil, err
	}
	return &rendered{value: res.Value, userErrors: res.Errs, levels: res.Levels}, nil
}

// patchOptionsFromLevels recovers a patch strategy declared anywhere in a chain.
// `+patchStrategy` is read off the doc comment on the `patch` field, which a
// merged value does not carry.
func patchOptionsFromLevels(levels []cue.Value, merged cue.Value) []sets.UnifyOption {
	if opts := sets.CreateUnifyOptionsForPatcher(merged); len(opts) > 0 {
		return opts
	}
	// A strategy is a whole-patch mode and the modes are mutually exclusive, so
	// the nearest declaration wins rather than every level contributing one:
	// collecting them all hands the merge jsonPatch and jsonMergePatch together.
	//
	// Levels run root first, so the walk is backwards. A level that declares no
	// strategy defers outward, which is how a root's still applies to a child
	// that says nothing about it.
	for i := len(levels) - 1; i >= 0; i-- {
		p := levels[i].LookupPath(value.FieldPath(PatchFieldName))
		if !p.Exists() {
			continue
		}
		if opts := sets.CreateUnifyOptionsForPatcher(p); len(opts) > 0 {
			return opts
		}
	}
	return nil
}

// compileLevel compiles one level with the render path's own compiler, so an
// inherited template sees the same provider packages. Each level is a file in
// its own right, so `context` and `parameter` are opened: a parent compiles once
// with no parameters, to read its declaration as a schema.
func compileLevel(ctx context.Context, src string) (cue.Value, error) {
	return velacuex.WorkloadCompiler.Get().CompileString(ctx, render.Template(src))
}

// compileLevelForSchema compiles a level only to read its parameters. Provider
// functions stay unresolved, since that pass supplies none for them to run on.
func compileLevelForSchema(ctx context.Context, src string) (cue.Value, error) {
	return velacuex.WorkloadCompiler.Get().CompileStringWithOptions(
		ctx, render.Template(src), cuex.DisableResolveProviderFunctions{})
}
