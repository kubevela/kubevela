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
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/build"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/pkg/errors"
	"gopkg.in/src-d/go-git.v4"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commontypes "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/appfile/helm"
	velacue "github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/cue/model"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/terraform"
)

// data types of parameter value
const (
	TerraformVariableString string = "string"
	TerraformVariableNumber string = "number"
	TerraformVariableBool   string = "bool"
	TerraformVariableList   string = "list"
	TerraformVariableTuple  string = "tuple"
	TerraformVariableMap    string = "map"
	TerraformVariableObject string = "object"
	TerraformVariableNull   string = ""
	TerraformVariableAny    string = "any"

	TerraformListTypePrefix   string = "list("
	TerraformTupleTypePrefix  string = "tuple("
	TerraformMapTypePrefix    string = "map("
	TerraformObjectTypePrefix string = "object("
	TerraformSetTypePrefix    string = "set("

	typeTraitDefinition        = "trait"
	typeComponentDefinition    = "component"
	typeWorkflowStepDefinition = "workflowstep"
	typePolicyStepDefinition   = "policy"
)

// ErrNoSectionParameterInCue means there is not parameter section in Cue template of a workload
type ErrNoSectionParameterInCue struct {
	capName string
}

func (e ErrNoSectionParameterInCue) Error() string {
	return fmt.Sprintf("capability %s doesn't contain section `parameter`", e.capName)
}

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

	Helm      *commontypes.Helm      `json:"helm"`
	Kube      *commontypes.Kube      `json:"kube"`
	Terraform *commontypes.Terraform `json:"terraform"`
	CapabilityBaseDefinition
}

// NewCapabilityComponentDef will create a CapabilityComponentDefinition
func NewCapabilityComponentDef(componentDefinition *v1beta1.ComponentDefinition) CapabilityComponentDefinition {
	var def CapabilityComponentDefinition
	def.Name = componentDefinition.Name
	if componentDefinition.Spec.Workload.Definition == (commontypes.WorkloadGVK{}) && componentDefinition.Spec.Workload.Type != "" {
		def.WorkloadType = util.ReferWorkload
		def.WorkloadDefName = componentDefinition.Spec.Workload.Type
	}
	if componentDefinition.Spec.Schematic != nil {
		if componentDefinition.Spec.Schematic.HELM != nil {
			def.WorkloadType = util.HELMDef
			def.Helm = componentDefinition.Spec.Schematic.HELM
		}
		if componentDefinition.Spec.Schematic.KUBE != nil {
			def.WorkloadType = util.KubeDef
			def.Kube = componentDefinition.Spec.Schematic.KUBE
		}
		if componentDefinition.Spec.Schematic.Terraform != nil {
			def.WorkloadType = util.TerraformDef
			def.Terraform = componentDefinition.Spec.Schematic.Terraform
		}
	}
	def.ComponentDefinition = *componentDefinition.DeepCopy()
	return def
}

// GetOpenAPISchema gets OpenAPI v3 schema by WorkloadDefinition name
func (def *CapabilityComponentDefinition) GetOpenAPISchema(pd *packages.PackageDiscover, name string) ([]byte, error) {
	capability, err := appfile.ConvertTemplateJSON2Object(name, def.ComponentDefinition.Spec.Extension, def.ComponentDefinition.Spec.Schematic)
	if err != nil {
		return nil, fmt.Errorf("failed to convert ComponentDefinition to Capability Object")
	}
	return getOpenAPISchema(capability, pd)
}

