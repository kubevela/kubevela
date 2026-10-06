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

	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/definition/inherit"
	"github.com/oam-dev/kubevela/pkg/schema"
)

// componentOutputSchemaData is outputSchemaData for a ComponentDefinition and
// what it extends.
func componentOutputSchemaData(ctx context.Context, cli client.Client, cd *v1beta1.ComponentDefinition) map[string]string {
	levels := []inherit.Level{{Name: cd.Name, Template: cd.Spec.Schematic.CUE.Template}}
	if cd.Spec.Extends != "" {
		ancestors, err := appfile.ComponentAncestors(ctx, cli, cd)
		if err != nil {
			klog.InfoS("skipping output schemas", "definition", cd.Name, "err", err.Error())
			return nil
		}
		levels = append(levels, ancestors...)
	}
	return outputSchemaData(ctx, levels)
}

// traitOutputSchemaData is outputSchemaData for a TraitDefinition and what it
// extends.
func traitOutputSchemaData(ctx context.Context, cli client.Client, td *v1beta1.TraitDefinition) map[string]string {
	levels := []inherit.Level{{Name: td.Name, Template: td.Spec.Schematic.CUE.Template}}
	if td.Spec.Extends != "" {
		ancestors, err := appfile.TraitAncestors(ctx, cli, td)
		if err != nil {
			klog.InfoS("skipping output schemas", "definition", td.Name, "err", err.Error())
			return nil
		}
		levels = append(levels, ancestors...)
	}
	return outputSchemaData(ctx, levels)
}

// outputSchemaData is the schema ConfigMap data describing what a definition
// applies: its `output` and its `outputs` by resource name, each key present
// only where it declares that block. levels are the definition and what it
// extends, nearest first; each is laid over the one it extends, as its render
// merges them. Only VelaUX reads these, so a definition they cannot be
// generated for, or whose chain cannot be read, stores neither and still
// reconciles.
func outputSchemaData(ctx context.Context, levels []inherit.Level) map[string]string {
	var schemas *schema.OutputSchemas
	for i := len(levels) - 1; i >= 0; i-- {
		lvl := levels[i]
		own, err := schema.GenerateOutputSchemas(ctx, lvl.Template)
		if err != nil {
			klog.InfoS("skipping output schemas", "definition", levels[0].Name, "level", lvl.Name, "err", err.Error())
			return nil
		}
		if schemas == nil {
			schemas = own
			continue
		}
		output, err := inherit.Inherits(lvl, schema.OutputFieldName)
		if err != nil {
			klog.InfoS("skipping output schemas", "definition", levels[0].Name, "level", lvl.Name, "err", err.Error())
			return nil
		}
		outputs, _ := inherit.Inherits(lvl, schema.OutputsFieldName)
		schemas = schemas.Extend(own, output, outputs)
	}
	data := map[string]string{}
	if schemas == nil {
		return data
	}
	if schemas.Output != nil {
		if b, err := json.Marshal(schemas.Output); err == nil {
			data[types.OutputSchema] = string(b)
		}
	}
	if len(schemas.Outputs) > 0 {
		if b, err := json.Marshal(schemas.Outputs); err == nil {
			data[types.OutputsSchema] = string(b)
		}
	}
	return data
}
