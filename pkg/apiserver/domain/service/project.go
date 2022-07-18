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

package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	terraformtypes "github.com/oam-dev/terraform-controller/api/types"
	terraformapi "github.com/oam-dev/terraform-controller/api/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
	"github.com/oam-dev/kubevela/pkg/multicluster"
)

// ProjectService project manage service.
type ProjectService interface {
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

type projectServiceImpl struct {
	Store         datastore.DataStore `inject:"datastore"`
	K8sClient     client.Client       `inject:"kubeClient"`
	RbacService   RBACService         `inject:""`
	TargetService TargetService       `inject:""`
	UserService   UserService         `inject:""`
	EnvService    EnvService          `inject:""`
}

// NewProjectService new project service
func NewProjectService() ProjectService {
	return &projectServiceImpl{}
}

// Init init default data
func (p *projectServiceImpl) Init(ctx context.Context) error {
	return p.InitDefaultProjectEnvTarget(ctx, model.DefaultInitNamespace)
}

// initDefaultProjectEnvTarget will initialize a default project with a default env that contain a default target
// the default env and default target both using the `default` namespace in control plane cluster
func (p *projectServiceImpl) InitDefaultProjectEnvTarget(ctx context.Context, defaultNamespace string) error {
	var project = model.Project{}
	entities, err := p.Store.List(ctx, &project, &datastore.ListOptions{FilterOptions: datastore.FilterOptions{
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
			if err := p.Store.Put(ctx, pro); err != nil {
				return err
			}
			// owner is empty, it is old data
			if init {
				if err := p.RbacService.InitDefaultRoleAndUsersForProject(ctx, pro); err != nil {
					return fmt.Errorf("init default role and users for project %s failure %w", pro.Name, err)
				}
			}
		}
		return nil
	}

	count, _ := p.Store.Count(ctx, &project, nil)
	if count > 0 {
		return nil
	}
	log.Logger.Info("no default project found, adding a default project with default env and target")

	_, err = p.CreateProject(ctx, apisv1.CreateProjectRequest{
		Name:        model.DefaultInitName,
		Alias:       "Default",
		Description: model.DefaultProjectDescription,
		Owner:       model.DefaultAdminUserName,
	})
	if err != nil {
		return fmt.Errorf("initialize project failed %w", err)
	}

	// initialize default target first
	_, err = p.TargetService.CreateTarget(ctx, apisv1.CreateTargetRequest{
		Name:        model.DefaultInitName,
		Alias:       "Default",
		Description: model.DefaultTargetDescription,
		Project:     model.DefaultInitName,
		Cluster: &apisv1.ClusterTarget{
			ClusterName: multicluster.ClusterLocalName,
			Namespace:   defaultNamespace,
		},
	})

	// for idempotence, ignore default target already exist error
	if err != nil && errors.Is(err, bcode.ErrTargetExist) {
		return fmt.Errorf("initialize default target failed %w", err)
	}

	// initialize default target first
	_, err = p.EnvService.CreateEnv(ctx, apisv1.CreateEnvRequest{
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
	return nil
}

// GetProject get project
func (p *projectServiceImpl) GetProject(ctx context.Context, projectName string) (*model.Project, error) {
	project := &model.Project{Name: projectName}
	if err := p.Store.Get(ctx, project); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, bcode.ErrProjectIsNotExist
		}
		return nil, err
	}
	return project, nil
}

func (p *projectServiceImpl) DetailProject(ctx context.Context, projectName string) (*apisv1.ProjectBase, error) {
	project, err := p.GetProject(ctx, projectName)
	if err != nil {
		return nil, err
	}
	var user = &model.User{Name: project.Owner}
	if project.Owner != "" {
		if err := p.Store.Get(ctx, user); err != nil {
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

func (p *projectServiceImpl) ListUserProjects(ctx context.Context, userName string) ([]*apisv1.ProjectBase, error) {
	var projectUser = model.ProjectUser{
		Username: userName,
	}
	entities, err := p.Store.List(ctx, &projectUser, nil)
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
	projectEntities, err := p.Store.List(ctx, &model.Project{}, &datastore.ListOptions{FilterOptions: datastore.FilterOptions{In: []datastore.InQueryOption{{
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
func (p *projectServiceImpl) ListProjects(ctx context.Context, page, pageSize int) (*apisv1.ListProjectResponse, error) {
	return listProjects(ctx, p.Store, page, pageSize)
}

// DeleteProject delete a project
func (p *projectServiceImpl) DeleteProject(ctx context.Context, name string) error {
	_, err := p.GetProject(ctx, name)
	if err != nil {
		return err
	}

	count, err := p.Store.Count(ctx, &model.Application{Project: name}, nil)
	if err != nil {
		return err
	}
	if count > 0 {
		return bcode.ErrProjectDenyDeleteByApplication
	}

	count, err = p.Store.Count(ctx, &model.Target{Project: name}, nil)
	if err != nil {
		return err
	}
	if count > 0 {
		return bcode.ErrProjectDenyDeleteByTarget
	}

	count, err = p.Store.Count(ctx, &model.Env{Project: name}, nil)
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

	roles, _ := p.RbacService.ListRole(ctx, name, 0, 0)
	for _, role := range roles.Roles {
		err := p.RbacService.DeleteRole(ctx, name, role.Name)
		if err != nil {
			return err
		}
	}

	permissions, _ := p.RbacService.ListPermissions(ctx, name)
	for _, perm := range permissions {
		err := p.RbacService.DeletePermission(ctx, name, perm.Name)
		if err != nil {
			return err
		}
	}
	if err := p.Store.Delete(ctx, &model.Project{Name: name}); err != nil {
		return err
	}
	// delete config-sync application
	return destroySyncConfigsApp(ctx, p.K8sClient, name)
}

// CreateProject create project
func (p *projectServiceImpl) CreateProject(ctx context.Context, req apisv1.CreateProjectRequest) (*apisv1.ProjectBase, error) {

	exist, err := p.Store.IsExist(ctx, &model.Project{Name: req.Name})
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
		if err := p.Store.Get(ctx, user); err != nil {
			return nil, bcode.ErrProjectOwnerIsNotExist
		}
	}

	newProject := &model.Project{
		Name:        req.Name,
		Description: req.Description,
		Alias:       req.Alias,
		Owner:       owner,
	}

	if err := p.Store.Add(ctx, newProject); err != nil {
		return nil, err
	}

	if err := p.RbacService.InitDefaultRoleAndUsersForProject(ctx, newProject); err != nil {
		log.Logger.Errorf("init default role and users for project failure %s", err.Error())
	}

	return ConvertProjectModel2Base(newProject, user), nil
}

// UpdateProject update project
func (p *projectServiceImpl) UpdateProject(ctx context.Context, projectName string, req apisv1.UpdateProjectRequest) (*apisv1.ProjectBase, error) {
	project, err := p.GetProject(ctx, projectName)
	if err != nil {
		return nil, err
	}
	project.Alias = req.Alias
	project.Description = req.Description
	var user = &model.User{Name: req.Owner}
	if req.Owner != "" {
		if err := p.Store.Get(ctx, user); err != nil {
			if errors.Is(err, datastore.ErrRecordNotExist) {
				return nil, bcode.ErrProjectOwnerIsNotExist
			}
			return nil, err
		}
		if _, err := p.AddProjectUser(ctx, projectName, apisv1.AddProjectUserRequest{
			UserName:  req.Owner,
			UserRoles: []string{"project-admin"},
		}); err != nil && !errors.Is(err, bcode.ErrProjectUserExist) {
			return nil, err
		}
		project.Owner = req.Owner
	}
	err = p.Store.Put(ctx, project)
	if err != nil {
		return nil, err
	}
	return ConvertProjectModel2Base(project, user), nil
}

func (p *projectServiceImpl) ListProjectUser(ctx context.Context, projectName string, page, pageSize int) (*apisv1.ListProjectUsersResponse, error) {
	var projectUser = model.ProjectUser{
		ProjectName: projectName,
	}
	entities, err := p.Store.List(ctx, &projectUser, &datastore.ListOptions{Page: page, PageSize: pageSize, SortBy: []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderDescending}}})
	if err != nil {
		return nil, err
	}
	var res apisv1.ListProjectUsersResponse
	for _, entity := range entities {
		res.Users = append(res.Users, ConvertProjectUserModel2Base(entity.(*model.ProjectUser)))
	}
	count, err := p.Store.Count(ctx, &projectUser, nil)
	if err != nil {
		return nil, err
	}
	res.Total = count
	return &res, nil
}

func (p *projectServiceImpl) AddProjectUser(ctx context.Context, projectName string, req apisv1.AddProjectUserRequest) (*apisv1.ProjectUserBase, error) {
	project, err := p.GetProject(ctx, projectName)
	if err != nil {
		return nil, err
	}
	_, err = p.UserService.GetUser(ctx, req.UserName)
	if err != nil {
		return nil, err
	}
	// check user roles
	for _, role := range req.UserRoles {
		var projectUser = model.Role{
			Name:    role,
			Project: projectName,
		}
		if err := p.Store.Get(ctx, &projectUser); err != nil {
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
	if err := p.Store.Add(ctx, &projectUser); err != nil {
		if errors.Is(err, datastore.ErrRecordExist) {
			return nil, bcode.ErrProjectUserExist
		}
		return nil, err
	}
	return ConvertProjectUserModel2Base(&projectUser), nil
}

func (p *projectServiceImpl) DeleteProjectUser(ctx context.Context, projectName string, userName string) error {
	project, err := p.GetProject(ctx, projectName)
	if err != nil {
		return err
	}
	var projectUser = model.ProjectUser{
		Username:    userName,
		ProjectName: project.Name,
	}
	if err := p.Store.Delete(ctx, &projectUser); err != nil {
		if errors.Is(err, datastore.ErrRecordExist) {
			return bcode.ErrProjectUserExist
		}
		return err
	}
	return nil
}

func (p *projectServiceImpl) UpdateProjectUser(ctx context.Context, projectName string, userName string, req apisv1.UpdateProjectUserRequest) (*apisv1.ProjectUserBase, error) {
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
		if err := p.Store.Get(ctx, &projectUser); err != nil {
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
	if err := p.Store.Get(ctx, &projectUser); err != nil {
		if errors.Is(err, datastore.ErrRecordExist) {
			return nil, bcode.ErrProjectUserExist
		}
		return nil, err
	}
	projectUser.UserRoles = req.UserRoles
	if err := p.Store.Put(ctx, &projectUser); err != nil {
		return nil, err
	}
	return ConvertProjectUserModel2Base(&projectUser), nil
}

func (p *projectServiceImpl) GetConfigs(ctx context.Context, projectName, configType string) ([]*apisv1.Config, error) {
	var (
		configs                  []*apisv1.Config
		legacyTerraformProviders []*apisv1.Config
		apps                     = &v1beta1.ApplicationList{}
	)
	if err := p.K8sClient.List(ctx, apps, client.InNamespace(types.DefaultKubeVelaNS),
		client.MatchingLabels{
			model.LabelSourceOfTruth: model.FromInner,
			types.LabelConfigCatalog: types.VelaCoreConfig,
		}); err != nil {
		return nil, err
	}

	if configType == types.TerraformProvider || configType == "" {
		// legacy providers
		var providers = &terraformapi.ProviderList{}
		if err := p.K8sClient.List(ctx, providers, client.InNamespace(types.DefaultAppNamespace)); err != nil {
			// this logic depends on the terraform addon, ignore the no matches kind error before the terraform addon is installed.
			if !meta.IsNoMatchError(err) {
				return nil, err
			}
			log.Logger.Infof("terraform Provider CRD is not installed")
		}
		for _, p := range providers.Items {
			if p.Labels[types.LabelConfigCatalog] == types.VelaCoreConfig {
				continue
			}
			t := p.CreationTimestamp.Time
			var status = configIsNotReady
			if p.Status.State == terraformtypes.ProviderIsReady {
				status = configIsReady
			}
			legacyTerraformProviders = append(legacyTerraformProviders, &apisv1.Config{
				Name:        p.Name,
				CreatedTime: &t,
				Status:      status,
			})
		}
	}

	switch configType {
	case types.TerraformProvider:
		for _, a := range apps.Items {
			appProject := a.Labels[types.LabelConfigProject]
			if a.Status.Phase != common.ApplicationRunning || (appProject != "" && appProject != projectName) ||
				!strings.Contains(a.Labels[types.LabelConfigType], types.TerraformComponentPrefix) {
				continue
			}
			configs = append(configs, retrieveConfigFromApplication(a, appProject))
		}

		configs = append(configs, legacyTerraformProviders...)
	case "":
		for _, a := range apps.Items {
			appProject := a.Labels[types.LabelConfigProject]
			if appProject != "" && appProject != projectName {
				continue
			}
			configs = append(configs, retrieveConfigFromApplication(a, appProject))
		}
		configs = append(configs, legacyTerraformProviders...)
	case types.DexConnector, types.HelmRepository, types.ImageRegistry:
		t := strings.ReplaceAll(configType, "config-", "")
		for _, a := range apps.Items {
			appProject := a.Labels[types.LabelConfigProject]
			if a.Status.Phase != common.ApplicationRunning || (appProject != "" && appProject != projectName) {
				continue
			}
			if a.Labels[types.LabelConfigType] == t {
				configs = append(configs, retrieveConfigFromApplication(a, appProject))
			}
		}
	default:
		return nil, errors.New("unsupported config type")
	}

	for i, c := range configs {
		if c.ConfigType != "" {
			d := &v1beta1.ComponentDefinition{}
			err := p.K8sClient.Get(ctx, client.ObjectKey{Namespace: types.DefaultKubeVelaNS, Name: c.ConfigType}, d)
			if err != nil {
				klog.InfoS("failed to get component definition", "ComponentDefinition", configType, "err", err)
			} else {
				configs[i].ConfigTypeAlias = DefinitionAlias(d.Annotations)
			}
		}
	}
	return configs, nil
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

func retrieveConfigFromApplication(a v1beta1.Application, project string) *apisv1.Config {
	var (
		applicationStatus = a.Status.Phase
		status            string
	)
	if applicationStatus == common.ApplicationRunning {
		status = configIsReady
	} else {
		status = configIsNotReady
	}
	return &apisv1.Config{
		ConfigType:        a.Labels[types.LabelConfigType],
		Name:              a.Name,
		Project:           project,
		CreatedTime:       &(a.CreationTimestamp.Time),
		ApplicationStatus: applicationStatus,
		Status:            status,
		Alias:             a.Annotations[types.AnnotationConfigAlias],
		Description:       a.Annotations[types.AnnotationConfigDescription],
	}
}

// NewTestProjectService create the project service instance for testing
func NewTestProjectService(ds datastore.DataStore, c client.Client) ProjectService {
	targetImpl := &targetServiceImpl{K8sClient: c, Store: ds}
	envImpl := &envServiceImpl{KubeClient: c, Store: ds}
	rbacService := &rbacServiceImpl{Store: ds}
	userService := &userServiceImpl{Store: ds, RbacService: rbacService, SysService: systemInfoServiceImpl{Store: ds}}
	projectService := &projectServiceImpl{
		K8sClient:     c,
		Store:         ds,
		RbacService:   rbacService,
		TargetService: targetImpl,
		UserService:   userService,
		EnvService:    envImpl,
	}
	userService.ProjectService = projectService
	envImpl.ProjectService = projectService
	return projectService
}
