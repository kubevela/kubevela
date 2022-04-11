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

package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	terraformapi "github.com/oam-dev/terraform-controller/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/multicluster"
)

// ProjectUsecase project manage usecase.
type ProjectUsecase interface {
	GetProject(ctx context.Context, projectName string) (*model.Project, error)
	DetailProject(ctx context.Context, projectName string) (*apisv1.ProjectBase, error)
	ListProjects(ctx context.Context, page, pageSize int) (*apisv1.ListProjectResponse, error)
	ListUserProjects(ctx context.Context, userName string) ([]*apisv1.ProjectBase, error)
	CreateProject(ctx context.Context, req apisv1.CreateProjectRequest) (*apisv1.ProjectBase, error)
	DeleteProject(ctx context.Context, projectName string) error
	UpdateProject(ctx context.Context, projectName string, req apisv1.UpdateProjectRequest) (*apisv1.ProjectBase, error)
	ListProjectUser(ctx context.Context, projectName string, page, pageSize int) (*apisv1.ListProjectUsersResponse, error)
	AddProjectUser(ctx context.Context, projectName string, req apisv1.AddProjectUserRequest) (*apisv1.ProjectUserBase, error)
	DeleteProjectUser(ctx context.Context, projectName string, userName string) error
	UpdateProjectUser(ctx context.Context, projectName string, userName string, req apisv1.UpdateProjectUserRequest) (*apisv1.ProjectUserBase, error)
	Init(ctx context.Context) error
	GetConfigs(ctx context.Context, projectName, configType string) ([]*apisv1.Config, error)
}

type projectUsecaseImpl struct {
	ds          datastore.DataStore
	k8sClient   client.Client
	rbacUsecase RBACUsecase
}

// NewProjectUsecase new project usecase
func NewProjectUsecase(ds datastore.DataStore, rbacUsecase RBACUsecase) ProjectUsecase {
	k8sClient, err := clients.GetKubeClient()
	if err != nil {
		log.Logger.Fatalf("get k8sClient failure: %s", err.Error())
	}
	p := &projectUsecaseImpl{ds: ds, k8sClient: k8sClient, rbacUsecase: rbacUsecase}
	return p
}

// Init init default data
func (p *projectUsecaseImpl) Init(ctx context.Context) error {
	return p.InitDefaultProjectEnvTarget(ctx, model.DefaultInitNamespace)
}

// initDefaultProjectEnvTarget will initialize a default project with a default env that contain a default target
// the default env and default target both using the `default` namespace in control plane cluster
func (p *projectUsecaseImpl) InitDefaultProjectEnvTarget(ctx context.Context, defaultNamespace string) error {
	var project = model.Project{}
	entities, err := p.ds.List(ctx, &project, &datastore.ListOptions{FilterOptions: datastore.FilterOptions{
		IsNotExist: []datastore.IsNotExistQueryOption{
			{
				Key: "owner",
			},
		},
	}})
	if err != nil {
		return fmt.Errorf("initialize project failed %w", err)
	}
	if len(entities) > 0 {
		for _, project := range entities {
			pro := project.(*model.Project)
			var init = pro.Owner == ""
			pro.Owner = model.DefaultAdminUserName
			if err := p.ds.Put(ctx, pro); err != nil {
				return err
			}
			// owner is empty, it is old data
			if init {
				if err := p.rbacUsecase.InitDefaultRoleAndUsersForProject(ctx, pro); err != nil {
					return fmt.Errorf("init default role and users for project %s failure %w", pro.Name, err)
				}
			}
		}
		return nil
	}

	count, _ := p.ds.Count(ctx, &project, nil)
	if count > 0 {
		return nil
	}
	log.Logger.Info("no default project found, adding a default project with default env and target")

	if err := createTargetNamespace(ctx, p.k8sClient, multicluster.ClusterLocalName, defaultNamespace, model.DefaultInitName); err != nil {
		return fmt.Errorf("initialize default target namespace failed %w", err)
	}
	// initialize default target first
	err = createTarget(ctx, p.ds, &model.Target{
		Name:        model.DefaultInitName,
		Alias:       "Default",
		Description: model.DefaultTargetDescription,
		Cluster: &model.ClusterTarget{
			ClusterName: multicluster.ClusterLocalName,
			Namespace:   defaultNamespace,
		},
	})
	// for idempotence, ignore default target already exist error
	if err != nil && errors.Is(err, bcode.ErrTargetExist) {
		return fmt.Errorf("initialize default target failed %w", err)
	}

	// initialize default target first
	err = createEnv(ctx, p.k8sClient, p.ds, &model.Env{
		Name:        model.DefaultInitName,
		Alias:       "Default",
		Description: model.DefaultEnvDescription,
		Project:     model.DefaultInitName,
		Namespace:   defaultNamespace,
		Targets:     []string{model.DefaultInitName},
	})
	// for idempotence, ignore default env already exist error
	if err != nil && errors.Is(err, bcode.ErrEnvAlreadyExists) {

		return fmt.Errorf("initialize default environment failed %w", err)
	}

	_, err = p.CreateProject(ctx, apisv1.CreateProjectRequest{
		Name:        model.DefaultInitName,
		Alias:       "Default",
		Description: model.DefaultProjectDescription,
		Owner:       model.DefaultAdminUserName,
	})
	if err != nil {
		return fmt.Errorf("initialize project failed %w", err)
	}
	return nil
}

