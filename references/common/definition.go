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

package common

import (
	"context"
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/encoding/gocode/gocodec"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/model/sets"
)

const (
	// DefinitionDescriptionKey the key for accessing definition description
	DefinitionDescriptionKey = "definition.oam.dev/description"
	// DefinitionUserPrefix defines the prefix of user customized label or annotation
	DefinitionUserPrefix = "custom.definition.oam.dev/"
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

// GetType get the type of Definition
func (def *Definition) GetType() string {
	kind := def.GetKind()
	for k, v := range DefinitionTypeToKind {
		if v == kind {
			return k
		}
	}
	return strings.ToLower(strings.TrimSuffix(kind, "Definition"))
}

// ToCUE converts Definition to CUE value (with predefined Definition's cue format)
func (def *Definition) ToCUE() (*cue.Value, error) {
	annotations := map[string]string{}
	for key, val := range def.GetAnnotations() {
		if strings.HasPrefix(key, DefinitionUserPrefix) {
			annotations[strings.TrimPrefix(key, DefinitionUserPrefix)] = val
		}
	}
	desc := def.GetAnnotations()[DefinitionDescriptionKey]
	labels := map[string]string{}
	for key, val := range def.GetLabels() {
		if strings.HasPrefix(key, DefinitionUserPrefix) {
			labels[strings.TrimPrefix(key, DefinitionUserPrefix)] = val
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
			"type": def.GetType(),
			"description": desc,
			"annotations": annotations,
			"labels":      labels,
			"spec":        spec,
		},
	}
	r := &cue.Runtime{}
	codec := gocodec.New(r, &gocodec.Config{})
	val, err := codec.Decode(obj)
	if err != nil {
		return nil, err
	}

	templateString, _, err := unstructured.NestedString(def.Object, DefinitionTemplateKeys...)
	if err != nil {
		return nil, err
	}
	templateString += "\ncontext: [string]: string"
	inst, err := r.Compile("-", templateString)
	if err != nil {
		return nil, err
	}
	templateVal := inst.Value()
	fields, err := templateVal.Fields()
	if err != nil {
		return nil, err
	}
	for fields.Next() {
		if k := fields.Label(); k != "context" {
			val = val.Fill(fields.Value(), "template", k)
		}
	}
	return &val, nil
}

// ToCUEString converts definition to CUE value and then encode to string
func (def *Definition) ToCUEString() (string, error) {
	val, err := def.ToCUE()
	if err != nil {
		return "", err
	}
	return sets.ToString(*val)
}

// FromCUE converts CUE value (predefined Definition's cue format) to Definition
func (def *Definition) FromCUE(val *cue.Value) error {
	if def.Object == nil {
		def.Object = map[string]interface{}{}
	}
	fields, err := val.Fields()
	if err != nil {
		return err
	}
	annotations := map[string]string{}
	for k, v := range def.GetAnnotations() {
		if !strings.HasPrefix(k, DefinitionUserPrefix) && k != DefinitionDescriptionKey {
			annotations[k] = v
		}
	}
	labels := map[string]string{}
	for k, v := range def.GetLabels() {
		if !strings.HasPrefix(k, DefinitionUserPrefix) {
			annotations[k] = v
		}
	}
	spec, ok := def.Object["spec"].(map[string]interface{})
	if !ok {
		spec = map[string]interface{}{}
	}
	templateString := ""
	codec := gocodec.New(&cue.Runtime{}, &gocodec.Config{})
	for fields.Next() {
		k := fields.Label()
		v := fields.Value()
		switch k {
		case "kind":
			kind, err := v.String()
			if err != nil {
				return err
			}
			def.SetGVK(kind)
		case "name":
			name, err := v.String()
			if err != nil {
				return err
			}
			def.SetName(name)
		case "description":
			desc, err := v.String()
			if err != nil {
				return err
			}
			annotations[DefinitionDescriptionKey] = desc
		case "annotations":
			var _annotations map[string]string
			if err := codec.Encode(v, &_annotations); err != nil {
				return err
			}
			for _k, _v := range _annotations {
				annotations[DefinitionUserPrefix+_k] = _v
			}
		case "labels":
			var _labels map[string]string
			if err := codec.Encode(v, &_labels); err != nil {
				return err
			}
			for _k, _v := range _labels {
				labels[DefinitionUserPrefix+_k] = _v
			}
		case "spec":
			if err := codec.Encode(v, &spec); err != nil {
				return err
			}
		case "template":
			templateString, err = sets.ToString(v)
			if err != nil {
				return err
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

// FromCUEString converts cue string into Definition
func (def *Definition) FromCUEString(cueString string) error {
	r := &cue.Runtime{}
	cueString += "\ncontext: [string]: string"
	inst, err := r.Compile("-", cueString)
	if err != nil {
		return err
	}
	val := inst.Value()
	return def.FromCUE(&val)
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
func SearchDefinition(definitionName string, c client.Client, definitionType string, namespace string) ([]unstructured.Unstructured, error) {
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
			Kind:    kind,
		})
		if err := c.List(ctx, &objs, listOptions...); err != nil {
			return nil, errors.Wrapf(err, "failed to get %s", kind)
		}
		for _, obj := range objs.Items {
			if definitionName == "*" || obj.GetName() == definitionName {
				definitions = append(definitions, obj)
			}
		}
	}
	return definitions, nil
}

// GetDefinitionDefaultSpec returns the default spec of Definition with given kind. This may be implemented with cue in the future.
func GetDefinitionDefaultSpec(kind string) map[string]interface{} {
	switch kind {
	case v1beta1.ComponentDefinitionKind:
		return map[string]interface{}{
			"workload": map[string]interface{}{
				"definitions": map[string]interface{}{
					"apiVersion": "<change me> apps/v1",
					"kind":       "<change me> Deployment",
				},
			},
			"schematic": map[string]interface{}{
				"cue": map[string]interface{}{
					"template": "output: {}\nparameters: {}\n",
				},
			},
		}
	case v1beta1.TraitDefinitionKind:
		return map[string]interface{}{
			"appliesToWorkloads": []string{},
			"conflictsWith":      []string{},
			"workloadRefPath":    "",
			"definitionRef":      "",
			"podDisruptive":      false,
			"schematic": map[string]interface{}{
				"cue": map[string]interface{}{
					"template": "patch: {}\n",
				},
			},
		}
	}
	return map[string]interface{}{}
}
