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

package utils

import (
	"context"
	"encoding/json"

	"cuelang.org/go/cue"
	"github.com/kubevela/pkg/cue/cuex"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/definition/inherit"
	"github.com/oam-dev/kubevela/pkg/schema"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
)

// inheritedComponentSchema generates the published parameter schema of a
// ComponentDefinition that extends another. The generator compiles the whole
// template, and an extending definition does not compile on its own: `$super` is
// supplied by the chain.
func inheritedComponentSchema(ctx context.Context, cli client.Client, cd *v1beta1.ComponentDefinition) ([]byte, error) {
	ancestors, err := appfile.ComponentAncestors(ctx, cli, cd)
	if err != nil {
		return nil, err
	}
	return inheritedSchema(ctx, cd.Name, cd.Spec.Schematic.CUE.Template, ancestors, inherit.ComponentSurface)
}

// inheritedTraitSchema is inheritedComponentSchema for a TraitDefinition.
func inheritedTraitSchema(ctx context.Context, cli client.Client, td *v1beta1.TraitDefinition) ([]byte, error) {
	ancestors, err := appfile.TraitAncestors(ctx, cli, td)
	if err != nil {
		return nil, err
	}
	return inheritedSchema(ctx, td.Name, td.Spec.Schematic.CUE.Template, ancestors, inherit.TraitSurface)
}

func inheritedSchema(ctx context.Context, name, template string, ancestors []inherit.Level, surface inherit.Surface) ([]byte, error) {
	chain := make([]inherit.Level, 0, len(ancestors)+1)
	chain = append(chain, inherit.Level{Name: name, Template: template})
	chain = append(chain, ancestors...)

	val, err := inherit.SchemaValue(ctx, chain, schema.BaseTemplate, surface, schemaCompiler)
	if err != nil {
		return nil, err
	}
	s, err := schema.ParseValueToSchema(val)
	if err != nil {
		return nil, err
	}
	return json.Marshal(s)
}

// schemaCompiler matches what ParsePropertiesToSchema uses, so a schema is
// generated the same way whether or not the definition extends anything.
// Provider functions stay off: only the shape of `parameter` is being read.
func schemaCompiler(ctx context.Context, src string) (cue.Value, error) {
	// `parameter` is opened for the same reason the render path opens it: a
	// template compiled with nothing supplied must still yield its own
	// declaration rather than failing on the parameters it wanted.
	val, err := providers.DefaultCompiler.Get().CompileStringWithOptions(ctx, src+"\nparameter: _\n", cuex.DisableResolveProviderFunctions{})
	if err != nil {
		return val, err
	}
	return val, val.Err()
}
