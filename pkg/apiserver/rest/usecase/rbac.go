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

type resourceMetadata struct {
	subResources map[string]resourceMetadata
	pathName     string
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
			"role":                {},
			"projectUser":         {},
			"applicationTemplate": {},
		},
		pathName: "projectName",
	},
	"cluster": {
		pathName: "clusterName",
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
	"user":          {},
	"role":          {},
	"systemSetting": {},
	"definition":    {},
}

var existResourcePaths = convert(ResourceMaps)

func checkResourcePath(resource string) (string, error) {
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

type rbacUsecase struct {
	ds datastore.DataStore
}

// RBACUsecase implement RBAC-related business logic.
type RBACUsecase interface {
	CheckPerm(resource string, actions ...string) func(req *restful.Request, res *restful.Response, chain *restful.FilterChain)
	GetUserPermPolicies(ctx context.Context, user *model.User, projectName string) ([]*model.PermPolicy, error)
	CreateRole(ctx context.Context, projectName string, req apisv1.CreateRoleRequest) (*apisv1.RoleBase, error)
	DeleteRole(ctx context.Context, projectName, roleName string) error
	UpdateRole(ctx context.Context, projectName, roleName string, req apisv1.UpdateRoleRequest) (*apisv1.RoleBase, error)
	ListRole(ctx context.Context, projectName string, page, pageSize int) (*apisv1.ListRolesResponse, error)
}

// NewRBACUsecase is the usecase service of RBAC
func NewRBACUsecase(ds datastore.DataStore) RBACUsecase {
	return &rbacUsecase{
		ds: ds,
	}
}

// registerResourceAction register resource actions
func registerResourceAction(resource string, actions ...string) {
	lock.Lock()
	defer lock.Unlock()
	if resourceActions == nil {
		resourceActions = make(map[string][]string)
	}
	if _, exist := ResourceMaps[resource]; !exist {
		path, err := checkResourcePath(resource)
		if err != nil {
			panic(fmt.Sprintf("resource %s is not exist", resource))
		}
		resource = path
	}
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

func (p *rbacUsecase) GetUserPermPolicies(ctx context.Context, user *model.User, projectName string) ([]*model.PermPolicy, error) {
	roles := user.UserRoles
	if projectName != "" {
		var projectUser = model.ProjectUser{
			ProjectName: projectName,
			Username:    user.Name,
		}
		if err := p.ds.Get(ctx, &projectUser); err == nil {
			roles = append(roles, projectUser.UserRoles...)
		}
	}
	entities, err := p.ds.List(ctx, &model.Role{}, &datastore.ListOptions{FilterOptions: datastore.FilterOptions{In: []datastore.InQueryOption{
		{
			Key:    "name",
			Values: roles,
		},
	}}})
	if err != nil {
		return nil, err
	}
	var permPolicyNames []string
	for _, entity := range entities {
		permPolicyNames = append(permPolicyNames, entity.(*model.Role).PermPolicies...)
	}
	return p.listPermPolices(ctx, projectName, permPolicyNames)
}

func (p *rbacUsecase) listPermPolices(ctx context.Context, projectName string, permPolicyNames []string) ([]*model.PermPolicy, error) {
	permEntities, err := p.ds.List(ctx, &model.PermPolicy{Project: projectName}, &datastore.ListOptions{FilterOptions: datastore.FilterOptions{In: []datastore.InQueryOption{
		{
			Key:    "name",
			Values: permPolicyNames,
		},
	}}})
	if err != nil {
		return nil, err
	}
	var perms []*model.PermPolicy
	for _, entity := range permEntities {
		perms = append(perms, entity.(*model.PermPolicy))
	}
	return perms, nil
}

func (p *rbacUsecase) CheckPerm(resource string, actions ...string) func(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
	registerResourceAction(resource, actions...)
	f := func(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
		// get login user info
		user, ok := req.Request.Context().Value(&apisv1.CtxKeyUser).(*model.User)
		if !ok {
			bcode.ReturnError(req, res, bcode.ErrUnauthorized)
			return
		}
		path, err := checkResourcePath(resource)
		if err != nil {
			log.Logger.Errorf("check resource path failure %s", err.Error())
			bcode.ReturnError(req, res, bcode.ErrForbidden)
			return
		}
		ra := &RequestResourceAction{}
		ra.SetResourceWithName(path, req)

		// get user's perm list.
		projectName := req.PathParameter("projectName")
		if projectName == "" {
			projectName = req.QueryParameter("project")
		}
		permPolicies, err := p.GetUserPermPolicies(req.Request.Context(), user, projectName)
		if err != nil {
			log.Logger.Errorf("get user's perm policies failure %s, user is %s", err.Error(), user.Name)
			bcode.ReturnError(req, res, bcode.ErrForbidden)
			return
		}
		if !ra.Match(permPolicies) {
			bcode.ReturnError(req, res, bcode.ErrForbidden)
			return
		}
		chain.ProcessFilter(req, res)
	}
	return f
}

func (p *rbacUsecase) CreateRole(ctx context.Context, projectName string, req apisv1.CreateRoleRequest) (*apisv1.RoleBase, error) {
	if projectName != "" {
		var project = model.Project{
			Name: projectName,
		}
		if err := p.ds.Get(ctx, &project); err != nil {
			return nil, bcode.ErrProjectIsNotExist
		}
	}
	if len(req.PermPolicies) == 0 {
		return nil, bcode.ErrRolePermPolicyCheckFailure
	}
	policies, err := p.listPermPolices(ctx, projectName, req.PermPolicies)
	if err != nil || len(policies) != len(req.PermPolicies) {
		return nil, bcode.ErrRolePermPolicyCheckFailure
	}
	var role = model.Role{
		Name:         req.Name,
		Alias:        req.Alias,
		Project:      projectName,
		PermPolicies: req.PermPolicies,
	}
	if err := p.ds.Add(ctx, &role); err != nil {
		if errors.Is(err, datastore.ErrRecordExist) {
			return nil, bcode.ErrRoleIsExist
		}
		return nil, err
	}
	return ConvertRole2Model(&role, policies), nil
}

func (p *rbacUsecase) DeleteRole(ctx context.Context, projectName, roleName string) error {
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

func (p *rbacUsecase) UpdateRole(ctx context.Context, projectName, roleName string, req apisv1.UpdateRoleRequest) (*apisv1.RoleBase, error) {
	if projectName != "" {
		var project = model.Project{
			Name: projectName,
		}
		if err := p.ds.Get(ctx, &project); err != nil {
			return nil, bcode.ErrProjectIsNotExist
		}
	}
	if len(req.PermPolicies) == 0 {
		return nil, bcode.ErrRolePermPolicyCheckFailure
	}
	policies, err := p.listPermPolices(ctx, projectName, req.PermPolicies)
	if err != nil || len(policies) != len(req.PermPolicies) {
		return nil, bcode.ErrRolePermPolicyCheckFailure
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
	role.PermPolicies = req.PermPolicies
	if err := p.ds.Put(ctx, &role); err != nil {
		return nil, err
	}
	return ConvertRole2Model(&role, policies), nil
}

func (p *rbacUsecase) ListRole(ctx context.Context, projectName string, page, pageSize int) (*apisv1.ListRolesResponse, error) {
	var role = model.Role{
		Project: projectName,
	}
	entities, err := p.ds.List(ctx, &role, &datastore.ListOptions{Page: page, PageSize: pageSize, SortBy: []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderDescending}}})
	if err != nil {
		return nil, err
	}
	var policySet = make(map[string]string)
	for _, entity := range entities {
		for _, p := range entity.(*model.Role).PermPolicies {
			policySet[p] = p
		}
	}
	policies, err := p.listPermPolices(ctx, projectName, utils.MapKey2Array(policySet))
	if err != nil {
		log.Logger.Errorf("list perm policies failure %s", err.Error())
	}
	var policyMap = make(map[string]*model.PermPolicy)
	for i, policy := range policies {
		policyMap[policy.Name] = policies[i]
	}

	var res apisv1.ListRolesResponse
	for _, entity := range entities {
		role := entity.(*model.Role)
		var rolePolicies []*model.PermPolicy
		for _, perm := range role.PermPolicies {
			rolePolicies = append(rolePolicies, policyMap[perm])
		}
		res.Roles = append(res.Roles, ConvertRole2Model(entity.(*model.Role), rolePolicies))
	}
	count, err := p.ds.Count(ctx, &role, nil)
	if err != nil {
		return nil, err
	}
	res.Total = count
	return &res, nil
}

// ConvertRole2Model convert role model to role base struct
func ConvertRole2Model(role *model.Role, policies []*model.PermPolicy) *apisv1.RoleBase {
	return &apisv1.RoleBase{
		CreateTime: role.CreateTime,
		UpdateTime: role.UpdateTime,
		Name:       role.Name,
		Alias:      role.Alias,
		PermPolicies: func() (list []apisv1.NameAlias) {
			for _, policy := range policies {
				list = append(list, apisv1.NameAlias{Name: policy.Name, Alias: policy.Alias})
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
func (r *ResourceName) Match(target *ResourceName) bool {
	current := r
	currentTarget := target
	for current != nil {
		if currentTarget == nil {
			return false
		}
		if current.Type == "*" {
			return true
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
	return true
}

// RequestResourceAction resource permission boundary
type RequestResourceAction struct {
	resource *ResourceName
	actions  []string
}

// PathParameter get path parameter interface
type PathParameter interface {
	PathParameter(name string) string
}

// SetResourceWithName format resource and assign a value from path parameter
func (r *RequestResourceAction) SetResourceWithName(resource string, req PathParameter) {
	resultKey := reg.FindAllString(resource, -1)
	for _, sourcekey := range resultKey {
		key := sourcekey[1 : len(sourcekey)-1]
		value := req.PathParameter(key)
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

func (r *RequestResourceAction) match(policy *model.PermPolicy) bool {
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
func (r *RequestResourceAction) Match(policies []*model.PermPolicy) bool {
	for _, policy := range policies {
		if r.match(policy) {
			if strings.EqualFold(policy.Effect, "allow") || strings.EqualFold(policy.Effect, "") {
				return true
			}
			if strings.EqualFold(policy.Effect, "deny") {
				return false
			}
		}
	}
	return false
}
