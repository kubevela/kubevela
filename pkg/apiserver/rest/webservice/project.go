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
		Filter(n.rbacUsecase.CheckPerm("Project", "List")).
		Returns(200, "OK", apis.ListProjectResponse{}).
		Writes(apis.ListProjectResponse{}))

	ws.Route(ws.POST("/").To(n.createproject).
		Doc("create a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(n.rbacUsecase.CheckPerm("Project", "Create")).
		Reads(apis.CreateProjectRequest{}).
		Returns(200, "OK", apis.ProjectBase{}).
		Writes(apis.ProjectBase{}))

	ws.Route(ws.GET("/{projectName}").To(n.detailProject).
		Doc("detail a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(n.rbacUsecase.CheckPerm("Project", "Detail")).
		Returns(200, "OK", apis.ProjectBase{}).
		Writes(apis.ProjectBase{}))

	ws.Route(ws.DELETE("/{projectName}").To(n.deleteProject).
		Doc("delete a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(n.rbacUsecase.CheckPerm("Project", "Delete")).
		Returns(200, "OK", apis.EmptyResponse{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.GET("/{projectName}/targets").To(n.listProjectTargets).
		Doc("get targets list belong to a project").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(n.rbacUsecase.CheckPerm("Project", "Detail")).
		Returns(200, "OK", apis.EmptyResponse{}).
		Writes(apis.EmptyResponse{}))

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
		log.Logger.Errorf("create application failure %s", err.Error())
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
	project, err := n.projectUsecase.GetProject(req.Request.Context(), req.PathParameter("projectName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	// Write back response data
	if err := res.WriteEntity(usecase.ConvertProjectModel2Base(project)); err != nil {
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
