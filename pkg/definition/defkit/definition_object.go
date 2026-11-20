/*
Copyright 2025 The KubeVela Authors.

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

package defkit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	defast "github.com/oam-dev/kubevela/pkg/definition/ast"
)

const (
	// userMetadataPrefix is put in front of label and annotation keys that are
	// not already under an oam.dev domain.
	userMetadataPrefix = "custom.definition.oam.dev/"
	descriptionKey     = "definition.oam.dev/description"
	aliasKey           = "definition.oam.dev/alias"
)

// definitionObject builds the custom resource of the given kind from a
// definition file as ToCue writes it: a header named after the definition
// followed by a `template` block.
//
// The header becomes metadata and spec. Only the body of `template`, preceded
// by the file's imports, becomes spec.schematic.cue.template, which is what
// the controller compiles. This is the conversion `vela def apply` performs,
// without the dependencies of that package.
func definitionObject(kind, cueStr string) (map[string]any, error) {
	f, err := parser.ParseFile("-", cueStr, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	var importDecls, headerDecls, templateDecls []ast.Decl
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.ImportDecl:
			importDecls = append(importDecls, d)
		case *ast.Field:
			// the label may be written as an identifier or as a quoted string
			if defast.GetFieldLabel(d.Label) == "template" {
				body, ok := d.Value.(*ast.StructLit)
				if !ok {
					return nil, fmt.Errorf("template of %s is not a struct", kind)
				}
				templateDecls = append(templateDecls, body.Elts...)
				continue
			}
			// status expressions are stored as strings
			if err := defast.EncodeMetadata(d); err != nil {
				return nil, fmt.Errorf("failed to parse metadata: %w", err)
			}
			headerDecls = append(headerDecls, d)
		}
	}
	if len(headerDecls) == 0 {
		return nil, fmt.Errorf("no metadata found in %s", kind)
	}
	if len(templateDecls) == 0 {
		return nil, fmt.Errorf("no template found in %s", kind)
	}
	imports, err := formatDecls(importDecls)
	if err != nil {
		return nil, err
	}
	header, err := formatDecls(headerDecls)
	if err != nil {
		return nil, err
	}
	template, err := formatDecls(templateDecls)
	if err != nil {
		return nil, err
	}
	if imp := strings.TrimSpace(imports); imp != "" {
		template = imp + "\n" + template
	}

	metadata, spec, err := decodeDefinitionHeader(header)
	if err != nil {
		return nil, err
	}
	spec["schematic"] = map[string]any{"cue": map[string]any{"template": template}}
	if err := validateDefinitionSpec(kind, spec); err != nil {
		return nil, fmt.Errorf("invalid definition spec: %w", err)
	}
	return map[string]any{
		"apiVersion": "core.oam.dev/v1beta1",
		"kind":       kind,
		"metadata":   metadata,
		"spec":       spec,
	}, nil
}

// validateDefinitionSpec rejects a spec with a field the definition kind does
// not have, such as a misspelled attribute, the way the API server would.
func validateDefinitionSpec(kind string, spec map[string]any) error {
	var typed any
	switch kind {
	case v1beta1.ComponentDefinitionKind:
		typed = &v1beta1.ComponentDefinitionSpec{}
	case v1beta1.TraitDefinitionKind:
		typed = &v1beta1.TraitDefinitionSpec{}
	case v1beta1.PolicyDefinitionKind:
		typed = &v1beta1.PolicyDefinitionSpec{}
	case v1beta1.WorkflowStepDefinitionKind:
		typed = &v1beta1.WorkflowStepDefinitionSpec{}
	default:
		return nil
	}
	bs, err := json.Marshal(spec)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(bs))
	decoder.DisallowUnknownFields()
	return decoder.Decode(typed)
}

func formatDecls(decls []ast.Decl) (string, error) {
	bs, err := format.Node(&ast.File{Decls: decls}, format.Simplify())
	if err != nil {
		return "", fmt.Errorf("failed to encode cue: %w", err)
	}
	return strings.TrimSpace(string(bs)) + "\n", nil
}

// decodeDefinitionHeader evaluates the header of a definition file and
// returns the metadata and spec it describes.
func decodeDefinitionHeader(header string) (map[string]any, map[string]any, error) {
	val := cuecontext.New().CompileString(header)
	if err := val.Err(); err != nil {
		return nil, nil, err
	}
	defs, err := val.Fields()
	if err != nil {
		return nil, nil, err
	}
	if !defs.Next() {
		return nil, nil, fmt.Errorf("no metadata found")
	}
	name := defs.Selector().Unquoted()
	fields, err := defs.Value().Fields()
	if err != nil {
		return nil, nil, err
	}
	if defs.Next() {
		return nil, nil, fmt.Errorf("duplicated definition name found, %s and %s", name, defs.Selector().Unquoted())
	}

	annotations := map[string]string{}
	labels := map[string]string{}
	spec := map[string]any{}
	for fields.Next() {
		v := fields.Value()
		switch fields.Selector().Unquoted() {
		case "extends":
			var extends string
			if err = v.Decode(&extends); err == nil {
				spec["extends"] = extends
			}
		case "abstract":
			var abstract bool
			if err = v.Decode(&abstract); err == nil {
				spec["abstract"] = abstract
			}
		case "alias":
			var alias string
			if err = v.Decode(&alias); err == nil {
				annotations[aliasKey] = alias
			}
		case "description":
			var description string
			if err = v.Decode(&description); err == nil {
				annotations[descriptionKey] = description
			}
		case "annotations":
			err = decodeUserMetadata(v, annotations)
		case "labels":
			err = decodeUserMetadata(v, labels)
		case "attributes":
			err = v.Decode(&spec)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("%s of %s: %w", fields.Selector(), name, err)
		}
	}
	return map[string]any{"name": name, "annotations": annotations, "labels": labels}, spec, nil
}

// decodeUserMetadata adds the string map in v to dst, prefixing the keys that
// are not under an oam.dev domain.
func decodeUserMetadata(v cue.Value, dst map[string]string) error {
	var src map[string]string
	if err := v.Decode(&src); err != nil {
		return err
	}
	for k, val := range src {
		if !strings.Contains(k, "oam.dev") {
			k = userMetadataPrefix + k
		}
		dst[k] = val
	}
	return nil
}
