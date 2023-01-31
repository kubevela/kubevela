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

package provider

import (
	"errors"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"

	monitorContext "github.com/kubevela/pkg/monitor/context"
	wfContext "github.com/kubevela/workflow/pkg/context"
	"github.com/kubevela/workflow/pkg/cue/model/value"
	"github.com/kubevela/workflow/pkg/types"

	"github.com/oam-dev/kubevela/pkg/config"
)

const (
	// ProviderName is provider name
	ProviderName = "config"
)

// ErrRequestInvalid means the request is invalid
var ErrRequestInvalid = errors.New("the request is in valid")

type provider struct {
	factory config.Factory
}

// CreateConfigProperties the request body for creating a config
type CreateConfigProperties struct {
	Name      string                 `json:"name"`
	Namespace string                 `json:"namespace"`
	Template  string                 `json:"template,omitempty"`
	Config    map[string]interface{} `json:"config"`
}

// Response the response body
type Response struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (p *provider) Create(ctx monitorContext.Context, wfCtx wfContext.Context, v *value.Value, act types.Action) error {
	var ccp CreateConfigProperties
	if err := v.UnmarshalTo(&ccp); err != nil {
		return ErrRequestInvalid
	}
	name := ccp.Template
	namespace := "vela-system"
	if strings.Contains(ccp.Template, "/") {
		namespacedName := strings.SplitN(ccp.Template, "/", 2)
		namespace = namespacedName[0]
		name = namespacedName[1]
	}
	configItem, err := p.factory.ParseConfig(ctx.GetContext(), config.NamespacedName{
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
		return err
	}
	return p.factory.CreateOrUpdateConfig(ctx.GetContext(), configItem, ccp.Namespace)
}

func (p *provider) Read(ctx monitorContext.Context, wfCtx wfContext.Context, v *value.Value, act types.Action) error {
	var nn config.NamespacedName
	if err := v.UnmarshalTo(&nn); err != nil {
		return ErrRequestInvalid
	}
	content, err := p.factory.ReadConfig(ctx.GetContext(), nn.Namespace, nn.Name)
	if err != nil {
		return err
	}
	return v.FillObject(content, "config")
}

func (p *provider) List(ctx monitorContext.Context, wfCtx wfContext.Context, v *value.Value, act types.Action) error {
	template, err := v.GetString("template")
	if err != nil {
		return ErrRequestInvalid
	}
	namespace, err := v.GetString("namespace")
	if err != nil {
		return ErrRequestInvalid
	}
	if strings.Contains(template, "/") {
		namespacedName := strings.SplitN(template, "/", 2)
		template = namespacedName[1]
	}
	configs, err := p.factory.ListConfigs(ctx.GetContext(), namespace, template, "", false)
	if err != nil {
		return err
	}
	var contents = []map[string]interface{}{}
	for _, config := range configs {
		contents = append(contents, map[string]interface{}{
			"name":        config.Name,
			"alias":       config.Alias,
			"description": config.Description,
			"config":      config.Properties,
		})
	}
	return v.FillObject(contents, "configs")
}

func (p *provider) Delete(ctx monitorContext.Context, wfCtx wfContext.Context, v *value.Value, act types.Action) error {
	var nn config.NamespacedName
	if err := v.UnmarshalTo(&nn); err != nil {
		return errors.New("the request is in valid")
	}
	return p.factory.DeleteConfig(ctx.GetContext(), nn.Namespace, nn.Name)
}

// Install register handlers to provider discover.
func Install(p types.Providers, cli client.Client, apply config.Dispatcher) {
	prd := &provider{
		factory: config.NewConfigFactoryWithDispatcher(cli, apply),
	}
	p.Register(ProviderName, map[string]types.Handler{
		"create": prd.Create,
		"read":   prd.Read,
		"list":   prd.List,
		"delete": prd.Delete,
	})
}
