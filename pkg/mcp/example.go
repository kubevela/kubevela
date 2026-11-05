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

package mcp

import (
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
	"sigs.k8s.io/yaml"
)

const (
	// maxExampleDepth bounds how deep a generated example nests objects.
	maxExampleDepth = 6
	exampleName     = "example"
	exampleSource   = "generated from the parameter schema: only the parameters that must be supplied, with the schema's default or first allowed value where it has one and a placeholder otherwise"
)

// DefinitionExample is a minimal usage of a definition, generated from its
// parameter schema rather than fetched, so it always matches the
// definition installed on the cluster.
type DefinitionExample struct {
	YAML string `json:"yaml" jsonschema:"Minimal usage: a full Application for a component, otherwise the fragment that goes into one."`
	// Placeholders are the parameter paths given a stand-in value.
	Placeholders []string `json:"placeholders,omitempty" jsonschema:"Parameter paths given a stand-in value. Replace each with a real value, then check the draft with dryrun_application."`
	Source       string   `json:"source" jsonschema:"How the example was made."`
}

// definitionExample builds the example for one definition. A trait, policy
// or workflow step is shown as the fragment of an Application it goes in,
// since it cannot stand alone.
func definitionExample(defType, name string, sch *openapi3.Schema) (*DefinitionExample, error) {
	var placeholders []string
	props := exampleObject(sch, "", &placeholders, 0)
	entry := map[string]any{"type": name}
	if len(props) > 0 {
		entry["properties"] = props
	}
	var doc any
	switch defType {
	case DefinitionTypeComponent:
		entry["name"] = exampleName
		doc = map[string]any{
			"apiVersion": "core.oam.dev/v1beta1",
			"kind":       "Application",
			"metadata":   map[string]any{"name": exampleName},
			"spec":       map[string]any{"components": []any{entry}},
		}
	case DefinitionTypeTrait:
		doc = map[string]any{"traits": []any{entry}}
	case DefinitionTypePolicy:
		entry["name"] = exampleName
		doc = map[string]any{"policies": []any{entry}}
	case DefinitionTypeWorkflowStep:
		entry["name"] = exampleName
		doc = map[string]any{"workflow": map[string]any{"steps": []any{entry}}}
	default:
		return nil, fmt.Errorf("no example for definition type %q", defType)
	}
	raw, err := yaml.Marshal(doc)
	if err != nil {
		return nil, err
	}
	out := string(raw)
	switch defType {
	case DefinitionTypeTrait:
		out = "# goes under a component in spec.components\n" + out
	case DefinitionTypePolicy, DefinitionTypeWorkflowStep:
		out = "# goes under spec of an Application\n" + out
	default:
	}
	return &DefinitionExample{YAML: out, Placeholders: placeholders, Source: exampleSource}, nil
}

// exampleObject fills the fields of an object schema that must be supplied.
func exampleObject(sch *openapi3.Schema, path string, placeholders *[]string, depth int) map[string]any {
	out := map[string]any{}
	if sch == nil || depth > maxExampleDepth {
		return out
	}
	for _, field := range mustSupply(sch) {
		ref := sch.Properties[field]
		var fs *openapi3.Schema
		if ref != nil {
			fs = ref.Value
		}
		out[field] = exampleValue(fs, joinPath(path, field), placeholders, depth+1)
	}
	return out
}

func exampleValue(sch *openapi3.Schema, path string, placeholders *[]string, depth int) any {
	if sch == nil {
		*placeholders = append(*placeholders, path)
		return "<" + path + ">"
	}
	if sch.Default != nil {
		return sch.Default
	}
	if len(sch.Enum) > 0 {
		*placeholders = append(*placeholders, path)
		return sch.Enum[0]
	}
	switch {
	case sch.Type.Is(openapi3.TypeObject):
		obj := exampleObject(sch, path, placeholders, depth)
		// A required map, such as the gateway trait's http paths, names no
		// fields to fill, so the empty value stands in for entries.
		if len(obj) == 0 {
			*placeholders = append(*placeholders, path)
		}
		return obj
	case sch.Type.Is(openapi3.TypeArray):
		if sch.Items == nil || sch.Items.Value == nil || depth > maxExampleDepth {
			*placeholders = append(*placeholders, path)
			return []any{}
		}
		return []any{exampleValue(sch.Items.Value, path+"[0]", placeholders, depth+1)}
	case sch.Type.Is(openapi3.TypeInteger):
		*placeholders = append(*placeholders, path)
		if sch.Min != nil {
			return int64(*sch.Min)
		}
		return 1
	case sch.Type.Is(openapi3.TypeNumber):
		*placeholders = append(*placeholders, path)
		if sch.Min != nil {
			return *sch.Min
		}
		return 1
	case sch.Type.Is(openapi3.TypeBoolean):
		*placeholders = append(*placeholders, path)
		return false
	default:
		*placeholders = append(*placeholders, path)
		return "<" + path + ">"
	}
}
