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
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/emicklei/go-restful/v3"

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/utils"
)

// resourceActions all register resources and actions
var resourceActions map[string][]string
var lock sync.Mutex
var reg = regexp.MustCompile(`(?U)\{.*\}`)

var defaultProjectPermissionTemplate = []*model.PermissionTemplate{
	{
		Name:      "project-read",
		Alias:     "Project Read",
		Resources: []string{"project:{projectName}"},
		Actions:   []string{"detail"},
		Effect:    "Allow",
		Scope:     "project",
	},
	{
		Name:      "app-management",
		Alias:     "App Management",
		Resources: []string{"project:{projectName}/application:*/*", "definition:*"},
		Actions:   []string{"*"},
		Effect:    "Allow",
		Scope:     "project",
	},
	{
		Name:      "env-management",
		Alias:     "Environment Management",
		Resources: []string{"project:{projectName}/environment:*"},
		Actions:   []string{"*"},
		Effect:    "Allow",
		Scope:     "project",
	},
	{
		Name:      "role-management",
		Alias:     "Role Management",
		Resources: []string{"project:{projectName}/role:*", "project:{projectName}/projectUser:*", "project:{projectName}/permission:*"},
		Actions:   []string{"*"},
		Effect:    "Allow",
		Scope:     "project",
	},
}

var defaultPlatformPermission = []*model.PermissionTemplate{
	{
		Name:      "cluster-management",
		Alias:     "Cluster Management",
		Resources: []string{"cluster:*/*"},
		Actions:   []string{"*"},
		Effect:    "Allow",
		Scope:     "platform",
	},
	{
		Name:      "project-management",
		Alias:     "Project Management",
		Resources: []string{"project:*"},
		Actions:   []string{"*"},
		Effect:    "Allow",
		Scope:     "platform",
	},
	{
		Name:      "addon-management",
		Alias:     "Addon Management",
		Resources: []string{"addon:*", "addonRegistry:*"},
		Actions:   []string{"*"},
		Effect:    "Allow",
		Scope:     "platform",
	},
	{
		Name:      "target-management",
		Alias:     "Target Management",
		Resources: []string{"target:*", "cluster:*/namespace:*"},
		Actions:   []string{"*"},
		Effect:    "Allow",
		Scope:     "platform",
	},
	{
		Name:      "user-management",
		Alias:     "User Management",
		Resources: []string{"user:*"},
		Actions:   []string{"*"},
		Effect:    "Allow",
		Scope:     "platform",
	},
	{
		Name:      "role-management",
		Alias:     "Platform Role Management",
		Resources: []string{"role:*", "permission:*"},
		Actions:   []string{"*"},
		Effect:    "Allow",
		Scope:     "platform",
	},
	{
		Name:      "admin",
		Alias:     "Admin",
		Resources: []string{"*"},
		Actions:   []string{"*"},
		Effect:    "Allow",
		Scope:     "platform",
	},
}

// ResourceMaps all resources definition for RBAC
var ResourceMaps = map[string]resourceMetadata{
	"project": {
		subResources: map[string]resourceMetadata{
			"application": {
				pathName: "appName",
				subResources: map[string]resourceMetadata{
					"component": {
						subResources: map[string]resourceMetadata{
							"trait": {
								pathName: "traitType",
							},
						},
						pathName: "compName",
					},
					"workflow": {
						subResources: map[string]resourceMetadata{
							"record": {
								pathName: "record",
							},
						},
						pathName: "workflowName",
					},
					"policy": {
						pathName: "policyName",
					},
					"revision": {
						pathName: "revision",
					},
					"envBinding": {
						pathName: "envName",
					},
					"trigger": {},
				},
			},
			"environment": {
				pathName: "envName",
			},
			"workflow": {
				pathName: "workflowName",
			},
			"role": {
				pathName: "roleName",
			},
			"permission": {},
			"projectUser": {
				pathName: "userName",
			},
			"applicationTemplate": {},
			"configType": {
				pathName: "configType",
			},
		},
		pathName: "projectName",
	},
	"cluster": {
		pathName: "clusterName",
		subResources: map[string]resourceMetadata{
			"namespace": {},
		},
	},
	"addon": {
		pathName: "addonName",
	},
	"addonRegistry": {
		pathName: "addonRegName",
	},
	"target": {
		pathName: "targetName",
	},
	"user": {
		pathName: "userName",
	},
	"role":          {},
	"permission":    {},
	"systemSetting": {},
	"definition":    {},
	"configType": {
		pathName: "configType",
		subResources: map[string]resourceMetadata{
			"config": {
				pathName: "name",
			},
		},
	},
}

