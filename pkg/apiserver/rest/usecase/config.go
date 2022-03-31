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
	"encoding/json"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/definition"
)

const (
	definitionAlias = definition.UserPrefix + "alias.config.oam.dev"
	definitionType  = definition.UserPrefix + "type.config.oam.dev"

	velaCoreConfig = "velacore-config"
)

// ConfigHandler handle CRUD of configs
type ConfigHandler interface {
	ListConfigTypes(ctx context.Context, query string) ([]*apis.ConfigType, error)
	GetConfigType(ctx context.Context, configType string) (*apis.ConfigType, error)
	CreateConfig(ctx context.Context, req apis.CreateConfigRequest) error
	GetConfigs(ctx context.Context, configType string) ([]*apis.Config, error)
	GetConfig(ctx context.Context, configType, name string) (*apis.Config, error)
	DeleteConfig(ctx context.Context, configType, name string) error
}

// NewConfigUseCase returns a config use case
func NewConfigUseCase(authenticationUseCase AuthenticationUsecase) ConfigHandler {
	k8sClient, err := clients.GetKubeClient()
	if err != nil {
		panic(err)
	}
	return &configUseCaseImpl{
		authenticationUseCase: authenticationUseCase,
		kubeClient:            k8sClient,
	}
}

type configUseCaseImpl struct {
	kubeClient            client.Client
	authenticationUseCase AuthenticationUsecase
}

// ListConfigTypes returns all config types
func (u *configUseCaseImpl) ListConfigTypes(ctx context.Context, query string) ([]*apis.ConfigType, error) {
	defs := &v1beta1.ComponentDefinitionList{}
	if err := u.kubeClient.List(ctx, defs, client.InNamespace(types.DefaultKubeVelaNS),
		client.MatchingLabels{definition.UserPrefix + "catalog.config.oam.dev": velaCoreConfig}); err != nil {
		return nil, err
	}

	var tfDefs []v1beta1.ComponentDefinition
	var configTypes []*apis.ConfigType

	for _, d := range defs.Items {
		if d.Labels[definitionType] == types.TerraformProvider {
			tfDefs = append(tfDefs, d)
			continue
		}
		configTypes = append(configTypes, &apis.ConfigType{
			Alias:       d.Annotations[definitionAlias],
			Name:        d.Name,
			Definitions: []string{d.Name},
			Description: d.Annotations[types.AnnoDefinitionDescription],
		})
	}

	tfType := &apis.ConfigType{
		Alias: "Terraform Cloud Provider",
		Name:  types.TerraformProvider,
	}
	definitions := make([]string, len(tfDefs))

	for i, tf := range tfDefs {
		definitions[i] = tf.Name
	}
	tfType.Definitions = definitions

	return append(configTypes, tfType), nil
}

// GetConfigType returns a config type
func (u *configUseCaseImpl) GetConfigType(ctx context.Context, configType string) (*apis.ConfigType, error) {
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

func (u *configUseCaseImpl) CreateConfig(ctx context.Context, req apis.CreateConfigRequest) error {
	app := v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: types.DefaultKubeVelaNS,
			Annotations: map[string]string{
				types.AnnotationConfigAlias: req.Alias,
			},
			Labels: map[string]string{
				model.LabelSourceOfTruth: model.FromInner,
				types.LabelConfigCatalog: velaCoreConfig,
				types.LabelConfigType:    req.ComponentType,
				types.LabelConfigProject: req.Project,
			},
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{
					Name:       req.Name,
					Type:       req.ComponentType,
					Properties: &runtime.RawExtension{Raw: []byte(req.Properties)},
				},
			},
		},
	}
	if err := u.kubeClient.Create(ctx, &app); err != nil {
		return err
	}

	// try to check whether the underlying config secrets is successfully created
	var succeeded bool
	var configApp v1beta1.Application
	for i := 0; i < 100; i++ {
		if err := u.kubeClient.Get(ctx, client.ObjectKey{Namespace: types.DefaultKubeVelaNS, Name: req.Name}, &configApp); err == nil {
			if configApp.Status.Phase == common.ApplicationRunning {
				succeeded = true
				break
			}
		}
		time.Sleep(time.Second)
	}
	// clean up failed application
	if !succeeded {
		if err := u.kubeClient.Delete(ctx, &app); err != nil {
			return err
		}
		return errors.New("failed to create config")
	}

	if succeeded && req.ComponentType == types.DexConnector {
		return u.authenticationUseCase.UpdateDexConfig(ctx)
	}

	return nil
}

func (u *configUseCaseImpl) GetConfigs(ctx context.Context, configType string) ([]*apis.Config, error) {
	switch configType {
	case types.TerraformProvider:
		defs := &v1beta1.ComponentDefinitionList{}
		if err := u.kubeClient.List(ctx, defs, client.InNamespace(types.DefaultKubeVelaNS),
			client.MatchingLabels{
				definition.UserPrefix + "catalog.config.oam.dev": velaCoreConfig,
				definition.UserPrefix + "type.config.oam.dev":    types.TerraformProvider,
			}); err != nil {
			return nil, err
		}
		var configs []*apis.Config
		for _, d := range defs.Items {
			subConfigs, err := u.getConfigsByConfigType(ctx, d.Name)
			if err != nil {
				return nil, err
			}
			configs = append(configs, subConfigs...)
		}
		return configs, nil

	default:
		return u.getConfigsByConfigType(ctx, configType)

	}
}

