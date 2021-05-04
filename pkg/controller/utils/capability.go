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

package utils

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/build"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commontypes "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/appfile/helm"
	mycue "github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/dsl/definition"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// ErrNoSectionParameterInCue means there is not parameter section in Cue template of a workload
const ErrNoSectionParameterInCue = "capability %s doesn't contain section `parameter`"

// CapabilityDefinitionInterface is the interface for Capability (WorkloadDefinition and TraitDefinition)
type CapabilityDefinitionInterface interface {
	GetCapabilityObject(ctx context.Context, k8sClient client.Client, namespace, name string) (*types.Capability, error)
	GetOpenAPISchema(ctx context.Context, k8sClient client.Client, namespace, name string) ([]byte, error)
}

// CapabilityComponentDefinition is the struct for ComponentDefinition
type CapabilityComponentDefinition struct {
	Name                string                      `json:"name"`
	ComponentDefinition v1beta1.ComponentDefinition `json:"componentDefinition"`

	WorkloadType    util.WorkloadType `json:"workloadType"`
	WorkloadDefName string            `json:"workloadDefName"`

	Helm *commontypes.Helm `json:"helm"`
	Kube *commontypes.Kube `json:"kube"`
	CapabilityBaseDefinition
}

// GetCapabilityObject gets types.Capability object by WorkloadDefinition name
func (def *CapabilityComponentDefinition) GetCapabilityObject(ctx context.Context, k8sClient client.Client, namespace, name string) (*types.Capability, error) {
	var componentDefinition v1beta1.ComponentDefinition
	var capability types.Capability
	objectKey := client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}
	err := k8sClient.Get(ctx, objectKey, &componentDefinition)
	if err != nil {
		return nil, fmt.Errorf("failed to get ComponentDefinition %s: %w", def.Name, err)
	}
	def.ComponentDefinition = componentDefinition

	switch def.WorkloadType {
	case util.ReferWorkload:
		var wd = new(v1alpha2.WorkloadDefinition)
		objectKey.Name = def.WorkloadDefName
		if err := k8sClient.Get(ctx, objectKey, wd); err != nil {
			return nil, fmt.Errorf("failed to get WorkloadDefinition that ComponentDefinition refers to")
		}
		capability, err = appfile.ConvertTemplateJSON2Object(name, wd.Spec.Extension, wd.Spec.Schematic)
	default:
		capability, err = appfile.ConvertTemplateJSON2Object(name, componentDefinition.Spec.Extension, componentDefinition.Spec.Schematic)
		if err != nil {
			return nil, fmt.Errorf("failed to convert ComponentDefinition to Capability Object")
		}
	}
	return &capability, err
}

// GetOpenAPISchema gets OpenAPI v3 schema by WorkloadDefinition name
func (def *CapabilityComponentDefinition) GetOpenAPISchema(ctx context.Context, k8sClient client.Client, pd *definition.PackageDiscover, namespace, name string) ([]byte, error) {
	capability, err := def.GetCapabilityObject(ctx, k8sClient, namespace, name)
	if err != nil {
		return nil, err
	}
	return getOpenAPISchema(*capability, pd)
}

// GetKubeSchematicOpenAPISchema gets OpenAPI v3 schema based on kube schematic parameters
func (def *CapabilityComponentDefinition) GetKubeSchematicOpenAPISchema(params []commontypes.KubeParameter) ([]byte, error) {
	required := []string{}
	properties := map[string]*openapi3.Schema{}
	for _, p := range params {
		var tmp *openapi3.Schema
		switch p.ValueType {
		case commontypes.StringType:
			tmp = openapi3.NewStringSchema()
		case commontypes.NumberType:
			tmp = openapi3.NewFloat64Schema()
		case commontypes.BooleanType:
			tmp = openapi3.NewBoolSchema()
		default:
			tmp = openapi3.NewStringSchema()
		}
		if p.Required != nil && *p.Required {
			required = append(required, p.Name)
		}
		// save FieldPaths into description
		tmp.Description = fmt.Sprintf("The value will be applied to fields: [%s].", strings.Join(p.FieldPaths, ","))
		if p.Description != nil {
			tmp.Description = fmt.Sprintf("%s\n %s", tmp.Description, *p.Description)
		}
		properties[p.Name] = tmp
	}
	s := openapi3.NewObjectSchema().WithProperties(properties)
	if len(required) > 0 {
		s.Required = required
	}
	b, err := s.MarshalJSON()
	if err != nil {
		return nil, errors.Wrap(err, "cannot marshal generated schema into json")
	}
	return b, nil
}