// GetProject get project
func (p *projectUsecaseImpl) GetProject(ctx context.Context, projectName string) (*model.Project, error) {
	project := &model.Project{Name: projectName}
	if err := p.ds.Get(ctx, project); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, bcode.ErrProjectIsNotExist
		}
		return nil, err
	}
	return project, nil
}

func (p *projectUsecaseImpl) DetailProject(ctx context.Context, projectName string) (*apisv1.ProjectBase, error) {
	project, err := p.GetProject(ctx, projectName)
	if err != nil {
		return nil, err
	}
	var user = &model.User{Name: project.Owner}
	if project.Owner != "" {
		if err := p.ds.Get(ctx, user); err != nil {
			log.Logger.Warnf("get project owner %s info failure %s", project.Owner, err.Error())
		}
	}
	return ConvertProjectModel2Base(project, user), nil
}

func listProjects(ctx context.Context, ds datastore.DataStore, page, pageSize int) (*apisv1.ListProjectResponse, error) {
	var project = model.Project{}
	entities, err := ds.List(ctx, &project, &datastore.ListOptions{Page: page, PageSize: pageSize, SortBy: []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderDescending}}})
	if err != nil {
		return nil, err
	}
	var projects []*apisv1.ProjectBase
	for _, entity := range entities {
		project := entity.(*model.Project)
		var user = &model.User{Name: project.Owner}
		if project.Owner != "" {
			if err := ds.Get(ctx, user); err != nil {
				log.Logger.Warnf("get project owner %s info failure %s", project.Owner, err.Error())
			}
		}
		projects = append(projects, ConvertProjectModel2Base(project, user))
	}
	total, err := ds.Count(ctx, &project, nil)
	if err != nil {
		return nil, err
	}
	return &apisv1.ListProjectResponse{Projects: projects, Total: total}, nil
}

func (p *projectUsecaseImpl) ListUserProjects(ctx context.Context, userName string) ([]*apisv1.ProjectBase, error) {
	var projectUser = model.ProjectUser{
		Username: userName,
	}
	entities, err := p.ds.List(ctx, &projectUser, nil)
	if err != nil {
		return nil, err
	}
	var projectNames []string
	for _, entity := range entities {
		projectNames = append(projectNames, entity.(*model.ProjectUser).ProjectName)
	}
	if len(projectNames) == 0 {
		return []*apisv1.ProjectBase{}, nil
	}
	projectEntities, err := p.ds.List(ctx, &model.Project{}, &datastore.ListOptions{FilterOptions: datastore.FilterOptions{In: []datastore.InQueryOption{{
		Key:    "name",
		Values: projectNames,
	}}}})
	if err != nil {
		return nil, err
	}
	var projectBases []*apisv1.ProjectBase
	for _, entity := range projectEntities {
		projectBases = append(projectBases, ConvertProjectModel2Base(entity.(*model.Project), nil))
	}
	return projectBases, nil
}

