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

package service

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/types"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
	"github.com/oam-dev/kubevela/pkg/config"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// ConfigService handle CRUD of config and template
type ConfigService interface {
	ListTemplates(ctx context.Context, project, scope string) ([]*apis.ConfigTemplate, error)
	GetTemplate(ctx context.Context, tem config.NamespacedName) (*apis.ConfigTemplateDetail, error)
	CreateConfig(ctx context.Context, project string, req apis.CreateConfigRequest) (*apis.Config, error)
	UpdateConfig(ctx context.Context, project string, name string, req apis.UpdateConfigRequest) (*apis.Config, error)
	ListConfigs(ctx context.Context, project, template string, withProperties bool) ([]*apis.Config, error)
	GetConfig(ctx context.Context, project, name string) (*apis.Config, error)
	DeleteConfig(ctx context.Context, project, name string) error
	CreateConfigDistribution(ctx context.Context, project string, req apis.CreateConfigDistributionRequest) error
	DeleteConfigDistribution(ctx context.Context, project, name string) error
	ListConfigDistributions(ctx context.Context, project string) ([]*config.Distribution, error)
}

// NewConfigService returns a config use case
func NewConfigService() ConfigService {
	return &configServiceImpl{}
}

type configServiceImpl struct {
	KubeClient     client.Client    `inject:"kubeClient"`
	ProjectService ProjectService   `inject:""`
	Factory        config.Factory   `inject:"configFactory"`
	Apply          apply.Applicator `inject:"apply"`
}

// ListTemplates list the config templates
func (u *configServiceImpl) ListTemplates(ctx context.Context, project, scope string) ([]*apis.ConfigTemplate, error) {
	queryTemplates, err := u.Factory.ListTemplates(ctx, types.DefaultKubeVelaNS, scope)
	if err != nil {
		return nil, err
	}
	if scope == "project" && project != "" {
		pro, err := u.ProjectService.GetProject(ctx, project)
		if err != nil {
			return nil, err
		}
		templates, err := u.Factory.ListTemplates(ctx, pro.GetNamespace(), scope)
		if err != nil {
			return nil, err
		}
		queryTemplates = append(queryTemplates, templates...)
	}
	var templates []*apis.ConfigTemplate
	for _, t := range queryTemplates {
		templates = append(templates, &apis.ConfigTemplate{
			Alias:       t.Alias,
			Name:        t.Name,
			Description: t.Description,
			Namespace:   t.Namespace,
			Scope:       t.Scope,
			Sensitive:   t.Sensitive,
			CreateTime:  t.CreateTime,
		})
	}
	sort.SliceStable(templates, func(i, j int) bool {
		return templates[i].Alias < templates[j].Alias
	})
	return templates, nil
}

// GetTemplate detail a template
func (u *configServiceImpl) GetTemplate(ctx context.Context, tem config.NamespacedName) (*apis.ConfigTemplateDetail, error) {
	if tem.Namespace == "" {
		tem.Namespace = types.DefaultKubeVelaNS
	}
	template, err := u.Factory.LoadTemplate(ctx, tem.Name, tem.Namespace)
	if err != nil {
		if errors.Is(err, config.ErrTemplateNotFound) {
			return nil, bcode.ErrTemplateNotFound
		}
		return nil, err
	}
	defaultUISchema := renderDefaultUISchema(template.Schema)
	t := &apis.ConfigTemplateDetail{
		ConfigTemplate: apis.ConfigTemplate{
			Alias:       template.Alias,
			Name:        template.Name,
			Description: template.Description,
			Namespace:   template.Namespace,
			Scope:       template.Scope,
			Sensitive:   template.Sensitive,
			CreateTime:  template.CreateTime,
		},
		APISchema: template.Schema,
		// TODO: Support to define the custom UI schema in the template cue script.
		UISchema: renderCustomUISchema(ctx, u.KubeClient, template.Name, "config", defaultUISchema),
	}
	return t, nil
}

