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

package api

import (
	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/service"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	pkgconfig "github.com/oam-dev/kubevela/pkg/config"
)

type project struct {
	RbacService        service.RBACService        `inject:""`
	ProjectService     service.ProjectService     `inject:""`
	TargetService      service.TargetService      `inject:""`
	ConfigService      service.ConfigService      `inject:""`
	PipelineService    service.PipelineService    `inject:""`
	PipelineRunService service.PipelineRunService `inject:""`
	ContextService     service.ContextService     `inject:""`
	RBACService        service.RBACService        `inject:""`
}

// NewProject new project
func NewProject() Interface {
	return &project{}
}

func (n *project) GetWebServiceRoute() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/projects").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for project manage")

	tags := []string{"project"}

	ws.Route(ws.GET("/").To(n.listProjects).
		Doc("list all projects").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(n.RbacService.CheckPerm("project", "list")).
		Returns(200, "OK", apis.ListProjectResponse{}).
		Writes(apis.ListProjectResponse{}))

	ws.Route(ws.POST("/").To(n.createProject).
		Doc("create a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(n.RbacService.CheckPerm("project", "create")).
		Reads(apis.CreateProjectRequest{}).
		Returns(200, "OK", apis.ProjectBase{}).
		Writes(apis.ProjectBase{}))

	ws.Route(ws.GET("/{projectName}").To(n.detailProject).
		Doc("detail a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string")).
		Filter(n.RbacService.CheckPerm("project", "detail")).
		Returns(200, "OK", apis.ProjectBase{}).
		Writes(apis.ProjectBase{}))

	ws.Route(ws.PUT("/{projectName}").To(n.updateProject).
		Doc("update a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string")).
		Filter(n.RbacService.CheckPerm("project", "update")).
		Reads(apis.UpdateProjectRequest{}).
		Returns(200, "OK", apis.ProjectBase{}).
		Writes(apis.ProjectBase{}))

	ws.Route(ws.DELETE("/{projectName}").To(n.deleteProject).
		Doc("delete a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string")).
		Filter(n.RbacService.CheckPerm("project", "delete")).
		Returns(200, "OK", apis.EmptyResponse{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.GET("/{projectName}/targets").To(n.listProjectTargets).
		Doc("get targets list belong to a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string")).
		Filter(n.RbacService.CheckPerm("project", "detail")).
		Returns(200, "OK", apis.EmptyResponse{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.POST("/{projectName}/users").To(n.createProjectUser).
		Doc("add a user to a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string")).
		Filter(n.RbacService.CheckPerm("project/projectUser", "create")).
		Reads(apis.AddProjectUserRequest{}).
		Returns(200, "OK", apis.ProjectUserBase{}).
		Writes(apis.ProjectUserBase{}))

	ws.Route(ws.GET("/{projectName}/users").To(n.listProjectUser).
		Doc("list all users belong to a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string")).
		Filter(n.RbacService.CheckPerm("project/projectUser", "list")).
		Returns(200, "OK", apis.ListProjectUsersResponse{}).
		Writes(apis.ListProjectUsersResponse{}))

	ws.Route(ws.PUT("/{projectName}/users/{userName}").To(n.updateProjectUser).
		Doc("update a user from a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.UpdateProjectUserRequest{}).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string")).
		Param(ws.PathParameter("userName", "identifier of the project user").DataType("string")).
		Filter(n.RbacService.CheckPerm("project/projectUser", "create")).
		Returns(200, "OK", apis.ProjectUserBase{}).
		Writes(apis.ProjectUserBase{}))

	ws.Route(ws.DELETE("/{projectName}/users/{userName}").To(n.deleteProjectUser).
		Doc("delete a user from a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.UpdateProjectUserRequest{}).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string")).
		Param(ws.PathParameter("userName", "identifier of the project user").DataType("string")).
		Filter(n.RbacService.CheckPerm("project/projectUser", "delete")).
		Returns(200, "OK", apis.EmptyResponse{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.GET("/{projectName}/roles").To(n.listProjectRoles).
		Doc("list all project level roles").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string")).
		Filter(n.RbacService.CheckPerm("project/role", "list")).
		Returns(200, "OK", apis.ListRolesResponse{}).
		Writes(apis.ListRolesResponse{}))

	ws.Route(ws.POST("/{projectName}/roles").To(n.createProjectRole).
		Doc("create project level role").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string")).
		Filter(n.RbacService.CheckPerm("project/role", "create")).
		Returns(200, "OK", apis.RoleBase{}).
		Reads(apis.CreateRoleRequest{}).
		Writes(apis.RoleBase{}))

	ws.Route(ws.PUT("/{projectName}/roles/{roleName}").To(n.updateProjectRole).
		Doc("update project level role").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string")).
		Param(ws.PathParameter("roleName", "identifier of the project role").DataType("string")).
		Filter(n.RbacService.CheckPerm("project/role", "update")).
		Reads(apis.UpdateRoleRequest{}).
		Returns(200, "OK", apis.RoleBase{}).
		Writes(apis.RoleBase{}))

	ws.Route(ws.DELETE("/{projectName}/roles/{roleName}").To(n.deleteProjectRole).
		Doc("delete project level role").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string")).
		Param(ws.PathParameter("roleName", "identifier of the project role").DataType("string")).
		Filter(n.RbacService.CheckPerm("project/role", "delete")).
		Returns(200, "OK", apis.EmptyResponse{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.GET("/{projectName}/permissions").To(n.listProjectPermissions).
		Doc("list all project level perm policies").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string")).
		Filter(n.RbacService.CheckPerm("project/permission", "list")).
		Returns(200, "OK", []apis.PermissionBase{}).
		Writes([]apis.PermissionBase{}))

	ws.Route(ws.POST("/{projectName}/permissions").To(n.createProjectPermission).
		Doc("create a project level perm policy").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string")).
		Filter(n.RbacService.CheckPerm("project/permission", "list")).
		Returns(200, "OK", []apis.PermissionBase{}).
		Writes([]apis.PermissionBase{}))

	ws.Route(ws.DELETE("/{projectName}/permissions/{permissionName}").To(n.deleteProjectPermission).
		Doc("delete a project level perm policy").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string")).
		Param(ws.PathParameter("permissionName", "identifier of the permission").DataType("string")).
		Filter(n.RbacService.CheckPerm("project/permission", "list")).
		Returns(200, "OK", []apis.PermissionBase{}).
		Writes([]apis.PermissionBase{}))

	ws.Route(ws.GET("/{projectName}/config_templates").To(n.getConfigTemplates).
		Doc("get the templates which are in a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(n.RbacService.CheckPerm("project/config", "list")).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string").Required(true)).
		Param(ws.QueryParameter("namespace", "the namespace of the template").DataType("string").Required(true)).
		Returns(200, "OK", apis.ListConfigTemplateResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ListConfigTemplateResponse{}))

	ws.Route(ws.GET("/{projectName}/config_templates/{templateName}").To(n.getConfigTemplate).
		Doc("Detail a template").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(n.RbacService.CheckPerm("project/config", "get")).
		Param(ws.PathParameter("templateName", "identifier of the config template").DataType("string")).
		Param(ws.QueryParameter("namespace", "the name of the namespace").DataType("string")).
		Returns(200, "OK", apis.ConfigTemplateDetail{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ConfigTemplateDetail{}))

	ws.Route(ws.GET("/{projectName}/configs").To(n.getConfigs).
		Doc("get configs which are in a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(n.RbacService.CheckPerm("project/config", "list")).
		Param(ws.QueryParameter("template", "the template name").DataType("string")).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string").Required(true)).
		Returns(200, "OK", apis.ListConfigResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ListConfigResponse{}))

	ws.Route(ws.POST("/{projectName}/configs").To(n.createConfig).
		Doc("create a config in a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(n.RbacService.CheckPerm("project/config", "list")).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string").Required(true)).
		Reads(apis.CreateConfigRequest{}).
		Returns(200, "OK", apis.Config{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.Config{}))

	ws.Route(ws.DELETE("/{projectName}/configs/{configName}").To(n.deleteConfig).
		Doc("delete a config from a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(n.RbacService.CheckPerm("project/config", "list")).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string").Required(true)).
		Param(ws.PathParameter("configName", "identifier of the config").DataType("string").Required(true)).
		Returns(200, "OK", apis.EmptyResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.PUT("/{projectName}/configs/{configName}").To(n.updateConfig).
		Doc("update a config in a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(n.RbacService.CheckPerm("project/config", "list")).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string").Required(true)).
		Param(ws.PathParameter("configName", "identifier of the config").DataType("string").Required(true)).
		Returns(200, "OK", apis.Config{}).
		Reads(apis.UpdateConfigRequest{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.Config{}))

	ws.Route(ws.GET("/{projectName}/configs/{configName}").To(n.detailConfig).
		Doc("detail a config in a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(n.RbacService.CheckPerm("project/config", "list")).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string").Required(true)).
		Param(ws.PathParameter("configName", "identifier of the config").DataType("string").Required(true)).
		Returns(200, "OK", apis.Config{}).
		Reads(apis.UpdateConfigRequest{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.Config{}))

	ws.Route(ws.POST("/{projectName}/distributions").To(n.applyDistribution).
		Doc("apply the distribution job of the config").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(n.RbacService.CheckPerm("project/config", "distribute")).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string").Required(true)).
		Reads(apis.CreateConfigDistributionRequest{}).
		Returns(200, "OK", apis.EmptyResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.GET("/{projectName}/distributions").To(n.listDistributions).
		Doc("list the distribution jobs of the config").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(n.RbacService.CheckPerm("project/config", "distribute")).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string").Required(true)).
		Returns(200, "OK", apis.ListConfigDistributionResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ListConfigDistributionResponse{}))

	ws.Route(ws.DELETE("/{projectName}/distributions/{distributionName}").To(n.deleteDistribution).
		Doc("delete a distribution job of the config").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(n.RbacService.CheckPerm("project/config", "distribute")).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string").Required(true)).
		Param(ws.PathParameter("distributionName", "identifier of the distribution").DataType("string").Required(true)).
		Returns(200, "OK", apis.EmptyResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.GET("/{projectName}/providers").To(n.getProviders).
		Doc("get providers which are in a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(n.RbacService.CheckPerm("project/provider", "list")).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string").Required(true)).
		Returns(200, "OK", apis.ListTerraformProviderResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ListTerraformProviderResponse{}))

	initPipelineRoutes(ws, n)
	ws.Filter(authCheckFilter)
	return ws
}

func (n *project) listProjects(req *restful.Request, res *restful.Response) {
	page, pageSize, err := utils.ExtractPagingParams(req, minPageSize, maxPageSize)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	projects, err := n.ProjectService.ListProjects(req.Request.Context(), page, pageSize)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(projects); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *project) createProject(req *restful.Request, res *restful.Response) {
	// Verify the validity of parameters
	var createReq apis.CreateProjectRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	// Call the domain layer code
	projectBase, err := n.ProjectService.CreateProject(req.Request.Context(), createReq)
	if err != nil {
		klog.Errorf("create project failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(projectBase); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *project) updateProject(req *restful.Request, res *restful.Response) {
	// Verify the validity of parameters
	var updateReq apis.UpdateProjectRequest
	if err := req.ReadEntity(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	// Call the domain layer code
	projectBase, err := n.ProjectService.UpdateProject(req.Request.Context(), req.PathParameter("projectName"), updateReq)
	if err != nil {
		klog.Errorf("update project failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(projectBase); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *project) detailProject(req *restful.Request, res *restful.Response) {
	project, err := n.ProjectService.DetailProject(req.Request.Context(), req.PathParameter("projectName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	// Write back response data
	if err := res.WriteEntity(project); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *project) deleteProject(req *restful.Request, res *restful.Response) {
	err := n.ProjectService.DeleteProject(req.Request.Context(), req.PathParameter("projectName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	// Write back response data
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *project) listProjectTargets(req *restful.Request, res *restful.Response) {
	project, err := n.ProjectService.GetProject(req.Request.Context(), req.PathParameter("projectName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	projects, err := n.TargetService.ListTargets(req.Request.Context(), 0, 0, project.Name)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	// Write back response data
	if err := res.WriteEntity(projects); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *project) createProjectUser(req *restful.Request, res *restful.Response) {
	// Verify the validity of parameters
	var createReq apis.AddProjectUserRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if len(createReq.UserRoles) == 0 {
		bcode.ReturnError(req, res, bcode.ErrProjectRoleCheckFailure)
		return
	}
	// Call the domain layer code
	userBase, err := n.ProjectService.AddProjectUser(req.Request.Context(), req.PathParameter("projectName"), createReq)
	if err != nil {
		klog.Errorf("create project user failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(userBase); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *project) listProjectUser(req *restful.Request, res *restful.Response) {
	page, pageSize, err := utils.ExtractPagingParams(req, minPageSize, maxPageSize)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	// Call the domain layer code
	users, err := n.ProjectService.ListProjectUser(req.Request.Context(), req.PathParameter("projectName"), page, pageSize)
	if err != nil {
		klog.Errorf("list project users failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(users); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *project) updateProjectUser(req *restful.Request, res *restful.Response) {
	// Verify the validity of parameters
	var updateReq apis.UpdateProjectUserRequest
	if err := req.ReadEntity(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if len(updateReq.UserRoles) == 0 {
		bcode.ReturnError(req, res, bcode.ErrProjectRoleCheckFailure)
		return
	}
	// Call the domain layer code
	userBase, err := n.ProjectService.UpdateProjectUser(req.Request.Context(), req.PathParameter("projectName"), req.PathParameter("userName"), updateReq)
	if err != nil {
		klog.Errorf("update project user failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(userBase); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *project) deleteProjectUser(req *restful.Request, res *restful.Response) {
	// Call the domain layer code
	err := n.ProjectService.DeleteProjectUser(req.Request.Context(), req.PathParameter("projectName"), req.PathParameter("userName"))
	if err != nil {
		klog.Errorf("delete project user failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *project) listProjectRoles(req *restful.Request, res *restful.Response) {
	if req.PathParameter("projectName") == "" {
		bcode.ReturnError(req, res, bcode.ErrProjectIsNotExist)
		return
	}
	page, pageSize, err := utils.ExtractPagingParams(req, minPageSize, maxPageSize)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	roles, err := n.RbacService.ListRole(req.Request.Context(), req.PathParameter("projectName"), page, pageSize)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(roles); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *project) createProjectRole(req *restful.Request, res *restful.Response) {
	if req.PathParameter("projectName") == "" {
		bcode.ReturnError(req, res, bcode.ErrProjectIsNotExist)
		return
	}
	// Verify the validity of parameters
	var createReq apis.CreateRoleRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	// Call the domain layer code
	projectBase, err := n.RbacService.CreateRole(req.Request.Context(), req.PathParameter("projectName"), createReq)
	if err != nil {
		klog.Errorf("create role failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(projectBase); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *project) updateProjectRole(req *restful.Request, res *restful.Response) {
	if req.PathParameter("projectName") == "" {
		bcode.ReturnError(req, res, bcode.ErrProjectIsNotExist)
		return
	}
	// Verify the validity of parameters
	var updateReq apis.UpdateRoleRequest
	if err := req.ReadEntity(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	// Call the domain layer code
	roleBase, err := n.RbacService.UpdateRole(req.Request.Context(), req.PathParameter("projectName"), req.PathParameter("roleName"), updateReq)
	if err != nil {
		klog.Errorf("update role failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(roleBase); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *project) deleteProjectRole(req *restful.Request, res *restful.Response) {
	if req.PathParameter("projectName") == "" {
		bcode.ReturnError(req, res, bcode.ErrProjectIsNotExist)
		return
	}
	err := n.RbacService.DeleteRole(req.Request.Context(), req.PathParameter("projectName"), req.PathParameter("roleName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	// Write back response data
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *project) listProjectPermissions(req *restful.Request, res *restful.Response) {
	if req.PathParameter("projectName") == "" {
		bcode.ReturnError(req, res, bcode.ErrProjectIsNotExist)
		return
	}
	policies, err := n.RbacService.ListPermissions(req.Request.Context(), req.PathParameter("projectName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(policies); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *project) createProjectPermission(req *restful.Request, res *restful.Response) {
	// Verify the validity of parameters
	var createReq apis.CreatePermissionRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	// Call the domain layer code
	permissionBase, err := n.RbacService.CreatePermission(req.Request.Context(), req.PathParameter("projectName"), createReq)
	if err != nil {
		klog.Errorf("create the permission failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(permissionBase); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *project) deleteProjectPermission(req *restful.Request, res *restful.Response) {
	err := n.RbacService.DeletePermission(req.Request.Context(), req.PathParameter("projectName"), req.PathParameter("permissionName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	// Write back response data
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *project) getConfigTemplates(req *restful.Request, res *restful.Response) {
	templates, err := n.ConfigService.ListTemplates(req.Request.Context(), req.PathParameter("projectName"), "project")
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	err = res.WriteEntity(apis.ListConfigTemplateResponse{Templates: templates})
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *project) getConfigTemplate(req *restful.Request, res *restful.Response) {
	t, err := n.ConfigService.GetTemplate(req.Request.Context(), pkgconfig.NamespacedName{
		Name:      req.PathParameter("templateName"),
		Namespace: req.QueryParameter("namespace"),
	})
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	err = res.WriteEntity(t)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *project) getConfigs(req *restful.Request, res *restful.Response) {
	configs, err := n.ConfigService.ListConfigs(req.Request.Context(), req.PathParameter("projectName"), req.QueryParameter("template"), false)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	err = res.WriteEntity(apis.ListConfigResponse{Configs: configs})
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *project) createConfig(req *restful.Request, res *restful.Response) {
	// Verify the validity of parameters
	var createReq apis.CreateConfigRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	config, err := n.ConfigService.CreateConfig(req.Request.Context(), req.PathParameter("projectName"), createReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	err = res.WriteEntity(config)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *project) updateConfig(req *restful.Request, res *restful.Response) {
	// Verify the validity of parameters
	var updateReq apis.UpdateConfigRequest
	if err := req.ReadEntity(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	config, err := n.ConfigService.UpdateConfig(req.Request.Context(), req.PathParameter("projectName"), req.PathParameter("configName"), updateReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	err = res.WriteEntity(config)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *project) detailConfig(req *restful.Request, res *restful.Response) {
	config, err := n.ConfigService.GetConfig(req.Request.Context(),
		req.PathParameter("projectName"), req.PathParameter("configName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	err = res.WriteEntity(config)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *project) deleteConfig(req *restful.Request, res *restful.Response) {
	err := n.ConfigService.DeleteConfig(req.Request.Context(), req.PathParameter("projectName"), req.PathParameter("configName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	err = res.WriteEntity(apis.EmptyResponse{})
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *project) getProviders(req *restful.Request, res *restful.Response) {
	providers, err := n.ProjectService.ListTerraformProviders(req.Request.Context(), req.PathParameter("projectName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	err = res.WriteEntity(apis.ListTerraformProviderResponse{Providers: providers})
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *project) applyDistribution(req *restful.Request, res *restful.Response) {
	// Verify the validity of parameters
	var createReq apis.CreateConfigDistributionRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	// Call the domain layer code
	err := n.ConfigService.CreateConfigDistribution(req.Request.Context(), req.PathParameter("projectName"), createReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *project) listDistributions(req *restful.Request, res *restful.Response) {
	distributions, err := n.ConfigService.ListConfigDistributions(req.Request.Context(), req.PathParameter("projectName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	err = res.WriteEntity(apis.ListConfigDistributionResponse{Distributions: distributions})
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *project) deleteDistribution(req *restful.Request, res *restful.Response) {
	err := n.ConfigService.DeleteConfigDistribution(req.Request.Context(), req.PathParameter("projectName"), req.PathParameter("distributionName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	err = res.WriteEntity(apis.EmptyResponse{})
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