// ListProjects list projects
func (p *projectUsecaseImpl) ListProjects(ctx context.Context, page, pageSize int) (*apisv1.ListProjectResponse, error) {
	return listProjects(ctx, p.ds, page, pageSize)
}

// DeleteProject delete a project
func (p *projectUsecaseImpl) DeleteProject(ctx context.Context, name string) error {
	_, err := p.GetProject(ctx, name)
	if err != nil {
		return err
	}

	count, err := p.ds.Count(ctx, &model.Application{Project: name}, nil)
	if err != nil {
		return err
	}
	if count > 0 {
		return bcode.ErrProjectDenyDeleteByApplication
	}

	count, err = p.ds.Count(ctx, &model.Target{Project: name}, nil)
	if err != nil {
		return err
	}
	if count > 0 {
		return bcode.ErrProjectDenyDeleteByTarget
	}

	count, err = p.ds.Count(ctx, &model.Env{Project: name}, nil)
	if err != nil {
		return err
	}
	if count > 0 {
		return bcode.ErrProjectDenyDeleteByEnvironment
	}

	users, _ := p.ListProjectUser(ctx, name, 0, 0)
	for _, user := range users.Users {
		err := p.DeleteProjectUser(ctx, name, user.UserName)
		if err != nil {
			return err
		}
	}

	roles, _ := p.rbacUsecase.ListRole(ctx, name, 0, 0)
	for _, role := range roles.Roles {
		err := p.rbacUsecase.DeleteRole(ctx, name, role.Name)
		if err != nil {
			return err
		}
	}

	permissions, _ := p.rbacUsecase.ListPermissions(ctx, name)
	for _, perm := range permissions {
		err := p.rbacUsecase.DeletePermission(ctx, name, perm.Name)
		if err != nil {
			return err
		}
	}
	return p.ds.Delete(ctx, &model.Project{Name: name})
}

// CreateProject create project
func (p *projectUsecaseImpl) CreateProject(ctx context.Context, req apisv1.CreateProjectRequest) (*apisv1.ProjectBase, error) {

	exist, err := p.ds.IsExist(ctx, &model.Project{Name: req.Name})
	if err != nil {
		log.Logger.Errorf("check project name is exist failure %s", err.Error())
		return nil, bcode.ErrProjectIsExist
	}
	if exist {
		return nil, bcode.ErrProjectIsExist
	}
	owner := req.Owner
	if owner == "" {
		loginUserName, ok := ctx.Value(&apisv1.CtxKeyUser).(string)
		if ok {
			owner = loginUserName
		}
	}
	var user = &model.User{Name: owner}
	if owner != "" {
		if err := p.ds.Get(ctx, user); err != nil {
			return nil, bcode.ErrProjectOwnerIsNotExist
		}
	}

	newProject := &model.Project{
		Name:        req.Name,
		Description: req.Description,
		Alias:       req.Alias,
		Owner:       owner,
	}

	if err := p.ds.Add(ctx, newProject); err != nil {
		return nil, err
	}

	if err := p.rbacUsecase.InitDefaultRoleAndUsersForProject(ctx, newProject); err != nil {
		log.Logger.Errorf("init default role and users for project failure %s", err.Error())
	}

	return ConvertProjectModel2Base(newProject, user), nil
}

// UpdateProject update project
func (p *projectUsecaseImpl) UpdateProject(ctx context.Context, projectName string, req apisv1.UpdateProjectRequest) (*apisv1.ProjectBase, error) {
	project, err := p.GetProject(ctx, projectName)
	if err != nil {
		return nil, err
	}
	project.Alias = req.Alias
	project.Description = req.Description
	var user = &model.User{Name: req.Owner}
	if req.Owner != "" {
		if err := p.ds.Get(ctx, user); err != nil {
			if errors.Is(err, datastore.ErrRecordNotExist) {
				return nil, bcode.ErrProjectOwnerIsNotExist
			}
			return nil, err
		}
		project.Owner = req.Owner
	}
	err = p.ds.Put(ctx, project)
	if err != nil {
		return nil, err
	}
	return ConvertProjectModel2Base(project, user), nil
}