func (u *configServiceImpl) CreateConfig(ctx context.Context, project string, req apis.CreateConfigRequest) (*apis.Config, error) {
	ns := types.DefaultKubeVelaNS
	if project != "" {
		pro, err := u.ProjectService.GetProject(ctx, project)
		if err != nil {
			return nil, err
		}
		ns = pro.GetNamespace()
		if err := utils.CreateNamespace(ctx, u.KubeClient, ns); err != nil && !apierrors.IsAlreadyExists(err) {
			return nil, err
		}
	}
	var properties = make(map[string]interface{})
	if err := json.Unmarshal([]byte(req.Properties), &properties); err != nil {
		return nil, err
	}
	if req.Template.Namespace == "" {
		req.Template.Namespace = types.DefaultKubeVelaNS
	}
	configItem, err := u.Factory.ParseConfig(ctx, config.NamespacedName(req.Template), config.Metadata{
		NamespacedName: config.NamespacedName{Name: req.Name, Namespace: ns},
		Properties:     properties,
		Alias:          req.Alias, Description: req.Description,
	})
	if err != nil {
		if errors.Is(err, config.ErrTemplateNotFound) {
			return nil, bcode.ErrTemplateNotFound
		}
		return nil, err
	}
	if err := u.Factory.CreateOrUpdateConfig(ctx, configItem, ns); err != nil {
		if errors.Is(err, config.ErrConfigExist) {
			return nil, bcode.ErrConfigExist
		}
		return nil, err
	}
	return convertConfig(project, *configItem), nil
}

func (u *configServiceImpl) UpdateConfig(ctx context.Context, project string, name string, req apis.UpdateConfigRequest) (*apis.Config, error) {
	ns := types.DefaultKubeVelaNS
	if project != "" {
		pro, err := u.ProjectService.GetProject(ctx, project)
		if err != nil {
			return nil, err
		}
		ns = pro.GetNamespace()
	}

	it, err := u.Factory.GetConfig(ctx, ns, name, false)
	if err != nil {
		if errors.Is(err, config.ErrSensitiveConfig) {
			return nil, bcode.ErrSensitiveConfig
		}
		if errors.Is(err, config.ErrConfigNotFound) {
			return nil, bcode.ErrConfigNotFound
		}
		return nil, err
	}

	var properties = make(map[string]interface{})
	if err := json.Unmarshal([]byte(req.Properties), &properties); err != nil {
		return nil, err
	}
	configItem, err := u.Factory.ParseConfig(ctx,
		it.Template.NamespacedName,
		config.Metadata{NamespacedName: config.NamespacedName{Name: it.Name, Namespace: ns}, Alias: req.Alias, Description: req.Description, Properties: properties})
	if err != nil {
		return nil, err
	}
	if err := u.Factory.CreateOrUpdateConfig(ctx, configItem, ns); err != nil {
		if errors.Is(err, config.ErrConfigExist) {
			return nil, bcode.ErrChangeTemplate
		}
		return nil, err
	}
	return convertConfig(project, *configItem), nil
}

// ListConfigs query the available configs.
// If the project is not empty, it means query all usable configs for this project.
func (u *configServiceImpl) ListConfigs(ctx context.Context, project string, template string, withProperties bool) ([]*apis.Config, error) {
	var list []*apis.Config
	scope := ""
	var projectNamespace string
	if project != "" {
		scope = "project"
		pro, err := u.ProjectService.GetProject(ctx, project)
		if err != nil {
			return nil, err
		}
		projectNamespace = pro.GetNamespace()
		// query the configs belong to the project scope from the system namespace
		configs, err := u.Factory.ListConfigs(ctx, pro.GetNamespace(), template, "", true)
		if err != nil {
			return nil, err
		}
		for i := range configs {
			list = append(list, convertConfig(project, *configs[i]))
		}
	}

	configs, err := u.Factory.ListConfigs(ctx, types.DefaultKubeVelaNS, template, scope, true)
	if err != nil {
		return nil, err
	}
	for i := range configs {
		if projectNamespace != "" {
			if err := u.Factory.MergeDistributionStatus(ctx, configs[i], projectNamespace); err != nil && !errors.Is(err, config.ErrNotFoundDistribution) {
				log.Logger.Warnf("fail to merge the status %s:%s", configs[i].Name, err.Error())
			}
		}
		item := convertConfig(project, *configs[i])
		item.Shared = true
		if !withProperties {
			item.Properties = nil
		}
		list = append(list, item)
	}
	return list, nil
}

