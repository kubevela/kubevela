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
	"github.com/oam-dev/kubevela/pkg/integration"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// IntegrationService handle CRUD of integration and template
type IntegrationService interface {
	ListTemplates(ctx context.Context, project, scope string) ([]*apis.IntegrationTemplate, error)
	GetTemplate(ctx context.Context, tem integration.NamespacedName) (*apis.IntegrationTemplateDetail, error)
	CreateIntegration(ctx context.Context, project string, req apis.CreateIntegrationRequest) (*apis.Integration, error)
	UpdateIntegration(ctx context.Context, project string, name string, req apis.UpdateIntegrationRequest) (*apis.Integration, error)
	ListIntegrations(ctx context.Context, project, template string, withProperties bool) ([]*apis.Integration, error)
	GetIntegration(ctx context.Context, project, name string) (*apis.Integration, error)
	DeleteIntegration(ctx context.Context, project, name string) error
	ApplyIntegrationDistribution(ctx context.Context, project string, req apis.ApplyIntegrationDistributionRequest) error
	DeleteIntegrationDistribution(ctx context.Context, project, name string) error
	ListIntegrationDistributions(ctx context.Context, project string) ([]*integration.Distribution, error)
}

// NewIntegrationService returns a integration use case
func NewIntegrationService() IntegrationService {
	return &integrationServiceImpl{}
}

type integrationServiceImpl struct {
	KubeClient     client.Client       `inject:"kubeClient"`
	ProjectService ProjectService      `inject:""`
	Factory        integration.Factory `inject:"integrationFactory"`
	Apply          apply.Applicator    `inject:"apply"`
}