// GetOpenAPISchemaFromTerraformComponentDefinition gets OpenAPI v3 schema by WorkloadDefinition name
func GetOpenAPISchemaFromTerraformComponentDefinition(configuration string) ([]byte, error) {
	schemas := make(map[string]*openapi3.Schema)
	var required []string
	variables, _, err := common.ParseTerraformVariables(configuration)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate capability properties")
	}
	for k, v := range variables {
		var schema *openapi3.Schema
		switch v.Type {
		case TerraformVariableString:
			schema = openapi3.NewStringSchema()
		case TerraformVariableNumber:
			schema = openapi3.NewFloat64Schema()
		case TerraformVariableBool:
			schema = openapi3.NewBoolSchema()
		case TerraformVariableList, TerraformVariableTuple:
			schema = openapi3.NewArraySchema()
		case TerraformVariableMap, TerraformVariableObject:
			schema = openapi3.NewObjectSchema()
		case TerraformVariableAny:
			switch v.Default.(type) {
			case []interface{}:
				schema = openapi3.NewArraySchema()
			case map[string]interface{}:
				schema = openapi3.NewObjectSchema()
			}
		case TerraformVariableNull:
			switch v.Default.(type) {
			case nil, string:
				schema = openapi3.NewStringSchema()
			case []interface{}:
				schema = openapi3.NewArraySchema()
			case map[string]interface{}:
				schema = openapi3.NewObjectSchema()
			case int, float64:
				schema = openapi3.NewFloat64Schema()
			default:
				return nil, fmt.Errorf("null type variable is NOT supported, please specify a type for the variable: %s", v.Name)
			}
		}

		// To identify unusual list type
		if schema == nil {
			switch {
			case strings.HasPrefix(v.Type, TerraformListTypePrefix) || strings.HasPrefix(v.Type, TerraformTupleTypePrefix) ||
				strings.HasPrefix(v.Type, TerraformSetTypePrefix):
				schema = openapi3.NewArraySchema()
			case strings.HasPrefix(v.Type, TerraformMapTypePrefix) || strings.HasPrefix(v.Type, TerraformObjectTypePrefix):
				schema = openapi3.NewObjectSchema()
			default:
				return nil, fmt.Errorf("the type `%s` of variable %s is NOT supported", v.Type, v.Name)
			}
		}
		schema.Title = k
		if v.Required {
			required = append(required, k)
		}
		if v.Default != nil {
			schema.Default = v.Default
		}
		schema.Description = v.Description
		schemas[v.Name] = schema
	}

	otherProperties := parseOtherProperties4TerraformDefinition()
	for k, v := range otherProperties {
		schemas[k] = v
	}

	return generateJSONSchemaWithRequiredProperty(schemas, required)
}

// GetTerraformConfigurationFromRemote gets Terraform Configuration(HCL)
func GetTerraformConfigurationFromRemote(name, remoteURL, remotePath string) (string, error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	cachePath := filepath.Join(userHome, ".vela", "terraform", name)
	// Check if the directory exists. If yes, remove it.
	entities, err := os.ReadDir(cachePath)
	if err != nil || len(entities) == 0 {
		fmt.Printf("loading terraform module %s into %s from %s\n", name, cachePath, remoteURL)
		if _, err = git.PlainClone(cachePath, false, &git.CloneOptions{
			URL:      remoteURL,
			Progress: os.Stdout,
		}); err != nil {
			return "", err
		}
	}

	tfPath := filepath.Join(cachePath, remotePath, "variables.tf")
	if _, err := os.Stat(tfPath); err != nil {
		tfPath = filepath.Join(cachePath, remotePath, "main.tf")
		if _, err := os.Stat(tfPath); err != nil {
			return "", errors.Wrap(err, "failed to find main.tf or variables.tf in Terraform configurations of the remote repository")
		}
	}
	conf, err := ioutil.ReadFile(filepath.Clean(tfPath))
	if err != nil {
		return "", errors.Wrap(err, "failed to read Terraform configuration")
	}
	return string(conf), nil
}

