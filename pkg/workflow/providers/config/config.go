/*
Copyright 2023 The KubeVela Authors.

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
	_ "embed"
	"errors"
	"strings"

	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"

	"github.com/oam-dev/kubevela/pkg/config"
	oamprovidertypes "github.com/oam-dev/kubevela/pkg/workflow/providers/types"
)

const (
	// ProviderName is provider name
	ProviderName = "config"
)

// ErrRequestInvalid means the request is invalid
var ErrRequestInvalid = errors.New("the request is in valid")

// CreateConfigProperties the request body for creating a config
type CreateConfigProperties struct {
	Name      string                 `json:"name"`
	Namespace string                 `json:"namespace"`
	Template  string                 `json:"template,omitempty"`
	Config    map[string]interface{} `json:"config"`
}

// CreateParams is the create params
type CreateParams = oamprovidertypes.Params[CreateConfigProperties]

// CreateConfig creates a config
func CreateConfig(ctx context.Context, params *CreateParams) (*any, error) {
	ccp := params.Params
	name := ccp.Template
	namespace := "vela-system"
	if strings.Contains(ccp.Template, "/") {
		namespacedName := strings.SplitN(ccp.Template, "/", 2)
		namespace = namespacedName[0]
		name = namespacedName[1]
	}
	factory := params.ConfigFactory
	configItem, err := factory.ParseConfig(ctx, config.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, config.Metadata{
		NamespacedName: config.NamespacedName{
			Name:      ccp.Name,
			Namespace: ccp.Namespace,
		},
		Properties: ccp.Config,
	})
	if err != nil {
		return nil, err
	}
	return nil, factory.CreateOrUpdateConfig(ctx, configItem, ccp.Namespace)
}

// ReadReturnVars is the read return vars
type ReadReturnVars struct {
	Config map[string]any `json:"config"`
}

// ReadReturns is the read returns
type ReadReturns = oamprovidertypes.Returns[ReadReturnVars]

// ReadConfig reads the config
func ReadConfig(ctx context.Context, params *oamprovidertypes.Params[config.NamespacedName]) (*ReadReturns, error) {
	nn := params.Params
	factory := params.ConfigFactory
	content, err := factory.ReadConfig(ctx, nn.Namespace, nn.Name)
	if err != nil {
		return nil, err
	}
	return &ReadReturns{
		Returns: ReadReturnVars{
			Config: content,
		},
	}, nil
}

// ListVars is the list vars
type ListVars struct {
	Namespace string `json:"namespace"`
	Template  string `json:"template"`
}

// ListReturnVars is the list return vars
type ListReturnVars struct {
	Configs []map[string]any `json:"configs"`
}

// ListReturns is the list returns
type ListReturns = oamprovidertypes.Returns[ListReturnVars]

// ListConfig lists the config
func ListConfig(ctx context.Context, params *oamprovidertypes.Params[ListVars]) (*ListReturns, error) {
	template := params.Params.Template
	namespace := params.Params.Namespace
	if template == "" || namespace == "" {
		return nil, ErrRequestInvalid
	}

	if strings.Contains(template, "/") {
		namespacedName := strings.SplitN(template, "/", 2)
		template = namespacedName[1]
	}
	factory := params.ConfigFactory
	configs, err := factory.ListConfigs(ctx, namespace, template, "", false)
	if err != nil {
		return nil, err
	}
	var contents = []map[string]interface{}{}
	for _, c := range configs {
		contents = append(contents, map[string]interface{}{
			"name":        c.Name,
			"alias":       c.Alias,
			"description": c.Description,
			"config":      c.Properties,
		})
	}
	return &ListReturns{
		Returns: ListReturnVars{
			Configs: contents,
		},
	}, nil
}

// DeleteConfig deletes a config
func DeleteConfig(ctx context.Context, params *oamprovidertypes.Params[config.NamespacedName]) (*any, error) {
	nn := params.Params
	factory := params.ConfigFactory
	return nil, factory.DeleteConfig(ctx, nn.Namespace, nn.Name)
}

//go:embed config.cue
var template string

// GetTemplate returns the cue template.
func GetTemplate() string {
	return template
}

// GetProviders returns the cue providers.
func GetProviders() map[string]cuexruntime.ProviderFn {
	return map[string]cuexruntime.ProviderFn{
		"create-config": oamprovidertypes.GenericProviderFn[CreateConfigProperties, any](CreateConfig),
		"read-config":   oamprovidertypes.GenericProviderFn[config.NamespacedName, ReadReturns](ReadConfig),
		"list-config":   oamprovidertypes.GenericProviderFn[ListVars, ListReturns](ListConfig),
		"delete-config": oamprovidertypes.GenericProviderFn[config.NamespacedName, any](DeleteConfig),
	}
}
