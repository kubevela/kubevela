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

package usecase

import (
	"context"
	"sync"

	"github.com/oam-dev/kubevela/pkg/apiserver/model"

	"github.com/pkg/errors"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/definition"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

const (
	labelVal              = "velacore-config"
	definitionAlias       = definition.UserPrefix + "alias.config.oam.dev"
	definitionType        = definition.UserPrefix + "type.config.oam.dev"
	terraformProviderType = "terraform-provider"
	// LabelProject is the label to store project name in an application
	LabelProject = "project"
)

// ConfigHandler handle CRUD of configs
type ConfigHandler interface {
	ListConfigTypes(ctx context.Context, query string) ([]*apis.ConfigType, error)
	GetConfigType(ctx context.Context, configType string) (*apis.ConfigType, error)
	CreateConfig(ctx context.Context, req apis.CreateApplicationRequest) (*apis.ApplicationBase, error)
	GetConfigs(ctx context.Context, configType string) ([]*apis.Config, error)
	GetConfig(ctx context.Context, configType, name string) (*apis.Config, error)
	DeleteConfig(ctx context.Context, configType, name string) error
}

// NewConfigUseCase returns a config use case
func NewConfigUseCase(useCase ApplicationUsecase) ConfigHandler {
	config, err := clients.GetKubeConfig()
	if err != nil {
		panic(err)
	}
	k8sClient, err := clients.GetKubeClient()
	if err != nil {
		panic(err)
	}
	dc, err := clients.GetDiscoveryClient()
	if err != nil {
		panic(err)
	}
	return &defaultConfigHandler{
		applicationUseCase: useCase,
		kubeClient:         k8sClient,
		config:             config,
		apply:              apply.NewAPIApplicator(k8sClient),
		mutex:              new(sync.RWMutex),
		discoveryClient:    dc,
	}
}

type defaultConfigHandler struct {
	kubeClient         client.Client
	config             *rest.Config
	apply              apply.Applicator
	discoveryClient    *discovery.DiscoveryClient
	mutex              *sync.RWMutex
	applicationUseCase ApplicationUsecase
}

// ListConfigTypes returns all config types
func (u *defaultConfigHandler) ListConfigTypes(ctx context.Context, query string) ([]*apis.ConfigType, error) {
	defs := &v1beta1.ComponentDefinitionList{}
	if err := u.kubeClient.List(ctx, defs, client.InNamespace(types.DefaultKubeVelaNS),
		client.MatchingLabels{definition.UserPrefix + "catalog.config.oam.dev": labelVal}); err != nil {
		return nil, err
	}

	var tfDefs []v1beta1.ComponentDefinition
	var types []*apis.ConfigType

	for _, d := range defs.Items {
		if d.Labels[definitionType] == terraformProviderType {
			tfDefs = append(tfDefs, d)
			continue
		}
		types = append(types, &apis.ConfigType{
			Alias:       d.Annotations[definitionAlias],
			Name:        d.Name,
			Definitions: []string{d.Name},
		})
	}

	tfType := &apis.ConfigType{
		Alias: "Terraform Cloud Provider",
		Name:  terraformProviderType,
	}
	definitions := make([]string, len(tfDefs))

	for i, tf := range tfDefs {
		definitions[i] = tf.Name
	}
	tfType.Definitions = definitions

	return append(types, tfType), nil
}

// GetConfigType returns a config type
func (u *defaultConfigHandler) GetConfigType(ctx context.Context, configType string) (*apis.ConfigType, error) {
	d := &v1beta1.ComponentDefinition{}
	if err := u.kubeClient.Get(ctx, client.ObjectKey{Namespace: types.DefaultKubeVelaNS, Name: configType}, d); err != nil {
		return nil, errors.Wrap(err, "failed to get config type")
	}

	t := &apis.ConfigType{
		Alias:       d.Annotations[definitionAlias],
		Name:        configType,
		Description: d.Annotations[types.AnnoDefinitionDescription],
	}
	return t, nil
}

func (u *defaultConfigHandler) CreateConfig(ctx context.Context, req apis.CreateApplicationRequest) (*apis.ApplicationBase, error) {
	return u.applicationUseCase.CreateApplication(ctx, req)
}

func (u *defaultConfigHandler) GetConfigs(ctx context.Context, configType string) ([]*apis.Config, error) {
	var apps = &v1beta1.ApplicationList{}
	if err := u.kubeClient.List(ctx, apps, client.InNamespace(types.DefaultKubeVelaNS),
		client.MatchingLabels{
			model.LabelSourceOfTruth: model.FromInner,
			types.LabelConfigCatalog: "velacore-config",
			types.LabelConfigType:    configType,
		}); err != nil {
		return nil, err
	}

	configs := make([]*apis.Config, len(apps.Items))
	for i, a := range apps.Items {
		configs[i] = &apis.Config{
			ConfigType:  a.Labels[types.LabelConfigType],
			Name:        a.Name,
			Project:     a.Labels[LabelProject],
			CreatedTime: &(a.CreationTimestamp.Time),
		}
	}
	return configs, nil
}

func (u *defaultConfigHandler) GetConfig(ctx context.Context, configType, name string) (*apis.Config, error) {
	var a = &v1beta1.Application{}
	if err := u.kubeClient.Get(ctx, client.ObjectKey{Namespace: types.DefaultKubeVelaNS, Name: name}, a); err != nil {
		return nil, err
	}

	config := &apis.Config{
		ConfigType:  a.Labels[types.LabelConfigType],
		Name:        a.Name,
		Project:     a.Labels[LabelProject],
		CreatedTime: &a.CreationTimestamp.Time,
	}

	return config, nil
}

func (u *defaultConfigHandler) DeleteConfig(ctx context.Context, configType, name string) error {
	var a = &v1beta1.Application{}
	if err := u.kubeClient.Get(ctx, client.ObjectKey{Namespace: types.DefaultKubeVelaNS, Name: name}, a); err != nil {
		return err
	}
	return u.kubeClient.Delete(ctx, a)
}