func parseOtherProperties4TerraformDefinition() map[string]*openapi3.Schema {
	otherProperties := make(map[string]*openapi3.Schema)

	// 1. writeConnectionSecretToRef
	secretName := openapi3.NewStringSchema()
	secretName.Title = "name"
	secretName.Description = terraform.TerraformSecretNameDescription

	secretNamespace := openapi3.NewStringSchema()
	secretNamespace.Title = "namespace"
	secretNamespace.Description = terraform.TerraformSecretNamespaceDescription

	secret := openapi3.NewObjectSchema()
	secret.Title = terraform.TerraformWriteConnectionSecretToRefName
	secret.Description = terraform.TerraformWriteConnectionSecretToRefDescription
	secret.Properties = openapi3.Schemas{
		"name":      &openapi3.SchemaRef{Value: secretName},
		"namespace": &openapi3.SchemaRef{Value: secretNamespace},
	}
	secret.Required = []string{"name"}

	otherProperties[terraform.TerraformWriteConnectionSecretToRefName] = secret

	// 2. providerRef
	providerName := openapi3.NewStringSchema()
	providerName.Title = "name"
	providerName.Description = "The name of the Terraform Cloud provider"

	providerNamespace := openapi3.NewStringSchema()
	providerNamespace.Title = "namespace"
	providerNamespace.Default = "default"
	providerNamespace.Description = "The namespace of the Terraform Cloud provider"

	var providerRefName = "providerRef"
	provider := openapi3.NewObjectSchema()
	provider.Title = providerRefName
	provider.Description = "specifies the Provider"
	provider.Properties = openapi3.Schemas{
		"name":      &openapi3.SchemaRef{Value: providerName},
		"namespace": &openapi3.SchemaRef{Value: providerNamespace},
	}
	provider.Required = []string{"name"}

	otherProperties[providerRefName] = provider

	// 3. deleteResource
	var deleteResourceName = "deleteResource"
	deleteResource := openapi3.NewBoolSchema()
	deleteResource.Title = deleteResourceName
	deleteResource.Description = "DeleteResource will determine whether provisioned cloud resources will be deleted when application is deleted"
	deleteResource.Default = true
	otherProperties[deleteResourceName] = deleteResource

	// 4. region
	var regionName = "region"
	region := openapi3.NewStringSchema()
	region.Title = regionName
	region.Description = "Region is cloud provider's region. It will override providerRef"
	otherProperties[regionName] = region

	return otherProperties

}

func generateJSONSchemaWithRequiredProperty(schemas map[string]*openapi3.Schema, required []string) ([]byte, error) {
	s := openapi3.NewObjectSchema().WithProperties(schemas)
	if len(required) > 0 {
		s.Required = required
	}
	b, err := s.MarshalJSON()
	if err != nil {
		return nil, errors.Wrap(err, "cannot marshal generated schema into json")
	}
	return b, nil
}

// GetKubeSchematicOpenAPISchema gets OpenAPI v3 schema based on kube schematic parameters for component and trait definition
func GetKubeSchematicOpenAPISchema(params []commontypes.KubeParameter) ([]byte, error) {
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

		if p.Description != nil {
			tmp.Description = fmt.Sprintf("%s %s", tmp.Description, *p.Description)
		} else {
			// save FieldPaths into description
			tmp.Description = fmt.Sprintf("The value will be applied to fields: [%s].", strings.Join(p.FieldPaths, ","))
		}
		properties[p.Name] = tmp
	}
	return generateJSONSchemaWithRequiredProperty(properties, required)
}

// StoreOpenAPISchema stores OpenAPI v3 schema in ConfigMap from WorkloadDefinition
func (def *CapabilityComponentDefinition) StoreOpenAPISchema(ctx context.Context, k8sClient client.Client,
	pd *packages.PackageDiscover, namespace, name, revName string) (string, error) {
	var jsonSchema []byte
	var err error
	switch def.WorkloadType {
	case util.HELMDef:
		jsonSchema, err = helm.GetChartValuesJSONSchema(ctx, def.Helm)
	case util.KubeDef:
		jsonSchema, err = GetKubeSchematicOpenAPISchema(def.Kube.Parameters)
	case util.TerraformDef:
		if def.Terraform == nil {
			return "", fmt.Errorf("no Configuration is set in Terraform specification: %s", def.Name)
		}
		configuration := def.Terraform.Configuration
		if def.Terraform.Type == "remote" {
			configuration, err = GetTerraformConfigurationFromRemote(def.Name, def.Terraform.Configuration, def.Terraform.Path)
			if err != nil {
				return "", fmt.Errorf("cannot get Terraform configuration %s from remote: %w", def.Name, err)
			}
		}
		jsonSchema, err = GetOpenAPISchemaFromTerraformComponentDefinition(configuration)
	default:
		jsonSchema, err = def.GetOpenAPISchema(pd, name)
	}
	if err != nil {
		return "", fmt.Errorf("failed to generate OpenAPI v3 JSON schema for capability %s: %w", def.Name, err)
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
	cmName, err := def.CreateOrUpdateConfigMap(ctx, k8sClient, namespace, componentDefinition.Name, typeComponentDefinition, componentDefinition.Labels, nil, jsonSchema, ownerReference)
	if err != nil {
		return cmName, err
	}

	// Create a configmap to store parameter for each definitionRevision
	defRev := new(v1beta1.DefinitionRevision)
	if err = k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: revName}, defRev); err != nil {
		return "", err
	}
	ownerReference = []metav1.OwnerReference{{
		APIVersion:         defRev.APIVersion,
		Kind:               defRev.Kind,
		Name:               defRev.Name,
		UID:                defRev.GetUID(),
		Controller:         pointer.BoolPtr(true),
		BlockOwnerDeletion: pointer.BoolPtr(true),
	}}
	_, err = def.CreateOrUpdateConfigMap(ctx, k8sClient, namespace, revName, typeComponentDefinition, defRev.Spec.ComponentDefinition.Labels, nil, jsonSchema, ownerReference)
	if err != nil {
		return cmName, err
	}
	return cmName, nil
}

