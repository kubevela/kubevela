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
	"os"
	"path/filepath"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-git/go-git/v5"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commontypes "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/appfile/helm"
	"github.com/oam-dev/kubevela/pkg/cue/script"
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

const (
	// GitCredsKnownHosts is a key in git credentials secret
	GitCredsKnownHosts string = "known_hosts"
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
func (def *CapabilityComponentDefinition) GetOpenAPISchema(name string) ([]byte, error) {
	capability, err := appfile.ConvertTemplateJSON2Object(name, def.ComponentDefinition.Spec.Extension, def.ComponentDefinition.Spec.Schematic)
	if err != nil {
		return nil, fmt.Errorf("failed to convert ComponentDefinition to Capability Object")
	}
	return getOpenAPISchema(capability)
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
func GetTerraformConfigurationFromRemote(name, remoteURL, remotePath string, sshPublicKey *gitssh.PublicKeys) (string, error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	cachePath := filepath.Join(userHome, ".vela", "terraform", name)
	// Check if the directory exists. If yes, remove it.
	entities, err := os.ReadDir(cachePath)
	if err != nil || len(entities) == 0 {
		fmt.Printf("loading terraform module %s into %s from %s\n", name, cachePath, remoteURL)
		cloneOptions := &git.CloneOptions{
			URL:      remoteURL,
			Progress: os.Stdout,
		}
		if sshPublicKey != nil {
			cloneOptions.Auth = sshPublicKey
		}
		if _, err = git.PlainClone(cachePath, false, cloneOptions); err != nil {
			return "", err
		}
	}
	sshKnownHostsPath := os.Getenv("SSH_KNOWN_HOSTS")
	_ = os.Remove(sshKnownHostsPath)

	tfPath := filepath.Join(cachePath, remotePath, "variables.tf")
	if _, err := os.Stat(tfPath); err != nil {
		tfPath = filepath.Join(cachePath, remotePath, "main.tf")
		if _, err := os.Stat(tfPath); err != nil {
			return "", errors.Wrap(err, "failed to find main.tf or variables.tf in Terraform configurations of the remote repository")
		}
	}
	conf, err := os.ReadFile(filepath.Clean(tfPath))
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

// GetGitSSHPublicKey gets a kubernetes secret containing the SSH private key based on gitCredentialsSecretReference parameters for component and trait definition
func GetGitSSHPublicKey(ctx context.Context, k8sClient client.Client, gitCredentialsSecretReference *v1.SecretReference) (*gitssh.PublicKeys, error) {
	gitCredentialsSecretName := gitCredentialsSecretReference.Name
	gitCredentialsSecretNamespace := gitCredentialsSecretReference.Namespace
	gitCredentialsNamespacedName := k8stypes.NamespacedName{Namespace: gitCredentialsSecretNamespace, Name: gitCredentialsSecretName}

	secret := &v1.Secret{}
	err := k8sClient.Get(ctx, gitCredentialsNamespacedName, secret)
	if err != nil {
		return nil, fmt.Errorf("failed to  get git credentials secret: %w", err)
	}
	needSecretKeys := []string{GitCredsKnownHosts, v1.SSHAuthPrivateKey}
	for _, key := range needSecretKeys {
		if _, ok := secret.Data[key]; !ok {
			err := errors.Errorf("'%s' not in git credentials secret", key)
			return nil, err
		}
	}

	klog.InfoS("Reconcile gitCredentialsSecretReference", "gitCredentialsSecretReference", klog.KRef(gitCredentialsSecretNamespace, gitCredentialsSecretName))

	sshPrivateKey := secret.Data[v1.SSHAuthPrivateKey]
	publicKey, err := gitssh.NewPublicKeys("git", sshPrivateKey, "")
	if err != nil {
		return nil, fmt.Errorf("failed to generate public key from private key: %w", err)
	}
	sshKnownHosts := secret.Data[GitCredsKnownHosts]
	sshDir := filepath.Join(os.TempDir(), ".ssh")
	sshKnownHostsPath := filepath.Join(sshDir, GitCredsKnownHosts)
	_ = os.Mkdir(sshDir, 0700)
	err = os.WriteFile(sshKnownHostsPath, sshKnownHosts, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to write known hosts into file: %w", err)
	}
	_ = os.Setenv("SSH_KNOWN_HOSTS", sshKnownHostsPath)
	return publicKey, nil
}

// StoreOpenAPISchema stores OpenAPI v3 schema in ConfigMap from WorkloadDefinition
func (def *CapabilityComponentDefinition) StoreOpenAPISchema(ctx context.Context, k8sClient client.Client, namespace, name, revName string) (string, error) {
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
			var publicKey *gitssh.PublicKeys
			publicKey = nil
			if def.Terraform.GitCredentialsSecretReference != nil {
				gitCredentialsSecretReference := def.Terraform.GitCredentialsSecretReference
				publicKey, err = GetGitSSHPublicKey(ctx, k8sClient, gitCredentialsSecretReference)
				if err != nil {
					return "", fmt.Errorf("issue with gitCredentialsSecretReference %s/%s: %w", gitCredentialsSecretReference.Namespace, gitCredentialsSecretReference.Name, err)
				}
			}
			configuration, err = GetTerraformConfigurationFromRemote(def.Name, def.Terraform.Configuration, def.Terraform.Path, publicKey)
			if err != nil {
				return "", fmt.Errorf("cannot get Terraform configuration %s from remote: %w", def.Name, err)
			}
		}
		jsonSchema, err = GetOpenAPISchemaFromTerraformComponentDefinition(configuration)
	default:
		jsonSchema, err = def.GetOpenAPISchema(name)
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
		Controller:         pointer.Bool(true),
		BlockOwnerDeletion: pointer.Bool(true),
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
		Controller:         pointer.Bool(true),
		BlockOwnerDeletion: pointer.Bool(true),
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
func (def *CapabilityTraitDefinition) GetOpenAPISchema(name string) ([]byte, error) {
	capability, err := appfile.ConvertTemplateJSON2Object(name, def.TraitDefinition.Spec.Extension, def.TraitDefinition.Spec.Schematic)
	if err != nil {
		return nil, fmt.Errorf("failed to convert WorkloadDefinition to Capability Object")
	}
	return getOpenAPISchema(capability)
}

// StoreOpenAPISchema stores OpenAPI v3 schema from TraitDefinition in ConfigMap
func (def *CapabilityTraitDefinition) StoreOpenAPISchema(ctx context.Context, k8sClient client.Client, namespace, name string, revName string) (string, error) {
	var jsonSchema []byte
	var err error
	switch def.DefCategoryType {
	case util.KubeDef: // Kube template
		jsonSchema, err = GetKubeSchematicOpenAPISchema(def.Kube.Parameters)
	default: // CUE  template
		jsonSchema, err = def.GetOpenAPISchema(name)
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
		Controller:         pointer.Bool(true),
		BlockOwnerDeletion: pointer.Bool(true),
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
		Controller:         pointer.Bool(true),
		BlockOwnerDeletion: pointer.Bool(true),
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
func (def *CapabilityStepDefinition) GetOpenAPISchema(name string) ([]byte, error) {
	capability, err := appfile.ConvertTemplateJSON2Object(name, nil, def.StepDefinition.Spec.Schematic)
	if err != nil {
		return nil, fmt.Errorf("failed to convert WorkflowStepDefinition to Capability Object")
	}
	return getOpenAPISchema(capability)
}

// StoreOpenAPISchema stores OpenAPI v3 schema from StepDefinition in ConfigMap
func (def *CapabilityStepDefinition) StoreOpenAPISchema(ctx context.Context, k8sClient client.Client, namespace, name string, revName string) (string, error) {
	var jsonSchema []byte
	var err error

	jsonSchema, err = def.GetOpenAPISchema(name)
	if err != nil {
		return "", fmt.Errorf("failed to generate OpenAPI v3 JSON schema for capability %s: %w", def.Name, err)
	}

	stepDefinition := def.StepDefinition
	ownerReference := []metav1.OwnerReference{{
		APIVersion:         stepDefinition.APIVersion,
		Kind:               stepDefinition.Kind,
		Name:               stepDefinition.Name,
		UID:                stepDefinition.GetUID(),
		Controller:         pointer.Bool(true),
		BlockOwnerDeletion: pointer.Bool(true),
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
		Controller:         pointer.Bool(true),
		BlockOwnerDeletion: pointer.Bool(true),
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
func (def *CapabilityPolicyDefinition) GetOpenAPISchema(name string) ([]byte, error) {
	capability, err := appfile.ConvertTemplateJSON2Object(name, nil, def.PolicyDefinition.Spec.Schematic)
	if err != nil {
		return nil, fmt.Errorf("failed to convert WorkflowStepDefinition to Capability Object")
	}
	return getOpenAPISchema(capability)
}

// StoreOpenAPISchema stores OpenAPI v3 schema from StepDefinition in ConfigMap
func (def *CapabilityPolicyDefinition) StoreOpenAPISchema(ctx context.Context, k8sClient client.Client, namespace, name, revName string) (string, error) {
	var jsonSchema []byte
	var err error

	jsonSchema, err = def.GetOpenAPISchema(name)
	if err != nil {
		return "", fmt.Errorf("failed to generate OpenAPI v3 JSON schema for capability %s: %w", def.Name, err)
	}

	policyDefinition := def.PolicyDefinition
	ownerReference := []metav1.OwnerReference{{
		APIVersion:         policyDefinition.APIVersion,
		Kind:               policyDefinition.Kind,
		Name:               policyDefinition.Name,
		UID:                policyDefinition.GetUID(),
		Controller:         pointer.Bool(true),
		BlockOwnerDeletion: pointer.Bool(true),
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
		Controller:         pointer.Bool(true),
		BlockOwnerDeletion: pointer.Bool(true),
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
func getOpenAPISchema(capability types.Capability) ([]byte, error) {
	cueTemplate := script.CUE([]byte(capability.CueTemplate))
	schema, err := cueTemplate.ParsePropertiesToSchema()
	if err != nil {
		return nil, err
	}
	klog.Infof("parsed %d properties by %s/%s", len(schema.Properties), capability.Type, capability.Name)
	parameter, err := schema.MarshalJSON()
	if err != nil {
		return nil, err
	}
	return parameter, nil
}
