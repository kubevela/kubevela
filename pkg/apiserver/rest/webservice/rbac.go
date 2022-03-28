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

type rbacWebService struct {
	rbacUsecase usecase.RBACUsecase
}

// NewRBACWebService new rbac webservice
func NewRBACWebService(rbacUsecase usecase.RBACUsecase) WebService {
	return &rbacWebService{rbacUsecase: rbacUsecase}
}

func (r *rbacWebService) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix).
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for rbac")

	tags := []string{"rbac"}

	ws.Route(ws.GET("/roles").To(r.listPlatformRoles).
		Doc("list all platform level roles").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(r.rbacUsecase.CheckPerm("role", "list")).
		Returns(200, "OK", apis.ListRolesResponse{}).
		Writes(apis.ListRolesResponse{}))

	ws.Route(ws.POST("/roles").To(r.createPlatformRole).
		Doc("create platform level role").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(r.rbacUsecase.CheckPerm("role", "create")).
		Returns(200, "OK", apis.RoleBase{}).
		Reads(apis.CreateRoleRequest{}).
		Writes(apis.RoleBase{}))

	ws.Route(ws.PUT("/roles/{roleName}").To(r.updatePlatformRole).
		Doc("update platform level role").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(r.rbacUsecase.CheckPerm("role", "update")).
		Reads(apis.UpdateRoleRequest{}).
		Returns(200, "OK", apis.RoleBase{}).
		Writes(apis.RoleBase{}))

	ws.Route(ws.DELETE("/roles/{roleName}").To(r.deletePlatformRole).
		Doc("update platform level role").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(r.rbacUsecase.CheckPerm("role", "delete")).
		Returns(200, "OK", apis.EmptyResponse{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.GET("/permissions").To(r.listPlatformPermissions).
		Doc("list all project level perm policies").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(r.rbacUsecase.CheckPerm("permission", "list")).
		Returns(200, "OK", []apis.PermissionBase{}).
		Writes([]apis.PermissionBase{}))

	ws.Filter(authCheckFilter)
	return ws
}

func (r *rbacWebService) listPlatformRoles(req *restful.Request, res *restful.Response) {
	page, pageSize, err := utils.ExtractPagingParams(req, minPageSize, maxPageSize)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	roles, err := r.rbacUsecase.ListRole(req.Request.Context(), "", page, pageSize)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(roles); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (r *rbacWebService) createPlatformRole(req *restful.Request, res *restful.Response) {
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
	projectBase, err := r.rbacUsecase.CreateRole(req.Request.Context(), "", createReq)
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

func (r *rbacWebService) updatePlatformRole(req *restful.Request, res *restful.Response) {
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
	roleBase, err := r.rbacUsecase.UpdateRole(req.Request.Context(), "", req.PathParameter("roleName"), updateReq)
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

func (r *rbacWebService) deletePlatformRole(req *restful.Request, res *restful.Response) {
	err := r.rbacUsecase.DeleteRole(req.Request.Context(), "", req.PathParameter("roleName"))
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

func (r *rbacWebService) listPlatformPermissions(req *restful.Request, res *restful.Response) {
	policies, err := r.rbacUsecase.ListPermissions(req.Request.Context(), "")
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(policies); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