var existResourcePaths = convert(ResourceMaps)

type resourceMetadata struct {
	subResources map[string]resourceMetadata
	pathName     string
}

func checkResourcePath(resource string) (string, error) {
	if sub, exist := ResourceMaps[resource]; exist {
		if sub.pathName != "" {
			return fmt.Sprintf("%s:{%s}", resource, sub.pathName), nil
		}
		return fmt.Sprintf("%s:*", resource), nil
	}
	path := ""
	exist := 0
	lastResourceName := resource[strings.LastIndex(resource, "/")+1:]
	for key, erp := range existResourcePaths {
		allMatchIndex := strings.Index(key, fmt.Sprintf("/%s/", resource))
		index := strings.Index(erp, fmt.Sprintf("/%s:", lastResourceName))
		if index > -1 && allMatchIndex > -1 {
			pre := erp[:index+len(lastResourceName)+2]
			next := strings.Replace(erp, pre, "", 1)
			nameIndex := strings.Index(next, "/")
			if nameIndex > -1 {
				pre += next[:nameIndex]
			}
			if pre != path {
				exist++
			}
			path = pre
		}
	}
	path = strings.TrimSuffix(path, "/")
	path = strings.TrimPrefix(path, "/")
	if exist == 1 {
		return path, nil
	}
	if exist > 1 {
		return path, fmt.Errorf("the resource name %s is not unique", resource)
	}
	return path, fmt.Errorf("there is no resource %s", resource)
}

func convert(sources map[string]resourceMetadata) map[string]string {
	list := make(map[string]string)
	for k, v := range sources {
		if len(v.subResources) > 0 {
			for sub, subWithPathName := range convert(v.subResources) {
				if subWithPathName != "" {
					withPathname := fmt.Sprintf("/%s:*%s", k, subWithPathName)
					if v.pathName != "" {
						withPathname = fmt.Sprintf("/%s:{%s}%s", k, v.pathName, subWithPathName)
					}
					list[fmt.Sprintf("/%s%s", k, sub)] = withPathname
				}
			}
		}
		withPathname := fmt.Sprintf("/%s:*/", k)
		if v.pathName != "" {
			withPathname = fmt.Sprintf("/%s:{%s}/", k, v.pathName)
		}
		list[fmt.Sprintf("/%s/", k)] = withPathname
	}
	return list
}

// registerResourceAction register resource actions
func registerResourceAction(resource string, actions ...string) {
	lock.Lock()
	defer lock.Unlock()
	if resourceActions == nil {
		resourceActions = make(map[string][]string)
	}
	path, err := checkResourcePath(resource)
	if err != nil {
		panic(fmt.Sprintf("resource %s is not exist", resource))
	}
	resource = path
	if _, exist := resourceActions[resource]; exist {
		for _, action := range actions {
			if !utils.StringsContain(resourceActions[resource], action) {
				resourceActions[resource] = append(resourceActions[resource], action)
			}
		}
	} else {
		resourceActions[resource] = actions
	}
}

type rbacUsecaseImpl struct {
	ds datastore.DataStore
}