// CapabilityTraitDefinition is the Capability struct for TraitDefinition
type CapabilityTraitDefinition struct {
	Name            string                  `json:"name"`
	TraitDefinition v1beta1.TraitDefinition `json:"traitDefinition"`

	DefCategoryType util.WorkloadType `json:"defCategoryType"`

	Kube *commontypes.Kube `json:"kube"`

	CapabilityBaseDefinition
}

// NewCapabilityTraitDef will create a CapabilityTraitDefinition
func NewCapabilityTraitDef(traitdefinition *v1beta1.TraitDefinition) CapabilityTraitDefinition {
	var def CapabilityTraitDefinition
	def.Name = traitdefinition.Name //  or def.Name = req.NamespacedName.Name
	if traitdefinition.Spec.Schematic != nil && traitdefinition.Spec.Schematic.KUBE != nil {
		def.DefCategoryType = util.KubeDef
		def.Kube = traitdefinition.Spec.Schematic.KUBE
	}
	def.TraitDefinition = *traitdefinition.DeepCopy()
	return def
}

// GetOpenAPISchema gets OpenAPI v3 schema by TraitDefinition name
func (def *CapabilityTraitDefinition) GetOpenAPISchema(pd *packages.PackageDiscover, name string) ([]byte, error) {
	capability, err := appfile.ConvertTemplateJSON2Object(name, def.TraitDefinition.Spec.Extension, def.TraitDefinition.Spec.Schematic)
	if err != nil {
		return nil, fmt.Errorf("failed to convert WorkloadDefinition to Capability Object")
	}
	return getOpenAPISchema(capability, pd)
}

// StoreOpenAPISchema stores OpenAPI v3 schema from TraitDefinition in ConfigMap
func (def *CapabilityTraitDefinition) StoreOpenAPISchema(ctx context.Context, k8sClient client.Client, pd *packages.PackageDiscover, namespace, name string, revName string) (string, error) {
	var jsonSchema []byte
	var err error
	switch def.DefCategoryType {
	case util.KubeDef: // Kube template
		jsonSchema, err = GetKubeSchematicOpenAPISchema(def.Kube.Parameters)
	default: // CUE  template
		jsonSchema, err = def.GetOpenAPISchema(pd, name)
	}
	if err != nil {
		return "", fmt.Errorf("failed to generate OpenAPI v3 JSON schema for capability %s: %w", def.Name, err)
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
	cmName, err := def.CreateOrUpdateConfigMap(ctx, k8sClient, namespace, traitDefinition.Name, typeTraitDefinition, traitDefinition.Labels, traitDefinition.Spec.AppliesToWorkloads, jsonSchema, ownerReference)
	if err != nil {
		return cmName, err
	}

	// Create a configmap to store parameter for each definitionRevision
	defRev := new(v1beta1.DefinitionRevision)
	if err = k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: revName}, defRev); err != nil {
		return "", err
	}
	ownerReference = []metav1.OwnerReference{{
		APIVersion:         defRev.APIVersion,
		Kind:               defRev.Kind,
		Name:               defRev.Name,
		UID:                defRev.GetUID(),
		Controller:         pointer.BoolPtr(true),
		BlockOwnerDeletion: pointer.BoolPtr(true),
	}}
	_, err = def.CreateOrUpdateConfigMap(ctx, k8sClient, namespace, revName, typeTraitDefinition, defRev.Spec.TraitDefinition.Labels, defRev.Spec.TraitDefinition.Spec.AppliesToWorkloads, jsonSchema, ownerReference)
	if err != nil {
		return cmName, err
	}
	return cmName, nil
}