// ListTemplates list the integration templates
func (u *integrationServiceImpl) ListTemplates(ctx context.Context, project, scope string) ([]*apis.IntegrationTemplate, error) {
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
	var templates []*apis.IntegrationTemplate
	for _, t := range queryTemplates {
		templates = append(templates, &apis.IntegrationTemplate{
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
func (u *integrationServiceImpl) GetTemplate(ctx context.Context, tem integration.NamespacedName) (*apis.IntegrationTemplateDetail, error) {
	if tem.Namespace == "" {
		tem.Namespace = types.DefaultKubeVelaNS
	}
	template, err := u.Factory.LoadTemplate(ctx, tem.Name, tem.Namespace)
	if err != nil {
		return nil, err
	}
	defaultUISchema := renderDefaultUISchema(template.Schema)
	t := &apis.IntegrationTemplateDetail{
		IntegrationTemplate: apis.IntegrationTemplate{
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
		UISchema: renderCustomUISchema(ctx, u.KubeClient, template.Name, "integration", defaultUISchema),
	}
	return t, nil
}

func (u *integrationServiceImpl) CreateIntegration(ctx context.Context, project string, req apis.CreateIntegrationRequest) (*apis.Integration, error) {
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
	integration, err := u.Factory.ParseIntegration(ctx, integration.NamespacedName(req.Template), integration.Metadata{
		NamespacedName: integration.NamespacedName{Name: req.Name, Namespace: ns},
		Properties:     properties,
		Alias:          req.Alias, Description: req.Description,
	})
	if err != nil {
		return nil, err
	}
	if err := u.Factory.ApplyIntegration(ctx, integration, ns); err != nil {
		return nil, err
	}
	return convertIntegration(project, *integration), nil
}

func (u *integrationServiceImpl) UpdateIntegration(ctx context.Context, project string, name string, req apis.UpdateIntegrationRequest) (*apis.Integration, error) {
	ns := types.DefaultKubeVelaNS
	if project != "" {
		pro, err := u.ProjectService.GetProject(ctx, project)
		if err != nil {
			return nil, err
		}
		ns = pro.GetNamespace()
	}

	it, err := u.Factory.GetIntegration(ctx, ns, name)
	if err != nil {
		if errors.Is(err, integration.ErrSensitiveIntegration) {
			return nil, bcode.ErrSensitiveIntegration
		}
		return nil, err
	}

	var properties = make(map[string]interface{})
	if err := json.Unmarshal([]byte(req.Properties), &properties); err != nil {
		return nil, err
	}
	integration, err := u.Factory.ParseIntegration(ctx,
		it.Template.NamespacedName,
		integration.Metadata{NamespacedName: integration.NamespacedName{Name: it.Name, Namespace: ns}, Alias: req.Alias, Description: req.Description, Properties: properties})
	if err != nil {
		return nil, err
	}
	if err := u.Factory.ApplyIntegration(ctx, integration, ns); err != nil {
		return nil, err
	}
	return convertIntegration(project, *integration), nil
}

// ListIntegrations query the available integrations.
// If the project is not empty, it means query all usable integrations for this project.
func (u *integrationServiceImpl) ListIntegrations(ctx context.Context, project string, template string, withProperties bool) ([]*apis.Integration, error) {
	var list []*apis.Integration
	scope := ""
	if project != "" {
		scope = "project"
		pro, err := u.ProjectService.GetProject(ctx, project)
		if err != nil {
			return nil, err
		}
		// query the integrations belong to the project scope from the system namespace
		integrations, err := u.Factory.ListIntegrations(ctx, pro.GetNamespace(), template, "")
		if err != nil {
			return nil, err
		}
		for i := range integrations {
			list = append(list, convertIntegration(project, *integrations[i]))
		}
	}

	integrations, err := u.Factory.ListIntegrations(ctx, types.DefaultKubeVelaNS, template, scope)
	if err != nil {
		return nil, err
	}
	for i := range integrations {
		item := convertIntegration(project, *integrations[i])
		item.Shared = true
		if !withProperties {
			item.Properties = nil
		}
		list = append(list, item)
	}
	return list, nil
}

// ApplyIntegrationDistribution distribute the integrations to the target namespaces.
func (u *integrationServiceImpl) ApplyIntegrationDistribution(ctx context.Context, project string, req apis.ApplyIntegrationDistributionRequest) error {
	pro, err := u.ProjectService.GetProject(ctx, project)
	if err != nil {
		return err
	}
	if len(req.Integrations) == 0 || len(req.Targets) == 0 {
		return bcode.ErrNoIntegrationOrTarget
	}
	var targets []*integration.ClusterTarget
	for _, t := range req.Targets {
		targets = append(targets, &integration.ClusterTarget{Namespace: t.Namespace, ClusterName: t.ClusterName})
	}

	var integrations []*integration.NamespacedName
	for _, t := range req.Integrations {
		integrations = append(integrations, &integration.NamespacedName{Namespace: t.Namespace, Name: t.Name})
	}
	return u.Factory.ApplyDistribution(ctx, pro.GetNamespace(), req.Name, &integration.ApplyDistributionSpec{
		Integrations: integrations,
		Targets:      targets,
	})
}

// ListDistributeIntegrations list the all distributions
func (u *integrationServiceImpl) ListIntegrationDistributions(ctx context.Context, project string) ([]*integration.Distribution, error) {
	pro, err := u.ProjectService.GetProject(ctx, project)
	if err != nil {
		return nil, err
	}
	return u.Factory.ListDistributions(ctx, pro.GetNamespace())
}

// DeleteIntegrationDistribution delete a distribution
func (u *integrationServiceImpl) DeleteIntegrationDistribution(ctx context.Context, project, name string) error {
	pro, err := u.ProjectService.GetProject(ctx, project)
	if err != nil {
		return err
	}
	return u.Factory.DeleteDistribution(ctx, pro.GetNamespace(), name)
}

func convertIntegration(project string, integration integration.Integration) *apis.Integration {
	return &apis.Integration{
		Template:    integration.Template.NamespacedName,
		Sensitive:   integration.Template.Sensitive,
		Name:        integration.Name,
		Namespace:   integration.Namespace,
		Project:     project,
		Alias:       integration.Alias,
		Description: integration.Description,
		CreatedTime: &integration.CreateTime,
		Properties:  integration.Properties,
		Secret:      integration.Secret,
	}
}

func (u *integrationServiceImpl) GetIntegration(ctx context.Context, project, name string) (*apis.Integration, error) {
	ns := types.DefaultKubeVelaNS
	if project != "" {
		pro, err := u.ProjectService.GetProject(ctx, project)
		if err != nil {
			return nil, err
		}
		ns = pro.GetNamespace()
	}

	it, err := u.Factory.GetIntegration(ctx, ns, name)
	if err != nil {
		if errors.Is(err, integration.ErrSensitiveIntegration) {
			return nil, bcode.ErrSensitiveIntegration
		}
		return nil, err
	}

	return convertIntegration(project, *it), nil
}

func (u *integrationServiceImpl) DeleteIntegration(ctx context.Context, project, name string) error {
	ns := types.DefaultKubeVelaNS
	if project != "" {
		pro, err := u.ProjectService.GetProject(ctx, project)
		if err != nil {
			return err
		}
		ns = pro.GetNamespace()
	}
	return u.Factory.DeleteIntegration(ctx, ns, name)
}