// CreateConfigDistribution distribute the configs to the target namespaces.
func (u *configServiceImpl) CreateConfigDistribution(ctx context.Context, project string, req apis.CreateConfigDistributionRequest) error {
	pro, err := u.ProjectService.GetProject(ctx, project)
	if err != nil {
		return err
	}
	if len(req.Configs) == 0 || len(req.Targets) == 0 {
		return bcode.ErrNoConfigOrTarget
	}
	var targets []*config.ClusterTarget
	for _, t := range req.Targets {
		if t.Namespace != "" && t.ClusterName != "" {
			targets = append(targets, &config.ClusterTarget{Namespace: t.Namespace, ClusterName: t.ClusterName})
		}
	}

	var configs []*config.NamespacedName
	for _, t := range req.Configs {
		if t.Name != "" {
			configs = append(configs, &config.NamespacedName{Namespace: t.Namespace, Name: t.Name})
		}
	}
	return u.Factory.CreateOrUpdateDistribution(ctx, pro.GetNamespace(), req.Name, &config.ApplyDistributionSpec{
		Configs: configs,
		Targets: targets,
	})
}

// ListDistributeConfigs list the all distributions
func (u *configServiceImpl) ListConfigDistributions(ctx context.Context, project string) ([]*config.Distribution, error) {
	pro, err := u.ProjectService.GetProject(ctx, project)
	if err != nil {
		return nil, err
	}
	return u.Factory.ListDistributions(ctx, pro.GetNamespace())
}

// DeleteConfigDistribution delete a distribution
func (u *configServiceImpl) DeleteConfigDistribution(ctx context.Context, project, name string) error {
	pro, err := u.ProjectService.GetProject(ctx, project)
	if err != nil {
		return err
	}
	return u.Factory.DeleteDistribution(ctx, pro.GetNamespace(), name)
}

func convertConfig(project string, config config.Config) *apis.Config {
	return &apis.Config{
		Template:    config.Template.NamespacedName,
		Sensitive:   config.Template.Sensitive,
		Name:        config.Name,
		Namespace:   config.Namespace,
		Project:     project,
		Alias:       config.Alias,
		Description: config.Description,
		CreatedTime: &config.CreateTime,
		Properties:  config.Properties,
		Secret:      config.Secret,
		Targets:     config.Targets,
	}
}

func (u *configServiceImpl) GetConfig(ctx context.Context, project, name string) (*apis.Config, error) {
	ns := types.DefaultKubeVelaNS
	if project != "" {
		pro, err := u.ProjectService.GetProject(ctx, project)
		if err != nil {
			return nil, err
		}
		ns = pro.GetNamespace()
	}

	it, err := u.Factory.GetConfig(ctx, ns, name, true)
	if err != nil {
		if errors.Is(err, config.ErrSensitiveConfig) {
			return nil, bcode.ErrSensitiveConfig
		}
		if errors.Is(err, config.ErrConfigNotFound) {
			return nil, bcode.ErrConfigNotFound
		}
		return nil, err
	}

	return convertConfig(project, *it), nil
}

func (u *configServiceImpl) DeleteConfig(ctx context.Context, project, name string) error {
	ns := types.DefaultKubeVelaNS
	if project != "" {
		pro, err := u.ProjectService.GetProject(ctx, project)
		if err != nil {
			return err
		}
		ns = pro.GetNamespace()
	}
	return u.Factory.DeleteConfig(ctx, ns, name)
}