// CapabilityStepDefinition is the Capability struct for WorkflowStepDefinition
type CapabilityStepDefinition struct {
	Name           string                         `json:"name"`
	StepDefinition v1beta1.WorkflowStepDefinition `json:"stepDefinition"`

	CapabilityBaseDefinition
}

// NewCapabilityStepDef will create a CapabilityStepDefinition
func NewCapabilityStepDef(stepdefinition *v1beta1.WorkflowStepDefinition) CapabilityStepDefinition {
	var def CapabilityStepDefinition
	def.Name = stepdefinition.Name
	def.StepDefinition = *stepdefinition.DeepCopy()
	return def
}

// GetOpenAPISchema gets OpenAPI v3 schema by StepDefinition name
func (def *CapabilityStepDefinition) GetOpenAPISchema(pd *packages.PackageDiscover, name string) ([]byte, error) {
	capability, err := appfile.ConvertTemplateJSON2Object(name, nil, def.StepDefinition.Spec.Schematic)
	if err != nil {
		return nil, fmt.Errorf("failed to convert WorkflowStepDefinition to Capability Object")
	}
	return getOpenAPISchema(capability, pd)
}

// StoreOpenAPISchema stores OpenAPI v3 schema from StepDefinition in ConfigMap
func (def *CapabilityStepDefinition) StoreOpenAPISchema(ctx context.Context, k8sClient client.Client, pd *packages.PackageDiscover, namespace, name string, revName string) (string, error) {
	var jsonSchema []byte
	var err error

	jsonSchema, err = def.GetOpenAPISchema(pd, name)
	if err != nil {
		return "", fmt.Errorf("failed to generate OpenAPI v3 JSON schema for capability %s: %w", def.Name, err)
	}

	stepDefinition := def.StepDefinition
	ownerReference := []metav1.OwnerReference{{
		APIVersion:         stepDefinition.APIVersion,
		Kind:               stepDefinition.Kind,
		Name:               stepDefinition.Name,
		UID:                stepDefinition.GetUID(),
		Controller:         pointer.BoolPtr(true),
		BlockOwnerDeletion: pointer.BoolPtr(true),
	}}
	cmName, err := def.CreateOrUpdateConfigMap(ctx, k8sClient, namespace, stepDefinition.Name, typeWorkflowStepDefinition, stepDefinition.Labels, nil, jsonSchema, ownerReference)
	if err != nil {
		return cmName, err
	}

	// Create a configmap to store parameter for each definitionRevision
	defRev := new(v1beta1.DefinitionRevision)
	if err = k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: revName}, defRev); err != nil {
		return "", err
	}
	ownerReference = []metav1.OwnerReference{{
		APIVersion:         defRev.APIVersion,
		Kind:               defRev.Kind,
		Name:               defRev.Name,
		UID:                defRev.GetUID(),
		Controller:         pointer.BoolPtr(true),
		BlockOwnerDeletion: pointer.BoolPtr(true),
	}}
	_, err = def.CreateOrUpdateConfigMap(ctx, k8sClient, namespace, revName, typeWorkflowStepDefinition, defRev.Spec.WorkflowStepDefinition.Labels, nil, jsonSchema, ownerReference)
	if err != nil {
		return cmName, err
	}
	return cmName, nil
}

