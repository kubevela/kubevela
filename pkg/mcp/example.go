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
	"math"
	"sort"

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

// exampleObject fills the fields of an object schema that must be supplied:
// its required fields without a default, those of the first oneOf branch when
// the object is a disjunction of shapes, and optional ones in name order when
// minProperties asks for more.
func exampleObject(sch *openapi3.Schema, path string, placeholders *[]string, depth int) map[string]any {
	out := map[string]any{}
	if sch == nil || depth > maxExampleDepth {
		return out
	}
	var branch *openapi3.Schema
	if len(sch.OneOf) > 0 && sch.OneOf[0] != nil {
		branch = sch.OneOf[0].Value
	}
	fieldSchema := func(field string) *openapi3.Schema {
		if ref := sch.Properties[field]; ref != nil {
			return ref.Value
		}
		if branch != nil {
			if ref := branch.Properties[field]; ref != nil {
				return ref.Value
			}
		}
		return nil
	}
	fields := mustSupply(sch)
	if branch != nil {
		// A struct disjunction encodes as the union of its fields with a oneOf
		// of required sets, so one branch's set is needed for a valid example.
		for _, f := range branch.Required {
			if fs := fieldSchema(f); fs != nil && fs.Default != nil {
				continue
			}
			if !containsString(fields, f) {
				fields = append(fields, f)
			}
		}
	}
	for _, field := range fields {
		out[field] = exampleValue(fieldSchema(field), joinPath(path, field), placeholders, depth+1)
	}
	if uint64(len(out)) < sch.MinProps {
		names := make([]string, 0, len(sch.Properties))
		for name := range sch.Properties {
			if _, done := out[name]; !done {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		for _, name := range names {
			if uint64(len(out)) >= sch.MinProps {
				break
			}
			out[name] = exampleValue(fieldSchema(name), joinPath(path, name), placeholders, depth+1)
		}
	}
	return out
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// exampleNumber picks a stand-in inside the schema's bounds: the minimum, just
// above an exclusive one, or 1 capped by the maximum, so the example passes
// validation once the user keeps it.
func exampleNumber(sch *openapi3.Schema, integer bool) any {
	v := 1.0
	switch {
	case sch.Min != nil:
		v = *sch.Min
		if sch.ExclusiveMin {
			if !integer && sch.Max != nil {
				v = (*sch.Min + *sch.Max) / 2
			} else {
				v = *sch.Min + 1
			}
		}
	case sch.Max != nil:
		v = math.Min(1, *sch.Max)
		if sch.ExclusiveMax && v >= *sch.Max {
			v = *sch.Max - 1
		}
	}
	if integer {
		v = math.Ceil(v)
		if sch.Max != nil && (v > *sch.Max || (sch.ExclusiveMax && v >= *sch.Max)) {
			v = math.Floor(*sch.Max)
			if sch.ExclusiveMax && v >= *sch.Max {
				v--
			}
		}
		return int64(v)
	}
	return v
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
		n := sch.MinItems
		if n == 0 {
			n = 1
		}
		items := make([]any, 0, n)
		for i := uint64(0); i < n; i++ {
			items = append(items, exampleValue(sch.Items.Value, fmt.Sprintf("%s[%d]", path, i), placeholders, depth+1))
		}
		return items
	case sch.Type.Is(openapi3.TypeInteger):
		*placeholders = append(*placeholders, path)
		return exampleNumber(sch, true)
	case sch.Type.Is(openapi3.TypeNumber):
		*placeholders = append(*placeholders, path)
		return exampleNumber(sch, false)
	case sch.Type.Is(openapi3.TypeBoolean):
		*placeholders = append(*placeholders, path)
		return false
	default:
		*placeholders = append(*placeholders, path)
		return "<" + path + ">"
	}
}