// StoreOpenAPISchema stores OpenAPI v3 schema in ConfigMap from WorkloadDefinition
func (def *CapabilityComponentDefinition) StoreOpenAPISchema(ctx context.Context, k8sClient client.Client,
	pd *definition.PackageDiscover, namespace, name, revName string) error {
	var jsonSchema []byte
	var err error
	switch def.WorkloadType {
	case util.HELMDef:
		jsonSchema, err = helm.GetChartValuesJSONSchema(ctx, def.Helm)
	case util.KubeDef:
		jsonSchema, err = def.GetKubeSchematicOpenAPISchema(def.Kube.Parameters)
	default:
		jsonSchema, err = def.GetOpenAPISchema(ctx, k8sClient, pd, namespace, name)
	}
	if err != nil {
		return fmt.Errorf("failed to generate OpenAPI v3 JSON schema for capability %s: %w", def.Name, err)
	}
	componentDefinition := def.ComponentDefinition
	ownerReference := []metav1.OwnerReference{{
		APIVersion:         componentDefinition.APIVersion,
		Kind:               componentDefinition.Kind,
		Name:               componentDefinition.Name,
		UID:                componentDefinition.GetUID(),
		Controller:         pointer.BoolPtr(true),
		BlockOwnerDeletion: pointer.BoolPtr(true),
	}}
	cmName, err := def.CreateOrUpdateConfigMap(ctx, k8sClient, namespace, componentDefinition.Name, jsonSchema, ownerReference)
	if err != nil {
		return err
	}
	def.ComponentDefinition.Status.ConfigMapRef = cmName

	_, err = def.CreateOrUpdateConfigMap(ctx, k8sClient, namespace, revName, jsonSchema, ownerReference)
	if err != nil {
		return err
	}
	return nil
}

// CapabilityTraitDefinition is the Capability struct for TraitDefinition
type CapabilityTraitDefinition struct {
	Name            string                  `json:"name"`
	TraitDefinition v1beta1.TraitDefinition `json:"traitDefinition"`
	CapabilityBaseDefinition
}

// GetCapabilityObject gets types.Capability object by TraitDefinition name
func (def *CapabilityTraitDefinition) GetCapabilityObject(ctx context.Context, k8sClient client.Client, namespace, name string) (*types.Capability, error) {
	var traitDefinition v1beta1.TraitDefinition
	var capability types.Capability
	capability.Name = def.Name
	objectKey := client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}
	err := k8sClient.Get(ctx, objectKey, &traitDefinition)
	if err != nil {
		return &capability, fmt.Errorf("failed to get WorkloadDefinition %s: %w", def.Name, err)
	}
	def.TraitDefinition = traitDefinition
	capability, err = appfile.ConvertTemplateJSON2Object(name, traitDefinition.Spec.Extension, traitDefinition.Spec.Schematic)
	if err != nil {
		return nil, fmt.Errorf("failed to convert WorkloadDefinition to Capability Object")
	}
	return &capability, err
}

// GetOpenAPISchema gets OpenAPI v3 schema by TraitDefinition name
func (def *CapabilityTraitDefinition) GetOpenAPISchema(ctx context.Context, k8sClient client.Client, pd *definition.PackageDiscover, namespace, name string) ([]byte, error) {
	capability, err := def.GetCapabilityObject(ctx, k8sClient, namespace, name)
	if err != nil {
		return nil, err
	}
	return getOpenAPISchema(*capability, pd)
}

// StoreOpenAPISchema stores OpenAPI v3 schema from TraitDefinition in ConfigMap
func (def *CapabilityTraitDefinition) StoreOpenAPISchema(ctx context.Context, k8sClient client.Client, pd *definition.PackageDiscover, namespace, name string) error {
	jsonSchema, err := def.GetOpenAPISchema(ctx, k8sClient, pd, namespace, name)
	if err != nil {
		return fmt.Errorf(util.ErrGenerateOpenAPIV2JSONSchemaForCapability, def.Name, err)
	}

	traitDefinition := def.TraitDefinition
	ownerReference := []metav1.OwnerReference{{
		APIVersion:         traitDefinition.APIVersion,
		Kind:               traitDefinition.Kind,
		Name:               traitDefinition.Name,
		UID:                traitDefinition.GetUID(),
		Controller:         pointer.BoolPtr(true),
		BlockOwnerDeletion: pointer.BoolPtr(true),
	}}
	cmName, err := def.CreateOrUpdateConfigMap(ctx, k8sClient, namespace, traitDefinition.Name, jsonSchema, ownerReference)
	if err != nil {
		return err
	}
	def.TraitDefinition.Status.ConfigMapRef = cmName
	return nil
}

// CapabilityBaseDefinition is the base struct for CapabilityWorkloadDefinition and CapabilityTraitDefinition
type CapabilityBaseDefinition struct {
}

