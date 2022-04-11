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

package webservice

import (
	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"

	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/usecase"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

type projectWebService struct {
	rbacUsecase    usecase.RBACUsecase
	projectUsecase usecase.ProjectUsecase
	targetUsecase  usecase.TargetUsecase
}

// NewProjectWebService new project webservice
func NewProjectWebService(projectUsecase usecase.ProjectUsecase, rbacUsecase usecase.RBACUsecase, targetUsecase usecase.TargetUsecase) WebService {
	return &projectWebService{projectUsecase: projectUsecase, rbacUsecase: rbacUsecase, targetUsecase: targetUsecase}
}

func (n *projectWebService) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/projects").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for project manage")

	tags := []string{"project"}

	ws.Route(ws.GET("/").To(n.listprojects).
		Doc("list all projects").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(n.rbacUsecase.CheckPerm("project", "list")).
		Returns(200, "OK", apis.ListProjectResponse{}).
		Writes(apis.ListProjectResponse{}))

	ws.Route(ws.POST("/").To(n.createproject).
		Doc("create a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(n.rbacUsecase.CheckPerm("project", "create")).
		Reads(apis.CreateProjectRequest{}).
		Returns(200, "OK", apis.ProjectBase{}).
		Writes(apis.ProjectBase{}))

	ws.Route(ws.GET("/{projectName}").To(n.detailProject).
		Doc("detail a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string")).
		Filter(n.rbacUsecase.CheckPerm("project", "detail")).
		Returns(200, "OK", apis.ProjectBase{}).
		Writes(apis.ProjectBase{}))

	ws.Route(ws.PUT("/{projectName}").To(n.updateProject).
		Doc("update a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string")).
		Filter(n.rbacUsecase.CheckPerm("project", "update")).
		Reads(apis.UpdateProjectRequest{}).
		Returns(200, "OK", apis.ProjectBase{}).
		Writes(apis.ProjectBase{}))

	ws.Route(ws.DELETE("/{projectName}").To(n.deleteProject).
		Doc("delete a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string")).
		Filter(n.rbacUsecase.CheckPerm("project", "delete")).
		Returns(200, "OK", apis.EmptyResponse{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.GET("/{projectName}/targets").To(n.listProjectTargets).
		Doc("get targets list belong to a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string")).
		Filter(n.rbacUsecase.CheckPerm("project", "detail")).
		Returns(200, "OK", apis.EmptyResponse{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.POST("/{projectName}/users").To(n.createProjectUser).
		Doc("add a user to a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string")).
		Filter(n.rbacUsecase.CheckPerm("project/projectUser", "create")).
		Reads(apis.AddProjectUserRequest{}).
		Returns(200, "OK", apis.ProjectUserBase{}).
		Writes(apis.ProjectUserBase{}))

	ws.Route(ws.GET("/{projectName}/users").To(n.listProjectUser).
		Doc("list all users belong to a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string")).
		Filter(n.rbacUsecase.CheckPerm("project/projectUser", "list")).
		Returns(200, "OK", apis.ListProjectUsersResponse{}).
		Writes(apis.ListProjectUsersResponse{}))

	ws.Route(ws.PUT("/{projectName}/users/{userName}").To(n.updateProjectUser).
		Doc("add a user to a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.UpdateProjectUserRequest{}).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string")).
		Param(ws.PathParameter("userName", "identifier of the project user").DataType("string")).
		Filter(n.rbacUsecase.CheckPerm("project/projectUser", "create")).
		Returns(200, "OK", apis.ProjectUserBase{}).
		Writes(apis.ProjectUserBase{}))

	ws.Route(ws.DELETE("/{projectName}/users/{userName}").To(n.deleteProjectUser).
		Doc("delete a user from a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.UpdateProjectUserRequest{}).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string")).
		Param(ws.PathParameter("userName", "identifier of the project user").DataType("string")).
		Filter(n.rbacUsecase.CheckPerm("project/projectUser", "delete")).
		Returns(200, "OK", apis.EmptyResponse{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.GET("/{projectName}/roles").To(n.listProjectRoles).
		Doc("list all project level roles").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string")).
		Filter(n.rbacUsecase.CheckPerm("project/role", "list")).
		Returns(200, "OK", apis.ListRolesResponse{}).
		Writes(apis.ListRolesResponse{}))

	ws.Route(ws.POST("/{projectName}/roles").To(n.createProjectRole).
		Doc("create project level role").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string")).
		Filter(n.rbacUsecase.CheckPerm("project/role", "create")).
		Returns(200, "OK", apis.RoleBase{}).
		Reads(apis.CreateRoleRequest{}).
		Writes(apis.RoleBase{}))

	ws.Route(ws.PUT("/{projectName}/roles/{roleName}").To(n.updateProjectRole).
		Doc("update project level role").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string")).
		Param(ws.PathParameter("roleName", "identifier of the project role").DataType("string")).
		Filter(n.rbacUsecase.CheckPerm("project/role", "update")).
		Reads(apis.UpdateRoleRequest{}).
		Returns(200, "OK", apis.RoleBase{}).
		Writes(apis.RoleBase{}))

	ws.Route(ws.DELETE("/{projectName}/roles/{roleName}").To(n.deleteProjectRole).
		Doc("delete project level role").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string")).
		Param(ws.PathParameter("roleName", "identifier of the project role").DataType("string")).
		Filter(n.rbacUsecase.CheckPerm("project/role", "delete")).
		Returns(200, "OK", apis.EmptyResponse{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.GET("/{projectName}/permissions").To(n.listProjectPermissions).
		Doc("list all project level perm policies").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string")).
		Filter(n.rbacUsecase.CheckPerm("project/permission", "list")).
		Returns(200, "OK", []apis.PermissionBase{}).
		Writes([]apis.PermissionBase{}))

	ws.Route(ws.GET("/{projectName}/configs/").To(n.getConfigs).
		Doc("get configs which are in a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(n.rbacUsecase.CheckPerm("project", "list")).
		Param(ws.PathParameter("projectName", "identifier of the project").DataType("string")).
		Returns(200, "OK", []*apis.Config{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes([]*apis.Config{}))

	ws.Filter(authCheckFilter)
	return ws
}

func (n *projectWebService) listprojects(req *restful.Request, res *restful.Response) {
	page, pageSize, err := utils.ExtractPagingParams(req, minPageSize, maxPageSize)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	projects, err := n.projectUsecase.ListProjects(req.Request.Context(), page, pageSize)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(projects); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *projectWebService) createproject(req *restful.Request, res *restful.Response) {
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
	// Call the usecase layer code
	projectBase, err := n.projectUsecase.CreateProject(req.Request.Context(), createReq)
	if err != nil {
		log.Logger.Errorf("create project failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(projectBase); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *projectWebService) updateProject(req *restful.Request, res *restful.Response) {
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
	// Call the usecase layer code
	projectBase, err := n.projectUsecase.UpdateProject(req.Request.Context(), req.PathParameter("projectName"), updateReq)
	if err != nil {
		log.Logger.Errorf("update project failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(projectBase); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *projectWebService) detailProject(req *restful.Request, res *restful.Response) {
	project, err := n.projectUsecase.DetailProject(req.Request.Context(), req.PathParameter("projectName"))
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

func (n *projectWebService) deleteProject(req *restful.Request, res *restful.Response) {
	err := n.projectUsecase.DeleteProject(req.Request.Context(), req.PathParameter("projectName"))
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

func (n *projectWebService) listProjectTargets(req *restful.Request, res *restful.Response) {
	project, err := n.projectUsecase.GetProject(req.Request.Context(), req.PathParameter("projectName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	projects, err := n.targetUsecase.ListTargets(req.Request.Context(), 0, 0, project.Name)
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

func (n *projectWebService) createProjectUser(req *restful.Request, res *restful.Response) {
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
	// Call the usecase layer code
	userBase, err := n.projectUsecase.AddProjectUser(req.Request.Context(), req.PathParameter("projectName"), createReq)
	if err != nil {
		log.Logger.Errorf("create project user failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(userBase); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *projectWebService) listProjectUser(req *restful.Request, res *restful.Response) {
	page, pageSize, err := utils.ExtractPagingParams(req, minPageSize, maxPageSize)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	// Call the usecase layer code
	users, err := n.projectUsecase.ListProjectUser(req.Request.Context(), req.PathParameter("projectName"), page, pageSize)
	if err != nil {
		log.Logger.Errorf("list project users failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(users); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *projectWebService) updateProjectUser(req *restful.Request, res *restful.Response) {
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
	// Call the usecase layer code
	userBase, err := n.projectUsecase.UpdateProjectUser(req.Request.Context(), req.PathParameter("projectName"), req.PathParameter("userName"), updateReq)
	if err != nil {
		log.Logger.Errorf("update project user failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(userBase); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *projectWebService) deleteProjectUser(req *restful.Request, res *restful.Response) {
	// Call the usecase layer code
	err := n.projectUsecase.DeleteProjectUser(req.Request.Context(), req.PathParameter("projectName"), req.PathParameter("userName"))
	if err != nil {
		log.Logger.Errorf("delete project user failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *projectWebService) listProjectRoles(req *restful.Request, res *restful.Response) {
	if req.PathParameter("projectName") == "" {
		bcode.ReturnError(req, res, bcode.ErrProjectIsNotExist)
		return
	}
	page, pageSize, err := utils.ExtractPagingParams(req, minPageSize, maxPageSize)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	roles, err := n.rbacUsecase.ListRole(req.Request.Context(), req.PathParameter("projectName"), page, pageSize)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(roles); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *projectWebService) createProjectRole(req *restful.Request, res *restful.Response) {
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
	// Call the usecase layer code
	projectBase, err := n.rbacUsecase.CreateRole(req.Request.Context(), req.PathParameter("projectName"), createReq)
	if err != nil {
		log.Logger.Errorf("create role failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(projectBase); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *projectWebService) updateProjectRole(req *restful.Request, res *restful.Response) {
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
	// Call the usecase layer code
	roleBase, err := n.rbacUsecase.UpdateRole(req.Request.Context(), req.PathParameter("projectName"), req.PathParameter("roleName"), updateReq)
	if err != nil {
		log.Logger.Errorf("update role failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(roleBase); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *projectWebService) deleteProjectRole(req *restful.Request, res *restful.Response) {
	if req.PathParameter("projectName") == "" {
		bcode.ReturnError(req, res, bcode.ErrProjectIsNotExist)
		return
	}
	err := n.rbacUsecase.DeleteRole(req.Request.Context(), req.PathParameter("projectName"), req.PathParameter("roleName"))
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

func (n *projectWebService) listProjectPermissions(req *restful.Request, res *restful.Response) {
	if req.PathParameter("projectName") == "" {
		bcode.ReturnError(req, res, bcode.ErrProjectIsNotExist)
		return
	}
	policies, err := n.rbacUsecase.ListPermissions(req.Request.Context(), req.PathParameter("projectName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(policies); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *projectWebService) getConfigs(req *restful.Request, res *restful.Response) {
	configs, err := n.projectUsecase.GetConfigs(req.Request.Context(), req.PathParameter("projectName"), req.QueryParameter("configType"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if configs == nil {
		if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
			bcode.ReturnError(req, res, err)
			return
		}
		return
	}
	err = res.WriteEntity(configs)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