func (p *projectUsecaseImpl) ListProjectUser(ctx context.Context, projectName string, page, pageSize int) (*apisv1.ListProjectUsersResponse, error) {
	var projectUser = model.ProjectUser{
		ProjectName: projectName,
	}
	entities, err := p.ds.List(ctx, &projectUser, &datastore.ListOptions{Page: page, PageSize: pageSize, SortBy: []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderDescending}}})
	if err != nil {
		return nil, err
	}
	var res apisv1.ListProjectUsersResponse
	for _, entity := range entities {
		res.Users = append(res.Users, ConvertProjectUserModel2Base(entity.(*model.ProjectUser)))
	}
	count, err := p.ds.Count(ctx, &projectUser, nil)
	if err != nil {
		return nil, err
	}
	res.Total = count
	return &res, nil
}

func (p *projectUsecaseImpl) AddProjectUser(ctx context.Context, projectName string, req apisv1.AddProjectUserRequest) (*apisv1.ProjectUserBase, error) {
	project, err := p.GetProject(ctx, projectName)
	if err != nil {
		return nil, err
	}
	// check user roles
	for _, role := range req.UserRoles {
		var projectUser = model.Role{
			Name:    role,
			Project: projectName,
		}
		if err := p.ds.Get(ctx, &projectUser); err != nil {
			return nil, bcode.ErrProjectRoleCheckFailure
		}
		if projectUser.Project != "" && projectUser.Project != projectName {
			return nil, bcode.ErrProjectRoleCheckFailure
		}
	}
	var projectUser = model.ProjectUser{
		Username:    req.UserName,
		ProjectName: project.Name,
		UserRoles:   req.UserRoles,
	}
	if err := p.ds.Add(ctx, &projectUser); err != nil {
		if errors.Is(err, datastore.ErrRecordExist) {
			return nil, bcode.ErrProjectUserExist
		}
		return nil, err
	}
	return ConvertProjectUserModel2Base(&projectUser), nil
}

func (p *projectUsecaseImpl) DeleteProjectUser(ctx context.Context, projectName string, userName string) error {
	project, err := p.GetProject(ctx, projectName)
	if err != nil {
		return err
	}
	var projectUser = model.ProjectUser{
		Username:    userName,
		ProjectName: project.Name,
	}
	if err := p.ds.Delete(ctx, &projectUser); err != nil {
		if errors.Is(err, datastore.ErrRecordExist) {
			return bcode.ErrProjectUserExist
		}
		return err
	}
	return nil
}

func (p *projectUsecaseImpl) UpdateProjectUser(ctx context.Context, projectName string, userName string, req apisv1.UpdateProjectUserRequest) (*apisv1.ProjectUserBase, error) {
	project, err := p.GetProject(ctx, projectName)
	if err != nil {
		return nil, err
	}
	// check user roles
	for _, role := range req.UserRoles {
		var projectUser = model.Role{
			Name:    role,
			Project: projectName,
		}
		if err := p.ds.Get(ctx, &projectUser); err != nil {
			return nil, bcode.ErrProjectRoleCheckFailure
		}
		if projectUser.Project != "" && projectUser.Project != projectName {
			return nil, bcode.ErrProjectRoleCheckFailure
		}
	}
	var projectUser = model.ProjectUser{
		Username:    userName,
		ProjectName: project.Name,
	}
	if err := p.ds.Get(ctx, &projectUser); err != nil {
		if errors.Is(err, datastore.ErrRecordExist) {
			return nil, bcode.ErrProjectUserExist
		}
		return nil, err
	}
	projectUser.UserRoles = req.UserRoles
	if err := p.ds.Put(ctx, &projectUser); err != nil {
		return nil, err
	}
	return ConvertProjectUserModel2Base(&projectUser), nil
}

