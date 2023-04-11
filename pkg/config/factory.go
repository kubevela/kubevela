/*
Copyright 2022 The KubeVela Authors.

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

package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	velacuex "github.com/oam-dev/kubevela/pkg/cue/cuex"

	cuelang "cuelang.org/go/cue"
	"github.com/kubevela/pkg/cue/cuex"

	"github.com/getkin/kin-openapi/openapi3"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	pkgtypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	icontext "github.com/oam-dev/kubevela/pkg/config/context"
	"github.com/oam-dev/kubevela/pkg/config/writer"
	"github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/cue/script"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// SaveInputPropertiesKey define the key name for saving the input properties in the secret.
const SaveInputPropertiesKey = "input-properties"

// SaveObjectReferenceKey define the key name for saving the outputs objects reference metadata in the secret.
const SaveObjectReferenceKey = "objects-reference"

// SaveExpandedWriterKey define the key name for saving the expanded writer config
const SaveExpandedWriterKey = "expanded-writer"

// SaveSchemaKey define the key name for saving the API schema
const SaveSchemaKey = "schema"

// SaveTemplateKey define the key name for saving the config-template
const SaveTemplateKey = "template"

// TemplateConfigMapNamePrefix the prefix of the configmap name.
const TemplateConfigMapNamePrefix = "config-template-"

// TemplateValidation define the key name for the config-template validation
const TemplateValidation = SaveTemplateKey + ".validation"

// TemplateOutput define the key name for the config-template output
const TemplateOutput = SaveTemplateKey + ".output"

// TemplateParameter define the key name for the config-template parameter
const TemplateParameter = SaveTemplateKey + ".parameter"

// ErrSensitiveConfig means this config can not be read directly.
var ErrSensitiveConfig = errors.New("the config is sensitive")

// ErrNoConfigOrTarget means the config or the target is empty.
var ErrNoConfigOrTarget = errors.New("you must specify the config name and destination to distribute")

// ErrNotFoundDistribution means the app of the distribution does not exist.
var ErrNotFoundDistribution = errors.New("the distribution does not found")

// ErrConfigExist means the config does exist.
var ErrConfigExist = errors.New("the config does exist")

// ErrConfigNotFound means the config does not exist
var ErrConfigNotFound = errors.New("the config does not exist")

// ErrTemplateNotFound means the template does not exist
var ErrTemplateNotFound = errors.New("the template does not exist")

// ErrChangeTemplate means the template of the config can not be changed
var ErrChangeTemplate = errors.New("the template of the config can not be changed")

// ErrChangeSecretType means the secret type of the config can not be changed
var ErrChangeSecretType = errors.New("the secret type of the config can not be changed")

// NamespacedName the namespace and name model
type NamespacedName struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// Template This is the spec of the config template, parse from the cue script.
type Template struct {
	NamespacedName
	Alias       string `json:"alias,omitempty"`
	Description string `json:"description,omitempty"`
	// Scope defines the usage scope of the configuration template. Provides two options: System or Namespace
	// System: The system users could use this template, and the config secret will save in the vela-system namespace.
	// Namespace: The config secret will save in the target namespace, such as this namespace belonging to one project.
	Scope string `json:"scope"`
	// Sensitive means this config config can not be read from the API or the workflow step, only support the safe way, such as Secret.
	Sensitive bool `json:"sensitive"`

	CreateTime time.Time `json:"createTime"`

	Template script.CUE `json:"template"`

	ExpandedWriter writer.ExpandedWriterConfig `json:"expandedWriter"`

	Schema *openapi3.Schema `json:"schema"`

	ConfigMap *v1.ConfigMap `json:"-"`
}

// Metadata users should provide this model.
type Metadata struct {
	NamespacedName
	Alias       string                 `json:"alias,omitempty"`
	Description string                 `json:"description,omitempty"`
	Properties  map[string]interface{} `json:"properties"`
}

// Config this is the config model, generated from the template and properties.
type Config struct {
	Metadata
	CreateTime time.Time
	Template   Template `json:"template"`
	// Secret this is default output way.
	Secret *v1.Secret `json:"secret"`

	// ExpandedWriterData
	ExpandedWriterData *writer.ExpandedWriterData `json:"expandedWriterData"`

	// OutputObjects this means users could define other objects.
	// This field assign value only on config render stage.
	OutputObjects map[string]*unstructured.Unstructured

	// ObjectReferences correspond OutputObjects
	ObjectReferences []v1.ObjectReference

	Targets []*ClusterTargetStatus
}

// ClusterTargetStatus merge the status of the distribution
type ClusterTargetStatus struct {
	ClusterTarget
	Status      string         `json:"status"`
	Application NamespacedName `json:"application"`
	Message     string         `json:"message"`
}

// ClusterTarget kubernetes delivery target
type ClusterTarget struct {
	ClusterName string `json:"clusterName"`
	Namespace   string `json:"namespace"`
}

// Distribution the config distribution model
type Distribution struct {
	Name        string                  `json:"name"`
	Namespace   string                  `json:"namespace"`
	CreatedTime time.Time               `json:"createdTime"`
	Configs     []*NamespacedName       `json:"configs"`
	Targets     []*ClusterTarget        `json:"targets"`
	Application pkgtypes.NamespacedName `json:"application"`
	Status      common.AppStatus        `json:"status"`
}

// CreateDistributionSpec the spec of the distribution
type CreateDistributionSpec struct {
	Configs []*NamespacedName
	Targets []*ClusterTarget
}

// Validation the response of the validation
type Validation struct {
	Result  bool   `json:"result"`
	Message string `json:"message"`
}

// Error return the error message
func (e *Validation) Error() string {
	return fmt.Sprintf("failed to validate config: %s", e.Message)
}

// Factory handle the config
type Factory interface {
	ParseTemplate(defaultName string, content []byte) (*Template, error)
	ParseConfig(ctx context.Context, template NamespacedName, meta Metadata) (*Config, error)

	LoadTemplate(ctx context.Context, name, ns string) (*Template, error)
	CreateOrUpdateConfigTemplate(ctx context.Context, ns string, it *Template) error
	DeleteTemplate(ctx context.Context, ns, name string) error
	ListTemplates(ctx context.Context, ns, scope string) ([]*Template, error)

	ReadConfig(ctx context.Context, namespace, name string) (map[string]interface{}, error)
	GetConfig(ctx context.Context, namespace, name string, withStatus bool) (*Config, error)
	ListConfigs(ctx context.Context, namespace, template, scope string, withStatus bool) ([]*Config, error)
	DeleteConfig(ctx context.Context, namespace, name string) error
	CreateOrUpdateConfig(ctx context.Context, i *Config, ns string) error
	IsExist(ctx context.Context, namespace, name string) (bool, error)

	CreateOrUpdateDistribution(ctx context.Context, ns, name string, ads *CreateDistributionSpec) error
	ListDistributions(ctx context.Context, ns string) ([]*Distribution, error)
	DeleteDistribution(ctx context.Context, ns, name string) error
	MergeDistributionStatus(ctx context.Context, config *Config, namespace string) error
}

// Dispatcher is a client for apply resources.
type Dispatcher func(context.Context, []*unstructured.Unstructured, []apply.ApplyOption) error

// NewConfigFactory create a config factory instance
func NewConfigFactory(cli client.Client) Factory {
	return &kubeConfigFactory{cli: cli, apiApply: defaultDispatcher(cli)}
}

// NewConfigFactoryWithDispatcher create a config factory instance with a specified dispatcher
func NewConfigFactoryWithDispatcher(cli client.Client, ds Dispatcher) Factory {
	if ds == nil {
		ds = defaultDispatcher(cli)
	}
	return &kubeConfigFactory{cli: cli, apiApply: ds}
}

func defaultDispatcher(cli client.Client) Dispatcher {
	api := apply.NewAPIApplicator(cli)
	return func(ctx context.Context, manifests []*unstructured.Unstructured, ao []apply.ApplyOption) error {
		for _, m := range manifests {
			if err := api.Apply(ctx, m, ao...); err != nil {
				return err
			}
		}
		return nil
	}
}

type kubeConfigFactory struct {
	cli      client.Client
	apiApply Dispatcher
}

// ParseTemplate parse a config template instance form the cue script
func (k *kubeConfigFactory) ParseTemplate(defaultName string, content []byte) (*Template, error) {
	cueScript := script.BuildCUEScriptWithDefaultContext(icontext.DefaultContext, content)
	value, err := cueScript.ParseToTemplateValue()
	if err != nil {
		return nil, fmt.Errorf("the cue script is invalid:%w", err)
	}
	name, err := value.GetString("metadata", "name")
	if err != nil {
		if defaultName == "" {
			return nil, fmt.Errorf("fail to get the name from the template metadata: %w", err)
		}
	}
	if defaultName != "" {
		name = defaultName
	}

	templateValue, err := value.LookupValue("template")
	if err != nil {
		return nil, err
	}
	schema, err := cueScript.ParsePropertiesToSchema("template")
	if err != nil {
		return nil, fmt.Errorf("the properties of the cue script is invalid:%w", err)
	}
	alias, err := value.GetString("metadata", "alias")
	if err != nil && !IsFieldNotExist(err) {
		klog.Warningf("fail to get the alias from the template metadata: %s", err.Error())
	}
	scope, err := value.GetString("metadata", "scope")
	if err != nil && !IsFieldNotExist(err) {
		klog.Warningf("fail to get the scope from the template metadata: %s", err.Error())
	}
	sensitive, err := value.GetBool("metadata", "sensitive")
	if err != nil && !IsFieldNotExist(err) {
		klog.Warningf("fail to get the sensitive from the template metadata: %s", err.Error())
	}
	template := &Template{
		NamespacedName: NamespacedName{
			Name: name,
		},
		Alias:          alias,
		Scope:          scope,
		Sensitive:      sensitive,
		Template:       cueScript,
		Schema:         schema,
		ExpandedWriter: writer.ParseExpandedWriterConfig(templateValue),
	}

	var configmap v1.ConfigMap
	configmap.Name = TemplateConfigMapNamePrefix + template.Name

	configmap.Data = map[string]string{
		SaveTemplateKey: string(template.Template),
	}
	if template.Schema != nil {
		data, err := yaml.Marshal(template.Schema)
		if err != nil {
			return nil, err
		}
		configmap.Data[SaveSchemaKey] = string(data)
	}
	data, err := yaml.Marshal(template.ExpandedWriter)
	if err != nil {
		return nil, err
	}
	configmap.Data[SaveExpandedWriterKey] = string(data)
	configmap.Labels = map[string]string{
		types.LabelConfigCatalog: types.VelaCoreConfig,
		types.LabelConfigScope:   template.Scope,
	}
	configmap.Annotations = map[string]string{
		types.AnnotationConfigDescription: template.Description,
		types.AnnotationConfigAlias:       template.Alias,
		types.AnnotationConfigSensitive:   fmt.Sprintf("%t", template.Sensitive),
	}
	template.ConfigMap = &configmap

	return template, nil
}

// IsFieldNotExist check whether the error type is the field not found
func IsFieldNotExist(err error) bool {
	return strings.Contains(err.Error(), "not exist")
}

// CreateOrUpdateConfigTemplate parse and update the config template
func (k *kubeConfigFactory) CreateOrUpdateConfigTemplate(ctx context.Context, ns string, it *Template) error {
	if ns != "" {
		it.ConfigMap.Namespace = ns
	}
	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(it.ConfigMap)
	if err != nil {
		return fmt.Errorf("fail to convert configmap to unstructured: %w", err)
	}
	us := &unstructured.Unstructured{Object: obj}
	us.SetAPIVersion("v1")
	us.SetKind("ConfigMap")
	return k.apiApply(ctx, []*unstructured.Unstructured{us}, []apply.ApplyOption{apply.DisableUpdateAnnotation(), apply.Quiet()})
}

func convertConfigMap2Template(cm v1.ConfigMap) (*Template, error) {
	if cm.Labels == nil || cm.Annotations == nil {
		return nil, fmt.Errorf("this configmap is not a valid config-template")
	}
	it := &Template{
		NamespacedName: NamespacedName{
			Name:      strings.Replace(cm.Name, TemplateConfigMapNamePrefix, "", 1),
			Namespace: cm.Namespace,
		},
		Alias:       cm.Annotations[types.AnnotationConfigAlias],
		Description: cm.Annotations[types.AnnotationConfigDescription],
		Sensitive:   cm.Annotations[types.AnnotationConfigSensitive] == "true",
		Scope:       cm.Labels[types.LabelConfigScope],
		CreateTime:  cm.CreationTimestamp.Time,
		Template:    script.CUE(cm.Data[SaveTemplateKey]),
	}
	if cm.Data[SaveSchemaKey] != "" {
		var schema openapi3.Schema
		err := yaml.Unmarshal([]byte(cm.Data[SaveSchemaKey]), &schema)
		if err != nil {
			return nil, fmt.Errorf("fail to parse the schema: %w", err)
		}
		it.Schema = &schema
	}
	if cm.Data[SaveExpandedWriterKey] != "" {
		var config writer.ExpandedWriterConfig
		err := yaml.Unmarshal([]byte(cm.Data[SaveExpandedWriterKey]), &config)
		if err != nil {
			return nil, fmt.Errorf("fail to parse the schema: %w", err)
		}
		it.ExpandedWriter = config
	}
	return it, nil
}

// DeleteTemplate delete the config template
func (k *kubeConfigFactory) DeleteTemplate(ctx context.Context, ns, name string) error {
	var configmap v1.ConfigMap
	if err := k.cli.Get(ctx, pkgtypes.NamespacedName{Namespace: ns, Name: TemplateConfigMapNamePrefix + name}, &configmap); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("the config template %s not found", name)
		}
		return fmt.Errorf("fail to delete the config template %s:%w", name, err)
	}
	return k.cli.Delete(ctx, &configmap)
}

// ListTemplates list the config templates
func (k *kubeConfigFactory) ListTemplates(ctx context.Context, ns, scope string) ([]*Template, error) {
	var list = &v1.ConfigMapList{}
	selector, err := labels.Parse(fmt.Sprintf("%s=%s", types.LabelConfigCatalog, types.VelaCoreConfig))
	if err != nil {
		return nil, err
	}
	if err := k.cli.List(ctx, list,
		client.MatchingLabelsSelector{Selector: selector},
		client.InNamespace(ns)); err != nil {
		return nil, err
	}
	var templates []*Template
	for _, item := range list.Items {
		it, err := convertConfigMap2Template(item)
		if err != nil {
			klog.Warningf("fail to parse the configmap %s:%s", item.Name, err.Error())
		}
		if it != nil {
			if scope == "" || it.Scope == scope {
				templates = append(templates, it)
			}
		}
	}
	return templates, nil
}

// LoadTemplate load the template
func (k *kubeConfigFactory) LoadTemplate(ctx context.Context, name, ns string) (*Template, error) {
	var cm v1.ConfigMap
	if err := k.cli.Get(ctx, pkgtypes.NamespacedName{Namespace: ns, Name: TemplateConfigMapNamePrefix + name}, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrTemplateNotFound
		}
		return nil, err
	}
	return convertConfigMap2Template(cm)
}

// ParseConfig merge the properties to template and build a config instance
// If the templateName is empty, means creating a secret without the template.
func (k *kubeConfigFactory) ParseConfig(ctx context.Context,
	template NamespacedName, meta Metadata,
) (*Config, error) {
	var secret v1.Secret

	config := &Config{
		Metadata: meta,
		Secret:   &secret,
	}

	if template.Name != "" {
		template, err := k.LoadTemplate(ctx, template.Name, template.Namespace)
		if err != nil {
			return nil, err
		}
		contextValue := icontext.ConfigRenderContext{
			Name:      meta.Name,
			Namespace: meta.Namespace,
		}
		// Compile the config template
		contextOption := cuex.WithExtraData("context", contextValue)
		parameterOption := cuex.WithExtraData(TemplateParameter, meta.Properties)
		val, err := velacuex.KubeVelaDefaultCompiler.Get().CompileStringWithOptions(ctx, string(template.Template), contextOption, parameterOption)
		if err != nil {
			return nil, fmt.Errorf("failed to compile config template: %w", err)
		}
		// Render the validation response and check validation result
		valid := val.LookupPath(cuelang.ParsePath(TemplateValidation))
		validation := Validation{}
		if valid.Exists() {
			if err := valid.Decode(&validation); err != nil {
				return nil, fmt.Errorf("the validation format must be validation")
			}
		}
		if len(validation.Message) > 0 {
			return nil, &validation
		}
		// Render the output secret
		output := val.LookupPath(cuelang.ParsePath(TemplateOutput))
		if output.Exists() {
			if err := output.Decode(&secret); err != nil {
				return nil, fmt.Errorf("the output format must be secret")
			}
		}
		if secret.Type == "" {
			secret.Type = v1.SecretType(fmt.Sprintf("%s/%s", "", template.Name))
		}
		if secret.Labels == nil {
			secret.Labels = map[string]string{}
		}
		secret.Labels[types.LabelConfigCatalog] = types.VelaCoreConfig
		secret.Labels[types.LabelConfigType] = template.Name
		secret.Labels[types.LabelConfigType] = template.Name
		secret.Labels[types.LabelConfigScope] = template.Scope

		if secret.Annotations == nil {
			secret.Annotations = map[string]string{}
		}
		secret.Annotations[types.AnnotationConfigSensitive] = fmt.Sprintf("%t", template.Sensitive)
		secret.Annotations[types.AnnotationConfigTemplateNamespace] = template.Namespace
		config.Template = *template

		// Render the expanded writer configuration
		data, err := writer.RenderForExpandedWriter(template.ExpandedWriter, config.Template.Template, contextValue, meta.Properties)
		if err != nil {
			return nil, fmt.Errorf("fail to render the content for the expanded writer:%w ", err)
		}
		config.ExpandedWriterData = data

		// Render the outputs objects
		outputs, err := template.Template.RunAndOutput(contextValue, meta.Properties, "template", "outputs")
		if err != nil && !cue.IsFieldNotExist(err) {
			return nil, err
		}
		if outputs != nil {
			var objects = map[string]interface{}{}
			if err := outputs.UnmarshalTo(&objects); err != nil {
				return nil, fmt.Errorf("the outputs is invalid %w", err)
			}
			var objectReferences []v1.ObjectReference
			config.OutputObjects = make(map[string]*unstructured.Unstructured)
			for k := range objects {
				if ob, ok := objects[k].(map[string]interface{}); ok {
					obj := &unstructured.Unstructured{Object: ob}
					config.OutputObjects[k] = obj
					objectReferences = append(objectReferences, v1.ObjectReference{
						Kind:       obj.GetKind(),
						Namespace:  obj.GetNamespace(),
						Name:       obj.GetName(),
						APIVersion: obj.GetAPIVersion(),
					})
				}
			}
			objectReferenceJSON, err := json.Marshal(objectReferences)
			if err != nil {
				return nil, err
			}
			if secret.Data == nil {
				secret.Data = map[string][]byte{}
			}
			secret.Data[SaveObjectReferenceKey] = objectReferenceJSON
		}
	} else {
		secret.Labels = map[string]string{
			types.LabelConfigCatalog: types.VelaCoreConfig,
			types.LabelConfigType:    "",
		}
		secret.Annotations = map[string]string{}
	}
	secret.Namespace = meta.Namespace
	if secret.Name == "" {
		secret.Name = meta.Name
	}
	secret.Annotations[types.AnnotationConfigAlias] = meta.Alias
	secret.Annotations[types.AnnotationConfigDescription] = meta.Description
	pp, err := json.Marshal(meta.Properties)
	if err != nil {
		return nil, err
	}
	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}
	secret.Data[SaveInputPropertiesKey] = pp

	return config, nil
}

// ReadConfig read the config secret
func (k *kubeConfigFactory) ReadConfig(ctx context.Context, namespace, name string) (map[string]interface{}, error) {
	var secret v1.Secret
	if err := k.cli.Get(ctx, pkgtypes.NamespacedName{Namespace: namespace, Name: name}, &secret); err != nil {
		return nil, err
	}
	if secret.Annotations[types.AnnotationConfigSensitive] == "true" {
		return nil, ErrSensitiveConfig
	}
	properties := secret.Data[SaveInputPropertiesKey]
	var input = map[string]interface{}{}
	if err := json.Unmarshal(properties, &input); err != nil {
		return nil, err
	}
	return input, nil
}

func (k *kubeConfigFactory) GetConfig(ctx context.Context, namespace, name string, withStatus bool) (*Config, error) {
	var secret v1.Secret
	if err := k.cli.Get(ctx, pkgtypes.NamespacedName{Namespace: namespace, Name: name}, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrConfigNotFound
		}
		return nil, err
	}
	if secret.Annotations[types.AnnotationConfigSensitive] == "true" {
		return nil, ErrSensitiveConfig
	}
	item, err := convertSecret2Config(&secret)
	if err != nil {
		return nil, err
	}
	if withStatus {
		if err := k.MergeDistributionStatus(ctx, item, item.Namespace); err != nil && !errors.Is(err, ErrNotFoundDistribution) {
			klog.Warningf("fail to merge the status %s:%s", item.Name, err.Error())
		}
	}
	return item, nil
}

// CreateOrUpdateConfig create or update the config.
// Write the expand config to the target server.
func (k *kubeConfigFactory) CreateOrUpdateConfig(ctx context.Context, i *Config, ns string) error {
	var secret v1.Secret
	if err := k.cli.Get(ctx, pkgtypes.NamespacedName{Namespace: i.Namespace, Name: i.Name}, &secret); err == nil {
		if secret.Labels[types.LabelConfigType] != i.Template.Name {
			return ErrChangeTemplate
		}
		if secret.Type != i.Secret.Type {
			return ErrChangeSecretType
		}
	}

	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(i.Secret)
	if err != nil {
		return fmt.Errorf("fail to convert secret to unstructured: %w", err)
	}
	us := &unstructured.Unstructured{Object: obj}
	us.SetAPIVersion("v1")
	us.SetKind("Secret")

	if err := k.apiApply(ctx, []*unstructured.Unstructured{us}, []apply.ApplyOption{apply.DisableUpdateAnnotation(), apply.Quiet()}); err != nil {
		return fmt.Errorf("fail to apply the secret: %w", err)
	}
	for key, obj := range i.OutputObjects {
		if err := k.apiApply(ctx, []*unstructured.Unstructured{obj}, []apply.ApplyOption{apply.DisableUpdateAnnotation(), apply.Quiet()}); err != nil {
			return fmt.Errorf("fail to apply the object %s: %w", key, err)
		}
	}
	readConfig := func(ctx context.Context, namespace, name string) (map[string]interface{}, error) {
		return k.ReadConfig(ctx, namespace, name)
	}
	if i.ExpandedWriterData != nil {
		if errs := writer.Write(ctx, i.ExpandedWriterData, readConfig); len(errs) > 0 {
			return errs[0]
		}
	}
	return nil
}

func (k *kubeConfigFactory) IsExist(ctx context.Context, namespace, name string) (bool, error) {
	var secret v1.Secret
	if err := k.cli.Get(ctx, pkgtypes.NamespacedName{Namespace: namespace, Name: name}, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (k *kubeConfigFactory) ListConfigs(ctx context.Context, namespace, template, scope string, withStatus bool) ([]*Config, error) {
	var list = &v1.SecretList{}
	requirement := fmt.Sprintf("%s=%s", types.LabelConfigCatalog, types.VelaCoreConfig)
	if template != "" {
		requirement = fmt.Sprintf("%s,%s=%s", requirement, types.LabelConfigType, template)
	}
	if scope != "" {
		requirement = fmt.Sprintf("%s,%s=%s", requirement, types.LabelConfigScope, scope)
	}
	selector, err := labels.Parse(requirement)
	if err != nil {
		return nil, err
	}
	if err := k.cli.List(ctx, list,
		client.MatchingLabelsSelector{Selector: selector},
		client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	var configs []*Config
	for i := range list.Items {
		item := list.Items[i]
		it, err := convertSecret2Config(&item)
		if err != nil {
			klog.Warningf("fail to parse the secret %s:%s", item.Name, err.Error())
		}
		if it != nil {
			if withStatus {
				if err := k.MergeDistributionStatus(ctx, it, it.Namespace); err != nil && !errors.Is(err, ErrNotFoundDistribution) {
					klog.Warningf("fail to merge the status %s:%s", item.Name, err.Error())
				}
			}
			configs = append(configs, it)
		}
	}
	return configs, nil
}

func (k *kubeConfigFactory) DeleteConfig(ctx context.Context, namespace, name string) error {
	var secret v1.Secret
	if err := k.cli.Get(ctx, pkgtypes.NamespacedName{Namespace: namespace, Name: name}, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("the config %s not found", name)
		}
		return fmt.Errorf("fail to delete the config %s:%w", name, err)
	}
	if secret.Labels[types.LabelConfigCatalog] != types.VelaCoreConfig {
		return fmt.Errorf("found a secret but is not a config")
	}

	if objects, exist := secret.Data[SaveObjectReferenceKey]; exist {
		var objectReferences []v1.ObjectReference
		if err := json.Unmarshal(objects, &objectReferences); err != nil {
			return err
		}
		for _, obj := range objectReferences {
			if err := k.cli.Delete(ctx, convertObjectReference2Unstructured(obj)); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("fail to clear the object %s:%w", obj.Name, err)
			}
		}
	}

	return k.cli.Delete(ctx, &secret)
}

func (k *kubeConfigFactory) MergeDistributionStatus(ctx context.Context, config *Config, namespace string) error {
	app := &v1beta1.Application{}
	if err := k.cli.Get(ctx, pkgtypes.NamespacedName{Namespace: namespace, Name: DefaultDistributionName(config.Name)}, app); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrNotFoundDistribution
		}
		return err
	}
	var targets []*ClusterTargetStatus
	for _, policy := range app.Spec.Policies {
		if policy.Type == v1alpha1.TopologyPolicyType {
			status := workflowv1alpha1.WorkflowStepPhasePending
			message := ""
			if app.Status.Workflow != nil {
				for _, step := range app.Status.Workflow.Steps {
					if policy.Name == strings.Replace(step.Name, "deploy-", "", 1) {
						status = step.Phase
						message = step.Message
					}
				}
			}
			var spec v1alpha1.TopologyPolicySpec
			if err := json.Unmarshal(policy.Properties.Raw, &spec); err == nil {
				for _, clu := range spec.Clusters {
					targets = append(targets, &ClusterTargetStatus{
						ClusterTarget: ClusterTarget{
							Namespace:   spec.Namespace,
							ClusterName: clu,
						},
						Application: NamespacedName{Name: app.Name, Namespace: app.Namespace},
						Status:      string(status),
						Message:     message,
					})
				}
			}
		}
	}
	config.Targets = append(config.Targets, targets...)
	return nil
}

func (k *kubeConfigFactory) CreateOrUpdateDistribution(ctx context.Context, ns, name string, ads *CreateDistributionSpec) error {
	policies := convertTarget2TopologyPolicy(ads.Targets)
	if len(policies) == 0 {
		return ErrNoConfigOrTarget
	}
	// create the share policy
	shareSpec := v1alpha1.SharedResourcePolicySpec{
		Rules: []v1alpha1.SharedResourcePolicyRule{{
			Selector: v1alpha1.ResourcePolicyRuleSelector{
				CompNames: []string{name},
			},
		}},
	}
	properties, err := json.Marshal(shareSpec)
	if err == nil {
		policies = append(policies, v1beta1.AppPolicy{
			Type: v1alpha1.SharedResourcePolicyType,
			Name: "share-config",
			Properties: &runtime.RawExtension{
				Raw: properties,
			},
		})
	}

	var objects []map[string]string
	for _, s := range ads.Configs {
		objects = append(objects, map[string]string{
			"name":      s.Name,
			"namespace": s.Namespace,
			"resource":  "secret",
		})
	}
	if len(objects) == 0 {
		return ErrNoConfigOrTarget
	}

	objectsBytes, err := json.Marshal(map[string][]map[string]string{"objects": objects})
	if err != nil {
		return err
	}

	reqByte, err := json.Marshal(ads)
	if err != nil {
		return err
	}

	distribution := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels: map[string]string{
				types.LabelSourceOfTruth: types.FromInner,
				// This label will override the secret label, then change the catalog of the distributed secrets.
				types.LabelConfigCatalog: types.CatalogConfigDistribution,
			},
			Annotations: map[string]string{
				types.AnnotationConfigDistributionSpec: string(reqByte),
				oam.AnnotationPublishVersion:           util.GenerateVersion("config"),
			},
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{
					Name:       name,
					Type:       v1alpha1.RefObjectsComponentType,
					Properties: &runtime.RawExtension{Raw: objectsBytes},
				},
			},
			Policies: policies,
		},
	}
	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(distribution)
	if err != nil {
		return fmt.Errorf("fail to convert application to unstructured: %w", err)
	}
	us := &unstructured.Unstructured{Object: obj}
	us.SetAPIVersion(v1beta1.SchemeGroupVersion.String())
	us.SetKind(v1beta1.ApplicationKind)

	return k.apiApply(ctx, []*unstructured.Unstructured{us}, []apply.ApplyOption{apply.DisableUpdateAnnotation(), apply.Quiet()})
}

func (k *kubeConfigFactory) ListDistributions(ctx context.Context, ns string) ([]*Distribution, error) {
	var apps v1beta1.ApplicationList
	if err := k.cli.List(ctx, &apps, client.MatchingLabels{
		types.LabelSourceOfTruth: types.FromInner,
		types.LabelConfigCatalog: types.CatalogConfigDistribution,
	}, client.InNamespace(ns)); err != nil {
		return nil, err
	}
	var list []*Distribution
	for _, app := range apps.Items {
		dis := &Distribution{
			Name:        app.Name,
			Namespace:   app.Namespace,
			CreatedTime: app.CreationTimestamp.Time,
			Application: pkgtypes.NamespacedName{
				Namespace: app.Namespace,
				Name:      app.Name,
			},
			Status: app.Status,
		}
		if spec, ok := app.Annotations[types.AnnotationConfigDistributionSpec]; ok {
			var req CreateDistributionSpec
			if err := json.Unmarshal([]byte(spec), &req); err == nil {
				dis.Targets = req.Targets
				dis.Configs = req.Configs
			}
		}
		list = append(list, dis)
	}
	return list, nil
}
func (k *kubeConfigFactory) DeleteDistribution(ctx context.Context, ns, name string) error {
	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
	}
	if err := k.cli.Delete(ctx, app); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrNotFoundDistribution
		}
		return err
	}
	return nil
}

func convertTarget2TopologyPolicy(targets []*ClusterTarget) (policies []v1beta1.AppPolicy) {
	for _, target := range targets {
		policySpec := v1alpha1.TopologyPolicySpec{
			Placement: v1alpha1.Placement{
				Clusters: []string{target.ClusterName},
			},
			Namespace: target.Namespace,
		}
		properties, err := json.Marshal(policySpec)
		if err == nil {
			policies = append(policies, v1beta1.AppPolicy{
				Type: v1alpha1.TopologyPolicyType,
				Name: fmt.Sprintf("%s-%s", target.ClusterName, target.Namespace),
				Properties: &runtime.RawExtension{
					Raw: properties,
				},
			})
		}
	}
	return
}

func convertSecret2Config(se *v1.Secret) (*Config, error) {
	if se == nil || se.Labels == nil {
		return nil, fmt.Errorf("this secret is not a valid config secret")
	}
	config := &Config{
		Metadata: Metadata{
			NamespacedName: NamespacedName{
				Name:      se.Name,
				Namespace: se.Namespace,
			},
		},
		CreateTime: se.CreationTimestamp.Time,
		Secret:     se,
		Template: Template{
			NamespacedName: NamespacedName{
				Name: se.Labels[types.LabelConfigType],
			},
		},
	}
	if se.Annotations != nil {
		config.Alias = se.Annotations[types.AnnotationConfigAlias]
		config.Description = se.Annotations[types.AnnotationConfigDescription]
		config.Template.Namespace = se.Annotations[types.AnnotationConfigTemplateNamespace]
		config.Template.Sensitive = se.Annotations[types.AnnotationConfigSensitive] == "true"
	}
	if !config.Template.Sensitive && len(se.Data[SaveInputPropertiesKey]) > 0 {
		var properties = map[string]interface{}{}
		if err := yaml.Unmarshal(se.Data[SaveInputPropertiesKey], &properties); err != nil {
			return nil, err
		}
		config.Properties = properties
	}
	if !config.Template.Sensitive {
		config.Secret = se
	} else {
		seCope := se.DeepCopy()
		seCope.Data = nil
		seCope.StringData = nil
		config.Secret = seCope
	}
	if content, ok := se.Data[SaveObjectReferenceKey]; ok {
		var objectReferences []v1.ObjectReference
		if err := json.Unmarshal(content, &objectReferences); err != nil {
			klog.Warningf("the object references are invalid, config:%s", se.Name)
		}
		config.ObjectReferences = objectReferences
	}
	return config, nil
}

func convertObjectReference2Unstructured(ref v1.ObjectReference) *unstructured.Unstructured {
	var obj unstructured.Unstructured
	obj.SetAPIVersion(ref.APIVersion)
	obj.SetNamespace(ref.Namespace)
	obj.SetKind(ref.Kind)
	obj.SetName(ref.Name)
	return &obj
}

// DefaultDistributionName generate the distribution name by a config name
func DefaultDistributionName(configName string) string {
	return fmt.Sprintf("distribute-%s", configName)
}
