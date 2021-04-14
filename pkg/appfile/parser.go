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

package appfile

import (
	"context"
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile/config"
	velacue "github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/dsl/definition"
	"github.com/oam-dev/kubevela/pkg/dsl/process"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

const (
	// AppfileBuiltinConfig defines the built-in config variable
	AppfileBuiltinConfig = "config"
)

// TemplateLoaderFn load template of a capability definition
type TemplateLoaderFn func(context.Context, discoverymapper.DiscoveryMapper, client.Reader, string, types.CapType) (*Template, error)

// LoadTemplate load template of a capability definition
func (fn TemplateLoaderFn) LoadTemplate(ctx context.Context, dm discoverymapper.DiscoveryMapper, c client.Reader, capName string, capType types.CapType) (*Template, error) {
	return fn(ctx, dm, c, capName, capType)
}

// Parser is an application parser
type Parser struct {
	client     client.Client
	dm         discoverymapper.DiscoveryMapper
	pd         *definition.PackageDiscover
	tmplLoader TemplateLoaderFn
}

// NewApplicationParser create appfile parser
func NewApplicationParser(cli client.Client, dm discoverymapper.DiscoveryMapper, pd *definition.PackageDiscover) *Parser {
	return &Parser{
		client:     cli,
		dm:         dm,
		pd:         pd,
		tmplLoader: LoadTemplate,
	}
}

// NewDryRunApplicationParser create an appfile parser for DryRun
func NewDryRunApplicationParser(cli client.Client, dm discoverymapper.DiscoveryMapper, pd *definition.PackageDiscover, defs []oam.Object) *Parser {
	return &Parser{
		client:     cli,
		dm:         dm,
		pd:         pd,
		tmplLoader: DryRunTemplateLoader(defs),
	}
}

// GenerateAppFile converts an application to an Appfile
func (p *Parser) GenerateAppFile(ctx context.Context, app *v1beta1.Application) (*Appfile, error) {
	ns := app.Namespace
	appName := app.Name

	appfile := new(Appfile)
	appfile.Name = appName
	appfile.Namespace = ns
	var wds []*Workload
	for _, comp := range app.Spec.Components {
		wd, err := p.parseWorkload(ctx, comp, appName, ns)
		if err != nil {
			return nil, err
		}
		wds = append(wds, wd)
	}
	appfile.Workloads = wds
	return appfile, nil
}

// parseWorkload resolve an ApplicationComponent and generate a Workload
// containing ALL information required by an Appfile.
func (p *Parser) parseWorkload(ctx context.Context, comp v1beta1.ApplicationComponent, appName, ns string) (*Workload, error) {
	templ, err := p.tmplLoader.LoadTemplate(ctx, p.dm, p.client, comp.Type, types.TypeComponentDefinition)
	if err != nil && !kerrors.IsNotFound(err) {
		return nil, errors.WithMessagef(err, "fetch type of %s", comp.Name)
	}
	settings, err := util.RawExtension2Map(&comp.Properties)
	if err != nil {
		return nil, errors.WithMessagef(err, "fail to parse settings for %s", comp.Name)
	}
	workload := &Workload{
		Traits:             []*Trait{},
		Name:               comp.Name,
		Type:               comp.Type,
		CapabilityCategory: templ.CapabilityCategory,
		FullTemplate:       templ,
		Params:             settings,
		engine:             definition.NewWorkloadAbstractEngine(comp.Name, p.pd),
	}

	if workload.IsCloudResourceConsumer() {
		requiredSecrets, err := parseWorkloadInsertSecretTo(ctx, p.client, ns, workload)
		if err != nil {
			return nil, err
		}
		workload.RequiredSecrets = requiredSecrets
	}

	userConfig := workload.GetUserConfigName()
	if userConfig != "" {
		cg := config.Configmap{Client: p.client}
		// TODO(wonderflow): envName should not be namespace when we have serverside env
		var envName = ns
		data, err := cg.GetConfigData(config.GenConfigMapName(appName, workload.Name, userConfig), envName)
		if err != nil {
			return nil, errors.Wrapf(err, "get config=%s for app=%s in namespace=%s", userConfig, appName, ns)
		}
		workload.UserConfigs = data
	}

	for _, traitValue := range comp.Traits {
		properties, err := util.RawExtension2Map(&traitValue.Properties)
		if err != nil {
			return nil, errors.Errorf("fail to parse properties of %s for %s", traitValue.Type, comp.Name)
		}
		trait, err := p.parseTrait(ctx, traitValue.Type, properties)
		if err != nil {
			return nil, errors.WithMessagef(err, "component(%s) parse trait(%s)", comp.Name, traitValue.Type)
		}

		workload.Traits = append(workload.Traits, trait)
	}
	for scopeType, instanceName := range comp.Scopes {
		gvk, err := getScopeGVK(ctx, p.client, p.dm, scopeType)
		if err != nil {
			return nil, err
		}
		workload.Scopes = append(workload.Scopes, Scope{
			Name: instanceName,
			GVK:  gvk,
		})
	}
	return workload, nil
}

func (p *Parser) parseTrait(ctx context.Context, name string, properties map[string]interface{}) (*Trait, error) {
	templ, err := p.tmplLoader.LoadTemplate(ctx, p.dm, p.client, name, types.TypeTrait)
	if kerrors.IsNotFound(err) {
		return nil, errors.Errorf("trait definition of %s not found", name)
	}
	if err != nil {
		return nil, err
	}
	return &Trait{
		Name:               name,
		CapabilityCategory: templ.CapabilityCategory,
		Params:             properties,
		Template:           templ.TemplateStr,
		HealthCheckPolicy:  templ.Health,
		CustomStatusFormat: templ.CustomStatus,
		FullTemplate:       templ,
		engine:             definition.NewTraitAbstractEngine(name, p.pd),
	}, nil
}

// GetOutputSecretNames set all secret names, which are generated by cloud resource, to context
func GetOutputSecretNames(workloads *Workload) (string, error) {
	secretName, err := getComponentSetting(process.OutputSecretName, workloads.Params)
	if err != nil {
		return "", err
	}

	return fmt.Sprint(secretName), nil
}

func parseWorkloadInsertSecretTo(ctx context.Context, c client.Client, namespace string, wl *Workload) ([]process.RequiredSecrets, error) {
	var requiredSecret []process.RequiredSecrets
	cueStr := velacue.BaseTemplate + wl.FullTemplate.TemplateStr
	r := cue.Runtime{}
	ins, err := r.Compile("-", cueStr)
	if err != nil {
		return nil, errors.Wrap(err, "cannot compile CUE template")
	}
	params := ins.Lookup("parameter")
	if !params.Exists() {
		return nil, nil
	}
	paramsSt, err := params.Struct()
	if err != nil {
		return nil, errors.Wrap(err, "cannot resolve parameters in CUE template")
	}
	for i := 0; i < paramsSt.Len(); i++ {
		fieldInfo := paramsSt.Field(i)
		fName := fieldInfo.Name
		cgs := fieldInfo.Value.Doc()
		for _, cg := range cgs {
			for _, comment := range cg.List {
				if comment == nil {
					continue
				}
				if strings.Contains(comment.Text, InsertSecretToTag) {
					contextName := strings.Split(comment.Text, InsertSecretToTag)[1]
					contextName = strings.TrimSpace(contextName)
					secretNameInterface, err := getComponentSetting(fName, wl.Params)
					if err != nil {
						return nil, err
					}
					secretName, ok := secretNameInterface.(string)
					if !ok {
						return nil, fmt.Errorf("failed to convert secret name %v to string", secretNameInterface)
					}
					secretData, err := extractSecret(ctx, c, namespace, secretName)
					if err != nil {
						return nil, err
					}
					requiredSecret = append(requiredSecret, process.RequiredSecrets{
						Name:        secretName,
						ContextName: contextName,
						Namespace:   namespace,
						Data:        secretData,
					})
				}
			}
		}

	}
	return requiredSecret, nil
}

func extractSecret(ctx context.Context, c client.Client, namespace, name string) (map[string]interface{}, error) {
	secretData := make(map[string]interface{})
	var secret v1.Secret
	if err := c.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, &secret); err != nil {
		return nil, fmt.Errorf("failed to get secret %s from namespace %s which is required by the component: %w",
			name, namespace, err)
	}
	for k, v := range secret.Data {
		secretData[k] = string(v)
	}
	if len(secretData) == 0 {
		return nil, fmt.Errorf("data in secret %s from namespace %s isn't available", name, namespace)
	}
	return secretData, nil
}

func getComponentSetting(settingParamName string, params map[string]interface{}) (interface{}, error) {
	if secretName, ok := params[settingParamName]; ok {
		return secretName, nil
	}
	return nil, fmt.Errorf("failed to get the value of component setting %s", settingParamName)
}

func getScopeGVK(ctx context.Context, cli client.Reader, dm discoverymapper.DiscoveryMapper,
	name string) (schema.GroupVersionKind, error) {
	var gvk schema.GroupVersionKind
	sd := new(v1alpha2.ScopeDefinition)
	err := util.GetDefinition(ctx, cli, sd, name)
	if err != nil {
		return gvk, err
	}

	return util.GetGVKFromDefinition(dm, sd.Spec.Reference)
}
