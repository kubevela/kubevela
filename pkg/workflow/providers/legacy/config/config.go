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

// Package config is the legacy (vela/op style) registration of the config workflow
// steps. The behaviour lives in pkg/workflow/providers/config; this package only maps
// the legacy parameter shape and step names onto it.
package config

import (
	"context"
	_ "embed"

	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"

	"github.com/oam-dev/kubevela/pkg/config"
	configprovider "github.com/oam-dev/kubevela/pkg/workflow/providers/config"
	oamprovidertypes "github.com/oam-dev/kubevela/pkg/workflow/providers/types"
)

const (
	// ProviderName is provider name
	ProviderName = "config"
)

// ErrRequestInvalid means the request is invalid
var ErrRequestInvalid = configprovider.ErrRequestInvalid

// CreateConfigProperties the request body for creating a config
type CreateConfigProperties = configprovider.CreateConfigProperties

// CreateParams is the create params
type CreateParams = oamprovidertypes.OAMParams[CreateConfigProperties]

// CreateConfig creates a config as a Config CR (see configprovider.Create)
func CreateConfig(ctx context.Context, params *CreateParams) (*any, error) {
	return nil, configprovider.Create(ctx, configprovider.DepsOf(params.RuntimeParams), params.Params)
}

// ReadResult is the read result
type ReadResult struct {
	Config map[string]any `json:"config"`
}

// ReadConfig reads the config from either backend
func ReadConfig(ctx context.Context, params *oamprovidertypes.OAMParams[config.NamespacedName]) (*ReadResult, error) {
	content, err := configprovider.Read(ctx, configprovider.DepsOf(params.RuntimeParams), params.Params)
	if err != nil {
		return nil, err
	}
	return &ReadResult{Config: content}, nil
}

// ListVars is the list vars
type ListVars = configprovider.ListVars

// ListResult is the list result
type ListResult struct {
	Configs []map[string]any `json:"configs"`
}

// ListConfig lists the configs of a template from both backends
func ListConfig(ctx context.Context, params *oamprovidertypes.OAMParams[ListVars]) (*ListResult, error) {
	contents, err := configprovider.List(ctx, configprovider.DepsOf(params.RuntimeParams), params.Params)
	if err != nil {
		return nil, err
	}
	return &ListResult{Configs: contents}, nil
}

// DeleteConfig deletes a config from whichever backend holds it
func DeleteConfig(ctx context.Context, params *oamprovidertypes.OAMParams[config.NamespacedName]) (*any, error) {
	return nil, params.RuntimeParams.ConfigFactory.DeleteConfig(ctx, params.Params.Namespace, params.Params.Name)
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
		"create-config": oamprovidertypes.OAMGenericProviderFn[CreateConfigProperties, any](CreateConfig),
		"read-config":   oamprovidertypes.OAMGenericProviderFn[config.NamespacedName, ReadResult](ReadConfig),
		"list-config":   oamprovidertypes.OAMGenericProviderFn[ListVars, ListResult](ListConfig),
		"delete-config": oamprovidertypes.OAMGenericProviderFn[config.NamespacedName, any](DeleteConfig),
	}
}