// RBACUsecase implement RBAC-related business logic.
type RBACUsecase interface {
	CheckPerm(resource string, actions ...string) func(req *restful.Request, res *restful.Response, chain *restful.FilterChain)
	GetUserPermissions(ctx context.Context, user *model.User, projectName string, withPlatform bool) ([]*model.Permission, error)
	CreateRole(ctx context.Context, projectName string, req apisv1.CreateRoleRequest) (*apisv1.RoleBase, error)
	DeleteRole(ctx context.Context, projectName, roleName string) error
	UpdateRole(ctx context.Context, projectName, roleName string, req apisv1.UpdateRoleRequest) (*apisv1.RoleBase, error)
	ListRole(ctx context.Context, projectName string, page, pageSize int) (*apisv1.ListRolesResponse, error)
	ListPermissionTemplate(ctx context.Context, projectName string) ([]apisv1.PermissionTemplateBase, error)
	ListPermissions(ctx context.Context, projectName string) ([]apisv1.PermissionBase, error)
	DeletePermission(ctx context.Context, projectName, permName string) error
	InitDefaultRoleAndUsersForProject(ctx context.Context, project *model.Project) error
	Init(ctx context.Context) error
}

// NewRBACUsecase is the usecase service of RBAC
func NewRBACUsecase(ds datastore.DataStore) RBACUsecase {
	rbacUsecase := &rbacUsecaseImpl{
		ds: ds,
	}
	return rbacUsecase
}

func (p *rbacUsecaseImpl) Init(ctx context.Context) error {
	count, _ := p.ds.Count(ctx, &model.Permission{}, &datastore.FilterOptions{
		IsNotExist: []datastore.IsNotExistQueryOption{
			{
				Key: "project",
			},
		},
	})
	if count > 0 {
		return nil
	}
	var batchData []datastore.Entity
	for _, policy := range defaultPlatformPermission {
		batchData = append(batchData, &model.Permission{
			Name:      policy.Name,
			Alias:     policy.Alias,
			Resources: policy.Resources,
			Actions:   policy.Actions,
			Effect:    policy.Effect,
		})
	}
	batchData = append(batchData, &model.Role{
		Name:        "admin",
		Alias:       "Admin",
		Permissions: []string{"admin"},
	})
	if err := p.ds.BatchAdd(context.Background(), batchData); err != nil {
		return fmt.Errorf("init the platform perm policies failure %w", err)
	}
	return nil
}

// GetUserPermissions get user permission policies, if projectName is empty, will only get the platform permission policies
func (p *rbacUsecaseImpl) GetUserPermissions(ctx context.Context, user *model.User, projectName string, withPlatform bool) ([]*model.Permission, error) {
	var permissionNames []string
	var perms []*model.Permission
	if withPlatform && len(user.UserRoles) > 0 {
		entities, err := p.ds.List(ctx, &model.Role{}, &datastore.ListOptions{FilterOptions: datastore.FilterOptions{
			In: []datastore.InQueryOption{
				{
					Key:    "name",
					Values: user.UserRoles,
				},
			},
			IsNotExist: []datastore.IsNotExistQueryOption{
				{
					Key: "project",
				},
			},
		}})
		if err != nil {
			return nil, err
		}
		for _, entity := range entities {
			permissionNames = append(permissionNames, entity.(*model.Role).Permissions...)
		}
		perms, err = p.listPermPolices(ctx, "", permissionNames)
		if err != nil {
			return nil, err
		}
	}
	if projectName != "" {
		var projectUser = model.ProjectUser{
			ProjectName: projectName,
			Username:    user.Name,
		}
		var roles []string
		if err := p.ds.Get(ctx, &projectUser); err == nil {
			roles = append(roles, projectUser.UserRoles...)
		}
		if len(roles) > 0 {
			entities, err := p.ds.List(ctx, &model.Role{Project: projectName}, &datastore.ListOptions{FilterOptions: datastore.FilterOptions{In: []datastore.InQueryOption{
				{
					Key:    "name",
					Values: roles,
				},
			}}})
			if err != nil {
				return nil, err
			}
			for _, entity := range entities {
				permissionNames = append(permissionNames, entity.(*model.Role).Permissions...)
			}
			projectPerms, err := p.listPermPolices(ctx, projectName, permissionNames)
			if err != nil {
				return nil, err
			}
			perms = append(perms, projectPerms...)
		}
	}
	return perms, nil
}