// CreateOrUpdateConfigMap creates ConfigMap to store OpenAPI v3 schema or or updates data in ConfigMap
func (def *CapabilityBaseDefinition) CreateOrUpdateConfigMap(ctx context.Context, k8sClient client.Client, namespace, definitionName string, jsonSchema []byte, ownerReferences []metav1.OwnerReference) (string, error) {
	cmName := fmt.Sprintf("%s%s", types.CapabilityConfigMapNamePrefix, definitionName)
	var cm v1.ConfigMap
	var data = map[string]string{
		types.OpenapiV3JSONSchema: string(jsonSchema),
	}
	// No need to check the existence of namespace, if it doesn't exist, API server will return the error message
	// before it's to be reconciled by ComponentDefinition/TraitDefinition controller.
	err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: cmName}, &cm)
	if err != nil && apierrors.IsNotFound(err) {
		cm = v1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "ConfigMap",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            cmName,
				Namespace:       namespace,
				OwnerReferences: ownerReferences,
				Labels: map[string]string{
					"definition.oam.dev": "schema",
				},
			},
			Data: data,
		}
		err = k8sClient.Create(ctx, &cm)
		if err != nil {
			return cmName, fmt.Errorf(util.ErrUpdateCapabilityInConfigMap, definitionName, err)
		}
		return cmName, nil
	}

	cm.Data = data
	if err = k8sClient.Update(ctx, &cm); err != nil {
		return cmName, fmt.Errorf(util.ErrUpdateCapabilityInConfigMap, definitionName, err)
	}
	return cmName, nil
}

// getDefinition is the main function for GetDefinition API
func getOpenAPISchema(capability types.Capability, pd *definition.PackageDiscover) ([]byte, error) {
	openAPISchema, err := generateOpenAPISchemaFromCapabilityParameter(capability, pd)
	if err != nil {
		return nil, err
	}
	schema, err := ConvertOpenAPISchema2SwaggerObject(openAPISchema)
	if err != nil {
		return nil, err
	}
	fixOpenAPISchema("", schema)

	parameter, err := schema.MarshalJSON()
	if err != nil {
		return nil, err
	}
	return parameter, nil
}

// generateOpenAPISchemaFromCapabilityParameter returns the parameter of a definition in cue.Value format
func generateOpenAPISchemaFromCapabilityParameter(capability types.Capability, pd *definition.PackageDiscover) ([]byte, error) {
	template, err := prepareParameterCue(capability.Name, capability.CueTemplate)
	if err != nil {
		return nil, err
	}

	template += mycue.BaseTemplate
	if pd == nil {
		var r cue.Runtime
		cueInst, err := r.Compile("-", template)
		if err != nil {
			return nil, err
		}
		return common.GenOpenAPI(cueInst)
	}
	bi := build.NewContext().NewInstance("", nil)
	err = bi.AddFile("-", template)
	if err != nil {
		return nil, err
	}

	cueInst, err := pd.ImportPackagesAndBuildInstance(bi)
	if err != nil {
		return nil, err
	}
	return common.GenOpenAPI(cueInst)
}

// GenerateOpenAPISchemaFromDefinition returns the parameter of a definition
func GenerateOpenAPISchemaFromDefinition(definitionName, cueTemplate string) ([]byte, error) {
	capability := types.Capability{
		Name:        definitionName,
		CueTemplate: cueTemplate,
	}
	return generateOpenAPISchemaFromCapabilityParameter(capability, nil)
}

// prepareParameterCue cuts `parameter` section form definition .cue file
func prepareParameterCue(capabilityName, capabilityTemplate string) (string, error) {
	var template string
	var withParameterFlag bool
	r := regexp.MustCompile("[[:space:]]*parameter:[[:space:]]*{.*")

	for _, text := range strings.Split(capabilityTemplate, "\n") {
		if r.MatchString(text) {
			// a variable has to be refined as a definition which starts with "#"
			text = fmt.Sprintf("parameter: #parameter\n#%s", text)
			withParameterFlag = true
		}
		template += fmt.Sprintf("%s\n", text)
	}

	if !withParameterFlag {
		return "", fmt.Errorf(ErrNoSectionParameterInCue, capabilityName)
	}
	return template, nil
}

// fixOpenAPISchema fixes tainted `description` filed, missing of title `field`.
func fixOpenAPISchema(name string, schema *openapi3.Schema) {
	t := schema.Type
	switch t {
	case "object":
		for k, v := range schema.Properties {
			s := v.Value
			fixOpenAPISchema(k, s)
		}
	case "array":
		fixOpenAPISchema("", schema.Items.Value)
	}
	if name != "" {
		schema.Title = name
	}

	description := schema.Description
	if strings.Contains(description, appfile.UsageTag) {
		description = strings.Split(description, appfile.UsageTag)[1]
	}
	if strings.Contains(description, appfile.ShortTag) {
		description = strings.Split(description, appfile.ShortTag)[0]
		description = strings.TrimSpace(description)
	}
	schema.Description = description
}

// ConvertOpenAPISchema2SwaggerObject converts OpenAPI v2 JSON schema to Swagger Object
func ConvertOpenAPISchema2SwaggerObject(data []byte) (*openapi3.Schema, error) {
	swagger, err := openapi3.NewSwaggerLoader().LoadSwaggerFromData(data)
	if err != nil {
		return nil, err
	}

	schemaRef, ok := swagger.Components.Schemas[mycue.ParameterTag]
	if !ok {
		return nil, errors.New(util.ErrGenerateOpenAPIV2JSONSchemaForCapability)
	}
	return schemaRef.Value, nil
}
