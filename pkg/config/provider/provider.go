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
	"context"
	"errors"
	"strings"

	"cuelang.org/go/cue"

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

func (p *provider) Create(ctx context.Context, v cue.Value) (cue.Value, error) {
	var ccp CreateConfigProperties
	if err := v.Decode(&ccp); err != nil {
		return v, ErrRequestInvalid
	}
	name := ccp.Template
	namespace := "vela-system"
	if strings.Contains(ccp.Template, "/") {
		namespacedName := strings.SplitN(ccp.Template, "/", 2)
		namespace = namespacedName[0]
		name = namespacedName[1]
	}
	configItem, err := p.factory.ParseConfig(ctx, config.NamespacedName{
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
		return v, err
	}
	return v, p.factory.CreateOrUpdateConfig(ctx, configItem, ccp.Namespace)
}

func (p *provider) Read(ctx context.Context, v cue.Value) (cue.Value, error) {
	var nn config.NamespacedName
	if err := v.Decode(&nn); err != nil {
		return v, ErrRequestInvalid
	}
	content, err := p.factory.ReadConfig(ctx, nn.Namespace, nn.Name)
	if err != nil {
		return v, err
	}
	v = v.FillPath(cue.ParsePath("config"), content)
	return v, v.Err()
}

func (p *provider) List(ctx context.Context, v cue.Value) (cue.Value, error) {
	template, err := v.LookupPath(cue.ParsePath("template")).String()
	if err != nil {
		return v, ErrRequestInvalid
	}
	namespace, err := v.LookupPath(cue.ParsePath("namespace")).String()
	if err != nil {
		return v, ErrRequestInvalid
	}
	if strings.Contains(template, "/") {
		namespacedName := strings.SplitN(template, "/", 2)
		template = namespacedName[1]
	}
	configs, err := p.factory.ListConfigs(ctx, namespace, template, "", false)
	if err != nil {
		return v, err
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
	v = v.FillPath(cue.ParsePath("configs"), contents)
	return v, v.Err()
}

func (p *provider) Delete(ctx context.Context, v cue.Value) (cue.Value, error) {
	var nn config.NamespacedName
	if err := v.Decode(&nn); err != nil {
		return v, errors.New("the request is in valid")
	}
	return v, p.factory.DeleteConfig(ctx, nn.Namespace, nn.Name)
}

// TODO(somefive) recover
// Install register handlers to provider discover.
// func Install(p types.Providers, cli client.Client, apply config.Dispatcher) {
//	prd := &provider{
//		factory: config.NewConfigFactoryWithDispatcher(cli, apply),
//	}
//	p.Register(ProviderName, map[string]types.Handler{
//		"create": prd.Create,
//		"read":   prd.Read,
//		"list":   prd.List,
//		"delete": prd.Delete,
//	})
// }