func (p *rbacUsecaseImpl) UpdatePermission(ctx context.Context, projetName string, permissionName string, req *apisv1.UpdatePermissionRequest) (*apisv1.PermissionBase, error) {
	perm := &model.Permission{
		Project: projetName,
		Name:    permissionName,
	}
	err := p.ds.Get(ctx, perm)
	if err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, bcode.ErrPermissionNotExist
		}
	}
	//TODO: check req validate
	perm.Actions = req.Actions
	perm.Alias = req.Alias
	perm.Resources = req.Resources
	perm.Effect = req.Effect
	if err := p.ds.Put(ctx, perm); err != nil {
		return nil, err
	}
	return &apisv1.PermissionBase{
		Name:       perm.Name,
		Alias:      perm.Alias,
		Resources:  perm.Resources,
		Actions:    perm.Actions,
		Effect:     perm.Effect,
		CreateTime: perm.CreateTime,
		UpdateTime: perm.UpdateTime,
	}, nil
}

func (p *rbacUsecaseImpl) listPermPolices(ctx context.Context, projectName string, permissionNames []string) ([]*model.Permission, error) {
	if len(permissionNames) == 0 {
		return []*model.Permission{}, nil
	}
	filter := datastore.FilterOptions{In: []datastore.InQueryOption{
		{
			Key:    "name",
			Values: permissionNames,
		},
	}}
	if projectName == "" {
		filter.IsNotExist = append(filter.IsNotExist, datastore.IsNotExistQueryOption{
			Key: "project",
		})
	}
	permEntities, err := p.ds.List(ctx, &model.Permission{Project: projectName}, &datastore.ListOptions{FilterOptions: filter})
	if err != nil {
		return nil, err
	}
	var perms []*model.Permission
	for _, entity := range permEntities {
		perms = append(perms, entity.(*model.Permission))
	}
	return perms, nil
}

func (p *rbacUsecaseImpl) CheckPerm(resource string, actions ...string) func(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
	registerResourceAction(resource, actions...)
	f := func(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
		// get login user info
		userName, ok := req.Request.Context().Value(&apisv1.CtxKeyUser).(string)
		if !ok {
			bcode.ReturnError(req, res, bcode.ErrUnauthorized)
			return
		}
		user := &model.User{Name: userName}
		if err := p.ds.Get(req.Request.Context(), user); err != nil {
			bcode.ReturnError(req, res, bcode.ErrUnauthorized)
			return
		}
		path, err := checkResourcePath(resource)
		if err != nil {
			log.Logger.Errorf("check resource path failure %s", err.Error())
			bcode.ReturnError(req, res, bcode.ErrForbidden)
			return
		}

		// multiple method for get the project name.
		getProjectName := func() string {
			if value := req.PathParameter("projectName"); value != "" {
				return value
			}
			if value := req.QueryParameter("project"); value != "" {
				return value
			}
			if value := req.QueryParameter("projectName"); value != "" {
				return value
			}
			if appName := req.PathParameter(ResourceMaps["project"].subResources["application"].pathName); appName != "" {
				app := &model.Application{Name: appName}
				if err := p.ds.Get(req.Request.Context(), app); err == nil {
					return app.Project
				}
			}
			if envName := req.PathParameter(ResourceMaps["project"].subResources["environment"].pathName); envName != "" {
				env := &model.Env{Name: envName}
				if err := p.ds.Get(req.Request.Context(), env); err == nil {
					return env.Project
				}
			}
			return ""
		}

		ra := &RequestResourceAction{}
		ra.SetResourceWithName(path, func(name string) string {
			if name == ResourceMaps["project"].pathName {
				return getProjectName()
			}
			return req.PathParameter(name)
		})

		// get user's perm list.
		projectName := getProjectName()
		permissions, err := p.GetUserPermissions(req.Request.Context(), user, projectName, true)
		if err != nil {
			log.Logger.Errorf("get user's perm policies failure %s, user is %s", err.Error(), user.Name)
			bcode.ReturnError(req, res, bcode.ErrForbidden)
			return
		}
		if !ra.Match(permissions) {
			bcode.ReturnError(req, res, bcode.ErrForbidden)
			return
		}
		chain.ProcessFilter(req, res)
	}
	return f
}

