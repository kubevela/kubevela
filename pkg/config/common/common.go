package common

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/oam-dev/kubevela/apis/types"
	v1 "k8s.io/api/core/v1"
	pkgtypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SaveInputPropertiesKey define the key name for saving the input properties in the secret.
const SaveInputPropertiesKey = "input-properties"

// ErrSensitiveConfig means this config can not be read directly.
var ErrSensitiveConfig = errors.New("the config is sensitive")

// TemplateConfigMapNamePrefix the prefix of the configmap name.
const TemplateConfigMapNamePrefix = "config-template-"

// SaveObjectReferenceKey define the key name for saving the outputs objects reference metadata in the secret.
const SaveObjectReferenceKey = "objects-reference"

// SaveExpandedWriterKey define the key name for saving the expanded writer config
const SaveExpandedWriterKey = "expanded-writer"

// SaveSchemaKey define the key name for saving the API schema
const SaveSchemaKey = "schema"

// SaveTemplateKey define the key name for saving the config-template
const SaveTemplateKey = "template"

// TemplateValidationReturns define the key name for the config-template validation returns
const TemplateValidationReturns = SaveTemplateKey + ".validation.$returns"

// TemplateOutput define the key name for the config-template output
const TemplateOutput = SaveTemplateKey + ".output"

// TemplateOutputs define the key name for the config-template outputs
const TemplateOutputs = SaveTemplateKey + ".outputs"

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

func ReadConfig(ctx context.Context, client client.Client, namespace string, name string) (map[string]interface{}, error) {
	var secret v1.Secret
	if err := client.Get(ctx, pkgtypes.NamespacedName{Namespace: namespace, Name: name}, &secret); err != nil {
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