// CapabilityPolicyDefinition is the Capability struct for PolicyDefinition
type CapabilityPolicyDefinition struct {
	Name             string                   `json:"name"`
	PolicyDefinition v1beta1.PolicyDefinition `json:"policyDefinition"`

	CapabilityBaseDefinition
}

// NewCapabilityPolicyDef will create a CapabilityPolicyDefinition
func NewCapabilityPolicyDef(policydefinition *v1beta1.PolicyDefinition) CapabilityPolicyDefinition {
	var def CapabilityPolicyDefinition
	def.Name = policydefinition.Name
	def.PolicyDefinition = *policydefinition.DeepCopy()
	return def
}

// GetOpenAPISchema gets OpenAPI v3 schema by StepDefinition name
func (def *CapabilityPolicyDefinition) GetOpenAPISchema(pd *packages.PackageDiscover, name string) ([]byte, error) {
	capability, err := appfile.ConvertTemplateJSON2Object(name, nil, def.PolicyDefinition.Spec.Schematic)
	if err != nil {
		return nil, fmt.Errorf("failed to convert WorkflowStepDefinition to Capability Object")
	}
	return getOpenAPISchema(capability, pd)
}

// StoreOpenAPISchema stores OpenAPI v3 schema from StepDefinition in ConfigMap
func (def *CapabilityPolicyDefinition) StoreOpenAPISchema(ctx context.Context, k8sClient client.Client,
	pd *packages.PackageDiscover, namespace, name, revName string) (string, error) {
	var jsonSchema []byte
	var err error

	jsonSchema, err = def.GetOpenAPISchema(pd, name)
	if err != nil {
		return "", fmt.Errorf("failed to generate OpenAPI v3 JSON schema for capability %s: %w", def.Name, err)
	}

	policyDefinition := def.PolicyDefinition
	ownerReference := []metav1.OwnerReference{{
		APIVersion:         policyDefinition.APIVersion,
		Kind:               policyDefinition.Kind,
		Name:               policyDefinition.Name,
		UID:                policyDefinition.GetUID(),
		Controller:         pointer.BoolPtr(true),
		BlockOwnerDeletion: pointer.BoolPtr(true),
	}}
	cmName, err := def.CreateOrUpdateConfigMap(ctx, k8sClient, namespace, policyDefinition.Name, typePolicyStepDefinition, policyDefinition.Labels, nil, jsonSchema, ownerReference)
	if err != nil {
		return cmName, err
	}

	// Create a configmap to store parameter for each definitionRevision
	defRev := new(v1beta1.DefinitionRevision)
	if err = k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: revName}, defRev); err != nil {
		return "", err
	}
	ownerReference = []metav1.OwnerReference{{
		APIVersion:         defRev.APIVersion,
		Kind:               defRev.Kind,
		Name:               defRev.Name,
		UID:                defRev.GetUID(),
		Controller:         pointer.BoolPtr(true),
		BlockOwnerDeletion: pointer.BoolPtr(true),
	}}
	_, err = def.CreateOrUpdateConfigMap(ctx, k8sClient, namespace, revName, typePolicyStepDefinition, defRev.Spec.PolicyDefinition.Labels, nil, jsonSchema, ownerReference)
	if err != nil {
		return cmName, err
	}
	return cmName, nil
}

// CapabilityBaseDefinition is the base struct for CapabilityWorkloadDefinition and CapabilityTraitDefinition
type CapabilityBaseDefinition struct {
}