func (p *rbacUsecaseImpl) CreateRole(ctx context.Context, projectName string, req apisv1.CreateRoleRequest) (*apisv1.RoleBase, error) {
	if projectName != "" {
		var project = model.Project{
			Name: projectName,
		}
		if err := p.ds.Get(ctx, &project); err != nil {
			return nil, bcode.ErrProjectIsNotExist
		}
	}
	if len(req.Permissions) == 0 {
		return nil, bcode.ErrRolePermissionCheckFailure
	}
	policies, err := p.listPermPolices(ctx, projectName, req.Permissions)
	if err != nil || len(policies) != len(req.Permissions) {
		return nil, bcode.ErrRolePermissionCheckFailure
	}
	var role = model.Role{
		Name:        req.Name,
		Alias:       req.Alias,
		Project:     projectName,
		Permissions: req.Permissions,
	}
	if err := p.ds.Add(ctx, &role); err != nil {
		if errors.Is(err, datastore.ErrRecordExist) {
			return nil, bcode.ErrRoleIsExist
		}
		return nil, err
	}
	return ConvertRole2Model(&role, policies), nil
}

func (p *rbacUsecaseImpl) DeleteRole(ctx context.Context, projectName, roleName string) error {
	var role = model.Role{
		Name:    roleName,
		Project: projectName,
	}
	if err := p.ds.Delete(ctx, &role); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return bcode.ErrRoleIsNotExist
		}
		return err
	}
	return nil
}

func (p *rbacUsecaseImpl) DeletePermission(ctx context.Context, projectName, permName string) error {
	var perm = model.Permission{
		Name:    permName,
		Project: projectName,
	}
	if err := p.ds.Delete(ctx, &perm); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return bcode.ErrRoleIsNotExist
		}
		return err
	}
	return nil
}

func (p *rbacUsecaseImpl) UpdateRole(ctx context.Context, projectName, roleName string, req apisv1.UpdateRoleRequest) (*apisv1.RoleBase, error) {
	if projectName != "" {
		var project = model.Project{
			Name: projectName,
		}
		if err := p.ds.Get(ctx, &project); err != nil {
			return nil, bcode.ErrProjectIsNotExist
		}
	}
	if len(req.Permissions) == 0 {
		return nil, bcode.ErrRolePermissionCheckFailure
	}
	policies, err := p.listPermPolices(ctx, projectName, req.Permissions)
	if err != nil || len(policies) != len(req.Permissions) {
		return nil, bcode.ErrRolePermissionCheckFailure
	}
	var role = model.Role{
		Name:    roleName,
		Project: projectName,
	}
	if err := p.ds.Get(ctx, &role); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, bcode.ErrRoleIsNotExist
		}
		return nil, err
	}
	role.Alias = req.Alias
	role.Permissions = req.Permissions
	if err := p.ds.Put(ctx, &role); err != nil {
		return nil, err
	}
	return ConvertRole2Model(&role, policies), nil
}