func (p *projectUsecaseImpl) GetConfigs(ctx context.Context, projectName, configType string) ([]*apisv1.Config, error) {
	var (
		configs                  []*apisv1.Config
		terraformProviders       []*apisv1.Config
		legacyTerraformProviders []*apisv1.Config
		apps                     = &v1beta1.ApplicationList{}
	)
	if err := p.k8sClient.List(ctx, apps, client.InNamespace(types.DefaultKubeVelaNS),
		client.MatchingLabels{
			model.LabelSourceOfTruth: model.FromInner,
			types.LabelConfigCatalog: velaCoreConfig,
		}); err != nil {
		return nil, err
	}

	if configType == types.TerraformProvider || configType == "" {
		// legacy providers
		var providers = &terraformapi.ProviderList{}
		if err := p.k8sClient.List(ctx, providers, client.InNamespace(types.DefaultAppNamespace)); err != nil {
			return nil, err
		}
		for _, p := range providers.Items {
			if p.Labels[types.LabelConfigCatalog] == velaCoreConfig {
				continue
			}
			t := p.CreationTimestamp.Time
			legacyTerraformProviders = append(legacyTerraformProviders, &apisv1.Config{
				Name:        p.Name,
				CreatedTime: &t,
			})
		}
	}

	switch configType {
	case types.TerraformProvider:
		for _, a := range apps.Items {
			appProject := a.Labels[types.LabelConfigProject]
			if a.Status.Phase != common.ApplicationRunning || (appProject != "" && appProject != projectName) ||
				!strings.Contains(a.Labels[types.LabelConfigType], "terraform-") {
				continue
			}
			terraformProviders = append(terraformProviders, &apisv1.Config{
				ConfigType:  a.Labels[types.LabelConfigType],
				Name:        a.Name,
				Project:     appProject,
				CreatedTime: &(a.CreationTimestamp.Time),
			})
		}

		terraformProviders = append(terraformProviders, legacyTerraformProviders...)
		return terraformProviders, nil
	case "":
		for _, a := range apps.Items {
			appProject := a.Labels[types.LabelConfigProject]
			if appProject != "" && appProject != projectName {
				continue
			}
			configs = append(configs, &apisv1.Config{
				ConfigType:  a.Labels[types.LabelConfigType],
				Name:        a.Name,
				Project:     appProject,
				CreatedTime: &(a.CreationTimestamp.Time),
			})
		}
		configs = append(configs, legacyTerraformProviders...)
		return configs, nil
	case types.DexConnector, types.HelmRepository, types.ImageRegistry:
		t := strings.ReplaceAll(configType, "config-", "")
		for _, a := range apps.Items {
			appProject := a.Labels[types.LabelConfigProject]
			if a.Status.Phase != common.ApplicationRunning || (appProject != "" && appProject != projectName) {
				continue
			}
			if a.Labels[types.LabelConfigType] == t {
				configs = append(configs, &apisv1.Config{
					ConfigType:  a.Labels[types.LabelConfigType],
					Name:        a.Name,
					Project:     appProject,
					CreatedTime: &(a.CreationTimestamp.Time),
				})
			}
		}
		return configs, nil
	default:
		return nil, errors.New("unsupported config type")
	}
}

// ConvertProjectModel2Base convert project model to base struct
func ConvertProjectModel2Base(project *model.Project, owner *model.User) *apisv1.ProjectBase {
	base := &apisv1.ProjectBase{
		Name:        project.Name,
		Description: project.Description,
		Alias:       project.Alias,
		CreateTime:  project.CreateTime,
		UpdateTime:  project.UpdateTime,
		Owner:       apisv1.NameAlias{Name: project.Owner},
	}
	if owner != nil && owner.Name == project.Owner {
		base.Owner = apisv1.NameAlias{Name: owner.Name, Alias: owner.Alias}
	}
	return base
}

// ConvertProjectUserModel2Base convert project user model to base struct
func ConvertProjectUserModel2Base(user *model.ProjectUser) *apisv1.ProjectUserBase {
	base := &apisv1.ProjectUserBase{
		UserName:   user.Username,
		UserRoles:  user.UserRoles,
		CreateTime: user.CreateTime,
		UpdateTime: user.UpdateTime,
	}
	return base
}
