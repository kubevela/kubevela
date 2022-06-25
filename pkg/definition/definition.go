/*
Copyright 2021 The KubeVela Authors.

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

// Package definition contains some helper functions used in vela CLI
// and vela addon mechanism
package definition

import (
	"context"
	"fmt"
	"strings"

	"github.com/oam-dev/kubevela/pkg/utils/filters"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/encoding/gocode/gocodec"
	"cuelang.org/go/tools/fix"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velacue "github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/cue/model/sets"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
)

const (
	// DescriptionKey the key for accessing definition description
	DescriptionKey = "definition.oam.dev/description"
	// AliasKey the key for accessing definition alias
	AliasKey = "definition.oam.dev/alias"
	// UserPrefix defines the prefix of user customized label or annotation
	UserPrefix = "custom.definition.oam.dev/"
)

var (
	// DefinitionTemplateKeys the keys for accessing definition template
	DefinitionTemplateKeys = []string{"spec", "schematic", "cue", "template"}
	// DefinitionTypeToKind maps the definition types to corresponding kinds
	DefinitionTypeToKind = map[string]string{
		"component":     v1beta1.ComponentDefinitionKind,
		"trait":         v1beta1.TraitDefinitionKind,
		"policy":        v1beta1.PolicyDefinitionKind,
		"workload":      v1beta1.WorkloadDefinitionKind,
		"scope":         v1beta1.ScopeDefinitionKind,
		"workflow-step": v1beta1.WorkflowStepDefinitionKind,
	}
)

// Definition the general struct for handling all kinds of definitions like ComponentDefinition or TraitDefinition
type Definition struct {
	unstructured.Unstructured
}

// SetGVK set the GroupVersionKind of Definition
func (def *Definition) SetGVK(kind string) {
	def.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   v1beta1.Group,
		Version: v1beta1.Version,
		Kind:    kind,
	})
}

// GetType gets the type of Definition
func (def *Definition) GetType() string {
	kind := def.GetKind()
	for k, v := range DefinitionTypeToKind {
		if v == kind {
			return k
		}
	}
	return strings.ToLower(strings.TrimSuffix(kind, "Definition"))
}

// SetType sets the type of Definition
func (def *Definition) SetType(t string) error {
	kind, ok := DefinitionTypeToKind[t]
	if !ok {
		return fmt.Errorf("invalid type %s", t)
	}
	def.SetGVK(kind)
	return nil
}

// ToCUE converts Definition to CUE value (with predefined Definition's cue format)
func (def *Definition) ToCUE() (*cue.Value, string, error) {
	annotations := map[string]string{}
	for key, val := range def.GetAnnotations() {
		if strings.HasPrefix(key, UserPrefix) {
			annotations[strings.TrimPrefix(key, UserPrefix)] = val
		}
	}
	alias := def.GetAnnotations()[AliasKey]
	desc := def.GetAnnotations()[DescriptionKey]
	labels := map[string]string{}
	for key, val := range def.GetLabels() {
		if strings.HasPrefix(key, UserPrefix) {
			labels[strings.TrimPrefix(key, UserPrefix)] = val
		}
	}
	spec := map[string]interface{}{}
	for key, val := range def.Object["spec"].(map[string]interface{}) {
		if key != "schematic" {
			spec[key] = val
		}
	}
	obj := map[string]interface{}{
		def.GetName(): map[string]interface{}{
			"type":        def.GetType(),
			"alias":       alias,
			"description": desc,
			"annotations": annotations,
			"labels":      labels,
			"attributes":  spec,
		},
	}
	r := &cue.Runtime{}
	codec := gocodec.New(r, &gocodec.Config{})
	val, err := codec.Decode(obj)
	if err != nil {
		return nil, "", err
	}

	templateString, _, err := unstructured.NestedString(def.Object, DefinitionTemplateKeys...)
	if err != nil {
		return nil, "", err
	}
	templateString, err = formatCUEString(templateString)
	if err != nil {
		return nil, "", err
	}
	return &val, templateString, nil
}

// ToCUEString converts definition to CUE value and then encode to string
func (def *Definition) ToCUEString() (string, error) {
	val, templateString, err := def.ToCUE()
	if err != nil {
		return "", err
	}
	metadataString, err := sets.ToString(*val)
	if err != nil {
		return "", err
	}

	f, err := parser.ParseFile("-", templateString, parser.ParseComments)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse template cue string")
	}
	f = fix.File(f)
	var importDecls, templateDecls []ast.Decl
	for _, decl := range f.Decls {
		if importDecl, ok := decl.(*ast.ImportDecl); ok {
			importDecls = append(importDecls, importDecl)
		} else {
			templateDecls = append(templateDecls, decl)
		}
	}
	importString, err := encodeDeclsToString(importDecls)
	if err != nil {
		return "", errors.Wrapf(err, "failed to encode import decls")
	}
	templateString, err = encodeDeclsToString(templateDecls)
	if err != nil {
		return "", errors.Wrapf(err, "failed to encode template decls")
	}
	templateString = fmt.Sprintf("template: {\n%s}", templateString)

	completeCUEString := importString + "\n" + metadataString + "\n" + templateString
	if completeCUEString, err = formatCUEString(completeCUEString); err != nil {
		return "", errors.Wrapf(err, "failed to format cue format string")
	}
	return completeCUEString, nil
}

// FromCUE converts CUE value (predefined Definition's cue format) to Definition
// nolint:gocyclo
func (def *Definition) FromCUE(val *cue.Value, templateString string) error {
	if def.Object == nil {
		def.Object = map[string]interface{}{}
	}
	annotations := map[string]string{}
	for k, v := range def.GetAnnotations() {
		if !strings.HasPrefix(k, UserPrefix) && k != DescriptionKey {
			annotations[k] = v
		}
	}
	labels := map[string]string{}
	for k, v := range def.GetLabels() {
		if !strings.HasPrefix(k, UserPrefix) {
			annotations[k] = v
		}
	}
	spec, ok := def.Object["spec"].(map[string]interface{})
	if !ok {
		spec = map[string]interface{}{}
	}
	codec := gocodec.New(&cue.Runtime{}, &gocodec.Config{})
	nameFlag := false
	fields, err := val.Fields()
	if err != nil {
		return err
	}
	for fields.Next() {
		definitionName := fields.Label()
		v := fields.Value()
		if nameFlag {
			return fmt.Errorf("duplicated definition name found, %s and %s", def.GetName(), definitionName)
		}
		nameFlag = true
		def.SetName(definitionName)
		_fields, err := v.Fields()
		if err != nil {
			return err
		}
		for _fields.Next() {
			_key := _fields.Label()
			_value := _fields.Value()
			switch _key {
			case "type":
				_type, err := _value.String()
				if err != nil {
					return err
				}
				if err = def.SetType(_type); err != nil {
					return err
				}
			case "alias":
				alias, err := _value.String()
				if err != nil {
					return err
				}
				annotations[AliasKey] = alias
			case "description":
				desc, err := _value.String()
				if err != nil {
					return err
				}
				annotations[DescriptionKey] = desc
			case "annotations":
				var _annotations map[string]string
				if err := codec.Encode(_value, &_annotations); err != nil {
					return err
				}
				for _k, _v := range _annotations {
					annotations[UserPrefix+_k] = _v
				}
			case "labels":
				var _labels map[string]string
				if err := codec.Encode(_value, &_labels); err != nil {
					return err
				}
				for _k, _v := range _labels {
					labels[UserPrefix+_k] = _v
				}
			case "attributes":
				if err := codec.Encode(_value, &spec); err != nil {
					return err
				}
			}
		}
	}
	def.SetAnnotations(annotations)
	def.SetLabels(labels)
	if err := unstructured.SetNestedField(spec, templateString, DefinitionTemplateKeys[1:]...); err != nil {
		return err
	}
	def.Object["spec"] = spec
	return nil
}

func encodeDeclsToString(decls []ast.Decl) (string, error) {
	s := ""
	for _, decl := range decls {
		bs, err := format.Node(decl, format.Simplify())
		if err != nil {
			return "", errors.Wrapf(err, "failed to encode decl to string: %v", decl)
		}
		s += string(bs) + "\n"
	}
	return s, nil
}

// FromCUEString converts cue string into Definition
func (def *Definition) FromCUEString(cueString string, config *rest.Config) error {
	r := &cue.Runtime{}
	f, err := parser.ParseFile("-", cueString, parser.ParseComments)
	if err != nil {
		return err
	}
	n := fix.File(f)
	var importDecls, metadataDecls, templateDecls []ast.Decl
	for _, decl := range n.Decls {
		if importDecl, ok := decl.(*ast.ImportDecl); ok {
			importDecls = append(importDecls, importDecl)
		} else if field, ok := decl.(*ast.Field); ok {
			label := ""
			switch l := field.Label.(type) {
			case *ast.Ident:
				label = l.Name
			case *ast.BasicLit:
				label = l.Value
			}
			if label == "" {
				return errors.Errorf("found unexpected decl when parsing cue: %v", label)
			}
			if label == "template" {
				if v, ok := field.Value.(*ast.StructLit); ok {
					templateDecls = append(templateDecls, v.Elts...)
				} else {
					return errors.Errorf("unexpected decl found in template: %v", decl)
				}
			} else {
				metadataDecls = append(metadataDecls, field)
			}
		}
	}
	if len(metadataDecls) == 0 {
		return errors.Errorf("no metadata found, invalid")
	}
	if len(templateDecls) == 0 {
		return errors.Errorf("no template found, invalid")
	}
	var importString, metadataString, templateString string
	if importString, err = encodeDeclsToString(importDecls); err != nil {
		return errors.Wrapf(err, "failed to encode import decls to string")
	}
	if metadataString, err = encodeDeclsToString(metadataDecls); err != nil {
		return errors.Wrapf(err, "failed to encode metadata decls to string")
	}
	// notice that current template decls are concatenated without any blank lines which might be inconsistent with original cue file, but it would not affect the syntax
	if templateString, err = encodeDeclsToString(templateDecls); err != nil {
		return errors.Wrapf(err, "failed to encode template decls to string")
	}

	inst, err := r.Compile("-", metadataString)
	if err != nil {
		return err
	}
	templateString, err = formatCUEString(importString + templateString)
	if err != nil {
		return err
	}
	// validate template
	if config != nil {
		pd, err := packages.NewPackageDiscover(config)
		if err != nil {
			return err
		}
		if _, err = value.NewValue(templateString+"\n"+velacue.BaseTemplate, pd, ""); err != nil {
			return err
		}
	} else if _, err = r.Compile("-", templateString+"\n"+velacue.BaseTemplate); err != nil {
		return err
	}
	val := inst.Value()
	return def.FromCUE(&val, templateString)
}

// ValidDefinitionTypes return the list of valid definition types
func ValidDefinitionTypes() []string {
	var types []string
	for k := range DefinitionTypeToKind {
		types = append(types, k)
	}
	return types
}

// SearchDefinition search the Definition in k8s by traversing all possible results across types or namespaces
func SearchDefinition(c client.Client, definitionType, namespace string, additionalFilters ...filters.Filter) ([]unstructured.Unstructured, error) {
	ctx := context.Background()
	var kinds []string
	if definitionType != "" {
		kind, ok := DefinitionTypeToKind[definitionType]
		if !ok {
			return nil, fmt.Errorf("invalid definition type %s", kind)
		}
		kinds = []string{kind}
	} else {
		for _, kind := range DefinitionTypeToKind {
			kinds = append(kinds, kind)
		}
	}
	var listOptions []client.ListOption
	if namespace != "" {
		listOptions = []client.ListOption{client.InNamespace(namespace)}
	}
	var definitions []unstructured.Unstructured
	for _, kind := range kinds {
		objs := unstructured.UnstructuredList{}
		objs.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   v1beta1.Group,
			Version: v1beta1.Version,
			Kind:    kind + "List",
		})
		if err := c.List(ctx, &objs, listOptions...); err != nil {
			return nil, errors.Wrapf(err, "failed to get %s", kind)
		}

		// Apply filters to the object list
		filteredList := filters.ApplyToList(objs, additionalFilters...)

		definitions = append(definitions, filteredList.Items...)
	}
	return definitions, nil
}

// GetDefinitionDefaultSpec returns the default spec of Definition with given kind. This may be implemented with cue in the future.
func GetDefinitionDefaultSpec(kind string) map[string]interface{} {
	switch kind {
	case v1beta1.ComponentDefinitionKind:
		return map[string]interface{}{
			"workload": map[string]interface{}{
				"definition": map[string]interface{}{
					"apiVersion": "<change me> apps/v1",
					"kind":       "<change me> Deployment",
				},
			},
			"schematic": map[string]interface{}{
				"cue": map[string]interface{}{
					"template": "output: {}\nparameter: {}\n",
				},
			},
		}
	case v1beta1.TraitDefinitionKind:
		return map[string]interface{}{
			"appliesToWorkloads": []interface{}{},
			"conflictsWith":      []interface{}{},
			"workloadRefPath":    "",
			"definitionRef":      "",
			"podDisruptive":      false,
			"schematic": map[string]interface{}{
				"cue": map[string]interface{}{
					"template": "patch: {}\nparameter: {}\n",
				},
			},
		}
	}
	return map[string]interface{}{}
}

func formatCUEString(cueString string) (string, error) {
	f, err := parser.ParseFile("-", cueString, parser.ParseComments)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse file during format cue string")
	}
	n := fix.File(f)
	b, err := format.Node(n, format.Simplify())
	if err != nil {
		return "", errors.Wrapf(err, "failed to format node during formating cue string")
	}
	return string(b), nil
}