func (p *rbacUsecaseImpl) ListRole(ctx context.Context, projectName string, page, pageSize int) (*apisv1.ListRolesResponse, error) {
	var role = model.Role{
		Project: projectName,
	}
	var filter datastore.FilterOptions
	if projectName == "" {
		filter.IsNotExist = append(filter.IsNotExist, datastore.IsNotExistQueryOption{
			Key: "project",
		})
	}
	entities, err := p.ds.List(ctx, &role, &datastore.ListOptions{FilterOptions: filter, Page: page, PageSize: pageSize, SortBy: []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderDescending}}})
	if err != nil {
		return nil, err
	}
	var policySet = make(map[string]string)
	for _, entity := range entities {
		for _, p := range entity.(*model.Role).Permissions {
			policySet[p] = p
		}
	}

	policies, err := p.listPermPolices(ctx, projectName, utils.MapKey2Array(policySet))
	if err != nil {
		log.Logger.Errorf("list perm policies failure %s", err.Error())
	}
	var policyMap = make(map[string]*model.Permission)
	for i, policy := range policies {
		policyMap[policy.Name] = policies[i]
	}
	var res apisv1.ListRolesResponse
	for _, entity := range entities {
		role := entity.(*model.Role)
		var rolePolicies []*model.Permission
		for _, perm := range role.Permissions {
			rolePolicies = append(rolePolicies, policyMap[perm])
		}
		res.Roles = append(res.Roles, ConvertRole2Model(entity.(*model.Role), rolePolicies))
	}
	count, err := p.ds.Count(ctx, &role, &filter)
	if err != nil {
		return nil, err
	}
	res.Total = count
	return &res, nil
}

// ListPermissionTemplate TODO:
func (p *rbacUsecaseImpl) ListPermissionTemplate(ctx context.Context, projectName string) ([]apisv1.PermissionTemplateBase, error) {
	return nil, nil
}

func (p *rbacUsecaseImpl) ListPermissions(ctx context.Context, projectName string) ([]apisv1.PermissionBase, error) {
	var filter datastore.FilterOptions
	if projectName == "" {
		filter.IsNotExist = append(filter.IsNotExist, datastore.IsNotExistQueryOption{
			Key: "project",
		})
	}
	permEntities, err := p.ds.List(ctx, &model.Permission{Project: projectName}, &datastore.ListOptions{FilterOptions: filter})
	if err != nil {
		return nil, err
	}
	var perms []apisv1.PermissionBase
	for _, entity := range permEntities {
		perm := entity.(*model.Permission)
		perms = append(perms, apisv1.PermissionBase{
			Name:       perm.Name,
			Alias:      perm.Alias,
			Resources:  perm.Resources,
			Actions:    perm.Actions,
			Effect:     perm.Effect,
			CreateTime: perm.CreateTime,
			UpdateTime: perm.UpdateTime,
		})
	}
	return perms, nil
}

func (p *rbacUsecaseImpl) InitDefaultRoleAndUsersForProject(ctx context.Context, project *model.Project) error {
	var batchData []datastore.Entity
	for _, permissionTemp := range defaultProjectPermissionTemplate {
		var rra = RequestResourceAction{}
		var formatedResource []string
		for _, resource := range permissionTemp.Resources {
			rra.SetResourceWithName(resource, func(name string) string {
				if name == ResourceMaps["project"].pathName {
					return project.Name
				}
				return ""
			})
			formatedResource = append(formatedResource, rra.GetResource().String())
		}
		batchData = append(batchData, &model.Permission{
			Name:      permissionTemp.Name,
			Alias:     permissionTemp.Alias,
			Project:   project.Name,
			Resources: formatedResource,
			Actions:   permissionTemp.Actions,
			Effect:    permissionTemp.Effect,
		})
	}
	batchData = append(batchData, &model.Role{
		Name:        "app-developer",
		Alias:       "App Developer",
		Permissions: []string{"project-read", "app-management", "env-management"},
		Project:     project.Name,
	}, &model.Role{
		Name:        "project-admin",
		Alias:       "Project Admin",
		Permissions: []string{"project-read", "app-management", "env-management", "role-management"},
		Project:     project.Name,
	})
	if project.Owner != "" {
		var projectUser = &model.ProjectUser{
			ProjectName: project.Name,
			UserRoles:   []string{"project-admin"},
			Username:    project.Owner,
		}
		batchData = append(batchData, projectUser)
	}
	return p.ds.BatchAdd(ctx, batchData)
}