// CreateOrUpdateConfigMap creates ConfigMap to store OpenAPI v3 schema or or updates data in ConfigMap
func (def *CapabilityBaseDefinition) CreateOrUpdateConfigMap(ctx context.Context, k8sClient client.Client, namespace,
	definitionName, definitionType string, labels map[string]string, appliedWorkloads []string, jsonSchema []byte, ownerReferences []metav1.OwnerReference) (string, error) {
	cmName := fmt.Sprintf("%s-%s%s", definitionType, types.CapabilityConfigMapNamePrefix, definitionName)
	var cm v1.ConfigMap
	var data = map[string]string{
		types.OpenapiV3JSONSchema: string(jsonSchema),
	}
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[types.LabelDefinition] = "schema"
	labels[types.LabelDefinitionName] = definitionName
	annotations := make(map[string]string)
	if appliedWorkloads != nil {
		annotations[types.AnnoDefinitionAppliedWorkloads] = strings.Join(appliedWorkloads, ",")
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
				Labels:          labels,
				Annotations:     annotations,
			},
			Data: data,
		}
		err = k8sClient.Create(ctx, &cm)
		if err != nil {
			return cmName, fmt.Errorf(util.ErrUpdateCapabilityInConfigMap, definitionName, err)
		}
		klog.InfoS("Successfully stored Capability Schema in ConfigMap", "configMap", klog.KRef(namespace, cmName))
		return cmName, nil
	}

	cm.Data = data
	cm.Labels = labels
	cm.Annotations = annotations
	if err = k8sClient.Update(ctx, &cm); err != nil {
		return cmName, fmt.Errorf(util.ErrUpdateCapabilityInConfigMap, definitionName, err)
	}
	klog.InfoS("Successfully update Capability Schema in ConfigMap", "configMap", klog.KRef(namespace, cmName))
	return cmName, nil
}

// getOpenAPISchema is the main function for GetDefinition API
func getOpenAPISchema(capability types.Capability, pd *packages.PackageDiscover) ([]byte, error) {
	openAPISchema, err := generateOpenAPISchemaFromCapabilityParameter(capability, pd)
	if err != nil {
		return nil, err
	}
	schema, err := ConvertOpenAPISchema2SwaggerObject(openAPISchema)
	if err != nil {
		return nil, err
	}
	FixOpenAPISchema("", schema)

	parameter, err := schema.MarshalJSON()
	if err != nil {
		return nil, err
	}
	return parameter, nil
}

// generateOpenAPISchemaFromCapabilityParameter returns the parameter of a definition in cue.Value format
func generateOpenAPISchemaFromCapabilityParameter(capability types.Capability, pd *packages.PackageDiscover) ([]byte, error) {
	template, err := PrepareParameterCue(capability.Name, capability.CueTemplate)
	if err != nil {
		if errors.As(err, &ErrNoSectionParameterInCue{}) {
			// return OpenAPI with empty object parameter, making it possible to generate ConfigMap
			var r cue.Runtime
			cueInst, _ := r.Compile("-", "")
			return common.GenOpenAPI(cueInst)
		}
		return nil, err
	}

	template += velacue.BaseTemplate
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

// PrepareParameterCue cuts `parameter` section form definition .cue file
func PrepareParameterCue(capabilityName, capabilityTemplate string) (string, error) {
	var template string
	var withParameterFlag bool
	r := regexp.MustCompile(`[[:space:]]*parameter:[[:space:]]*`)
	trimRe := regexp.MustCompile(`\s+`)

	for _, text := range strings.Split(capabilityTemplate, "\n") {
		if r.MatchString(text) {
			// a variable has to be refined as a definition which starts with "#"
			// text may be start with space or tab, we should clean up text
			text = fmt.Sprintf("parameter: #parameter\n#%s", trimRe.ReplaceAllString(text, ""))
			withParameterFlag = true
		}
		template += fmt.Sprintf("%s\n", text)
	}

	if !withParameterFlag {
		return "", ErrNoSectionParameterInCue{capName: capabilityName}
	}
	return template, nil
}

// FixOpenAPISchema fixes tainted `description` filed, missing of title `field`.
func FixOpenAPISchema(name string, schema *openapi3.Schema) {
	t := schema.Type
	switch t {
	case "object":
		for k, v := range schema.Properties {
			s := v.Value
			FixOpenAPISchema(k, s)
		}
	case "array":
		if schema.Items != nil {
			FixOpenAPISchema("", schema.Items.Value)
		}
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
	swagger, err := openapi3.NewLoader().LoadFromData(data)
	if err != nil {
		return nil, err
	}

	schemaRef, ok := swagger.Components.Schemas[model.ParameterFieldName]
	if !ok {
		return nil, errors.New(util.ErrGenerateOpenAPIV2JSONSchemaForCapability)
	}
	return schemaRef.Value, nil
}