func (u *configUseCaseImpl) getConfigsByConfigType(ctx context.Context, configType string) ([]*apis.Config, error) {
	var apps = &v1beta1.ApplicationList{}
	if err := u.kubeClient.List(ctx, apps, client.InNamespace(types.DefaultKubeVelaNS),
		client.MatchingLabels{
			model.LabelSourceOfTruth: model.FromInner,
			types.LabelConfigCatalog: velaCoreConfig,
			types.LabelConfigType:    configType,
		}); err != nil {
		return nil, err
	}

	configs := make([]*apis.Config, len(apps.Items))
	for i, a := range apps.Items {
		configs[i] = &apis.Config{
			ConfigType:  a.Labels[types.LabelConfigType],
			Name:        a.Name,
			Project:     a.Labels[types.LabelConfigProject],
			CreatedTime: &(a.CreationTimestamp.Time),
		}
	}
	return configs, nil
}

func (u *configUseCaseImpl) GetConfig(ctx context.Context, configType, name string) (*apis.Config, error) {
	var a = &v1beta1.Application{}
	if err := u.kubeClient.Get(ctx, client.ObjectKey{Namespace: types.DefaultKubeVelaNS, Name: name}, a); err != nil {
		return nil, err
	}

	config := &apis.Config{
		ConfigType:  a.Labels[types.LabelConfigType],
		Name:        a.Name,
		Project:     a.Labels[types.LabelConfigProject],
		CreatedTime: &a.CreationTimestamp.Time,
	}

	return config, nil
}

func (u *configUseCaseImpl) DeleteConfig(ctx context.Context, configType, name string) error {
	var a = &v1beta1.Application{}
	if err := u.kubeClient.Get(ctx, client.ObjectKey{Namespace: types.DefaultKubeVelaNS, Name: name}, a); err != nil {
		return err
	}
	return u.kubeClient.Delete(ctx, a)
}

// ApplicationDeployTarget is the struct of application deploy target
type ApplicationDeployTarget struct {
	Namespace string   `json:"namespace"`
	Clusters  []string `json:"clusters"`
}

// SyncConfigs will sync configs to working clusters
func SyncConfigs(ctx context.Context, k8sClient client.Client, project string, targets []*model.ClusterTarget) error {
	name := fmt.Sprintf("config-sync-%s", project)
	// get all configs which can be synced to working clusters in the project
	var secrets v1.SecretList
	if err := k8sClient.List(ctx, &secrets, client.InNamespace(types.DefaultKubeVelaNS),
		client.MatchingLabels{
			types.LabelConfigCatalog:            velaCoreConfig,
			types.LabelConfigProject:            project,
			types.LabelConfigSyncToMultiCluster: "true",
		}); err != nil {
		return err
	}

	objects := make([]map[string]string, len(secrets.Items))
	for i, s := range secrets.Items {
		objects[i] = map[string]string{
			"name":     s.Name,
			"resource": "secret",
		}
	}
	objectsBytes, err := json.Marshal(map[string][]map[string]string{"objects": objects})
	if err != nil {
		return err
	}

	var app = &v1beta1.Application{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: types.DefaultKubeVelaNS, Name: name}, app); err != nil {
		if !kerrors.IsNotFound(err) {
			return err
		}
		// config sync application doesn't exist, create one

		scratch := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: types.DefaultKubeVelaNS,
				Labels: map[string]string{
					model.LabelSourceOfTruth: model.FromInner,
					types.LabelConfigCatalog: velaCoreConfig,
					types.LabelConfigProject: project,
				},
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       name,
						Type:       "ref-objects",
						Properties: &runtime.RawExtension{Raw: objectsBytes},
					},
				},
			},
		}
		if err := k8sClient.Create(ctx, scratch); err != nil {
			return err
		}
	}
	// config sync application exists, update it
	app.Spec.Components = []common.ApplicationComponent{
		{Properties: &runtime.RawExtension{Raw: objectsBytes}},
	}
	currentTargets := make([]ApplicationDeployTarget, len(app.Spec.Policies))
	for i, p := range app.Spec.Policies {
		var t ApplicationDeployTarget
		if err := json.Unmarshal(p.Properties.Raw, &t); err != nil {
			return err
		}
		currentTargets[i] = t
	}

	mergedTarget := mergeTargets(currentTargets, targets)
	mergedPolicies := make([]v1beta1.AppPolicy, len(mergedTarget))
	for i, t := range mergedTarget {
		properties, err := json.Marshal(t)
		if err != nil {
			return err
		}
		mergedPolicies[i] = v1beta1.AppPolicy{
			Type: "topology",
			Name: t.Namespace,
			Properties: &runtime.RawExtension{
				Raw: properties,
			},
		}
	}
	app.Spec.Policies = mergedPolicies
	return k8sClient.Update(ctx, app)
}

func mergeTargets(currentTargets []ApplicationDeployTarget, targets []*model.ClusterTarget) []ApplicationDeployTarget {
	var mergedTargets []ApplicationDeployTarget
	for _, c := range currentTargets {
		var hasSameNamespace bool
		for _, t := range targets {
			if c.Namespace == t.Namespace {
				hasSameNamespace = true
				clusters := append(c.Clusters, t.ClusterName)
				mergedTargets = append(mergedTargets, ApplicationDeployTarget{Namespace: c.Namespace, Clusters: clusters})
			}
		}
		if !hasSameNamespace {
			mergedTargets = append(mergedTargets, c)
		}
	}

	for _, t := range targets {
		var hasSameNamespace bool
		for _, c := range currentTargets {
			if c.Namespace == t.Namespace {
				hasSameNamespace = true
			}
		}
		if !hasSameNamespace {
			mergedTargets = append(mergedTargets, ApplicationDeployTarget{Namespace: t.Namespace, Clusters: []string{t.ClusterName}})
		}
	}

	return mergedTargets
}