// ConvertRole2Model convert role model to role base struct
func ConvertRole2Model(role *model.Role, policies []*model.Permission) *apisv1.RoleBase {
	return &apisv1.RoleBase{
		CreateTime: role.CreateTime,
		UpdateTime: role.UpdateTime,
		Name:       role.Name,
		Alias:      role.Alias,
		Permissions: func() (list []apisv1.NameAlias) {
			for _, policy := range policies {
				if policy != nil {
					list = append(list, apisv1.NameAlias{Name: policy.Name, Alias: policy.Alias})
				}
			}
			return
		}(),
	}
}

// ResourceName it is similar to ARNs
// <type>:<value>/<type>:<value>
type ResourceName struct {
	Type  string
	Value string
	Next  *ResourceName
}

// ParseResourceName parse string to ResourceName
func ParseResourceName(resource string) *ResourceName {
	RNs := strings.Split(resource, "/")
	var resourceName = ResourceName{}
	var current = &resourceName
	for _, rn := range RNs {
		rnData := strings.Split(rn, ":")
		if len(rnData) == 2 {
			current.Type = rnData[0]
			current.Value = rnData[1]
		}
		if len(rnData) == 1 {
			current.Type = rnData[0]
			current.Value = "*"
		}
		var next = &ResourceName{}
		current.Next = next
		current = next
	}
	return &resourceName
}

// Match the resource types same with target and resource value include
// target resource means request resources
func (r *ResourceName) Match(target *ResourceName) bool {
	current := r
	currentTarget := target
	for current != nil && current.Type != "" {
		if current.Type == "*" {
			return true
		}
		if currentTarget == nil || currentTarget.Type == "" {
			return false
		}
		if current.Type != currentTarget.Type {
			return false
		}
		if current.Value != currentTarget.Value && current.Value != "*" {
			return false
		}
		current = current.Next
		currentTarget = currentTarget.Next
	}
	if currentTarget != nil && currentTarget.Type != "" {
		return false
	}
	return true
}

func (r *ResourceName) String() string {
	strBuilder := &strings.Builder{}
	current := r
	for current != nil && current.Type != "" {
		strBuilder.WriteString(fmt.Sprintf("%s:%s/", current.Type, current.Value))
		current = current.Next
	}
	return strings.TrimSuffix(strBuilder.String(), "/")
}

// RequestResourceAction resource permission boundary
type RequestResourceAction struct {
	resource *ResourceName
	actions  []string
}

// SetResourceWithName format resource and assign a value from path parameter
func (r *RequestResourceAction) SetResourceWithName(resource string, pathParameter func(name string) string) {
	resultKey := reg.FindAllString(resource, -1)
	for _, sourcekey := range resultKey {
		key := sourcekey[1 : len(sourcekey)-1]
		value := pathParameter(key)
		if value == "" {
			value = "*"
		}
		resource = strings.Replace(resource, sourcekey, value, 1)
	}
	r.resource = ParseResourceName(resource)
}

// GetResource return the resource after be formated
func (r *RequestResourceAction) GetResource() *ResourceName {
	return r.resource
}

// SetActions set request actions
func (r *RequestResourceAction) SetActions(actions []string) {
	r.actions = actions
}

func (r *RequestResourceAction) match(policy *model.Permission) bool {
	// match actions, the policy actions will include the actions of request
	if !utils.SliceIncludeSlice(policy.Actions, r.actions) && !utils.StringsContain(policy.Actions, "*") {
		return false
	}
	// match resources
	for _, resource := range policy.Resources {
		resourceName := ParseResourceName(resource)
		if resourceName.Match(r.resource) {
			return true
		}
	}
	return false
}

// Match determines whether the request resources and actions matches the user permission set.
func (r *RequestResourceAction) Match(policies []*model.Permission) bool {
	for _, policy := range policies {
		if strings.EqualFold(policy.Effect, "deny") {
			if r.match(policy) {
				return false
			}
		}
	}
	for _, policy := range policies {
		if strings.EqualFold(policy.Effect, "allow") || policy.Effect == "" {
			if r.match(policy) {
				return true
			}
		}
	}
	return false
}
